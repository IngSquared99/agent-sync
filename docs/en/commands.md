# Command Reference

## Overview

```
agsy                        interactive menu (with status summary)
agsy init [sources...]      generate / edit agsy.yaml
agsy doctor                 environment health check (read-only)
agsy plan                   preview build + mount results (read-only)
agsy apply                  pre-checks → confirm → wipe & rebuild → mount
agsy status                 three-way comparison, report gaps (read-only; exit 0=in sync 1=gaps)
agsy promote [item] [--to <source>] [--all]
                            write artifact-end changes back to the sources
agsy clean                  remove links and artifacts (uninstall)
agsy version                version info
agsy help                   usage text
```

**Global flag** `--yes` / `-y`: treat every confirmation as yes. Designed for CI, scripts, git hooks; without it, actions needing confirmation in non-interactive environments are **cancelled, never forced**.

**Common behavior**:

- Except for `init` (current directory only), every command searches **upward** for `agsy.yaml` (same convention as git), so running from a project subdirectory works.
- Interface language follows `AGSY_LANG` / `LC_ALL` / `LANG` (`zh*` → Traditional Chinese).
- When double-clicked on Windows, agsy pauses before the window closes so the output stays readable.

Suggested rhythm per situation:

```
first time            init → doctor → plan → apply
edited sources        (plan) → apply
edited artifact end   status → promote → apply
unsure                status
removing              clean
```

---

## `agsy` (interactive menu)

Running without arguments opens the menu:

- **No config found**: treats it as first use and leads into `init`.
- **Config found**: a one-line status summary at the top (counts of behind / new / local changes / untracked / missing outputs / mount issues), then the apply / plan / promote / status / doctor / init / clean options. With local changes present, the apply option is annotated "confirmation required first".

Every feature is reachable from here — no commands to memorize.

---

## `agsy init [sources...]`

Generates (or edits) `agsy.yaml`.

### Fresh setup

