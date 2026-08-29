# Core Concepts: What is agsy?

## The problem it solves

When you use several AI development tools at once (Claude Code, OpenAI Codex, Google Antigravity, Cursor…), each tool reads its own instruction locations, in its own shapes:

- Claude Code reads rules from `.claude/rules/` and skills from `.claude/skills/`
- Codex reads the root `AGENTS.md` and skills from `.agents/skills/`
- Antigravity reads the root `AGENTS.md`, skills from `.agents/skills/`, and `/name` workflows from `.agents/workflows/`
- Cursor reads the root `AGENTS.md` and skills from `.agents/skills/`

The same coding conventions, skills, and procedures end up copied several times, in several formats, and every change has to be synced to every copy. On top of that you often want a personal shared library layered with a per-project one.

**agsy (agent-sync)** solves exactly this:

> It **merges** instruction files from multiple sources into a single build output directory (`.agsy/` by default), **converts** them into each tool's native shape, then **mounts** the output into each tool's read location via links.

Edit only the sources, run `agsy apply` once, and every tool updates at the same time.

## Know these terms first

The rest of the documentation uses these terms throughout.

| Term | Meaning |
|------|---------|
| source | An original instruction library you maintain (the `sources` array); there can be several |
| output (artifacts) | The directory `apply` builds, `.agsy/` by default. Everything inside is generated and the whole directory can be rebuilt |
| mount | Creating a "link" at each tool's read location that points into the output |
| link (symlink / junction / hard link) | An OS-level pointer at another directory or file — **no second copy of the content exists** |
| category | The three kinds of instruction files: rules, skills, workflows |
| derived form | An output produced by conversion rather than verbatim copy: the concatenated `AGENTS.md`, a workflow's skill form, a workflow's redirect stub |
| manifest | `.agsy/.agsy-manifest.json`, the build record; agsy uses it to tell what changed on which side |
| source tag | The source identifier appended to a filename when same-name items are kept via rename, e.g. `-fromlib-all-ai-lib` |
| adapter | A built-in mount preset for a tool, used by `init` to generate the mount config |
| tools | The `build.tools` list; the closed set of names a workflow's `target:` may reference |

## One-way data flow

agsy's data flow is strictly one way:

```
 sources you maintain ──▶ build (copy + convert) ──▶ .agsy/ ──▶ mount (links) ──▶ each tool
```

**The sources are the single source of truth. Everything in `.agsy/` — and therefore everything a tool reads through a mount — is a read-only, rebuildable artifact.** There is no write-back: when a mounted file is edited, `status` reports the change and `apply` lists it, asks for confirmation, and rebuilds over it. Keeping a change always means moving it into a source by hand (status names the destination), so AI-authored content passes human review before entering the library.

## The three layers

```
┌─────────────────────┐
│  sources             │  ~/all-ai-lib/       (personal shared library)
│  (originals you      │  ./repo-ai-lib/      (in-project library)
│   maintain)          │
└─────────┬───────────┘
          │  ① agsy apply: scan → merge → copy → convert
          ▼
┌─────────────────────┐
│  output              │  .agsy/rules/        rules, verbatim
│  (rebuildable,       │  .agsy/AGENTS.md     rules, concatenated (derived)
│   read-only)         │  .agsy/skills/       skills + workflow skill forms
│                      │  .agsy/workflows/    workflow stubs
└─────────┬───────────┘
          │  ② agsy apply: create links
          ▼
┌─────────────────────┐
│  mount               │  AGENTS.md        → .agsy/AGENTS.md
│  (where AI tools     │  .claude/rules    → .agsy/rules
│   actually read)     │  .claude/skills   → .agsy/skills
│                      │  .agents/skills   → .agsy/skills
│                      │  .agents/workflows→ .agsy/workflows
└─────────────────────┘
```

- **Sources**: the originals you maintain and version-control. Order = priority (earlier wins).
- **Output** (`build.out`, `.agsy/` by default): the built product. `apply` wipes and rebuilds it every time.
- **Mount**: links at each tool's read location. Directories are symlinks (junctions on Windows); the root `AGENTS.md` is a file symlink (hard link on Windows). Tools see links; the content lives in `.agsy/`.

## The three categories

Instruction files are split into three categories by purpose. Sources store them in three subdirectories (`rules/`, `skills/`, `workflows/` by default), each with its own format rules:

| Category | Source format | What it is | Output forms |
|----------|--------------|------------|--------------|
| rules | single `.md` files | long-lived conventions and style guides, always in context | verbatim copies in `rules/` (for Claude Code) **and** one concatenated `AGENTS.md` (for Codex / Cursor / Antigravity) |
| skills | **directory** containing `SKILL.md` | a packaged capability; tools pick it up when a task matches its description | verbatim copies in `skills/` |
| workflows | single `.md` files | human-triggered procedures and SOPs, invoked as `/name` | a **skill form** in `skills/` (front matter gains `disable-model-invocation: true`, so tools that honor it never run the procedure on their own) **and** a redirect **stub** in `workflows/` for Antigravity's `/name` |

Why workflows become skills: Claude Code, Codex and Cursor read commands and procedures through the skills mechanism, gated by front matter rather than by a separate directory. A workflow source stays a plain single file; build packages it into the skill shape those tools expect. Antigravity reads a `workflows/` directory for `/name` invocation, so build leaves a short stub there that tells the agent to load the corresponding skill (the content exists exactly once, in the skill form).

## What each tool ends up reading

| Tool | rules | skills | workflows |
|------|-------|--------|-----------|
| Claude Code | `.claude/rules/` (per-file) | `.claude/skills/` | skill form in `.claude/skills/`, invoked as `/name` |
| Codex | root `AGENTS.md` | `.agents/skills/` | skill form in `.agents/skills/` |
| Antigravity | root `AGENTS.md` | `.agents/skills/` | stub in `.agents/workflows/` → `/name` loads the skill |
| Cursor | root `AGENTS.md` | `.agents/skills/` | skill form in `.agents/skills/`, invoked as `/name` |

`.agents/skills/` is one directory natively read by Codex, Antigravity and Cursor — one link serves all three. Claude Code does not read `AGENTS.md`, which is why it alone gets the per-file `.claude/rules/` mount.

## Source directories must follow the naming convention

By default agsy scans `rules/`, `skills/` and `workflows/` inside each source. A library that has always used different names can be connected without moving files via `build.categories.<cat>.from` — see [Configuration](config.md).

→ Next chapter: [Installation](install.md)
