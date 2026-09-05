package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
)

// Vendor-view tests: after apply, every file is read through the mount paths
// each vendor documents, and its shape is checked against what that vendor
// accepts. Nothing here reads the output directory directly — a tool never
// does either.

// vendorFixture: two sources, every category, per-vendor differences.
func vendorFixture(t *testing.T) string {
	t.Helper()
	proj := t.TempDir()
	lib := filepath.Join(proj, "lib")
	// rules
	write(t, filepath.Join(lib, "rules", "security.md"), "# Security\nNever commit secrets.\n")
	write(t, filepath.Join(lib, "rules", "style.md"), "# Style\nTabs.\n")
	// skills
	write(t, filepath.Join(lib, "skills", "code-review", "SKILL.md"), "---\nname: code-review\ndescription: review code\n---\nbody\n")
	write(t, filepath.Join(lib, "skills", "code-review", "run.sh"), "#!/bin/sh\necho hi\n")
	os.Chmod(filepath.Join(lib, "skills", "code-review", "run.sh"), 0o755)
	// workflows: every target combination
	write(t, filepath.Join(lib, "workflows", "deploy.md"), "---\ntarget: [claude]\n---\n# Deploy\nsteps\n")
	write(t, filepath.Join(lib, "workflows", "standup.md"), "---\ntarget: [antigravity]\n---\n# Standup\nsteps\n")
	write(t, filepath.Join(lib, "workflows", "hotfix.md"), "---\ntarget: [claude, antigravity]\n---\n# Hotfix\nsteps\n")
	write(t, filepath.Join(lib, "workflows", "release-note.md"), "# Release note\nsteps\n")
	// hooks: command for all four (with per-vendor matcher), Stop for two
	// tools, an http handler only claude accepts, a prompt handler claude and
	// cursor accept, an mcp_tool handler claude and codex accept, an event
	// only antigravity has.
	write(t, filepath.Join(lib, "hooks", "block-rm", "hook.yaml"), `description: block destructive shell
events:
  PreToolUse:
    - matcher: Bash
      hooks:
        - command: ./block-rm.sh
          timeout: 10
      overrides:
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
`)
	write(t, filepath.Join(lib, "hooks", "block-rm", "block-rm.sh"), "#!/bin/sh\nexit 2\n")
	os.Chmod(filepath.Join(lib, "hooks", "block-rm", "block-rm.sh"), 0o755)
	write(t, filepath.Join(lib, "hooks", "tests-green", "hook.yaml"), `target: [claude, codex]
events:
  Stop:
    - hooks:
        - command: python3 ./check.py
`)
	write(t, filepath.Join(lib, "hooks", "tests-green", "check.py"), "import sys\nsys.exit(0)\n")
	write(t, filepath.Join(lib, "hooks", "mixed", "hook.yaml"), `events:
  PostToolUse:
    - matcher: Edit|Write
      hooks:
        - type: http
          url: http://localhost:8080/audit
        - type: prompt
          prompt: "Was this edit safe? $ARGUMENTS"
        - type: mcp_tool
          server: scanner
          tool: scan
          input: { path: "${tool_input.file_path}" }
        - command: ./log.sh
          statusMessage: my own message
  SessionStart:
    - hooks:
        - command: ./banner.sh
  PreInvocation:
    - hooks:
        - command: ./before-model.sh
`)
	for _, f := range []string{"log.sh", "banner.sh", "before-model.sh"} {
		write(t, filepath.Join(lib, "hooks", "mixed", f), "#!/bin/sh\nexit 0\n")
		os.Chmod(filepath.Join(lib, "hooks", "mixed", f), 0o755)
	}
	// a pre-existing Claude settings file with user content
	write(t, filepath.Join(proj, ".claude", "settings.json"), `{
  "permissions": {"allow": ["Bash(go test ./...)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Edit", "hooks": [{"type": "command", "command": ".claude/hooks/mine.sh"}]}
    ]
  }
}
`)
	return proj
}

