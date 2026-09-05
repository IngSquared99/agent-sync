# Scenario Guide: Every apply Situation

What exactly happens when the two ends disagree, and when do the guard rails step in — this chapter lays out the scenarios as reference tables. When unsure, run `agsy status` (read-only) first and match its output against this chapter.

## The baseline: how the manifest knows "who changed"

On every `apply`, the manifest (`.agsy/.agsy-manifest.json`) records two kinds of fingerprints per item:

```
                       ┌── SrcHash: fingerprint of the SOURCE at build time
 snapshot at build ────┤
                       └── one hash per OUTPUT (verbatim copies and derived forms alike)

 at any later moment:
   current source   vs  SrcHash        differs → the source changed   (list A → apply syncs it)
   current output   vs  its hash       differs → the artifact side changed (list B → apply discards it)
```

The build-time snapshot is the third reference point that lets agsy tell **who** changed something, not merely that the two ends differ.

## The main matrix: source side × artifact side

The "artifact side" is what you or an AI tool actually touch through the mount links (`.claude/`, `.agents/`, the root `AGENTS.md`) — the files inside `.agsy/`.

| # | Source side | Artifact side | status shows | apply |
|---|-------------|---------------|--------------|-------|
| 1 | unchanged | unchanged | (no gaps) | rebuild, content identical |
| 2 | **edited** | unchanged | list A: update | ✅ rebuild, every tool gets the new content |
| 3 | unchanged | **edited** | list B: modified, with keep-guidance | ⚠ lists it, asks "discard and rebuild?" |
| 4 | **edited** | **edited** | list A + list B | ⚠ same confirmation; after consent the source version wins |
| 5 | **file added** | — | list A: add | ✅ collected into the output |
| 6 | — | **file added** | list B: untracked, "move it into a source" | ⚠ lists, then **deletes** after confirmation |
| 7 | **file deleted** | unchanged | list A: remove ⚠ (derived forms disappear too) | the item vanishes from every tool (status warned beforehand) |
| 8 | **whole source path missing** (not cloned / disk unmounted / typo) | any | ⚠ source path missing (clearly distinguished from "file deleted") | ✘ **refuses entirely** — never rebuild from an incomplete source list |
| 9 | unchanged | **output copy deleted** | missing outputs | ✅ rebuilds it |
| 10 | — | **derived form edited** (AGENTS.md, a stub, a workflow's skill form) | list B: modified, guidance points at the source file | ⚠ asks, then regenerates it |

Two general rules cover the table:

1. **apply always sides with the sources** — a rebuild makes the output match the sources' current state, so anything on the artifact side will be overwritten or deleted, but always **listed and confirmed first** (rows 3, 4, 6, 10).
2. **Keeping an artifact-side change is always a manual move into a source.** There is no write-back command; status names the destination for every entry. AI-authored content passes through your hands before it enters the library.

## Keeping an artifact-side change: the one flow

Untracked files are usually **new** files an AI tool created through a mount (e.g. a new rule it wrote for you). To keep one:

```sh
# move it from the output into a source's matching subdirectory
mv .agsy/rules/new-rule.md ~/all-ai-lib/rules/
agsy apply     # from now on it is a tracked, first-class item
```

A **modified** tracked file works the same way, except you merge the change into the existing source file instead of moving the whole file. For an edited **derived form**, edit the source it was generated from:

| Edited file | Edit this instead |
|-------------|-------------------|
| `AGENTS.md` (root or in `.agsy/`) | the matching source rule — each section is marked `<!-- agsy: rules/… -->` |
| `workflows/<name>.md` (stub) | the source workflow file |
| `skills/<name>/SKILL.md` of a workflow's skill form | the source workflow file |

Running apply without keeping anything shows every change in the confirmation list and discards them on consent.

## Mount-link scenarios

The mount links themselves can misbehave — a problem of the "channel", not of content:

| Scenario | status shows | How it happens | Fix |
|----------|--------------|----------------|-----|
| link missing | ✘ link missing | deleted by hand, or apply never ran | `agsy apply` recreates it |
| link points elsewhere | ✘ points to …, not the configured target | `build.out` changed, or the link was tampered with | `agsy apply` recreates it |
| link broken (target gone) | ✘ target does not exist | `.agsy/` deleted, or the project moved (Windows junctions store absolute paths) | `agsy apply` recreates it |
| mount point occupied by a **real** directory or file | ✘ occupied | the tool directory already had a same-named folder, or a real `AGENTS.md` exists at the root | ✘ apply **refuses** and **never deletes it** — move the content into a source (usually what you want) or remove it yourself, then apply |
| orphan link | ⚠ no longer referenced by the config | an earlier apply created it, then the tool was removed from mount | apply reports only; delete manually or let `agsy clean` remove it |

## Merge scenarios (Claude Code's settings.json)

`.claude/settings.json` is your file; agsy owns only the entries under `hooks` whose command points into `.agsy/hooks/`:

| Scenario | status shows | Handling |
|----------|-------------|----------|
| you edited an entry agsy wrote (say matcher Bash → Edit) | list B: agsy entries were edited | apply asks, then rebuilds the whole group. To keep it: move it into a group of your own (command not under `.agsy/`) or into `settings.local.json` |
| you added your own handler inside agsy's group | same (the group counts as edited) | same — agsy's groups only ever hold agsy's handlers |
| you added your own group, changed permissions, model, other keys | not an anomaly | apply and clean preserve them verbatim |
| agsy's entries were deleted, or the whole file | ✘ entries are missing | `agsy apply` restores them |
| the file is not a JSON object (broken, an array) or is a symlink | ✘ apply refuses until fixed | fix by hand; clean skips it and says so |
| the sources hold no hooks at all | nothing to merge ✔ | an absent file is not created; an existing one is not rewritten — neither by apply nor by clean |
| you removed the `merge` entry from `agsy.yaml` | ⚠ orphan: still holds agsy entries | apply leaves it alone (and keeps reporting it); remove the entries by hand or let `agsy clean` strip them |
| you moved the project or renamed `build.out` | in sync ✔ after `agsy apply` | the old groups are recognised by their `agsy:` mark and replaced, never duplicated |

## Conditions that stop apply before the build starts

These checks deliberately run **before** the discard confirmation:

| Condition | Message gist | Fix |
|-----------|--------------|-----|
| any source path missing | refuses to rebuild from an incomplete list | fix the path (clone / mount / typo) |
| mount point occupied by a real path | refuses and lists; never deletes | move or delete it yourself |
| workflow target errors (unknown tool name), `hook.yaml` errors (parse failure, unknown event, missing command, referenced script absent) | lists the files to fix | fix the front matter / `hook.yaml`, or extend `build.tools` |
| a merge target is a symlink or not a JSON object | refuses and lists it | fix `.claude/settings.json` by hand |
| a mount point occupied by a real `hooks.json` | refuses and lists it; never deletes | rewrite its content as a hook in a source, then remove the file |
| name conflicts (`on_conflict: error`) | lists the conflict groups | rename or delete one copy |
| final output-path collisions (cross-category included) | lists the collision groups | rename one of them |
| manifest corrupt but `.agsy/` exists | contents unknowable | ⚠ extra confirmation before rebuilding |
| manifest newer than this agsy | old rules must not parse new data | upgrade agsy (or delete `.agsy/` and rebuild) |

## Quick decision map

```
 Run agsy status first, then:

 artifact-side changes?
   ├─ keep some ──▶ move / merge each into its source (guidance names it) ──▶ agsy apply
   └─ discard ───▶ agsy apply (confirm the discard)

 only source changes / missing outputs? ──▶ agsy apply

 mount anomalies?
   ├─ occupied by a real path ──▶ move it away ──▶ agsy apply
   ├─ orphan links / orphan hook entries ──▶ delete manually or agsy clean
   └─ anything else (missing / wrong / broken) ──▶ agsy apply

 source path missing? ──▶ fix the path first (clone / mount the disk / fix the typo); run nothing until then
```

→ Next chapter: [FAQ](faq.md)
