# agent-sync (agsy)

**English** | [繁體中文](README.zh-TW.md)

**A single-source sync tool for AI instruction files** — merge your rules / skills / workflows scattered across multiple places into one build output, then mount it where Claude Code, Antigravity, and other AI coding tools expect to find it. **Edit one source, and every tool stays in sync.**

> Naming cheat sheet: the project is **agent-sync**, the command is **`agsy`**, the config file is **`agsy.yaml`**, and the build output lives in **`.agsy/`**.

---

## What is this? What problem does it solve?

If you use more than one AI coding tool, you know this pain: every tool dictates its own read location —

- Claude Code reads `.claude/` (rules, skills, commands)
- Antigravity reads `.agents/` (rules, skills, workflows)
- And you probably keep a personal instruction library shared across projects (say, `~/all-ai-lib`)

The same "Python style guide" has to be copied into several places. One edit means N updates. They *will* drift apart.

**How agsy works**: you keep instruction files in "sources" (a shared library, a directory inside the project — as many as you like), and agsy does two things:

```
   Sources (originals)         build (copy & merge)           mount (directory links)

  ~/all-ai-lib/  ──┐                                        .claude/rules   ──→ .agsy/rules
               ├── merge ──→  .agsy/ (single output) ←── link ──  .claude/skills  ──→ .agsy/skills
  ./repo-ai-lib/  ──┘                                         .agents/rules   ──→ .agsy/rules
                                                        .agents/workflows … and so on
```

- **build**: **copies** and merges all sources into a single `.agsy/` output (sources are never touched directly)
- **mount**: creates **directory links** in `.claude/`, `.agents/` pointing at the output (junctions on Windows — no system settings required)

---

## Quick start (3 steps)

