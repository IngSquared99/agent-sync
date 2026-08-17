package build

import "testing"

// Skills renamed by the conflict strategy must satisfy the Agent Skills name
// spec (lowercase a-z0-9 and single hyphens; name == directory name), or the
// skill silently fails to load in Claude Code. All categories share "-fromlib-".
func TestTaggedNames(t *testing.T) {
	if got := tagged("python-style.md", "god-lib", false); got != "python-style-fromlib-god-lib.md" {
		t.Errorf("file tag = %q, want python-style-fromlib-god-lib.md", got)
	}
	if got := tagged("code-review", "god-lib", true); got != "code-review-fromlib-god-lib" {
		t.Errorf("skill tag = %q, want code-review-fromlib-god-lib", got)
	}
}

func TestSanitizeSkillName(t *testing.T) {
	cases := map[string]string{
		"code-review-fromlib-god-lib": "code-review-fromlib-god-lib",
		"Code-Review-fromlib-God-Lib": "code-review-fromlib-god-lib", // uppercase folded
		"api_doc-fromlib-my.lib":   "api-doc-fromlib-my-lib",   // punctuation → hyphen
		"a--b":                     "a-b",                      // no consecutive hyphens
		"-edge-":                   "edge",                     // no leading/trailing hyphen
		"日本語":                      "skill",                    // nothing valid left → fallback
	}
	for in, want := range cases {
		if got := sanitizeSkillName(in); got != want {
			t.Errorf("sanitizeSkillName(%q) = %q, want %q", in, got, want)
		}
	}
}
