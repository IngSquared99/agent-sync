# Adapters

## What is an adapter?

Adapters are **built-in mount presets, one per tool**: where the tool's config directory is, which names its rules / skills / workflows links should use, and its dedicated workflow bucket.

Key points:

- Adapters act **only during `agsy init`** — you pick tools, and init writes the matching `mount` entries and buckets into `agsy.yaml`.
- **They are no runtime dependency at all**: `plan` / `apply` / `status` read only `agsy.yaml`. After init you may edit the mount section freely; the presets impose nothing.

## Built-in adapters

> For a quick "which category mounts where" lookup, the [summary table in Core Concepts](overview.md#supported-tools-and-their-directories) is enough; below are the per-tool details.

### Claude Code (bucket: `claude`, directory `.claude/`)

| Link | Target in artifacts | Purpose |
|------|--------------------| --------|
| `.claude/rules` | `rules` | rule files |
| `.claude/skills` | `skills` | Agent Skills |
| `.claude/commands` | `workflows/claude` | slash commands — a workflow with `target: claude` appears in Claude Code as `/<filename>` |

All three categories are mounted; the most complete adapter.

### OpenAI Codex (bucket: `codex`, directory `.codex/`)

| Link | Target in artifacts | Purpose |
|------|--------------------| --------|
| `.codex/prompts` | `workflows/codex` | custom prompts |

**Workflows (prompts) only**, following the Codex CLI custom-prompts convention (the source notes that project-level support still needs real-world verification). With **only** Codex selected, init therefore warns: rules and skills would be built but nothing would read them — add links yourself if needed.

### Antigravity (bucket: `agents`, directory `.agents/`)

| Link | Target in artifacts |
|------|--------------------|
| `.agents/rules` | `rules` |
| `.agents/skills` | `skills` |
| `.agents/workflows` | `workflows/agents` |

## Buckets and adapters

Each adapter declares one bucket. During init, `route.buckets` = the union of the selected adapters' buckets (plus hand-added ones, plus buckets used by custom mounts). Workflows name their tools with `target:`; untargeted ones follow `route.default`.

## Custom mounts (connecting a tool not on the list)

Any tool that reads Markdown from a fixed directory can be connected by hand — add a mount entry to `agsy.yaml`:

```yaml
build:
  route:
    buckets: [claude, codex, mytool]   # add the dedicated bucket here
    default: [claude, codex, mytool]

mount:
  - dir: .mytool
    links:
      rules:   rules                # shared rules
      prompts: workflows/mytool     # dedicated workflow bucket
```

Rules recap (details in the [Configuration chapter](config.md)): a link target's first level must be some category's `to` value; only workflows take a second bucket level (which must exist in `route.buckets`); `dir` must be unique and must not sit inside the artifacts or a source.

**Custom entries survive init**: rerunning `agsy init` later keeps non-built-in mount entries and custom buckets verbatim, labeled "custom mount" on screen.

## (Advanced) adding a built-in adapter to the agsy project

For contributors: built-in adapters are the YAML files under `adapters/` in the repository, embedded via `go:embed`. Adding a tool is one file:

```yaml
# adapters/mytool.yaml
name: mytool          # internal id
display: My Tool      # label in the init menu
bucket: mytool        # dedicated workflow bucket
mount:
  dir: .mytool
  links:
    rules:   rules
    prompts: workflows/mytool
```

After recompiling, the tool appears in init's multi-select list (sorted by name).

→ Next chapter: [Scenario Guide](scenarios.md)
