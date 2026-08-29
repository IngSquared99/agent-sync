package build

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The scan (Accepts) rejects symlinks, but the user-confirmation window
// between Compute and Execute is unbounded: a rule swapped for a symlink in
// the meantime must be refused by the copy itself, never have its target
// content copied into the mounted output.
func TestExecuteRefusesFileSwappedForSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires extra privileges")
	}
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	p := compute(t, cfg)

	secret := filepath.Join(t.TempDir(), "secret.txt")
	writeFile(t, secret, "private key material\n")
	rule := filepath.Join(lib, "rules", "git-commit.md")
	if err := os.Remove(rule); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, rule); err != nil {
		t.Fatal(err)
	}

	if _, err := Execute(cfg, p); err == nil {
		t.Fatal("Execute must refuse to copy a file swapped for a symlink")
	}
}

// A UTF-8 BOM (some Windows editors prepend it) must not silently disable
// front-matter parsing: target routing has to keep working, and the BOM must
// not leak into the derived AGENTS.md.
func TestBOMDoesNotDisableFrontMatter(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "workflows", "bomflow.md"),
		"\ufeff---\ntarget: [claude]\n---\nbody\n")
	writeFile(t, filepath.Join(lib, "rules", "bomrule.md"), "\ufeffbom rule\n")
	p := compute(t, cfg)

	for _, it := range p.Items {
		if it.Category == "workflows" && it.Name == "bomflow.md" {
			if len(it.Tools) != 1 || it.Tools[0] != "claude" {
				t.Errorf("BOM disabled target routing: tools = %v", it.Tools)
			}
		}
	}

	if _, err := Execute(cfg, p); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(cfg.OutDir(), "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "\ufeff") {
		t.Error("BOM leaked into the derived AGENTS.md")
	}
	skill, err := os.ReadFile(filepath.Join(cfg.OutDir(), "skills", "bomflow", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(skill), "\ufeff") || strings.Contains(string(skill), "target:") {
		t.Error("derived skill must carry neither the BOM nor the target field")
	}
}

// An explicit disable-model-invocation: false is overridden by the derived
// skill; the override must be said out loud in the plan, never applied
// silently.
func TestDisableModelInvocationOverrideIsNoted(t *testing.T) {
	cfg, lib, _ := setupTwoSources(t, "rename", "error")
	writeFile(t, filepath.Join(lib, "workflows", "eager.md"),
		"---\ndisable-model-invocation: false\n---\nbody\n")
	p := compute(t, cfg)
	for _, it := range p.Items {
		if it.Category == "workflows" && it.Name == "eager.md" {
			if it.RouteNote == "" {
				t.Error("overriding disable-model-invocation: false must leave a route note")
			}
			return
		}
	}
	t.Fatal("eager.md not found in plan")
}

// A tampered manifest with a relative From must be rejected wholesale, like a
// non-local output path.
func TestLoadManifestRejectsRelativeFrom(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ManifestName),
		`{"version":1,"items":[{"category":"rules","name":"x.md","from":"../../etc/passwd","outs":[]}]}`)
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("a relative manifest From must be rejected")
	}
}
