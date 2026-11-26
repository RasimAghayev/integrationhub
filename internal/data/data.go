// Package data turns stored execution responses into queryable schemas and
// exportable datasets.
package data

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Field describes one inferred field of a JSON document.
type Field struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// Infer derives a flat field list from a JSON object sample. Nested objects are
// flattened with dot notation; arrays are typed by their first element.
func Infer(sample []byte) ([]Field, error) {
	if len(sample) == 0 {
		return nil, fmt.Errorf("data: empty sample")
	}
	var doc any
	if err := json.Unmarshal(sample, &doc); err != nil {
		return nil, fmt.Errorf("data: sample is not valid JSON: %w", err)
	}
	fields := map[string]*Field{}
	collect("", doc, fields, false)
	out := make([]Field, 0, len(fields))
	for _, f := range fields {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func collect(prefix string, val any, fields map[string]*Field, nullable bool) {
	name := prefix
	switch v := val.(type) {
	case map[string]any:
		if name != "" {
			fields[name] = &Field{Name: name, Type: "object", Nullable: nullable}
		}
		for k, child := range v {
			childName := k
			if name != "" {
				childName = name + "." + k
			}
			collect(childName, child, fields, nullable)
		}
	case []any:
		if name != "" {
			fields[name] = &Field{Name: name, Type: "array", Nullable: nullable}
		}
		for _, child := range v {
			collect(name+"[]", child, fields, nullable)
		}
	case string:
		fields[name] = &Field{Name: name, Type: "string", Nullable: nullable}
	case bool:
		fields[name] = &Field{Name: name, Type: "boolean", Nullable: nullable}
	case float64:
		t := "number"
		if v == float64(int64(v)) {
			t = "integer"
		}
		fields[name] = &Field{Name: name, Type: t, Nullable: nullable}
	case nil:
		fields[name] = &Field{Name: name, Type: "null", Nullable: true}
	default:
		fields[name] = &Field{Name: name, Type: "unknown", Nullable: nullable}
	}
}

// Record is one exported execution response.
type Record struct {
	ExecutionID string
	Status      string
	StatusCode  int
	Body        map[string]any
	Raw         string
}

// Records builds export records from raw response bodies, skipping invalid JSON.
func Records(items []RecordInput) []Record {
	out := make([]Record, 0, len(items))
	for _, in := range items {
		rec := Record{ExecutionID: in.ExecutionID, Status: in.Status, StatusCode: in.StatusCode, Raw: in.Body}
		var body map[string]any
		if err := json.Unmarshal([]byte(in.Body), &body); err == nil {
			rec.Body = body
		}
		out = append(out, rec)
	}
	return out
}

// RecordInput is the minimal execution data export needs.
type RecordInput struct {
	ExecutionID string
	Status      string
	StatusCode  int
	Body        string
}

// ToCSV flattens records into a CSV document with a stable column order.
func ToCSV(records []Record) ([]byte, error) {
	columns := map[string]bool{"execution_id": true, "status": true, "status_code": true}
	for _, r := range records {
		for k := range r.Body {
			columns[k] = true
		}
	}
	cols := make([]string, 0, len(columns))
	for c := range columns {
		cols = append(cols, c)
	}
	sort.Strings(cols)

	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := w.Write(cols); err != nil {
		return nil, err
	}
	for _, r := range records {
		row := make([]string, 0, len(cols))
		for _, c := range cols {
			switch c {
			case "execution_id":
				row = append(row, r.ExecutionID)
			case "status":
				row = append(row, r.Status)
			case "status_code":
				row = append(row, strconv.Itoa(r.StatusCode))
			default:
				row = append(row, stringify(r.Body[c]))
			}
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, fmt.Errorf("data: write csv: %w", err)
	}
	return []byte(sb.String()), nil
}

func stringify(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		encoded, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(encoded)
	}
}
