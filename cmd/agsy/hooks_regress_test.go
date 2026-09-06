package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// Hooks category, end to end through the commands: a project path with
// shell metacharacters, idle merge targets, a renamed build.out or a moved
// project, override commands, orphaned merge targets, target notes.

// hookProjectAt builds a project (rules + one command hook) under sub inside
// a fresh temp dir, so the project path can carry spaces and metacharacters.
func hookProjectAt(t *testing.T, sub string) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), sub)
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md"), "# style\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), e2eHook)
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\necho RAN >> \"$AGSY_MARK\"\nexit 0\n")
	if err := os.Chmod(filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	return proj
}

func initAndApply(t *testing.T, proj string) {
	t.Helper()
	chdir(t, proj)
	prompt.AssumeYes = true
	t.Cleanup(func() { prompt.AssumeYes = false })
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
}

func firstCommand(t *testing.T, registry string) string {
	t.Helper()
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct{ Command string } `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(read(t, registry)), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Hooks["PreToolUse"][0].Hooks[0].Command
}

// The registry command runs through a shell: a project directory with a
// space or a pipe in its name still runs the script.
func TestHooksPathWithShellMetacharacters(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell check")
	}
	proj := hookProjectAt(t, "my proj | it's")
	initAndApply(t, proj)
	cmd := firstCommand(t, filepath.Join(proj, ".agsy", "hooks.codex.json"))
	if !strings.HasPrefix(cmd, "'") {
		t.Errorf("path with metacharacters must be quoted: %q", cmd)
	}
	mark := filepath.Join(t.TempDir(), "mark")
	c := exec.Command("sh", "-c", cmd)
	c.Env = append(os.Environ(), "AGSY_MARK="+mark)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("shell could not run the registry command %q: %v\n%s", cmd, err, out)
	}
	if _, err := os.Stat(mark); err != nil {
		t.Error("the hook script did not run")
	}
	// The quoted path is recognised as agsy's: a second apply replaces the
	// group, status is clean.
	if code := cmdApply(); code != 0 {
		t.Fatal("second apply failed")
	}
	s := read(t, filepath.Join(proj, ".claude", "settings.json"))
	if n := strings.Count(s, "block-rm.sh"); n != 1 {
		t.Errorf("settings.json holds the hook %d times, want 1:\n%s", n, s)
	}
	if code := cmdStatus(false); code != 0 {
		t.Error("status must be clean after two applies")
	}
}

// A plain path stays unquoted.
func TestHooksPlainPathUnquoted(t *testing.T) {
	proj := hookProjectAt(t, "plain")
	initAndApply(t, proj)
	cmd := firstCommand(t, filepath.Join(proj, ".agsy", "hooks.codex.json"))
	if strings.ContainsAny(cmd, `'"`) {
		t.Errorf("plain path must not be quoted: %q", cmd)
	}
}

// With no hooks, neither apply nor clean creates or rewrites the merge target.
func TestCleanLeavesIdleMergeTargetAlone(t *testing.T) {
	proj := newProject(t)
	chdir(t, proj)
	settings := filepath.Join(proj, ".claude", "settings.json")
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	if _, err := os.Stat(settings); err == nil {
		t.Fatal("apply must not create settings.json when there is nothing to merge")
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if raw, err := os.ReadFile(settings); err == nil {
		t.Errorf("clean created %s out of nothing: %q", settings, raw)
	}

	// Same with a user file that holds nothing of agsy's: byte for byte.
	orig := "{\n    \"permissions\": {\"allow\": [\"Bash(npm test)\"]},\n    \"hooks\": {\"PreToolUse\": []}\n}\n"
	write(t, settings, orig)
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	if got := read(t, settings); got != orig {
		t.Errorf("apply rewrote a file holding nothing of agsy's:\n%s", got)
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if got := read(t, settings); got != orig {
		t.Errorf("clean rewrote a file holding nothing of agsy's:\n%s", got)
	}
}

// After build.out is renamed the owner mark still identifies the previous
// groups; they are replaced, not kept.
func TestHooksOutRenameReplacesOldGroups(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	cfgPath := filepath.Join(proj, "agsy.yaml")
	write(t, cfgPath, strings.Replace(read(t, cfgPath), "out: .agsy ", "out: .build", 1))
	if code := cmdApply(); code != 0 {
		t.Fatal("apply after out rename failed")
	}
	s := read(t, filepath.Join(proj, ".claude", "settings.json"))
	if n := strings.Count(s, "block-rm.sh"); n != 1 {
		t.Errorf("settings.json holds the hook %d times after out rename, want 1:\n%s", n, s)
	}
	if !strings.Contains(s, filepath.Join(".build", "hooks")) {
		t.Errorf("settings.json must point into the new output:\n%s", s)
	}
}

// After the project is moved the old absolute paths are stale: status
// reports it, the owner mark still identifies the groups, apply rewrites.
func TestHooksMovedProjectReplacesOldGroups(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "before")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md"), "# style\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), e2eHook)
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
	initAndApply(t, proj)
	os.Chdir(root)
	moved := filepath.Join(root, "after")
	if err := os.Rename(proj, moved); err != nil {
		t.Fatal(err)
	}
	chdir(t, moved)
	if code := cmdStatus(false); code == 0 {
		t.Error("status must report the stale merge entries after the move")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply after move failed")
	}
	s := read(t, filepath.Join(moved, ".claude", "settings.json"))
	if n := strings.Count(s, "block-rm.sh"); n != 1 {
		t.Errorf("settings.json holds the hook %d times after the move, want 1:\n%s", n, s)
	}
	if strings.Contains(s, "before") {
		t.Errorf("stale path survived the move:\n%s", s)
	}
}

// A statusMessage set in hook.yaml is written verbatim; ownership then
// rests on the command path.
func TestHooksUserStatusMessageKept(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: block rm
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: ./block-rm.sh
          statusMessage: my own message
`)
	initAndApply(t, proj)
	s := read(t, filepath.Join(proj, ".claude", "settings.json"))
	if !strings.Contains(s, "my own message") || strings.Contains(s, "agsy:block-rm") {
		t.Errorf("user statusMessage must be preserved verbatim:\n%s", s)
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("second apply failed")
	}
	if n := strings.Count(read(t, filepath.Join(proj, ".claude", "settings.json")), "block-rm.sh"); n != 1 {
		t.Errorf("path-owned group must be replaced, not duplicated (%d copies)", n)
	}
}

// An override command pointing at a missing ./ script is refused like a
// main handler's.
func TestHooksOverrideMissingScriptRefused(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: x
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: ./block-rm.sh
      overrides:
        codex:
          hooks:
            - command: ./does-not-exist.sh
`)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code == 0 {
		t.Fatal("apply must refuse a registry that would point at a missing script")
	}
	if _, err := os.Stat(filepath.Join(proj, ".agsy", "hooks.codex.json")); err == nil {
		t.Error("nothing must be built")
	}
	if paths := build.HookScriptPaths(filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm")); len(paths) != 2 {
		t.Errorf("doctor's script list must include the override's command: %v", paths)
	}
}

