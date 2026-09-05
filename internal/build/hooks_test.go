package build

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/config"
)

const hooksYAML = `version: 2
sources:
  - ../lib
build:
  out: .agsy
  on_conflict: {rules: rename, skills: error, workflows: rename, hooks: %s}
  tools: [claude, codex, antigravity, cursor]
mount:
  - dir: .
    links: {AGENTS.md: AGENTS.md}
  - dir: .claude
    links: {rules: rules, skills: skills}
    merge: {settings.json: hooks.claude.json}
  - dir: .agents
    links: {skills: skills, workflows: workflows, hooks.json: hooks.antigravity.json}
  - dir: .codex
    links: {hooks.json: hooks.codex.json}
  - dir: .cursor
    links: {hooks.json: hooks.cursor.json}
`

const blockRM = `description: 擋下 rm -rf
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - type: command
          command: ./block-rm.sh
          timeout: 10
      overrides:
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
`

func setupHooks(t *testing.T, strategy string) (*config.Config, string) {
	t.Helper()
	cfg := newProject(t, strings.Replace(hooksYAML, "%s", strategy, 1))
	lib := filepath.Join(filepath.Dir(cfg.BaseDir), "lib")
	writeFile(t, filepath.Join(lib, "hooks", "block-rm", "hook.yaml"), blockRM)
	writeFile(t, filepath.Join(lib, "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
	return cfg, lib
}

func readJSON(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]interface{}{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, raw)
	}
	return m
}

func TestHookAccepts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ok", "hook.yaml"), blockRM)
	writeFile(t, filepath.Join(dir, "nofile", "x.sh"), "")
	writeFile(t, filepath.Join(dir, "single.yaml"), "")
	if ok, _ := Accepts("hooks", filepath.Join(dir, "ok"), true); !ok {
		t.Error("directory with hook.yaml must be accepted")
	}
	if ok, r := Accepts("hooks", filepath.Join(dir, "nofile"), true); ok || !strings.Contains(r, "hook.yaml") {
		t.Errorf("directory without hook.yaml must be rejected: %q", r)
	}
	if ok, _ := Accepts("hooks", filepath.Join(dir, "single.yaml"), false); ok {
		t.Error("single file must be rejected")
	}
	if err := os.Symlink(filepath.Join(dir, "single.yaml"), filepath.Join(dir, "ok", "link")); err == nil {
		if ok, r := Accepts("hooks", filepath.Join(dir, "ok"), true); ok || !strings.Contains(r, "symbolic link") {
			t.Errorf("hook containing a symlink must be rejected: %q", r)
		}
	}
}

func TestHookRegistriesGolden(t *testing.T) {
	cfg, _ := setupHooks(t, "error")
	p := compute(t, cfg)
	if len(p.RouteErrors) > 0 {
		t.Fatalf("unexpected route errors: %v", p.RouteErrors)
	}
	var hook *Item
	for i := range p.Items {
		if p.Items[i].Category == "hooks" {
			hook = &p.Items[i]
		}
	}
	if hook == nil || hook.Hook == nil {
		t.Fatal("hook item not resolved")
	}
	for _, tool := range config.HookTools {
		if !hook.HookOut[tool] {
			t.Errorf("hook should reach %s", tool)
		}
	}
	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	out := cfg.OutDir()
	script := filepath.Join(out, "hooks", "block-rm", "block-rm.sh")
	if _, err := os.Stat(script); err != nil {
		t.Fatal("script not copied")
	}

	// claude / codex: nested, matcher Bash, absolute command
	for _, tool := range []string{"claude", "codex"} {
		doc := readJSON(t, filepath.Join(out, config.HookRegistryFiles[tool]))
		hooks := doc["hooks"].(map[string]interface{})
		groups := hooks["PreToolUse"].([]interface{})
		g := groups[0].(map[string]interface{})
		if g["matcher"] != "Bash" {
			t.Errorf("%s matcher = %v", tool, g["matcher"])
		}
		h := g["hooks"].([]interface{})[0].(map[string]interface{})
		if h["type"] != "command" || h["command"] != script || h["timeout"] != float64(10) {
			t.Errorf("%s handler = %v", tool, h)
		}
	}
	// antigravity: named, matcher run_command
	ag := readJSON(t, filepath.Join(out, "hooks.antigravity.json"))
	entry := ag["block-rm"].(map[string]interface{})
	if entry["enabled"] != true {
		t.Errorf("antigravity enabled = %v", entry["enabled"])
	}
	g := entry["PreToolUse"].([]interface{})[0].(map[string]interface{})
	if g["matcher"] != "run_command" {
		t.Errorf("antigravity matcher = %v", g["matcher"])
	}
	// cursor: flat, preToolUse, matcher Shell, no type
	cu := readJSON(t, filepath.Join(out, "hooks.cursor.json"))
	if cu["version"] != float64(1) {
		t.Errorf("cursor version = %v", cu["version"])
	}
	fh := cu["hooks"].(map[string]interface{})["preToolUse"].([]interface{})[0].(map[string]interface{})
	if fh["command"] != script || fh["matcher"] != "Shell" || fh["timeout"] != float64(10) {
		t.Errorf("cursor handler = %v", fh)
	}
	if _, has := fh["type"]; has {
		t.Error("cursor command handler must not carry type")
	}
	// key order is deterministic: type before command in nested shape
	raw, _ := os.ReadFile(filepath.Join(out, "hooks.claude.json"))
	if !strings.Contains(string(raw), "\"type\": \"command\",\n") || strings.Index(string(raw), "\"type\"") > strings.Index(string(raw), "\"command\"") {
		t.Errorf("handler key order wrong:\n%s", raw)
	}
	// manifest carries one registry item per tool
	m, err := LoadManifest(out)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, it := range m.Items {
		if it.Category == "hooks-registry" {
			n++
			if len(it.Outs) != 1 || !it.Outs[0].Derived {
				t.Errorf("registry item %s not derived: %+v", it.Name, it.Outs)
			}
		}
	}
	if n != 4 {
		t.Errorf("registry items = %d, want 4", n)
	}
}

