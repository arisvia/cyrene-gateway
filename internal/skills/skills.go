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
			for line := range strings.SplitSeq(fm, "\n") {
				line = strings.TrimSpace(line)
				if after, ok :=strings.CutPrefix(line, "name:"); ok  {
					s.Name = strings.TrimSpace(after)
				} else if after, ok :=strings.CutPrefix(line, "description:"); ok  {
					s.Description = strings.TrimSpace(after)
				}
			}
		}
	}

	if s.Name == "" {
		s.Name = id
	}
	return s
}
