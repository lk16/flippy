// Package web serves flippy's admin website: static HTML page shells, with live data fetched client-side.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"net/http"

	"github.com/lk16/flippy/static"
	webassets "github.com/lk16/flippy/wasm/edax-eval"
)

//go:embed templates/*.html
var templatesFS embed.FS

// page is one entry in the sidebar.
type page struct {
	ID    string
	Title string
	Path  string
}

// pages lists every page in sidebar order; ID must match its template filename and route in Handler.
var pages = []page{
	{ID: "game", Title: "Board", Path: "/game"},
	{ID: "stats", Title: "Stats", Path: "/stats"},
	{ID: "clients", Title: "Clients", Path: "/clients"},
}

// pageData is the root template data every page renders with.
type pageData struct {
	Title  string
	Active string
	Pages  []page
}

// Server serves the admin website.
type Server struct {
	templates map[string]*template.Template
}

// NewServer parses every page's templates.
func NewServer() (*Server, error) {
	templates := make(map[string]*template.Template, len(pages))

	for _, p := range pages {
		tmpl, err := template.ParseFS(templatesFS, "templates/layout.html", "templates/"+p.ID+".html")
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s template: %w", p.ID, err)
		}
		templates[p.ID] = tmpl
	}

	return &Server{templates: templates}, nil
}

// render executes pageID's template into w.
func (s *Server) render(w http.ResponseWriter, pageID string) {
	tmpl, ok := s.templates[pageID]
	if !ok {
		http.Error(w, "page not found", http.StatusNotFound)
		return
	}

	data := pageData{Active: pageID, Pages: pages}
	for _, p := range pages {
		if p.ID == pageID {
			data.Title = p.Title
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handlePage returns a handler that renders pageID's static page shell.
func (s *Server) handlePage(pageID string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.render(w, pageID)
	}
}

// Handler returns the HTTP handler serving every page and static asset.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/game", http.StatusFound)
	})

	for _, p := range pages {
		mux.HandleFunc("GET "+p.Path, s.handlePage(p.ID))
	}

	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static.FS)))
	mux.Handle("GET /static/wasm/", http.StripPrefix("/static/wasm/", http.FileServerFS(webassets.FS)))

	return mux
}
