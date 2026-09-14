package trails

import (
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"strings"
)

type Renderer interface {
	Render(c *Context, w io.Writer, name string, data any) error
}

type templateRenderer struct {
	templates  map[string]*template.Template
	layoutName string
}

func newTemplateRenderer(viewFs fs.FS, layoutName string, funcMap template.FuncMap) (Renderer, error) {
	layoutMatches, err := fs.Glob(viewFs, "layouts/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("renderer: glob layouts: %w", err)
	}

	var layoutFile string
	layoutFiles := []string{}
	for _, file := range layoutMatches {
		base := path.Base(file)
		if strings.HasPrefix(base, "_") {
			layoutFiles = append(layoutFiles, file)
			continue
		}
		if strings.TrimSuffix(base, ".gohtml") == layoutName {
			layoutFile = file
		}
	}
	if layoutFile != "" {
		layoutFiles = append(layoutFiles, layoutFile)
	}

	entries, err := fs.ReadDir(viewFs, ".")
	if err != nil {
		return nil, fmt.Errorf("renderer: read views root: %w", err)
	}

	templates := map[string]*template.Template{}

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "layouts" {
			continue
		}
		controller := entry.Name()
		controllerEntries, err := fs.ReadDir(viewFs, controller)
		if err != nil {
			return nil, fmt.Errorf("renderer: read %s: %w", controller, err)
		}

		var partials []string
		var actions []string

		for _, ce := range controllerEntries {
			if ce.IsDir() || !strings.HasSuffix(ce.Name(), ".gohtml") {
				continue
			}
			file := path.Join(controller, ce.Name())
			if strings.HasPrefix(ce.Name(), "_") {
				partials = append(partials, file)
			} else {
				actions = append(actions, file)
			}
		}

		for _, action := range actions {
			if layoutFile == "" {
				return nil, fmt.Errorf("renderer: layout %q not found in layouts", layoutName)
			}

			name := strings.TrimSuffix(path.Base(action), ".gohtml")
			key := controller + "/" + name

			files := make([]string, 0, len(layoutFiles)+len(partials)+1)
			files = append(files, layoutFiles...)
			files = append(files, partials...)
			files = append(files, action)

			tmpl, err := template.New(name).Funcs(funcMap).ParseFS(viewFs, files...)
			if err != nil {
				return nil, fmt.Errorf("renderer: parse %s: %w", key, err)
			}
			templates[key] = tmpl
		}
	}

	return &templateRenderer{
		templates:  templates,
		layoutName: layoutName,
	}, nil
}

func (t *templateRenderer) Render(c *Context, w io.Writer, name string, data any) error {
	tmpl, ok := t.templates[name]
	if !ok {
		return fmt.Errorf("renderer: template %q not found", name)
	}

	return tmpl.ExecuteTemplate(w, t.layoutName, data)
}
