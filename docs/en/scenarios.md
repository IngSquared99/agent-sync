# Scenario Guide: Every apply and promote Situation

What exactly do apply and promote do when the two ends disagree, and when do the guard rails step in — this chapter lays out **every scenario** as reference tables. When unsure, run `agsy status` (read-only) first and match its output against this chapter.

## The baseline: how the manifest knows "who changed"

On every `apply`, the manifest (`.agsy/.agsy-manifest.json`) records **two fingerprints** per item:

```
                       ┌── SrcHash: fingerprint of the SOURCE file at build time
 snapshot at build ────┤
                       └── Hash   : fingerprint of the ARTIFACT copy at build time

 at any later moment:
   current source file  vs  SrcHash   differs → the source end changed (behind → apply)
   current artifact file vs Hash      differs → the artifact end changed (local changes → promote)
   both differ                        → both ends changed (manual resolution)
```

The build-time snapshot is the third reference point that lets agsy tell **who** changed something, not merely that the two ends differ.

## The main matrix: source end × artifact end

The "artifact end" is what you or an AI tool actually touch through the mount links (`.claude/` etc.) — see the [term definitions](overview.md#know-these-terms-first).

| # | Source end | Artifact end | status shows | apply | promote |
|---|-----------|--------------|--------------|-------|---------|
| 1 | unchanged | unchanged | (no gaps) | rebuild, content identical | nothing to do |
| 2 | **edited** | unchanged | `behind` | ✅ rebuild, sync the new content | nothing to write back |
| 3 | unchanged | **edited** | `local changes` | ⚠ lists them, asks "discard and rebuild?" | ✅ write back |
| 4 | **edited** | **edited** | `local changes` + ⚠ source changed too | ⚠ asks "discard?" (artifact-end edits would be lost) | ✘ **refuse** — overwriting would destroy the source's new content; merge manually (see "Scenario 4 walkthrough") |
| 5 | **file added** | — | `new` | ✅ collected into the artifacts | unrelated |
| 6 | — | **file added** | `untracked` | ⚠ lists, then **deletes** after confirmation (rebuild = sources win) | ✘ cannot write back — it has no source location (see "Scenario 6 walkthrough") |
| 7 | **file deleted** | unchanged | `behind` + ⚠ source deleted | the item **disappears** from the artifacts (status warned beforehand) | nothing to write back |
| 8 | **file deleted** | **edited** | `local changes` + ⚠ source file deleted | ⚠ asks "discard?"; item disappears on consent | ⚠ asks explicitly "**recreate the source file?**" |
| 9 | **whole source path missing** (not cloned / disk unmounted / typo) | any | ⚠ source path missing (clearly distinguished from "file deleted") | ✘ **refuses entirely** — never rebuild from an incomplete source list, or the absent source's items would be wiped | ✘ refuses items from that source — fix the path first |
| 10 | unchanged | **artifact copy deleted** | `missing artifacts` | ✅ rebuilds it | nothing to write back (not a local change) |
| 11 | — | **multiple bucket copies, edited to DIFFERENT contents** | `local changes` + ⚠ copies diverged | ⚠ asks "discard?" | ✘ refuse — a human must pick the right one |
| 12 | — | **multiple bucket copies, edited to IDENTICAL contents** | `local changes` (one item) | ⚠ asks "discard?" | ✅ writes back and syncs every copy |

Three general laws that replace memorizing the table:

1. **apply always sides with the sources** — a rebuild makes the artifacts match the sources' current state, so anything unpromoted on the artifact end will be overwritten or deleted, but always **listed and confirmed first** (rows 3, 4, 6, 8, 11, 12).
2. **promote acts only when the write-back is safe** — anything that would destroy new source content (4), has nowhere to go (6, 10), or needs a human decision (11) is refused or turned into a question.
3. **"path missing" and "file deleted" are different things** (9 vs 7) — the former is usually an environment problem (repo not cloned, disk not mounted); agsy stops and asks you to fix the path, and never misreads it as deleted files.

## Scenario 4 walkthrough: both ends edited

1. `agsy status` — find the item and note its original source path (printed in the report).
2. Compare the two copies manually: the **source file** (e.g. `~/all-ai-lib/rules/x.md`) vs the **artifact file** (e.g. `.agsy/rules/x.md`; the file reached via `.claude/rules/x.md` is the same one).
3. Merge what you want to keep into the **source file** (sources are the single source of truth).
4. `agsy apply` — baselines reset; back to scenario 1.

## Scenario 6 walkthrough: keeping an untracked file

Untracked files are usually **new** files an AI tool created through a tool directory (e.g. a new rule it wrote for you). To keep one:

```sh
# move it from the artifacts into a source's matching subdirectory
mv .agsy/rules/new-rule.md ~/all-ai-lib/rules/
agsy apply     # from now on it is a tracked, first-class item
```

Running apply without moving it shows the file in the confirmation list and deletes it on consent.

## Mount-link scenarios

The mount links themselves can misbehave — this is not a content difference between the two ends but a problem of the "channel" itself:

| Scenario | status shows | How it happens | Fix |
|----------|--------------|----------------|-----|
| link missing | ✘ link missing | deleted by hand, or apply never ran | `agsy apply` recreates it |
| link points elsewhere | ✘ points to …, not the configured target | `build.out` changed, or the link was tampered with | `agsy apply` recreates it |
| link broken (target gone) | ✘ target does not exist | `.agsy/` deleted, or the project moved (Windows junctions store absolute paths) | `agsy apply` recreates it |
| mount point occupied by a **real** directory/file | ✘ occupied by a real directory | the tool directory already had a same-named folder | ✘ apply **refuses** and **never deletes it** — move the content into a source (usually what you want) or remove it yourself, then apply |
| orphan link | ⚠ no longer referenced by the config | an earlier apply created it, then the tool was removed from mount | apply reports only; delete manually or let `agsy clean` remove it |

## Conditions that stop apply before the build starts

These checks deliberately run **before** the discard confirmation — you must not consent to losing changes and sit through a build only to find the run could never succeed:

| Condition | Message gist | Fix |
|-----------|--------------|-----|
| any source path missing | refuses to rebuild from an incomplete list | fix the path (clone / mount / typo) |
| mount point occupied by a real directory | refuses and lists; never deletes | move or delete it yourself |
| workflow routing errors (target names a nonexistent bucket) | lists the files to fix | fix the front matter or extend `route.buckets` |
| name conflicts (`on_conflict: error`) | lists the conflict groups | rename or delete one copy |
| final-name collisions | lists the collision groups | rename (post-rename coincidences are always blocked, never overwritten) |
| manifest corrupt but `.agsy/` exists | contents unknowable | ⚠ extra confirmation before rebuilding |
| manifest newer than this agsy | old rules must not parse new data | upgrade agsy (or delete `.agsy/` and rebuild) |

## promote's complete guard list

Beyond the content judgments in the matrix, promote checks **where it writes** with a full set of safeguards:

| Guard | Rule |
|-------|------|
| destination lock | writes only into the item's **own slot**: `<configured source>/<category subdir>/<original name>` — a tampered manifest cannot redirect the write |
| `--to` allowlist | only sources **configured** in `agsy.yaml`; anywhere else means leaving agsy's management — refused |
| no writing through symlinks | a symlink at the destination is refused outright |
| extra confirmation outside the project | a destination in a shared library affects every project using it — asked once more |
| `--all --to` unsupported | batch redirection would desync sources from the manifest |
| renamed-skill name restore | the `SKILL.md` `name` is restored to the original on write-back (`-fromlib-…` belongs only to the artifacts) |
| `--to` redirect reminder | the old source still holds the item — the next build sees both same-name copies; delete the old one once verified |
| baseline refresh after write-back | both fingerprints are refreshed: the change is no longer reported and cannot be misread as "source changed too"; run apply next to complete the cycle |

## Quick decision map

```
 Run agsy status first, then:

 local changes?
   ├─ keep them ──▶ agsy promote ──▶ agsy apply
   ├─ discard ────▶ agsy apply (confirm the discard)
   └─ "source changed too" ──▶ merge into the source manually (Scenario 4 walkthrough) ──▶ agsy apply

 untracked files?
   ├─ keep ──▶ move into a source subdir (Scenario 6 walkthrough) ──▶ agsy apply
   └─ discard ──▶ agsy apply (confirm the deletion)

 only behind / new / missing artifacts? ──▶ agsy apply

 mount anomalies?
   ├─ occupied by a real directory ──▶ move it away ──▶ agsy apply
   ├─ orphan links ──▶ delete manually or agsy clean
   └─ anything else (missing / wrong / broken) ──▶ agsy apply

 source path missing? ──▶ fix the path first (clone / mount the disk / fix the typo); run nothing until then
```

→ Next chapter: [FAQ](faq.md)