func TestHookEmptyRegistriesAlwaysWritten(t *testing.T) {
	cfg := newProject(t, strings.Replace(hooksYAML, "%s", "error", 1))
	writeFile(t, filepath.Join(filepath.Dir(cfg.BaseDir), "lib", "rules", "a.md"), "a\n")
	p := compute(t, cfg)
	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	out := cfg.OutDir()
	want := map[string]string{
		"hooks.claude.json":      "{\n  \"hooks\": {}\n}\n",
		"hooks.codex.json":       "{\n  \"hooks\": {}\n}\n",
		"hooks.antigravity.json": "{}\n",
		"hooks.cursor.json":      "{\n  \"version\": 1,\n  \"hooks\": {}\n}\n",
	}
	for f, body := range want {
		raw, err := os.ReadFile(filepath.Join(out, f))
		if err != nil {
			t.Fatalf("%s missing: %v", f, err)
		}
		if string(raw) != body {
			t.Errorf("%s = %q, want %q", f, raw, body)
		}
	}
}

func TestHookVendorGapsAreNotesNotErrors(t *testing.T) {
	cfg, lib := setupHooks(t, "error")
	writeFile(t, filepath.Join(lib, "hooks", "greet", "hook.yaml"), `events:
  SessionStart:
    - hooks:
        - type: command
          command: ./greet.sh
        - type: http
          url: http://localhost:1
`)
	writeFile(t, filepath.Join(lib, "hooks", "greet", "greet.sh"), "")
	p := compute(t, cfg)
	if len(p.RouteErrors) > 0 {
		t.Fatalf("vendor gaps must not be errors: %v", p.RouteErrors)
	}
	var greet Item
	for _, it := range p.Items {
		if it.Name == "greet" {
			greet = it
		}
	}
	if greet.HookOut["antigravity"] {
		t.Error("antigravity has no SessionStart; hook must not reach it")
	}
	if !greet.HookOut["claude"] || !greet.HookOut["codex"] || !greet.HookOut["cursor"] {
		t.Errorf("reach = %v", greet.HookOut)
	}
	for _, want := range []string{"antigravity has no SessionStart", "codex does not support handler type \"http\""} {
		if !strings.Contains(greet.RouteNote, want) {
			t.Errorf("note missing %q in %q", want, greet.RouteNote)
		}
	}
	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	ag := readJSON(t, filepath.Join(cfg.OutDir(), "hooks.antigravity.json"))
	if _, has := ag["greet"]; has {
		t.Error("antigravity registry must not contain a hook with no reachable events")
	}
	cl := readJSON(t, filepath.Join(cfg.OutDir(), "hooks.claude.json"))
	hs := cl["hooks"].(map[string]interface{})["SessionStart"].([]interface{})[0].(map[string]interface{})["hooks"].([]interface{})
	if len(hs) != 2 {
		t.Errorf("claude keeps both handlers, got %d", len(hs))
	}
	if hs[1].(map[string]interface{})["statusMessage"] != "agsy:greet" {
		t.Errorf("non-command handler must carry the owner mark: %v", hs[1])
	}
	if _, has := hs[0].(map[string]interface{})["statusMessage"]; has {
		t.Error("command handlers need no owner mark")
	}
	cx := readJSON(t, filepath.Join(cfg.OutDir(), "hooks.codex.json"))
	hs = cx["hooks"].(map[string]interface{})["SessionStart"].([]interface{})[0].(map[string]interface{})["hooks"].([]interface{})
	if len(hs) != 1 {
		t.Errorf("codex drops the http handler, got %d", len(hs))
	}
}

