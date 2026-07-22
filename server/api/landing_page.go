package api

import (
	_ "embed"
	"html/template"
	"net/http"
)

//go:embed landing_page.html
var landingPageHTML string

var landingPageTmpl = template.Must(template.New("landing").Parse(landingPageHTML))

type landingPageData struct {
	Title   string
	Content template.HTML
}

// LandingPage serves a configurable HTML landing page at /.
func (s *APIServer) LandingPage(w http.ResponseWriter, r *http.Request) {
	lp := s.erupeConfig.API.LandingPage
	if !lp.Enabled {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := landingPageData{
		Title:   lp.Title,
		Content: template.HTML(lp.Content),
	}
	if err := landingPageTmpl.Execute(w, data); err != nil {
		s.logger.Error("Failed to render landing page")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}
