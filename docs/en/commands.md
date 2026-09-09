# Command Reference

## Overview

```
agsy                        menu with a status summary
agsy init [sources...]      write or edit agsy.yaml
agsy doctor                 environment check (read-only)
agsy plan                   preview what apply would do (read-only)
agsy apply                  check → confirm → rebuild the output → mount
agsy status                 report gaps (read-only; exit 0 = in sync, 1 = gaps)
agsy clean                  remove links and output
agsy version                version
agsy help                   usage
```

Which to use when:

```
 first time                    init → doctor → plan → apply
 edited a source               apply (plan first if unsure)
 an AI edited a mounted file   status → move what to keep into a source → apply
 unsure of the state           status
 done with agsy                clean
```

Common to every command:

- Except `init` (current folder only), every command searches upward for `agsy.yaml`, so running from a project subfolder works.
- `--yes` (or `-y`): every confirmation counts as "yes". For CI, scripts and git hooks. Without it, an action needing confirmation is cancelled when nobody can answer (no terminal).
- Interface language follows `AGSY_LANG` / `LC_ALL` / `LANG`; see [Installation](install.md).
- On Windows, when launched by double-click, agsy pauses before the window closes so the output stays readable.
- `apply`, `clean` and `init` create `.agsy.lock` next to `agsy.yaml` while running, so two commands cannot run in one project at once; it is removed on exit. A lock older than 15 minutes (left by a crash) is taken over.

## `agsy` (menu)

Run without arguments:

- No `agsy.yaml` found: leads into `init`.
- Found: the first line is a status summary (counts of source changes, artifact-side changes, missing outputs, mount anomalies), followed by apply / plan / status / doctor / init / clean.

## `agsy init`

Writes or edits `agsy.yaml`.

### First time

Four questions in order:

1. Source paths, one per line.
2. Tools to serve (multi-select).
3. The same-name conflict strategy per category (required; recommended rules=rename, skills=error, workflows=rename, hooks=error).
4. Output folder (default `.agsy`).

After writing:

- If a real `AGENTS.md` already exists in the project root, a reminder: agsy will place a link there and does not merge or overwrite real files. Move the content into a source's `rules/`, or rename the file to keep it.
- A `.gitignore` checklist (`.agsy/`, `.agsy.lock`, each link, the root `AGENTS.md`) to pick from; `settings.json` is your file and is not listed.

### Editing (agsy.yaml exists)

- Every question is prefilled with the current value; Enter keeps it.
- Tools already served are preselected; a hand-edited `build.tools`, `categories` and custom mount entries are carried over as is.
- A line-by-line diff is shown and confirmed before writing; no change, no write.
- Comments in the yaml are replaced by the template's.

### Non-interactive (CI, scripts)

```sh
agsy init --yes ~/all-ai-lib ./repo-ai-lib
```

Sources come from the arguments; `--yes` accepts the recommended defaults.

## `agsy doctor`

Read-only health check, in order:

1. `agsy.yaml` is found and well-formed.
2. Every source path exists (missing = ✘).
3. Each source's subfolders: a missing one is only ⚠; an existing one is counted with the same rules as build, skipped files listed with the reason.
4. Every mount point: absent (can be created) / already a link / pointing elsewhere or broken (apply repairs) / occupied by a real folder or file (apply fails; handle by hand). Every merge target: absent (apply creates it) / a JSON object (can be merged) / a symbolic link or not a JSON object (apply fails).
5. hooks: whether scripts have the executable bit on macOS / Linux (hint `chmod +x` when not; files run through an interpreter are not checked); tools in `build.tools` whose registry nothing mounts (only when the sources contain hooks).
6. Link capability: a temporary link is created and removed.

Ends with `N errors, M warnings`; exit code 1 when there are errors.

## `agsy plan`

Lists everything apply would do, writing nothing. Three parts:

