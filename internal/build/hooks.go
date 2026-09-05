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
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// HookFile is the declaration file every hook directory must contain.
const HookFile = "hook.yaml"

// OwnerMark prefixes the statusMessage agsy sets on non-command handlers in
// nested registries, so a merge target can recognise groups that contain no
// command path into the output.
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
			"PreCompact", "PostToolUseFailure", "StopFailure"),
		types: set("command", "http", "mcp_tool", "prompt", "agent"),
		shape: shapeNested,
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
	for ev, groups := range spec.Events {
		if !isGenericEvent(ev) {
			problems = append(problems, fmt.Sprintf(i18n.T("unknown event %q, valid values: %v"), ev, genericEvents))
			continue
		}
		if len(groups) == 0 {
			problems = append(problems, fmt.Sprintf(i18n.T("event %s has no handlers"), ev))
		}
		for gi, g := range groups {
			if len(g.Hooks) == 0 {
				problems = append(problems, fmt.Sprintf(i18n.T("event %s has no handlers"), ev))
			}
			for hi, h := range g.Hooks {
				spec.Events[ev][gi].Hooks[hi] = normalizeYAML(h).(map[string]interface{})
				h = spec.Events[ev][gi].Hooks[hi]
				typ := handlerType(h)
				if typ == "command" {
					if c, _ := h["command"].(string); strings.TrimSpace(c) == "" {
						problems = append(problems, fmt.Sprintf(i18n.T("handler %d of %s has no command"), hi+1, ev))
					}
				}
			}
			for _, ov := range g.Overrides {
				if ov == nil {
					continue
				}
				for i, h := range ov.Hooks {
					if h != nil {
						ov.Hooks[i] = normalizeYAML(h).(map[string]interface{})
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
// tool in build.tools.
func specTargets(cfg *config.Config, spec *HookSpec) ([]string, []string) {
	var targets, bad []string
	switch v := spec.Target.(type) {
	case string:
		targets = []string{v}
	case []interface{}:
		for _, x := range v {
			if s, ok := x.(string); ok {
				targets = append(targets, s)
			}
		}
	}
	if len(targets) == 0 {
		return append([]string{}, cfg.Build.Tools...), nil
	}
	for _, t := range targets {
		if !cfg.HasTool(t) {
			bad = append(bad, t)
		}
	}
	return targets, bad
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
		targets, bad := specTargets(cfg, spec)
		if len(bad) > 0 {
			p.RouteErrors = append(p.RouteErrors, fmt.Sprintf(i18n.T("%s: %s refers to unknown tool %q, valid values: %v"), it.Name, TargetField, bad[0], cfg.Build.Tools))
			continue
		}
		// overrides keys must name a tool with a dialect; tools absent from
		// build.tools are ignored so a shared library can carry overrides for
		// every vendor. A name without a dialect is an error.
		badOv := ""
		for _, groups := range spec.Events {
			for _, g := range groups {
				for tool := range g.Overrides {
					if !HasHookDialect(tool) && badOv == "" {
						badOv = tool
					}
				}
			}
		}
		if badOv != "" {
			p.RouteErrors = append(p.RouteErrors, fmt.Sprintf(i18n.T("%s: overrides refers to unknown tool %q, valid values: %v"), it.Name, badOv, config.HookTools))
			continue
		}
		// Every ./ path a command refers to must exist in the hook directory:
		// a registry pointing at a missing script would fail at the worst
		// moment (inside the tool), so it fails here instead.
		missing := ""
		for _, groups := range spec.Events {
			for _, g := range groups {
				for _, h := range g.Hooks {
					if handlerType(h) != "command" {
						continue
					}
					c, _ := h["command"].(string)
					for _, tok := range strings.Fields(c) {
						if strings.HasPrefix(tok, "./") {
							if _, err := os.Stat(filepath.Join(it.From, filepath.FromSlash(tok[2:]))); err != nil && missing == "" {
								missing = tok[2:]
							}
						}
					}
				}
			}
		}
		if missing != "" {
			p.RouteErrors = append(p.RouteErrors, fmt.Sprintf(i18n.T("%s: command refers to ./%s, which does not exist in the hook directory"), it.Name, missing))
			continue
		}
		// Hook names double as Antigravity's top-level keys and appear in
		// paths; keep them in the skill-name character set.
		if clean := sanitizeSkillName(it.OutName); clean != it.OutName {
			it.RouteNote = appendNote(it.RouteNote, fmt.Sprintf(i18n.T("directory name is normalized to %q (lowercase a-z, 0-9 and hyphens)"), clean))
			it.OutName = clean
		}
		it.Hook = spec
		it.Tools = targets
		it.HookOut = map[string]bool{}
		for _, tool := range cfg.Build.Tools {
			if !HasHookDialect(tool) {
				continue
			}
			if !containsStr(targets, tool) {
				continue
			}
			reg, notes := translateHook(spec, it.OutName, tool, "")
			for _, n := range notes {
				it.RouteNote = appendNote(it.RouteNote, n)
			}
			it.HookOut[tool] = len(reg.events) > 0
		}
	}
}

// HookScriptPaths lists the ./ paths the hook's command handlers refer to
// (relative to the hook directory), for doctor's executable-bit check. A
// broken hook.yaml yields nothing; plan reports it.
func HookScriptPaths(hookDir string) []string {
	spec, err := parseHookSpec(filepath.Join(hookDir, HookFile))
	if err != nil || spec == nil {
		return nil
	}
	var out []string
	for _, groups := range spec.Events {
		for _, g := range groups {
			for _, h := range g.Hooks {
				if handlerType(h) != "command" {
					continue
				}
				c, _ := h["command"].(string)
				for _, tok := range strings.Fields(c) {
					if strings.HasPrefix(tok, "./") && !containsStr(out, tok[2:]) {
						out = append(out, tok[2:])
					}
				}
			}
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
				} else if d.shape == shapeNested {
					// Ownership of a merged group is derived from a command path
					// into the output. Non-command handlers have no such path, so
					// they carry the display-only statusMessage field with the
					// OwnerMark prefix (see mount.ownsGroup).
					if _, has := h["statusMessage"]; !has {
						h["statusMessage"] = OwnerMark + name
					}
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

func cloneHandler(h map[string]interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(h))
	for k, v := range h {
		m[k] = v
	}
	return m
}

// rewriteCommand replaces every whitespace-separated token starting with ./
// by the absolute path inside absHookDir. Only ./ tokens are touched: the
// rule stays predictable and documentable ("python3 ./check.py" works).
func rewriteCommand(cmd, absHookDir string) string {
	if absHookDir == "" {
		return cmd
	}
	toks := strings.Fields(cmd)
	for i, tok := range toks {
		if strings.HasPrefix(tok, "./") {
			toks[i] = filepath.Join(absHookDir, filepath.FromSlash(tok[2:]))
		}
	}
	return strings.Join(toks, " ")
}

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
// event → groups, for merge. Group order inside the file is preserved.
func LoadRegistryGroups(path string) (RegistryGroups, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return RegistryGroups{}, err
	}
	var doc struct {
		Hooks map[string][]json.RawMessage `json:"hooks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return RegistryGroups{}, fmt.Errorf(i18n.T("hook registry %s is not valid JSON: %w"), path, err)
	}
	r := RegistryGroups{Groups: map[string][]interface{}{}}
	for _, ev := range genericEvents { // native == generic for nested dialects
		gs, ok := doc.Hooks[ev]
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
