package template

import (
	"strings"
	"testing"
)

func TestRender(t *testing.T) {
	out, err := Render(`{"name":"{{.name}}","n":{{.n}}}`, map[string]any{"name": "go", "n": 3})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if out != `{"name":"go","n":3}` {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRenderEmptyBody(t *testing.T) {
	out, err := Render("   ", nil)
	if err != nil || out != "" {
		t.Fatalf("empty body: out=%q err=%v", out, err)
	}
}

func TestRenderMissingKeyIsError(t *testing.T) {
	if _, err := Render(`{{.missing}}`, map[string]any{}); err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestRenderParseError(t *testing.T) {
	if _, err := Render(`{{.name`, map[string]any{"name": "x"}); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestRenderPath(t *testing.T) {
	got := RenderPath("/users/{id}/posts/{postID}", map[string]string{"id": "42", "postID": "7"})
	if got != "/users/42/posts/7" {
		t.Fatalf("unexpected path: %s", got)
	}
	if strings.Contains(got, "{") {
		t.Fatalf("placeholder left: %s", got)
	}
}
