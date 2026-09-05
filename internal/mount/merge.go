// Merge mounts: used for Claude Code, whose hooks live in the "hooks" key of
// .claude/settings.json alongside user settings and therefore cannot be
// linked as a whole file. agsy owns exactly the matcher groups under "hooks"
// whose command points into the output hooks directory (or that carry the
// OwnerMark); the criterion mirrors IsManagedLink. Every other top-level key
// and every foreign group is preserved as is.
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
)

// MergePlan describes one merge target (shared by plan / status / apply / clean).
type MergePlan struct {
	Dir      string // mount dir as written in the config
	Name     string // file name inside dir
	FilePath string // absolute path of the target file
	Registry string // absolute path of the registry file in the output
	Tool     string
	State    MergeState
	Created  bool   // per the manifest: the file was created by agsy
	Owned    int    // agsy groups currently in the file
	Note     string // explanation for Invalid / Modified
}

// kv is one top-level member of a JSON object, raw, in file order.
type kv struct {
	key string
	raw json.RawMessage
}

// InspectMerge reports the state of every merge entry. records (from the
// manifest, untrusted) only contribute the created flag and the hash of the
// last write; ownership is always re-derived from the file content.
func InspectMerge(cfg *config.Config, records []build.MergeRecord) ([]MergePlan, error) {
	out := cfg.OutDir()
	hooksAbs := filepath.Join(out, filepath.FromSlash(cfg.Build.Categories["hooks"].To))
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
				Tool:     config.RegistryTool(sub),
			}
			var rec *build.MergeRecord
			for i := range records {
				if filepath.Clean(records[i].Path) == filepath.Clean(mp.FilePath) && records[i].Key == MergeKey {
					rec = &records[i]
				}
			}
			if rec != nil {
				mp.Created = rec.Created
			}
			inspectMerge(&mp, hooksAbs, rec)
			plans = append(plans, mp)
		}
	}
	return plans, nil
}

