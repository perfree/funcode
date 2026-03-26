package skill

import (
	"fmt"
	"path/filepath"
	"strings"
)

type FileRef struct {
	RelPath string
	AbsPath string
}

type Skill struct {
	Name         string
	Title        string
	Description  string
	RootDir      string
	SkillFile    string
	Instructions string
	Source       string

	Scripts     []FileRef
	Resources   []FileRef
	References  []FileRef
	Assets      []FileRef
	Templates   []FileRef
	LinkedFiles []FileRef
}

func (s Skill) DisplayName() string {
	if strings.TrimSpace(s.Title) != "" {
		return s.Title
	}
	return s.Name
}

func (s Skill) PromptContext() string {
	var b strings.Builder
	b.WriteString("An active skill package is attached to this task.\n")
	b.WriteString("Follow the skill instructions when they are relevant.\n")
	b.WriteString("Resolve every relative path against the skill root directory.\n\n")
	b.WriteString("Skill name: ")
	b.WriteString(s.Name)
	if title := strings.TrimSpace(s.Title); title != "" && title != s.Name {
		b.WriteString("\nSkill title: ")
		b.WriteString(title)
	}
	if desc := strings.TrimSpace(s.Description); desc != "" {
		b.WriteString("\nDescription: ")
		b.WriteString(desc)
	}
	b.WriteString("\nSkill root: ")
	b.WriteString(s.RootDir)
	b.WriteString("\nSkill file: ")
	b.WriteString(s.SkillFile)
	b.WriteString("\n\nSKILL.md instructions:\n")
	b.WriteString(strings.TrimSpace(s.Instructions))

	appendSection := func(title string, refs []FileRef) {
		if len(refs) == 0 {
			return
		}
		b.WriteString("\n\n")
		b.WriteString(title)
		b.WriteString(":\n")
		for _, ref := range refs {
			b.WriteString("- ")
			b.WriteString(ref.RelPath)
			b.WriteString(" -> ")
			b.WriteString(ref.AbsPath)
			b.WriteString("\n")
		}
	}

	appendSection("Scripts", s.Scripts)
	appendSection("Resources", s.Resources)
	appendSection("References", s.References)
	appendSection("Assets", s.Assets)
	appendSection("Templates", s.Templates)
	appendSection("Referenced files mentioned in SKILL.md", s.LinkedFiles)

	b.WriteString("\n\nUse Read/Glob/Grep/Bash on these paths as needed instead of guessing their contents.")
	return b.String()
}

func (s Skill) Details() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Skill: %s\n", s.Name))
	if s.Title != "" && s.Title != s.Name {
		b.WriteString(fmt.Sprintf("Title: %s\n", s.Title))
	}
	if s.Description != "" {
		b.WriteString(fmt.Sprintf("Description: %s\n", s.Description))
	}
	b.WriteString(fmt.Sprintf("Source: %s\n", s.Source))
	b.WriteString(fmt.Sprintf("Root: %s\n", s.RootDir))
	b.WriteString(fmt.Sprintf("SKILL.md: %s\n", s.SkillFile))

	appendSection := func(label string, refs []FileRef) {
		if len(refs) == 0 {
			return
		}
		b.WriteString(fmt.Sprintf("\n%s:\n", label))
		for _, ref := range refs {
			b.WriteString(fmt.Sprintf("- `%s`\n", filepath.ToSlash(ref.RelPath)))
		}
	}

	appendSection("Scripts", s.Scripts)
	appendSection("Resources", s.Resources)
	appendSection("References", s.References)
	appendSection("Assets", s.Assets)
	appendSection("Templates", s.Templates)
	appendSection("Linked Files", s.LinkedFiles)

	b.WriteString("\nSKILL.md:\n\n")
	b.WriteString(strings.TrimSpace(s.Instructions))
	return b.String()
}
