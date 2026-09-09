package config

import (
	"strings"
	"testing"

	"github.com/IngSquared99/agent-sync/i18n"
)

// Message assertions below compare the English source strings.
func init() { i18n.SetLang("en") }

// Config validation for the hooks category: one case per rule.

func TestHooksCategoryDefaults(t *testing.T) {
	cfg, err := load(t, baseYAML)
	if err != nil {
		t.Fatal(err)
	}
	h := cfg.Build.Categories["hooks"]
	if h.From != "hooks" || h.To != "hooks" {
		t.Fatalf("hooks category default = %+v", h)
	}
	if cfg.Version != 1 {
		t.Fatalf("version: 1 must still load as written, got %d", cfg.Version)
	}
}

func TestOnConflictHooksRequiredWithUpgradeHint(t *testing.T) {
	body := strings.Replace(baseYAML,
		"on_conflict: {rules: rename, skills: error, workflows: rename, hooks: error}",
		"on_conflict: {rules: rename, skills: error, workflows: rename}", 1)
	mustFail(t, body, "build.on_conflict.hooks is not set (new in v0.2.0)")
}

func TestRegistryNamesReserved(t *testing.T) {
	body := strings.Replace(baseYAML, "  out: .agsy\n",
		"  out: .agsy\n  categories: {hooks: {to: hooks.codex.json}}\n", 1)
	mustFail(t, body, "reserved for a derived hook registry file")
}

func TestRegistryLinkable(t *testing.T) {
	body := baseYAML + "  - dir: .codex\n    links: {hooks.json: hooks.codex.json}\n"
	if _, err := load(t, body); err != nil {
		t.Fatalf("link to registry must be accepted: %v", err)
	}
}

func TestRegistryLinkRequiresTool(t *testing.T) {
	body := baseYAML + "  - dir: .cursor\n    links: {hooks.json: hooks.cursor.json}\n"
	mustFail(t, body, `build.tools does not list "cursor"`)
}

func TestMergeAccepted(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, skills: skills}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	cfg, err := load(t, body)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mount[0].Merge["settings.json"] != "hooks.claude.json" {
		t.Fatalf("merge not loaded: %+v", cfg.Mount[0])
	}
	if !cfg.HookRegistryMounted("claude") || cfg.HookRegistryMounted("codex") {
		t.Fatal("HookRegistryMounted wrong")
	}
}

func TestMergeOnlyEntryIsEnough(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    merge: {settings.json: hooks.claude.json}\n", 1)
	if _, err := load(t, body); err != nil {
		t.Fatalf("a mount with only merge must be accepted: %v", err)
	}
}

func TestMergeTargetMustBeRegistry(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {settings.json: skills}\n", 1)
	mustFail(t, body, "only a hook registry of the claude / codex shape can be merged")
}

func TestMergeToolMustBeListed(t *testing.T) {
	body := strings.Replace(strings.Replace(baseYAML, "tools: [claude, codex]", "tools: [claude]", 1),
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {settings.json: hooks.codex.json}\n", 1)
	mustFail(t, body, `build.tools does not list "codex"`)
}

// Only registries of the nested shape can be read back by merge; the flat
// (cursor) and named (antigravity) ones are refused at config time rather
// than loading as empty and leaving the target silently idle.
func TestMergeRefusesNonNestedRegistry(t *testing.T) {
	for _, reg := range []string{"hooks.cursor.json", "hooks.antigravity.json"} {
		body := strings.Replace(baseYAML,
			"    links: {rules: rules, skills: skills}\n",
			"    links: {rules: rules}\n    merge: {settings.json: "+reg+"}\n", 1)
		mustFail(t, body, "only a hook registry of the claude / codex shape can be merged")
	}
}

// A merge target must lie inside the project even with outside_project:
// a shared user-level settings.json would be claimed by every project.
func TestMergeOutsideProjectRefused(t *testing.T) {
	outside := t.TempDir()
	body := strings.Replace(baseYAML,
		"  - dir: .claude\n    links: {rules: rules, skills: skills}\n",
		"  - dir: "+outside+"\n    outside_project: true\n    links: {rules: rules}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "merge targets must be inside the project")
	// links alone outside the project stay allowed with the opt-in
	body = strings.Replace(baseYAML,
		"  - dir: .claude\n    links: {rules: rules, skills: skills}\n",
		"  - dir: "+outside+"\n    outside_project: true\n    links: {rules: rules, skills: skills}\n", 1)
	if _, err := load(t, body); err != nil {
		t.Fatalf("links outside the project with outside_project must be accepted: %v", err)
	}
}

func TestMergeAndLinkSameName(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {settings.json: hooks.claude.json}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "both as a link and as a merge target")
}

func TestMergeNameNoSeparators(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules}\n    merge: {\"a/b.json\": hooks.claude.json}\n", 1)
	mustFail(t, body, "merge name")
}

func TestMergeMergedAcrossSameDir(t *testing.T) {
	body := baseYAML + "  - dir: ./.claude\n    merge: {settings.json: hooks.claude.json}\n" +
		"  - dir: .claude\n    merge: {settings.json: hooks.codex.json}\n"
	mustFail(t, body, "merge.settings.json more than once")
}

// One registry is consumed one way: a link and a merge of the same file
// would run the same hooks twice.
func TestRegistryLinkedAndMergedRefused(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, hooks.json: hooks.claude.json}\n    merge: {settings.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "a registry is either linked or merged")
}

// One registry is merged into one file only: two merge entries (in one
// mount or across mounts) naming the same registry would run its hooks
// twice.
func TestRegistryMergedTwiceRefused(t *testing.T) {
	body := strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, skills: skills}\n    merge: {settings.json: hooks.claude.json, settings.local.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "a registry is merged into one file only")
	body = strings.Replace(baseYAML,
		"    links: {rules: rules, skills: skills}\n",
		"    links: {rules: rules, skills: skills}\n    merge: {settings.json: hooks.claude.json}\n  - dir: .other\n    merge: {settings.json: hooks.claude.json}\n", 1)
	mustFail(t, body, "a registry is merged into one file only")
}

// A mount with neither links nor merge entries names both in its error.
func TestMountNeedsLinksOrMerge(t *testing.T) {
	body := strings.Replace(baseYAML, "    links: {rules: rules, skills: skills}\n", "    links: {}\n", 1)
	mustFail(t, body, "has neither links nor merge entries")
}
