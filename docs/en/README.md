# agent-sync (agsy) Documentation

The complete guide to agsy, ordered as "install → quick start → deep dives → troubleshooting".
Browse chapters via the sidebar on the left; the search box at the top-right covers the full text. When reading on GitHub, use the file links in the table below.

## Contents

| Chapter | Covers |
|---------|--------|
| [Core Concepts](overview.md) | what agsy is, the problem it solves, the three layers and data flow |
| [Installation](install.md) | Homebrew / winget / from-source, interface language |
| [Quick Start](quickstart.md) | first sync in four steps, command cheat sheet, CLI walkthroughs |
| [Configuration](config.md) | every agsy.yaml field, with the safety rules |
| [Command Reference](commands.md) | command overview plus per-command details and situations |
| [Adapters](adapters.md) | built-in adapters (Claude Code / Codex / Antigravity) and custom mounts |
| [Scenario Guide](scenarios.md) | apply / promote behavior under every combination of changes |
| [FAQ](faq.md) | common questions from the user's point of view |

## Suggested reading paths

- **First time**: Core Concepts → Installation → Quick Start; run `init → plan → apply` once, then return for Configuration and the Command Reference.
- **Understanding every config line**: Configuration.
- **A command behaved unexpectedly**: the matching Command Reference section, the Scenario Guide, and the FAQ.
- **Connecting a new AI tool**: Adapters.
