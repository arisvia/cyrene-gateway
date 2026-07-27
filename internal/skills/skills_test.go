package skills

import "testing"

func TestList(t *testing.T) {
	list := List()
	if len(list) == 0 {
		t.Fatal("expected at least one skill")
	}

	// Check entry skill exists
	var found bool
	for _, s := range list {
		if s.ID == "cyrene" {
			found = true
			if s.Name != "cyrene" {
				t.Errorf("expected name 'cyrene', got %q", s.Name)
			}
			if s.Description == "" {
				t.Error("expected non-empty description for entry skill")
			}
			if s.Content == "" {
				t.Error("expected non-empty content for entry skill")
			}
		}
	}
	if !found {
		t.Error("entry skill 'cyrene' not found")
	}
}

func TestParseSkill(t *testing.T) {
	content := "---\nname: test-skill\ndescription: A test skill\n---\n\n# Test\n\nHello"
	s := parseSkill("test", content)
	if s.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", s.Name)
	}
	if s.Description != "A test skill" {
		t.Errorf("expected description 'A test skill', got %q", s.Description)
	}
}
