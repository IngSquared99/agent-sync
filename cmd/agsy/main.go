// agent-sync (agsy): merges AI instruction files from multiple sources into a
// single build artifact, then mounts it into each tool.
// Dual entry: no args → interactive menu; with args → direct execution (§7).
package main

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"github.com/IngSquared99/agent-sync/adapters"

	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/prompt"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// Version info is injected by the build process: go build -ldflags "-X main.version=v1.0.0 ..."
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Adapter is a vendor mount preset (factory template for init, not a runtime dependency)
type Adapter struct {
	Name    string `yaml:"name"`
	Display string `yaml:"display"`
	Bucket  string `yaml:"bucket"`
	Mount   struct {
		Dir   string            `yaml:"dir"`
		Links map[string]string `yaml:"links"`
	} `yaml:"mount"`
}

func loadAdapters() ([]Adapter, error) {
	entries, err := adapters.FS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	var out []Adapter
	for _, e := range entries {
		raw, err := adapters.FS.ReadFile(e.Name())
		if err != nil {
			return nil, err
		}
		var a Adapter
		if err := yaml.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf(i18n.T("failed to parse adapter %s: %w"), e.Name(), err)
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func main() {
	code := run()
	// When launched by double-click on Windows, pause here — otherwise the
	// window closes before the result can be seen (§9)
	prompt.PauseIfDoubleClicked()
	os.Exit(code)
}

func run() int {
	args := os.Args[1:]
	// Global flag: --yes / -y answers y to every confirmation (for CI, scripts, git hooks).
	// Filter it out before parsing the subcommand so it isn't mistaken for a promote item name.
	var kept []string
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			prompt.AssumeYes = true
			continue
		}
		kept = append(kept, a)
	}
	args = kept
	if len(args) == 0 {
		return cmdMenu()
	}
	cmd := args[0]
	rest := args[1:]
	var code int
	switch cmd {
	case "init":
		code = cmdInit(rest)
	case "plan":
		code = cmdPlan()
	case "apply":
		code = cmdApply()
	case "status":
		code = cmdStatus(true)
	case "promote":
		code = cmdPromote(rest)
	case "clean":
		code = cmdClean()
	case "doctor":
		code = cmdDoctor()
	case "version", "--version", "-v":
		fmt.Printf("agsy %s (commit %s, built %s, %s, %s/%s)\n",
			version, commit, date, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "help", "--help", "-h":
		printHelp()
	default:
		fmt.Fprintf(os.Stderr, i18n.T("unknown command %q\n\n"), cmd)
		printHelp()
		code = 2
	}
	return code
}

func printHelp() {
	fmt.Println(strings.TrimSpace(i18n.T(`
agsy — merge and mount tool for multi-source AI instruction files (agent-sync)

Usage:
  agsy              interactive menu
  agsy init [sources...]  generate agsy.yaml; non-interactive runs pass sources as arguments (enters edit mode if it already exists)
  agsy doctor       environment health check, performs no actions
  agsy plan         preview the result of build and mount without writing anything
  agsy apply        pre-checks → confirm local changes → clear artifacts → build → mount
  agsy status       compare sources / artifacts / mounts and report gaps (exit 0=in sync 1=gaps found)
  agsy promote      write changes in the artifacts back to the sources
                    agsy promote <item>          single item
                    agsy promote <item> --to <source>  single item, rerouted
                    agsy promote --all           write back everything (each to its original source)
  agsy clean        remove mount links and build artifacts (uninstall)
  agsy version      tool version
  agsy help         this help text

Global flags:
  --yes, -y         treat every confirmation as y (CI / scripts and other non-interactive contexts)
                    when unset and nobody can answer, actions needing confirmation are cancelled, never forced
`)))
}

// loadConfig is the shared config loading (part of the pre-checks).
// Also works when run from a project subdirectory (FindUp searches upward);
// init is the exception — it only looks at the current directory.
func loadConfig() (*config.Config, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	p, ok := config.FindUp(wd)
	if !ok {
		return nil, fmt.Errorf(i18n.T("%s not found (searched up to the root directory; run agsy init first to create the config)"), config.FileName)
	}
	return config.Load(p)
}

func errExit(err error) int {
	fmt.Fprintln(os.Stderr, "✘", err)
	return 1
}
