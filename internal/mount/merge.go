// Merge mounts: used for Claude Code, whose hooks live in the "hooks" key of
// .claude/settings.json alongside user settings and therefore cannot be
// linked as a whole file. agsy owns exactly the matcher groups under "hooks"
// that carry the OwnerMark or whose command points into the output hooks
// directory (the current one, or the one recorded in the manifest); the
// criterion mirrors IsManagedLink. Every other top-level key and every
// foreign group is preserved as is.
package mount

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/build"
	"github.com/IngSquared99/agent-sync/internal/config"
)

// MergeKey is the top-level key merge entries live under.
const MergeKey = "hooks"

// emptyHash is the canonical hash of "no groups at all".
var emptyHash = build.CanonicalHash(nil)

// MergeState is the current state of one merge target.
type MergeState int

const (
	MergeMissing  MergeState = iota // file does not exist → apply creates it
	MergeClean                      // agsy groups present and identical to the last apply
	MergeModified                   // agsy groups present but changed on the artifact side
	MergeAbsent                     // valid file, no agsy groups yet → apply adds them
	MergeInvalid                    // symlink, not a regular file, or not a JSON object → apply refuses, clean skips
	MergeIdle                       // nothing to merge and nothing of agsy's in the file (or no file) → apply and clean leave it alone
	MergeStale                      // agsy groups present but their command paths point outside the current output (project moved, build.out renamed) → apply rewrites
)

// MergePlan describes one merge target (shared by plan / status / apply / clean).
type MergePlan struct {
	Dir      string // mount dir as written in the config
	Name     string // file name inside dir
	FilePath string // absolute path of the target file
	Registry string // absolute path of the registry file in the output
	State    MergeState
	Created  bool   // per the manifest: the file was created by agsy
	Owned    int    // agsy groups currently in the file
	Note     string // explanation for Invalid / Modified / Stale
}

// kv is one top-level member of a JSON object, raw, in file order.
type kv struct {
	key string
	raw json.RawMessage
}

// ownerPrefixes lists the output hooks directories whose paths mark a group
// as agsy's: the current one plus those recorded in the manifest. The
// manifest is untrusted; a prefix only widens which groups under the hooks
// key are rewritten, nothing outside that key.
func ownerPrefixes(cfg *config.Config, records []build.MergeRecord) []string {
	cur := filepath.Join(cfg.OutDir(), filepath.FromSlash(cfg.Build.Categories["hooks"].To))
	out := []string{cur}
	for _, r := range records {
		if r.HooksDir != "" && filepath.Clean(r.HooksDir) != filepath.Clean(cur) && !containsPath(out, r.HooksDir) {
			out = append(out, r.HooksDir)
		}
	}
	return out
}

func containsPath(list []string, p string) bool {
	for _, x := range list {
		if filepath.Clean(x) == filepath.Clean(p) {
			return true
		}
	}
	return false
}

