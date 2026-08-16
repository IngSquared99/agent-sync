// Package i18n provides zero-dependency message localization.
//
// Design: English source strings double as message keys (gettext style).
// Translations live in embedded JSON files under locales/, one per
// language, mapping the English string to its translation. When no
// translation exists, the English source string is shown as-is — the
// tool never fails because a translation is missing.
package i18n

import (
	"embed"
	"encoding/json"
	"os"
	"strings"
)

//go:embed locales/*.json
var localeFS embed.FS

// table holds the active translation map; nil means English (source).
var table map[string]string

func init() { SetLang(Detect()) }

// Detect resolves the UI language: AGSY_LANG overrides everything,
// then LC_ALL / LANG; any value starting with "zh" selects zh-TW.
// Default is English.
func Detect() string {
	for _, k := range []string{"AGSY_LANG", "LC_ALL", "LANG"} {
		v := strings.ToLower(os.Getenv(k))
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "zh") {
			return "zh-TW"
		}
		return "en"
	}
	return "en"
}

// SetLang loads the translation table for lang; "en" clears it.
// Unknown languages silently fall back to English.
func SetLang(lang string) {
	if lang == "en" {
		table = nil
		return
	}
	raw, err := localeFS.ReadFile("locales/" + lang + ".json")
	if err != nil {
		table = nil
		return
	}
	m := map[string]string{}
	if err := json.Unmarshal(raw, &m); err == nil {
		table = m
	}
}

// T returns msg translated into the active language, or msg itself
// when no translation is available.
func T(msg string) string {
	if table == nil {
		return msg
	}
	if v, ok := table[msg]; ok {
		return v
	}
	return msg
}