**Build preview**: the sources (priority, existence, tag); per category the items collected, who is renamed, what each workflow produces (`skill skills/<name>`, `stub workflows/<name>.md`); each hook's description, the tools it reaches (`claude ✓  codex ✓  antigravity —  cursor ✓`) and why not; one line per derived file; items dropped by `first`; files failing the rules with the reason; every problem that would stop the build.

**Mount preview**: what happens to each link (create / recreate / repair / occupied) and each merge target (create / merge / update existing entries / edited entries will be asked about / not a JSON object fails).

**Summary**: one line of counts.

Exit code 1 with conflicts, collisions, target or `hook.yaml` errors; usable as a CI gate.

## `agsy apply`

The real run: empty the output → rebuild → mount.

### 1. Pre-checks

All must pass before any confirmation:

| Check | On failure |
|-------|-----------|
| every source path exists | ✘ refuses |
| no mount point occupied by a real folder or file (including a real root `AGENTS.md`, a real `.codex/hooks.json`, …) | ✘ refuses and lists them; nothing is deleted for you |
| every merge target is not a symbolic link and is a JSON object (or absent) | ✘ refuses and lists them |
| no target or `hook.yaml` errors, name conflicts, output path collisions | ✘ refuses; `plan` has the full list |

### 2. Two lists

- **List A: source changes** (informational): the updates, additions and removals this run syncs. An item deleted from its source is marked; all its outputs disappear with it.
- **List B: artifact-side changes** (confirmed): everything changed through a link since the last build: edited files, new files, edited derived files, edited agsy entries in `settings.json`. Each carries "to keep: move to <source>". Continuing discards them all; without a terminal and without `--yes`, apply cancels.
- Output folder present but manifest unreadable: the content is unknown, so apply always asks before emptying it.

### 3. build → mount → merge

```
 empty .agsy/
   → write a manifest holding only the previous merge records
   → copy rules, skills and hooks as they are
   → convert workflows (skill + stub)
   → assemble AGENTS.md
   → translate the four hook registries
   → write the manifest
   → create links
   → merge (hooks.claude.json → the hooks field of .claude/settings.json)
```

If mount or merge fails, the build result stays intact; fix the cause and rerun `agsy apply`. If the build itself fails, the manifest written first keeps the merge records (which file agsy created, which containers it added), so a later clean still knows what is agsy's.

### 4. Orphan report

Links or merge entries created by an earlier apply that the current config no longer names are listed as a reminder, never deleted; remove them by hand or with `agsy clean`.

## `agsy status`

Read-only. Prints the same two lists as apply, plus mount state:

- **List A**: source → output: content changes, additions, deletions, and missing outputs (the output copy was deleted; apply rebuilds it). "Source path missing" and "file deleted" are told apart.
- **List B**: artifact side: everything the next apply would discard, each with where to move it.
- **Mounts**: the state of each link (ok / missing / wrong target or broken / occupied / orphan) and each merge target (in sync / nothing to merge / agsy entries edited / entries missing / not a JSON object / orphan).
- **Summary**: `source changes N │ artifact-side changes N │ missing outputs N │ mount anomalies N` and the suggested next step.

Exit code `0` = fully in sync, `1` = any gap. Suited to CI and git hooks.

## `agsy clean`

Removes what agsy created in this project, after confirmation, in order:

1. agsy's hook entries are removed from merge targets, orphaned ones included. Only agsy's entries and the containers apply introduced go; a file agsy created that holds nothing else (emptied now, or by an earlier apply) is deleted; a file agsy did not create is never deleted, and one without agsy entries is not rewritten.
2. Mount links (including the root `AGENTS.md` and the `hooks.json` links) and orphaned links recorded in the manifest are removed; each is verified to be a link into the output first. Real folders and files are skipped and reported. Mount folders left empty are removed too.
3. The whole output folder is deleted.

`agsy.yaml` stays. One `agsy apply` rebuilds everything.

## `agsy version` / `agsy help`

```sh
agsy version    # agsy v1.2.3 (commit, build time, go version, platform)
agsy help       # usage (same as --help / -h)
```

→ Next: [Adapters](adapters.md)
