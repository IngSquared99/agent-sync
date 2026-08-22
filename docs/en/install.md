# Installation

Pick the method for your operating system — each is a single command:

| Method | Platform | Command | Prerequisite |
|--------|----------|---------|--------------|
| Method 1: Homebrew | macOS | `brew install ingsquared99/tap/agsy` | Homebrew installed |
| Method 2: winget | Windows 10 / 11 | `winget install IngSquared99.agsy` | none — built into Windows |
| Method 3: Go from source | all platforms (use this on Linux) | `go install …` (see below) | Go installed |

**Security notes**: methods 1 and 2 install prebuilt binaries from GitHub Releases — compiled from the public source by a public CI pipeline, with the SHA-256 checksum of every file pinned in the Homebrew cask and winget manifest, so downloads are verifiable and auditable. Method 3 compiles the source directly on your machine and involves no prebuilt binary at all. agsy itself has **zero third-party dependencies** (standard library only).

## Method 1: Homebrew (macOS)

```sh
brew install ingsquared99/tap/agsy
```

- brew downloads the binary matching your machine (Apple Silicon / Intel) from GitHub Releases and verifies it.
- The install handles the macOS quarantine attribute, so the first run does **not** trigger the "unverified developer" warning.
- No Homebrew yet? Follow the instructions at <https://brew.sh> (the standard package manager for macOS developers — install once, use forever).

## Method 2: winget (Windows)

```powershell
winget install IngSquared99.agsy
```

- winget is the official package manager **built into** Windows 10 / 11 — nothing to install first; open a terminal (PowerShell or cmd) and run it.
- Open a **new** terminal window afterwards, then run `agsy version` to confirm.

## Method 3: build from source (all platforms; use this on Linux)

Requires **Go 1.22 or newer** (latest stable recommended). No Go yet: macOS `brew install go`, Windows `winget install GoLang.Go`, Linux via your distribution's packages (e.g. `apt install golang-go`) or <https://go.dev/dl/>.

**Quick version** — one command; the Go toolchain fetches the source, compiles locally, and installs into `~/go/bin/`:

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

If the terminal cannot find `agsy` afterwards, `~/go/bin` is not on PATH (PATH = the list of directories the terminal searches for commands):

```sh
# macOS (default zsh): add to the shell config, then open a new terminal
# Linux (bash): same line into ~/.bashrc instead
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

**Full version** — for reading the code first or modifying it (also requires Git):

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go test ./...                # (optional) run the test suite
go build -o agsy ./cmd/agsy  # produces the agsy binary (agsy.exe on Windows)
mv agsy ~/go/bin/            # put it in any directory on PATH
```

There are no other framework or library requirements — no npm, no pip; `go build` is all of it.

## Verify the installation

```sh
agsy version
# e.g. agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

Version info printed = installed. You can then run a read-only environment health check in any project:

```sh
agsy doctor
```

## Interface language: how Chinese / English is decided

agsy ships with Traditional Chinese and English interfaces and **picks one automatically**. At startup it checks three environment variables in order (environment variables = OS-level settings visible to every terminal program) and uses the first one that has a value:

```
 AGSY_LANG set?  ──yes──▶ use it
     │ no
     ▼
 LC_ALL set?    ──yes──▶ use it
     │ no
     ▼
 LANG set?      ──yes──▶ use it
     │ no
     ▼
   English
```

There is exactly one rule: **a value starting with `zh` (e.g. `zh_TW.UTF-8`, `zh-TW`) → Traditional Chinese; anything else → English.**

The division of labor:

- `LC_ALL`, `LANG`: the **operating system's own** locale settings, not something agsy invented. On a Chinese-locale system they are typically already `zh_TW.UTF-8`, so agsy starts in Chinese with zero setup.
- `AGSY_LANG`: **agsy's dedicated switch** with the highest priority, for overriding the system locale (e.g. an English system where you want the Chinese interface).

To set the language manually:

```sh
export AGSY_LANG=zh-TW    # force Chinese in this terminal window
export AGSY_LANG=en       # force English
```

`export` affects only the current terminal window; to make it permanent, add the line to your shell config (macOS default zsh → `~/.zshrc`) and open a new terminal.

## Upgrade and uninstall

| | Method 1 Homebrew | Method 2 winget | Method 3 Go |
|---|---|---|---|
| Upgrade | `brew upgrade agsy` | `winget upgrade IngSquared99.agsy` | rerun `go install …@latest` |
| Remove binary | `brew uninstall agsy` | `winget uninstall IngSquared99.agsy` | delete `~/go/bin/agsy` |

Before uninstalling, run `agsy clean` in every project that used agsy (removes mount links and the `.agsy/` artifacts; `agsy.yaml` is kept — delete it manually if unwanted).

→ Next chapter: [Quick Start](quickstart.md)
