package build

import (
	"path/filepath"
	"testing"

	"github.com/IngSquared99/agent-sync/internal/config"
)

// RemoveOut is the second line of defense behind config validation: even a
// hand-built Config pointing out inside a source must never delete it.
func TestRemoveOutRefusesOutInsideSource(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Sources: []string{"./a-lib"},
		Build:   config.BuildCfg{Out: filepath.Join("a-lib", "rules")},
		BaseDir: dir,
	}
	if err := RemoveOut(cfg); err == nil {
		t.Fatal("RemoveOut must refuse an output directory nested inside a source")
	}
}
