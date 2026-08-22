# Quick Start: First Sync in Four Steps

The whole path is four steps, roughly 10 minutes end to end:

```
 Step 0          Step 1          Step 2          Step 3
 Prepare    ──▶  init        ──▶  plan       ──▶  apply           ──▶ 🎉 every tool in sync
 sources         (one Q&A)       (read-only       (build + mount)
 (place files)                    preview)
```

> The terminal screens below are **illustrative** (details may vary by version) — they show what to expect at each step.

## Two ways to operate

agsy has a dual entry point: no arguments opens an interactive menu, arguments run a command directly. If you'd rather not memorize commands, just type `agsy`:

```text
$ agsy
agsy v1.2.3

  Status: behind 0 │ new 2 │ local changes 1 │ untracked 0 │ missing outputs 0 │ mount issues 0

What would you like to do?
  apply    rebuild outputs and mount      ⚠ 1 local change, confirmation required first
  plan     preview changes without writing
  promote  write back local changes (1 item)
> status   view detailed status
  doctor   environment health check
  init     configure (enters edit mode if config exists)
  clean    remove outputs and mounts
  Exit
```

On the first run in a project, with no config file yet, the menu leads you straight into `init`.

## Command cheat sheet

| Command | What it does | Writes anything? |
|---------|--------------|------------------|
| `agsy init [sources...]` | Q&A-style generation of `agsy.yaml` (edit mode if it exists) | writes `agsy.yaml` |
| `agsy doctor` | environment health check: config, sources, mount points, link capability | read-only |
| `agsy plan` | full preview of build + mount | read-only |
| `agsy apply` | pre-checks → confirm → wipe & rebuild artifacts → mount | writes `.agsy/` and links |
| `agsy status` | three-way comparison of sources / artifacts / mounts | read-only (action menu at the end) |
| `agsy promote` | write artifact-end changes back to sources | writes sources |
| `agsy clean` | remove links and `.agsy/` (uninstall; keeps `agsy.yaml`) | deletes artifacts |
| `agsy version` / `agsy help` | version / usage | read-only |

Global flag: `--yes` (`-y`) = answer yes to every confirmation, for CI, scripts, and git hooks. **Without `--yes`, any action needing confirmation in a non-interactive environment is cancelled — never forced.**

## Step 0: prepare source directories

