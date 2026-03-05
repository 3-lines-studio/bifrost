package content

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark"
	meta "github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/parser"
)

type Page struct {
	Slug        string
	Title       string
	Description string
	Order       int
	HTML        string
}

type NavItem struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Active bool   `json:"active"`
}

type Loader struct {
	dir string
}

func NewLoader(dir string) *Loader {
	return &Loader{dir: dir}
}

func (l *Loader) LoadAll() ([]Page, error) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("reading content dir %s: %w", l.dir, err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(meta.Meta),
	)

	var pages []Page
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(l.dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		ctx := parser.NewContext()
		var buf bytes.Buffer
		if err := md.Convert(raw, &buf, parser.WithContext(ctx)); err != nil {
			return nil, fmt.Errorf("converting %s: %w", entry.Name(), err)
		}

		metadata := meta.Get(ctx)
		slug := strings.TrimSuffix(entry.Name(), ".md")

		page := Page{
			Slug:        slug,
			Title:       stringMeta(metadata, "title", slug),
			Description: stringMeta(metadata, "description", ""),
			Order:       intMeta(metadata, "order", 999),
			HTML:        buf.String(),
		}
		pages = append(pages, page)
	}

	sort.Slice(pages, func(i, j int) bool {
		if pages[i].Order != pages[j].Order {
			return pages[i].Order < pages[j].Order
		}
		return pages[i].Slug < pages[j].Slug
	})

	return pages, nil
}

func BuildNav(pages []Page) []NavItem {
	nav := make([]NavItem, len(pages))
	for i, p := range pages {
		nav[i] = NavItem{
			Slug:  p.Slug,
			Title: p.Title,
		}
	}
	return nav
}

func stringMeta(m map[string]interface{}, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return fallback
}

func intMeta(m map[string]interface{}, key string, fallback int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case float64:
			return int(n)
		}
	}
	return fallback
}
