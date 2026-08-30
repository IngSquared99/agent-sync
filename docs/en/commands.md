# Command Reference

## Overview

```
agsy                        interactive menu (with status summary)
agsy init [sources...]      generate / edit agsy.yaml
agsy doctor                 environment health check (read-only)
agsy plan                   preview build + mount results (read-only)
agsy apply                  pre-checks → confirm → wipe & rebuild → mount
agsy status                 report gaps as two lists (read-only; exit 0=in sync 1=gaps)
agsy clean                  remove links and the output (uninstall)
agsy version                version info
agsy help                   usage text
```

**Global flag** `--yes` / `-y`: treat every confirmation as yes. Designed for CI, scripts, git hooks; without it, actions needing confirmation in non-interactive environments are **cancelled, never forced**.

**Common behavior**:

- Except for `init` (current directory only), every command searches **upward** for `agsy.yaml` (same convention as git), so running from a project subdirectory works.
- Interface language follows `AGSY_LANG` / `LC_ALL` / `LANG` (`zh*` → Traditional Chinese).
- When double-clicked on Windows, agsy pauses before the window closes so the output stays readable.
- `apply`, `clean` and `init` guard against concurrent runs in one project with a lock file (`.agsy.lock`, next to `agsy.yaml`), removed when the command finishes. A second run started meanwhile reports the lock and exits; a leftover lock older than 15 minutes (e.g. after a crash) is taken over automatically.

Suggested rhythm per situation:

```
first time              init → doctor → plan → apply
edited sources          (plan) → apply
AI edited a mounted file  status → move what's worth keeping into a source → apply
unsure                  status
removing                clean
```

---

## `agsy` (interactive menu)

Running without arguments opens the menu:

- **No config found**: treats it as first use and leads into `init`.
- **Config found**: a one-line status summary at the top (source changes / artifact-side changes / missing outputs / mount issues), then the apply / plan / status / doctor / init / clean options.

---

## `agsy init [sources...]`

Generates (or edits) `agsy.yaml`.

### Fresh setup

Asks in order: source paths (one per line) → tools to serve (multi-select: Claude Code, OpenAI Codex, Antigravity, Cursor) → conflict strategy per category (mandatory; recommended rules=rename, skills=error, workflows=rename) → output directory (default `.agsy`). After writing:

- If a real `AGENTS.md` already exists at the project root, init says so immediately: agsy mounts its own generated `AGENTS.md` there and never merges or overwrites a real file — move its content into a source `rules/` directory, or rename the file to keep it.
- Offers a checklist of generated paths to add to `.gitignore` (`.agsy/`, the lock file `.agsy.lock`, the mount links, the root `AGENTS.md`); pick the ones you want — some teams version their links on purpose.

### Edit mode (agsy.yaml exists)

- Every question is pre-filled with the current value — **Enter keeps it**.
- Currently served tools come pre-checked; hand-added `build.tools` entries, `categories` edits and custom mount entries are carried over verbatim.
- A **line-by-line diff** is shown before writing, with confirmation; no changes → exits without writing.
- Note: yaml comments are replaced by template comments — re-add custom comments after writing.

### Non-interactive environments (CI / scripts)

Conflict strategies require an explicit choice, so with nobody able to answer, the default is to cancel. For non-interactive use:

```sh
agsy init --yes ~/all-ai-lib ./repo-ai-lib
```

Sources as arguments; `--yes` is the explicit consent to the recommended defaults.

---

## `agsy doctor`

Read-only health check; performs no actions:

1. `agsy.yaml` found and valid.
2. Each source path's existence (missing = ✘ error).
3. Each source's category subdirectories: a missing subdirectory is only a ⚠ note; existing ones get a count of collectible files — **the count follows exactly the same acceptance rules as the build**, and every skipped file is listed with its reason.
4. Each mount point's state: absent (creatable) / already a link / pointing elsewhere or broken (apply repairs) / occupied by a real directory or file (apply will fail; handle manually).
5. **Link capability probe**: an actual temporary link is created and removed.

