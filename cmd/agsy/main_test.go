package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// End-to-end tests for the CLI layer: call the cmd functions directly and
// verify exit codes and the actual filesystem results. This layer had no
// tests at all before, yet flag parsing, confirmation flows, and command
// ordering all live here.

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// chdir switches to dir and restores the old cwd when the test ends
// (Go 1.22 has no t.Chdir).
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

// newProject prepares a project with source material but no agsy.yaml yet.
func newProject(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	write(t, filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md"), "# 風格\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "skills", "api-doc", "SKILL.md"), "---\nname: api-doc\n---\n內文\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "workflows", "release-note.md"), "沒標 target\n")
	return proj
}

// init's guard rails when non-interactive (go test's stdin is not a terminal)
// and the --yes authorization (§12-19).
func TestInitThenApplyThenClean(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)

	// Non-interactive without --yes: init must always cancel instead of
	// silently writing a config file with defaults applied.
	if code := cmdInit(nil); code == 0 {
		t.Fatal("non-interactive init without --yes should not succeed")
	}
	if _, err := os.Stat(filepath.Join(proj, "agsy.yaml")); err == nil {
		t.Fatal("a cancelled init should not write agsy.yaml")
	}

	// --yes = explicit authorization to adopt the suggested defaults; only then is the config produced.
	prompt.AssumeYes = true
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatalf("init --yes exit code = %d", code)
	}
	prompt.AssumeYes = false
	raw, err := os.ReadFile(filepath.Join(proj, "agsy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"sources:", "on_conflict:", "buckets:", "mount:"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("generated config is missing %q", want)
		}
	}

	// Edit mode: every question keeps its current value → content unchanged → no write, no noise.
	prompt.AssumeYes = true
	if code := cmdInit(nil); code != 0 {
		t.Errorf("edit mode with no content change should exit quietly, got %d", code)
	}
	prompt.AssumeYes = false

	// Edit mode: content really differs (hand-written comments would be
	// replaced by template comments) → confirmation required; non-interactive
	// without --yes → cancel, and not a single byte of the original may change.
	withComment := string(raw) + "# 我的手寫註解\n"
	write(t, filepath.Join(proj, "agsy.yaml"), withComment)
	if code := cmdInit(nil); code == 0 {
		t.Error("init must not overwrite when there are changes and nobody can confirm")
	}
	after, err := os.ReadFile(filepath.Join(proj, "agsy.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != withComment {
		t.Error("a cancelled init modified the original file")
	}
	write(t, filepath.Join(proj, "agsy.yaml"), string(raw))

	if code := cmdPlan(); code != 0 {
		t.Fatalf("plan exit code = %d", code)
	}
	// plan must not write anything.
	if _, err := os.Stat(filepath.Join(proj, ".agsy")); err == nil {
		t.Error("plan should not create .agsy/")
	}

	if code := cmdApply(); code != 0 {
		t.Fatalf("apply exit code = %d", code)
	}
	for _, p := range []string{".agsy/rules/python-style.md", ".agsy/skills/api-doc/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(proj, filepath.FromSlash(p))); err != nil {
			t.Errorf("missing output %s", p)
		}
	}

	// Right after apply: status is all green with exit code 0 (usable directly as a CI gate).
	if code := cmdStatus(false); code != 0 {
		t.Errorf("status right after apply should be 0, got %d", code)
	}

	// clean requires confirmation; non-interactive without --yes should cancel.
	if code := cmdClean(); code == 0 {
		t.Error("non-interactive clean without --yes should not run")
	}
	if _, err := os.Stat(filepath.Join(proj, ".agsy")); err != nil {
		t.Fatal("a cancelled clean deleted the outputs")
	}

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdClean(); code != 0 {
		t.Fatalf("clean --yes exit code = %d", code)
	}
	if _, err := os.Stat(filepath.Join(proj, ".agsy")); err == nil {
		t.Error("outputs should be removed after clean")
	}
	if _, err := os.Stat(filepath.Join(proj, "agsy.yaml")); err != nil {
		t.Error("clean should not touch agsy.yaml")
	}
}

// Output modified → status reports a local change with exit code 1 → promote
// writes it back → status returns to zero.
func TestStatusPromoteCycle(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()

	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}

	// Simulate an AI editing the output through the mount: release-note has
	// no target, so each of the two buckets holds a copy. Deliberately modify
	// the second copy — updating only the first copy's hash would make status
	// report the same thing forever.
	claudeCopy := filepath.Join(proj, ".claude", "commands", "release-note.md")
	if _, err := os.Stat(claudeCopy); err != nil {
		t.Skipf("no usable directory link on this platform, skipping: %v", err)
	}
	write(t, claudeCopy, "沒標 target\n## AI 補的段落\n")

	if code := cmdStatus(false); code != 1 {
		t.Errorf("status with a local change should be 1, got %d", code)
	}
	if code := cmdPromote([]string{"workflows/release-note.md"}); code != 0 {
		t.Fatalf("promote exit code = %d", code)
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "workflows", "release-note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "AI 補的段落") {
		t.Error("the change was not written back to the source")
	}
	// After write-back it must not be reported as a local change again
	// (only a "lag" remains, reminding you to apply).
	report := captureLocals(t)
	if report != 0 {
		t.Errorf("still %d local changes after promote", report)
	}
}

// captureLocals reruns detection and returns the number of local changes.
func captureLocals(t *testing.T) int {
	t.Helper()
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadManifestForTest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := collectForTest(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRunParsesGlobalYesFlag(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	defer func() { prompt.AssumeYes = false }()

	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"agsy", "version"}
	if code := run(); code != 0 {
		t.Errorf("version exit code = %d", code)
	}
	if prompt.AssumeYes {
		t.Error("AssumeYes should not be set without --yes")
	}

	os.Args = []string{"agsy", "--yes", "version"}
	if code := run(); code != 0 {
		t.Errorf("version exit code = %d", code)
	}
	if !prompt.AssumeYes {
		t.Error("--yes should be parsed as a global flag")
	}

	os.Args = []string{"agsy", "沒有這個指令"}
	if code := run(); code != 2 {
		t.Errorf("unknown command exit code = %d, want 2", code)
	}
}
