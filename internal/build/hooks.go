// Hooks category: a hook is a directory holding hook.yaml plus its scripts.
// Build copies the directory verbatim and writes one registry per tool in
// build.tools (hooks.<tool>.json at the output root).
//
//   - hook.yaml declares events under generic names (Claude Code / Codex
//     naming).
//   - Each tool's dialect maps event names, lists supported handler types
//     and selects the registry shape.
//   - Events or types a tool lacks are skipped and recorded as RouteNotes for
//     plan to list.
//   - ./ paths in command handlers are rewritten to absolute paths into the
//     output, identically for every tool.
package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// HookFile is the declaration file every hook directory must contain.
const HookFile = "hook.yaml"

// OwnerMark prefixes the statusMessage of every handler written for a
// merged dialect (Claude Code); a statusMessage given in hook.yaml follows
// the mark (see markStatus). mount.ownsGroup identifies agsy groups by this
// mark or by a command path into the output hooks directory; the mark is
// path-independent.
const OwnerMark = "agsy:"

// HookSpec is the parsed hook.yaml.
type HookSpec struct {
	Description string                 `yaml:"description"`
	Target      interface{}            `yaml:"target"` // string or list; same semantics as workflows
	Events      map[string][]HookGroup `yaml:"events"`
}

// HookGroup is one matcher group under an event.
type HookGroup struct {
	Matcher   string                   `yaml:"matcher"`
	Hooks     []map[string]interface{} `yaml:"hooks"`
	Overrides map[string]*HookOverride `yaml:"overrides"`
}

// HookOverride is a per-tool override of a group: matcher and per-index
// handler fields.
type HookOverride struct {
	Matcher *string                  `yaml:"matcher"`
	Hooks   []map[string]interface{} `yaml:"hooks"`
}

// genericEvents is the closed set of event names hook.yaml may use, in the
// order registries list them. It is the union of every dialect's events so
// "every tool, unless the vendor has no such hook" holds literally.
var genericEvents = []string{
	"PreToolUse", "PostToolUse", "Stop",
	"SessionStart", "SessionEnd", "UserPromptSubmit", "PermissionRequest",
	"SubagentStart", "SubagentStop", "PreCompact", "PostCompact",
	"PostToolUseFailure", "StopFailure", "Interrupt",
	"PreInvocation", "PostInvocation",
}

type hookShape int

const (
	shapeNested hookShape = iota // {"hooks": {Event: [{matcher, hooks:[…]}]}}  (claude, codex)
	shapeNamed                   // {hookName: {enabled, Event: [{matcher, hooks:[…]}]}} (antigravity)
	shapeFlat                    // {"version":1, "hooks": {event: [{command, matcher, …}]}} (cursor)
)

// dialect is one tool's hook vocabulary, per the vendor's documentation.
type dialect struct {
	events     map[string]string // generic → native; absent = the vendor has no such hook
	types      map[string]bool   // supported handler types
	shape      hookShape
	matcherOn  map[string]bool // named shape: events that honor a matcher (nil = all)
	flatFields map[string]bool // flat shape: handler fields carried over besides command/matcher
	ownerMark  bool            // registry is merged into a user file: every handler gets OwnerMark
}

func same(names ...string) map[string]string {
	m := map[string]string{}
	for _, n := range names {
		m[n] = n
	}
	return m
}

func set(names ...string) map[string]bool {
	m := map[string]bool{}
	for _, n := range names {
		m[n] = true
	}
	return m
}

