package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Loader struct{}

func NewLoader() *Loader { return &Loader{} }

func (l *Loader) LoadFromRoots(roots []SearchRoot) (map[string]*Skill, []string, error) {
	skills := make(map[string]*Skill)
	var warnings []string

	for _, root := range roots {
		if strings.TrimSpace(root.Path) == "" {
			continue
		}
		info, err := os.Stat(root.Path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			warnings = append(warnings, fmt.Sprintf("scan %s: %v", root.Path, err))
			continue
		}
		if !info.IsDir() {
			continue
		}

		err = filepath.Walk(root.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == ".git" || name == "node_modules" || name == "vendor" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(info.Name(), "SKILL.md") {
				return nil
			}

			skill, loadErr := l.LoadSkill(path, root.Name)
			if loadErr != nil {
				warnings = append(warnings, fmt.Sprintf("load %s: %v", path, loadErr))
				return nil
			}
			skills[skill.Name] = skill
			return nil
		})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("walk %s: %v", root.Path, err))
		}
	}

	return skills, warnings, nil
}

func (l *Loader) LoadSkill(skillFile string, source string) (*Skill, error) {
	rootDir := filepath.Dir(skillFile)
	contentBytes, err := os.ReadFile(skillFile)
	if err != nil {
		return nil, err
	}
	content := strings.ReplaceAll(string(contentBytes), "\r\n", "\n")

	name := filepath.Base(rootDir)
	title, description := parseSkillDoc(content)

	skill := &Skill{
		Name:         name,
		Title:        title,
		Description:  description,
		RootDir:      rootDir,
		SkillFile:    skillFile,
		Instructions: content,
		Source:       source,
	}

	skill.Scripts = collectDirFiles(rootDir, "scripts")
	skill.Resources = collectDirFiles(rootDir, "resources")
	skill.References = collectDirFiles(rootDir, "references")
	skill.Assets = collectDirFiles(rootDir, "assets")
	skill.Templates = collectDirFiles(rootDir, "templates")
	skill.LinkedFiles = detectLinkedFiles(rootDir, content)
	return skill, nil
}

func parseSkillDoc(content string) (string, string) {
	lines := strings.Split(content, "\n")
	title := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title = strings.TrimSpace(strings.TrimPrefix(trimmed, "# "))
			break
		}
	}

	var paragraphs []string
	var current []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if len(current) > 0 {
				paragraphs = append(paragraphs, strings.Join(current, " "))
				current = nil
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		current = append(current, trimmed)
	}
	if len(current) > 0 {
		paragraphs = append(paragraphs, strings.Join(current, " "))
	}

	description := ""
	if len(paragraphs) > 0 {
		description = paragraphs[0]
	}
	return title, description
}

func collectDirFiles(rootDir string, dirName string) []FileRef {
	target := filepath.Join(rootDir, dirName)
	info, err := os.Stat(target)
	if err != nil || !info.IsDir() {
		return nil
	}

	var refs []FileRef
	_ = filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(rootDir, path)
		if relErr != nil {
			return nil
		}
		refs = append(refs, FileRef{
			RelPath: filepath.ToSlash(rel),
			AbsPath: path,
		})
		return nil
	})

	sort.Slice(refs, func(i, j int) bool { return refs[i].RelPath < refs[j].RelPath })
	return refs
}

var (
	backtickPathRegex = regexp.MustCompile("`([^`]+)`")
	markdownLinkRegex = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
)

func detectLinkedFiles(rootDir string, content string) []FileRef {
	seen := map[string]bool{}
	var refs []FileRef

	addCandidate := func(candidate string) {
		candidate = strings.TrimSpace(candidate)
		candidate = strings.Trim(candidate, "\"'")
		if candidate == "" {
			return
		}
		if strings.Contains(candidate, "://") || strings.HasPrefix(candidate, "#") {
			return
		}
		candidate = filepath.Clean(filepath.FromSlash(candidate))
		if filepath.IsAbs(candidate) {
			return
		}
		abs := filepath.Join(rootDir, candidate)
		info, err := os.Stat(abs)
		if err != nil || info.IsDir() {
			return
		}
		rel, err := filepath.Rel(rootDir, abs)
		if err != nil {
			return
		}
		rel = filepath.ToSlash(rel)
		if seen[rel] {
			return
		}
		seen[rel] = true
		refs = append(refs, FileRef{RelPath: rel, AbsPath: abs})
	}

	for _, match := range backtickPathRegex.FindAllStringSubmatch(content, -1) {
		if len(match) == 2 {
			addCandidate(match[1])
		}
	}
	for _, match := range markdownLinkRegex.FindAllStringSubmatch(content, -1) {
		if len(match) == 2 {
			addCandidate(match[1])
		}
	}

	sort.Slice(refs, func(i, j int) bool { return refs[i].RelPath < refs[j].RelPath })
	return refs
}
