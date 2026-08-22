# Core Concepts: What is agsy?

## The problem it solves

When you use several AI development tools at once (Claude Code, OpenAI Codex, Antigravity…), each tool reads its own instruction directory:

- Claude Code reads `.claude/rules/`, `.claude/skills/`, `.claude/commands/`
- Codex reads `.codex/prompts/`
- Antigravity reads `.agents/`

The same coding conventions, skills, and workflows end up copied several times, and every change has to be synced to every copy. On top of that you often want a personal shared library layered with a per-project one.

**agsy (agent-sync)** is the merge-and-mount tool for exactly this:

> It **merges** instruction files from multiple sources into a single build output directory (`.agsy/` by default), then **mounts** it into each tool's read location via directory links.

Edit only the sources, run `agsy apply` once, and every tool updates at the same time.

## Know these terms first

The rest of the documentation uses these terms throughout. Skim them now; come back to this table whenever you need a refresher.

| Term | Meaning |
|------|---------|
| source | An original instruction library you maintain (the `sources` array); there can be several |
| build out (artifacts) | The directory `apply` builds, `.agsy/` by default. Everything inside is a copy and the whole directory can be rebuilt |
| mount | Creating a "link" inside each tool's directory that points into the artifacts |
| link (symlink / junction) | An OS-level "shortcut": an entry that points at another directory. Opening it opens the target — **no second copy of the files exists** |
| category | The three kinds of instruction files: rules, skills, workflows |
| bucket | Per-tool subdirectories under `workflows/` in the artifacts (e.g. `claude`, `codex`); each tool mounts its own bucket |
| routing | The build-time decision of which buckets each workflow goes into, driven by the file's `target` marker |
| manifest | `.agsy/.agsy-manifest.json`, the build record; agsy uses it to tell which side changed |
| source tag | The source identifier appended to a filename when same-name items are kept via rename, e.g. `-fromlib-all-ai-lib` |
| adapter | A built-in mount preset for a tool, used by `init` to generate the mount config |
| behind | The source was updated but the artifacts were not rebuilt yet → run `apply` |
| local changes | The artifact side was edited and not yet written back → run `promote` |
| untracked | Files added on the artifact side that the manifest does not know (`apply` deletes them; move them into a source to keep them) |
| orphan (orphan link) | A link created by an earlier `apply` that the current config no longer references |

