package transform

import "testing"

func TestApplyLua(t *testing.T) {
	out, err := Apply(`
out = {}
out.total = payload.count * 2
out.label = string.upper(payload.name)
out.items = {}
for i, item in ipairs(payload.items) do
  out.items[i] = item * 3
end
`, map[string]any{
		"count": float64(4),
		"name":  "acme",
		"items": []any{float64(1), float64(2)},
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if out["total"] != float64(8) {
		t.Errorf("total = %#v", out["total"])
	}
	if out["label"] != "ACME" {
		t.Errorf("label = %#v", out["label"])
	}
	items, ok := out["items"].([]any)
	if !ok || len(items) != 2 || items[1] != float64(6) {
		t.Errorf("items = %#v", out["items"])
	}
}

func TestApplyLuaErrors(t *testing.T) {
	if _, err := Apply("", nil); err == nil {
		t.Fatal("expected error for empty script")
	}
	if _, err := Apply("error('boom')", nil); err == nil {
		t.Fatal("expected runtime error")
	}
	if _, err := Apply("x = 1", nil); err == nil {
		t.Fatal("expected error when out is missing")
	}
	if _, err := Apply("out = 5", nil); err == nil {
		t.Fatal("expected error when out is not a table")
	}
	if _, err := Apply("out = { [1] = 'a' }", nil); err == nil {
		t.Fatal("expected error for non-string keys")
	}
}

func TestApplyLuaNilAndNested(t *testing.T) {
	out, err := Apply(`
out = { nested = { a = 1 }, nothing = nil, flag = true }
`, map[string]any{})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	nested, ok := out["nested"].(map[string]any)
	if !ok || nested["a"] != float64(1) {
		t.Fatalf("nested = %#v", out["nested"])
	}
	if out["flag"] != true {
		t.Fatalf("flag = %#v", out["flag"])
	}
}
