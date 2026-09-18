package trails

import (
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Renderer renders views. The only implementation is trails' own
// html/template-based one, built internally from ViewFS/LayoutName;
// there is currently no supported way to substitute a different one.
type Renderer interface {
	// Render renders name (the "controller/action" template set) into w,
	// executing the configured layout around it. c is used only when a
	// RequestFuncMap is configured, to build that request's extra
	// template functions.
	Render(c *Context, w io.Writer, name string, data any) error

	// RenderNamed renders the named block ({{define "block"}}...{{end}})
	// belonging to action's template set to a string, without executing
	// the layout. It backs Context.RenderBlock.
	RenderNamed(action, block string, data any) (template.HTML, error)
}

type templateRenderer struct {
	templates map[string]*template.Template
	// execTemplates holds a freely-executable copy of each action's
	// template, used by RenderNamed/RenderBlock instead of templates when
	// requestFuncMap is set. Render clones templates[name] per request (to
	// bind request-scoped functions); html/template forbids Clone on a
	// template that has ever been Execute'd, so RenderNamed must never
	// execute the same object Render clones from — hence the split copy.
	execTemplates  map[string]*template.Template
	layoutName     string
	requestFuncMap func(c *Context) template.FuncMap
}

func newTemplateRenderer(viewFs fs.FS, layoutName string, funcMap template.FuncMap, requestFuncMap func(c *Context) template.FuncMap) (Renderer, error) {
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
	var execTemplates map[string]*template.Template
	if requestFuncMap != nil {
		execTemplates = map[string]*template.Template{}
	}

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

			if requestFuncMap != nil {
				execCopy, err := tmpl.Clone()
				if err != nil {
					return nil, fmt.Errorf("renderer: clone %s for direct execution: %w", key, err)
				}
				execTemplates[key] = execCopy
			}
		}
	}

	return &templateRenderer{
		templates:      templates,
		execTemplates:  execTemplates,
		layoutName:     layoutName,
		requestFuncMap: requestFuncMap,
	}, nil
}

func (t *templateRenderer) Render(c *Context, w io.Writer, name string, data any) error {
	tmpl, ok := t.templates[name]
	if !ok {
		return fmt.Errorf("renderer: template %q not found", name)
	}

	if t.requestFuncMap == nil {
		return tmpl.ExecuteTemplate(w, t.layoutName, data)
	}

	clone, err := tmpl.Clone()
	if err != nil {
		return fmt.Errorf("renderer: clone %s: %w", name, err)
	}

	return clone.Funcs(t.requestFuncMap(c)).ExecuteTemplate(w, t.layoutName, data)
}

func (t *templateRenderer) RenderNamed(action, block string, data any) (template.HTML, error) {
	templates := t.templates
	if t.requestFuncMap != nil {
		templates = t.execTemplates
	}

	tmpl, ok := templates[action]
	if !ok {
		return "", fmt.Errorf("renderer: template %q not found", action)
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, block, data); err != nil {
		return "", fmt.Errorf("renderer: block %q in %q: %w", block, action, err)
	}

	return template.HTML(buf.String()), nil
}
