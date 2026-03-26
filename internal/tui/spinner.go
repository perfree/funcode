package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var spinnerFrames = []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"}

// SpinnerModel is a simple spinner
type SpinnerModel struct {
	frame     int
	active    bool
	label     string
	startedAt time.Time
	style     lipgloss.Style
}

// NewSpinner creates a new spinner
func NewSpinner() SpinnerModel {
	return SpinnerModel{
		style: lipgloss.NewStyle().Foreground(lipgloss.Color("#bb9af7")),
	}
}

// SpinnerTickMsg triggers spinner animation
type SpinnerTickMsg time.Time

func spinnerTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return SpinnerTickMsg(t)
	})
}

// Start starts the spinner
func (s *SpinnerModel) Start(label string) tea.Cmd {
	return s.StartWithStartedAt(label, time.Now())
}

// StartWithStartedAt starts the spinner while keeping a stable elapsed timer.
func (s *SpinnerModel) StartWithStartedAt(label string, startedAt time.Time) tea.Cmd {
	s.active = true
	s.label = label
	if startedAt.IsZero() {
		startedAt = time.Now()
	}
	s.startedAt = startedAt
	return spinnerTick()
}

// Stop stops the spinner
func (s *SpinnerModel) Stop() {
	s.active = false
}

// Update handles tick messages
func (s SpinnerModel) Update(msg tea.Msg) (SpinnerModel, tea.Cmd) {
	if !s.active {
		return s, nil
	}
	if _, ok := msg.(SpinnerTickMsg); ok {
		s.frame = (s.frame + 1) % len(spinnerFrames)
		return s, spinnerTick()
	}
	return s, nil
}

// View renders the spinner
func (s SpinnerModel) View() string {
	if !s.active {
		return ""
	}
	label := s.label
	if !s.startedAt.IsZero() {
		label += " " + s.style.Copy().Faint(true).Render("["+formatElapsedSince(s.startedAt)+"]")
	}
	return "  " + s.style.Render(spinnerFrames[s.frame]) + " " + label
}

func formatElapsedSince(startedAt time.Time) string {
	if startedAt.IsZero() {
		return ""
	}
	return formatElapsed(time.Since(startedAt))
}

func formatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		minutes := int(d / time.Minute)
		seconds := int(d/time.Second) % 60
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
	hours := int(d / time.Hour)
	minutes := int(d/time.Minute) % 60
	seconds := int(d/time.Second) % 60
	return fmt.Sprintf("%dh%02dm%02ds", hours, minutes, seconds)
}