var dialects = map[string]dialect{
	"claude": {
		events: same("PreToolUse", "PostToolUse", "Stop", "SessionStart", "SessionEnd",
			"UserPromptSubmit", "PermissionRequest", "SubagentStart", "SubagentStop",
			"PreCompact", "PostCompact", "PostToolUseFailure", "StopFailure"),
		types:     set("command", "http", "mcp_tool", "prompt", "agent"),
		shape:     shapeNested,
		ownerMark: true,
	},
	"codex": {
		events: same("PreToolUse", "PostToolUse", "Stop", "SessionStart", "SessionEnd",
			"UserPromptSubmit", "PermissionRequest", "SubagentStart", "SubagentStop",
			"PreCompact", "PostCompact", "Interrupt"),
		types: set("command", "mcp_tool"),
		shape: shapeNested,
	},
	"antigravity": {
		events:    same("PreToolUse", "PostToolUse", "Stop", "PreInvocation", "PostInvocation"),
		types:     set("command"),
		shape:     shapeNamed,
		matcherOn: set("PreToolUse", "PostToolUse"),
	},
	"cursor": {
		events: map[string]string{
			"PreToolUse": "preToolUse", "PostToolUse": "postToolUse", "Stop": "stop",
			"SessionStart": "sessionStart", "SessionEnd": "sessionEnd",
			"UserPromptSubmit": "beforeSubmitPrompt",
			"SubagentStart":    "subagentStart", "SubagentStop": "subagentStop",
			"PreCompact": "preCompact", "PostToolUseFailure": "postToolUseFailure",
		},
		types:      set("command", "prompt"),
		shape:      shapeFlat,
		flatFields: set("timeout", "failClosed", "loop_limit", "prompt"),
	},
}

// HasHookDialect reports whether agsy knows how to write a registry for tool.
func HasHookDialect(tool string) bool {
	_, ok := dialects[tool]
	return ok
}

// isGenericEvent reports whether name is an allowed hook.yaml event.
func isGenericEvent(name string) bool {
	for _, e := range genericEvents {
		if e == name {
			return true
		}
	}
	return false
}

