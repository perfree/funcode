package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/perfree/funcode/pkg/types"
)

// ChatMessage represents a rendered chat message
type ChatMessage struct {
	Role      types.Role
	RoleName  string
	Duration  string
	Text      string
	ToolCalls []ToolCallDisplay
}

// ToolCallDisplay holds tool call display info
type ToolCallDisplay struct {
	Name     string
	Status   string // running, completed, error
	Duration string
	Result   string
	Expanded bool // whether to show full result
}

var (
	markdownCodeFenceRegex  = regexp.MustCompile("^```")
	markdownHeadingRegex    = regexp.MustCompile(`^(#{1,3})\s+(.+)$`)
	markdownOrderedRegex    = regexp.MustCompile(`^(\d+)\.\s+(.+)$`)
	markdownBulletRegex     = regexp.MustCompile(`^[-*+]\s+(.+)$`)
	markdownLinkRegex       = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	markdownBoldRegex       = regexp.MustCompile(`\*\*(.+?)\*\*`)
	markdownItalicRegex     = regexp.MustCompile(`\*(.+?)\*`)
	markdownInlineCodeRegex = regexp.MustCompile("`([^`]+)`")
)

// RenderMessage renders a chat message with polished styling
func RenderMessage(theme *Theme, msg ChatMessage, width int, expandAll bool, markdownRender bool) string {
	var b strings.Builder

	b.WriteString(renderMessageHeader(theme, msg))
	b.WriteString("\n")
	b.WriteString(renderMessageBody(theme, msg, width, expandAll, markdownRender))
	return b.String()
}

func renderMessageHeader(theme *Theme, msg ChatMessage) string {
	switch msg.Role {
	case types.RoleUser:
		return theme.UserLabel.Render("  [USER] You")
	case types.RoleAssistant:
		name := "Assistant"
		if msg.RoleName != "" {
			name = msg.RoleName
		}
		return theme.AssistantLabel.Render("  [AI] " + name)
	case types.RoleTool:
		return theme.ToolLabel.Render("  [TOOL] Tool")
	default:
		return ""
	}
}

