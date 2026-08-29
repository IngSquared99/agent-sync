package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Single-file items over MaxMDBytes are ignored with a stated reason; skill
// directories may carry large assets and are exempt.
func TestAcceptsRejectsOversizedMD(t *testing.T) {
	i18n.SetLang("en")
	dir := t.TempDir()
	big := filepath.Join(dir, "big.md")
	if err := os.WriteFile(big, make([]byte, MaxMDBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("rules", big, false); ok || !strings.Contains(reason, "MB") {
		t.Fatalf("oversized rule must be ignored with a size reason, got ok=%v reason=%q", ok, reason)
	}
	small := filepath.Join(dir, "small.md")
	if err := os.WriteFile(small, []byte("# ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("workflows", small, false); !ok {
		t.Fatalf("small file must be accepted, got %q", reason)
	}
	// skills: a big asset next to SKILL.md must not disqualify the skill
	skill := filepath.Join(dir, "myskill")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nname: myskill\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skill, "data.bin"), make([]byte, MaxMDBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, reason := Accepts("skills", skill, true); !ok {
		t.Fatalf("skill with a large asset must stay accepted, got %q", reason)
	}
}

// Only the top-level front-matter name is the skill's identity; a nested
// name field (e.g. under metadata:) must survive a rename untouched.
func TestRewriteSkillNameTopLevelOnly(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "SKILL.md")
	src := "---\nmetadata:\n  name: nested-keep\nname: original\n---\nbody\n"
	if err := os.WriteFile(md, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RewriteSkillName(md, "renamed"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(md)
	s := string(got)
	if !strings.Contains(s, "\nname: renamed\n") {
		t.Errorf("top-level name not rewritten:\n%s", s)
	}
	if !strings.Contains(s, "  name: nested-keep\n") {
		t.Errorf("nested name must stay untouched:\n%s", s)
	}
	if strings.Contains(s, "name: original") {
		t.Errorf("original top-level name still present:\n%s", s)
	}
}

// verifySrcUnchanged closes the copy-then-hash race: a fingerprint mismatch
// between the verbatim output and the source baseline aborts the build.
func TestVerifySrcUnchanged(t *testing.T) {
	i18n.SetLang("en")
	// single file: hashes must match exactly
	mi := &ManifestItem{SrcHash: "sha256:a", Outs: []OutEntry{{Hash: "sha256:b"}}}
	if err := verifySrcUnchanged(mi, Item{Category: "rules"}); err == nil {
		t.Fatal("mismatched single-file hash must abort the build")
	}
	mi.Outs[0].Hash = "sha256:a"
	if err := verifySrcUnchanged(mi, Item{Category: "rules"}); err != nil {
		t.Fatalf("matching hashes must pass: %v", err)
	}
	// renamed skill: SKILL.md is rewritten on purpose and is the only allowed diff
	mi2 := &ManifestItem{
		SrcFiles: map[string]string{"SKILL.md": "x", "a.txt": "y"},
		Outs:     []OutEntry{{Files: map[string]string{"SKILL.md": "z", "a.txt": "y"}}},
	}
	if err := verifySrcUnchanged(mi2, Item{Category: "skills", IsDir: true, Renamed: true}); err != nil {
		t.Fatalf("renamed skill's rewritten SKILL.md must be tolerated: %v", err)
	}
	if err := verifySrcUnchanged(mi2, Item{Category: "skills", IsDir: true}); err == nil {
		t.Fatal("non-renamed skill with a differing file must abort the build")
	}
	// derived outputs are never compared
	mi3 := &ManifestItem{SrcHash: "sha256:a", Outs: []OutEntry{{Hash: "sha256:b", Derived: true}}}
	if err := verifySrcUnchanged(mi3, Item{Category: "rules"}); err != nil {
		t.Fatalf("derived outputs are exempt: %v", err)
	}
}
