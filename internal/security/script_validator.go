package security

import (
	"bufio"
	"fmt"
	"strings"
)

// ScriptValidator validates script content.
type ScriptValidator struct {
	blockedCommands []string
	allowedCommands []string
	maxSize         int
	maxLines        int
}

// NewScriptValidator creates a script validator.
func NewScriptValidator(blocked, allowed []string, maxSize, maxLines int) *ScriptValidator {
	if maxSize <= 0 {
		maxSize = 524288 // 500KB
	}
	if maxLines <= 0 {
		maxLines = 1000
	}
	return &ScriptValidator{
		blockedCommands: blocked,
		allowedCommands: allowed,
		maxSize:         maxSize,
		maxLines:        maxLines,
	}
}

// Validate checks script content.
func (v *ScriptValidator) Validate(script string) error {
	if len(script) > v.maxSize {
		return fmt.Errorf("script too large: %d bytes (max: %d)", len(script), v.maxSize)
	}

	lines := strings.Split(script, "\n")
	if len(lines) > v.maxLines {
		return fmt.Errorf("script too long: %d lines (max: %d)", len(lines), v.maxLines)
	}

	commands := v.extractCommands(script)

	for _, cmd := range commands {
		for _, blocked := range v.blockedCommands {
			if v.matchesCommand(cmd, blocked) {
				return fmt.Errorf("blocked command detected: %s", cmd)
			}
		}
	}

	if len(v.allowedCommands) > 0 {
		for _, cmd := range commands {
			allowed := false
			for _, a := range v.allowedCommands {
				if v.matchesCommand(cmd, a) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("command not in allowlist: %s", cmd)
			}
		}
	}

	return nil
}

func (v *ScriptValidator) extractCommands(script string) []string {
	commands := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(script))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		cmd := fields[0]
		cmd = strings.TrimPrefix(cmd, "|")
		cmd = strings.TrimPrefix(cmd, "&&")
		cmd = strings.TrimPrefix(cmd, "||")
		cmd = strings.TrimPrefix(cmd, ";")
		if cmd == "sudo" || cmd == "su" {
			if len(fields) > 1 {
				cmd = fields[1]
			}
		}
		commands[cmd] = true
	}

	out := make([]string, 0, len(commands))
	for c := range commands {
		out = append(out, c)
	}
	return out
}

func (v *ScriptValidator) matchesCommand(cmd, pattern string) bool {
	if cmd == pattern {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(cmd, prefix)
	}
	return false
}
