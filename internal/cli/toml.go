package cli

import (
	"fmt"
	"regexp"
	"strings"
)

// This file implements a minimal, section-aware TOML line editor. It is not a
// full TOML parser — it only supports the subset of operations the CLI tool
// adapters need: reading/writing string and integer fields within [section]
// blocks while preserving the rest of the file. This mirrors the upstream
// 9router approach (regex-based section editing) and avoids a third-party
// TOML dependency.

// tomlString quotes a value as a TOML basic string.
func tomlString(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range v {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

var tomlSectionRe = regexp.MustCompile(`^\[([^\]]+)\]\s*$`)

// tomlSectionBounds returns the [start,end) line indices of a section's body
// (the lines after the header up to the next section header or EOF). The
// header index itself is returned as headerIdx (-1 if absent).
func tomlSectionBounds(lines []string, section string) (headerIdx, bodyStart, bodyEnd int) {
	headerIdx = -1
	for i, line := range lines {
		if m := tomlSectionRe.FindStringSubmatch(strings.TrimSpace(line)); m != nil && m[1] == section {
			headerIdx = i
			bodyStart = i + 1
			break
		}
	}
	if headerIdx == -1 {
		return -1, -1, -1
	}
	bodyEnd = len(lines)
	for i := bodyStart; i < len(lines); i++ {
		if tomlSectionRe.MatchString(strings.TrimSpace(lines[i])) {
			bodyEnd = i
			break
		}
	}
	return headerIdx, bodyStart, bodyEnd
}

// tomlGetField reads a string field from a section.
func tomlGetField(content, section, key string) (string, bool) {
	lines := strings.Split(content, "\n")
	_, bodyStart, bodyEnd := tomlSectionBounds(lines, section)
	if bodyStart == -1 {
		return "", false
	}
	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	for i := bodyStart; i < bodyEnd; i++ {
		if m := re.FindStringSubmatch(lines[i]); m != nil {
			return m[1], true
		}
	}
	return "", false
}

// tomlSetField sets a string field within a section, creating the section and
// field if necessary.
func tomlSetField(content, section, key, value string) string {
	lines := strings.Split(content, "\n")
	headerIdx, bodyStart, bodyEnd := tomlSectionBounds(lines, section)
	line := fmt.Sprintf("%s = %s", key, tomlString(value))

	if headerIdx == -1 {
		// Append a new section at the end.
		prefix := content
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		return prefix + "\n[" + section + "]\n" + line + "\n"
	}

	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*`)
	for i := bodyStart; i < bodyEnd; i++ {
		if re.MatchString(lines[i]) {
			lines[i] = line
			return strings.Join(lines, "\n")
		}
	}
	// Insert at the top of the section body.
	newLines := append([]string{}, lines[:bodyStart]...)
	newLines = append(newLines, line)
	newLines = append(newLines, lines[bodyStart:]...)
	return strings.Join(newLines, "\n")
}

// tomlSetIntField sets an integer field within a section.
func tomlSetIntField(content, section, key string, value int) string {
	lines := strings.Split(content, "\n")
	headerIdx, bodyStart, bodyEnd := tomlSectionBounds(lines, section)
	line := fmt.Sprintf("%s = %d", key, value)

	if headerIdx == -1 {
		prefix := content
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			prefix += "\n"
		}
		return prefix + "\n[" + section + "]\n" + line + "\n"
	}

	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*`)
	for i := bodyStart; i < bodyEnd; i++ {
		if re.MatchString(lines[i]) {
			lines[i] = line
			return strings.Join(lines, "\n")
		}
	}
	newLines := append([]string{}, lines[:bodyStart]...)
	newLines = append(newLines, line)
	newLines = append(newLines, lines[bodyStart:]...)
	return strings.Join(newLines, "\n")
}

// tomlDeleteSection removes an entire [section] block (header + body).
func tomlDeleteSection(content, section string) string {
	lines := strings.Split(content, "\n")
	headerIdx, _, bodyEnd := tomlSectionBounds(lines, section)
	if headerIdx == -1 {
		return content
	}
	newLines := append([]string{}, lines[:headerIdx]...)
	newLines = append(newLines, lines[bodyEnd:]...)
	out := strings.Join(newLines, "\n")
	return regexp.MustCompile(`\n{3,}`).ReplaceAllString(out, "\n\n")
}

// tomlDeleteField removes a single key within a section.
func tomlDeleteField(content, section, key string) string {
	lines := strings.Split(content, "\n")
	_, bodyStart, bodyEnd := tomlSectionBounds(lines, section)
	if bodyStart == -1 {
		return content
	}
	re := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=\s*[^\r\n]*\r?\n?`)
	var newLines []string
	for i, line := range lines {
		if i >= bodyStart && i < bodyEnd && re.MatchString(line) {
			continue
		}
		newLines = append(newLines, line)
	}
	return strings.Join(newLines, "\n")
}

// tomlGetTopLevel reads a top-level (before any section) string field.
func tomlGetTopLevel(content, key string) (string, bool) {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`)
	if m := re.FindStringSubmatch(content); m != nil {
		return m[1], true
	}
	return "", false
}

// tomlSetTopLevel sets a top-level string field (before any section header).
func tomlSetTopLevel(content, key, value string) string {
	line := fmt.Sprintf("%s = %s", key, tomlString(value))
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=\s*[^\r\n]*`)
	if re.MatchString(content) {
		return re.ReplaceAllString(content, line)
	}
	// Insert before the first section header, or at the top.
	if idx := strings.Index(content, "\n["); idx >= 0 {
		return content[:idx] + "\n" + line + content[idx:]
	}
	if strings.HasPrefix(strings.TrimLeft(content, "\n"), "[") {
		return line + "\n" + content
	}
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content + line + "\n"
}

// tomlDeleteTopLevel removes a top-level field.
func tomlDeleteTopLevel(content, key string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `\s*=\s*[^\r\n]*\r?\n?`)
	return re.ReplaceAllString(content, "")
}

// --- .env file helpers ---

// envUpsert sets KEY=value within env file text, replacing an existing line.
func envUpsert(envText, key, value string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=.*$`)
	line := key + "=" + value
	if re.MatchString(envText) {
		return re.ReplaceAllString(envText, line)
	}
	if envText != "" && !strings.HasSuffix(envText, "\n") {
		envText += "\n"
	}
	return envText + line + "\n"
}

// envRemove deletes a KEY=... line from env file text.
func envRemove(envText, key string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=.*\r?\n?`)
	return re.ReplaceAllString(envText, "")
}
