package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"path"
	"strings"
	texttemplate "text/template"
)

type Renderer interface {
	Render(name string, data any) (html string, text string, err error)
}

type renderer struct {
	html       map[string]*template.Template
	text       map[string]*texttemplate.Template
	layoutName string
}

const textSuffix = ".text.gohtml"

func NewRenderer(viewFS fs.FS, layoutName string, funcMap template.FuncMap) (Renderer, error) {
	layoutMatches, err := fs.Glob(viewFS, "layouts/*.gohtml")
	if err != nil {
		return nil, fmt.Errorf("mail: glob layouts: %w", err)
	}

	var layoutFile string
	var layoutFiles []string
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

	entries, err := fs.ReadDir(viewFS, ".")
	if err != nil {
		return nil, fmt.Errorf("mail: read views root: %w", err)
	}

	htmlTemplates := map[string]*template.Template{}
	textTemplates := map[string]*texttemplate.Template{}
	textFuncMap := texttemplate.FuncMap(funcMap)

	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "layouts" {
			continue
		}
		mailerName := entry.Name()

		mailerEntries, err := fs.ReadDir(viewFS, mailerName)
		if err != nil {
			return nil, fmt.Errorf("mail: read %s: %w", mailerName, err)
		}

		var partials []string
		var actions []string
		textFiles := map[string]string{}

		for _, me := range mailerEntries {
			if me.IsDir() || !strings.HasSuffix(me.Name(), ".gohtml") {
				continue
			}
			file := path.Join(mailerName, me.Name())

			if strings.HasSuffix(me.Name(), textSuffix) {
				action := strings.TrimSuffix(me.Name(), textSuffix)
				textFiles[action] = file
				continue
			}

			if strings.HasPrefix(me.Name(), "_") {
				partials = append(partials, file)
				continue
			}

			actions = append(actions, file)
		}

		for _, action := range actions {
			if layoutFile == "" {
				return nil, fmt.Errorf("mail: layout %q not found in layouts", layoutName)
			}

			name := strings.TrimSuffix(path.Base(action), ".gohtml")
			key := mailerName + "/" + name

			files := make([]string, 0, len(layoutFiles)+len(partials)+1)
			files = append(files, layoutFiles...)
			files = append(files, partials...)
			files = append(files, action)

			tmpl, err := template.New(name).Funcs(funcMap).ParseFS(viewFS, files...)
			if err != nil {
				return nil, fmt.Errorf("mail: parse %s: %w", key, err)
			}
			htmlTemplates[key] = tmpl

			if textFile, ok := textFiles[name]; ok {
				textTmpl, err := texttemplate.New(path.Base(textFile)).Funcs(textFuncMap).ParseFS(viewFS, textFile)
				if err != nil {
					return nil, fmt.Errorf("mail: parse %s: %w", key, err)
				}
				textTemplates[key] = textTmpl
			}
		}
	}

	return &renderer{html: htmlTemplates, text: textTemplates, layoutName: layoutName}, nil
}

func (r *renderer) Render(name string, data any) (string, string, error) {
	tmpl, ok := r.html[name]
	if !ok {
		return "", "", fmt.Errorf("mail: template %q not found", name)
	}

	var htmlBuf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&htmlBuf, r.layoutName, data); err != nil {
		return "", "", fmt.Errorf("mail: render %s: %w", name, err)
	}

	var text string
	if textTmpl, ok := r.text[name]; ok {
		var textBuf bytes.Buffer
		if err := textTmpl.Execute(&textBuf, data); err != nil {
			return "", "", fmt.Errorf("mail: render %s (text): %w", name, err)
		}
		text = textBuf.String()
	}

	return htmlBuf.String(), text, nil
}