func renderMessageBody(theme *Theme, msg ChatMessage, width int, expandAll bool, markdownRender bool) string {
	var b strings.Builder
	if msg.Text != "" {
		if markdownRender && msg.Role == types.RoleAssistant {
			rendered := renderMarkdown(theme, msg.Text, width)
			if rendered != "" {
				b.WriteString(rendered)
				b.WriteString("\n")
			}
		} else {
			for _, line := range strings.Split(msg.Text, "\n") {
				if msg.Role == types.RoleUser {
					b.WriteString("  " + highlightMentions(line, theme.MentionHighlight) + "\n")
				} else {
					b.WriteString("  " + line + "\n")
				}
			}
		}
	}

	for _, tc := range msg.ToolCalls {
		var statusStr string
		switch tc.Status {
		case "completed":
			statusStr = theme.ToolOK.Render(" done")
		case "error":
			statusStr = theme.ToolErr.Render(" error")
		default:
			statusStr = theme.Dimmed.Render(" running...")
		}

		toolHeader := fmt.Sprintf("%s %s",
			theme.ToolName.Render(tc.Name),
			statusStr,
		)
		if tc.Duration != "" {
			toolHeader += " " + theme.Dimmed.Render("["+tc.Duration+"]")
		}

		boxWidth := width - 8
		if boxWidth < 40 {
			boxWidth = 40
		}

		toolContent := toolHeader
		if tc.Result != "" {
			shouldExpand := expandAll || tc.Expanded

			if shouldExpand {
				toolContent += "\n" + renderFullResult(theme, tc.Result, boxWidth-4)
			} else {
				preview, totalLines := getPreviewLine(tc.Result)
				toolContent += "\n" + theme.Dimmed.Render(preview)
				if totalLines > 1 {
					toolContent += " " + theme.Dimmed.Render(fmt.Sprintf("[+%d lines · Ctrl+B expand]", totalLines-1))
				}
			}
		}

		b.WriteString(theme.ToolBox.Width(boxWidth).Render(toolContent))
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderMarkdown(theme *Theme, text string, width int) string {
	var b strings.Builder
	lines := strings.Split(text, "\n")
	inCodeBlock := false
	var codeLines []string

	flushCodeBlock := func() {
		if len(codeLines) == 0 {
			return
		}
		blockWidth := width - 8
		if blockWidth < 32 {
			blockWidth = 32
		}
		b.WriteString(theme.MarkdownCodeBlock.Width(blockWidth).Render(strings.Join(codeLines, "\n")))
		b.WriteString("\n")
		codeLines = nil
	}

	for _, rawLine := range lines {
		line := strings.TrimRight(rawLine, "\r")
		trimmed := strings.TrimSpace(line)

		if markdownCodeFenceRegex.MatchString(trimmed) {
			if inCodeBlock {
				flushCodeBlock()
				inCodeBlock = false
			} else {
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		if trimmed == "" {
			b.WriteString("\n")
			continue
		}

		if matches := markdownHeadingRegex.FindStringSubmatch(trimmed); len(matches) == 3 {
			content := renderInlineMarkdown(theme, matches[2])
			switch len(matches[1]) {
			case 1:
				b.WriteString("  " + theme.MarkdownHeading1.Render(content) + "\n")
			case 2:
				b.WriteString("  " + theme.MarkdownHeading2.Render(content) + "\n")
			default:
				b.WriteString("  " + theme.MarkdownHeading3.Render(content) + "\n")
			}
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			quote := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			b.WriteString("  " + theme.MarkdownQuote.Render("│ "+renderInlineMarkdown(theme, quote)) + "\n")
			continue
		}

		if matches := markdownOrderedRegex.FindStringSubmatch(trimmed); len(matches) == 3 {
			b.WriteString("  " + theme.MarkdownBullet.Render(matches[1]+".") + " " + renderInlineMarkdown(theme, matches[2]) + "\n")
			continue
		}

		if matches := markdownBulletRegex.FindStringSubmatch(trimmed); len(matches) == 2 {
			b.WriteString("  " + theme.MarkdownBullet.Render("•") + " " + renderInlineMarkdown(theme, matches[1]) + "\n")
			continue
		}

		if trimmed == "---" || trimmed == "***" {
			ruleWidth := width - 4
			if ruleWidth < 8 {
				ruleWidth = 8
			}
			b.WriteString("  " + theme.Separator.Render(strings.Repeat("─", ruleWidth)) + "\n")
			continue
		}

		b.WriteString("  " + renderInlineMarkdown(theme, line) + "\n")
	}

	if inCodeBlock {
		flushCodeBlock()
	}

	return strings.TrimRight(b.String(), "\n")
}

func renderInlineMarkdown(theme *Theme, line string) string {
	rendered := line

	rendered = markdownLinkRegex.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := markdownLinkRegex.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		return theme.MarkdownLink.Render(parts[1]) + " " + theme.Dimmed.Render("("+parts[2]+")")
	})

	rendered = markdownInlineCodeRegex.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := markdownInlineCodeRegex.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return theme.MarkdownCode.Render(parts[1])
	})

	rendered = markdownBoldRegex.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := markdownBoldRegex.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return theme.MarkdownBold.Render(parts[1])
	})

	rendered = markdownItalicRegex.ReplaceAllStringFunc(rendered, func(match string) string {
		parts := markdownItalicRegex.FindStringSubmatch(match)
		if len(parts) != 2 {
			return match
		}
		return theme.MarkdownItalic.Render(parts[1])
	})

	return rendered
}

// getPreviewLine returns the first non-empty line and total line count
func getPreviewLine(s string) (string, int) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	preview := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			preview = trimmed
			break
		}
	}
	if len(preview) > 100 {
		preview = preview[:100] + "..."
	}
	return preview, len(lines)
}

// renderFullResult renders the full tool result with proper formatting
func renderFullResult(theme *Theme, result string, maxWidth int) string {
	var b strings.Builder
	lines := strings.Split(result, "\n")
	for _, line := range lines {
		if len(line) > maxWidth && maxWidth > 0 {
			for len(line) > maxWidth {
				b.WriteString(line[:maxWidth] + "\n")
				line = line[maxWidth:]
			}
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