1. Install `agsy`: open a terminal and paste one line (see [Install](#install) below)
2. `cd` into your project
3. Type `agsy` and follow the prompts

```
$ cd my-project
$ agsy

⚠ agsy.yaml not found — looks like the first run in this project
? Create the config now?
❯ Yes, start setup (init)
```

### The daily loop (one picture)

```
              apply (rebuild output from sources & mount)
        ┌────────────────────────────────────────────────┐
        │                                                ▼
   Sources (originals)                        .agsy/ (output) ◄─── .claude/ .agents/ (links)
        ▲                                                │            AI tools read here —
        │                                                │            and sometimes edit here
        └────────────────────────────────────────────────┘
              promote (write output-side edits back to sources)
```

Day to day, there are only three moves:

| Situation | Command |
|---|---|
| I edited a source | `agsy apply` |
| An AI tool edited a mounted file | `agsy promote` to write it back, then `agsy apply` |
| Not sure what state things are in | `agsy status` |

---

## Commands

| Command | What it does |
|---|---|
| `agsy` | Interactive menu (when run with no arguments) |
| `agsy init` | Q&A-style generation of `agsy.yaml`; if one exists, enters **edit mode** (current values as defaults, Enter to keep, shows a change diff before writing) |
| `agsy doctor` | Environment health check — inspects only, changes nothing |
| `agsy plan` | Previews what build / mount would do, **writes nothing** |
| `agsy apply` | Rebuilds the output and mounts it (checks for un-promoted local edits first) |
| `agsy status` | Compares sources / output / mounts and reports drift (CI-friendly: exit 0 = in sync) |
| `agsy promote` | **Writes back** edits from the output to their sources (for when an AI tool edited a mounted file) |
| `agsy clean` | Uninstall: removes links and output, leaving only `agsy.yaml` |
| `agsy version` | Version, commit, build time, platform |
| `agsy help` | Command help |

Global flag `--yes` / `-y`: treats every confirmation as "y" — for CI, git hooks, and scripts. Without it, whenever nobody can answer a prompt (non-interactive environment), any action that needs confirmation is cancelled rather than forced.

Commands can be run from any subdirectory of the project; agsy walks upward to find `agsy.yaml` (same convention as git). The only exception is `agsy init` — it creates the config right where you run it.

---

## Supported tools (adapters)

`adapters/` ships mount presets for each tool; they populate the choices when `agsy init` asks "which tools should be mounted?":

| Tool | Mount directory | Linked content |
|---|---|---|
| Claude Code | `.claude/` | rules, skills, commands |
| Antigravity | `.agents/` | rules, skills, workflows |
| OpenAI Codex | `.codex/` | prompts (mount convention still being verified) |

**Want a tool that's not on the list?** Adapters are just factory templates for init, not a runtime dependency — add a `mount` entry to `agsy.yaml` yourself. **No code changes, no waiting for a release**:

```yaml
mount:
  - dir: .sometool      # the directory that tool reads from
    links:
      rules: rules      # tool's subdirectory → layer in the build output
```

## What the config looks like

`agsy init` generates a commented `agsy.yaml`; afterwards just edit it directly (run `agsy apply` to take effect):

```yaml
version: 1

sources:            # ordered array — earlier wins
  - ~/all-ai-lib        # shared library across projects
  - ./repo-ai-lib         # in-project source

build:
  out: .agsy
  on_conflict:      # same-name handling: first / rename / error (required per category)
    rules:     rename
    skills:    error
    workflows: rename
  route:            # workflows are routed by the front-matter `target` field
    field: target
    default: [agents, claude]
    buckets: [agents, claude]

mount:
  - dir: .claude    # Claude Code
    links:
      rules:    rules
      skills:   skills
      commands: workflows/claude
  - dir: .agents    # Antigravity
    links:
      rules:     rules
      skills:    skills
      workflows: workflows/agents
```

## What gets collected

| Category | Source subdirectory | Unit |
|---|---|---|
| rules | `rule/` | single `.md` file |
| skills | `skill/` | directory (containing `SKILL.md`) |
| workflows | `workflow/` | single `.md` file |

Anything that doesn't match is skipped — but **never silently**: `agsy plan` and `agsy doctor` list every skipped file and the reason.

---

## Why this isn't just another copy script

Most tools in this space are a single-layer flow: "one source directory → copy or link into each tool's location." agsy takes a different path in three places:

### Feature 1: True multi-source merging, with conflicts decided by you

The usual approach allows one source directory, or at best a global/project "fallback" where one shadows the other. agsy's `sources` is an **ordered array** — a shared library, a team library, and a project library can all be **stacked and merged**, with order as priority. Name collisions are handled by the strategy you choose:

```
  ~/all-all-ai-lib/rule/python-style.md ──┐               rename strategy (keep both, tag origins)
                                  ├── collision! ──→  python-style@all-ai-lib.md
  ./repo-all-ai-lib/rule/python-style.md ───┘                   python-style@repo-ai-lib.md

                                                  first: keep only the higher-priority copy
                                                  error: stop and list the conflicts for you
```

Strategies are set per category (rules / skills / workflows independently) and are **required** — the consequences of a collision differ per project, so agsy never decides for you silently.

### Feature 2: An intermediate build layer — sources stay clean, forever

Single-layer syncing has a hidden hazard: when an AI tool edits a file at the mount location, the edit punches straight through into your originals — contaminating every project that shares them. agsy deliberately adds one layer in between:

```
   Sources (originals) ────────── never touched directly by any tool
      │  build = copy
      ▼
   .agsy/ (a disposable sandbox) ◄──── AI edits land here, and stop here
      │  mount = link                     │
      ▼                                   │ want to keep it → promote back to source
   .claude/  .agents/                     │ don't → the next apply rebuilds it away
```

The properties this layer buys are all connected: because it exists, "sources stay clean" is a structural guarantee rather than a convention; routing (Feature 3) happens entirely at build time, so the mount layer stays as simple as "one link per directory"; and since the output is always rebuildable, `clean` gives you a complete, safe uninstall.

### Feature 3: Per-file routing — this file goes only to that tool

Not every workflow belongs in every tool. Tag a `target` in the file's front-matter and build routes it automatically:

```markdown
---
target: [claude]
---
```

```
  deploy.md        (target: claude)  ──→  only .claude/commands/
  standup.md       (target: agents)  ──→  only .agents/workflows/
  hotfix.md        (target: both)    ──→  both
  release-note.md  (untagged)        ──→  your configured default (factory: everywhere)
```

No "copy everything to every tool" waste, and no files quietly vanishing — `agsy plan` shows the routing destination for every file, and even annotates "placed on both sides because the default applied."

### Table stakes: two-way sync and drift detection

Every file is recorded in a manifest at build time (sha256 fingerprint + source lineage), so `agsy status` can separate drift into its **two directions** at a glance:

```
  Sources changed (time to apply)        Output changed (time to promote)
  ├─ behind: a source was updated        ├─ local edits: an AI tool edited a mounted
  ├─ new: a source has new files         │   file — with its original home identified
  └─ source deleted (flagged loudly)     └─ directory items list exactly which files changed
```

`promote` writes things back **to where they came from** (renamed files automatically regain their original names); in non-interactive environments `status` reports via exit code (0 = in sync), ready for CI or a git hook.

### Safety baseline

- **The tool never deletes anything it didn't create** — links are agsy's own, freely rebuilt; your real directories and files trigger an error and are never removed on your behalf
- `build.out` is guarded: pointing it outside the project, at your home directory, or at a source path is rejected at config-validation time
- `apply` always checks for un-promoted edits first — nothing is silently overwritten
- `plan` never writes anything — look before you leap

### Download and run

A single executable with **zero external dependencies** — no runtime to install. On Windows, mounting uses junctions: **no Developer Mode, no administrator rights**. macOS / Linux / Windows (x64 and arm64) all supported.

---

## FAQ

**Q: `.claude/` already contains my own files — will they be deleted?**
No. agsy only deletes links it created itself; a real directory or file always triggers an error asking you to handle it manually. Never deleted on your behalf.

**Q: Does Windows need Developer Mode or admin rights?**
No. agsy mounts with junctions on Windows — a regular account is enough.

**Q: Links broke after moving the project to another path?**
Just rerun `agsy apply` (Windows junctions store absolute paths and need rebuilding after a move).

**Q: Should `.agsy/` go into version control?**
No — it's a rebuildable artifact. `agsy init` offers to add it to `.gitignore`; `agsy.yaml` on the other hand *should* be committed — teammates clone and run `agsy apply` to grow identical mounts.

**Q: An AI tool edited a mounted file directly — is my source contaminated?**
No. The edit actually lands in the `.agsy/` output layer; sources are unaffected. Keep it with `agsy promote`, or let the next `agsy apply` rebuild it away.

**Q: What about same-name files (both the shared library and the project have `python-style.md`)?**
You decide the strategy at init: keep both with origin tags (rename), keep only the higher-priority one (first), or stop with an error for manual handling (error). For skills we recommend `error` — skills trigger on their `description` semantics, and two similar ones coexisting makes triggering unpredictable.

**Q: I downloaded the executable from the web page myself and Mac says it "cannot verify the developer"?**
Browser downloads get quarantined by macOS (command-line downloads don't). Click "Done" (not "Move to Trash"), then run `xattr -d com.apple.quarantine <path>` to clear it; this is the normal consequence of an unpaid signature, not malware.

**Q: Can the interface show Chinese?**
It follows your system language automatically (a Chinese terminal `LANG` shows Traditional Chinese, everything else shows English); to force a language, prefix commands with `AGSY_LANG=en` or `AGSY_LANG=zh`.

**Q: `status` says mounts are fine, but the tool still doesn't see my instructions?**
First check that `.claude/` (etc.) isn't a real directory you created by hand. agsy verifies each link both "is a link" and "points to the right place"; anything wrong is flagged ✘ and `agsy apply` repairs it.

---

## Install

### One-line install (recommended)

**Mac / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
```

The script detects your platform, downloads the matching build from [Releases](https://github.com/IngSquared99/agent-sync/releases), and puts `agsy` on your PATH — it is a few dozen readable lines, feel free to inspect it first: [install.sh](install.sh) / [install.ps1](install.ps1).

### Homebrew (Mac)

```bash
brew install IngSquared99/tap/agsy
```

### Go (developers)

```bash
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

Run `agsy version` to confirm the install.

<details>
<summary>Manual download (no script)</summary>

```bash
# Mac (Apple silicon, M-series)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_apple_silicon.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Mac (Intel)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_intel.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Linux (x64; on Arm machines replace x64 with arm64)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_linux_x64.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

```powershell
# Windows (PowerShell)
iwr https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_windows_x64.zip -OutFile "$env:TEMP\agsy.zip"; Expand-Archive "$env:TEMP\agsy.zip" "$env:LOCALAPPDATA\Programs\agsy" -Force; [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";$env:LOCALAPPDATA\Programs\agsy", "User")
# Open a new terminal window afterwards so PATH takes effect; on Arm laptops replace x64 with arm64
```

Mac / Linux will ask for your password once (moving into /usr/local/bin requires it); command-line downloads don't get macOS's quarantine flag, so the "cannot verify the developer" warning never appears.

</details>

### Build from source (developers)

Requires [Go](https://go.dev/dl/) 1.22+:

```
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go build -o agsy ./cmd/agsy    # Windows: go build -o agsy.exe ./cmd/agsy
go test ./...             # zero external dependencies — no go mod download needed
```