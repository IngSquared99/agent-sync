# Security Policy

## Supported versions

Only the latest release receives security fixes. agsy has no external module
dependencies (the YAML parser is a vendored copy of go-yaml; everything else is
the Go standard library), so upgrading is low-risk — please stay current.

## Reporting a vulnerability

Please report vulnerabilities privately via
[GitHub Security Advisories](https://github.com/IngSquared99/agent-sync/security/advisories/new)
("Report a vulnerability" on the repository's Security tab). Do not open a
public issue for an unpatched vulnerability. You should receive a response
within a week.

## Threat model (what agsy defends against)

These properties are deliberate and their regression counts as a
vulnerability:

- **Sources are only half trusted.** Symbolic links are never collected or
  copied (a link inside a shared library must not smuggle files from outside
  it — e.g. a private key — into the mounted output), and the copy phase
  re-checks with `O_NOFOLLOW` on Unix. Irregular files (FIFOs, sockets,
  devices) are refused at scan time and again at every open (`O_NONBLOCK`
  plus an fstat check) — a named pipe planted in a source must not be able
  to hang a build.
- **The manifest is untrusted.** It lives in the AI-writable output; output
  paths that escape the out directory, relative source paths, and unverified
  mount records are all rejected. Consumers only remove a recorded link after
  verifying it really is a link into the output.
- **Destructive paths are validated.** `build.out` (wiped by apply, removed
  by clean) must be a dedicated directory inside the project, never the
  project root, a source, or the home directory — checked against resolved
  (symlink-free) locations.
- **Mounts outside the project require an explicit opt-in**
  (`outside_project: true`), so an `agsy.yaml` in a cloned repository cannot
  silently point global tool configuration (e.g. `~/.claude`) at repository
  content.
- **Merge touches one key of one file, never through a link.** The only
  user-owned file agsy writes is a merge target (Claude Code's
  `.claude/settings.json`): it rewrites the `hooks` key's entries that carry
  the `agsy:` status mark or whose command points into the output, and
  preserves everything else; a target that is a symbolic link or not a JSON
  object is refused. Merge targets must lie inside the project —
  `outside_project: true` does not extend to merge — and recorded merge
  targets outside the project are never opened.
- **Hook registries carry absolute paths into the output only.** A `command`
  in a registry is rewritten from a `./` path inside the hook directory and
  must exist there at build time; nothing outside the output is referenced.
- **No write-back.** Everything a tool reads through a mount is a rebuildable
  artifact; keeping an artifact-side change always requires a human moving it
  into a source.
