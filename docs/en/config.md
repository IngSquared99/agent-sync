# Configuration: agsy.yaml

`agsy.yaml` is agsy's only config file. It sits in the project root and is usually written by `agsy init`. This chapter explains every field so you can edit it by hand. For any unfamiliar term, see [Core Concepts](overview.md).

## Full example

```yaml
# paths: ~ = home folder; relative = from the folder holding this file; absolute = as is
version: 2

sources:                      # sources, in order: earlier ones win
  - ~/all-ai-lib
  - ./repo-ai-lib

build:
  out: .agsy                  # output folder, must be inside the project (apply empties it)

  categories:                 # source subfolder → output subfolder
    rules:     { from: rules, to: rules }
    skills:    { from: skills, to: skills }
    workflows: { from: workflows, to: workflows }
    hooks:     { from: hooks, to: hooks }

  on_conflict:                # same-name files: first / rename / error (all four required)
    rules:     rename
    skills:    error
    workflows: rename
    hooks:     error

  tools: [claude, codex, antigravity, cursor]   # tools to serve

mount:
  - dir: .                    # project root: AGENTS.md for Codex / Cursor / Antigravity
    links:
      AGENTS.md: AGENTS.md
  - dir: .claude              # Claude Code
    links:
      rules:  rules
      skills: skills
    merge:                    # hooks merged into the "hooks" field of settings.json
      settings.json: hooks.claude.json
  - dir: .agents              # shared by Codex + Antigravity + Cursor
    links:
      skills:     skills
      workflows:  workflows
      hooks.json: hooks.antigravity.json
  - dir: .codex
    links:
      hooks.json: hooks.codex.json
  - dir: .cursor
    links:
      hooks.json: hooks.cursor.json
```

## How to write paths

| Form | Meaning | For |
|------|---------|-----|
| `~/xxx` | under the home folder | a source library shared across projects |
| `./xxx` or `xxx` | relative to **the folder holding agsy.yaml** (not your current folder) | a source library inside the project |
| `/abs/path` | as is | special setups |

`~` means your own home folder only; `~someone` is an error.

Because relative paths anchor on the config file, running agsy from a project subfolder works the same: every command except `init` searches upward for `agsy.yaml`.

## Field by field

### `version`

Format version of the config file, currently `2`. Omitting it means the current version. A number above what the running agsy supports asks you to upgrade agsy.

### `sources` (required, at least one)

The list of source folders.

- Order is priority: with `on_conflict: first` the earlier source's file survives; `AGENTS.md` is assembled in this order too.
- Common pair: `[~/shared, ./project]`.
- A source missing a subfolder (no `workflows/`, say) is normal.
- When a whole source path is missing, `plan` still previews but `apply` refuses: rebuilding without one source would delete everything that source contributes.
- The same path may not be listed twice, and no source may lie inside another.

Every source gets an automatic **source tag**: the last path segment without its leading dot (`~/all-ai-lib` → `all-ai-lib`). When two tags match, the parent folder name is folded in; a number is appended only if they still match.

### `build.out` (output folder)

Default `.agsy`. `apply` empties it every run and `clean` deletes it, so only a **dedicated subfolder inside the project** is accepted. Rejected: the project root or anything above it, any location outside the project, the home folder, any source's location or anything inside a source.

To change `out`: run `agsy clean` under the **old** config first, then edit `agsy.yaml`, then `agsy apply`.

### `build.categories`

- `from`: the subfolder scanned in every source.
- `to`: the subfolder in the output.
- Setting one side leaves the other at its default.
- The four `from` values must differ, and so must the four `to` values; `to` may not be `AGENTS.md` or `hooks.<tool>.json` (reserved for derived files).
- An existing library with other subfolder names (`prompts/`, say) is connected with `from: prompts`; no files need moving.

Which files are collected:

