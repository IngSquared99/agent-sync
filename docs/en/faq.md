# FAQ

| Section | Questions | When |
|---------|-----------|------|
| [Concepts and daily use](#concepts-and-daily-use) | Q1–Q6 | getting started |
| [What happened to my file](#what-happened-to-my-file) | Q7–Q12 | a file was not collected, renamed, or may be deleted |
| [Tools and formats](#tools-and-formats) | Q13–Q17 | how each tool sees the output |
| [Platform and environment](#platform-and-environment) | Q18–Q24 | Windows, CI, removal, safety |
| [hooks](#hooks) | Q25–Q32 | guard scripts |

## Concepts and daily use

### Q1: I edited a file in `.claude/rules/` directly. Will the next apply overwrite it?

Yes, but it asks first. `.claude/rules` is a link to `.agsy/rules`; you edited the output copy. `apply` lists the change and asks for confirmation. To keep it: merge the change into the source file (status names it), then apply.

### Q2: What goes under version control?

`agsy.yaml` is the project's sync config and is meant to be committed. `.agsy/`, `.agsy.lock`, the links and the root `AGENTS.md` are generated; whether they go under version control is your call, and `init` asks whether to add them to `.gitignore`.

### Q3: When do tools see a source change?

After `agsy apply`. agsy is not a daemon and does not watch files.

### Q4: What is the difference between `plan`, `status` and `doctor`?

- `doctor`: environment health. Is the config valid, do the sources exist, can links be created.
- `plan`: a preview of this build. What is collected, converted, renamed, skipped, and what happens to each link.
- `status`: the current state against the last apply. Which sources changed (list A), what changed on the artifact side (list B), are the links healthy.

All three are read-only.

### Q5: Can I run agsy from a project subfolder?

Yes. Except `init`, every command searches upward for `agsy.yaml`. `init` looks at the current folder only; the config is created where you stand.

### Q6: Can several projects share one source library?

Yes. Each project has its own `agsy.yaml` whose `sources` point at the same `~/all-ai-lib`.

## What happened to my file

### Q7: Why was my skill not collected?

`agsy doctor` or `agsy plan` states the reason. The rules: a skill is a folder; it contains `SKILL.md`; it holds no symbolic links; its name does not start with `.`.

### Q8: Why was a file in the rules folder not collected?

rules and workflows take single `.md` files only: folders, other extensions, names starting with `.` and symbolic links are not collected. The reason appears in `plan`'s exclusion list.

### Q9: status reports artifact-side changes and apply will delete them. What now?

Those are files modified or added through a link, usually by an AI tool. They live in the rebuildable output and the next apply discards them. To keep one: move or merge it into the source status names, then apply. That manual step is the review gate for AI-produced content.

### Q10: What is `-fromlib-xxx` in a filename?

The source tag added by the `rename` strategy. When `python-style.md` exists in two sources, the output holds `python-style-fromlib-all-ai-lib.md` and `python-style-fromlib-repo-ai-lib.md`; both are kept and the name says where each came from. For a skill, the `name` inside `SKILL.md` is rewritten too.

### Q11: Why does my workflow not show up in one tool?

Check with `plan`: `target:` may not list that tool; a typo in `target:` stops the build; the tool also has to mount the matching output (Claude / Codex / Cursor mount `skills`, Antigravity mounts `workflows`).

### Q12: I deleted something inside `.agsy/` (or the whole folder).

The output is rebuildable: one `agsy apply` restores everything. If the manifest is gone too, apply asks once more before rebuilding.

## Tools and formats

### Q13: Why does a workflow become a skill?

Claude Code, Codex and Cursor read procedures through the skills mechanism: a `SKILL.md` folder plus a header field deciding who may trigger it. agsy packages every single-file workflow into that form with `disable-model-invocation: true`, so in tools honouring the field only a person can run it with `/name`. The source stays a plain `.md`.

### Q14: What is the stub in `.agents/workflows/`?

Antigravity triggers workflows with `/name` from that folder. The stub keeps `/name` working while the content exists once (as the skill): it tells the AI to load the matching skill and follow it. A workflow whose `target:` is only `antigravity` has no skill form; the full content is placed there instead.

### Q15: Why do `.claude/rules/` and the root `AGENTS.md` hold the same rules?

Different tools read different forms. Claude Code reads per-file `.claude/rules/` and not `AGENTS.md`; Codex, Cursor and Antigravity read the single combined `AGENTS.md`. Both come from the same sources.

### Q16: My project already has its own `AGENTS.md`.

agsy does not merge or overwrite real files: `init` reminds you, `apply` refuses, until you decide: move the content into a source's `rules/` (every tool then reads it), or rename the file to keep it.

### Q17: Can an AI trigger a deploy workflow on its own?

Not in Claude Code or Cursor: the skill carries `disable-model-invocation: true`, and only a person typing `/name` runs it. Codex and Antigravity ignore the field, so the model may run it on its own; watch procedures with side effects.

## Platform and environment

### Q18: Does Windows need administrator rights?

No. Folders are mounted as junctions and files as hard links; a normal account can create both. A junction stores an absolute path, so rerun `agsy apply` after moving the project.

### Q19: status shows many warnings on a new machine.

When a whole source path is missing, status says "source path missing": usually the shared library is not cloned yet, an external disk is not mounted, or the path has a typo. `apply` refuses in that state too. Fix the path and continue.

### Q20: Can I use it in CI or a git hook?

Yes:

- `agsy status`: exit `0` = in sync, `1` = gaps; use it as a check.
- Add `--yes` to acting commands (`agsy apply --yes`); without it, confirmations in a non-interactive environment cancel.
- `agsy init --yes <sources...>`: non-interactive setup.

### Q21: How do I switch the interface language?

`export AGSY_LANG=zh-TW` (any value starting with `zh`) = Traditional Chinese; `AGSY_LANG=en` = English. Unset, the system's `LC_ALL` / `LANG` decide. Every `zh` value (including `zh-CN`) maps to the Traditional Chinese interface.

### Q22: How do I remove agsy completely?

1. Run `agsy clean` in every project.
2. Delete `agsy.yaml` by hand if unwanted.
3. Remove the binary the way it was installed: `brew uninstall agsy` or delete `~/go/bin/agsy`.

### Q23: status reports an "orphan link".

A link created by an earlier apply for a tool you have since removed from the mount config. The tool still reads old content through it, so status keeps reminding. `apply` does not delete it; remove it by hand, or `agsy clean` removes it with everything else. A merge target dropped from the config (Claude Code's `settings.json`) is reported the same way.

### Q24: Can agsy copy out a file a symbolic link in a source points at (a private key, say)?

No. Scanning never collects symbolic links (inside skill and hook folders too), and the copy step checks again.

## hooks

### Q25: What is a hook? A webhook?

No. After you send a message, an AI tool runs a loop: think → run a tool → result back to the model → … A hook is a checkpoint at a fixed position in that loop (before a tool runs, `PreToolUse`; before stopping, `Stop`): the AI pauses there, runs your script locally and feeds it the situation as JSON; exit code 2 blocks, 0 allows. Everything stays on your machine. A webhook is a service sending an HTTP request to your URL when something happens; only the name is shared.

### Q26: The rules already say "no rm -rf". Why a hook too?

Rules are text the model reads; compliance is probabilistic. A hook is a program; blocked means blocked. Anything expressible as "if … then not allowed" goes in a hook; style and preference stay in rules.

### Q27: Why does Claude Code use merge while the other three use links?

Codex, Antigravity and Cursor each have a dedicated `hooks.json`, so a link to the whole file is the clean option. Claude Code's hooks live in the `hooks` field of `.claude/settings.json`, a file that also holds your permissions, model and other settings, so agsy merges only its own entries into it. Two clues identify them: a handler's `statusMessage` starting with `agsy:`, or a command pointing into `.agsy/hooks/`. Merge targets must lie inside the project; `~/.claude/settings.json` stays yours.

### Q28: Where do my own hooks go?

Each tool's personal file, layered on the project one: Claude Code `.claude/settings.local.json` or `~/.claude/settings.json`; Codex `~/.codex/hooks.json` or `config.toml`; Cursor `~/.cursor/hooks.json`; Antigravity `~/.gemini/config/` (merging behavior is not documented by the vendor; test it).

### Q29: Can one script serve all four tools?

The registry can (agsy translates event names, structure and paths); the script has to handle the differences itself: the JSON each tool feeds it differs, and the name of the "shell tool" in the matcher differs (Bash / Bash / run_command / Shell, set via `overrides`). Check `hook_event_name` or the tool's fields first.

### Q30: A hook does not fire in one tool.

`agsy plan` prints `claude ✓  codex ✓  antigravity —  cursor ✓` under every hook with the reason: the tool has no such moment (Antigravity has five events), does not support the handler type (Codex has no `http`), or `target:` leaves it out. Some tools also require project-level hooks to be "trusted" first (the workspace trust dialog); accept it on first launch.

### Q31: The script does not run on Windows.

The registry holds absolute paths, quoted when needed, but a `.sh` cannot run on Windows directly. Name an interpreter in `hook.yaml` (`command: python3 ./check.py`) or use the vendor's own field through `overrides`, such as `overrides.codex.hooks[0].commandWindows`.

### Q32: apply says `build.on_conflict.hooks is not set`.

The conflict strategy for hooks is required like the other three. Add `hooks: error` under `on_conflict` in `agsy.yaml`. For all four tools to receive hooks, rerun `agsy init` so the adapters add `.codex`, `.cursor`, `.agents/hooks.json` and the `.claude` `merge`, or add them by hand per [Configuration](config.md).

→ Back to: [Core Concepts](overview.md)