Ends with `N errors, M warnings`; exit code 1 when errors exist.

---

## `agsy plan`

Rehearses everything apply would do, **guaranteed to write nothing**. Three sections:

- **Build preview**: the source list (priority, existence, tags); per category, the items to collect, who gets renamed, and for each workflow which forms it produces (`skill skills/<name>`, `stub workflows/<name>.md`); the derived `AGENTS.md` line; items dropped by `first`; excluded files with reasons; and every blocker — name conflicts, output-path collisions, target errors — listed in full.
- **Mount preview**: for every link, what will happen — create / delete-and-recreate / repair / blocked by a real path.
- **Summary**: one line of counts.

Exit code: 1 when conflicts / collisions / target errors exist (usable as a CI gate).

---

## `agsy apply`

The wipe → rebuild → mount run.

### 1. Pre-checks (all must pass before any confirmation)

| Check | On failure |
|-------|-----------|
| every source path exists | ✘ refuse: never rebuild from an incomplete source list |
| no mount point occupied by a real directory or file (a pre-existing real `AGENTS.md` included) | ✘ refuse and list them; agsy never deletes what it did not create |
| no target errors / name conflicts / output-path collisions | ✘ refuse; `plan` shows the full list |

The checks deliberately run **before** the discard confirmation: fatal problems must surface before you agree to discard anything.

### 2. The two lists (every run)

- **List A — source changes** (informational): what this apply will sync — updated, added and removed items. A removed source is flagged: its outputs, including derived forms, disappear.
- **List B — artifact-side changes** (needs confirmation): everything changed through the mounts since the last build — modified files, untracked additions, edited derived forms. Each entry carries "to keep it: …" guidance naming the source to move or merge it into. Continuing discards them all; without a TTY and without `--yes`, apply cancels and touches nothing.
- Output directory exists but the manifest is unreadable: contents unknowable, so apply always asks before wiping.

### 3. Build → mount

Wipe the output → copy rules and skills verbatim → convert workflows (skill form + stub) → concatenate `AGENTS.md` → write the manifest → create the links. **If the mount step fails**, the build results remain intact — fix the issue and rerun `agsy apply`.

### 4. Orphan-link report

Links created by an earlier apply that the current config no longer references are listed as a reminder — **reported, never deleted**; remove them manually or via `agsy clean`.

---

## `agsy status`

Read-only. Prints the same two lists apply confirms, plus mount health:

- **List A** — sources → output: content changed / new / deleted (with "the item and its derived forms disappear after apply" flagged), plus missing outputs (deleted output copies; apply rebuilds them). "Source path missing" (repo not cloned, disk not mounted) is clearly separated from "file deleted".
- **List B** — artifact side: everything the next apply will discard, each with its keep-guidance. There is no write-back: to keep a change, move or merge it into a source, then apply.
- **Mounts**: per-link states (fine / missing / wrong target or broken / occupied / orphaned).
- **Summary**: `source changes N │ artifact-side changes N │ missing outputs N │ mount anomalies N` plus the suggested next command.

Exit code `0` = fully in sync; `1` = any gap. Suited to CI / git hooks; without a TTY only the report prints. In an interactive terminal with gaps present, an action menu can jump straight into apply.

---

## `agsy clean`

Uninstall: after confirmation, removes the mount links (the root `AGENTS.md` link included) and the whole output directory; **`agsy.yaml` is kept**. Only agsy-created things are deleted — real directories and files are skipped and reported; orphan links recorded in the manifest are removed too (each path verified to actually be a link into the output before touching it); mount directories left empty by link removal are also removed. `agsy apply` afterwards rebuilds everything.

---

## `agsy version` / `agsy help`

```sh
agsy version    # agsy v1.2.3 (commit …, built …, go version, platform/arch)
agsy help       # usage overview (same as --help / -h)
```

→ Next chapter: [Adapters](adapters.md)
