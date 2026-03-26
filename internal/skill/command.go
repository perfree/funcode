package skill

import (
	"fmt"
	"strings"
)

type CommandAction string

const (
	CommandNone  CommandAction = ""
	CommandShow  CommandAction = "show"
	CommandUse   CommandAction = "use"
	CommandClear CommandAction = "clear"
	CommandRun   CommandAction = "run"
	CommandHelp  CommandAction = "help"
)

type Command struct {
	Action CommandAction
	Name   string
	Task   string
}

func ParseCommand(input string) (Command, bool) {
	input = strings.TrimSpace(input)
	if input == "/skills" {
		return Command{Action: CommandShow}, true
	}
	if !strings.HasPrefix(input, "/skill") {
		return Command{}, false
	}

	fields := strings.Fields(input)
	if len(fields) == 1 {
		return Command{Action: CommandHelp}, true
	}

	switch fields[1] {
	case "show":
		if len(fields) < 3 {
			return Command{Action: CommandHelp}, true
		}
		return Command{Action: CommandShow, Name: fields[2]}, true
	case "use":
		if len(fields) < 3 {
			return Command{Action: CommandHelp}, true
		}
		return Command{Action: CommandUse, Name: fields[2]}, true
	case "clear":
		return Command{Action: CommandClear}, true
	case "run":
		if len(fields) < 4 {
			return Command{Action: CommandHelp}, true
		}
		task := strings.TrimSpace(strings.TrimPrefix(input, "/skill run "+fields[2]))
		return Command{Action: CommandRun, Name: fields[2], Task: task}, true
	default:
		return Command{Action: CommandHelp}, true
	}
}

func HelpText(active *Skill) string {
	var b strings.Builder
	b.WriteString("Skill commands:\n")
	b.WriteString("- /skills\n")
	b.WriteString("- /skill show <name>\n")
	b.WriteString("- /skill use <name>\n")
	b.WriteString("- /skill run <name> <task>\n")
	b.WriteString("- /skill clear\n")
	if active != nil {
		b.WriteString(fmt.Sprintf("\nActive skill: %s", active.Name))
	}
	return b.String()
}
