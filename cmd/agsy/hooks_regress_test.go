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
	"github.com/IngSquared99/agent-sync/internal/mount"
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
	if !strings.Contains(s, "agsy:block-rm · my own message") {
		t.Errorf("user statusMessage must follow the owner mark:\n%s", s)
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

// A project whose first apply had no hooks must still merge the hooks added
// later: "nothing to merge" is judged by the current registry, not by the
// record of the previous (empty) write. Both a missing settings.json and an
// existing one without agsy groups are covered.
func TestHooksAddedAfterIdleApplyReachSettings(t *testing.T) {
	for _, preexisting := range []bool{false, true} {
		proj := newProject(t)
		chdir(t, proj)
		settings := filepath.Join(proj, ".claude", "settings.json")
		if preexisting {
			write(t, settings, "{\"permissions\": {}}\n")
		}
		prompt.AssumeYes = true
		t.Cleanup(func() { prompt.AssumeYes = false })
		if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
			t.Fatal("init failed")
		}
		if code := cmdApply(); code != 0 {
			t.Fatal("first apply failed")
		}
		if !preexisting {
			if _, err := os.Stat(settings); err == nil {
				t.Fatal("no hooks: settings.json must not be created")
			}
		}
		write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), e2eHook)
		write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
		if code := cmdApply(); code != 0 {
			t.Fatal("second apply failed")
		}
		if !strings.Contains(read(t, settings), "block-rm.sh") {
			t.Errorf("preexisting=%v: hook added after an idle apply did not reach settings.json:\n%s", preexisting, read(t, settings))
		}
		// and removing it again empties the file back out
		os.RemoveAll(filepath.Join(proj, "repo-ai-lib", "hooks"))
		if code := cmdApply(); code != 0 {
			t.Fatal("third apply failed")
		}
		if strings.Contains(read(t, settings), "block-rm.sh") {
			t.Errorf("preexisting=%v: removed hook still in settings.json", preexisting)
		}
	}
}

// A handler with its own statusMessage and a command naming no ./ path
// still carries the owner mark, so it is replaced on every apply instead of
// being re-added.
func TestHooksMarkedHandlerWithoutLocalPathNotDuplicated(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: x
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: npm run lint
          statusMessage: linting
`)
	initAndApply(t, proj)
	for i := 0; i < 2; i++ {
		if code := cmdApply(); code != 0 {
			t.Fatal("apply failed")
		}
	}
	s := read(t, filepath.Join(proj, ".claude", "settings.json"))
	if n := strings.Count(s, "npm run lint"); n != 1 {
		t.Errorf("group must be replaced, not accumulated (%d copies):\n%s", n, s)
	}
	if !strings.Contains(s, "agsy:block-rm · linting") {
		t.Errorf("owner mark must precede the user's statusMessage:\n%s", s)
	}
}

// An absolute path that is not one of the hook's own scripts (an
// interpreter) does not make the merge target Stale.
func TestHooksInterpreterPathIsNotStale(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: x
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: /usr/bin/env sh ./block-rm.sh
`)
	initAndApply(t, proj)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	plans, err := mount.InspectMerge(cfg, m.Merges)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plans {
		if p.State != mount.MergeClean {
			t.Errorf("right after apply, state = %d (%s), want Clean", p.State, p.Note)
		}
	}
}