func inspectMerge(mp *MergePlan, hooksAbs string, rec *build.MergeRecord) {
	// What the last apply wrote (or, before any apply, what the registry
	// holds). An empty set means "nothing to merge": a missing file or a
	// file without agsy groups is then the correct state, not a gap.
	want := ""
	if rec != nil {
		want = rec.Hash
	} else if r, err := build.LoadRegistryGroups(mp.Registry); err == nil {
		want = build.CanonicalHash(toRaw(r))
	}
	fi, err := os.Lstat(mp.FilePath)
	switch {
	case err != nil:
		mp.State = MergeMissing
		if want == emptyHash {
			mp.State = MergeClean
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
	top, err := readTop(mp.FilePath)
	if err != nil {
		mp.State = MergeInvalid
		mp.Note = err.Error()
		return
	}
	events, err := hooksOf(top)
	if err != nil {
		mp.State = MergeInvalid
		mp.Note = err.Error()
		return
	}
	owned := map[string][]json.RawMessage{}
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.raw, &groups); err != nil {
			mp.State = MergeInvalid
			mp.Note = fmt.Sprintf(i18n.T("%s.%s is not an array"), MergeKey, ev.key)
			return
		}
		for _, g := range groups {
			if ownsGroup(g, hooksAbs) {
				owned[ev.key] = append(owned[ev.key], g)
				mp.Owned++
			}
		}
	}
	if mp.Owned == 0 {
		mp.State = MergeAbsent
		if want == emptyHash {
			mp.State = MergeClean
		}
		return
	}
	if want == "" || build.CanonicalHash(owned) == want {
		mp.State = MergeClean
		return
	}
	mp.State = MergeModified
	mp.Note = i18n.T("agsy entries in the hooks key were modified")
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

// ownsGroup reports whether a matcher group belongs to agsy: a command
// handler whose command names a path inside the output hooks directory, or a
// handler whose statusMessage carries build.OwnerMark.
func ownsGroup(group json.RawMessage, hooksAbs string) bool {
	var g struct {
		Hooks []struct {
			Command       string `json:"command"`
			StatusMessage string `json:"statusMessage"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(group, &g); err != nil {
		return false
	}
	prefix := filepath.Clean(hooksAbs) + string(filepath.Separator)
	for _, h := range g.Hooks {
		if strings.HasPrefix(h.StatusMessage, build.OwnerMark) {
			return true
		}
		for _, tok := range strings.Fields(h.Command) {
			if hasPathPrefix(tok, prefix) {
				return true
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
// re-indentation). Invalid targets abort before anything is written.
func ApplyMerge(cfg *config.Config, plans []MergePlan) ([]build.MergeRecord, error) {
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
	var records []build.MergeRecord
	for _, p := range plans {
		reg, err := build.LoadRegistryGroups(p.Registry)
		if err != nil {
			return records, err
		}
		incoming := toRaw(reg)
		_, exists := os.Lstat(p.FilePath)
		created := exists != nil || p.Created
		if len(incoming) == 0 && (exists != nil || p.Owned == 0) {
			// Nothing to merge and nothing of agsy's in the file: the file
			// is neither created nor rewritten.
			records = append(records, build.MergeRecord{Path: p.FilePath, Key: MergeKey, Hash: emptyHash, Created: p.Created})
			continue
		}
		if err := writeMerged(p.FilePath, hooksAbs, incoming, reg.Order); err != nil {
			return records, fmt.Errorf(i18n.T("failed to merge into %s: %w"), p.FilePath, err)
		}
		records = append(records, build.MergeRecord{
			Path: p.FilePath, Key: MergeKey, Hash: build.CanonicalHash(incoming), Created: created,
		})
	}
	return records, nil
}

// writeMerged rewrites path with agsy groups replaced by incoming (nil =
// remove all). Returns whether the resulting document is empty.
func writeMerged(path, hooksAbs string, incoming map[string][]json.RawMessage, order []string) error {
	top, err := readTop(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	events, err := hooksOf(top)
	if err != nil {
		return err
	}
	// Existing events, foreign groups only, in file order.
	var evOrder []string
	kept := map[string][]json.RawMessage{}
	for _, ev := range events {
		var groups []json.RawMessage
		if err := json.Unmarshal(ev.raw, &groups); err != nil {
			return fmt.Errorf(i18n.T("%s.%s is not an array"), MergeKey, ev.key)
		}
		evOrder = append(evOrder, ev.key)
		for _, g := range groups {
			if !ownsGroup(g, hooksAbs) {
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
	// Rebuild the hooks object.
	var hb bytes.Buffer
	hb.WriteByte('{')
	n := 0
	for _, ev := range evOrder {
		groups := append(append([]json.RawMessage{}, kept[ev]...), incoming[ev]...)
		if len(groups) == 0 {
			continue
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
	// Rebuild the top level: same order, hooks replaced or appended / dropped.
	var out []kv
	placed := false
	for _, m := range top {
		if m.key == MergeKey {
			if n > 0 {
				out = append(out, kv{key: MergeKey, raw: hb.Bytes()})
			}
			placed = true
			continue
		}
		out = append(out, m)
	}
	if !placed && n > 0 {
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
		return err
	}
	pretty.WriteByte('\n')
	return atomicWrite(path, pretty.Bytes())
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

// RemoveMerge strips agsy groups from every merge target (clean). A file
// agsy created that becomes empty is deleted; otherwise it is written back
// without agsy's groups. Invalid targets are skipped and reported.
func RemoveMerge(cfg *config.Config, records []build.MergeRecord) (cleaned, deleted, skipped []string, err error) {
	plans, err := InspectMerge(cfg, records)
	if err != nil {
		return nil, nil, nil, err
	}
	hooksAbs := filepath.Join(cfg.OutDir(), filepath.FromSlash(cfg.Build.Categories["hooks"].To))
	for _, p := range plans {
		switch p.State {
		case MergeMissing:
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
		if e := writeMerged(p.FilePath, hooksAbs, nil, nil); e != nil {
			return cleaned, deleted, skipped, e
		}
		if p.Created && isEmptyObject(p.FilePath) {
			if e := os.Remove(p.FilePath); e == nil {
				deleted = append(deleted, p.FilePath)
				removeIfEmpty(filepath.Dir(p.FilePath))
				continue
			}
		}
		cleaned = append(cleaned, p.FilePath)
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
