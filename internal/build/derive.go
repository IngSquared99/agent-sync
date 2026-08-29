// Derived forms: the conversions build produces alongside the verbatim
// copies, so every tool reads its native shape of the same content.
//   - workflow → skill  (skills/<name>/SKILL.md, invoked as /name)
//   - workflow → stub   (workflows/<name>.md, points the agent at the skill)
//   - rules    → AGENTS.md (single concatenated file at the output root)
//
// All derived outputs are read-only artifacts like everything else in the
// output; they are regenerated from the sources on every apply.
package build

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/IngSquared99/agent-sync/i18n"
	"github.com/IngSquared99/agent-sync/internal/config"
	"github.com/IngSquared99/agent-sync/internal/yaml"
)

// WorkflowToSkill converts a single-file workflow into a skill directory at
// dstDir. The body is kept verbatim; the front matter is rebuilt:
//   - name: the sanitized skill name (must match the directory per the Agent
//     Skills spec)
//   - description: front-matter description, else the first "# " heading,
//     else the name — without one the skill never surfaces in any tool
//   - disable-model-invocation: true — workflows are human-triggered
//   - the agsy-specific target field is stripped; other fields carry over
func WorkflowToSkill(srcPath, dstDir, skillName string) error {
	raw, err := readSourceFile(srcPath)
	if err != nil {
		return err
	}
	fm, err := frontMatter(srcPath)
	if err != nil {
		return err
	}
	body := strings.TrimPrefix(string(raw), bomPrefix)
	if fm != nil {
		body = stripFrontMatter(body)
	} else {
		fm = map[string]interface{}{}
	}
	desc, _ := fm["description"].(string)
	if desc == "" {
		desc = firstHeading(body)
	}
	if desc == "" {
		desc = skillName
	}
	delete(fm, TargetField)
	delete(fm, "name")
	delete(fm, "description")
	delete(fm, "disable-model-invocation")

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + skillName + "\n")
	sb.WriteString("description: " + yamlScalar(desc) + "\n")
	sb.WriteString("disable-model-invocation: true\n")
	if len(fm) > 0 {
		// yaml.Marshal sorts map keys, keeping the output deterministic.
		enc, err := yaml.Marshal(fm)
		if err != nil {
			return err
		}
		sb.Write(enc)
	}
	sb.WriteString("---\n")
	if !strings.HasPrefix(body, "\n") {
		sb.WriteString("\n")
	}
	sb.WriteString(body)

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dstDir, "SKILL.md"), []byte(sb.String()), 0o644)
}

// WriteWorkflowVerbatim writes a stub-tool-only workflow into the workflows
// output: the content unchanged except that the agsy-specific target field is
// stripped from the front matter. Reports whether the content was rewritten.
func WriteWorkflowVerbatim(srcPath, dst string) (bool, error) {
	fm, err := frontMatter(srcPath)
	if err != nil {
		return false, err
	}
	if _, ok := fm[TargetField]; !ok {
		return false, copyFile(srcPath, dst)
	}
	raw, err := readSourceFile(srcPath)
	if err != nil {
		return false, err
	}
	body := stripFrontMatter(strings.TrimPrefix(string(raw), bomPrefix))
	delete(fm, TargetField)
	if len(fm) == 0 {
		return true, os.WriteFile(dst, []byte(strings.TrimPrefix(body, "\n")), 0o644)
	}
	enc, err := yaml.Marshal(fm)
	if err != nil {
		return false, err
	}
	out := "---\n" + string(enc) + "---\n" + body
	return true, os.WriteFile(dst, []byte(out), 0o644)
}

// readSourceFile reads a source file, refusing symbolic links (the same
// guard copyFile applies to verbatim copies). openNoFollow closes the window
// between the Lstat check and the read.
func readSourceFile(srcPath string) ([]byte, error) {
	fi, err := os.Lstat(srcPath)
	if err != nil {
		return nil, err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf(i18n.T("refusing to copy symbolic link %s"), srcPath)
	}
	f, err := openNoFollow(srcPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// WriteWorkflowStub writes the workflows/<name>.md stub: a few lines that keep
// /name available in tools reading the workflows directory, redirecting the
// agent to the derived skill that holds the full content.
func WriteWorkflowStub(dst, outName, skillName, skillsTo string) error {
	title := strings.TrimSuffix(outName, filepath.Ext(outName))
	body := fmt.Sprintf(
		"# %s\n\nThe full content of this workflow is maintained as the `%s` skill\n(read it from your skills directory: %s/%s/SKILL.md).\nLoad that skill and follow its steps exactly; do not improvise, add or skip steps.\n",
		title, skillName, skillsTo, skillName)
	return os.WriteFile(dst, []byte(body), 0o644)
}

// ConcatRules writes the derived AGENTS.md: every rule concatenated in item
// order (= source priority). Each section is preceded by a marker comment
// naming the rule file it came from, so a change inside the file can be
// traced back to its source.
// It reads the rule copies already placed in the output rather than the
// sources: reading the sources a second time would let a source edited
// mid-build produce an AGENTS.md that disagrees with the rules/ copies made
// moments earlier. The output copies are tool-owned and contain no links.
func ConcatRules(cfg *config.Config, p *Plan, dst string) error {
	var sb strings.Builder
	sb.WriteString("<!-- Generated by agsy. Do not edit: this file is rebuilt from the sources on every apply. -->\n")
	rulesTo := cfg.Build.Categories["rules"].To
	outDir := filepath.Dir(dst)
	for _, it := range p.Items {
		if it.Category != "rules" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(outDir, filepath.FromSlash(rulesTo), it.OutName))
		if err != nil {
			return err
		}
		content := strings.TrimPrefix(string(raw), bomPrefix)
		sb.WriteString("\n<!-- agsy: " + rulesTo + "/" + it.OutName + " -->\n")
		sb.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			sb.WriteString("\n")
		}
	}
	return os.WriteFile(dst, []byte(sb.String()), 0o644)
}

// stripFrontMatter removes a leading front-matter block (frontMatter already
// verified it parses) and returns the body.
func stripFrontMatter(s string) string {
	s = strings.TrimPrefix(s, bomPrefix)
	if !strings.HasPrefix(s, "---\n") && !strings.HasPrefix(s, "---\r\n") {
		return s
	}
	rest := s[strings.Index(s, "\n")+1:]
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") == "---" {
			return strings.Join(lines[i+1:], "\n")
		}
	}
	return s
}

// firstHeading returns the text of the first markdown "# " heading.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(t, "# "))
		}
	}
	return ""
}

// yamlScalar renders a string as a safe single-line YAML scalar.
func yamlScalar(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	enc, err := yaml.Marshal(s)
	if err != nil {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return strings.TrimRight(string(enc), "\n")
}
