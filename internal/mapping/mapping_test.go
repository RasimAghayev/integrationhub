package mapping

import (
	"errors"
	"testing"

	"github.com/RasimAghayev/integrationhub/internal/domain"
)

func sample() map[string]any {
	return map[string]any{
		"data": map[string]any{
			"id":    "42",
			"name":  "  Acme  ",
			"items": []any{map[string]any{"sku": "a1"}, map[string]any{"sku": "b2"}},
		},
		"active": "true",
	}
}

func TestApplyBasic(t *testing.T) {
	mappings := []domain.FieldMapping{
		{Source: "data.id", Target: "id", Transform: "to_int"},
		{Source: "data.name", Target: "name", Transform: "trim"},
		{Source: "data.items.1.sku", Target: "second_sku", Transform: "upper"},
		{Source: "active", Target: "is_active", Transform: "to_bool"},
		{Source: "missing", Target: "nope"},
	}
	out, err := Apply(sample(), mappings, domain.TransformConfig{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["id"] != int64(42) {
		t.Errorf("id = %#v", out["id"])
	}
	if out["name"] != "Acme" {
		t.Errorf("name = %#v", out["name"])
	}
	if out["second_sku"] != "B2" {
		t.Errorf("second_sku = %#v", out["second_sku"])
	}
	if out["is_active"] != true {
		t.Errorf("is_active = %#v", out["is_active"])
	}
	if _, ok := out["nope"]; ok {
		t.Error("optional missing field must be skipped")
	}
}

func TestApplyRequiredMissing(t *testing.T) {
	_, err := Apply(sample(), []domain.FieldMapping{{Source: "data.missing", Target: "x", Required: true}}, domain.TransformConfig{})
	if !errors.Is(err, ErrMissingField) {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyNestedTarget(t *testing.T) {
	out, err := Apply(sample(), []domain.FieldMapping{{Source: "data.id", Target: "result.meta.id"}}, domain.TransformConfig{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	meta, ok := out["result"].(map[string]any)
	if !ok || meta["meta"].(map[string]any)["id"] != "42" {
		t.Fatalf("nested target not built: %#v", out)
	}
}

func TestApplyTemplateTransform(t *testing.T) {
	out, err := Apply(sample(), []domain.FieldMapping{{Source: "data.items", Target: "items"}}, domain.TransformConfig{
		Type:     domain.TransformTemplate,
		Template: `{"count": {{ len .items }}}`,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["count"] != float64(2) {
		t.Fatalf("count = %#v", out["count"])
	}
}

func TestApplyDefaultAndTemplateField(t *testing.T) {
	out, err := Apply(sample(), []domain.FieldMapping{
		{Source: "missing", Target: "region", Transform: "default:eu"},
		{Source: "data.name", Target: "label", Transform: "template:{{ .value | printf \"%q\" }}"},
	}, domain.TransformConfig{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["region"] != "eu" {
		t.Errorf("region = %#v", out["region"])
	}
	if out["label"] != `"  Acme  "` {
		t.Errorf("label = %#v", out["label"])
	}
}

func TestApplyBadInt(t *testing.T) {
	_, err := Apply(map[string]any{"a": "x"}, []domain.FieldMapping{{Source: "a", Target: "b", Transform: "to_int"}}, domain.TransformConfig{})
	if err == nil {
		t.Fatal("expected to_int error")
	}
}

func TestApplyResponse(t *testing.T) {
	out, err := ApplyResponse([]byte(`{"data":{"id":"9"}}`),
		[]domain.FieldMapping{{Source: "data.id", Target: "id", Transform: "to_int"}},
		domain.TransformConfig{})
	if err != nil {
		t.Fatalf("ApplyResponse: %v", err)
	}
	if out != `{"id":9}` {
		t.Fatalf("out = %s", out)
	}
	if _, err := ApplyResponse([]byte(`not json`), nil, domain.TransformConfig{}); err == nil {
		t.Fatal("expected error for non-object body")
	}
	if _, err := ApplyResponse(nil, nil, domain.TransformConfig{}); err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestApplyLuaTransform(t *testing.T) {
	out, err := Apply(sample(), []domain.FieldMapping{{Source: "data.id", Target: "id"}}, domain.TransformConfig{
		Type:   domain.TransformLua,
		Script: `out = { doubled = payload.id * 2 }`,
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["doubled"] != float64(84) {
		t.Fatalf("doubled = %#v", out["doubled"])
	}
}

func TestValidateMappings(t *testing.T) {
	if err := ValidateMappings([]domain.FieldMapping{{Source: "a", Target: "b"}}); err != nil {
		t.Fatalf("valid mappings rejected: %v", err)
	}
	if err := ValidateMappings([]domain.FieldMapping{{Source: "a", Target: "b"}, {Source: "c", Target: "b"}}); err == nil {
		t.Fatal("expected duplicate target error")
	}
	if err := ValidateMappings([]domain.FieldMapping{{Source: "a", Target: "b", Transform: "nope"}}); err == nil {
		t.Fatal("expected unsupported transform error")
	}
}
