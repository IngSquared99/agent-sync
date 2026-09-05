# Adapters

An adapter is a built-in mount preset for one AI tool: which directory it reads, and which parts of the output it should see. `agsy init` turns the adapters you select into the `mount` section (and the `build.tools` list) of `agsy.yaml`. Adapters are factory templates, not a runtime dependency — the generated config is yours to edit.

## Built-in adapters

| Adapter | Mounts | Rules via | Hooks via | Notes |
|---------|--------|-----------|-----------|-------|
| `claude` (Claude Code) | `.claude/rules → rules`, `.claude/skills → skills`, `.claude/settings.json ⇐ hooks.claude.json` (merge) | per-file `.claude/rules/` | the `hooks` key of `settings.json` | Claude Code does not read `AGENTS.md`; its hooks have no separate file, hence merge; workflows arrive as skills, invoked with `/name` |
| `codex` (OpenAI Codex) | `.agents/skills → skills`, `.codex/hooks.json → hooks.codex.json` | root `AGENTS.md` | `.codex/hooks.json` | two mount dirs: skills through the shared `.agents`, hooks through Codex's own `.codex` |
| `antigravity` (Google Antigravity) | `.agents/skills → skills`, `.agents/workflows → workflows`, `.agents/hooks.json → hooks.antigravity.json` | root `AGENTS.md` | `.agents/hooks.json` | `.agents/rules` is not mounted — Antigravity reads `AGENTS.md`, so a rules directory would duplicate every rule. `/name` invokes the stub, which loads the skill; hooks sit in the already-mounted `.agents` |
| `cursor` (Cursor) | `.agents/skills → skills`, `.cursor/hooks.json → hooks.cursor.json` | root `AGENTS.md` | `.cursor/hooks.json` | Cursor reads `.agents/skills/` natively, so it shares the `.agents` mount; hooks go through `.cursor` |

Selecting any of `codex`, `antigravity` or `cursor` makes init add the root mount:

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

Adapters sharing a directory are merged into one mount entry — selecting Codex, Antigravity and Cursor together produces a single `.agents` block (links and merge entries alike). An adapter may have several mount directories: Codex and Cursor each add one for their hooks.

## Four places for hooks

All four vendors implement hooks the same way (your script runs at checkpoints of the agent lifecycle); they differ only in where and how the registry is written. Three have a dedicated file, linked like `AGENTS.md`; only Claude Code keeps hooks inside a `settings.json` that also holds other settings, hence merge. Build translates per dialect: Claude / Codex share a shape, Antigravity wraps it in the hook name, Cursor renames events and flattens handlers. Details in the `hook.yaml` section of [Configuration](config.md).

## The shared .agents directory

`.agents/skills/` is an emerging cross-tool convention: Codex, Antigravity and Cursor all read it natively. agsy leans on this — one link serves three tools, and the skill form of every workflow is available to all of them.

Two per-tool caveats worth knowing:

- `disable-model-invocation: true` (set on every workflow's skill form) is honored by Claude Code and Cursor: only a human can trigger the procedure. Codex and Antigravity ignore the field — in those tools the model may decide to run a workflow on its own, so treat side-effect-heavy workflows accordingly.
- In Antigravity, `/name` runs the stub, which instructs the agent to load the corresponding skill. That is one extra indirection; in rare cases an agent may not follow it.

## Custom mounts

A tool not on the list gets its own hand-written mount entry:

```yaml
mount:
  - dir: .someothertool
    links:
      skills: skills
```

`init`'s edit mode preserves custom entries as is. A link may target each category's `to` value, `AGENTS.md` or a hook registry file; see [Configuration](config.md#mount-required-at-least-one) for the validation rules. Custom tools get no hook registry yet: the dialect table lives inside agsy (`internal/build/hooks.go`) and registries are written for the four built-in tools only.

## Adding a built-in adapter

Adapters live in the `adapters/` directory of the agsy source tree, one YAML file per tool:

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # set when the tool reads the root AGENTS.md
mounts:                    # several directories are fine
  - dir: .newtool
    links:
      skills: skills
```

Drop a file in, rebuild, and `init` offers the new tool.

→ Next chapter: [Scenario Guide](scenarios.md)
