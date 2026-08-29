# Quick Start

First sync in four steps, all inside the project directory.

## Step 0: prepare a source library

A source is a directory with up to three subdirectories; any subset works:

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # a plain markdown file
├── skills/
│   └── code-review/
│       └── SKILL.md             # a directory with a SKILL.md
└── workflows/
    └── deploy.md                # a plain markdown file, optional target: front matter
```

You can point at one library or several; a common setup is a personal shared library (`~/all-ai-lib`) plus an in-project one (`./repo-ai-lib`).

## Step 1: `agsy init` — generate the config

```
$ cd your-project
$ agsy init
Setting up agsy (Enter accepts the default)

Source paths, ordered by priority (~ prefix = shared library, ./ prefix = in-project)
  source 1: ~/all-ai-lib
  source 2: ./repo-ai-lib
  source 3: ⏎

Which tools should be served? (space-separate multiple numbers, a = all, Enter = all)
    1) Claude Code (.claude/)
    2) OpenAI Codex (.agents/)
    3) Antigravity (.agents/)
    4) Cursor (.agents/)
Enter your choice: a

How should same-name conflicts in rules be handled?(recommended rename…)     ❯ rename
How should same-name conflicts in skills be handled?(recommended error…)     ❯ error
How should same-name conflicts in workflows be handled?                      ❯ rename

Build output directory (default: .agsy): ⏎

✔ Wrote agsy.yaml

The following generated paths are rebuildable and usually belong in .gitignore:
Add which entries to .gitignore? (a = all) a
  ✔ Added 6 entries to .gitignore
  Next: agsy plan to preview → agsy apply to execute
```

Answer **a** (all) to the `.gitignore` question unless your team versions the links on purpose; `agsy.yaml` itself **should** be committed.

Non-interactive form for scripts: `agsy init --yes ~/all-ai-lib ./repo-ai-lib`.

## Step 2: `agsy plan` — preview without writing

```
$ agsy plan
```

The preview lists, per category, everything the build would collect: which rules get renamed by the conflict strategy, which forms each workflow produces (`skill skills/deploy` / `stub workflows/deploy.md`), the derived `AGENTS.md` line, every excluded file with its reason, and what happens to each mount link. Nothing is written; adjust and rerun plan as needed.

## Step 3: `agsy apply` — build and mount

```
$ agsy apply
✔ build done: 12 items → .agsy/
✔ mount done: 6 links
```

Resulting project layout:

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         (all rules, concatenated)
├── .claude/
│   ├── rules          → .agsy/rules
│   └── skills         → .agsy/skills
├── .agents/
│   ├── skills         → .agsy/skills
│   └── workflows      → .agsy/workflows
└── .agsy/             the built output
```

Each tool reads its native locations and finds the same content. `/deploy` in Claude Code or Cursor runs the workflow's skill form; in Antigravity it runs the stub, which loads that skill.

## Step 4: the daily loop

```
edit sources  ──▶  agsy apply  ──▶  every tool is current
                     ▲
status: check gaps ──┘  (exit code 1 when anything is out of sync)
```

When an AI tool writes through a mount (a new rule, an edited skill), `agsy status` lists it with guidance; move what should be kept into a source, then apply. Details: [Command Reference](commands.md) and [Scenario Guide](scenarios.md).

## Command cheat sheet

```
agsy            menu with a status summary
agsy doctor     environment health check
agsy plan       preview (read-only)
agsy apply      build + mount (confirms discards first)
agsy status     two gap lists + mount health (read-only, CI-friendly exit code)
agsy clean      uninstall from this project
```

→ Next chapter: [Configuration](config.md)
