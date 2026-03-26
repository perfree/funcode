package tui

import "github.com/charmbracelet/lipgloss"

// Theme holds all UI styles - Tokyo Night inspired
type Theme struct {
	StatusBar      lipgloss.Style
	UserLabel      lipgloss.Style
	AssistantLabel lipgloss.Style
	ToolLabel      lipgloss.Style
	ErrorText      lipgloss.Style
	InputPrompt    lipgloss.Style
	ToolBox        lipgloss.Style
	ToolName       lipgloss.Style
	ToolOK         lipgloss.Style
	ToolErr        lipgloss.Style
	Logo           lipgloss.Style
	ModelBadge     lipgloss.Style
	RoleBadge      lipgloss.Style
	TokenCount     lipgloss.Style
	Dimmed         lipgloss.Style
	Separator      lipgloss.Style
	StreamText     lipgloss.Style
	SpinnerStyle   lipgloss.Style
	WelcomeTitle   lipgloss.Style
	WelcomeText    lipgloss.Style

	// Statusline (bottom bar)
	StatusLine lipgloss.Style
	SlModel    lipgloss.Style
	SlRole     lipgloss.Style
	SlContext  lipgloss.Style
	SlTokens   lipgloss.Style
	SlCwd      lipgloss.Style
	SlSep      lipgloss.Style

	// Mention popup
	MentionBox       lipgloss.Style
	MentionItem      lipgloss.Style
	MentionSelected  lipgloss.Style
	MentionDim       lipgloss.Style
	MentionHighlight lipgloss.Style // @mention highlighting in text

	// Slash command popup
	CommandBox      lipgloss.Style
	CommandItem     lipgloss.Style
	CommandSelected lipgloss.Style
	CommandDim      lipgloss.Style

	// Approval prompt
	ApprovalBox   lipgloss.Style
	ApprovalKey   lipgloss.Style
	ApprovalAllow lipgloss.Style
	ApprovalDeny  lipgloss.Style

	// Markdown
	MarkdownHeading1  lipgloss.Style
	MarkdownHeading2  lipgloss.Style
	MarkdownHeading3  lipgloss.Style
	MarkdownBullet    lipgloss.Style
	MarkdownQuote     lipgloss.Style
	MarkdownCode      lipgloss.Style
	MarkdownCodeBlock lipgloss.Style
	MarkdownBold      lipgloss.Style
	MarkdownItalic    lipgloss.Style
	MarkdownLink      lipgloss.Style
}

// DefaultTheme returns a polished dark theme (Tokyo Night palette)
func DefaultTheme() *Theme {
	return &Theme{
		StatusBar: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1b26")).
			Foreground(lipgloss.Color("#a9b1d6")),

		UserLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true),

		AssistantLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bb9af7")).
			Bold(true),

		ToolLabel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e0af68")).
			Bold(true),

		ErrorText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f7768e")).
			Bold(true),

		InputPrompt: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")).
			Bold(true),

		ToolBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#3b3d57")).
			Padding(0, 1).
			MarginLeft(2),

		ToolName: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e0af68")).
			Bold(true),

		ToolOK: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ece6a")),

		ToolErr: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f7768e")),

		Logo: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true),

		ModelBadge: lipgloss.NewStyle().
			Background(lipgloss.Color("#3d59a1")).
			Foreground(lipgloss.Color("#c0caf5")).
			Padding(0, 1),

		RoleBadge: lipgloss.NewStyle().
			Background(lipgloss.Color("#5a3e8e")).
			Foreground(lipgloss.Color("#c0caf5")).
			Padding(0, 1),

		TokenCount: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		Dimmed: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		Separator: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3b3d57")),

		StreamText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a9b1d6")),

		SpinnerStyle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bb9af7")),

		WelcomeTitle: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true),

		WelcomeText: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		StatusLine: lipgloss.NewStyle().
			Background(lipgloss.Color("#1a1b26")).
			Foreground(lipgloss.Color("#a9b1d6")),

		SlModel: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true),

		SlRole: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#bb9af7")).
			Bold(true),

		SlContext: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")),

		SlTokens: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ece6a")),

		SlCwd: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		SlSep: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#3b3d57")),

		MentionBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7aa2f7")).
			Padding(0, 1).
			MarginLeft(3),

		MentionItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a9b1d6")).
			Padding(0, 1),

		MentionSelected: lipgloss.NewStyle().
			Background(lipgloss.Color("#3d59a1")).
			Foreground(lipgloss.Color("#c0caf5")).
			Bold(true).
			Padding(0, 1),

		MentionDim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		MentionHighlight: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")).
			Bold(true),

		CommandBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7dcfff")).
			Padding(0, 1).
			MarginLeft(3),

		CommandItem: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a9b1d6")).
			Padding(0, 1),

		CommandSelected: lipgloss.NewStyle().
			Background(lipgloss.Color("#33475b")).
			Foreground(lipgloss.Color("#7dcfff")).
			Bold(true).
			Padding(0, 1),

		CommandDim: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")),

		ApprovalBox: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#e0af68")).
			Padding(0, 1).
			MarginLeft(2),

		ApprovalKey: lipgloss.NewStyle().
			Background(lipgloss.Color("#3d59a1")).
			Foreground(lipgloss.Color("#c0caf5")).
			Bold(true).
			Padding(0, 1),

		ApprovalAllow: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ece6a")).
			Bold(true),

		ApprovalDeny: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#f7768e")).
			Bold(true),

		MarkdownHeading1: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7aa2f7")).
			Bold(true),

		MarkdownHeading2: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")).
			Bold(true),

		MarkdownHeading3: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#9ece6a")).
			Bold(true),

		MarkdownBullet: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#e0af68")).
			Bold(true),

		MarkdownQuote: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#565f89")).
			Italic(true),

		MarkdownCode: lipgloss.NewStyle().
			Background(lipgloss.Color("#24283b")).
			Foreground(lipgloss.Color("#e0af68")).
			Padding(0, 1),

		MarkdownCodeBlock: lipgloss.NewStyle().
			Background(lipgloss.Color("#1f2335")).
			Foreground(lipgloss.Color("#c0caf5")).
			Padding(0, 1).
			MarginLeft(2),

		MarkdownBold: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#c0caf5")).
			Bold(true),

		MarkdownItalic: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a9b1d6")).
			Italic(true),

		MarkdownLink: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7dcfff")).
			Underline(true),
	}
}