func TestHookRouteErrors(t *testing.T) {
	cfg, lib := setupHooks(t, "error")
	writeFile(t, filepath.Join(lib, "hooks", "bad-event", "hook.yaml"), "events:\n  beforeShellExecution:\n    - hooks: [{command: ./x}]\n")
	writeFile(t, filepath.Join(lib, "hooks", "bad-event", "x"), "")
	writeFile(t, filepath.Join(lib, "hooks", "bad-tool", "hook.yaml"), "target: [nosuch]\nevents:\n  Stop:\n    - hooks: [{command: ./x}]\n")
	writeFile(t, filepath.Join(lib, "hooks", "bad-tool", "x"), "")
	writeFile(t, filepath.Join(lib, "hooks", "bad-ov", "hook.yaml"), "events:\n  Stop:\n    - hooks: [{command: ./x}]\n      overrides: {nope: {matcher: a}}\n")
	writeFile(t, filepath.Join(lib, "hooks", "bad-ov", "x"), "")
	writeFile(t, filepath.Join(lib, "hooks", "no-cmd", "hook.yaml"), "events:\n  Stop:\n    - hooks: [{type: command}]\n")
	writeFile(t, filepath.Join(lib, "hooks", "missing", "hook.yaml"), "events:\n  Stop:\n    - hooks: [{command: ./gone.sh}]\n")
	writeFile(t, filepath.Join(lib, "hooks", "broken", "hook.yaml"), "events: [\n")
	p := compute(t, cfg)
	wants := []string{"unknown event \"beforeShellExecution\"", "unknown tool \"nosuch\"", "overrides refers to unknown tool \"nope\"", "has no command", "./gone.sh, which does not exist", "parse failed"}
	for _, w := range wants {
		found := false
		for _, e := range p.RouteErrors {
			if strings.Contains(e, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("route error %q missing in %v", w, p.RouteErrors)
		}
	}
	if len(p.RouteErrors) != len(wants) {
		t.Errorf("route errors = %d, want %d: %v", len(p.RouteErrors), len(wants), p.RouteErrors)
	}
	if _, err := Execute(cfg, p); err == nil {
		t.Error("Execute must refuse while route errors exist")
	}
}

func TestHookTargetExcludesTool(t *testing.T) {
	cfg, lib := setupHooks(t, "error")
	writeFile(t, filepath.Join(lib, "hooks", "block-rm", "hook.yaml"), "target: [claude]\n"+strings.TrimPrefix(blockRM, "description: 擋下 rm -rf\n"))
	p := compute(t, cfg)
	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	cx := readJSON(t, filepath.Join(cfg.OutDir(), "hooks.codex.json"))
	if len(cx["hooks"].(map[string]interface{})) != 0 {
		t.Error("codex must not receive a hook targeting claude only")
	}
}

func TestHookRewriteCommand(t *testing.T) {
	abs := filepath.Join(string(filepath.Separator), "p", "hooks", "h")
	cases := map[string]string{
		"./x.sh":              filepath.Join(abs, "x.sh"),
		"python3 ./check.py":  "python3 " + filepath.Join(abs, "check.py"),
		"./a ./b":             filepath.Join(abs, "a") + " " + filepath.Join(abs, "b"),
		"npm test":            "npm test",
		"./sub/dir/run --v 1": filepath.Join(abs, "sub", "dir", "run") + " --v 1",
	}
	for in, want := range cases {
		if got := rewriteCommand(in, abs); got != want {
			t.Errorf("rewrite(%q) = %q, want %q", in, got, want)
		}
	}
	if got := rewriteCommand("./x", ""); got != "./x" {
		t.Errorf("empty dir must not rewrite, got %q", got)
	}
}

func TestHookRenameAndCollision(t *testing.T) {
	cfg, lib := setupHooks(t, "rename")
	flow := filepath.Join(cfg.BaseDir, ".flow")
	writeFile(t, filepath.Join(cfg.Path), strings.Replace(strings.Replace(hooksYAML, "%s", "rename", 1), "  - ../lib\n", "  - ../lib\n  - ./.flow\n", 1))
	cfg2, err := config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(flow, "hooks", "block-rm", "hook.yaml"), blockRM)
	writeFile(t, filepath.Join(flow, "hooks", "block-rm", "block-rm.sh"), "")
	_ = lib
	p := compute(t, cfg2)
	got := names(p, "hooks")
	if len(got) != 2 || got[0] != "block-rm-fromlib-lib" || got[1] != "block-rm-fromlib-flow" {
		t.Fatalf("renamed hooks = %v", got)
	}
	if _, err := Execute(cfg2, p); err != nil {
		t.Fatal(err)
	}
	ag := readJSON(t, filepath.Join(cfg2.OutDir(), "hooks.antigravity.json"))
	if len(ag) != 2 {
		t.Errorf("antigravity registry keys = %v", ag)
	}
	cl := readJSON(t, filepath.Join(cfg2.OutDir(), "hooks.claude.json"))
	if n := len(cl["hooks"].(map[string]interface{})["PreToolUse"].([]interface{})); n != 2 {
		t.Errorf("claude groups = %d, want 2", n)
	}
}