| Rule | Notes |
|------|-------|
| name starts with `.` | always skipped, not reported (`.DS_Store` and friends) |
| symbolic link | never collected; a link could carry files from outside the source (a private key) into the output |
| irregular file (FIFO, socket, device) | never collected; a skill or hook folder containing one is skipped as a whole |
| rules / workflows | single `.md` files only; over 5 MB is skipped |
| skills | folders only, with a `SKILL.md`; any symbolic link inside skips the whole folder |
| hooks | folders only, with a `hook.yaml`; same rules as skills. The folder name is the hook name: lowercase a–z, 0–9 and hyphens; other names are normalized, with a note in plan |

Files that fail a rule are listed with the reason by `plan` and `doctor`.

### `build.on_conflict` (all four categories required)

What to do when two sources hold a file of the same name. There is no default; the choice is explicit.

| Strategy | Behavior | For |
|----------|----------|-----|
| `rename` | keep both, with the source tag in the name: `python-style.md` → `python-style-fromlib-all-ai-lib.md` | rules |
| `error` | stop and list the conflicts for a person to resolve | skills, hooks |
| `first` | keep only the highest-priority one, drop the rest (plan lists what is dropped) | when "project overrides shared" is certain |

`error` is recommended for skills: with two same-named skills, which one a tool picks is unpredictable, and the pair usually should be merged into one. The same holds for hooks.

Even after rename, two items can still end up at the same **final output path**, across categories too: a workflow named `deploy.md` produces `skills/deploy`, colliding with a skill named `deploy`. That always stops the build.

### `build.tools` (required)

The list of tools to serve. `init` fills it from the tools you pick. A workflow's or hook's `target:` may only name tools listed here; a typo stops the build.

A link to the workflows output requires `antigravity` here (the only tool reading that folder); listing `antigravity` requires a workflows link somewhere.

### A workflow's `target:`

In the workflow's `.md` header (front matter):

```yaml
---
target: [claude, codex]   # or a single one: target: claude
---
```

- Absent: every tool in `build.tools`.
- Present: only the listed tools. Any listed tool other than `antigravity` produces the skill; listing `antigravity` produces the stub in `workflows/`.
- The skills folder is read by several tools, so exclusion is coarse: with a partial list, every tool that mounts skills still sees the skill; plan notes it.
- `target:` is agsy's field and is removed from the output.

## A hook's `hook.yaml`

Every hook folder holds one `hook.yaml`.

### Example

```yaml
description: block rm -rf                     # optional; shown by plan
target: [claude, codex, antigravity, cursor]  # optional; as for workflows: absent = all of build.tools
events:                                       # required; at least one event
  PreToolUse:                                 # event name (table below)
    - matcher: Bash                           # optional; run only when this tool fires
      hooks:                                  # required; at least one handler
        - type: command                       # optional; default command
          command: ./block-rm.sh              # relative to this hook folder
          timeout: 10                         # optional; other fields pass through
      overrides:                              # optional; when one tool differs
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
```

The shape of a hook:

```
 events
 └── event (which moment)
     └── group (optional matcher: which tool fires it)
         ├── hooks: one or more handlers (what runs)
         └── overrides: exceptions for one tool
```

### Event names

Events use the Claude Code / Codex names; agsy translates for the other two. "—" means that tool has no such moment; it is skipped, with a note in plan.

| Event | claude | codex | antigravity | cursor |
|---|---|---|---|---|
| `PreToolUse` | ✓ | ✓ | ✓ | `preToolUse` |
| `PostToolUse` | ✓ | ✓ | ✓ | `postToolUse` |
| `Stop` | ✓ | ✓ | ✓ | `stop` |
| `SessionStart` / `SessionEnd` | ✓ | ✓ | — | `sessionStart` / `sessionEnd` |
| `UserPromptSubmit` | ✓ | ✓ | — | `beforeSubmitPrompt` |
| `PermissionRequest` | ✓ | ✓ | — | — |
| `SubagentStart` / `SubagentStop` | ✓ | ✓ | — | `subagentStart` / `subagentStop` |
| `PreCompact` | ✓ | ✓ | — | `preCompact` |
| `PostCompact` | ✓ | ✓ | — | — |
| `Interrupt` | — | ✓ | — | — |
| `PostToolUseFailure` | ✓ | — | — | `postToolUseFailure` |
| `StopFailure` | ✓ | — | — | — |
| `PreInvocation` / `PostInvocation` | — | — | ✓ | — |

