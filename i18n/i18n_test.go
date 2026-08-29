package i18n

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// loadZhTW reads the zh-TW locale from disk (not the embedded copy) so the
// test always checks the current working tree.
func loadZhTW(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("locales/zh-TW.json")
	if err != nil {
		t.Fatalf("read zh-TW.json: %v", err)
	}
	m := map[string]string{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("zh-TW.json is not valid JSON: %v", err)
	}
	return m
}

// collectSourceKeys scans every non-vendored .go file for i18n.T("...")
// literals and returns the decoded key set.
func collectSourceKeys(t *testing.T) map[string]bool {
	t.Helper()
	re := regexp.MustCompile(`i18n\.T\((` + "`[^`]*`" + `|"(?:[^"\\]|\\.)*")\)`)
	keys := map[string]bool{}
	root := ".."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Vendored code and build outputs are out of scope.
			// Skip vendored code, build outputs, and this package itself
			// (its comments mention i18n.T("...") as documentation).
			if info.Name() == "yaml" || info.Name() == "dist" || info.Name() == ".git" || info.Name() == "i18n" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(raw), -1) {
			lit := m[1]
			var s string
			if strings.HasPrefix(lit, "`") {
				s = strings.Trim(lit, "`")
			} else {
				var uerr error
				s, uerr = strconv.Unquote(lit)
				if uerr != nil {
					continue
				}
			}
			keys[s] = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

// Every i18n.T key used in source must have a zh-TW translation; a missing
// entry means Chinese users silently get English for that message.
func TestAllSourceKeysTranslated(t *testing.T) {
	zh := loadZhTW(t)
	for k := range collectSourceKeys(t) {
		if _, ok := zh[k]; !ok {
			t.Errorf("missing zh-TW translation for key:\n  %q", k)
		}
	}
}

// Every zh-TW entry must correspond to a key actually used in source;
// orphans are stale translations that hide real gaps.
func TestNoOrphanTranslations(t *testing.T) {
	zh := loadZhTW(t)
	used := collectSourceKeys(t)
	for k := range zh {
		if !used[k] {
			t.Errorf("orphan zh-TW entry (key not found in source):\n  %q", k)
		}
	}
}

// Placeholder verbs must survive translation: a %s dropped from the
// translated string would corrupt the rendered message at runtime.
func TestPlaceholdersMatch(t *testing.T) {
	verb := regexp.MustCompile(`%\[?\d*\]?[a-zA-Z]`)
	zh := loadZhTW(t)
	for k, v := range zh {
		if len(verb.FindAllString(k, -1)) != len(verb.FindAllString(v, -1)) {
			t.Errorf("placeholder count mismatch:\n  en: %q\n  zh: %q", k, v)
		}
	}
}

func TestFallbackToEnglish(t *testing.T) {
	SetLang("en")
	if got := T("no such key"); got != "no such key" {
		t.Errorf("English mode must return the source string, got %q", got)
	}
	SetLang("nope")
	if got := T("no such key"); got != "no such key" {
		t.Errorf("unknown language must fall back to source, got %q", got)
	}
}
