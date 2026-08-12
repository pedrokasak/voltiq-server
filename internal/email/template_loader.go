package email

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"strings"
)

//go:embed templates/*.html
var templateFS embed.FS

// TemplateLoader loads and renders email templates
type TemplateLoader struct {
	templates map[string]*template.Template
}

// NewTemplateLoader creates a new template loader
func NewTemplateLoader() (*TemplateLoader, error) {
	loader := &TemplateLoader{
		templates: make(map[string]*template.Template),
	}

	// Load all templates
	entries, err := fs.ReadDir(templateFS, "templates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".html") {
			name := strings.TrimSuffix(entry.Name(), ".html")
			content, err := templateFS.ReadFile("templates/" + entry.Name())
			if err != nil {
				return nil, fmt.Errorf("failed to read template %s: %w", entry.Name(), err)
			}

			tmpl, err := template.New(name).Parse(string(content))
			if err != nil {
				return nil, fmt.Errorf("failed to parse template %s: %w", entry.Name(), err)
			}

			loader.templates[name] = tmpl
		}
	}

	return loader, nil
}

// Render renders a template with the given data
func (l *TemplateLoader) Render(name string, data any) (string, error) {
	tmpl, ok := l.templates[name]
	if !ok {
		return "", fmt.Errorf("template not found: %s", name)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", name, err)
	}

	return buf.String(), nil
}

// GetDunningTemplateName returns the template name for a dunning stage
func GetDunningTemplateName(stage int) string {
	switch stage {
	case 1:
		return "dunning_stage_1"
	case 2:
		return "dunning_stage_2"
	case 3:
		return "dunning_stage_3"
	default:
		return "dunning_stage_1"
	}
}