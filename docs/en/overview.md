# Core Concepts: What is agsy?

## The problem it solves

When you use several AI development tools at once (Claude Code, OpenAI Codex, Google Antigravity, Cursor…), each tool reads its own instruction locations, in its own shapes:

- Claude Code reads rules from `.claude/rules/` and skills from `.claude/skills/`
- Codex reads the root `AGENTS.md` and skills from `.agents/skills/`
- Antigravity reads the root `AGENTS.md`, skills from `.agents/skills/`, and `/name` workflows from `.agents/workflows/`
- Cursor reads the root `AGENTS.md` and skills from `.agents/skills/`
- Each tool's hooks (agent lifecycle guards) live in yet another place and shape: `.claude/settings.json`, `.codex/hooks.json`, `.agents/hooks.json`, `.cursor/hooks.json`

The same coding conventions, skills, procedures and guards end up copied several times, in several formats, and every change has to be synced to every copy. On top of that you often want a personal shared library layered with a per-project one.

**agsy (agent-sync)** solves exactly this:

> It **merges** instruction files from multiple sources into a single build output directory (`.agsy/` by default), **converts** them into each tool's native shape, then **mounts** the output into each tool's read location via links.

Edit only the sources, run `agsy apply` once, and every tool updates at the same time.

## Know these terms first

The rest of the documentation uses these terms throughout.

| Term | Meaning |
|------|---------|
| source | An original instruction library you maintain (the `sources` array); there can be several |
| output (artifacts) | The directory `apply` builds, `.agsy/` by default. Everything inside is generated and the whole directory can be rebuilt |
| mount | Creating a "link" at each tool's read location that points into the output |
| link (symlink / junction / hard link) | An OS-level pointer at another directory or file — **no second copy of the content exists** |
| category | The four kinds of instruction files: rules, skills, workflows, hooks |
| derived form | An output produced by conversion rather than verbatim copy: the concatenated `AGENTS.md`, a workflow's skill form, a workflow's redirect stub, each tool's hook registry |
| hook registry | `.agsy/hooks.<tool>.json`, one per tool, translated from every hook's `hook.yaml` |
| merge | The mount mode used only for Claude Code: the registry is merged into the `hooks` key of `.claude/settings.json`, everything else in that file untouched |
| manifest | `.agsy/.agsy-manifest.json`, the build record; agsy uses it to tell what changed on which side |
| source tag | The source identifier appended to a filename when same-name items are kept via rename, e.g. `-fromlib-all-ai-lib` |
| adapter | A built-in mount preset for a tool, used by `init` to generate the mount config |
| tools | The `build.tools` list; the closed set of names a workflow's or hook's `target:` may reference |

## One-way data flow

agsy's data flow is strictly one way:

```
 sources you maintain ──▶ build (copy + convert) ──▶ .agsy/ ──▶ mount (links / merge) ──▶ each tool
```

**The sources are the single source of truth. Everything in `.agsy/` — and therefore everything a tool reads through a mount — is a read-only, rebuildable artifact.** There is no write-back: when a mounted file is edited, `status` reports the change and `apply` lists it, asks for confirmation, and rebuilds over it. Keeping a change always means moving it into a source by hand (status names the destination), so AI-authored content passes human review before entering the library.

## The three layers

```
┌─────────────────────┐
│  sources             │  ~/all-ai-lib/       (personal shared library)
│  (originals you      │  ./repo-ai-lib/      (in-project library)
│   maintain)          │
└─────────┬───────────┘
          │  ① agsy apply: scan → merge → copy → convert
          ▼
┌─────────────────────┐
│  output              │  .agsy/rules/        rules, verbatim
│  (rebuildable,       │  .agsy/AGENTS.md     rules, concatenated (derived)
│   read-only)         │  .agsy/skills/       skills + workflow skill forms
│                      │  .agsy/workflows/    workflow stubs
│                      │  .agsy/hooks/        hooks, verbatim (scripts + hook.yaml)
│                      │  .agsy/hooks.*.json  hook registries, one per tool (derived)
└─────────┬───────────┘
          │  ② agsy apply: create links (Claude's hooks: merge)
          ▼
┌─────────────────────┐
│  mount               │  AGENTS.md          → .agsy/AGENTS.md
│  (where AI tools     │  .claude/rules      → .agsy/rules
│   actually read)     │  .claude/skills     → .agsy/skills
│                      │  .claude/settings.json ⇐ .agsy/hooks.claude.json (merge)
│                      │  .agents/skills     → .agsy/skills
│                      │  .agents/workflows  → .agsy/workflows
│                      │  .agents/hooks.json → .agsy/hooks.antigravity.json
│                      │  .codex/hooks.json  → .agsy/hooks.codex.json
│                      │  .cursor/hooks.json → .agsy/hooks.cursor.json
└─────────────────────┘
```

