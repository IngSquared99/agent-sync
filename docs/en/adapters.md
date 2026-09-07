# Adapters

An adapter is one tool's built-in mount preset: which folder it reads and which parts of the output it should see. `agsy init` turns the adapters you pick into the `mount` section and `build.tools` of `agsy.yaml`. Adapters are templates; the generated config is yours to edit.

## The four built-in adapters

| Adapter | Mounts | rules read from | hooks read from |
|---------|--------|-----------------|-----------------|
| `claude` (Claude Code) | `.claude/rules → rules`, `.claude/skills → skills`, `.claude/settings.json ⇐ hooks.claude.json` (merge) | `.claude/rules/`, one file each | the `hooks` field of `settings.json` |
| `codex` (OpenAI Codex) | `.agents/skills → skills`, `.codex/hooks.json → hooks.codex.json` | root `AGENTS.md` | `.codex/hooks.json` |
| `antigravity` (Google Antigravity) | `.agents/skills → skills`, `.agents/workflows → workflows`, `.agents/hooks.json → hooks.antigravity.json` | root `AGENTS.md` | `.agents/hooks.json` |
| `cursor` (Cursor) | `.agents/skills → skills`, `.cursor/hooks.json → hooks.cursor.json` | root `AGENTS.md` | `.cursor/hooks.json` |

Details:

- Claude Code does not read `AGENTS.md`, so it gets the per-file `.claude/rules/` mount. Its hooks have no separate file, hence merge. Workflows arrive as skills, triggered with `/name`.
- Antigravity gets no `.agents/rules` mount: it reads `AGENTS.md`, and a rules folder on top would show every rule twice. `/name` runs the stub, which loads the skill.
- Codex and Cursor each use two folders: skills through the shared `.agents`, hooks through their own `.codex` / `.cursor`.

Picking any of `codex`, `antigravity` or `cursor` adds the root mount:

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

Adapters sharing a folder are merged into one mount entry: picking all three produces a single `.agents` block.

## The shared `.agents` folder

```
 .agents/skills/  ◀── Codex
                  ◀── Antigravity
                  ◀── Cursor
```

All three read `.agents/skills/` natively; one link serves them all, and every workflow's skill form reaches all three.

Two tool differences:

- A workflow's skill form carries `disable-model-invocation: true`. Claude Code and Cursor honour it: only a person can trigger the procedure. Codex and Antigravity ignore the field; the model may run the workflow on its own, so watch procedures with side effects.
- In Antigravity, `/name` runs the stub, one indirection more; in rare cases the AI may not follow it.

## The four hook locations

The hook mechanism is the same in all four tools (your script runs at fixed moments of the AI's loop); what differs is where the registry lives and its format:

| Tool | Registry | Mounted by | Format |
|------|----------|-----------|--------|
| Claude Code | the `hooks` field of `.claude/settings.json` | merge | event → group → handler |
| Codex | `.codex/hooks.json` | link | same as Claude |
| Antigravity | `.agents/hooks.json` | link | wrapped in an extra hook-name layer |
| Cursor | `.cursor/hooks.json` | link | different event names, flat structure |

Translation details are in the `hook.yaml` section of [Configuration](config.md).

## Custom mounts

A tool outside the four can be served with a hand-written mount entry:

```yaml
mount:
  - dir: .someothertool
    links:
      skills: skills
```

`init`'s edit mode keeps custom entries. A link may point at a category's `to`, at `AGENTS.md` or at a registry. A custom tool gets no registry of its own: registries are produced for the four built-in tools only.

## Adding a built-in adapter (for developers)

Adapters live in the source tree under `adapters/`, one YAML per tool:

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # set when the tool reads the root AGENTS.md
mounts:
  - dir: .newtool
    links:
      skills: skills
```

Drop the file in, rebuild, and `init` offers the tool. For a hook registry as well, add a dialect entry in `internal/build/hooks.go` (event mapping, supported handler types, registry shape) and register the file name in `internal/config`.

→ Next: [Scenario Guide](scenarios.md)
