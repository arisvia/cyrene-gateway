package skills

import (
	"embed"
	"io/fs"
	"strings"
)

//go:embed all:data
var dataFS embed.FS

// Skill represents a bundled skill manifest.
type Skill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}

// List returns all bundled skills parsed from embedded SKILL.md files.
func List() []Skill {
	var skills []Skill

	entries, err := fs.ReadDir(dataFS, "data")
	if err != nil {
		return skills
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		content, err := fs.ReadFile(dataFS, "data/"+entry.Name()+"/SKILL.md")
		if err != nil {
			continue
		}
		s := parseSkill(entry.Name(), string(content))
		skills = append(skills, s)
	}
	return skills
}

func parseSkill(id, content string) Skill {
	s := Skill{ID: id, Content: content}

	// Parse YAML frontmatter
	if strings.HasPrefix(content, "---\n") {
		end := strings.Index(content[4:], "\n---")
		if end > 0 {
			fm := content[4 : 4+end]
			for _, line := range strings.Split(fm, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "name:") {
					s.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
				} else if strings.HasPrefix(line, "description:") {
					s.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}

	if s.Name == "" {
		s.Name = id
	}
	return s
}
