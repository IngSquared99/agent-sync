package main

import (
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/state"
)

// Thin wrappers for tests: spare main_test from reassembling the build / state calls.
func loadManifestForTest(out string) (*build.Manifest, error) {
	return build.LoadManifest(out)
}

func collectForTest(cfg *config.Config, m *build.Manifest) (int, error) {
	r, err := state.Collect(cfg, m)
	if err != nil {
		return 0, err
	}
	return len(r.Locals), nil
}