// A ./ path wrapped in quotes is refused up front: the existence check
// would accept it while the rewrite would leave it relative.
func TestHooksQuotedLocalPathRefused(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: x
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: '"./block-rm.sh" --strict'
`)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() {
		if code := cmdApply(); code == 0 {
			t.Error("apply must refuse a quoted ./ path")
		}
	})
	if !strings.Contains(out, "quotes a ./ path") {
		t.Errorf("the refusal must name the quoted path:\n%s", out)
	}
}

// Override problems are reported, never dropped: an index past the group's
// handlers, and a command emptied by the override.
func TestHooksOverrideProblemsReported(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), `description: x
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: ./block-rm.sh
      overrides:
        cursor:
          hooks:
            - {}
            - command: ./does-not-exist.sh
        codex:
          hooks:
            - command: ""
`)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() { cmdPlan() })
	for _, want := range []string{"overrides.cursor of PreToolUse lists 2 handlers, but the group has only 1", "overrides.codex leaves handler 1 of PreToolUse without a command"} {
		if !strings.Contains(out, want) {
			t.Errorf("plan must report %q:\n%s", want, out)
		}
	}
}

// A mount or merge failure after the build must not lose the previous
// merge records (created flag, containers agsy added): the manifest is
// written with them right after the build. The failure here is a .claude
// that became a plain file, which breaks the mount step.
func TestHooksMergeRecordsSurviveMergeFailure(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	before, err := build.LoadManifest(cfg.OutDir())
	if err != nil || len(before.Merges) != 1 || !before.Merges[0].Created {
		t.Fatalf("expected one created merge record, got %+v (%v)", before.Merges, err)
	}
	claudeDir := filepath.Join(proj, ".claude")
	if err := os.RemoveAll(claudeDir); err != nil {
		t.Fatal(err)
	}
	write(t, claudeDir, "not a directory\n")
	if code := cmdApply(); code == 0 {
		t.Fatal("apply should fail when .claude is a plain file")
	}
	after, err := build.LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Merges) != 1 || !after.Merges[0].Created || !after.Merges[0].AddedKey {
		t.Errorf("merge record lost after a failed merge step: %+v", after.Merges)
	}
}

// Merge records are project-relative; a manifest holding an absolute or
// escaping record path was tampered with and is refused as a whole.
func TestManifestRejectsNonLocalMergePath(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := build.LoadManifest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/usr", "../usr", ""} {
		m.Merges[0].HooksDir = bad
		if bad == "" {
			m.Merges[0].Path = ""
		}
		if err := build.WriteManifest(cfg.OutDir(), m); err != nil {
			t.Fatal(err)
		}
		if _, err := build.LoadManifest(cfg.OutDir()); err == nil || !strings.Contains(err.Error(), "invalid merge record path") {
			t.Errorf("record path %q must be refused, got %v", bad, err)
		}
	}
}

// After the project is moved, the project-relative records still resolve:
// apply rewrites the stale paths and status is clean afterwards, with no
// record left pointing at the old location.
func TestHooksMovedProjectRecordsFollow(t *testing.T) {
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
	if code := cmdApply(); code != 0 {
		t.Fatal("apply after move failed")
	}
	out := captureStdout(t, func() {
		if code := cmdStatus(false); code != 0 {
			t.Error("status must be clean after apply in the moved project")
		}
	})
	if strings.Contains(out, "outside the project") {
		t.Errorf("no record may point at the old location:\n%s", out)
	}
	if m := read(t, filepath.Join(moved, ".agsy", ".agsy-manifest.json")); strings.Contains(m, "before") {
		t.Errorf("manifest still names the old location:\n%s", m)
	}
}

// A tampered hooks directory in the manifest cannot claim the user's own
// groups: the manifest is refused, and clean still strips agsy's groups (by
// their mark) while the user's survive.
func TestHooksTamperedHooksDirDoesNotClaimUserGroups(t *testing.T) {
	proj := hookProjectAt(t, "p")
	settings := filepath.Join(proj, ".claude", "settings.json")
	write(t, settings, "{\n  \"hooks\": {\"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"/usr/bin/env echo bye\"}]}]}\n}\n")
	initAndApply(t, proj)
	mp := filepath.Join(proj, ".agsy", ".agsy-manifest.json")
	write(t, mp, strings.Replace(read(t, mp), `"hooksDir": ".agsy/hooks"`, `"hooksDir": "../../usr"`, 1))
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	s := read(t, settings)
	if !strings.Contains(s, "echo bye") {
		t.Errorf("the user's group must survive clean:\n%s", s)
	}
	if strings.Contains(s, "agsy:") {
		t.Errorf("agsy's own group must still be stripped:\n%s", s)
	}
}

// Removing every hook from the sources leaves the settings.json agsy
// created as an empty object; clean deletes it (it holds nothing of the
// user's), matching the case where clean runs while the hooks still exist.
func TestCleanDeletesCreatedEmptyIdleTarget(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	settings := filepath.Join(proj, ".claude", "settings.json")
	if err := os.RemoveAll(filepath.Join(proj, "repo-ai-lib", "hooks")); err != nil {
		t.Fatal(err)
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply without hooks failed")
	}
	if s := read(t, settings); strings.TrimSpace(s) != "{}" {
		t.Fatalf("apply must leave an empty object, got %q", s)
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if _, err := os.Lstat(settings); !os.IsNotExist(err) {
		t.Errorf("settings.json created by agsy and left empty must be deleted by clean, stat err=%v", err)
	}
}

// A file the user created stays through the same sequence, even when it
// ends up empty.
func TestCleanKeepsUserEmptyIdleTarget(t *testing.T) {
	proj := hookProjectAt(t, "p")
	settings := filepath.Join(proj, ".claude", "settings.json")
	write(t, settings, "{}\n")
	initAndApply(t, proj)
	if err := os.RemoveAll(filepath.Join(proj, "repo-ai-lib", "hooks")); err != nil {
		t.Fatal(err)
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply without hooks failed")
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if _, err := os.Lstat(settings); err != nil {
		t.Errorf("a user-created file must not be deleted: %v", err)
	}
}

// An edited hook registry is reported as a derived file with guidance
// pointing at hook.yaml, not as a source path.
func TestRegistryEditedGuidance(t *testing.T) {
	proj := hookProjectAt(t, "p")
	initAndApply(t, proj)
	reg := filepath.Join(proj, ".agsy", "hooks.codex.json")
	write(t, reg, read(t, reg)+"\n")
	out := captureStdout(t, func() { cmdStatus(false) })
	if !strings.Contains(out, "edit the matching hook.yaml") || strings.Contains(out, "not inside the configured sources") {
		t.Errorf("registry change must point at hook.yaml:\n%s", out)
	}
}

// A ./ path inside a quoted string would be rewritten into a broken command
// line (the whitespace tokenizer and the quote-aware one disagree); it is
// refused like a directly quoted ./ path.
func TestHooksQuotedLocalPathInsideStringRefused(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), "events:\n  PreToolUse:\n    - hooks:\n        - command: \"sh -c 'echo ./block-rm.sh'\"\n")
	chdir(t, proj)
	prompt.AssumeYes = true
	t.Cleanup(func() { prompt.AssumeYes = false })
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() {
		if code := cmdPlan(); code == 0 {
			t.Error("plan must fail")
		}
	})
	if !strings.Contains(out, "quotes a ./ path") {
		t.Errorf("expected the quoted-path error:\n%s", out)
	}
}

// A target field that is neither a string nor a list of strings is an
// error, not "every tool".
func TestHooksTargetMalformedRefused(t *testing.T) {
	proj := hookProjectAt(t, "p")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), "target: 123\nevents:\n  PreToolUse:\n    - hooks:\n        - command: ./block-rm.sh\n")
	chdir(t, proj)
	prompt.AssumeYes = true
	t.Cleanup(func() { prompt.AssumeYes = false })
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() {
		if code := cmdPlan(); code == 0 {
			t.Error("plan must fail")
		}
	})
	if !strings.Contains(out, "must be a tool name or a list of tool names") {
		t.Errorf("expected the malformed-target error:\n%s", out)
	}
}

// plan prints a hook's description under its line.
func TestPlanShowsHookDescription(t *testing.T) {
	proj := hookProjectAt(t, "p")
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() { cmdPlan() })
	if !strings.Contains(out, "block rm") {
		t.Errorf("plan must show the hook description:\n%s", out)
	}
}