Asks in order: source paths (one per line) → tools to mount (multi-select from built-in adapters) → conflict strategy per category (mandatory; recommended rules=rename, skills=error, workflows=rename) → output directory (default `.agsy`) → destination for untargeted workflows. (Full screen walkthrough and per-question guidance: [Quick Start Step 1](quickstart.md#step-1-agsy-init--answer-five-questions).) After writing:

- Warns when a category would be built but no selected tool mounts it (e.g. Codex-only leaves rules/skills unread).
- Offers to add the output directory to `.gitignore` (skipped silently when the entry exists).

### Edit mode (agsy.yaml exists)

- Every question is pre-filled with the current value — **Enter keeps it**.
- Currently mounted tools come pre-checked.
- **Hand-added content survives**: custom mount entries, hand-added buckets, modified `categories` and `route.field` are all carried over verbatim.
- A **line-by-line diff** is shown before writing, with confirmation; no changes → exits without writing.
- Note: yaml comments are replaced by template comments — re-add custom comments after writing (version-controlling agsy.yaml exists for exactly this moment).

### Non-interactive environments (CI / scripts)

The core of init is that conflict strategies require an explicit choice, so with nobody able to answer, the default is to cancel. For non-interactive use:

```sh
agsy init --yes ~/all-ai-lib ./repo-ai-lib
```

Sources as arguments; `--yes` is the explicit consent to the recommended defaults (rules=rename, skills=error, workflows=rename).

---

## `agsy doctor`

Read-only health check; performs no actions:

1. `agsy.yaml` found and valid.
2. Each source path's existence (missing = ✘ error).
3. Each source's category subdirectories: a missing subdirectory is only a ⚠ note (not an error); existing ones get a count of collectible files — **the count follows exactly the same acceptance rules as the build**, and every skipped file is listed with its reason (e.g. "skill directory has no SKILL.md", "extension is not .md").
4. Each mount point's state: absent (creatable) / already a link / pointing elsewhere or broken (apply repairs) / occupied by a real directory (apply will fail; handle manually).
5. **Link capability probe**: an actual temporary link is created and removed — on Windows this verifies junction support (no privileges needed).

Ends with `N errors, M warnings`; exit code 1 when errors exist.

**Use it for**: verifying the environment after installation, "why wasn't my file collected", checking source paths after switching machines.

---

## `agsy plan`

Rehearses everything apply would do, **guaranteed to write nothing**. Three sections:

- **Build preview**: the source list (priority, existence, tags — a missing source is marked "the preview below EXCLUDES this source"); per category, the items to collect, who gets renamed (with tag), which buckets each workflow routes to and why; items dropped by `first`; excluded files with reasons; and every blocker — name conflicts, final-name collisions, routing errors — listed in full.
- **Mount preview**: for every link, what will happen — create / delete-and-recreate / repair / blocked by a real directory (apply would fail).
- **Summary**: one line — items │ renamed │ conflicts │ collisions │ dropped │ excluded │ links │ mount anomalies.

Exit code: 1 when conflicts / collisions / routing errors exist (usable as a CI gate).

---

## `agsy apply`

The real "wipe, rebuild, mount" run.

### 1. Pre-checks (all must pass before any confirmation)

| Check | On failure |
|-------|-----------|
| every source path exists | ✘ refuse: never rebuild from an incomplete source list (missing sources' items would be wiped) |
| no mount point occupied by a real directory/file | ✘ refuse and list them; move them yourself (agsy never deletes what it did not create) |
| no routing errors / name conflicts / collisions | ✘ refuse; `plan` shows the full list |

> The checks deliberately run **before** the discard confirmation: fatal problems must surface before you agree to discard local changes and wait through a doomed-to-fail build.

### 2. Confirm local changes (every run)

- Manifest readable: items compared one by one. **Local changes** (artifacts edited, not yet promoted) or **untracked files** (added on the artifact end; the rebuild deletes them) are listed with a "discard and rebuild?" confirmation.
- Artifact directory exists but the manifest is unreadable: its contents are unknowable, so it always asks.
- Artifact directory absent: first build, nothing can be overwritten — proceeds directly.

### 3. Build → mount

Wipe the artifacts → copy per plan (renamed skills get their `SKILL.md` name rewritten) → ensure every link target directory exists (even with zero workflows, `workflows/<bucket>/` is created so no link dangles) → write the manifest → create the links. **If the mount step fails** (e.g. the filesystem rejects links), the build results remain intact — fix the issue and rerun `agsy apply`; only the mount step was incomplete.

### 4. Orphan-link report

apply records the links it creates. Links created by an earlier apply that the current config no longer references (e.g. a tool removed from mount) are listed as a reminder — **reported, never deleted**; remove them manually or via `agsy clean`.

> The full matrix of apply's behavior under every combination of changes (who edited, who deleted, who added) is in the [Scenario Guide](scenarios.md).

---

## `agsy status`

Three-way comparison (sources ↔ manifest ↔ artifacts/mounts), always read-only.

### Report structure

**① sources → artifacts (apply's direction)**

- `behind`: source updated, artifacts not rebuilt. Two sub-states are called out: "source file deleted ⚠" (the item disappears after apply) and "source path missing ⚠" (shared repo not cloned? disk not mounted? — fix the path first, do not rush to apply).
- `new`: new files in the sources, not yet in the artifacts.
- `missing artifacts`: artifact copies were deleted (unreadable through the mounts; apply rebuilds them).

**② artifacts → sources (promote's direction)**

- `local changes`: artifacts differ from when built; can be written back. Possible annotations: ⚠ the source changed as well (manual comparison needed) / ⚠ multiple bucket copies changed to different contents / ⚠ the source file was deleted (promote would recreate it) / ⚠ the whole source root is missing (promote refuses).
- `untracked`: files added on the artifact end, unknown to the manifest — **promote cannot write them back (no origin), apply deletes them**; move them into a source to keep them.

**③ mounts + ④ summary**

Per-link states (fine / missing / wrong target or broken / occupied by a real directory / orphaned), then a one-line count plus the suggested next command (e.g. "run agsy promote first to keep the changes, then agsy apply to rebuild").

### Exit code and action menu

- `0` = fully in sync; `1` = any gap. Suited to CI / git hooks; without a TTY only the report prints.
- In an interactive terminal with gaps present, an action menu at the end jumps straight into promote / apply.

---

## `agsy promote`

Writes artifact-end changes (the files tools actually read and write through the mount links) **back to the sources**:

```sh
agsy promote                          # interactive multi-select; per-item destination override
agsy promote skills/code-review       # single item ("category/name")
agsy promote code-review              # bare name works too; ambiguity across categories lists the full forms
agsy promote skills/code-review --to ./repo-ai-lib   # single item, redirected to another source
agsy promote --all                    # everything back to its original source (list first, then confirm)
```

`--to` only accepts sources **configured in agsy.yaml**; `--all --to` is not supported.

### Protection rules (judged per item; one failure never aborts the batch)

| Situation | Behavior |
|-----------|----------|
| the source changed as well (both ends edited) | ✘ refuse — overwriting would destroy the source's new content; merge manually |
| multiple bucket copies changed to **different** contents | ✘ refuse — a human picks the right one (identical edits count as one change and write back normally) |
| the source file was deleted | ⚠ asks explicitly: "recreate it?" |
| the whole source root is missing | ✘ refuse — fix the source path first |
| destination outside the project (a shared library) | ⚠ extra confirmation — it affects every project using that library |

### Automatic follow-ups after writing back

- Renamed skills: the `SKILL.md` `name` is **restored to the original** (the `-fromlib-…` name belongs only to the artifacts).
- Multi-bucket workflows: all copies are synced to the written-back content.
- The manifest baselines are refreshed: the promoted change is no longer reported; run `apply` next to complete the cycle.
- With `--to`: a reminder that **the old source still holds the item** — the next build would pick up both same-name copies; delete the old one once the new location is verified.

> The full matrix of when promote writes back versus refuses is in the [Scenario Guide](scenarios.md).

---

## `agsy clean`

Uninstall: after confirmation, removes the mount links and the whole artifact directory; **`agsy.yaml` is kept**. Only agsy-created things are deleted — real directories/files are skipped and reported; orphan links recorded in the manifest are removed too (each path verified to actually be a link into the artifacts before touching it); mount directories left empty by link removal are also removed. `agsy apply` afterwards rebuilds everything.

---

## `agsy version` / `agsy help`

```sh
agsy version    # agsy v1.2.3 (commit …, built …, go version, platform/arch)
agsy help       # usage overview (same as --help / -h)
```

→ Next chapter: [Adapters](adapters.md)
