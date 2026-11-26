// Package template renders request body templates using text/template.
//
// Best practice (see pkg.go.dev/text/template): templates are parsed once and
// executed many times; missing map keys are treated as errors so a typo fails
// loudly instead of rendering "<no value>"; output is buffered so a mid-render
// failure never leaks a partial body.
package template

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

// Render executes body as a text/template against data. An empty body yields
// an empty string. Missing keys and parse/exec errors are returned.
func Render(body string, data map[string]any) (string, error) {
	if strings.TrimSpace(body) == "" {
		return "", nil
	}
	tmpl, err := template.New("body").Option("missingkey=error").Parse(body)
	if err != nil {
		return "", fmt.Errorf("template: parse: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template: execute: %w", err)
	}
	return buf.String(), nil
}

// RenderPath substitutes {name} placeholders in a path template with params.
// Unknown placeholders are left untouched.
func RenderPath(path string, params map[string]string) string {
	for k, v := range params {
		path = strings.ReplaceAll(path, "{"+k+"}", v)
	}
	return path
}
