package skills

import (
	"os"
	"testing"
)

func TestNewAndList(t *testing.T) {
	root := t.TempDir()

	path, err := New(root, "Frontend", "React Patterns")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := os.Stat(path + string(os.PathSeparator) + "SKILL.md"); err != nil {
		t.Fatalf("SKILL.md not created: %v", err)
	}

	if _, err := New(root, "frontend", "react-patterns"); err == nil {
		t.Error("expected New to refuse to overwrite an existing skill")
	}

	if _, err := New(root, "Backend", "Go Services"); err != nil {
		t.Fatalf("New (second category): %v", err)
	}

	cats, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories, got %d: %+v", len(cats), cats)
	}
	if cats[0].Name != "backend" || len(cats[0].Skills) != 1 || cats[0].Skills[0].Name != "go-services" {
		t.Errorf("unexpected backend category: %+v", cats[0])
	}
	if cats[1].Name != "frontend" || len(cats[1].Skills) != 1 || cats[1].Skills[0].Name != "react-patterns" {
		t.Errorf("unexpected frontend category: %+v", cats[1])
	}
}

func TestNewCategoryShowsUpEvenEmpty(t *testing.T) {
	root := t.TempDir()

	path, err := New(root, "Backend", "Go Services")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = path

	emptyPath, err := NewCategory(root, "Types")
	if err != nil {
		t.Fatalf("NewCategory: %v", err)
	}
	if _, err := os.Stat(emptyPath + string(os.PathSeparator) + ".gitkeep"); err != nil {
		t.Fatalf(".gitkeep not created: %v", err)
	}

	// Idempotent: calling it again on the same category must not error.
	if _, err := NewCategory(root, "types"); err != nil {
		t.Fatalf("NewCategory should be idempotent: %v", err)
	}

	cats, err := List(root)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cats) != 2 {
		t.Fatalf("expected 2 categories (backend with a skill, types empty), got %d: %+v", len(cats), cats)
	}
	if cats[0].Name != "backend" || len(cats[0].Skills) != 1 {
		t.Errorf("unexpected backend category: %+v", cats[0])
	}
	if cats[1].Name != "types" || len(cats[1].Skills) != 0 {
		t.Errorf("expected empty types category to still show up, got: %+v", cats[1])
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"React Patterns": "react-patterns",
		"  trim me  ":    "trim-me",
		"":               "sin-nombre",
		"Go/Services!!":  "goservices",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}
