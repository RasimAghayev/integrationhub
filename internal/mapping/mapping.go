// Package mapping normalizes upstream response payloads into the platform's
// canonical shape using declarative field mappings and transforms.
package mapping

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"text/template"

	"github.com/RasimAghayev/integrationhub/internal/domain"
	"github.com/RasimAghayev/integrationhub/internal/transform"
)

// ErrMissingField is returned when a required source field is absent.
var ErrMissingField = fmt.Errorf("mapping: required field missing")

// Apply projects payload onto a new object using mappings, then applies the
// optional response transform. The input payload is never mutated.
func Apply(payload map[string]any, mappings []domain.FieldMapping, cfg domain.TransformConfig) (map[string]any, error) {
	out := make(map[string]any, len(mappings))
	for _, m := range mappings {
		val, ok := lookup(payload, m.Source)
		if !ok || val == nil {
			if strings.HasPrefix(m.Transform, "default:") {
				val = strings.TrimPrefix(m.Transform, "default:")
				ok = true
			}
		}
		if !ok || val == nil {
			if m.Required {
				return nil, fmt.Errorf("%w: %s", ErrMissingField, m.Source)
			}
			continue
		}
		transformed, err := applyTransform(m.Transform, val, payload)
		if err != nil {
			return nil, err
		}
		if err := assign(out, m.Target, transformed); err != nil {
			return nil, err
		}
	}
	return applyConfig(out, cfg)
}

func applyConfig(out map[string]any, cfg domain.TransformConfig) (map[string]any, error) {
	switch cfg.Type {
	case "", domain.TransformNone:
		return out, nil
	case domain.TransformTemplate:
		if strings.TrimSpace(cfg.Template) == "" {
			return nil, fmt.Errorf("mapping: transform template is required")
		}
		tpl, err := template.New("transform").Parse(cfg.Template)
		if err != nil {
			return nil, fmt.Errorf("mapping: parse transform template: %w", err)
		}
		var sb strings.Builder
		if err := tpl.Execute(&sb, out); err != nil {
			return nil, fmt.Errorf("mapping: execute transform template: %w", err)
		}
		var res map[string]any
		if err := json.Unmarshal([]byte(sb.String()), &res); err != nil {
			return nil, fmt.Errorf("mapping: transform template must render a JSON object: %w", err)
		}
		return res, nil
	case domain.TransformLua:
		return transform.Apply(cfg.Script, out)
	default:
		return nil, fmt.Errorf("mapping: unsupported transform type %q", cfg.Type)
	}
}

func applyTransform(name string, val any, payload map[string]any) (any, error) {
	switch {
	case name == "" || name == "none":
		return val, nil
	case name == "upper":
		return strings.ToUpper(fmt.Sprint(val)), nil
	case name == "lower":
		return strings.ToLower(fmt.Sprint(val)), nil
	case name == "trim":
		return strings.TrimSpace(fmt.Sprint(val)), nil
	case name == "to_string":
		return fmt.Sprint(val), nil
	case name == "to_int":
		switch v := val.(type) {
		case float64:
			return int64(v), nil
		case int:
			return int64(v), nil
		case int64:
			return v, nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("mapping: to_int: %w", err)
			}
			return n, nil
		default:
			return nil, fmt.Errorf("mapping: to_int: unsupported type %T", val)
		}
	case name == "to_bool":
		switch v := val.(type) {
		case bool:
			return v, nil
		case string:
			b, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return nil, fmt.Errorf("mapping: to_bool: %w", err)
			}
			return b, nil
		default:
			return nil, fmt.Errorf("mapping: to_bool: unsupported type %T", val)
		}
	case strings.HasPrefix(name, "default:"):
		if val == nil || fmt.Sprint(val) == "" {
			return strings.TrimPrefix(name, "default:"), nil
		}
		return val, nil
	case strings.HasPrefix(name, "template:"):
		tpl, err := template.New("field").Parse(strings.TrimPrefix(name, "template:"))
		if err != nil {
			return nil, fmt.Errorf("mapping: parse field template: %w", err)
		}
		var sb strings.Builder
		if err := tpl.Execute(&sb, map[string]any{"value": val, "payload": payload}); err != nil {
			return nil, fmt.Errorf("mapping: execute field template: %w", err)
		}
		return sb.String(), nil
	default:
		return nil, fmt.Errorf("mapping: unsupported transform %q", name)
	}
}

// lookup resolves a dotted path such as "data.items.0.id" or "data.ids.1".
func lookup(obj map[string]any, path string) (any, bool) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return nil, false
	}
	var cur any = obj
	for _, p := range parts {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[p]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(p)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// assign writes value at a dotted target path, creating intermediate objects.
func assign(obj map[string]any, path string, value any) error {
	parts := splitPath(path)
	if len(parts) == 0 {
		return fmt.Errorf("mapping: empty target path")
	}
	cur := obj
	for _, p := range parts[:len(parts)-1] {
		next, ok := cur[p]
		if !ok {
			child := make(map[string]any)
			cur[p] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			return fmt.Errorf("mapping: target %q conflicts with a scalar value", path)
		}
		cur = child
	}
	cur[parts[len(parts)-1]] = value
	return nil
}

func splitPath(path string) []string {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	path = strings.ReplaceAll(path, "[", ".")
	path = strings.ReplaceAll(path, "]", "")
	var out []string
	for _, p := range strings.Split(path, ".") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ApplyResponse maps a raw JSON response body and returns the normalized JSON.
func ApplyResponse(body []byte, mappings []domain.FieldMapping, cfg domain.TransformConfig) (string, error) {
	if len(body) == 0 {
		return "", fmt.Errorf("mapping: response body is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("mapping: response is not a JSON object: %w", err)
	}
	out, err := Apply(payload, mappings, cfg)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("mapping: encode output: %w", err)
	}
	return string(encoded), nil
}

// ValidateMappings checks a mapping set before it is persisted.
func ValidateMappings(mappings []domain.FieldMapping) error {
	seen := make(map[string]bool, len(mappings))
	for idx, m := range mappings {
		if strings.TrimSpace(m.Source) == "" || strings.TrimSpace(m.Target) == "" {
			return fmt.Errorf("mappings[%d]: source and target are required", idx)
		}
		if seen[m.Target] {
			return fmt.Errorf("mappings[%d]: duplicate target %q", idx, m.Target)
		}
		seen[m.Target] = true
		if !domain.ValidateTransform(m.Transform) {
			return fmt.Errorf("mappings[%d]: unsupported transform %q", idx, m.Transform)
		}
	}
	return nil
}
