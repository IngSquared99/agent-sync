# Adapters

An adapter is a built-in mount preset for one AI tool: which directory it reads, and which parts of the output it should see. `agsy init` turns the adapters you select into the `mount` section (and the `build.tools` list) of `agsy.yaml`. Adapters are factory templates, not a runtime dependency — the generated config is yours to edit.

## Built-in adapters

| Adapter | Mounts | Rules via | Notes |
|---------|--------|-----------|-------|
| `claude` (Claude Code) | `.claude/rules → rules`, `.claude/skills → skills` | per-file `.claude/rules/` | Claude Code does not read `AGENTS.md`; workflows arrive as skills, invoked with `/name` |
| `codex` (OpenAI Codex) | `.agents/skills → skills` | root `AGENTS.md` | Codex's only project-level locations are `AGENTS.md` and `.agents/skills/` |
| `antigravity` (Google Antigravity) | `.agents/skills → skills`, `.agents/workflows → workflows` | root `AGENTS.md` | `.agents/rules` is not mounted — Antigravity reads `AGENTS.md`, so a rules directory would duplicate every rule. `/name` invokes the stub, which loads the skill |
| `cursor` (Cursor) | `.agents/skills → skills` | root `AGENTS.md` | Cursor reads `.agents/skills/` natively, so it shares the `.agents` mount |

Selecting any of `codex`, `antigravity` or `cursor` makes init add the root mount:

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

Adapters sharing a directory are merged into one mount entry — selecting Codex, Antigravity and Cursor together produces a single `.agents` block.

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

`init`'s edit mode preserves custom entries as is. A link may target each category's `to` value or `AGENTS.md`; see [Configuration](config.md#mount-required-at-least-one) for the validation rules.

## Adding a built-in adapter

Adapters live in the `adapters/` directory of the agsy source tree, one YAML file per tool:

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # set when the tool reads the root AGENTS.md
mount:
  dir: .newtool
  links:
    skills: skills
```

Drop a file in, rebuild, and `init` offers the new tool.

→ Next chapter: [Scenario Guide](scenarios.md)
