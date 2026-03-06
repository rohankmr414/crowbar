package main

import (
	"strings"
)

// Command represents a server console command/cvar with an optional description.
type Command struct {
	Name        string
	Description string
}

// ParseFindOutput parses the output of the `find <prefix>` RCON command.
// The output format is lines like:
//
//	"sv_cheats" = "0" ( def. "0" ) archive notify replicated - If set to 1, ...
//	"sm_kick"                                                - sm_kick <#userid|name> [reason]
//
// We extract the command name and description from each line.
func ParseFindOutput(output string) []Command {
	var cmds []Command
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Lines start with a quoted command name.
		if !strings.HasPrefix(line, "\"") {
			continue
		}

		// Extract name between first pair of quotes.
		closeQuote := strings.Index(line[1:], "\"")
		if closeQuote < 0 {
			continue
		}
		name := line[1 : closeQuote+1]

		if name == "" || seen[name] {
			continue
		}
		seen[name] = true

		// Extract description after " - ".
		desc := ""
		dashIdx := strings.Index(line, " - ")
		if dashIdx >= 0 {
			desc = strings.TrimSpace(line[dashIdx+3:])
		}

		cmds = append(cmds, Command{Name: name, Description: desc})
	}

	return cmds
}

// FilterCommands returns commands from the given list whose name starts with the given prefix.
func FilterCommands(prefix string, cmds []Command) []Command {
	if prefix == "" || len(cmds) == 0 {
		return nil
	}
	prefix = strings.ToLower(prefix)
	var matches []Command
	for _, cmd := range cmds {
		if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			matches = append(matches, cmd)
		}
	}
	return matches
}
