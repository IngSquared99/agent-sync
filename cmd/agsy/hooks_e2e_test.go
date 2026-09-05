package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Message assertions below compare the English source strings.
func init() { i18n.SetLang("en") }

// End-to-end flow for the hooks category: init → apply writes four registries,
// three links and the settings.json merge → an edited agsy entry is reported
// by status and restored by apply → clean leaves only user content.

const e2eHook = `description: block rm
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: ./block-rm.sh
      overrides:
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
`

func newHookProject(t *testing.T) string {
	t.Helper()
	proj := newProject(t)
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "hook.yaml"), e2eHook)
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
	return proj
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(raw)
}

func TestHooksInitApplyStatusClean(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	// pre-existing user settings must survive every step
	settings := filepath.Join(proj, ".claude", "settings.json")
	write(t, settings, "{\n  \"permissions\": {\"allow\": [\"Bash(npm test)\"]},\n  \"hooks\": {\"Stop\": [{\"hooks\": [{\"type\": \"command\", \"command\": \"echo bye\"}]}]}\n}\n")

	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	cfgRaw := read(t, filepath.Join(proj, "agsy.yaml"))
	for _, want := range []string{"version: 2", "hooks:     error", "merge:", "settings.json: hooks.claude.json", "dir: .codex", "dir: .cursor", "hooks.json: hooks.antigravity.json"} {
		if !strings.Contains(cfgRaw, want) {
			t.Errorf("agsy.yaml lacks %q:\n%s", want, cfgRaw)
		}
	}
	gi := read(t, filepath.Join(proj, ".gitignore"))
	if !strings.Contains(gi, ".codex/hooks.json") || strings.Contains(gi, "settings.json") {
		t.Errorf(".gitignore wrong:\n%s", gi)
	}

	if code := cmdPlan(); code != 0 {
		t.Fatal("plan failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	script := filepath.Join(proj, ".agsy", "hooks", "block-rm", "block-rm.sh")
	for _, link := range []string{".codex/hooks.json", ".cursor/hooks.json", ".agents/hooks.json"} {
		p := filepath.Join(proj, link)
		if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s should be a link", link)
		}
		if !strings.Contains(read(t, p), script) {
			t.Errorf("%s does not point at the script", link)
		}
	}
	s := read(t, settings)
	if !strings.Contains(s, "Bash(npm test)") || !strings.Contains(s, "echo bye") || !strings.Contains(s, script) {
		t.Errorf("settings.json after apply:\n%s", s)
	}
	if strings.Index(s, `"permissions"`) > strings.Index(s, `"hooks"`) {
		t.Errorf("key order changed:\n%s", s)
	}
	if code := cmdStatus(false); code != 0 {
		t.Fatal("status right after apply should be 0")
	}

	// user edits the agsy group → status 1 → apply (confirmed) restores
	edited := strings.Replace(s, `"matcher": "Bash"`, `"matcher": "Edit"`, 1)
	if edited == s {
		t.Fatalf("expected agsy group in settings:\n%s", s)
	}
	if err := os.WriteFile(settings, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdStatus(false); code != 1 {
		t.Fatal("status should report the modified merge entry")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply after edit failed")
	}
	if s2 := read(t, settings); !strings.Contains(s2, `"matcher": "Bash"`) || !strings.Contains(s2, "echo bye") {
		t.Errorf("apply did not restore the agsy group:\n%s", s2)
	}

	// a user-added group is untouched by apply and survives clean
	s3 := strings.Replace(read(t, settings), `"Stop": [`, `"Stop": [{"hooks": [{"command": "mine.sh"}]}, `, 1)
	if err := os.WriteFile(settings, []byte(s3), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdStatus(false); code != 0 {
		t.Fatal("foreign group must not count as a gap")
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	after := read(t, settings)
	if strings.Contains(after, ".agsy") || !strings.Contains(after, "mine.sh") || !strings.Contains(after, "echo bye") || !strings.Contains(after, "Bash(npm test)") {
		t.Errorf("settings.json after clean:\n%s", after)
	}
	for _, link := range []string{".codex/hooks.json", ".cursor/hooks.json", ".agents/hooks.json", ".agsy"} {
		if _, err := os.Lstat(filepath.Join(proj, link)); err == nil {
			t.Errorf("%s should be gone after clean", link)
		}
	}
}

func TestHooksApplyRefusesRealRegistryAndSymlinkSettings(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	write(t, filepath.Join(proj, ".codex", "hooks.json"), "{}")
	if code := cmdApply(); code == 0 {
		t.Fatal("apply must refuse when .codex/hooks.json is a real file")
	}
	os.Remove(filepath.Join(proj, ".codex", "hooks.json"))
	real := filepath.Join(proj, ".claude", "real.json")
	write(t, real, "{}")
	if err := os.Symlink(real, filepath.Join(proj, ".claude", "settings.json")); err != nil {
		t.Skip("symlinks unavailable")
	}
	if code := cmdApply(); code == 0 {
		t.Fatal("apply must refuse when settings.json is a symlink")
	}
	if _, err := os.Stat(filepath.Join(proj, ".agsy")); err == nil {
		t.Error("nothing should be built when the pre-check fails")
	}
}

func TestHooksCreatedSettingsRemovedByClean(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	settings := filepath.Join(proj, ".claude", "settings.json")
	if _, err := os.Stat(settings); err != nil {
		t.Fatal("apply should create settings.json")
	}
	if code := cmdClean(); code != 0 {
		t.Fatal("clean failed")
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Error("settings.json created by agsy must be removed when empty")
	}
}

func TestUpgradeFromV1ConfigExplains(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	write(t, filepath.Join(proj, "agsy.yaml"), `version: 1
sources: [./repo-ai-lib]
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename}
  tools: [claude]
mount:
  - dir: .claude
    links: {rules: rules, skills: skills}
`)
	_, err := loadConfig()
	if err == nil || !strings.Contains(err.Error(), "new in v0.2.0") {
		t.Fatalf("v1 config must fail with the upgrade hint, got %v", err)
	}
	// adding the one line is enough; no registry needs to be mounted
	write(t, filepath.Join(proj, "agsy.yaml"), `version: 1
sources: [./repo-ai-lib]
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}
  tools: [claude]
mount:
  - dir: .claude
    links: {rules: rules, skills: skills}
`)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdApply(); code != 0 {
		t.Fatal("apply with hooks unmounted must still work")
	}
	if _, err := os.Stat(filepath.Join(proj, ".claude", "settings.json")); err == nil {
		t.Error("no merge configured → settings.json must not be created")
	}
}

// captureStdout runs fn and returns what it printed.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		buf := make([]byte, 0, 1<<16)
		tmp := make([]byte, 4096)
		for {
			n, err := r.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
		}
		done <- string(buf)
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
}

func TestHooksPlanAndDoctorOutput(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	// a second hook with a vendor gap and a non-executable script
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "greet", "hook.yaml"), "events:\n  SessionStart:\n    - hooks: [{command: ./greet.sh}]\n")
	write(t, filepath.Join(proj, "repo-ai-lib", "hooks", "greet", "greet.sh"), "#!/bin/sh\n")
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	out := captureStdout(t, func() {
		if code := cmdPlan(); code != 0 {
			t.Error("plan failed")
		}
	})
	for _, want := range []string{
		"hooks → .agsy/hooks/",
		"block-rm",
		"antigravity ✓  claude ✓  codex ✓  cursor ✓",
		"antigravity —",
		"greet: antigravity has no SessionStart",
		"hooks.antigravity.json, hooks.claude.json, hooks.codex.json, hooks.cursor.json",
		"settings.json ⇐ hooks.claude.json",
		"merge into \"hooks\"; file will be created",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan output lacks %q:\n%s", want, out)
		}
	}
	out = captureStdout(t, func() { cmdDoctor() })
	for _, want := range []string{"hooks/", "2 directories", "greet/greet.sh is not executable", "settings.json", "absent, apply creates it"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output lacks %q:\n%s", want, out)
		}
	}
	// a tool listed but not mounted for hooks → hint in plan and doctor
	cfgRaw := read(t, filepath.Join(proj, "agsy.yaml"))
	cfgRaw = strings.Replace(cfgRaw, "  - dir: .cursor        # Cursor\n    links:\n      hooks.json: hooks.cursor.json\n", "", 1)
	if err := os.WriteFile(filepath.Join(proj, "agsy.yaml"), []byte(cfgRaw), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() { cmdPlan() })
	if !strings.Contains(out, `build.tools lists "cursor", but nothing mounts hooks.cursor.json`) {
		t.Errorf("plan must warn about the unmounted registry:\n%s", out)
	}
	out = captureStdout(t, func() { cmdDoctor() })
	if !strings.Contains(out, `build.tools lists "cursor"`) {
		t.Errorf("doctor must warn about the unmounted registry:\n%s", out)
	}
}

