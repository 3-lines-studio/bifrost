package store

import (
	"errors"
	"sort"
	"strings"
	"sync"
)

type Entry struct {
	Author string
	Text   string
}

type Project struct {
	Slug    string
	Name    string
	Summary string
	Log     []Entry
}

type Doc struct {
	Title string
	Body  string
}

var (
	mu       sync.Mutex
	loads    = map[string]int{}
	projects = []Project{
		{Slug: "bifrost", Name: "Bifrost", Summary: "Go + Vite + React SSR framework.", Log: []Entry{{Author: "don-berti", Text: "App Router merged."}}},
		{Slug: "axe", Name: "Axe", Summary: "The coding agent embedded in Jimmy.", Log: []Entry{{Author: "jimmy", Text: "Telemetry sink wired."}}},
		{Slug: "wax", Name: "Wax", Summary: "Fetches pages and returns Markdown.", Log: []Entry{}},
	}
	docs = map[string]Doc{
		"":           {Title: "Docs", Body: "Pick a page on the left. Every page comes from the same catch-all route."},
		"markers":    {Title: "Directory markers", Body: "One trailing underscore captures one segment, two capture the rest, tilde makes a route group and a leading underscore keeps a folder private."},
		"loaders":    {Title: "Loaders", Body: "A page.go file exports Load, which returns a map or a bifrost.PageData. Params, search params and pathname are merged into the props."},
		"errors":     {Title: "Errors", Body: "A loader returns bifrost.NotFound, bifrost.Redirect or bifrost.Status, and the nearest error.tsx or not-found.tsx takes over."},
		"middleware": {Title: "Middleware", Body: "Every middleware.go wraps the routes below it, over pages and over route.go handlers alike."},
		"metadata":   {Title: "Metadata", Body: "Layouts and pages export metadata or generateMetadata and the objects merge from the outside in."},
	}
)

func Loads(route string) int {
	mu.Lock()
	defer mu.Unlock()
	loads[route]++
	return loads[route]
}

func Snapshot() map[string]int {
	mu.Lock()
	defer mu.Unlock()
	out := make(map[string]int, len(loads))
	for key, value := range loads {
		out[key] = value
	}
	return out
}

func Projects() []Project {
	mu.Lock()
	defer mu.Unlock()
	out := make([]Project, len(projects))
	copy(out, projects)
	return out
}

func FindProject(slug string) (Project, bool) {
	mu.Lock()
	defer mu.Unlock()
	for _, project := range projects {
		if project.Slug == slug {
			return project, true
		}
	}
	return Project{}, false
}

func AddLog(slug, author, text string) (Project, error) {
	mu.Lock()
	defer mu.Unlock()
	if strings.TrimSpace(text) == "" {
		return Project{}, errors.New("empty log entry")
	}
	for index := range projects {
		if projects[index].Slug != slug {
			continue
		}
		projects[index].Log = append(projects[index].Log, Entry{Author: author, Text: text})
		return projects[index], nil
	}
	return Project{}, errors.New("unknown project")
}

func DocPaths() []string {
	mu.Lock()
	defer mu.Unlock()
	paths := make([]string, 0, len(docs))
	for path := range docs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func FindDoc(path string) (Doc, bool) {
	mu.Lock()
	defer mu.Unlock()
	doc, ok := docs[strings.Trim(path, "/")]
	return doc, ok
}
