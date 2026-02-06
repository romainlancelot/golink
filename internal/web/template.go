package web

import (
	"embed"
	"html/template"
	"io"
	"sort"
)

//go:embed templates/index.html
var templateFS embed.FS

var indexTmpl = template.Must(template.ParseFS(templateFS, "templates/index.html"))

// linkEntry is a single link for template rendering.
type linkEntry struct {
	Key string
	URL string
}

// pageData holds the data passed to the HTML template.
type pageData struct {
	PrefilledKey string
	Links        []linkEntry
}

// renderTemplate writes the rendered index page to w.
func renderTemplate(w io.Writer, prefilledKey string, links map[string]string) error {
	sorted := make([]linkEntry, 0, len(links))
	for k, v := range links {
		sorted = append(sorted, linkEntry{Key: k, URL: v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Key < sorted[j].Key
	})

	return indexTmpl.Execute(w, pageData{
		PrefilledKey: prefilledKey,
		Links:        sorted,
	})
}