- **Sources**: the originals you maintain and version-control. Order = priority (earlier wins).
- **Output** (`build.out`, `.agsy/` by default): the built product. `apply` wipes and rebuilds it every time.
- **Mount**: links at each tool's read location. Directories are symlinks (junctions on Windows); the root `AGENTS.md` and the three hook registry files are file symlinks (hard links on Windows). Tools see links; the content lives in `.agsy/`. The one exception is Claude Code's hooks: it has no separate file, so agsy **merges** the registry into the `hooks` key of `.claude/settings.json`, owning only the entries it wrote.

## The four categories

Instruction files are split into four categories by purpose. Sources store them in four subdirectories (`rules/`, `skills/`, `workflows/`, `hooks/` by default), each with its own format rules:

| Category | Source format | What it is | Output forms |
|----------|--------------|------------|--------------|
| rules | single `.md` files | long-lived conventions and style guides, always in context | verbatim copies in `rules/` (for Claude Code) **and** one concatenated `AGENTS.md` (for Codex / Cursor / Antigravity) |
| skills | **directory** containing `SKILL.md` | a packaged capability; tools pick it up when a task matches its description | verbatim copies in `skills/` |
| workflows | single `.md` files | human-triggered procedures and SOPs, invoked as `/name` | a **skill form** in `skills/` (front matter gains `disable-model-invocation: true`, so tools that honor it never run the procedure on their own) **and** a redirect **stub** in `workflows/` for Antigravity's `/name` |
| hooks | **directory** containing `hook.yaml` (scripts alongside) | programs attached to the agent lifecycle: block a tool call before it runs, refuse to stop, check afterwards — the model cannot choose to ignore them | verbatim copies in `hooks/` **and** one **registry** per tool, `hooks.<tool>.json` (event names and structure translated per vendor; script paths rewritten to absolute paths into the output) |

### rules teach, hooks enforce

A rule is text placed in the model's context: the model has read it, but whether it complies is probabilistic. A hook is a program running outside the agent: at a hook point (say `PreToolUse`) the agent pauses, hands the situation to your script as JSON, and if the script exits with code 2 the action simply does not happen. Anything expressible as an `if` (files that must not be touched, checks that must run, conditions under which the agent may not stop) belongs in hooks; style and preference, which no program can judge, stay in rules. Use both layers; which one a given rule goes into is your call.

Why workflows become skills: Claude Code, Codex and Cursor read commands and procedures through the skills mechanism, gated by front matter rather than by a separate directory. A workflow source stays a plain single file; build packages it into the skill shape those tools expect. Antigravity reads a `workflows/` directory for `/name` invocation, so build leaves a short stub there that tells the agent to load the corresponding skill (the content exists exactly once, in the skill form).

## What each tool ends up reading

| Tool | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/` (per-file) | `.claude/skills/` | skill form in `.claude/skills/`, invoked as `/name` | `hooks` key of `.claude/settings.json` (merge) |
| Codex | root `AGENTS.md` | `.agents/skills/` | skill form in `.agents/skills/` | `.codex/hooks.json` |
| Antigravity | root `AGENTS.md` | `.agents/skills/` | stub in `.agents/workflows/` → `/name` loads the skill | `.agents/hooks.json` |
| Cursor | root `AGENTS.md` | `.agents/skills/` | skill form in `.agents/skills/`, invoked as `/name` | `.cursor/hooks.json` |

`.agents/skills/` is one directory natively read by Codex, Antigravity and Cursor — one link serves all three. Claude Code does not read `AGENTS.md`, which is why it alone gets the per-file `.claude/rules/` mount.

## Source directories must follow the naming convention

By default agsy scans `rules/`, `skills/`, `workflows/` and `hooks/` inside each source. A library that has always used different names can be connected without moving files via `build.categories.<cat>.from` — see [Configuration](config.md).

→ Next chapter: [Installation](install.md)
