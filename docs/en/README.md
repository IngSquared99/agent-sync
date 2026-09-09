# agent-sync (agsy) Documentation

The agsy documentation, ordered as "concepts → install → quick start → deep dives → troubleshooting". Browse chapters via the sidebar on the left; the search box at the top right covers the full text. When reading on GitHub, use the file links in the table below.

## Contents

| Chapter | Covers |
|---------|--------|
| [Core Concepts](overview.md) | the problem it solves, the terms, the four categories, what each tool reads |
| [Installation](install.md) | Homebrew / from source, interface language, upgrade and removal |
| [Quick Start](quickstart.md) | first sync in four steps, command cheat sheet |
| [Configuration](config.md) | agsy.yaml field by field, hook.yaml, merge, error quick reference |
| [Command Reference](commands.md) | command overview plus per-command details |
| [Adapters](adapters.md) | built-in adapters (Claude Code / Codex / Antigravity / Cursor) and custom mounts |
| [Scenario Guide](scenarios.md) | what status shows and apply does when the two sides differ |
| [FAQ](faq.md) | common questions from the user's point of view |

## Suggested reading paths

- **First time**: Core Concepts → Installation → Quick Start; run `init → plan → apply` once, then return for Configuration and the Command Reference.
- **Understanding every config line**: Configuration.
- **A command behaved unexpectedly**: the matching Command Reference section, the Scenario Guide, and the FAQ.
- **Connecting a new AI tool**: Adapters.