Prepare at least one source. Subdirectories **must** be named `rules/`, `skills/`, `workflows/` (plural — see the [naming convention](overview.md#source-directories-must-follow-the-naming-convention)). The common combination is "personal shared library + in-project library":

```sh
mkdir -p ~/all-ai-lib/rules ~/all-ai-lib/skills ~/all-ai-lib/workflows
mkdir -p ./repo-ai-lib/rules ./repo-ai-lib/skills ./repo-ai-lib/workflows
```

Then place your instruction files by category:

```
~/all-ai-lib/                     ./repo-ai-lib/
├── rules/                        ├── rules/
│   └── python-style.md           │   └── api-naming.md
├── skills/                       ├── skills/
│   └── code-review/              │   (fine to leave empty)
│       └── SKILL.md              └── workflows/
└── workflows/                        └── deploy.md
    └── release.md
```

Format rules, one line each:

- `rules/` — single `.md` files
- `skills/` — a **directory** with a mandatory `SKILL.md` (no symlinks inside)
- `workflows/` — single `.md` files; optional `target:` front matter for routing

## Step 1: `agsy init` — answer five questions

Run `agsy init` at the project root; the whole process is one questionnaire:

```text
$ agsy init
Setting up agsy (Enter accepts the default)

Source paths, ordered by priority (~ prefix = shared library, ./ prefix = in-project)
One per line; press Enter on an empty line to finish (e.g. ~/all-ai-lib, ./repo-ai-lib)
  source 1: ~/all-ai-lib
  source 2: ./repo-ai-lib
  source 3: ⏎

Which tools should be mounted?
  [x] Claude Code (.claude/)
  [ ] OpenAI Codex (.codex/)
  [x] Antigravity (.agents/)

How should same-name conflicts in rules be handled? (recommended rename)
> rename   keep both copies, tagging filenames with their source
  error    stop and list conflicts for you to resolve manually (most conservative)
  first    keep only the copy from the higher-priority source, discard the rest

How should same-name conflicts in skills be handled? (recommended error)…
How should same-name conflicts in workflows be handled? (recommended rename)…

Build output directory [.agsy] ⏎

Where should workflows without a target go?
> All buckets (recommended: a file showing up everywhere is easier to understand)
  Nowhere, and warn during plan / apply

✔ Wrote agsy.yaml

.agsy/ is a rebuildable artifact; add it to .gitignore? y
  Next: agsy plan to preview → agsy apply to execute
```

The five questions at a glance:

| # | Question | Suggested answer | Why |
|---|----------|------------------|-----|
| 1 | Source paths? | shared library first, project library after | order = priority; earlier wins |
| 2 | Which tools to mount? | the ones you actually use | rerun init anytime to add or remove |
| 3 | Same-name conflicts? (asked per category) | rules=`rename`, skills=`error`, workflows=`rename` | rename keeps both, error is safest, first keeps the higher-priority copy |
| 4 | Output directory? | just press Enter (`.agsy`) | must be a dedicated directory inside the project |
| 5 | Untargeted workflows go where? | all buckets | "appears everywhere" beats "mysteriously missing" |

Two closing notes:

- Answer **y** to the `.gitignore` question — artifacts stay out of version control; `agsy.yaml` itself **should** be committed.
- Non-interactive environments (CI): `agsy init --yes ~/all-ai-lib ./repo-ai-lib` — sources as arguments, `--yes` = accept the recommended defaults.

## Step 2: `agsy plan` — read-only preview, three things to check

`plan` rehearses everything apply would do, **guaranteed to write nothing**:

```text
$ agsy plan
read agsy.yaml ✔
sources (by priority):
  [1] ~/all-ai-lib    ✔   tag: @all-ai-lib
  [2] ./repo-ai-lib   ✔   tag: @repo-ai-lib

═══ build preview ═══

rules → .agsy/rules/ (strategy: rename)
  git-commit.md                        ← [1] all-ai-lib
  python-style-fromlib-all-ai-lib.md   ← [1] all-ai-lib    ⚠ duplicate name, source tag appended
  python-style-fromlib-repo-ai-lib.md  ← [2] repo-ai-lib   ⚠ duplicate name, source tag appended

skills → .agsy/skills/ (strategy: error)
  code-review                          ← [1] all-ai-lib

workflows → .agsy/workflows/ (strategy: rename)
  release.md                           ← [1] all-ai-lib   → claude

═══ mount preview ═══

.claude/
  commands   → workflows/claude   (missing, will be created)
  rules      → rules              (missing, will be created)
  skills     → skills             (missing, will be created)

═══ summary ═══
5 items │ 2 renamed │ 0 conflicts │ 0 name collisions │ 0 dropped (first) │ 0 excluded │ 6 links │ 0 mount anomalies │ 0 mount conflicts

No files were written. Run agsy apply once everything looks right.
```

Only three places need your attention:

1. **The `← [n] source-tag` on each line**: confirm every file comes from the source you expect; `⚠` means a same-name item was renamed (both copies kept).
2. **Any `✘` blocks**: name conflicts, collisions, routing errors — apply refuses while any exist; fix them per the list first.
3. **The summary line**: conflicts / collisions / mount anomalies all at 0 → safe to continue.

## Step 3: `agsy apply` — build and mount for real

```text
$ agsy apply
✔ build done: 5 items → .agsy/
✔ mount done: 6 links
```

Two green lines and it is done. Open `.claude/` — `rules`, `skills`, `commands` are now links into `.agsy/`, readable by the tool directly.

If it stops midway, a protection fired: a missing source path, a mount point occupied by a real directory, or unpromoted local changes (listed, with a discard confirmation). Handle per the message and rerun; details in the [command reference](commands.md#agsy-apply).

## The daily loop: three lines

```
 Normal edits:       edit sources  ──▶  agsy apply              (all tools synced)
 Artifact end edited: agsy promote (write back) ──▶ agsy apply  (both ends consistent again)
 Unsure:             agsy status                                (it tells you which to run)
```

**One mental model**: sources are the single source of truth. Normal changes go into sources + `apply`; anything edited on the artifact end (accidentally or via an AI tool) gets collected back with `promote`.

## In CI / git hooks

`status` exit codes are designed for automation: `0` = fully in sync, `1` = gaps found.

```sh
# pre-commit hook: block the commit when sources changed but apply was forgotten
agsy status || { echo "agsy out of sync — run agsy apply first"; exit 1; }

# rebuild inside CI (non-interactive, accept all confirmations)
agsy apply --yes
```

→ Next chapter: [Configuration: agsy.yaml](config.md)
