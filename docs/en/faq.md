# FAQ

| Section | Questions | When to read |
|---------|-----------|--------------|
| [Concepts & daily use](#concepts--daily-use) | Q1–Q6 | just getting started, questions about the daily flow |
| [What happened to my file](#what-happened-to-my-file) | Q7–Q12 | a file was not collected, got renamed, or might be deleted |
| [Tools & formats](#tools--formats) | Q13–Q17 | how each AI tool sees the output |
| [Platform & environment](#platform--environment) | Q18–Q24 | Windows, CI, uninstall, security |

---

## Concepts & daily use

### Q1: I edited a file inside `.claude/rules/` directly — will the next apply overwrite it?

`.claude/rules` is a link into `.agsy/rules`; what you edited is an output copy, and yes, the rebuild regenerates it. Before doing so, `apply` detects the change and **lists it with a confirmation** — it never overwrites silently. To keep the edit: merge it into the source file (status names it), then `apply`.

### Q2: Should `.agsy/` be version-controlled? What about `agsy.yaml`?

- `.agsy/`, the lock file `.agsy.lock`, the mount links, and the root `AGENTS.md` — **no**: all generated (the lock guards a running command and is removed when it finishes; the rest are rebuildable artifacts); `init` offers to add them to `.gitignore` for you.
- `agsy.yaml` — **yes**: it is the project's sync configuration.

### Q3: After editing a source, when do the tools see the new content?

After `agsy apply` finishes. agsy is not a daemon and does not watch files — remember to apply after editing (a git hook can remind you: non-zero `agsy status` exit code).

### Q4: What's the difference between `plan`, `status`, and `doctor`?

- `doctor`: **environment** health — is the config valid, do the sources exist, can links be created?
- `plan`: a full rehearsal of **this build** — what gets collected, converted, renamed, skipped, and what happens to each link.
- `status`: **current state vs the baseline** — which sources changed (list A), what was changed through the mounts (list B), are the links healthy?

All three are read-only and always safe to run.

### Q5: Can I run agsy from a project subdirectory?

Yes. Every command except `init` searches upward for `agsy.yaml`, like git. `init` deliberately looks only at the current directory — where the config gets created is decided by where you stand.

### Q6: Can several projects on one machine share one source library?

Yes — that is the core design. Each project keeps its own `agsy.yaml` with sources pointing at the same `~/all-ai-lib`.

---

## What happened to my file

### Q7: Why wasn't my skill collected?

Check against the acceptance rules (`agsy doctor` or `agsy plan` states the reason directly): a skill must be a **directory**; the directory must contain `SKILL.md`; the directory must contain **no symbolic links** (security rule; the whole skill is skipped); the name must not start with `.`.

### Q8: Why wasn't a file in my rules directory collected?

rules / workflows accept **single `.md` files** only: directories, non-`.md` extensions, dot-prefixed names, and symlinks are all rejected. Every reason appears in `plan`'s exclusion list.

### Q9: status reports artifact-side changes that apply will delete — how do I keep them?

Those are files someone (usually an AI tool) modified or added through a mount. They live in the rebuildable output, so the next apply discards them. **To keep one: move or merge it into the source that status names, then run `apply`.** That manual step is the review gate for AI-authored content.

### Q10: What is the `-fromlib-xxx` in a filename?

The **source tag** appended by the `rename` conflict strategy: when `python-style.md` exists in two sources, the outputs are `python-style-fromlib-all-ai-lib.md` and `python-style-fromlib-repo-ai-lib.md` — both kept, origin visible in the name. For skills the front-matter `name` is rewritten too.

### Q11: Why doesn't my workflow appear in some tool?

Check with `plan`: its `target:` may name a tool list that excludes it, a `target:` typo is a build-blocking error, and the tool must mount the matching output (`skills` for Claude / Codex / Cursor, `workflows` for Antigravity).

### Q12: I accidentally deleted things inside `.agsy/` (or the whole directory)?

The output is rebuildable by design: one `agsy apply` restores everything. If the manifest was damaged, apply asks one extra confirmation and then rebuilds.

---

## Tools & formats

### Q13: Why do workflows turn into skills?

Claude Code, Codex and Cursor read procedures through the skills mechanism — a `SKILL.md` directory with front matter deciding who may trigger it. The build packages each single-file workflow into that shape and sets `disable-model-invocation: true`, so in tools that honor the flag only a human can run it as `/name`. Your source stays a plain markdown file.

### Q14: What is the stub in `.agents/workflows/`?

Antigravity invokes workflows from that directory as `/name`. The stub keeps `/name` working while the content lives exactly once in the skill form: it tells the agent to load the corresponding skill and follow it. A workflow whose `target:` is only `antigravity` has no skill form, so its full content (minus the `target:` field) is placed there instead.

### Q15: Why is there both `.claude/rules/` and a root `AGENTS.md` with the same rules?

Different tools read different shapes. Claude Code reads per-file rules from `.claude/rules/` and does not read `AGENTS.md`; Codex, Cursor and Antigravity read the single concatenated `AGENTS.md`. Both are generated from the same sources, so they never diverge — and both are read-only.

### Q16: I already have my own AGENTS.md — what happens?

agsy never merges into or overwrites a real file: `init` warns immediately and `apply` refuses until you decide — move its content into a source `rules/` directory (then it reaches every tool), or rename the file to keep it outside agsy's management.

### Q17: Can an AI tool trigger a deploy workflow on its own?

In Claude Code and Cursor, no: the skill form carries `disable-model-invocation: true`, and only a human `/name` runs it. Codex and Antigravity ignore that field, so the model may decide to run a workflow itself — keep that in mind for side-effect-heavy procedures.

---

## Platform & environment

### Q18: Does Windows require administrator rights or Developer Mode?

No. agsy uses **junctions** for directory mounts and a **hard link** for the root `AGENTS.md`; a regular account can create both. One caveat: junctions store **absolute paths** — after moving the project, rerun `agsy apply` (macOS / Linux use relative symlinks and are unaffected).

### Q19: New machine (or shared library not cloned yet) and status shows many warnings?

When an entire source path is missing, status clearly separates "**source path missing**" from "source file deleted": the former usually means the shared repo is not cloned, an external disk is not mounted, or the path has a typo. `apply` also refuses to rebuild in this state. Fix the paths, then proceed.

### Q20: Can I use it in CI or git hooks?

Yes, by design:

- `agsy status`: exit `0` = in sync, `1` = gaps — use it directly as a check.
- Add `--yes` to action commands (e.g. `agsy apply --yes`); without it, confirmations in non-interactive environments are **cancelled**, so nothing is ever destroyed by surprise.
- `agsy init --yes <sources...>`: non-interactive initialization.

### Q21: How do I switch the interface language?

`export AGSY_LANG=zh-TW` (any value starting with `zh`) = Traditional Chinese; `AGSY_LANG=en` = English. Without `AGSY_LANG`, the system's `LC_ALL` / `LANG` decide. Note that **every** `zh` variant — `zh-CN` and `zh-Hans` included — currently maps to the Traditional Chinese (zh-TW) interface; no Simplified Chinese translation exists yet.

### Q22: How do I remove agsy completely?

1. Run `agsy clean` in every project (removes links, the root `AGENTS.md` link, and `.agsy/`; only agsy-created things — real files are skipped and reported).
2. Delete `agsy.yaml` if unwanted.
3. Delete the binary per your install method: `brew uninstall agsy` / `winget uninstall IngSquared99.agsy` / for Go installs, delete `~/go/bin/agsy`.

### Q23: status reports "orphan links" — what are they?

Links created by an earlier apply whose tool you later removed from the mount config. The tool keeps reading old content through them, so status keeps reminding you. `apply` never deletes them; remove them manually, or `agsy clean` clears them along with everything else.

The same applies to a merge target (Claude Code's `settings.json`) you removed from the mount config: agsy's entries stay in the `hooks` key and the tool keeps running them, so status lists the file as an orphan until you remove the entries by hand or run `agsy clean`.

### Q24: Could agsy copy out files that symlinks in my sources point to (e.g. a private key)?

No. Scanning never collects symbolic links (including inside skill directories — a skill containing a link is skipped entirely), and the copy phase refuses links as a second line of defense. The output is mounted for every tool to read; a link must never smuggle in files from outside a source.

## Hooks

### Q25: What is a hook, exactly? A webhook?

No. After you send a prompt, an AI coding tool runs a loop: the model thinks → runs a tool → the result goes back to the model → … A hook is a checkpoint at a fixed position in that loop (say "before a tool runs", `PreToolUse`, or "about to stop", `Stop`): the agent pauses there, runs your script locally, feeds it the situation as JSON on stdin, and an exit code 2 blocks the action while 0 lets it through. Everything happens on your machine, no network involved. A webhook is "a service sends an HTTP request to your URL when something happens" — the two only share the word.

### Q26: My rules already say "never rm -rf"; why a hook?

A rule is text the model reads; compliance is probabilistic — a long context or a conflicting goal can push it aside. A hook is a program: blocked means blocked, the model gets no vote. Anything expressible as an `if` belongs in a hook; style and preference, which no program can judge, stay in rules.

### Q27: Why does Claude Code use merge while the other three use links?

Codex, Antigravity and Cursor each have a dedicated `hooks.json`, so handing the whole file to agsy through a link is the clean option. Claude Code's hooks can only live in the `hooks` key of `.claude/settings.json`, next to your permissions, model and other settings — a link would swallow them — so agsy merges only its own entries into that file. Recognition works like links: an entry whose command points into `.agsy/hooks/` is agsy's, no marker needed.

### Q28: Where do my own hooks go?

The project-level registry belongs to agsy; personal hooks go to each vendor's personal layer, and all four stack layers rather than replacing them: Claude Code `.claude/settings.local.json` or `~/.claude/settings.json`; Codex `~/.codex/hooks.json` or `config.toml`; Cursor `~/.cursor/hooks.json`; Antigravity `~/.gemini/config/` (merge behaviour undocumented — test it).

### Q29: Can one script serve all four tools?

The registry can (agsy translates event names, structure and paths); the script has to handle the differences itself: the JSON each vendor feeds it differs (Claude / Codex use `tool_input.command`, Cursor and Antigravity have their own), and the "shell tool" name in matchers differs (Bash / Bash / run_command / Shell — set it with `overrides`). Branch on `hook_event_name` or the vendor's fields first.

### Q30: My hook does nothing in one tool?

`agsy plan` prints `claude ✓  codex ✓  antigravity —  cursor ✓` under every hook with the reason: the vendor documents no such hook point (Antigravity has five events and no `SessionStart`), the handler type is unsupported (Codex has no `http`), or `target:` left the tool out. Also, Claude Code and Codex both require project-level hooks to be trusted first (the workspace trust dialog) — accept it when the tool first starts.

### Q31: The script does not run on Windows?

agsy guarantees a correct registry (paths are absolute and quoted when they contain spaces or shell metacharacters), but a `.sh` cannot run on Windows directly. Name an interpreter in `hook.yaml` (`command: python3 ./check.py`) or use the vendor's own field through `overrides`, e.g. `overrides.codex.hooks[0].commandWindows`.

### Q32: After upgrading from v0.1, apply says `build.on_conflict.hooks is not set`?

v0.2.0 adds the hooks category, and its conflict strategy is mandatory like the others. Add one line, `hooks: error`, under `on_conflict` in `agsy.yaml`; mounting no registry at all is fine. To enable hooks for all four tools, rerun `agsy init` so the adapters add `.codex`, `.cursor`, `.agents/hooks.json` and the `.claude` `merge`, or add them by hand per [Configuration](config.md).

→ Back to: [Core Concepts](overview.md)
