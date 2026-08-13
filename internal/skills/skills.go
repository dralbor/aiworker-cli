// Package skills creates and lists Claude Code skills laid out by category
// folder (skills/frontend, skills/backend, skills/types, ...), matching the
// convention Claude Code itself uses for skill discovery.
package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Root returns the local skills directory: ~/.claude/skills.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "skills"), nil
}

// Category groups skills found under one category folder.
type Category struct {
	Name   string
	Skills []Skill
}

// Skill is one skill folder: root/<category>/<name>/SKILL.md.
type Skill struct {
	Name string
	Path string
}

// List walks root and groups every skill it finds by its immediate parent
// folder (the category). Skills directly under root (no category) are
// grouped under "(sin categoria)".
func List(root string) ([]Category, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	byCategory := map[string][]Skill{}

	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(root, e.Name())
		if _, err := os.Stat(filepath.Join(full, "SKILL.md")); err == nil {
			byCategory["(sin categoria)"] = append(byCategory["(sin categoria)"], Skill{Name: e.Name(), Path: full})
			continue
		}
		// Not a skill itself: a category folder, one level deep. Register it
		// even if it turns out to have zero skills (e.g. just created with
		// "f", nothing inside yet but a .gitkeep).
		if _, ok := byCategory[e.Name()]; !ok {
			byCategory[e.Name()] = nil
		}
		items, err := os.ReadDir(full)
		if err != nil {
			continue
		}
		for _, item := range items {
			if !item.IsDir() || strings.HasPrefix(item.Name(), ".") {
				continue
			}
			skillDir := filepath.Join(full, item.Name())
			if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); err == nil {
				byCategory[e.Name()] = append(byCategory[e.Name()], Skill{Name: item.Name(), Path: skillDir})
			}
		}
	}

	var cats []string
	for c := range byCategory {
		cats = append(cats, c)
	}
	sort.Strings(cats)

	out := make([]Category, 0, len(cats))
	for _, c := range cats {
		skillsInCat := byCategory[c]
		sort.Slice(skillsInCat, func(i, j int) bool { return skillsInCat[i].Name < skillsInCat[j].Name })
		out = append(out, Category{Name: c, Skills: skillsInCat})
	}
	return out, nil
}

var slugRe = regexp.MustCompile(`[^a-z0-9-]+`)

// Slug lowercases and hyphenates free text into a filesystem-safe folder name.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = slugRe.ReplaceAllString(s, "")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "sin-nombre"
	}
	return s
}

// New scaffolds root/category/name/SKILL.md from a minimal template. It
// refuses to overwrite an existing skill.
func New(root, category, name string) (string, error) {
	category = Slug(category)
	name = Slug(name)

	dir := filepath.Join(root, category, name)
	if _, err := os.Stat(dir); err == nil {
		return "", fmt.Errorf("ya existe %s", dir)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creando %s: %w", dir, err)
	}

	template := fmt.Sprintf(`---
name: %s
description: TODO - una linea: cuando se deberia usar esta skill?
---

# %s

TODO: instrucciones para esta skill.
`, name, strings.Title(strings.ReplaceAll(name, "-", " ")))

	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(template), 0o644); err != nil {
		return "", fmt.Errorf("escribiendo %s: %w", skillPath, err)
	}
	return dir, nil
}

// NewCategory scaffolds an empty root/category folder with a .gitkeep
// placeholder so it exists (and is git-trackable, and visible in List) even
// before any skill lives in it. Idempotent: an already-existing category is
// left as-is (just makes sure the placeholder is there).
func NewCategory(root, category string) (string, error) {
	category = Slug(category)
	dir := filepath.Join(root, category)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creando %s: %w", dir, err)
	}
	keep := filepath.Join(dir, ".gitkeep")
	if _, err := os.Stat(keep); os.IsNotExist(err) {
		if err := os.WriteFile(keep, nil, 0o644); err != nil {
			return "", fmt.Errorf("escribiendo %s: %w", keep, err)
		}
	}
	return dir, nil
}
