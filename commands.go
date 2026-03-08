package main

import (
	"strings"

	"github.com/sahilm/fuzzy"
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

// ParseCvarlistOutput parses the output of the `cvarlist` RCON command.
// The output format is lines like:
//
//	sv_cheats                      : 0          : , "notify", "rep"     : Allow cheats on server
//	mp_autoteambalance             : 1          :                      :
//	askconnect_accept              : cmd        :                      : Accept a redirect request by the server.
//
// Each line has the format: name : value_or_cmd : flags : description
// The last line is typically a summary like "total convars/concommands".
func ParseCvarlistOutput(output string) []Command {
	var cmds []Command
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Skip summary/footer lines (e.g. "350 total convars/concommands")
		if strings.Contains(line, "total convars") || strings.Contains(line, "total concommands") {
			continue
		}

		// Split into at most 4 parts on " : "
		parts := strings.SplitN(line, " : ", 4)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		if name == "" || seen[name] {
			continue
		}

		// Skip lines that start with `-` (sometimes footer separators)
		if strings.HasPrefix(name, "-") {
			continue
		}

		seen[name] = true

		desc := ""
		if len(parts) >= 4 {
			desc = strings.TrimSpace(parts[3])
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

// RankCommandsFuzzy returns commands ranked by fuzzy relevance using sahilm/fuzzy.
func RankCommandsFuzzy(query string, cmds []Command) []Command {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" || len(cmds) == 0 {
		return nil
	}

	names := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		names = append(names, strings.ToLower(cmd.Name))
	}

	matches := fuzzy.Find(query, names)
	if len(matches) == 0 {
		return nil
	}

	out := make([]Command, 0, len(matches))
	for _, m := range matches {
		out = append(out, cmds[m.Index])
	}
	return out
}
