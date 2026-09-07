package mount

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Message assertions below compare the English source strings.
func init() { i18n.SetLang("en") }

const mergeYAML = `version: 2
sources:
  - ./.flow
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}
  tools: [claude]
mount:
  - dir: .claude
    links: {rules: rules}
    merge: {settings.json: hooks.claude.json}
`

// setupMerge writes a project whose output already holds a claude registry
// with one agsy group for hook "block-rm".
func setupMerge(t *testing.T) (*config.Config, string, string) {
	t.Helper()
	proj := t.TempDir()
	p := filepath.Join(proj, config.FileName)
	if err := os.WriteFile(p, []byte(mergeYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	out := cfg.OutDir()
	if err := os.MkdirAll(filepath.Join(out, "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(out, "hooks", "block-rm", "block-rm.sh")
	reg := `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":` + jsonStr(script) + `,"timeout":10}]}]}}`
	if err := os.WriteFile(filepath.Join(out, "hooks.claude.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfg, filepath.Join(proj, ".claude", "settings.json"), script
}

func jsonStr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func writeSettings(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func applyOnce(t *testing.T, cfg *config.Config, recs []build.MergeRecord) ([]MergePlan, []build.MergeRecord) {
	t.Helper()
	plans, err := InspectMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	out, err := ApplyMerge(cfg, plans, recs)
	if err != nil {
		t.Fatal(err)
	}
	return plans, out
}

func TestMergeCreatesFileAndRecordsCreated(t *testing.T) {
	cfg, settings, script := setupMerge(t)
	plans, recs := applyOnce(t, cfg, nil)
	if plans[0].State != MergeMissing {
		t.Fatalf("state = %v", plans[0].State)
	}
	raw, err := os.ReadFile(settings)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), script) || !strings.HasPrefix(string(raw), "{\n  \"hooks\": {") {
		t.Errorf("settings.json:\n%s", raw)
	}
	if len(recs) != 1 || !recs[0].Created || recs[0].Hash == "" {
		t.Errorf("records = %+v", recs)
	}
	again, _ := InspectMerge(cfg, recs)
	if again[0].State != MergeClean || again[0].Owned != 1 {
		t.Errorf("after apply: state=%v owned=%d", again[0].State, again[0].Owned)
	}
}

func TestMergePreservesForeignContentAndOrder(t *testing.T) {
	cfg, settings, script := setupMerge(t)
	writeSettings(t, settings, `{
  "zeta": {"deep": [1, 2, {"x": null}]},
  "permissions": {"allow": ["Bash(npm test)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": ".claude/hooks/mine.sh"}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "echo bye"}]}
    ]
  },
  "alpha": true
}
`)
	plans, recs := applyOnce(t, cfg, nil)
	if plans[0].State != MergeAbsent {
		t.Fatalf("state = %v", plans[0].State)
	}
	raw, _ := os.ReadFile(settings)
	s := string(raw)
	// top-level order kept; foreign groups kept; agsy group appended after
	for _, pair := range [][2]string{{`"zeta"`, `"permissions"`}, {`"permissions"`, `"hooks"`}, {`"hooks"`, `"alpha"`}, {`mine.sh`, script}, {`"PreToolUse"`, `"Stop"`}} {
		if strings.Index(s, pair[0]) > strings.Index(s, pair[1]) || !strings.Contains(s, pair[1]) {
			t.Errorf("order/content wrong for %v:\n%s", pair, s)
		}
	}
	if !strings.Contains(s, `"deep": [`) || !strings.Contains(s, `"x": null`) || !strings.Contains(s, `echo bye`) {
		t.Errorf("foreign content lost:\n%s", s)
	}
	if recs[0].Created {
		t.Error("pre-existing file must not be marked created")
	}
	// second apply is idempotent
	applyOnce(t, cfg, recs)
	raw2, _ := os.ReadFile(settings)
	if string(raw2) != s {
		t.Errorf("second apply changed the file:\n%s\n---\n%s", s, raw2)
	}
}

func TestMergeDetectsModifiedAndReplacesWholeGroup(t *testing.T) {
	cfg, settings, script := setupMerge(t)
	_, recs := applyOnce(t, cfg, nil)
	raw, _ := os.ReadFile(settings)
	edited := strings.Replace(string(raw), `"timeout": 10`, `"timeout": 99`, 1)
	if err := os.WriteFile(settings, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, _ := InspectMerge(cfg, recs)
	if plans[0].State != MergeModified {
		t.Fatalf("state = %v, want Modified", plans[0].State)
	}
	// a user handler smuggled into the agsy group is replaced with the group
	mixed := strings.Replace(edited, `"timeout": 99`, `"timeout": 99}, {"type": "command", "command": "my-extra.sh"`, 1)
	if err := os.WriteFile(settings, []byte(mixed), 0o644); err != nil {
		t.Fatal(err)
	}
	applyOnce(t, cfg, recs)
	raw, _ = os.ReadFile(settings)
	if strings.Contains(string(raw), "my-extra.sh") || strings.Contains(string(raw), `"timeout": 99`) || !strings.Contains(string(raw), script) {
		t.Errorf("group not rebuilt:\n%s", raw)
	}
}

func TestMergeInvalidTargets(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	for name, body := range map[string]string{"array": "[1,2]", "broken": "{\"a\":", "hooksNotObject": `{"hooks": 5}`, "eventNotArray": `{"hooks": {"Stop": {}}}`} {
		writeSettings(t, settings, body)
		plans, err := InspectMerge(cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plans[0].State != MergeInvalid {
			t.Errorf("%s: state = %v, want Invalid", name, plans[0].State)
		}
		if _, err := ApplyMerge(cfg, plans, nil); err == nil {
			t.Errorf("%s: ApplyMerge must refuse", name)
		}
	}
	// symlink
	os.Remove(settings)
	real := filepath.Join(filepath.Dir(settings), "real.json")
	writeSettings(t, real, "{}")
	if err := os.Symlink(real, settings); err == nil {
		plans, _ := InspectMerge(cfg, nil)
		if plans[0].State != MergeInvalid || !strings.Contains(plans[0].Note, "symbolic link") {
			t.Errorf("symlink: %v %q", plans[0].State, plans[0].Note)
		}
	}
	// empty file counts as {}
	os.Remove(settings)
	writeSettings(t, settings, "")
	plans, _ := InspectMerge(cfg, nil)
	if plans[0].State != MergeAbsent {
		t.Errorf("empty file: state = %v", plans[0].State)
	}
}

func TestRemoveMergeDeletesOnlyCreatedFiles(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	_, recs := applyOnce(t, cfg, nil)
	cleaned, deleted, skipped, err := RemoveMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1 || len(cleaned) != 0 || len(skipped) != 0 {
		t.Errorf("cleaned=%v deleted=%v skipped=%v", cleaned, deleted, skipped)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Error("created file must be deleted when empty after clean")
	}
	if _, err := os.Stat(filepath.Dir(settings)); !os.IsNotExist(err) {
		t.Error("empty .claude dir must be removed too")
	}

	// pre-existing file: keep it, strip agsy groups, drop empty hooks key
	writeSettings(t, settings, `{"model": "opus", "hooks": {"Stop": [{"hooks": [{"command": "echo"}]}]}}`)
	_, recs = applyOnce(t, cfg, nil)
	cleaned, deleted, _, err = RemoveMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(settings)
	if len(cleaned) != 1 || len(deleted) != 0 || strings.Contains(string(raw), ".agsy") || !strings.Contains(string(raw), `"echo"`) || !strings.Contains(string(raw), `"opus"`) {
		t.Errorf("clean result:\n%s (cleaned=%v deleted=%v)", raw, cleaned, deleted)
	}
	// user-only hooks removed → hooks key dropped entirely
	writeSettings(t, settings, `{"model": "opus"}`)
	_, recs = applyOnce(t, cfg, nil)
	RemoveMerge(cfg, recs)
	raw, _ = os.ReadFile(settings)
	if strings.Contains(string(raw), "hooks") || !strings.Contains(string(raw), "opus") {
		t.Errorf("hooks key should be dropped:\n%s", raw)
	}
}

func TestOwnsGroup(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "p", ".agsy", "hooks")
	in := filepath.Join(abs, "h", "x.sh")
	cases := map[string]bool{
		`{"hooks":[{"command":` + jsonStr(in) + `}]}`:                   true,
		`{"hooks":[{"command":` + jsonStr("python3 "+in) + `}]}`:        true,
		`{"hooks":[{"command":"./x.sh"}]}`:                              false,
		`{"hooks":[{"command":` + jsonStr(abs+"-other/x.sh") + `}]}`:    false,
		`{"hooks":[{"type":"http","url":"http://x"}]}`:                  false,
		`{"hooks":[{"command":"a"},{"command":` + jsonStr(in) + `}]}`:   true,
		`{"hooks":[{"command":` + jsonStr(filepath.ToSlash(in)) + `}]}`: true,
		`5`: false,
		`{"hooks":[{"type":"http","url":"http://x","statusMessage":"agsy:greet"}]}`: true,
		`{"hooks":[{"type":"http","url":"http://x","statusMessage":"mine"}]}`:       false,
		// quoted path (project directory with a space)
		`{"hooks":[{"command":` + jsonStr("'"+in+"'") + `}]}`:               true,
		`{"hooks":[{"command":` + jsonStr("python3 \""+in+"\" --x") + `}]}`: true,
	}
	for g, want := range cases {
		if got := ownsGroup(json.RawMessage(g), []string{abs}); got != want {
			t.Errorf("ownsGroup(%s) = %v, want %v", g, got, want)
		}
	}
}

func TestIsFileTarget(t *testing.T) {
	for _, s := range []string{"AGENTS.md", "hooks.claude.json", "hooks.cursor.json", "/hooks.codex.json/"} {
		if !IsFileTarget(s) {
			t.Errorf("%s should be a file target", s)
		}
	}
	for _, s := range []string{"rules", "hooks", "skills"} {
		if IsFileTarget(s) {
			t.Errorf("%s should not be a file target", s)
		}
	}
}

// ── additional edge cases ────────────────────────────────────────────

func TestMergeHooksNullAndKeyPosition(t *testing.T) {
	cfg, settings, script := setupMerge(t)
	// "hooks": null is treated as absent; the key keeps its position
	writeSettings(t, settings, `{"hooks": null, "model": "opus"}`)
	applyOnce(t, cfg, nil)
	raw, _ := os.ReadFile(settings)
	s := string(raw)
	if strings.Index(s, `"hooks"`) > strings.Index(s, `"model"`) || !strings.Contains(s, script) {
		t.Errorf("hooks must stay first and be filled:\n%s", s)
	}
	// absent key is appended at the end
	writeSettings(t, settings, `{"model": "opus", "env": {"A": "1"}}`)
	applyOnce(t, cfg, nil)
	raw, _ = os.ReadFile(settings)
	s = string(raw)
	if strings.Index(s, `"env"`) > strings.Index(s, `"hooks"`) {
		t.Errorf("hooks must be appended after existing keys:\n%s", s)
	}
}

func TestMergePreservesFilePermissions(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	writeSettings(t, settings, `{}`)
	if err := os.Chmod(settings, 0o600); err != nil {
		t.Fatal(err)
	}
	applyOnce(t, cfg, nil)
	fi, _ := os.Stat(settings)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(filepath.Dir(settings))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".agsy-merge-") {
			t.Error("temp file left behind")
		}
	}
}

func TestMergeUserGroupWithNonAgsyStatusMessageIsForeign(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	writeSettings(t, settings, `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "http", "url": "http://x", "statusMessage": "agsy-lookalike"}]}]}}`)
	plans, _ := InspectMerge(cfg, nil)
	if plans[0].Owned != 0 {
		t.Error("only the exact agsy: prefix marks ownership")
	}
	applyOnce(t, cfg, nil)
	raw, _ := os.ReadFile(settings)
	if !strings.Contains(string(raw), "agsy-lookalike") {
		t.Error("foreign group must survive")
	}
}

func TestMergeEmptyRegistryLeavesFileAlone(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	if err := os.WriteFile(filepath.Join(cfg.OutDir(), "hooks.claude.json"), []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// no file → none created, state clean
	plans, recs := applyOnce(t, cfg, nil)
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Error("empty registry must not create settings.json")
	}
	if plans[0].State != MergeIdle {
		t.Errorf("state = %v, want Idle", plans[0].State)
	}
	// existing user file → untouched byte for byte
	body := "{\n\t\"model\": \"opus\"\n}\n"
	writeSettings(t, settings, body)
	plans, recs = applyOnce(t, cfg, recs)
	raw, _ := os.ReadFile(settings)
	if string(raw) != body {
		t.Errorf("file must be left as is when nothing is merged:\n%s", raw)
	}
	if plans[0].State != MergeIdle {
		t.Errorf("state = %v, want Idle", plans[0].State)
	}
	// Idle target: clean neither creates nor rewrites the file.
	cleaned, deleted, _, err := RemoveMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(cleaned) != 0 || len(deleted) != 0 {
		t.Errorf("clean must not touch an idle target: cleaned=%v deleted=%v", cleaned, deleted)
	}
	if raw, _ := os.ReadFile(settings); string(raw) != body {
		t.Errorf("clean rewrote a file holding nothing of agsy's:\n%s", raw)
	}
	os.Remove(settings)
	if _, _, _, err := RemoveMerge(cfg, recs); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(settings); !os.IsNotExist(err) {
		t.Error("clean must not create a settings.json out of nothing")
	}
}

func TestRemoveMergeSkipsInvalidAndReports(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	_, recs := applyOnce(t, cfg, nil)
	writeSettings(t, settings, `not json`)
	cleaned, deleted, skipped, err := RemoveMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(skipped) != 1 || len(cleaned)+len(deleted) != 0 {
		t.Errorf("cleaned=%v deleted=%v skipped=%v", cleaned, deleted, skipped)
	}
	raw, _ := os.ReadFile(settings)
	if string(raw) != "not json" {
		t.Error("invalid file must not be touched")
	}
}

func TestInspectMergeIgnoresForeignManifestRecords(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	writeSettings(t, settings, `{}`)
	// a record for another path must not influence this target
	plans, _ := InspectMerge(cfg, []build.MergeRecord{{Path: "/elsewhere/settings.json", Key: MergeKey, Hash: "x", Created: true}})
	if plans[0].Created {
		t.Error("created flag must come from a record for this exact path")
	}
}

// Pre-existing empty containers survive: an event array that was empty
// before apply is empty again after clean, and the "hooks" key stays. Only
// a file agsy created is deleted.
func TestMergeKeepsUserEmptyShells(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	writeSettings(t, settings, `{"model":"opus","hooks":{"PreToolUse":[],"Stop":[]}}`)
	_, recs := applyOnce(t, cfg, nil)
	raw, _ := os.ReadFile(settings)
	if !strings.Contains(string(raw), `"Stop": []`) {
		t.Errorf("apply must keep the user's empty Stop array:\n%s", raw)
	}
	if _, _, _, err := RemoveMerge(cfg, recs); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(settings)
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if string(doc["hooks"]) == "" {
		t.Fatalf("clean must keep the hooks key the user wrote:\n%s", raw)
	}
	var hooks map[string][]json.RawMessage
	if err := json.Unmarshal(doc["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks["PreToolUse"]) != 0 || len(hooks["Stop"]) != 0 || len(hooks) != 2 {
		t.Errorf("clean must restore the user's empty arrays: %s", doc["hooks"])
	}

	// File created by apply: deleted by clean.
	os.Remove(settings)
	_, recs = applyOnce(t, cfg, nil)
	if _, deleted, _, err := RemoveMerge(cfg, recs); err != nil || len(deleted) != 1 {
		t.Fatalf("file created by agsy must be deleted on clean: %v %v", deleted, err)
	}
}

// Groups whose command points into the hooks directory recorded in the
// manifest are agsy's.
func TestMergeRecordedHooksDirStillOwned(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	old := filepath.Join(filepath.Dir(cfg.OutDir()), ".old-out", "hooks")
	group := `{"matcher":"Bash","hooks":[{"type":"command","command":` + jsonStr(filepath.Join(old, "block-rm", "block-rm.sh")) + `}]}`
	writeSettings(t, settings, `{"hooks":{"PreToolUse":[`+group+`]}}`)
	hash := build.CanonicalHash(map[string][]json.RawMessage{"PreToolUse": {json.RawMessage(group)}})
	recs := []build.MergeRecord{{Path: settings, Key: MergeKey, Hash: hash, Created: false, HooksDir: old}}
	plans, err := InspectMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].Owned != 1 || plans[0].State != MergeStale {
		t.Fatalf("group under the recorded hooks dir must be owned and stale, got owned=%d state=%v", plans[0].Owned, plans[0].State)
	}
	_, recs = applyOnce(t, cfg, recs)
	raw, _ := os.ReadFile(settings)
	if strings.Contains(string(raw), ".old-out") || strings.Count(string(raw), "block-rm.sh") != 1 {
		t.Errorf("old group must be replaced, not kept beside the new one:\n%s", raw)
	}
	if recs[0].HooksDir != filepath.Join(cfg.OutDir(), "hooks") {
		t.Errorf("record must carry the current hooks dir: %q", recs[0].HooksDir)
	}
}

// When a hook moves to another event, the event array apply introduced
// earlier is removed once empty.
func TestMergeDropsOwnEmptyEventOnChange(t *testing.T) {
	cfg, settings, script := setupMerge(t)
	writeSettings(t, settings, `{"model":"opus"}`)
	_, recs := applyOnce(t, cfg, nil)
	if recs[0].AddedKey != true || len(recs[0].AddedEvents) != 1 || recs[0].AddedEvents[0] != "PreToolUse" {
		t.Fatalf("record must name the shells apply added: %+v", recs[0])
	}
	reg := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":` + jsonStr(script) + `}]}]}}`
	if err := os.WriteFile(filepath.Join(cfg.OutDir(), "hooks.claude.json"), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	_, recs = applyOnce(t, cfg, recs)
	raw, _ := os.ReadFile(settings)
	if strings.Contains(string(raw), "PreToolUse") || !strings.Contains(string(raw), `"Stop"`) {
		t.Errorf("empty PreToolUse shell must go, Stop must appear:\n%s", raw)
	}
	if len(recs[0].AddedEvents) != 1 || recs[0].AddedEvents[0] != "Stop" {
		t.Errorf("record must follow: %+v", recs[0])
	}
}

// An idle record (nothing was merged last time) must not stick: once the
// registry gains groups, inspect reports Missing / Absent and apply merges.
func TestMergeIdleRecordDoesNotStick(t *testing.T) {
	cfg, settings, _ := setupMerge(t)
	reg := filepath.Join(cfg.OutDir(), "hooks.claude.json")
	full, _ := os.ReadFile(reg)
	if err := os.WriteFile(reg, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, recs := applyOnce(t, cfg, nil)
	if plans[0].State != MergeIdle || recs[0].Hash != emptyHash {
		t.Fatalf("first apply: state %v hash %s", plans[0].State, recs[0].Hash)
	}
	// registry gains a group (a hook was added and built)
	if err := os.WriteFile(reg, full, 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err := InspectMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].State != MergeMissing {
		t.Errorf("state = %v, want Missing (registry now has groups)", plans[0].State)
	}
	plans, recs = applyOnce(t, cfg, recs)
	raw, err := os.ReadFile(settings)
	if err != nil || !strings.Contains(string(raw), "block-rm.sh") {
		t.Fatalf("apply after an idle record must merge: %v\n%s", err, raw)
	}
	// same with an existing file holding nothing of agsy's
	writeSettings(t, settings, `{"model":"opus"}`)
	if err := os.WriteFile(reg, []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, recs = applyOnce(t, cfg, nil)
	if err := os.WriteFile(reg, full, 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err = InspectMerge(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if plans[0].State != MergeAbsent {
		t.Errorf("state = %v, want Absent", plans[0].State)
	}
	applyOnce(t, cfg, recs)
	raw, _ = os.ReadFile(settings)
	if !strings.Contains(string(raw), "block-rm.sh") || !strings.Contains(string(raw), `"model"`) {
		t.Errorf("apply must merge into the existing file and keep its keys:\n%s", raw)
	}
}

// Stale means "a script path of this hook points into a previous output
// hooks directory": a path under a recorded previous directory, or (without
// a record) one whose tail is hooks/<name>/ outside the current directory.
// Interpreter paths and other absolute paths never count.
func TestHasStalePath(t *testing.T) {
	cur := filepath.Join(t.TempDir(), "proj", ".agsy", "hooks")
	old := filepath.Join(t.TempDir(), "old", ".agsy", "hooks")
	group := func(cmd, status string) map[string][]json.RawMessage {
		g := `{"hooks":[{"type":"command","command":` + jsonStr(cmd) + `,"statusMessage":` + jsonStr(status) + `}]}`
		return map[string][]json.RawMessage{"PreToolUse": {json.RawMessage(g)}}
	}
	mark := build.OwnerMark + "block-rm"
	curScript := filepath.Join(cur, "block-rm", "block-rm.sh")
	oldScript := filepath.Join(old, "block-rm", "block-rm.sh")
	cases := []struct {
		name     string
		owned    map[string][]json.RawMessage
		prefixes []string
		want     bool
	}{
		{"current path", group(curScript, mark), []string{cur}, false},
		{"interpreter before current path", group("/usr/bin/env sh "+curScript, mark), []string{cur}, false},
		{"recorded previous dir", group(oldScript, mark), []string{cur, old}, true},
		{"previous dir by tail, no record", group(oldScript, mark), []string{cur}, true},
		{"previous dir by tail, user message after mark", group(oldScript, mark+" · running"), []string{cur}, true},
		{"foreign absolute path, no record", group("/opt/tools/hooks/other/run.sh", mark), []string{cur}, false},
		{"unmarked group, foreign path", group("/opt/x/hooks/block-rm/run.sh", ""), []string{cur}, false},
	}
	for _, c := range cases {
		if got := hasStalePath(c.owned, "hooks", c.prefixes); got != c.want {
			t.Errorf("%s: stale=%v, want %v", c.name, got, c.want)
		}
	}
}

// Recorded merge targets outside the project are never opened, but they
// are reported as foreign rather than dropped.
func TestMergeOrphansReportsForeignRecords(t *testing.T) {
	cfg, _, _ := setupMerge(t)
	outside := filepath.Join(t.TempDir(), ".claude", "settings.json")
	recs := []build.MergeRecord{{Path: outside, Key: MergeKey, Hash: "x"}}
	orphans, foreign, err := MergeOrphans(cfg, recs)
	if err != nil {
		t.Fatal(err)
	}
	if len(orphans) != 0 {
		t.Errorf("outside path must not be inspected as an orphan: %v", orphans)
	}
	if len(foreign) != 1 || foreign[0] != filepath.Clean(outside) {
		t.Errorf("outside path must be reported as foreign: %v", foreign)
	}
}