> **Two ends and a channel**: file content lives at exactly two ends — the "**source end**" (originals inside sources) and the "**artifact end**" (copies inside `.agsy/`). Mounts are links: a file opened through a tool directory (`.claude/` etc.) *is* the artifact end. The link itself is called the "**mount link**" — a channel that stores no content; see [Scenario Guide "Mount-link scenarios"](scenarios.md#mount-link-scenarios) for its failure modes.

## The three layers

The agsy world has three layers and two actions. The simplest picture first:

```
 Sources you maintain   ── ①build (copy) ──▶   Artifact dir   ── ②mount (link) ──▶   Each AI tool's dir
   sources                                      .agsy/                                .claude/ .codex/ …
```

- **Sources**: the originals you maintain and version-control. There can be several; order = priority (earlier wins).
- **Artifacts** (`build.out`, `.agsy/` by default): the built product. **Treat the whole directory as disposable, rebuildable output** — `apply` wipes and rebuilds it every time, so never put hand-written work directly inside (unless you plan to write it back with `promote`).
- **Mount**: links inside each tool's directory pointing into the artifacts. Tools see links; the content lives in `.agsy/`.

Now look at ① and ② separately.

### Action ① "build": merge-copy multiple sources into one

```
 ~/all-ai-lib/rules/python-style.md  ──┐
 ~/all-ai-lib/rules/git-commit.md    ──┤  copy & merge
 ./repo-ai-lib/rules/api-naming.md   ──┘
                                        ▼
                               .agsy/rules/python-style.md
                               .agsy/rules/git-commit.md
                               .agsy/rules/api-naming.md
```

One takeaway: **artifact files are copies**. Sources can live in several places (a personal shared library, a per-project one…); the build gathers them into one place — and precisely because they are copies, `.agsy/` can be deleted and rebuilt at any time.

### Action ② "mount": create a link, not another copy

```
 .claude/rules   ────────── link (shortcut) ──────────▶   .agsy/rules/
 (tools read here)                                       (files actually live here)
```

One takeaway here too: `.claude/rules` **is not a real directory** — it is a link (symlink on macOS/Linux, junction on Windows). Like a shortcut, opening it shows the content of `.agsy/rules/`.

So: the same file never occupies space twice, and the moment `.agsy/` is updated, every tool sees the new content **immediately** — no extra sync step.

### Both actions together: the full picture

With ① and ② understood, a fully mounted project looks like this:

```
┌─────────────────────┐
│  sources             │  ~/all-ai-lib/       (personal shared library)
│  (originals you      │  ./repo-ai-lib/      (in-project library)
│   maintain)          │
└─────────┬───────────┘
          │  ① agsy apply: scan → merge → copy
          ▼
┌─────────────────────┐
│  build.out           │  .agsy/rules/
│  (rebuildable        │  .agsy/skills/
│   copies)            │  .agsy/workflows/<bucket>/
└─────────┬───────────┘
          │  ② agsy apply: create directory links (symlink / junction)
          ▼
┌─────────────────────┐
│  mount               │  .claude/rules  → ../.agsy/rules
│  (where AI tools     │  .claude/skills → ../.agsy/skills
│   actually read)     │  .codex/prompts → ../.agsy/workflows/codex
└─────────────────────┘
```

## The three categories

Instruction files are split into three categories by purpose:

- **rules**: long-lived conventions and style guides — coding style, naming, commit message format… AI tools read them as always-on background rules.
- **skills**: a packaged "capability" — a directory holding a description (`SKILL.md`) plus optional scripts and assets. Tools pick a skill up automatically when a task matches its description.
- **workflows**: step-by-step procedures or reusable commands — a release flow, a code-review SOP… In Claude Code they become slash commands (`/release` and the like).

Sources store them in three same-named subdirectories, each with its own format rules:

| Category | Source subdir (default) | Format | Output location |
|----------|------------------------|--------|-----------------|
| rules | `rules/` | single `.md` file | `.agsy/rules/` |
| skills | `skills/` | **directory** containing `SKILL.md` | `.agsy/skills/` |
| workflows | `workflows/` | single `.md` file, optional `target` front matter | `.agsy/workflows/<bucket>/` |

A typical source:

```
~/all-ai-lib/
├── rules/
│   ├── python-style.md
│   └── git-commit.md
├── skills/
│   └── code-review/
│       ├── SKILL.md
│       └── scripts/…
└── workflows/
    └── release.md        (front matter: target: claude)
```

## Supported tools and their directories

After mounting, where does each category appear for each tool? The built-in adapters currently cover three tools (details and customization in the [Adapters chapter](adapters.md)):

| Tool | Mount dir | rules | skills | workflows |
|------|-----------|-------|--------|-----------|
| Claude Code | `.claude/` | `.claude/rules` | `.claude/skills` | `.claude/commands` (become slash commands) |
| OpenAI Codex | `.codex/` | — (not mounted) | — (not mounted) | `.codex/prompts` |
| Antigravity | `.agents/` | `.agents/rules` | `.agents/skills` | `.agents/workflows` |

Every cell in this table is a **link**; the actual content lives in `.agsy/`. Codex, per its conventions, only reads prompts, so rules and skills have no mount location there (`init` warns about this when only Codex is selected).

## Source directories must follow the naming convention

Whether it is a personal shared library (`~/all-ai-lib`) or an in-project one (`./repo-ai-lib`), **the subdirectories must be named `rules/`, `skills/`, `workflows/`** (plural) — the scanner only recognizes these three names. A misspelled or differently named directory (`rule/`, `my-rules/`) is skipped entirely and its files silently never appear.

```
✔ correct                        ✘ not scanned
~/all-ai-lib/rules/…             ~/all-ai-lib/rule/…
~/all-ai-lib/skills/…            ~/all-ai-lib/Skill/…
~/all-ai-lib/workflows/…         ~/all-ai-lib/wf/…
```

> If an existing library really uses different names, there is no need to move files: set `build.categories.<category>.from` in `agsy.yaml` to the actual subdirectory name — see the [Configuration chapter](config.md). When unsure whether something is being scanned, `agsy doctor` gives an immediate answer.

## Workflow routing: buckets and routing

rules and skills are built once and **shared by every tool**; workflows are different — command/prompt formats are not portable across tools, so the artifact `workflows/` directory is subdivided per tool:

```
.agsy/workflows/
├── claude/        ←  Claude Code only (.claude/commands mounts here)
└── codex/         ←  Codex only (.codex/prompts mounts here)
```

Connecting the two terms:

- These subdirectories are **buckets**: one bucket per tool; each tool mounts only its own.
- The build-time decision of which buckets each workflow lands in is **routing**.

Routing is driven by the `target` marker in a workflow's front matter:

```
 workflows/release.md (target: claude)           ──▶  claude bucket only → visible to Claude Code only
 workflows/deploy.md  (target: [claude, codex])  ──▶  one copy per bucket → visible to both tools
 workflows/note.md    (no target)                ──▶  destination decided by route.default
```

The marker sits at the very top of the file:

```markdown
---
target: claude          # Claude Code only
# or target: [claude, codex]  # both
---
Release procedure…
```

**What if `target` is missing?** The workflow falls into the buckets listed in `route.default`. Setting default to "all buckets" is recommended — a file that appears everywhere is easier to reason about than one that silently disappears. Details in the [Configuration](config.md) and [Adapters](adapters.md) chapters.

## Two-way data flow: apply and promote

**apply (forward): sources → artifacts.** Normal edits take this path — edit a source, run apply, every tool syncs:

```
 sources (you edit here)
    │
    │  agsy apply (rebuild + mount)
    ▼
 .agsy/ ──links──▶  every tool reads the new content immediately
```

**promote (reverse): artifacts → sources.** When you (or an AI tool) edit a file through a tool directory — mounts are links, so the artifact-end copy in `.agsy/` is what actually changed — promote writes the change back:

```
 .claude/skills/… (an AI tool edited a file here)
    ‖  the mount is a link, so the artifact-end copy in .agsy/ is what changed
    │
    │  agsy promote (write back)
    ▼
 sources (the change is preserved — the next apply will not overwrite it)
```

agsy records each item's source and artifact hashes at build time in `.agsy/.agsy-manifest.json`, so `status` can tell precisely: which items are "source updated, not yet rebuilt" (behind), which are "artifact edited, not yet written back" (local changes), and whether both sides changed at once (manual merge needed).

## Built-in safety rules

These principles appear throughout the documentation:

1. **Real files are never touched**: if a mount point is occupied by a real directory or file (not an agsy-created link), agsy reports an error and asks you to handle it — it never deletes on your behalf.
2. **Conflict strategy must be explicit**: `on_conflict` is required per category; there is no implicit default.
3. **Confirmation before deletion**: `apply` wipes the artifact directory; if unpromoted changes are detected it always asks first. Non-interactive runs without `--yes` are cancelled, never forced.
4. **Artifact directory guard rails**: `build.out` may only be a dedicated directory inside the project; pointing it at the home directory, a source, or the project root is rejected at config validation.
5. **Symbolic links are never collected**: symlinks in sources (including inside skill directories) are skipped, so a link can never smuggle files from outside a source into the artifacts.

→ Next chapter: [Installation](install.md)
