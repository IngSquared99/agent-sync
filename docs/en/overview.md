# Core Concepts: What is agsy?

This chapter is about the problem and the approach, not about operating the tool. Read it before Installation and Quick Start; every term used in later chapters is defined here.

## 1. The problem: one set of rules, four places to keep it

AI coding tools (Claude Code, OpenAI Codex, Google Antigravity, Cursor) all read "instruction files for the AI": coding conventions, skills, procedures, guard scripts. Each reads them from a different place, in a different shape:

```
 one "coding conventions" file

   Claude Code  ──▶ .claude/rules/       (one file per rule)
   Codex        ──▶ AGENTS.md            (all rules in one file)
   Antigravity  ──▶ AGENTS.md
   Cursor       ──▶ AGENTS.md

 one "block rm -rf" guard

   Claude Code  ──▶ a section inside .claude/settings.json
   Codex        ──▶ .codex/hooks.json
   Antigravity  ──▶ .agents/hooks.json
   Cursor       ──▶ .cursor/hooks.json     (each in its own format)
```

With two or more tools, every instruction exists as several copies, and every edit has to be repeated for each.

## 2. What agsy does: maintain one copy, generate the rest

agsy is a command-line tool. You maintain **one** set of instruction files (the "source"), run `agsy apply` once, and it does three things:

```
 ① source           ② output                    ③ mount
 the one copy   ──▶ copied + converted     ──▶ a "link" at each tool's read
 you maintain       into .agsy/                location, pointing at the output

 example:
 ~/all-ai-lib/rules/python-style.md
        │
        ▼ apply
 .agsy/rules/python-style.md         ◀── .claude/rules (link)
 .agsy/AGENTS.md (all rules, one file) ◀── AGENTS.md (link)
```

A "link" is an operating-system feature: something that looks like a folder or file but points at another location. When a tool opens `.claude/rules/`, it sees the content of `.agsy/rules/`; there is no second copy.

Edit the source, run `agsy apply` again, and all four tools update at once.

## 3. Terms

The rest of the documentation uses these words.

| Term | Meaning | Think of it as |
|------|---------|----------------|
| source | A folder of instruction files you maintain; there can be several | the original |
| output | The folder `apply` generates, `.agsy/` by default. Everything inside is generated and can be rebuilt any time | a printed copy |
| mount | Placing links at each tool's read location, pointing at the output | putting the copy on each desk |
| link | An OS-level pointer at another folder or file; the content exists once | a shortcut |
| category | The four kinds of instruction file: rules, skills, workflows, hooks | see next section |
| derived | Output produced by conversion rather than copying: the combined `AGENTS.md`, a workflow turned into a skill, each tool's hook registry | a translation |
| registry | One hooks file per tool, `.agsy/hooks.<tool>.json` | a roster in each tool's format |
| merge | Claude Code only: merging the hooks registry into the `hooks` field of `.claude/settings.json`, leaving everything else untouched | adding your lines to someone else's notebook |
| manifest | `.agsy/.agsy-manifest.json`, the record of the last apply; `status` uses it to tell which side changed | the stub of the last print run |
| source tag | The source name appended to a filename when both copies of a same-named file are kept | a stamp of origin |
| adapter | A tool's built-in mount preset; `init` uses it to generate the config | a factory template |
| tools | The list of tools to serve, in the config | the recipient list |

## 4. The four categories

Instruction files come in four kinds, each in its own subfolder of a source.

| Category | What it is | Source shape | In a word |
|----------|-----------|--------------|-----------|
| rules | Standing conventions and style requirements the AI reads and follows | one `.md` file | teach |
| skills | Packaged abilities the AI picks up on its own when a task matches | a folder with `SKILL.md` | know |
| workflows | Procedures a person triggers by typing `/name` | one `.md` file | do |
| hooks | Small programs that run before or after the AI acts; they block what is not allowed, and the AI cannot opt out | a folder with `hook.yaml` and scripts | block |

**Rules teach, hooks block.** Rules are text the AI reads; whether it complies is probabilistic. Hooks are programs that run outside the AI; blocked means blocked. Anything that can be written as "if … then not allowed" belongs in hooks; style and preference, which no program can judge, belong in rules.

## 5. What each category produces

```
 source                      output (.agsy/)
 ─────────────────────       ───────────────────────────────────────
 rules/a.md           ──▶    rules/a.md              as is
                      ──▶    AGENTS.md               all rules in one file
 skills/x/SKILL.md    ──▶    skills/x/SKILL.md       as is
 workflows/deploy.md  ──▶    skills/deploy/SKILL.md  as a skill (Claude / Codex / Cursor)
                      ──▶    workflows/deploy.md     a stub (Antigravity's /deploy)
 hooks/block-rm/      ──▶    hooks/block-rm/         as is (scripts + hook.yaml)
                      ──▶    hooks.claude.json       ┐
                      ──▶    hooks.codex.json        │ one registry per tool
                      ──▶    hooks.antigravity.json  │
                      ──▶    hooks.cursor.json       ┘
```

Why a workflow becomes a skill: Claude Code, Codex and Cursor read procedures through the skills mechanism, with a header field deciding who may trigger them. The source stays a plain `.md`; agsy does the conversion. Antigravity triggers `/name` from the `workflows/` folder, so a short "stub" is left there telling the AI to load the matching skill.

## 6. What each tool ends up reading

| Tool | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/` | `.claude/skills/` | the skill in `.claude/skills/`, via `/name` | the `hooks` field of `.claude/settings.json` (merge) |
| Codex | root `AGENTS.md` | `.agents/skills/` | same | `.codex/hooks.json` |
| Antigravity | root `AGENTS.md` | `.agents/skills/` | the stub in `.agents/workflows/` | `.agents/hooks.json` |
| Cursor | root `AGENTS.md` | `.agents/skills/` | same as Claude | `.cursor/hooks.json` |

`.agents/skills/` is read by Codex, Antigravity and Cursor alike: one link serves three tools. Claude Code does not read `AGENTS.md`, so it gets its own `.claude/rules/` mount.

## 7. One direction only

```
 source ──▶ apply ──▶ output ──▶ link ──▶ tool
```

The source is the only original. The output, and everything the tools read, is a rebuildable copy; nothing is written back from copy to original. When an AI tool edits the output through a link (adds a rule for you, say), `agsy status` lists it; to keep it, move it into a source yourself, then apply. That manual step is the review gate: AI-produced content passes through a person before it enters the original.

## 8. agsy only writes inside the repository

- Sources may live anywhere (a shared library in your home folder, `~/all-ai-lib`); agsy only reads them.
- The output and the links live inside the project folder.
- Each tool's personal settings (`~/.claude`, `~/.codex` and the like) are not write targets; that is where your own things go.

## 9. Naming the source subfolders

agsy looks for `rules/`, `skills/`, `workflows/` and `hooks/` in every source; any may be missing. An existing library that uses other names (`prompts/`, say) can be pointed at through `build.categories.<category>.from` in the config, without moving files; see [Configuration](config.md).

→ Next: [Installation](install.md)
