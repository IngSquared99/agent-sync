# Scenario Guide: when the two sides differ

A source changed, an AI edited the output, a link broke… this chapter tabulates what `status` shows and what `apply` does in each case. When unsure, run the read-only `agsy status` first and match its output against these tables.

## 1. Two rules

1. **apply follows the source.** Rebuilding brings the output back to the source's current state; artifact-side changes are overwritten or deleted, but always listed and confirmed first.
2. **Keeping an artifact-side change means moving it into a source.** There is no write-back command; status names the destination for every item.

## 2. How agsy tells who changed what

Every `apply`, the manifest records two fingerprints:

```
                     ┌── fingerprint of the source
 at build time ──────┤
                     └── fingerprint of each output

 any time later:
   current source vs source fingerprint   differs → the source changed   (list A, apply syncs it)
   current output vs output fingerprint   differs → the artifact changed (list B, apply discards it)
```

## 3. Main matrix: source side × artifact side

The "artifact side" is what you or an AI tool touch through the links, i.e. the files inside `.agsy/`.

| # | Source side | Artifact side | status shows | apply |
|---|-------------|---------------|--------------|-------|
| 1 | unchanged | unchanged | no gaps | rebuilds, same content |
| 2 | changed | unchanged | list A: update | rebuilds; every tool gets the new content |
| 3 | unchanged | changed | list B: modified, with keep hint | lists it, asks "discard and rebuild?" |
| 4 | changed | changed | list A + list B | same; after consent the source wins |
| 5 | file added | — | list A: new | collected into the output |
| 6 | — | file added | list B: untracked, "move into a source" | lists it, deletes after confirmation |
| 7 | file deleted | unchanged | list A: removed ⚠ (derived forms go too) | the item disappears from every tool |
| 8 | whole source path missing | any | ⚠ source path missing | ✘ refuses to run |
| 9 | unchanged | output copy deleted | missing output | rebuilds it |
| 10 | — | derived file edited (AGENTS.md, stub, a workflow's skill) | list B: modified, hint points at the source file | asks, then regenerates |

## 4. Keeping an artifact-side change

An untracked file is usually one an AI tool added through a link (a new rule it wrote for you). To keep it:

```sh
# move it from the output into the matching subfolder of a source
mv .agsy/rules/new-rule.md ~/all-ai-lib/rules/
agsy apply
```

An edited tracked file: merge the change into the source file. An edited derived file: edit its source.

| Edited file | Edit instead |
|-------------|--------------|
| `AGENTS.md` (root or inside `.agsy/`) | the source rule; each section starts with an `<!-- agsy: rules/… -->` marker |
| `workflows/<name>.md` (stub) | the source workflow file |
| `skills/<name>/SKILL.md` (a workflow's skill) | the source workflow file |

Keeping nothing and running apply: the confirmation lists every change; consent discards them.

## 5. Link trouble

These are problems with the "channel", not content differences:

| Scenario | status shows | Cause | Fix |
|----------|--------------|-------|-----|
| link missing | ✘ link missing | deleted by hand, or apply never ran | `agsy apply` |
| link points elsewhere | ✘ points at …, not the configured target | `build.out` changed, or the link was altered | `agsy apply` |
| link broken | ✘ target does not exist | `.agsy/` deleted, or the project moved (Windows junctions store absolute paths) | `agsy apply` |
| mount point occupied by a real folder or file | ✘ occupied | the tool folder already had a folder of that name, or a real `AGENTS.md` exists in the root | apply refuses and deletes nothing. Move the content into a source or remove it yourself, then apply |
| orphan link | ⚠ no longer referenced by the config | created by an earlier apply, tool since removed from mount | apply only reports; delete by hand or `agsy clean` |

## 6. Merge trouble (Claude Code's settings.json)

`.claude/settings.json` is your file; agsy owns only the groups under `hooks` whose `statusMessage` starts with `agsy:` or whose command points into `.agsy/hooks/`.

| Scenario | status shows | Fix |
|----------|--------------|-----|
| you edited an entry agsy wrote (matcher Bash → Edit, say) | list B: agsy entries were edited | apply asks, then rebuilds the group. To keep it: move it into a group of your own (drop the `agsy:` mark, command not under `.agsy/`) or into `settings.local.json` |
| you added your own handler inside agsy's group | same | same; agsy's groups hold only agsy's handlers |
| you added your own group, changed permissions, model, other fields | not an anomaly | apply and clean leave them alone |
| agsy's entries deleted, or the whole file | ✘ entries missing | `agsy apply` |
| the file is not a JSON object (broken, an array) or is a symbolic link | ✘ apply refuses until fixed | fix by hand; clean skips it and says so |
| the sources hold no hooks | nothing to merge ✔ | an absent file is not created; an existing one is not rewritten |
| the `merge` entry was removed from `agsy.yaml` | ⚠ orphan: still holds agsy entries | apply leaves it and keeps reporting; remove by hand or `agsy clean` |
| a recorded merge target lies outside the project | ⚠ not checked | agsy does not open files outside the project; open it and remove the entries starting with `agsy:` |
| the project moved or `build.out` was renamed | in sync ✔ after apply | the old groups are recognized by the `agsy:` mark and replaced |

## 7. Conditions that stop apply before the build

These checks run before the discard confirmation:

| Condition | Message gist | Fix |
|-----------|--------------|-----|
| a source path is missing | refuses to rebuild from an incomplete list | fix the path (clone, mount the disk, fix the typo) |
| a mount point is occupied by a real path | refuses and lists it | move or delete it yourself |
| workflow target error, `hook.yaml` error (parse failure, unknown event, missing command, a quoted `./` path, an override index past the group or one that empties a command, a referenced script missing) | lists the files to fix | fix the front matter or `hook.yaml`, or extend `build.tools` |
| a merge target is a symbolic link or not a JSON object | refuses and lists it | fix `.claude/settings.json` by hand |
| a mount point is occupied by a real `hooks.json` | refuses and lists it | rewrite its content as a hook in a source, then delete the file |
| name conflict (`on_conflict: error`) | lists the conflicting pair | rename or delete one |
| final output path collision (across categories too) | lists the colliding pair | rename one |
| manifest corrupt but `.agsy/` exists | content unknown | one more confirmation |
| manifest newer than this agsy | cannot be parsed | upgrade agsy, or delete `.agsy/` and rebuild |

## 8. Quick decisions

```
 run agsy status, then:

 artifact-side changes?
   ├─ keep some  ──▶ move each into a source (the hint names it) ──▶ agsy apply
   └─ discard all ──▶ agsy apply (confirm the discard)

 only source changes or missing outputs? ──▶ agsy apply

 mount anomalies?
   ├─ occupied by a real path ──▶ move it away ──▶ agsy apply
   ├─ orphan link or orphan hook entries ──▶ delete by hand or agsy clean
   └─ anything else (missing, wrong target, broken) ──▶ agsy apply

 source path missing? ──▶ fix the path first; run nothing until it is
```

→ Next: [FAQ](faq.md)
