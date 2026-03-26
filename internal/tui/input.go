package tui

import (
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// InputModel handles user text input
type InputModel struct {
	value        []rune
	cursor       int
	focused      bool
	prompt       string
	width        int
	MentionStyle lipgloss.Style
}

// NewInput creates a new input model
func NewInput(prompt string) InputModel {
	return InputModel{
		prompt:  prompt,
		focused: true,
		width:   80,
	}
}

// Focus sets focus
func (m *InputModel) Focus() { m.focused = true }

// Blur removes focus
func (m *InputModel) Blur() { m.focused = false }

// Value returns the current input value
func (m InputModel) Value() string { return string(m.value) }

// SetWidth sets the display width
func (m *InputModel) SetWidth(w int) { m.width = w }

// Reset clears the input
func (m *InputModel) Reset() { m.value = nil; m.cursor = 0 }

// SetValue sets the input value and moves cursor to end
func (m *InputModel) SetValue(s string) {
	m.value = []rune(normalizeNewlines(s))
	m.cursor = len(m.value)
}

func (m *InputModel) InsertString(s string) {
	runes := []rune(normalizeNewlines(s))
	newVal := make([]rune, len(m.value)+len(runes))
	copy(newVal, m.value[:m.cursor])
	copy(newVal[m.cursor:], runes)
	copy(newVal[m.cursor+len(runes):], m.value[m.cursor:])
	m.value = newVal
	m.cursor += len(runes)
}

// Update handles key events
func (m InputModel) Update(msg tea.Msg) (InputModel, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyBackspace:
			if m.cursor > 0 {
				m.value = append(m.value[:m.cursor-1], m.value[m.cursor:]...)
				m.cursor--
			}
		case tea.KeyDelete:
			if m.cursor < len(m.value) {
				m.value = append(m.value[:m.cursor], m.value[m.cursor+1:]...)
			}
		case tea.KeyLeft:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyRight:
			if m.cursor < len(m.value) {
				m.cursor++
			}
		case tea.KeyHome, tea.KeyCtrlA:
			m.cursor = 0
		case tea.KeyEnd, tea.KeyCtrlE:
			m.cursor = len(m.value)
		case tea.KeyCtrlU:
			m.value = m.value[m.cursor:]
			m.cursor = 0
		case tea.KeyCtrlK:
			m.value = m.value[:m.cursor]
		case tea.KeyRunes:
			// Insert characters at cursor
			m.InsertString(string(msg.Runes))
		}
	}

	return m, nil
}

// View renders the input with @mention highlighting
func (m InputModel) View() string {
	var b strings.Builder
	b.WriteString(m.prompt)

	text := m.value
	cursorPos := m.cursor

	if !m.focused || cursorPos > len(text) {
		b.WriteString(highlightMentions(string(text), m.MentionStyle))
		return b.String()
	}

	// Render with cursor: split at cursor position, highlight each part, insert cursor
	before := string(text[:cursorPos])
	after := string(text[cursorPos:])
	b.WriteString(highlightMentions(before, m.MentionStyle))
	b.WriteString("\u2588") // Block cursor
	b.WriteString(highlightMentions(after, m.MentionStyle))
	return b.String()
}

func normalizeNewlines(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// highlightMentions applies MentionStyle to @word segments in the text
func highlightMentions(text string, style lipgloss.Style) string {
	if text == "" {
		return ""
	}
	runes := []rune(text)
	var b strings.Builder
	i := 0
	for i < len(runes) {
		if runes[i] == '@' {
			// Collect @mention token
			j := i + 1
			for j < len(runes) && !unicode.IsSpace(runes[j]) {
				j++
			}
			if j > i+1 {
				mention := string(runes[i:j])
				b.WriteString(style.Render(mention))
				i = j
				continue
			}
		}
		b.WriteRune(runes[i])
		i++
	}
	return b.String()
}