func TestHookConflictError(t *testing.T) {
	cfg, _ := setupHooks(t, "error")
	writeFile(t, cfg.Path, strings.Replace(strings.Replace(hooksYAML, "%s", "error", 1), "  - ../lib\n", "  - ../lib\n  - ./.flow\n", 1))
	cfg2, err := config.Load(cfg.Path)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(cfg.BaseDir, ".flow", "hooks", "block-rm", "hook.yaml"), blockRM)
	writeFile(t, filepath.Join(cfg.BaseDir, ".flow", "hooks", "block-rm", "block-rm.sh"), "")
	p := compute(t, cfg2)
	if len(p.Conflicts) != 1 || p.Conflicts[0].Category != "hooks" {
		t.Fatalf("conflicts = %v", p.Conflicts)
	}
}

func TestHookNameNormalized(t *testing.T) {
	cfg, lib := setupHooks(t, "error")
	writeFile(t, filepath.Join(lib, "hooks", "My_Hook", "hook.yaml"), "events:\n  Stop:\n    - hooks: [{command: ./x}]\n")
	writeFile(t, filepath.Join(lib, "hooks", "My_Hook", "x"), "")
	p := compute(t, cfg)
	for _, it := range p.Items {
		if it.Name == "My_Hook" {
			if it.OutName != "my-hook" || !strings.Contains(it.RouteNote, "normalized") {
				t.Errorf("OutName=%q note=%q", it.OutName, it.RouteNote)
			}
		}
	}
}

func TestCanonicalHashStable(t *testing.T) {
	a := map[string][]json.RawMessage{"PreToolUse": {json.RawMessage(`{"matcher":"Bash","hooks":[]}`), json.RawMessage(`{"matcher": "Edit"}`)}}
	b := map[string][]json.RawMessage{"PreToolUse": {json.RawMessage(`{ "matcher" : "Edit" }`), json.RawMessage(`{"matcher":"Bash","hooks":[]}`)}}
	if CanonicalHash(a) != CanonicalHash(b) {
		t.Error("hash must ignore formatting and group order within an event")
	}
	c := map[string][]json.RawMessage{"Stop": {json.RawMessage(`{"matcher":"Bash","hooks":[]}`)}}
	if CanonicalHash(a) == CanonicalHash(c) {
		t.Error("different events must hash differently")
	}
}

func TestLoadRegistryGroups(t *testing.T) {
	cfg, _ := setupHooks(t, "error")
	p := compute(t, cfg)
	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRegistryGroups(filepath.Join(cfg.OutDir(), "hooks.claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Order) != 1 || r.Order[0] != "PreToolUse" || len(r.Groups["PreToolUse"]) != 1 {
		t.Errorf("groups = %+v", r)
	}
}