func TestHooksCleanSkipsInvalidMergeTarget(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	settings := filepath.Join(proj, ".claude", "settings.json")
	if err := os.WriteFile(settings, []byte("[]"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := cmdStatus(false); code != 1 {
		t.Fatal("invalid settings.json must be a gap")
	}
	if code := cmdApply(); code == 0 {
		t.Fatal("apply must refuse while settings.json is invalid")
	}
	out := captureStdout(t, func() {
		if code := cmdClean(); code != 0 {
			t.Error("clean must still succeed")
		}
	})
	if !strings.Contains(out, "Skipped (symbolic link or not a JSON object)") {
		t.Errorf("clean must report the skipped target:\n%s", out)
	}
	if raw := read(t, settings); raw != "[]" {
		t.Error("invalid file must be left untouched")
	}
	if _, err := os.Lstat(filepath.Join(proj, ".codex", "hooks.json")); err == nil {
		t.Error("links must still be removed")
	}
	if _, err := os.Stat(filepath.Join(proj, ".agsy")); err == nil {
		t.Error("output must still be removed")
	}
}

func TestHooksStatusRestoresDeletedRegistryLink(t *testing.T) {
	proj := newHookProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	defer func() { prompt.AssumeYes = false }()
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	os.Remove(filepath.Join(proj, ".codex", "hooks.json"))
	os.Remove(filepath.Join(proj, ".claude", "settings.json"))
	if code := cmdStatus(false); code != 1 {
		t.Fatal("missing registry link and merge target must be gaps")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	if code := cmdStatus(false); code != 0 {
		t.Fatal("apply must restore both")
	}
}
