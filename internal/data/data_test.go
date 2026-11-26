package data

import (
	"strings"
	"testing"
)

func TestInferFlatAndNested(t *testing.T) {
	fields, err := Infer([]byte(`{"id":1,"name":"a","ok":true,"tags":["x"],"meta":{"n":2.5},"nil":null}`))
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	got := map[string]string{}
	for _, f := range fields {
		got[f.Name] = f.Type
	}
	want := map[string]string{
		"id": "integer", "name": "string", "ok": "boolean",
		"tags": "array", "tags[]": "string", "meta": "object",
		"meta.n": "number", "nil": "null",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestInferRejectsInvalid(t *testing.T) {
	if _, err := Infer([]byte("nope")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := Infer(nil); err == nil {
		t.Fatal("expected error for empty sample")
	}
}

func TestToCSV(t *testing.T) {
	records := Records([]RecordInput{
		{ExecutionID: "e1", Status: "succeeded", StatusCode: 200, Body: `{"b":2,"a":"x"}`},
		{ExecutionID: "e2", Status: "failed", StatusCode: 500, Body: `not-json`},
	})
	out, err := ToCSV(records)
	if err != nil {
		t.Fatalf("ToCSV: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d: %s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "a,b,execution_id,status,status_code") {
		t.Fatalf("header = %s", lines[0])
	}
	if !strings.Contains(lines[1], "e1") || !strings.Contains(lines[1], "x") {
		t.Fatalf("row = %s", lines[1])
	}
}

func TestRecordsSkipsInvalidJSON(t *testing.T) {
	records := Records([]RecordInput{{ExecutionID: "e1", Body: "oops"}})
	if len(records) != 1 || records[0].Body != nil {
		t.Fatalf("records = %+v", records)
	}
	if records[0].Raw != "oops" {
		t.Fatalf("raw = %q", records[0].Raw)
	}
}
