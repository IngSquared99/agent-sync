# Installation

Pick one method for your operating system. Confirm with `agsy version` afterwards.

## 1. Choose a method

| Your system | Use | Needs |
|-------------|-----|-------|
| macOS | Method 1: Homebrew | Homebrew installed |
| Windows 10 / 11, Linux, or building yourself | Method 2: Go source | Go 1.22 or newer |

Method 1 downloads the prebuilt binary from GitHub Releases, compiled by a public CI run from public source; the install definition pins each file's SHA-256 checksum. Method 2 compiles the source on your own machine; there is no winget package for Windows yet, use Method 2 there. agsy has no external dependencies.

## 2. Install

### Method 1: Homebrew (macOS)

```sh
brew install ingsquared99/tap/agsy
```

The matching Apple Silicon or Intel build is picked automatically. The first run does not show an "unverified developer" warning. Without Homebrew: install it per <https://brew.sh>.

### Method 2: from source (any platform)

Without Go: macOS `brew install go`, Windows `winget install GoLang.Go`, Linux via your distribution (`apt install golang-go`, for example) or <https://go.dev/dl/>.

One line:

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

The binary lands in `~/go/bin/` (`%USERPROFILE%\go\bin\` on Windows). If the terminal cannot find `agsy` afterwards, that folder is not on PATH (the list of folders the terminal searches for commands):

```sh
# macOS (zsh): add to the shell config, then open a new terminal; Linux (bash): ~/.bashrc
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

The Go installer for Windows usually adds that folder to PATH already; if not, add `%USERPROFILE%\go\bin` to the user Path under "Edit the system environment variables", then **open a new terminal window**.

To read or modify the code first:

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go test ./...                # optional: run the tests
go build -o agsy ./cmd/agsy  # produces agsy (agsy.exe on Windows)
mv agsy ~/go/bin/            # any folder on PATH
```

## 3. Confirm

```sh
agsy version
# e.g. agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

A version line means it is installed. A read-only health check can follow:

```sh
agsy doctor
```

## 4. Interface language

agsy has a Traditional Chinese and an English interface, chosen automatically. The order of checks:

```
 AGSY_LANG set? ──yes──▶ use it
     │ no
 LC_ALL set?    ──yes──▶ use it
     │ no
 LANG set?      ──yes──▶ use it
     │ no
   English
```

One rule: a value starting with `zh` (such as `zh_TW.UTF-8`) → Traditional Chinese; anything else → English.

- `LC_ALL` and `LANG` are the operating system's own language settings.
- `AGSY_LANG` is agsy's own switch, with the highest priority, for overriding the system setting.

To set it by hand:

```sh
export AGSY_LANG=zh-TW    # Chinese in this terminal window
export AGSY_LANG=en       # English
```

`export` applies to the current terminal window. To make it permanent, add the line to the shell config (`~/.zshrc` on macOS).

## 5. Upgrade and remove

| | Homebrew | Go |
|---|---|---|
| Upgrade | `brew upgrade agsy` | rerun `go install …@latest` |
| Remove | `brew uninstall agsy` | delete `~/go/bin/agsy` |

Before removing, run `agsy clean` in every project that used agsy (it removes the links and `.agsy/`; `agsy.yaml` stays, delete it yourself if unwanted).

→ Next: [Quick Start](quickstart.md)