// parseHookSpec reads and validates a hook.yaml. Structural problems are
// returned as one error listing every finding, so plan shows them at once.
func parseHookSpec(path string) (*HookSpec, error) {
	raw, err := readSourceFile(path)
	if err != nil {
		return nil, err
	}
	spec := &HookSpec{}
	if err := yaml.Unmarshal(bytes.TrimPrefix(raw, []byte(bomPrefix)), spec); err != nil {
		return nil, fmt.Errorf(i18n.T("hook.yaml parse failed: %w"), err)
	}
	var problems []string
	if len(spec.Events) == 0 {
		problems = append(problems, i18n.T("events is empty"))
	}
	// Events are visited in a fixed order so the problem list is stable.
	evNames := make([]string, 0, len(spec.Events))
	for ev := range spec.Events {
		evNames = append(evNames, ev)
	}
	sort.Strings(evNames)
	for _, ev := range evNames {
		groups := spec.Events[ev]
		if !isGenericEvent(ev) {
			problems = append(problems, fmt.Sprintf(i18n.T("unknown event %q, valid values: %v"), ev, genericEvents))
			continue
		}
		if len(groups) == 0 {
			problems = append(problems, fmt.Sprintf(i18n.T("event %s has no handlers"), ev))
		}
		for gi, g := range groups {
			if len(g.Hooks) == 0 {
				problems = append(problems, fmt.Sprintf(i18n.T("group %d of %s has no handlers"), gi+1, ev))
			}
			for hi, h := range g.Hooks {
				spec.Events[ev][gi].Hooks[hi] = normalizeYAML(h).(map[string]interface{})
				h = spec.Events[ev][gi].Hooks[hi]
				if handlerType(h) == "command" {
					c, _ := h["command"].(string)
					if strings.TrimSpace(c) == "" {
						problems = append(problems, fmt.Sprintf(i18n.T("handler %d of %s has no command"), hi+1, ev))
					} else if hasQuotedLocalPath(c) {
						problems = append(problems, fmt.Sprintf(i18n.T("handler %d of %s quotes a ./ path; write it without quotes, build adds them when the rewritten path needs quoting"), hi+1, ev))
					}
				}
			}
			// Overrides are validated on the merged handler (group handler +
			// override fields), the view translateHook renders: an index past
			// the group's handlers and a command emptied by the override are
			// both errors.
			ovTools := make([]string, 0, len(g.Overrides))
			for tool := range g.Overrides {
				ovTools = append(ovTools, tool)
			}
			sort.Strings(ovTools)
			for _, tool := range ovTools {
				ov := g.Overrides[tool]
				if ov == nil {
					continue
				}
				if len(ov.Hooks) > len(g.Hooks) {
					problems = append(problems, fmt.Sprintf(i18n.T("overrides.%s of %s lists %d handlers, but the group has only %d"), tool, ev, len(ov.Hooks), len(g.Hooks)))
				}
				for i, h := range ov.Hooks {
					if h == nil {
						continue
					}
					ov.Hooks[i] = normalizeYAML(h).(map[string]interface{})
					if i >= len(g.Hooks) {
						continue
					}
					merged := cloneHandler(g.Hooks[i])
					for k, v := range ov.Hooks[i] {
						merged[k] = v
					}
					if handlerType(merged) != "command" {
						continue
					}
					c, _ := merged["command"].(string)
					if strings.TrimSpace(c) == "" {
						problems = append(problems, fmt.Sprintf(i18n.T("overrides.%s leaves handler %d of %s without a command"), tool, i+1, ev))
					} else if hasQuotedLocalPath(c) {
						problems = append(problems, fmt.Sprintf(i18n.T("overrides.%s: handler %d of %s quotes a ./ path; write it without quotes, build adds them when the rewritten path needs quoting"), tool, i+1, ev))
					}
				}
			}
		}
	}
	if len(problems) > 0 {
		return spec, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return spec, nil
}

// handlerType returns the handler's type, defaulting to command.
func handlerType(h map[string]interface{}) string {
	if t, ok := h["type"].(string); ok && t != "" {
		return t
	}
	return "command"
}

// hasQuotedLocalPath reports whether quoting changes which ./ paths a
// command names. rewriteCommand rewrites whitespace-separated tokens and
// splitCommand (ownership, existence checks) honors quotes; a command where
// the two disagree — "./x", 'a ./x', sh -c 'echo ./x' — would be rewritten
// into a broken command line, so parseHookSpec rejects it (build quotes
// rewritten paths itself).
func hasQuotedLocalPath(cmd string) bool {
	var plain []string
	for _, tok := range strings.Fields(cmd) {
		if strings.HasPrefix(tok, "./") {
			plain = append(plain, tok)
		}
	}
	var quoted []string
	for _, tok := range splitCommand(cmd) {
		if strings.HasPrefix(tok, "./") {
			quoted = append(quoted, tok)
		}
	}
	if len(plain) != len(quoted) {
		return true
	}
	for i := range plain {
		if plain[i] != quoted[i] {
			return true
		}
	}
	return false
}

// normalizeYAML converts map[interface{}]interface{} (as the YAML decoder may
// produce for nested objects) into map[string]interface{} recursively so the
// value can be JSON-encoded.
func normalizeYAML(v interface{}) interface{} {
	switch x := v.(type) {
	case map[interface{}]interface{}:
		m := map[string]interface{}{}
		for k, val := range x {
			m[fmt.Sprint(k)] = normalizeYAML(val)
		}
		return m
	case map[string]interface{}:
		m := map[string]interface{}{}
		for k, val := range x {
			m[k] = normalizeYAML(val)
		}
		return m
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, val := range x {
			out[i] = normalizeYAML(val)
		}
		return out
	default:
		return v
	}
}

// specTargets resolves the target field like workflows do: absent = every
// tool in build.tools. Returns the targets, the names not in build.tools,
// and whether the field had a shape other than a string or a list of
// strings (reported, not treated as absent).
func specTargets(cfg *config.Config, spec *HookSpec) (targets, bad []string, malformed bool) {
	targets, malformed = TargetList(spec.Target)
	if malformed {
		return nil, nil, true
	}
	if len(targets) == 0 {
		return append([]string{}, cfg.Build.Tools...), nil, false
	}
	for _, t := range targets {
		if !cfg.HasTool(t) {
			bad = append(bad, t)
		}
	}
	return targets, bad, false
}

// TargetList reads a target field: a string or a list of strings. Anything
// else (a number, a map, a list with a non-string entry) is malformed; nil
// means absent.
func TargetList(v interface{}) (targets []string, malformed bool) {
	switch x := v.(type) {
	case nil:
		return nil, false
	case string:
		return []string{x}, false
	case []interface{}:
		for _, e := range x {
			s, ok := e.(string)
			if !ok {
				return nil, true
			}
			targets = append(targets, s)
		}
		return targets, false
	default:
		return nil, true
	}
}

// resolveHooks parses every hook, validates targets and overrides, and
// decides which tools each hook reaches. Problems that make a hook
// untranslatable are RouteErrors (apply refuses); vendor gaps are RouteNotes
// (plan lists them, the hook still ships where it can).
func resolveHooks(cfg *config.Config, p *Plan) {
	for i := range p.Items {
		it := &p.Items[i]
		if it.Category != "hooks" {
			continue
		}
		spec, err := parseHookSpec(filepath.Join(it.From, HookFile))
		if err != nil {
			p.RouteErrors = append(p.RouteErrors, fmt.Sprintf("%s: %v", it.From, err))
			continue
		}
		// Every problem of a hook is listed, like parseHookSpec's findings.
		var errs []string
		targets, bad, malformed := specTargets(cfg, spec)
		if malformed {
			errs = append(errs, fmt.Sprintf(i18n.T("%s: %s must be a tool name or a list of tool names"), it.Name, TargetField))
		}
		for _, t := range bad {
			errs = append(errs, fmt.Sprintf(i18n.T("%s: %s refers to unknown tool %q, valid values: %v"), it.Name, TargetField, t, cfg.Build.Tools))
		}
		// overrides keys must name a tool with a dialect; tools absent from
		// build.tools are ignored so a shared library can carry overrides for
		// every vendor. A name without a dialect is an error.
		seenOv := map[string]bool{}
		for _, ev := range genericEvents {
			for _, g := range spec.Events[ev] {
				for _, tool := range sortedOverrideTools(g) {
					if !HasHookDialect(tool) && !seenOv[tool] {
						seenOv[tool] = true
						errs = append(errs, fmt.Sprintf(i18n.T("%s: overrides refers to unknown tool %q, valid values: %v"), it.Name, tool, config.HookTools))
					}
				}
			}
		}
		// Every ./ path a command refers to must exist in the hook directory:
		// a registry pointing at a missing script would fail inside the tool,
		// so it fails here instead. Override commands are checked as well.
		seenMissing := map[string]bool{}
		for _, c := range commandsOf(spec) {
			for _, tok := range splitCommand(c) {
				if !strings.HasPrefix(tok, "./") || seenMissing[tok] {
					continue
				}
				if _, err := os.Stat(filepath.Join(it.From, filepath.FromSlash(tok[2:]))); err != nil {
					seenMissing[tok] = true
					errs = append(errs, fmt.Sprintf(i18n.T("%s: command refers to ./%s, which does not exist in the hook directory"), it.Name, tok[2:]))
				}
			}
		}
		if len(errs) > 0 {
			p.RouteErrors = append(p.RouteErrors, errs...)
			continue
		}
		// Hook names double as Antigravity's top-level keys and appear in
		// paths; keep them in the skill-name character set. A name with no
		// usable character at all becomes "hook".
		if clean := sanitizeSkillName(it.OutName); clean != it.OutName {
			if clean == "skill" && !strings.Contains(strings.ToLower(it.OutName), "skill") {
				clean = "hook"
			}
			it.RouteNote = appendNote(it.RouteNote, fmt.Sprintf(i18n.T("directory name is normalized to %q (lowercase a-z, 0-9 and hyphens)"), clean))
			it.OutName = clean
		}
		it.Hook = spec
		it.Tools = targets
		it.HookOut = map[string]bool{}
		var excluded, noDialect []string
		for _, tool := range cfg.Build.Tools {
			if !containsStr(targets, tool) {
				if HasHookDialect(tool) {
					excluded = append(excluded, tool)
				}
				continue
			}
			if !HasHookDialect(tool) {
				// build.tools is an open list; a tool without a dialect gets
				// no registry, which plan reports.
				noDialect = append(noDialect, tool)
				continue
			}
			reg, notes := translateHook(spec, it.OutName, tool, "")
			for _, n := range notes {
				it.RouteNote = appendNote(it.RouteNote, n)
			}
			it.HookOut[tool] = len(reg.events) > 0
		}
		if len(excluded) > 0 {
			it.RouteNote = appendNote(it.RouteNote, fmt.Sprintf(i18n.T("%s: %s leaves out %s"), it.OutName, TargetField, strings.Join(excluded, ", ")))
		}
		for _, tool := range noDialect {
			it.RouteNote = appendNote(it.RouteNote, fmt.Sprintf(i18n.T("%s: agsy has no hook registry format for %q; the hook does not reach that tool"), it.OutName, tool))
		}
	}
}

// commandsOf lists every command string of a hook: the handlers of each
// group plus every per-tool override that replaces a command. Order is
// deterministic (events in genericEvents order, tools sorted).
func commandsOf(spec *HookSpec) []string {
	var out []string
	for _, ev := range genericEvents {
		for _, g := range spec.Events[ev] {
			for _, h := range g.Hooks {
				if handlerType(h) == "command" {
					c, _ := h["command"].(string)
					out = append(out, c)
				}
			}
			for _, tool := range sortedOverrideTools(g) {
				ov := g.Overrides[tool]
				if ov == nil {
					continue
				}
				for hi, oh := range ov.Hooks {
					if oh == nil || hi >= len(g.Hooks) {
						continue
					}
					merged := cloneHandler(g.Hooks[hi])
					for k, v := range oh {
						merged[k] = v
					}
					if _, replaced := oh["command"]; replaced && handlerType(merged) == "command" {
						c, _ := merged["command"].(string)
						out = append(out, c)
					}
				}
			}
		}
	}
	return out
}

// sortedOverrideTools lists the tools a group carries overrides for, sorted.
func sortedOverrideTools(g HookGroup) []string {
	tools := make([]string, 0, len(g.Overrides))
	for tool := range g.Overrides {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

// HookScriptPaths lists the ./ paths that command handlers execute directly
// (the first token of the command, relative to the hook directory), for
// doctor's executable-bit check. Paths passed to an interpreter are not
// listed. Override commands are included. A broken hook.yaml yields
// nothing; plan reports it.
func HookScriptPaths(hookDir string) []string {
	spec, err := parseHookSpec(filepath.Join(hookDir, HookFile))
	if err != nil || spec == nil {
		return nil
	}
	var out []string
	for _, c := range commandsOf(spec) {
		toks := splitCommand(c)
		if len(toks) > 0 && strings.HasPrefix(toks[0], "./") && !containsStr(out, toks[0][2:]) {
			out = append(out, toks[0][2:])
		}
	}
	sort.Strings(out)
	return out
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// translated is one hook rendered in one tool's dialect: native event name →
// groups (nested/named) or flat handlers (flat).
type translated struct {
	events map[string][]interface{}
	order  []string // native event names in genericEvents order
}

// translateHook renders spec for tool. absHookDir is the directory ./ paths
// are rewritten to (empty while only computing reach and notes). Vendor gaps
// come back as notes; nothing is dropped silently.
func translateHook(spec *HookSpec, name, tool, absHookDir string) (translated, []string) {
	d := dialects[tool]
	out := translated{events: map[string][]interface{}{}}
	var notes []string
	for _, ev := range genericEvents {
		groups, ok := spec.Events[ev]
		if !ok {
			continue
		}
		native, has := d.events[ev]
		if !has {
			notes = append(notes, fmt.Sprintf(i18n.T("%s: %s has no %s; this event is skipped there"), name, tool, ev))
			continue
		}
		for _, g := range groups {
			matcher := g.Matcher
			var ov *HookOverride
			if g.Overrides != nil {
				ov = g.Overrides[tool]
			}
			if ov != nil && ov.Matcher != nil {
				matcher = *ov.Matcher
			}
			var handlers []map[string]interface{}
			for hi, h := range g.Hooks {
				h = cloneHandler(h)
				if ov != nil && hi < len(ov.Hooks) && ov.Hooks[hi] != nil {
					for k, v := range ov.Hooks[hi] {
						h[k] = v
					}
				}
				typ := handlerType(h)
				if !d.types[typ] {
					notes = append(notes, fmt.Sprintf(i18n.T("%s: %s does not support handler type %q; that handler is skipped there"), name, tool, typ))
					continue
				}
				if typ == "command" {
					c, _ := h["command"].(string)
					h["command"] = rewriteCommand(c, absHookDir)
				} else if _, has := h["command"]; has {
					// Non-command handler carrying a command field (type
					// changed by an override): the field is dropped, with a note.
					delete(h, "command")
					notes = append(notes, fmt.Sprintf(i18n.T("%s: handler %d of %s is of type %q for %s; its command field is dropped there"), name, hi+1, ev, typ, tool))
				}
				if d.ownerMark {
					// Merged registry: every handler carries the display-only
					// statusMessage with the OwnerMark prefix; mount.ownsGroup
					// relies on it when the command names no path into the
					// output. A statusMessage from hook.yaml follows the mark.
					h["statusMessage"] = markStatus(name, h["statusMessage"])
				}
				handlers = append(handlers, h)
			}
			if len(handlers) == 0 {
				continue
			}
			switch d.shape {
			case shapeFlat:
				for _, h := range handlers {
					fh := orderedObj{}
					typ := handlerType(h)
					if typ != "command" {
						fh.set("type", typ)
					}
					if typ == "command" {
						fh.set("command", h["command"])
					}
					if matcher != "" {
						fh.set("matcher", matcher)
					}
					for _, k := range sortedKeys(h) {
						if k == "type" || k == "command" || k == "matcher" {
							continue
						}
						if d.flatFields[k] {
							fh.set(k, h[k])
						} else {
							notes = append(notes, fmt.Sprintf(i18n.T("%s: %s ignores handler field %q"), name, tool, k))
						}
					}
					out.events[native] = append(out.events[native], fh)
				}
			default:
				grp := orderedObj{}
				useMatcher := matcher != "" && (d.matcherOn == nil || d.matcherOn[ev])
				if useMatcher {
					grp.set("matcher", matcher)
				} else if matcher != "" {
					notes = append(notes, fmt.Sprintf(i18n.T("%s: %s ignores the matcher on %s; every %s fires that handler there"), name, tool, ev, ev))
				}
				var hs []interface{}
				for _, h := range handlers {
					hs = append(hs, orderedHandler(h))
				}
				grp.set("hooks", hs)
				out.events[native] = append(out.events[native], grp)
			}
		}
		if _, ok := out.events[native]; ok && !containsStr(out.order, native) {
			out.order = append(out.order, native)
		}
	}
	return out, notes
}

// markStatus builds the statusMessage of a merged handler: the OwnerMark
// and hook name, followed by the message hook.yaml gave (if any). A message
// already carrying the mark is returned as is (a hook.yaml copied from a
// registry).
func markStatus(name string, given interface{}) string {
	mark := OwnerMark + name
	s, _ := given.(string)
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return mark
	case strings.HasPrefix(s, OwnerMark):
		return s
	default:
		return mark + " · " + s
	}
}

func cloneHandler(h map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(h))
	for k, v := range h {
		m[k] = v
	}
	return m
}

// rewriteCommand replaces every whitespace-separated token starting with ./
// by the absolute path inside absHookDir. Only ./ tokens are touched; the
// rest of the string (other tokens, spacing) is kept as is ("python3
// ./check.py" works). The vendor runs the command through a shell, so a
// rewritten path is quoted when it contains spaces or shell metacharacters.
func rewriteCommand(cmd, absHookDir string) string {
	if absHookDir == "" {
		return cmd
	}
	var b strings.Builder
	i := 0
	for i < len(cmd) {
		if isSpace(cmd[i]) {
			b.WriteByte(cmd[i])
			i++
			continue
		}
		j := i
		for j < len(cmd) && !isSpace(cmd[j]) {
			j++
		}
		tok := cmd[i:j]
		if strings.HasPrefix(tok, "./") {
			tok = shellQuote(filepath.Join(absHookDir, filepath.FromSlash(tok[2:])))
		}
		b.WriteString(tok)
		i = j
	}
	return b.String()
}

func isSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// shellQuote quotes p when it contains anything beyond the plain path
// character set: double quotes on Windows (cmd.exe / PowerShell), POSIX
// single quotes elsewhere (only a literal single quote needs escaping).
func shellQuote(p string) string {
	if !needsQuote(p) {
		return p
	}
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"`
	}
	return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}

func needsQuote(p string) bool {
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '\\':
		default:
			return true
		}
	}
	return false
}

