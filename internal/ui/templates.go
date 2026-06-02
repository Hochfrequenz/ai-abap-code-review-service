// internal/ui/templates.go
package ui

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"

	"github.com/hochfrequenz/go-sap-btp-cf-template/internal/reviewstore"
)

//go:embed templates/*
var templateFS embed.FS

// Templates holds the parsed template set.
type Templates struct {
	t *template.Template
}

// MustLoadTemplates parses all embedded templates and panics on error.
// Call once at startup.
func MustLoadTemplates() Templates {
	t := template.New("").Funcs(template.FuncMap{
		// safeHTML allows goldmark-rendered HTML to pass through unescaped.
		// Input is always from the Claude API via goldmark, not from user input.
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	})
	var err error
	t, err = t.ParseFS(templateFS, "templates/*")
	if err != nil {
		panic(fmt.Sprintf("parse templates: %v", err))
	}
	return Templates{t: t}
}

// RenderStatus renders the _status.html fragment for the given job state.
func (tmpl Templates) RenderStatus(job *reviewstore.Job) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "_status.html", job); err != nil {
		return "", fmt.Errorf("render _status.html: %w", err)
	}
	return buf.String(), nil
}

// RenderIndex renders the index.html page.
func (tmpl Templates) RenderIndex() (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "index.html", nil); err != nil {
		return "", fmt.Errorf("render index.html: %w", err)
	}
	return buf.String(), nil
}

// RenderReview renders the review.html page with the current job embedded.
func (tmpl Templates) RenderReview(job *reviewstore.Job) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "review.html", job); err != nil {
		return "", fmt.Errorf("render review.html: %w", err)
	}
	return buf.String(), nil
}
