# Quick Start

A first sync in four steps, all inside the project folder.

```
 ① prepare a source  ──▶  ② agsy init  ──▶  ③ agsy plan  ──▶  ④ agsy apply
    put files in it         write config      preview only       build + mount
```

## Step 1: prepare a source folder

A source is a folder with up to four subfolders; keep only the ones you need:

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # a .md file
├── skills/
│   └── code-review/
│       └── SKILL.md             # a folder with SKILL.md
├── workflows/
│   └── deploy.md                # a .md file
└── hooks/
    └── block-rm/
        ├── hook.yaml            # declares: at which moment, which script
        └── block-rm.sh          # the script (exit code 2 = block)
```

A common setup is two sources: a personal shared library (`~/all-ai-lib`) plus one inside the project (`./repo-ai-lib`).

## Step 2: `agsy init` writes the config

Run it in the project folder and answer the prompts. Enter accepts the default.

```
$ cd your-project
$ agsy init

Source paths in priority order (~ = shared library, ./ = inside the project)
  source 1: ~/all-ai-lib
  source 2: ./repo-ai-lib
  source 3: ⏎

Which tools to serve? (a = all)
    1) Antigravity (.agents/)
    2) Claude Code (.claude/)
    3) OpenAI Codex (.agents/, .codex/)
    4) Cursor (.agents/, .cursor/)
Your choice: a

Same-name conflicts in rules?        ❯ rename
Same-name conflicts in skills?       ❯ error
Same-name conflicts in workflows?    ❯ rename
Same-name conflicts in hooks?        ❯ error

Build output directory (default: .agsy): ⏎

✔ wrote agsy.yaml

The following generated paths are rebuildable and usually belong in .gitignore:
Add which entries to .gitignore? (a = all) a
```

A "same-name conflict" is two sources holding a file with the same name: `rename` keeps both (the source name is appended to the filename), `error` stops and lets you decide, `first` keeps only the higher-priority one.

The last question is whether to add the generated paths to `.gitignore`; that is your call. `agsy.yaml` itself is meant to be committed.

Non-interactive form for scripts and CI: `agsy init --yes ~/all-ai-lib ./repo-ai-lib`.

## Step 3: `agsy plan` previews

```
$ agsy plan
```

Lists everything apply would do: which files are collected, which are renamed, what each workflow produces, which tools each hook reaches, what happens to each link. Nothing is written. Adjust and run it again if something looks off.

## Step 4: `agsy apply` builds and mounts

```
$ agsy apply
✔ build done: 13 items → .agsy/
✔ mount done: 8 links
✔ merge done: .claude/settings.json ← hooks.claude.json
```

The project afterwards (`→` is a link, `⇐` a merge):

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         (all rules, one file)
├── .claude/
│   ├── rules          → .agsy/rules
│   ├── skills         → .agsy/skills
│   └── settings.json  ⇐ .agsy/hooks.claude.json (only the "hooks" field)
├── .agents/
│   ├── skills         → .agsy/skills
│   ├── workflows      → .agsy/workflows
│   └── hooks.json     → .agsy/hooks.antigravity.json
├── .codex/
│   └── hooks.json     → .agsy/hooks.codex.json
├── .cursor/
│   └── hooks.json     → .agsy/hooks.cursor.json
└── .agsy/             the output
```

Every tool reads the same content from its own location. Typing `/deploy` in Claude Code or Cursor runs that workflow; `/deploy` in Antigravity does too. All four run `block-rm.sh` before executing a shell command; when it returns exit code 2, the command does not run.

## Day to day

```
 edit a source  ──▶  agsy apply  ──▶  all four tools are current
                        ▲
 agsy status ───────────┘  shows what is out of sync (exit code 1 when anything is)
```

When an AI tool changes the output through a link (adds a rule, say), `agsy status` lists it and names the source to move it to. Move what you want to keep, then apply.

## Command cheat sheet

```
agsy            menu with a status summary
agsy doctor     environment check (read-only)
agsy plan       preview (read-only)
agsy apply      build + mount (lists what will be discarded and asks first)
agsy status     two gap lists + link state (read-only)
agsy clean      remove what agsy created in this project
```

→ Next: [Configuration](config.md)