// A merge target dropped from the mount config: status reports it as an
// orphan, apply keeps the record, clean strips it.
func TestMergeOrphanReportedAndCleaned(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	settings := filepath.Join(proj, ".claude", "settings.json")
	cfgPath := filepath.Join(proj, "agsy.yaml")
	raw := read(t, cfgPath)
	// drop the merge block but keep the .claude links
	i := strings.Index(raw, "    merge:")
	j := strings.Index(raw[i:], "\n  - dir:")
	write(t, cfgPath, raw[:i]+raw[i+j+1:])

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.MergeOrphans) != 1 || filepath.Clean(rep.MergeOrphans[0]) != filepath.Clean(settings) || !rep.HasGap {
		t.Fatalf("orphaned merge target must be reported: %v", rep.MergeOrphans)
	}
	if code := cmdStatus(false); code == 0 {
		t.Error("status must exit 1 with an orphaned merge target")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	if !strings.Contains(read(t, settings), "block-rm.sh") {
		t.Error("apply must not touch the orphaned target")
	}
	m, _ = build.LoadManifest(cfg.OutDir())
	if len(m.Merges) != 1 {
		t.Errorf("apply must keep the orphan on record so status keeps reporting it: %+v", m.Merges)
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if _, err := os.Stat(settings); err == nil {
		t.Errorf("clean must strip the orphaned entries and delete the file it created:\n%s", read(t, settings))
	}
}

// target: naming a tool without a hook dialect, or leaving tools out, is
// noted.
func TestHooksTargetNotes(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `target: [claude, gemini]
events:
  PreToolUse:
    - hooks: [{command: ./block-rm.sh}]
`)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	cfgPath := filepath.Join(proj, "agsy.yaml")
	write(t, cfgPath, strings.Replace(read(t, cfgPath), "tools: [", "tools: [gemini, ", 1))
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := build.ExpandSources(cfg)
	p, err := build.Compute(cfg, sources)
	if err != nil {
		t.Fatal(err)
	}
	var note string
	for _, it := range p.Items {
		if it.Category == "hooks" {
			note = it.RouteNote
		}
	}
	for _, want := range []string{`no hook registry format for "gemini"`, "target leaves out antigravity, codex, cursor"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q: %q", want, note)
		}
	}
}

// A copied project carries the original's merge record; that path lies
// outside the copy and is not an orphan of the copy.
func TestMergeOrphanIgnoresPathsOutsideProject(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "orig")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md"), "# style\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), e2eHook)
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
	initAndApply(t, proj)
	os.Chdir(root)
	copyDir := filepath.Join(root, "copy")
	if out, err := exec.Command("cp", "-R", proj, copyDir).CombinedOutput(); err != nil {
		t.Skipf("cp -R unavailable: %v %s", err, out)
	}
	chdir(t, copyDir)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.MergeOrphans) != 0 {
		t.Errorf("the original's settings.json is not an orphan of the copy: %v", rep.MergeOrphans)
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if !strings.Contains(read(t, filepath.Join(proj, ".claude", "settings.json")), "block-rm.sh") {
		t.Error("clean in the copy must not touch the original's settings.json")
	}
}
