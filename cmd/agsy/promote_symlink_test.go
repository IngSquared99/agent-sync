package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// The output directory is the layer mounted AI tools can write to. The build
// direction already refuses symbolic links (Accepts / copyDir); promote must
// refuse them as well: otherwise a single link in .agsy/ pointing at an
// external file (e.g. a private key) would have its content written back as
// a real source file, then built and mounted for every tool on the next
// apply — bypassing the whole build-side defense.

// setupPromoteProject creates a project, runs apply once, and returns the
// project path plus the path of an external "secret" file.
func setupPromoteProject(t *testing.T) (proj, secret string) {
	t.Helper()
	proj = newProject(t)
	chdir(t, proj)
	prompt.AssumeYes = true
	t.Cleanup(func() { prompt.AssumeYes = false })
	if code := cmdInit([]string{"./repo-ai-lib"}); code != 0 {
		t.Fatal("init failed")
	}
	if code := cmdApply(); code != 0 {
		t.Fatal("apply failed")
	}
	secret = filepath.Join(t.TempDir(), "id_rsa")
	write(t, secret, "SECRET-KEY\n")
	return proj, secret
}

// Single-file item: the artifact copy rules/x.md is replaced by a symlink.
func TestPromoteRefusesSymlinkFileOnArtifactSide(t *testing.T) {
	proj, secret := setupPromoteProject(t)
	outCopy := filepath.Join(proj, ".agsy", "rules", "python-style.md")
	if err := os.Remove(outCopy); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, outCopy); err != nil {
		t.Skip("symlinks not supported here:", err)
	}

	// status must list it as a local change and flag the symbolic link
	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadManifestForTest(cfg.OutDir())
	if err != nil {
		t.Fatal(err)
	}
	rep, err := state.Collect(cfg, m)
	if err != nil {
		t.Fatal(err)
	}
	var found *state.LocalChange
	for i := range rep.Locals {
		if rep.Locals[i].Item.Name == "python-style.md" {
			found = &rep.Locals[i]
		}
	}
	if found == nil {
		t.Fatal("status did not list the symlinked artifact as a local change")
	}
	if len(found.Symlinks) == 0 {
		t.Fatal("status did not flag the symbolic link on the artifact side")
	}

	if code := cmdPromote([]string{"rules/python-style.md"}); code == 0 {
		t.Fatal("promote should refuse an artifact that is a symbolic link")
	}
	src, err := os.ReadFile(filepath.Join(proj, "repo-ai-lib", "rules", "python-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(src) == "SECRET-KEY\n" {
		t.Fatal("promote wrote the symlink target's content into the source")
	}
}

// Directory item: a symlink is dropped inside the skill directory. --all must
// block it too, without affecting the other items.
func TestPromoteRefusesSymlinkInsideSkillOnArtifactSide(t *testing.T) {
	proj, secret := setupPromoteProject(t)
	skillOut := filepath.Join(proj, ".agsy", "skills", "api-doc")
	if _, err := os.Stat(skillOut); err != nil {
		t.Skip("fixture has no api-doc skill:", err)
	}
	if err := os.Symlink(secret, filepath.Join(skillOut, "notes.md")); err != nil {
		t.Skip("symlinks not supported here:", err)
	}

	if code := cmdPromote([]string{"--all"}); code == 0 {
		t.Fatal("promote --all should report failure for the skill containing a symlink")
	}
	leaked := filepath.Join(proj, "repo-ai-lib", "skills", "api-doc", "notes.md")
	if _, err := os.Lstat(leaked); err == nil {
		t.Fatal("promote copied a symlink (or its content) into the source skill")
	}
}

// Last line of defense: even if the guard were bypassed, writeBack itself
// must refuse to copy symbolic links.
func TestWriteBackRejectsSymlinks(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	write(t, secret, "SECRET-KEY\n")

	// single-file source is a link
	link := filepath.Join(dir, "link.md")
	if err := os.Symlink(secret, link); err != nil {
		t.Skip("symlinks not supported here:", err)
	}
	dest := filepath.Join(dir, "dest.md")
	if err := writeBack(link, dest); err == nil {
		t.Fatal("writeBack should refuse a symlink source file")
	}
	if _, err := os.Stat(dest); err == nil {
		t.Fatal("writeBack created the destination from a symlink")
	}

	// directory source contains a link
	srcDir := filepath.Join(dir, "skill")
	write(t, filepath.Join(srcDir, "SKILL.md"), "---\nname: skill\n---\n")
	if err := os.Symlink(secret, filepath.Join(srcDir, "inner")); err != nil {
		t.Fatal(err)
	}
	destDir := filepath.Join(dir, "skill-dest")
	if err := writeBack(srcDir, destDir); err == nil {
		t.Fatal("writeBack should refuse a directory containing a symlink")
	}
	if _, err := os.Stat(destDir); err == nil {
		t.Fatal("writeBack left a destination directory behind after refusing")
	}
}
