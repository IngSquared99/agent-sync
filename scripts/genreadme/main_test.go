package main

import (
	"strings"
	"testing"
)

// transform is what turns a docs chapter into a README section; a regression
// here ships a broken README through the sync-readme workflow without any
// human in the loop, so its rules are pinned down one by one.
func TestTransform(t *testing.T) {
	l := languages[0] // en; the rules under test are language-independent
	in := strings.Join([]string{
		"# Chapter Title",
		"",
		"Intro with a [doc link](config.md#some-anchor) inline.",
		"",
		"## Section",
		"",
		"```sh",
		"# a shell comment, not a heading",
		"## neither is this",
		"```",
		"",
		"### Subsection",
		"",
		"![demo](../assets/demo.en.gif)",
		"",
		"→ Next chapter: [Commands](commands.md)",
		"",
	}, "\n")
	got := transform(in, l, "B")

	assertContains := func(want string) {
		t.Helper()
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n--- got ---\n%s", want, got)
		}
	}
	assertNotContains := func(bad string) {
		t.Helper()
		if strings.Contains(got, bad) {
			t.Errorf("output must not contain %q\n--- got ---\n%s", bad, got)
		}
	}

	// The chapter title becomes the lettered README heading.
	if !strings.HasPrefix(got, "## B. Chapter Title") {
		t.Errorf("first line = %q, want lettered chapter heading", strings.SplitN(got, "\n", 2)[0])
	}
	// In-chapter sections are demoted one level and get breathing room.
	assertContains("<br>\n\n### Section")
	assertContains("#### Subsection")
	// Code-fence contents pass through untouched — no demotion inside fences.
	assertContains("\n# a shell comment, not a heading\n")
	assertContains("\n## neither is this\n")
	// Chapter navigation lines are dropped entirely.
	assertNotContains("Next chapter")
	// Doc-page links are rewritten to the site, anchors dropped.
	assertContains("](" + site + "/en/config)")
	assertNotContains("some-anchor")
	assertNotContains("](config.md")
	// Chapter-relative image paths become repository-relative ones.
	assertContains("![demo](docs/assets/demo.en.gif)")
	assertNotContains("](../assets/")
	// Output ends with exactly one trailing newline.
	if !strings.HasSuffix(got, "\n") || strings.HasSuffix(got, "\n\n") {
		t.Errorf("output must end with exactly one newline, got %q", got[len(got)-4:])
	}
}

// Both language definitions must agree on the structural fields the
// transform relies on, so a chapter renders the same shape in each README.
func TestLanguagesAreStructurallyComplete(t *testing.T) {
	if len(languages) != 2 {
		t.Fatalf("expected en + zh-TW, got %d languages", len(languages))
	}
	for _, l := range languages {
		if l.dir == "" || l.out == "" || l.header == "" || l.deeper == "" || l.navDrop == "" {
			t.Errorf("language %q has an empty structural field: %+v", l.dir, l)
		}
		if !strings.Contains(l.header, "AUTO-GENERATED") && !strings.Contains(l.header, "自動組裝") {
			t.Errorf("language %q header lacks the do-not-edit marker", l.dir)
		}
	}
}