// InspectMerge reports the state of every merge entry. records (from the
// manifest, untrusted) only contribute the created flag, the hash of the
// last write and the recorded hooks directory; ownership is always
// re-derived from the file content.
func InspectMerge(cfg *config.Config, records []build.MergeRecord) ([]MergePlan, error) {
	out := cfg.OutDir()
	prefixes := ownerPrefixes(cfg, records)
	var plans []MergePlan
	for _, m := range cfg.Mount {
		mdir, err := cfg.ExpandPath(m.Dir)
		if err != nil {
			return nil, err
		}
		names := make([]string, 0, len(m.Merge))
		for k := range m.Merge {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, name := range names {
			sub := strings.Trim(filepath.ToSlash(m.Merge[name]), "/")
			mp := MergePlan{
				Dir: m.Dir, Name: name,
				FilePath: filepath.Join(mdir, name),
				Registry: filepath.Join(out, filepath.FromSlash(sub)),
			}
			rec := findRecord(records, mp.FilePath)
			if rec != nil {
				mp.Created = rec.Created
			}
			inspectMerge(&mp, cfg.Build.Categories["hooks"].To, prefixes, rec)
			plans = append(plans, mp)
		}
	}
	return plans, nil
}

func findRecord(records []build.MergeRecord, path string) *build.MergeRecord {
	var rec *build.MergeRecord
	for i := range records {
		if filepath.Clean(records[i].Path) == filepath.Clean(path) && records[i].Key == MergeKey {
			rec = &records[i]
		}
	}
	return rec
}

func inspectMerge(mp *MergePlan, hooksTo string, prefixes []string, rec *build.MergeRecord) {
	// have: what the registry holds now — decides whether there is anything
	// to merge (a registry without groups makes a missing file, or a file
	// without agsy groups, Idle rather than a gap). want: what the last
	// apply wrote per the manifest — decides Clean vs Modified; before any
	// apply the registry stands in.
	have := ""
	if r, err := build.LoadRegistryGroups(mp.Registry); err == nil {
		have = build.CanonicalHash(toRaw(r))
	}
	want := have
	if rec != nil {
		want = rec.Hash
	}
	fi, err := os.Lstat(mp.FilePath)
	switch {
	case err != nil:
		mp.State = MergeMissing
		if have == emptyHash {
			mp.State = MergeIdle
		}
		return
	case fi.Mode()&os.ModeSymlink != 0:
		mp.State = MergeInvalid
		mp.Note = i18n.T("is a symbolic link; merge targets must be regular files")
		return
	case !fi.Mode().IsRegular():
		mp.State = MergeInvalid
		mp.Note = i18n.T("is not a regular file")
		return
	}
	owned, err := ownedGroups(mp.FilePath, prefixes)
	if err != nil {
		mp.State = MergeInvalid
		mp.Note = err.Error()
		return
	}
	for _, gs := range owned {
		mp.Owned += len(gs)
	}
	if mp.Owned == 0 {
		mp.State = MergeAbsent
		if have == emptyHash {
			mp.State = MergeIdle
		}
		return
	}
	if want == "" || build.CanonicalHash(owned) == want {
		mp.State = MergeClean
		if hasStalePath(owned, hooksTo, prefixes) {
			mp.State = MergeStale
			mp.Note = i18n.T("agsy entries point at a previous output path")
		}
		return
	}
	mp.State = MergeModified
	mp.Note = i18n.T("agsy entries in the hooks key were modified")
}

// hasStalePath reports whether an owned command handler names a script
// inside a previous output hooks directory: a path under one of the
// directories the manifest recorded (prefixes[1:]), or an absolute path
// outside the current directory (prefixes[0]) whose tail is
// <hooksTo>/<hook name>/… (hook name from the handler's OwnerMark; covers a
// renamed build.out with no record left). Other absolute paths (an
// interpreter such as /usr/bin/env) do not count.
func hasStalePath(owned map[string][]json.RawMessage, hooksTo string, prefixes []string) bool {
	cur := filepath.Clean(prefixes[0]) + string(filepath.Separator)
	for _, gs := range owned {
		for _, g := range gs {
			var x struct {
				Hooks []struct {
					Command       string `json:"command"`
					StatusMessage string `json:"statusMessage"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(g, &x); err != nil {
				continue
			}
			for _, h := range x.Hooks {
				name := hookNameOf(h.StatusMessage)
				for _, tok := range build.SplitCommand(h.Command) {
					if !filepath.IsAbs(tok) || hasPathPrefix(tok, cur) {
						continue
					}
					for _, old := range prefixes[1:] {
						if hasPathPrefix(tok, filepath.Clean(old)+string(filepath.Separator)) {
							return true
						}
					}
					if name != "" && strings.Contains(filepath.ToSlash(tok), "/"+hooksTo+"/"+name+"/") {
						return true
					}
				}
			}
		}
	}
	return false
}

// hookNameOf extracts the hook name from an OwnerMark statusMessage
// ("agsy:<name>" or "agsy:<name> · message"); "" when the mark is absent.
func hookNameOf(status string) string {
	if !strings.HasPrefix(status, build.OwnerMark) {
		return ""
	}
	rest := strings.TrimPrefix(status, build.OwnerMark)
	if i := strings.IndexAny(rest, " \t"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// ownedGroups returns the agsy-owned groups of a merge target by event. An
// error means the file is not usable as a merge target.
func ownedGroups(path string, prefixes []string) (map[string][]json.RawMessage, error) {
	top, err := readTop(path)
	if err != nil {
		return nil, err
	}
	events, err := hooksOf(top)
	if err != nil {
		return nil, err
	}
	owned := map[string][]json.RawMessage{}
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.raw, &groups); err != nil {
			return nil, fmt.Errorf(i18n.T("%s.%s is not an array"), MergeKey, ev.key)
		}
		for _, g := range groups {
			if ownsGroup(g, prefixes) {
				owned[ev.key] = append(owned[ev.key], g)
			}
		}
	}
	return owned, nil
}

// MergeOrphans lists recorded merge targets the mount config no longer
// names that still hold agsy groups. Reported by status, stripped by clean,
// never touched by apply (same rule as orphaned links). Record paths are
// untrusted: only paths inside the project directory are inspected (a
// copied project must not reach into the original's files), and a file is
// listed only when it verifiably holds agsy groups. Recorded paths outside
// the project are never opened and come back as foreign, for status to
// report as unchecked.
func MergeOrphans(cfg *config.Config, records []build.MergeRecord) (orphans, foreign []string, err error) {
	plans, err := InspectMerge(cfg, records)
	if err != nil {
		return nil, nil, err
	}
	current := map[string]bool{}
	for _, p := range plans {
		current[filepath.Clean(p.FilePath)] = true
	}
	prefixes := ownerPrefixes(cfg, records)
	base := filepath.Clean(cfg.BaseDir) + string(filepath.Separator)
	seen := map[string]bool{}
	for _, r := range records {
		p := filepath.Clean(r.Path)
		if r.Key != MergeKey || current[p] || seen[p] {
			continue
		}
		seen[p] = true
		if !hasPathPrefix(p, base) {
			foreign = append(foreign, p)
			continue
		}
		fi, err := os.Lstat(p)
		if err != nil || !fi.Mode().IsRegular() {
			continue
		}
		owned, err := ownedGroups(p, prefixes)
		if err != nil {
			continue
		}
		if len(owned) > 0 {
			orphans = append(orphans, p)
		}
	}
	sort.Strings(orphans)
	sort.Strings(foreign)
	return orphans, foreign, nil
}

func toRaw(r build.RegistryGroups) map[string][]json.RawMessage {
	out := map[string][]json.RawMessage{}
	for ev, gs := range r.Groups {
		for _, g := range gs {
			switch x := g.(type) {
			case json.RawMessage:
				out[ev] = append(out[ev], x)
			default:
				if b, err := json.Marshal(x); err == nil {
					out[ev] = append(out[ev], b)
				}
			}
		}
	}
	return out
}

// readTop parses the file as a JSON object and returns its members in file
// order, values kept raw. Anything but an object is an error.
func readTop(path string) ([]kv, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil // empty file: treat as {}
	}
	return parseObject(raw)
}

func parseObject(raw []byte) ([]kv, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf(i18n.T("is not valid JSON: %v"), err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, errors.New(i18n.T("is not a JSON object"))
	}
	var members []kv
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf(i18n.T("is not valid JSON: %v"), err)
		}
		key, _ := kt.(string)
		var val json.RawMessage
		if err := dec.Decode(&val); err != nil {
			return nil, fmt.Errorf(i18n.T("is not valid JSON: %v"), err)
		}
		members = append(members, kv{key: key, raw: val})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf(i18n.T("is not valid JSON: %v"), err)
	}
	return members, nil
}

// hooksOf returns the members of the "hooks" object (event → raw array), or
// nil when the key is absent. A "hooks" that is not an object is an error.
func hooksOf(top []kv) ([]kv, error) {
	for _, m := range top {
		if m.key != MergeKey {
			continue
		}
		if bytes.Equal(bytes.TrimSpace(m.raw), []byte("null")) {
			return nil, nil
		}
		events, err := parseObject(m.raw)
		if err != nil {
			return nil, fmt.Errorf(i18n.T("key %q %v"), MergeKey, err)
		}
		return events, nil
	}
	return nil, nil
}

// ownsGroup reports whether a matcher group belongs to agsy: a handler
// whose statusMessage carries build.OwnerMark, or a command handler whose
// command names a path (quoted or not) inside one of prefixes.
func ownsGroup(group json.RawMessage, prefixes []string) bool {
	var g struct {
		Hooks []struct {
			Command       string `json:"command"`
			StatusMessage string `json:"statusMessage"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(group, &g); err != nil {
		return false
	}
	for _, h := range g.Hooks {
		if strings.HasPrefix(h.StatusMessage, build.OwnerMark) {
			return true
		}
		for _, tok := range build.SplitCommand(h.Command) {
			for _, dir := range prefixes {
				if hasPathPrefix(tok, filepath.Clean(dir)+string(filepath.Separator)) {
					return true
				}
			}
		}
	}
	return false
}

func hasPathPrefix(p, prefix string) bool {
	p = filepath.Clean(filepath.FromSlash(p)) + string(filepath.Separator)
	if runtime.GOOS == "windows" {
		return strings.HasPrefix(strings.ToLower(p), strings.ToLower(prefix))
	}
	return strings.HasPrefix(p, prefix)
}

// ApplyMerge writes every merge target: agsy groups are replaced by the
// registry's groups, everything else is preserved byte-for-byte (apart from
// re-indentation). Invalid targets abort before anything is written; Idle
// ones are not touched.
func ApplyMerge(cfg *config.Config, plans []MergePlan, records []build.MergeRecord) ([]build.MergeRecord, error) {
	var bad []string
	for _, p := range plans {
		if p.State == MergeInvalid {
			bad = append(bad, p.FilePath+"  ("+p.Note+")")
		}
	}
	if len(bad) > 0 {
		return nil, fmt.Errorf(i18n.T("the following merge targets cannot be updated (symbolic link or not a JSON object):\n  %s\nfix them manually, then retry"), strings.Join(bad, "\n  "))
	}
	hooksAbs := filepath.Join(cfg.OutDir(), filepath.FromSlash(cfg.Build.Categories["hooks"].To))
	prefixes := ownerPrefixes(cfg, records)
	var out []build.MergeRecord
	for _, p := range plans {
		reg, err := build.LoadRegistryGroups(p.Registry)
		if err != nil {
			return out, err
		}
		incoming := toRaw(reg)
		_, exists := os.Lstat(p.FilePath)
		created := exists != nil || p.Created
		prior := shellsOf(findRecord(records, p.FilePath))
		if len(incoming) == 0 && (exists != nil || p.Owned == 0) {
			// Nothing to merge and nothing of agsy's in the file: the file
			// is neither created nor rewritten.
			out = append(out, build.MergeRecord{Path: p.FilePath, Key: MergeKey, Hash: emptyHash, Created: p.Created, HooksDir: hooksAbs})
			continue
		}
		added, err := writeMerged(p.FilePath, prefixes, incoming, reg.Order, prior)
		if err != nil {
			return out, fmt.Errorf(i18n.T("failed to merge into %s: %w"), p.FilePath, err)
		}
		out = append(out, build.MergeRecord{
			Path: p.FilePath, Key: MergeKey, Hash: build.CanonicalHash(incoming), Created: created, HooksDir: hooksAbs,
			AddedKey: added.key, AddedEvents: added.events,
		})
	}
	return out, nil
}

// shells names the containers apply introduced into a merge target: the
// "hooks" key and event arrays. Kept in the manifest; apply and clean remove
// exactly those when empty and leave pre-existing (possibly empty) ones.
type shells struct {
	key    bool
	events []string
}

func shellsOf(rec *build.MergeRecord) shells {
	if rec == nil {
		return shells{}
	}
	return shells{key: rec.AddedKey, events: rec.AddedEvents}
}

func (s shells) has(ev string) bool {
	for _, e := range s.events {
		if e == ev {
			return true
		}
	}
	return false
}

// writeMerged rewrites path with agsy groups replaced by incoming (nil =
// remove all). Pre-existing containers (event arrays, the "hooks" key) are
// kept even when they end up empty; containers listed in prior are dropped
// once empty. Returns the containers agsy is responsible for after the
// write.
func writeMerged(path string, prefixes []string, incoming map[string][]json.RawMessage, order []string, prior shells) (shells, error) {
	top, err := readTop(path)
	if err != nil && !os.IsNotExist(err) {
		return prior, err
	}
	events, err := hooksOf(top)
	if err != nil {
		return prior, err
	}
	// Existing events, foreign groups only, in file order.
	var evOrder []string
	kept := map[string][]json.RawMessage{}
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.raw, &groups); err != nil {
			return prior, fmt.Errorf(i18n.T("%s.%s is not an array"), MergeKey, ev.key)
		}
		evOrder = append(evOrder, ev.key)
		kept[ev.key] = []json.RawMessage{}
		for _, g := range groups {
			if !ownsGroup(g, prefixes) {
				kept[ev.key] = append(kept[ev.key], g)
			}
		}
	}
	for _, ev := range order {
		found := false
		for _, e := range evOrder {
			if e == ev {
				found = true
			}
		}
		if !found {
			evOrder = append(evOrder, ev)
		}
	}
	hadKey := false
	for _, m := range top {
		if m.key == MergeKey {
			hadKey = true
		}
	}
	// Rebuild the hooks object.
	var hb bytes.Buffer
	hb.WriteByte('{')
	n := 0
	next := shells{key: prior.key || !hadKey}
	for _, ev := range evOrder {
		groups := append(append([]json.RawMessage{}, kept[ev]...), incoming[ev]...)
		_, existed := kept[ev]
		mine := !existed || prior.has(ev)
		if len(groups) == 0 && mine {
			continue
		}
		if mine {
			next.events = append(next.events, ev)
		}
		if n > 0 {
			hb.WriteByte(',')
		}
		n++
		kb, _ := json.Marshal(ev)
		hb.Write(kb)
		hb.WriteString(":[")
		for i, g := range groups {
			if i > 0 {
				hb.WriteByte(',')
			}
			hb.Write(g)
		}
		hb.WriteByte(']')
	}
	hb.WriteByte('}')
	keepKey := n > 0 || (hadKey && !prior.key)
	if !keepKey {
		next = shells{}
	}
	// Rebuild the top level: same order, hooks replaced or appended / dropped.
	var out []kv
	placed := false
	for _, m := range top {
		if m.key == MergeKey {
			if keepKey {
				out = append(out, kv{key: MergeKey, raw: hb.Bytes()})
			}
			placed = true
			continue
		}
		out = append(out, m)
	}
	if !placed && keepKey {
		out = append(out, kv{key: MergeKey, raw: hb.Bytes()})
	}
	var tb bytes.Buffer
	tb.WriteByte('{')
	for i, m := range out {
		if i > 0 {
			tb.WriteByte(',')
		}
		kb, _ := json.Marshal(m.key)
		tb.Write(kb)
		tb.WriteByte(':')
		tb.Write(m.raw)
	}
	tb.WriteByte('}')
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, tb.Bytes(), "", "  "); err != nil {
		return prior, err
	}
	pretty.WriteByte('\n')
	return next, atomicWrite(path, pretty.Bytes())
}

// atomicWrite writes via a temp file in the same directory and renames it
// over path, keeping the existing file's permissions.
func atomicWrite(path string, data []byte) error {
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		perm = fi.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agsy-merge-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// RemoveMerge strips agsy groups from every merge target (clean), orphaned
// targets included. A file agsy created that becomes empty is deleted;
// otherwise it is written back without agsy's groups. Idle targets are not
// touched. Invalid targets are skipped and reported.
func RemoveMerge(cfg *config.Config, records []build.MergeRecord) (cleaned, deleted, skipped []string, err error) {
	plans, err := InspectMerge(cfg, records)
	if err != nil {
		return nil, nil, nil, err
	}
	prefixes := ownerPrefixes(cfg, records)
	strip := func(path string, created bool) error {
		if _, e := writeMerged(path, prefixes, nil, nil, shellsOf(findRecord(records, path))); e != nil {
			return e
		}
		if created && isEmptyObject(path) {
			if e := os.Remove(path); e == nil {
				deleted = append(deleted, path)
				removeIfEmpty(filepath.Dir(path))
				return nil
			}
		}
		cleaned = append(cleaned, path)
		return nil
	}
	for _, p := range plans {
		switch p.State {
		case MergeMissing, MergeIdle:
			continue
		case MergeInvalid:
			skipped = append(skipped, p.FilePath)
			continue
		case MergeAbsent:
			if p.Created && isEmptyObject(p.FilePath) {
				if e := os.Remove(p.FilePath); e == nil {
					deleted = append(deleted, p.FilePath)
					removeIfEmpty(filepath.Dir(p.FilePath))
				}
			}
			continue
		}
		if e := strip(p.FilePath, p.Created); e != nil {
			return cleaned, deleted, skipped, e
		}
	}
	orphans, _, err := MergeOrphans(cfg, records)
	if err != nil {
		return cleaned, deleted, skipped, err
	}
	for _, o := range orphans {
		created := false
		if rec := findRecord(records, o); rec != nil {
			created = rec.Created
		}
		if e := strip(o, created); e != nil {
			return cleaned, deleted, skipped, e
		}
	}
	return cleaned, deleted, skipped, nil
}

func isEmptyObject(path string) bool {
	top, err := readTop(path)
	return err == nil && len(top) == 0
}

func removeIfEmpty(dir string) {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) == 0 {
		_ = os.Remove(dir)
	}
}
