# FAQ

Organized from the user's point of view, in four sections — jump straight to your situation:

| Section | Questions | When to read |
|---------|-----------|--------------|
| [Concepts & daily use](#concepts--daily-use) | Q1–Q6 | just getting started, questions about the daily flow |
| [What happened to my file](#what-happened-to-my-file) | Q7–Q12 | a file was not collected, got renamed, or might be deleted |
| [Conflicts & write-back](#conflicts--write-back) | Q13–Q17 | name conflicts, promote refusing |
| [Platform & environment](#platform--environment) | Q18–Q25 | Windows permissions, CI, uninstall, security |

---

## Concepts & daily use

### Q1: I edited a file inside `.claude/rules/` directly — will the next apply overwrite it?

`.claude/rules` is a link into `.agsy/rules`; what you edited is the artifact copy. Before rebuilding, `apply` detects this as a "local change" and **lists it with a confirmation** — it never overwrites silently. To keep the edit: run `agsy promote` to write it back to the source first, then `apply`.

### Q2: Should `.agsy/` be version-controlled? What about `agsy.yaml`?

- `.agsy/` — **no**: it is a rebuildable artifact; `init` even offers to add it to `.gitignore` for you.
- `agsy.yaml` — **yes**: it is the project's sync configuration, and also your safety net when init's edit mode replaces yaml comments.

### Q3: After editing a source, when do the tools see the new content?

After `agsy apply` finishes. agsy is not a daemon and does not watch files — remember to apply after editing (a git hook can remind you: non-zero `agsy status` exit code).

### Q4: What's the difference between `plan`, `status`, and `doctor`?

- `doctor`: **environment** health — is the config valid, do the sources exist, can links be created?
- `plan`: a full rehearsal of **this build** — what gets collected, who is renamed, who is skipped, what happens to each link.
- `status`: **current state vs the baseline** — sources changed but not rebuilt? artifact end edited but not written back? links healthy?

All three are read-only and always safe to run.

### Q5: Can I run agsy from a project subdirectory?

Yes. Every command except `init` searches upward for `agsy.yaml`, like git. `init` deliberately looks only at the current directory — where the config gets created is decided by where you stand, so a parent project's config can never be edited by accident.

### Q6: Can several projects on one machine share one source library?

Yes — that is the core design. Each project keeps its own `agsy.yaml` with sources pointing at the same `~/all-ai-lib`. Note: `promote` into a shared library asks an extra confirmation, because it affects every project using it.

---

## What happened to my file

### Q7: Why wasn't my skill collected?

Check against the acceptance rules (`agsy doctor` or `agsy plan` states the reason directly):

- a skill must be a **directory**, not a single `.md` file;
- the directory must contain `SKILL.md` (without it, it counts as assets or a draft);
- the directory must contain **no symbolic links** (security rule; the whole skill is skipped);
- the name must not start with `.`.

### Q8: Why wasn't a file in my rules directory collected?

rules / workflows accept **single `.md` files** only: directories, non-`.md` extensions, dot-prefixed names, and symlinks are all rejected. Every reason appears in `plan`'s exclusion list.

### Q9: status reports untracked files and apply will delete them — what now?

Untracked = files someone (usually an AI tool) **added** on the artifact end that the manifest does not know. They have no source location, so `promote` cannot write them back and `apply`'s rebuild deletes them. **To keep one: move it into a source's matching subdirectory (e.g. `~/all-ai-lib/rules/`), then run `apply`.**

### Q10: What is the `-fromlib-xxx` in a filename?

The **source tag** appended by the `rename` conflict strategy: when `python-style.md` exists in two sources, the outputs are `python-style-fromlib-all-ai-lib.md` and `python-style-fromlib-repo-ai-lib.md` — both kept, origin visible in the name. For skills the front-matter `name` is rewritten too (and restored on promote).

### Q11: Why doesn't my workflow appear in any tool?

Three possibilities, all flagged by `plan`:

1. its `target:` names a value missing from `route.buckets` → routing error, apply refuses;
2. no `target` and `route.default` is empty → placed in no bucket (with a warning);
3. its bucket has no mount link reading it (e.g. the bucket `codex` exists but `.codex` is not mounted).

### Q12: I accidentally deleted things inside `.agsy/` (or the whole directory)?

Nothing lost. Artifacts are rebuildable by design: one `agsy apply` restores everything. If the manifest was damaged, apply asks one extra confirmation (it cannot know whether the directory held your edits) and then rebuilds.

---

## Conflicts & write-back

### Q13: What happens when two sources have same-named files?

Decided per category by `on_conflict`: `rename` = keep both (source tags appended); `error` = stop with a list for manual handling; `first` = keep only the higher-priority copy (earlier in `sources`). A mandatory init question — no implicit default.

### Q14: Why is `error` recommended for skills instead of `rename`?

Skills trigger on description semantics — with two same-named skills coexisting, which one a tool picks is unpredictable; and rename must rewrite the `SKILL.md` name as well. A same-name skill usually means the copies should be merged, so stopping for manual handling is the recommendation.

### Q15: status says "the source changed as well" and promote refuses — what now?

That is the protection: overwriting would destroy the source's new content. Compare the two copies manually (the source file vs the artifact file in `.agsy/`), merge what you want to keep into the **source**, then `apply`. The rebuild resets the baselines and everything returns to normal.

### Q16: How do I move an item from the shared library into the project library?

After editing the content on the artifact end:

```sh
agsy promote skills/code-review --to ./repo-ai-lib
```

Note the reminder afterwards: **the shared library still holds the old copy** — the next build would collect both same-name copies. Delete the old one once the new location is verified.

### Q17: `promote --all` versus writing back one by one?

`--all` first lists everything grouped by source (write-backs into shared libraries outside the project get an extra warning), confirms once, then writes back item by item; a single failure (e.g. missing source root) never aborts the batch, and failures are listed at the end. `--all` cannot be combined with `--to`.

---

## Platform & environment

### Q18: Does Windows require administrator rights or Developer Mode?

No. agsy uses **junctions** (directory reparse points) instead of symlinks on Windows; a regular account can create them. One caveat: junctions store **absolute paths** — after moving the project, rerun `agsy apply` to rebuild the links (macOS / Linux use relative symlinks and are unaffected).

### Q19: New machine (or shared library not cloned yet) and status shows a pile of warnings?

When an entire source path is missing, status clearly separates "**source path missing**" from "source file deleted": the former usually means the shared repo is not cloned, an external disk is not mounted, or the path has a typo. `apply` also refuses to rebuild in this state (protecting the absent source's items). Fix the paths, then proceed.

### Q20: Can I use it in CI or git hooks?

Yes, by design:

- `agsy status`: exit `0` = in sync, `1` = gaps — use it directly as a check; without a TTY only the report prints, no interactive menu.
- Add `--yes` to action commands (e.g. `agsy apply --yes`); without it, confirmations in non-interactive environments are **cancelled**, so nothing is ever destroyed by surprise.
- `agsy init --yes <sources...>`: non-interactive initialization.

### Q21: How do I switch the interface language?

`export AGSY_LANG=zh-TW` (any value starting with `zh`) = Traditional Chinese; `AGSY_LANG=en` = English. Without `AGSY_LANG`, the system's `LC_ALL` / `LANG` decide.

### Q22: How do I remove agsy completely?

1. Run `agsy clean` in every project (removes links and `.agsy/`; only agsy-created things — real files are skipped and reported).
2. Delete `agsy.yaml` if unwanted.
3. Delete the binary per your install method: `brew uninstall agsy` / `winget uninstall IngSquared99.agsy` / for Go installs, delete `~/go/bin/agsy`.

### Q23: status reports "orphan links" — what are they?

Links created by an earlier apply whose tool you later removed from the mount config. The tool keeps reading old content through them, so status keeps reminding you. `apply` never deletes them (it only manages links in the current config); remove them manually, or `agsy clean` clears them along with everything else.

### Q24: A real `.claude/skills` directory (not a link) already occupies the mount point?

agsy **never deletes real directories/files it did not create**. Move the existing content into a source (usually exactly what you want: let agsy manage it), or back it up and delete the directory yourself, then run `apply`.

### Q25: Could agsy copy out files that symlinks in my sources point to (e.g. a private key)?

No. Scanning never collects symbolic links (including inside skill directories — a skill containing a link is skipped entirely), and the copy phase refuses links as a second line of defense. This is deliberate: the artifacts are mounted for every tool to read, and a link must never smuggle in files from outside a source.