### Handler types

| Tool | Supported `type` |
|------|------------------|
| claude | command, http, mcp_tool, prompt, agent |
| codex | command, mcp_tool |
| antigravity | command |
| cursor | command, prompt |

An unsupported handler is skipped for that tool and reported.

### Translation rules

1. Tools not in `target` get nothing in their registry; plan notes which are left out.
2. Events the tool lacks and unsupported types are skipped and noted. Antigravity honours a matcher on `PreToolUse` / `PostToolUse` only; a matcher on another event is dropped with a note. When no event of a hook maps to a tool, the registry is still written (empty).
3. `overrides.<tool>`: `matcher` replaces the group's matcher; `hooks[i]` overrides fields of the i-th handler. An index past the group's handlers, or an override that leaves a command handler without a `command`, is an error. An override that changes `type` away from `command` drops the `command` field for that tool, with a note. Override keys only need to be one of the four tools agsy knows; ones not in `build.tools` are ignored; an unknown name is an error.
4. Other handler fields pass through as written. Cursor's shape differs and keeps only `timeout`, `failClosed`, `loop_limit` and `prompt`; the rest is dropped with a note.

### Path rewriting

Every whitespace-separated token of `command` that starts with `./` is rewritten to an absolute path into `.agsy/hooks/<name>/…`; the rest is kept as written (`python3 ./check.py` works).

```
 hook.yaml     command: python3 ./check.py
                                 │
                                 ▼ apply
 registry      command: python3 /Users/me/proj/.agsy/hooks/block-rm/check.py
```

- A rewritten path with spaces or special characters is quoted automatically. **Do not quote a `./` path yourself** (`"./x.sh"` is refused).
- The referenced file must exist inside the hook folder, or apply refuses; override commands included.
- The registry holds this machine's absolute paths; every machine runs its own apply.

### Scripts handle vendor differences themselves

agsy translates the registry, not the script. The JSON each tool feeds a script differs (Claude / Codex use `tool_input.command`; Cursor and Antigravity have their own). A script meant for all four checks `hook_event_name` or the tool's fields itself.

## `mount` (required, at least one)

Each entry: `dir` is the folder the links go in (`.` is the project root); `links` maps `link name: top-level name in the output`.

- A link may point at a category's `to` value, at `AGENTS.md`, or at one of the four registries `hooks.<tool>.json` (that tool must be in `build.tools`). Only top-level names.
- Several entries with the same `dir` are merged; same-named links pointing at different targets are an error.
- `dir` may not lie inside the output (the links would vanish when apply empties it) or inside a source (the links would be scanned as source content); a link name may not contain path separators.
- A `dir` outside the project folder requires `outside_project: true` on that entry. This is a safety gate: an `agsy.yaml` in a cloned repository must not reach your home folder's settings without you knowing. Add it only to configs you wrote yourself.
- Link implementation: relative symlinks on macOS / Linux. On Windows, junctions for folders and hard links for files, neither needing administrator rights; a junction stores an absolute path, so rerun `agsy apply` after moving the project.

## `merge`: Claude Code only

```yaml
  - dir: .claude
    merge:
      settings.json: hooks.claude.json
```

### Why not a link

Claude Code's hooks live in the `hooks` field of `.claude/settings.json`, a file that also holds your permissions, model and other settings. A link would replace the whole file, so agsy merges only its own entries into it.

```
 .claude/settings.json (your file)
 {
   "permissions": {…},         ← untouched
   "hooks": {
     "Stop": [ yours ],         ← untouched
     "PreToolUse": [ agsy's ]   ← only this is replaced
   }
 }
```

### How agsy's entries are recognized

A group under `hooks` counts as agsy's when any of its handlers matches either clue:

- `statusMessage` starts with `agsy:`. Every handler agsy writes carries `agsy:<hook name>`; a `statusMessage` from `hook.yaml` follows it (`agsy:block-rm · linting`).
- The `command` path points into `.agsy/hooks/` (the current output folder, or the one recorded in the manifest).

The mark is path-independent, so groups survive a moved project, a renamed `build.out`, and a command without any `./` path.

### What each command does to a merge target

| Command | Action |
|---------|--------|
| apply | removes the old agsy groups and adds the registry's; every other field and group is kept in order (the file is re-indented with two spaces). A missing file is created and recorded as agsy-created. With nothing in the registry and no agsy groups in the file, the file is neither created nor rewritten |
| status | an edited agsy group → listed as an artifact-side change; a deleted one → listed as missing. Your own groups are never an anomaly. A file dropped from the config but still holding agsy groups → reported as an orphan |
| clean | removes the agsy groups; the `hooks` field and event arrays apply introduced go with them, pre-existing empty containers stay; the file is deleted only if agsy created it and it is now empty |

A target that is a symbolic link, or not a JSON object, stops apply before the build and is skipped by clean with a report.

### Limits

- The `merge` value may only be `hooks.claude.json` or `hooks.codex.json` (merge reads back only those shapes), and that tool must be in `build.tools`.
- `merge` and `links` in one `dir` may not share a name; one registry may not be both linked and merged.
- A `dir` carrying merge entries must lie inside the project; `outside_project: true` does not apply to merge. agsy owns the project-level file only.
- An entry with only `merge` and no `links` is valid.

### Where your own hooks go

Each tool's personal file, layered on top of the project file: Claude Code `.claude/settings.local.json` or `~/.claude/settings.json`; Codex `~/.codex/hooks.json` or `config.toml`; Cursor `~/.cursor/hooks.json`; Antigravity `~/.gemini/config/` (merging behavior is not documented by the vendor; test it).

### The reverse check is only a hint

Linking `hooks.<tool>.json` while that tool is not in `build.tools` is an error (the file would stay empty). The reverse, a tool listed without its registry mounted, is not an error; `plan` and `doctor` print a hint only when the sources actually contain hooks.

## Error quick reference

Validation reports every problem at once. Common messages:

| Message | Fix |
|---------|-----|
| `sources is not set; at least one source path is required` | add at least one source |
| `build.on_conflict.rules is not set…` | set a strategy for all four categories |
| `build.on_conflict.hooks is not set…` | add `hooks: error` under `on_conflict`, or rerun `agsy init` |
| `build.categories.x.to must not be "hooks.codex.json"…` | that name is reserved for a registry; pick another |
| `mount … merge.settings.json points to "z", but only a hook registry of the claude / codex shape can be merged` | merge may only point at `hooks.claude.json` or `hooks.codex.json` |
| `mount dir … resolves outside the project directory (…), but it carries merge entries` | merge targets must be inside the project |
| `mount … merge.x and link y both consume "z"` | a registry is either linked or merged |
| `mount … links.hooks.json points to "hooks.cursor.json", but build.tools does not list "cursor"` | add `cursor` to `build.tools` or drop the link |
| `build.tools is not set…` | list the tools, e.g. `[claude, codex, antigravity, cursor]` |
| `build.out (…) is not inside the project directory (…)` | use a dedicated folder inside the project |
| `build.categories.x.to and y are both "…"` | give one of them a different `to` |
| `mount … links.x points to "z", but the output has no such top level` | the target must be a category's `to`, `AGENTS.md` or a registry |
| `sources … resolve to the same directory` / `… are nested` | list each source once; un-nest them |
| `mount dir … resolves outside the project directory` | if intended, add `outside_project: true` to that entry |
| `a mount links to the workflows output …, but build.tools does not list "antigravity"` | add it to `build.tools`, or drop the workflows link |
| `version: N exceeds the maximum … supported by this agsy` | upgrade agsy |

→ Next: [Command Reference](commands.md)