func applyFixture(t *testing.T) string {
	t.Helper()
	proj := vendorFixture(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	t.Cleanup(func() { prompt.AssumeYes = false })
	if code := cmdInit([]string{"./lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	if code := cmdStatus(false); code != 0 {
		t.Fatal("status must be clean right after apply")
	}
	return proj
}

// ── helpers that read like a vendor ─────────────────────────────────

// readThrough reads a path the way a tool does: following links.
func readThrough(t *testing.T, path string) string {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("%s: not readable through the mount: %v", path, err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return string(raw)
}

func readJSONThrough(t *testing.T, path string) map[string]interface{} {
	t.Helper()
	m := map[string]interface{}{}
	if err := json.Unmarshal([]byte(readThrough(t, path)), &m); err != nil {
		t.Fatalf("%s: not a JSON object: %v", path, err)
	}
	return m
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir) // follows a directory link like a tool would
	if err != nil {
		t.Fatalf("%s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// frontMatterOf returns the YAML front matter lines of a markdown file.
func frontMatterOf(s string) string {
	if !strings.HasPrefix(s, "---\n") {
		return ""
	}
	rest := s[4:]
	i := strings.Index(rest, "\n---\n")
	if i < 0 {
		return ""
	}
	return rest[:i]
}

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// checkSkillsDir validates a skills directory as every vendor reads it:
// each entry is a directory with SKILL.md whose front-matter name equals the
// directory name; workflow-derived skills carry disable-model-invocation.
func checkSkillsDir(t *testing.T, dir string, wantWorkflowSkills []string) {
	t.Helper()
	names := listDir(t, dir)
	seen := map[string]bool{}
	for _, n := range names {
		seen[n] = true
		if !skillNameRe.MatchString(n) {
			t.Errorf("%s: skill name %q violates the Agent Skills spec", dir, n)
		}
		fm := frontMatterOf(readThrough(t, filepath.Join(dir, n, "SKILL.md")))
		if !strings.Contains(fm, "name: "+n+"\n") {
			t.Errorf("%s/%s: front-matter name must equal the directory name:\n%s", dir, n, fm)
		}
		if !strings.Contains(fm, "description: ") {
			t.Errorf("%s/%s: missing description", dir, n)
		}
	}
	for _, w := range wantWorkflowSkills {
		if !seen[w] {
			t.Errorf("%s: workflow-derived skill %q missing (have %v)", dir, w, names)
			continue
		}
		fm := frontMatterOf(readThrough(t, filepath.Join(dir, w, "SKILL.md")))
		if !strings.Contains(fm, "disable-model-invocation: true") {
			t.Errorf("%s/%s: workflow skill must carry disable-model-invocation: true", dir, w)
		}
		if strings.Contains(fm, "target:") {
			t.Errorf("%s/%s: agsy's target field must not leak into the skill", dir, w)
		}
	}
	if seen["run.sh"] {
		t.Error("skills dir must not contain loose files")
	}
}

// checkAgentsMD validates the root AGENTS.md as Codex / Antigravity / Cursor
// read it: one file, every rule present, in source order.
func checkAgentsMD(t *testing.T, proj string) {
	t.Helper()
	s := readThrough(t, filepath.Join(proj, "AGENTS.md"))
	for _, want := range []string{"Never commit secrets.", "Tabs."} {
		if !strings.Contains(s, want) {
			t.Errorf("AGENTS.md lacks rule content %q", want)
		}
	}
	if strings.Index(s, "Never commit secrets.") > strings.Index(s, "Tabs.") {
		t.Error("AGENTS.md must keep rule order (security.md before style.md)")
	}
}

// handlerCommand returns the command of a handler map and asserts it names
// an existing, executable file when it points into the output.
func checkCommand(t *testing.T, where string, h map[string]interface{}) {
	t.Helper()
	c, _ := h["command"].(string)
	if c == "" {
		t.Errorf("%s: command handler without command: %v", where, h)
		return
	}
	for _, tok := range strings.Fields(c) {
		if !strings.Contains(tok, ".agsy") {
			continue
		}
		if !filepath.IsAbs(tok) {
			t.Errorf("%s: command path %q is not absolute", where, tok)
		}
		fi, err := os.Stat(tok)
		if err != nil {
			t.Errorf("%s: command path %q does not exist", where, tok)
			continue
		}
		if runtime.GOOS != "windows" && strings.HasSuffix(tok, ".sh") && fi.Mode().Perm()&0o111 == 0 {
			t.Errorf("%s: script %q lost its executable bit", where, tok)
		}
	}
}

func groupsOf(t *testing.T, where string, v interface{}) []map[string]interface{} {
	t.Helper()
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("%s: expected an array, got %T", where, v)
	}
	var out []map[string]interface{}
	for _, g := range arr {
		m, ok := g.(map[string]interface{})
		if !ok {
			t.Fatalf("%s: expected objects in the array, got %T", where, g)
		}
		out = append(out, m)
	}
	return out
}

// checkNestedGroup validates one Claude / Codex style matcher group.
func checkNestedGroup(t *testing.T, where string, g map[string]interface{}, allowedTypes map[string]bool) []map[string]interface{} {
	t.Helper()
	if m, has := g["matcher"]; has {
		if _, ok := m.(string); !ok {
			t.Errorf("%s: matcher must be a string", where)
		}
	}
	hs := groupsOf(t, where+".hooks", g["hooks"])
	for _, h := range hs {
		typ, _ := h["type"].(string)
		if typ == "" {
			typ = "command"
		}
		if !allowedTypes[typ] {
			t.Errorf("%s: handler type %q is not accepted by this vendor", where, typ)
		}
		if typ == "command" {
			checkCommand(t, where, h)
		}
	}
	return hs
}

// ── Claude Code ─────────────────────────────────────────────────────

func TestVendorViewClaudeCode(t *testing.T) {
	proj := applyFixture(t)
	// rules: per-file directory
	rules := listDir(t, filepath.Join(proj, ".claude", "rules"))
	if strings.Join(rules, ",") != "security.md,style.md" {
		t.Errorf(".claude/rules = %v", rules)
	}
	if !strings.Contains(readThrough(t, filepath.Join(proj, ".claude", "rules", "security.md")), "Never commit secrets.") {
		t.Error("rule content not readable through .claude/rules")
	}
	// skills: real skills + every workflow with a non-antigravity target
	checkSkillsDir(t, filepath.Join(proj, ".claude", "skills"), []string{"deploy", "hotfix", "release-note"})
	if _, err := os.Stat(filepath.Join(proj, ".claude", "skills", "standup")); err == nil {
		t.Error("standup targets antigravity only; Claude must not see it")
	}
	// hooks: settings.json, user content intact, agsy groups valid
	settings := readJSONThrough(t, filepath.Join(proj, ".claude", "settings.json"))
	if _, ok := settings["permissions"]; !ok {
		t.Error("user permissions key lost")
	}
	hooks, ok := settings["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("settings.hooks missing")
	}
	allowed := map[string]bool{"command": true, "http": true, "mcp_tool": true, "prompt": true, "agent": true}
	for ev := range hooks {
		switch ev {
		case "PreToolUse", "PostToolUse", "Stop", "SessionStart":
		default:
			t.Errorf("Claude registry carries event %q which Claude Code does not document", ev)
		}
	}
	if _, has := hooks["PreInvocation"]; has {
		t.Error("PreInvocation is Antigravity-only and must not reach Claude")
	}
	pre := groupsOf(t, "PreToolUse", hooks["PreToolUse"])
	if len(pre) != 2 {
		t.Fatalf("PreToolUse groups = %d (user Edit group + agsy Bash group)", len(pre))
	}
	if pre[0]["matcher"] != "Edit" {
		t.Error("user group must stay first")
	}
	if pre[1]["matcher"] != "Bash" {
		t.Errorf("agsy group matcher = %v (Claude names the shell tool Bash)", pre[1]["matcher"])
	}
	for _, g := range pre {
		checkNestedGroup(t, "PreToolUse", g, allowed)
	}
	post := groupsOf(t, "PostToolUse", hooks["PostToolUse"])
	hs := checkNestedGroup(t, "PostToolUse", post[0], allowed)
	if len(hs) != 4 {
		t.Errorf("Claude accepts every handler type; got %d handlers", len(hs))
	}
	types := map[string]bool{}
	for _, h := range hs {
		types[h["type"].(string)] = true
		if h["type"] == "command" && h["statusMessage"] != "my own message" {
			t.Errorf("user statusMessage must be preserved: %v", h)
		}
		if h["type"] == "http" && h["statusMessage"] != "agsy:mixed" {
			t.Errorf("non-command handler must carry the owner mark: %v", h)
		}
		if h["type"] == "mcp_tool" {
			in, _ := h["input"].(map[string]interface{})
			if in["path"] != "${tool_input.file_path}" {
				t.Errorf("mcp_tool input must pass through as an object: %v", h["input"])
			}
		}
	}
	for _, want := range []string{"http", "prompt", "mcp_tool", "command"} {
		if !types[want] {
			t.Errorf("Claude registry lacks handler type %s", want)
		}
	}
	stop := groupsOf(t, "Stop", hooks["Stop"])
	c, _ := checkNestedGroup(t, "Stop", stop[0], allowed)[0]["command"].(string)
	if !strings.HasPrefix(c, "python3 ") || !strings.Contains(c, string(filepath.Separator)+"check.py") {
		t.Errorf("interpreter form must be kept with the path rewritten: %q", c)
	}
}

// ── Codex ───────────────────────────────────────────────────────────

func TestVendorViewCodex(t *testing.T) {
	proj := applyFixture(t)
	checkAgentsMD(t, proj)
	checkSkillsDir(t, filepath.Join(proj, ".agents", "skills"), []string{"deploy", "hotfix", "release-note"})
	reg := readJSONThrough(t, filepath.Join(proj, ".codex", "hooks.json"))
	hooks, ok := reg["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("codex hooks.json must have a hooks object")
	}
	allowed := map[string]bool{"command": true, "mcp_tool": true}
	for ev, v := range hooks {
		switch ev {
		case "PreToolUse", "PostToolUse", "Stop", "SessionStart":
		default:
			t.Errorf("Codex registry carries event %q Codex does not document", ev)
		}
		for _, g := range groupsOf(t, ev, v) {
			checkNestedGroup(t, ev, g, allowed)
		}
	}
	post := groupsOf(t, "PostToolUse", hooks["PostToolUse"])
	hs := groupsOf(t, "PostToolUse.hooks", post[0]["hooks"])
	if len(hs) != 2 {
		t.Errorf("Codex keeps mcp_tool and command only; got %d", len(hs))
	}
	if pre := groupsOf(t, "PreToolUse", hooks["PreToolUse"]); pre[0]["matcher"] != "Bash" {
		t.Errorf("Codex shell tool is Bash, got %v", pre[0]["matcher"])
	}
	if _, has := hooks["Stop"]; !has {
		t.Error("tests-green targets codex; Stop must be present")
	}
}

// ── Antigravity ─────────────────────────────────────────────────────

func TestVendorViewAntigravity(t *testing.T) {
	proj := applyFixture(t)
	checkAgentsMD(t, proj)
	checkSkillsDir(t, filepath.Join(proj, ".agents", "skills"), []string{"hotfix", "release-note"})
	// workflows directory: a stub (or full content) for every antigravity target
	wf := listDir(t, filepath.Join(proj, ".agents", "workflows"))
	if strings.Join(wf, ",") != "hotfix.md,release-note.md,standup.md" {
		t.Errorf(".agents/workflows = %v", wf)
	}
	stub := readThrough(t, filepath.Join(proj, ".agents", "workflows", "hotfix.md"))
	if !strings.Contains(stub, "skills/hotfix/SKILL.md") {
		t.Errorf("hotfix stub must point at the derived skill:\n%s", stub)
	}
	full := readThrough(t, filepath.Join(proj, ".agents", "workflows", "standup.md"))
	if !strings.Contains(full, "steps") || strings.Contains(full, "target:") {
		t.Errorf("antigravity-only workflow must arrive verbatim without target:\n%s", full)
	}
	// hooks: hook name → { enabled, Event: [...] }
	reg := readJSONThrough(t, filepath.Join(proj, ".agents", "hooks.json"))
	if _, has := reg["hooks"]; has {
		t.Fatal("Antigravity registry must not use a top-level hooks key")
	}
	if _, has := reg["tests-green"]; has {
		t.Error("tests-green targets claude+codex only")
	}
	for name, v := range reg {
		entry, ok := v.(map[string]interface{})
		if !ok || entry["enabled"] != true {
			t.Fatalf("%s: expected {enabled: true, …}, got %v", name, v)
		}
		for ev, gv := range entry {
			if ev == "enabled" {
				continue
			}
			switch ev {
			case "PreToolUse", "PostToolUse", "PreInvocation", "PostInvocation", "Stop":
			default:
				t.Errorf("%s: event %q is not an Antigravity hook", name, ev)
			}
			for _, g := range groupsOf(t, name+"."+ev, gv) {
				if _, has := g["matcher"]; has && ev != "PreToolUse" && ev != "PostToolUse" {
					t.Errorf("%s.%s: matcher only applies to Pre/PostToolUse", name, ev)
				}
				for _, h := range checkNestedGroup(t, name+"."+ev, g, map[string]bool{"command": true}) {
					if h["type"] != "command" {
						t.Errorf("%s: Antigravity accepts command handlers only, got %v", name, h["type"])
					}
				}
			}
		}
	}
	br := reg["block-rm"].(map[string]interface{})
	if g := groupsOf(t, "block-rm.PreToolUse", br["PreToolUse"]); g[0]["matcher"] != "run_command" {
		t.Errorf("Antigravity shell tool is run_command, got %v", g[0]["matcher"])
	}
	mixed := reg["mixed"].(map[string]interface{})
	if _, has := mixed["SessionStart"]; has {
		t.Error("Antigravity has no SessionStart")
	}
	if _, has := mixed["PreInvocation"]; !has {
		t.Error("PreInvocation is an Antigravity event and must be present")
	}
	if hs := groupsOf(t, "mixed.PostToolUse.hooks", groupsOf(t, "mixed.PostToolUse", mixed["PostToolUse"])[0]["hooks"]); len(hs) != 1 {
		t.Errorf("only the command handler survives for Antigravity, got %d", len(hs))
	}
}

// ── Cursor ──────────────────────────────────────────────────────────

func TestVendorViewCursor(t *testing.T) {
	proj := applyFixture(t)
	checkAgentsMD(t, proj)
	checkSkillsDir(t, filepath.Join(proj, ".agents", "skills"), []string{"deploy", "hotfix", "release-note"})
	reg := readJSONThrough(t, filepath.Join(proj, ".cursor", "hooks.json"))
	if reg["version"] != float64(1) {
		t.Errorf("cursor hooks.json version = %v", reg["version"])
	}
	hooks, ok := reg["hooks"].(map[string]interface{})
	if !ok {
		t.Fatal("cursor hooks.json must have a hooks object")
	}
	cursorEvents := map[string]bool{"preToolUse": true, "postToolUse": true, "stop": true, "sessionStart": true, "sessionEnd": true,
		"beforeSubmitPrompt": true, "subagentStart": true, "subagentStop": true, "preCompact": true, "postToolUseFailure": true}
	for ev, v := range hooks {
		if !cursorEvents[ev] {
			t.Errorf("Cursor registry carries event %q Cursor does not document", ev)
		}
		for _, h := range groupsOf(t, ev, v) {
			if _, has := h["hooks"]; has {
				t.Errorf("%s: Cursor handlers are flat; no inner hooks array", ev)
			}
			typ, _ := h["type"].(string)
			switch typ {
			case "":
				checkCommand(t, ev, h)
			case "prompt":
				if _, ok := h["prompt"].(string); !ok {
					t.Errorf("%s: prompt handler without prompt", ev)
				}
			default:
				t.Errorf("%s: Cursor accepts command (implicit) or prompt, got %q", ev, typ)
			}
			for k := range h {
				switch k {
				case "command", "matcher", "timeout", "failClosed", "loop_limit", "type", "prompt":
				default:
					t.Errorf("%s: field %q is not a Cursor handler field", ev, k)
				}
			}
		}
	}
	pre := groupsOf(t, "preToolUse", hooks["preToolUse"])
	if pre[0]["matcher"] != "Shell" {
		t.Errorf("Cursor shell tool is Shell, got %v", pre[0]["matcher"])
	}
	if _, has := hooks["stop"]; has {
		t.Error("tests-green targets claude+codex only; Cursor must not get stop")
	}
	post := groupsOf(t, "postToolUse", hooks["postToolUse"])
	if len(post) != 2 {
		t.Errorf("Cursor keeps prompt and command handlers, got %d", len(post))
	}
	if _, has := hooks["PreToolUse"]; has {
		t.Error("Cursor event names are camelCase")
	}
}

// ── mount plumbing every vendor relies on ───────────────────────────

func TestVendorViewLinksResolveIntoOutput(t *testing.T) {
	proj := applyFixture(t)
	links := []string{"AGENTS.md", ".claude/rules", ".claude/skills", ".agents/skills", ".agents/workflows",
		".agents/hooks.json", ".codex/hooks.json", ".cursor/hooks.json"}
	for _, l := range links {
		p := filepath.Join(proj, l)
		fi, err := os.Lstat(p)
		if err != nil {
			t.Errorf("%s missing", l)
			continue
		}
		if runtime.GOOS != "windows" && fi.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s should be a symlink", l)
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Errorf("%s: broken link: %v", l, err)
			continue
		}
		if !strings.HasPrefix(target, filepath.Join(proj, ".agsy")+string(filepath.Separator)) {
			t.Errorf("%s resolves to %s, outside the output", l, target)
		}
	}
	// settings.json is a real file, never a link
	if fi, _ := os.Lstat(filepath.Join(proj, ".claude", "settings.json")); fi.Mode()&os.ModeSymlink != 0 {
		t.Error("settings.json must be a regular file")
	}
	// the hooks scripts directory is not mounted anywhere
	for _, d := range []string{".claude/hooks", ".codex/hooks", ".cursor/hooks", ".agents/hooks"} {
		if _, err := os.Lstat(filepath.Join(proj, d)); err == nil {
			t.Errorf("%s must not exist: registries point straight into the output", d)
		}
	}
}