// splitCommand splits a command string into tokens with shell quoting
// applied: single- and double-quoted runs stay one token, quotes removed, so
// a path written by rewriteCommand comes back as the plain path. Used for
// the ownership check of a registry and for ./ references in a hook.yaml.
func splitCommand(cmd string) []string {
	var out []string
	var cur strings.Builder
	inTok := false
	quote := byte(0)
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if quote == '"' && c == '\\' && i+1 < len(cmd) && cmd[i+1] == '"' {
				cur.WriteByte('"')
				i++
			} else {
				cur.WriteByte(c)
			}
		case c == '\'' || c == '"':
			quote = c
			inTok = true
		case c == '\\' && runtime.GOOS != "windows" && i+1 < len(cmd) && !isSpace(cmd[i+1]):
			// POSIX escape outside quotes ('\'' inside a single-quoted path).
			// On Windows a backslash is a path separator.
			cur.WriteByte(cmd[i+1])
			i++
			inTok = true
		case isSpace(c):
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteByte(c)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

// SplitCommand exports splitCommand for mount's ownership check.
func SplitCommand(cmd string) []string { return splitCommand(cmd) }

// orderedObj is a JSON object with a fixed key order (encoding/json sorts map
// keys, which would scatter "type"/"command" among other fields and make the
// registries harder to read).
type orderedObj struct {
	keys []string
	vals map[string]interface{}
}

func (o *orderedObj) set(k string, v interface{}) {
	if o.vals == nil {
		o.vals = map[string]interface{}{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o orderedObj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// orderedHandler renders a handler with type first, command second, the
// rest alphabetically.
func orderedHandler(h map[string]interface{}) orderedObj {
	o := orderedObj{}
	o.set("type", handlerType(h))
	if c, ok := h["command"]; ok {
		o.set("command", c)
	}
	for _, k := range sortedKeys(h) {
		if k == "type" || k == "command" {
			continue
		}
		o.set(k, h[k])
	}
	return o
}

// RegistryGroups is the payload merge consumes: native event → the groups
// agsy contributes (nested shape only).
type RegistryGroups struct {
	Order  []string
	Groups map[string][]interface{}
}

// buildRegistry collects every hook's translation for tool into one
// registry document. It reads the hook.yaml copies already placed in the
// output (like ConcatRules reads the placed rules) so a source edited
// mid-build cannot make the registry disagree with the copied scripts.
func buildRegistry(cfg *config.Config, p *Plan, tool, outDir string) (interface{}, error) {
	d := dialects[tool]
	hooksTo := cfg.Build.Categories["hooks"].To
	merged := RegistryGroups{Groups: map[string][]interface{}{}}
	named := orderedObj{}
	for _, it := range p.Items {
		if it.Category != "hooks" || it.Hook == nil || !containsStr(it.Tools, tool) {
			continue
		}
		absHookDir := filepath.Join(outDir, filepath.FromSlash(hooksTo), it.OutName)
		spec, err := parseHookSpec(filepath.Join(absHookDir, HookFile))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", it.OutName, err)
		}
		tr, _ := translateHook(spec, it.OutName, tool, absHookDir)
		if d.shape == shapeNamed {
			if len(tr.order) == 0 {
				continue
			}
			entry := orderedObj{}
			entry.set("enabled", true)
			for _, ev := range tr.order {
				entry.set(ev, tr.events[ev])
			}
			named.set(it.OutName, entry)
			continue
		}
		for _, ev := range tr.order {
			if !containsStr(merged.Order, ev) {
				merged.Order = append(merged.Order, ev)
			}
			merged.Groups[ev] = append(merged.Groups[ev], tr.events[ev]...)
		}
	}
	// Events in genericEvents order (native names differ only for the flat
	// dialect, whose mapping preserves that order).
	sort.SliceStable(merged.Order, func(i, j int) bool {
		return eventRank(merged.Order[i], d) < eventRank(merged.Order[j], d)
	})
	switch d.shape {
	case shapeNamed:
		if named.vals == nil {
			named.vals = map[string]interface{}{}
		}
		return named, nil
	case shapeFlat:
		doc := orderedObj{}
		doc.set("version", 1)
		doc.set("hooks", eventsObj(merged))
		return doc, nil
	default:
		doc := orderedObj{}
		doc.set("hooks", eventsObj(merged))
		return doc, nil
	}
}

// eventRank returns the position of a native event name in genericEvents.
func eventRank(native string, d dialect) int {
	for i, ev := range genericEvents {
		if d.events[ev] == native {
			return i
		}
	}
	return len(genericEvents)
}

func eventsObj(r RegistryGroups) orderedObj {
	o := orderedObj{vals: map[string]interface{}{}}
	for _, ev := range r.Order {
		o.set(ev, r.Groups[ev])
	}
	return o
}

// WriteHookRegistry writes tool's registry file to dst (two-space indented,
// deterministic key order, trailing newline).
func WriteHookRegistry(cfg *config.Config, p *Plan, tool, dst string) error {
	doc, err := buildRegistry(cfg, p, tool, filepath.Dir(dst))
	if err != nil {
		return err
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(dst, append(raw, '\n'), 0o644)
}

// LoadRegistryGroups reads a nested-shape registry (claude / codex) back as
// event → groups, for merge. Group order inside the file is preserved. A
// file of another shape (no "hooks" object, or an event name outside the
// generic set, e.g. the flat dialect's camelCase names) is an error.
func LoadRegistryGroups(path string) (RegistryGroups, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RegistryGroups{}, err
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return RegistryGroups{}, fmt.Errorf(i18n.T("hook registry %s is not valid JSON: %w"), path, err)
	}
	hooksRaw, ok := top["hooks"]
	if !ok {
		return RegistryGroups{}, fmt.Errorf(i18n.T("hook registry %s has no \"hooks\" object; only the claude / codex registries can be merged"), path)
	}
	var hooks map[string][]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooks); err != nil {
		return RegistryGroups{}, fmt.Errorf(i18n.T("hook registry %s is not valid JSON: %w"), path, err)
	}
	for ev := range hooks {
		if !isGenericEvent(ev) {
			return RegistryGroups{}, fmt.Errorf(i18n.T("hook registry %s uses event name %q, which is not the claude / codex shape; only those registries can be merged"), path, ev)
		}
	}
	r := RegistryGroups{Groups: map[string][]interface{}{}}
	for _, ev := range genericEvents { // native == generic for nested dialects
		gs, ok := hooks[ev]
		if !ok {
			continue
		}
		r.Order = append(r.Order, ev)
		for _, g := range gs {
			r.Groups[ev] = append(r.Groups[ev], g)
		}
	}
	return r, nil
}

// CanonicalHash fingerprints a set of groups independent of formatting:
// every group is compacted, sorted within its event, events sorted by name.
func CanonicalHash(groups map[string][]json.RawMessage) string {
	var evs []string
	for ev := range groups {
		evs = append(evs, ev)
	}
	sort.Strings(evs)
	h := sha256.New()
	for _, ev := range evs {
		var lines []string
		for _, g := range groups[ev] {
			var buf bytes.Buffer
			if err := json.Compact(&buf, g); err != nil {
				buf.Reset()
				buf.Write(g)
			}
			lines = append(lines, buf.String())
		}
		sort.Strings(lines)
		h.Write([]byte(ev + "\n"))
		for _, l := range lines {
			h.Write([]byte(l + "\n"))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
