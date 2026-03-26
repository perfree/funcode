package skill

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/perfree/funcode/internal/config"
)

type SearchRoot struct {
	Name string
	Path string
}

type Manager struct {
	skills   map[string]*Skill
	warnings []string
}

func Load(projectPath string) (*Manager, error) {
	loader := NewLoader()
	roots := []SearchRoot{
		{Name: "global", Path: filepath.Join(config.GlobalConfigDir(), "skills")},
		{Name: "project", Path: filepath.Join(projectPath, config.ProjectConfigDir(), "skills")},
		{Name: "workspace", Path: filepath.Join(projectPath, "skills")},
		{Name: "workspace-examples", Path: filepath.Join(projectPath, "skills", "examples")},
	}

	skills, warnings, err := loader.LoadFromRoots(roots)
	if err != nil {
		return nil, err
	}
	return &Manager{skills: skills, warnings: warnings}, nil
}

func (m *Manager) Warnings() []string {
	return append([]string(nil), m.warnings...)
}

func (m *Manager) List() []*Skill {
	var items []*Skill
	for _, skill := range m.skills {
		items = append(items, skill)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	return items
}

func (m *Manager) Get(name string) (*Skill, bool) {
	skill, ok := m.skills[name]
	return skill, ok
}

func (m *Manager) BuildListText() string {
	items := m.List()
	if len(items) == 0 {
		return "No skills found."
	}
	var b strings.Builder
	b.WriteString("Available skills:\n\n")
	for _, skill := range items {
		b.WriteString("- ")
		b.WriteString(skill.Name)
		if skill.Description != "" {
			b.WriteString(": ")
			b.WriteString(skill.Description)
		}
		b.WriteString(" [")
		b.WriteString(skill.Source)
		b.WriteString("]\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m *Manager) BuildShowText(name string) (string, error) {
	skill, ok := m.Get(name)
	if !ok {
		return "", fmt.Errorf("skill %q not found", name)
	}
	return skill.Details(), nil
}
