package rbac

import "testing"

func TestDefaultPolicy(t *testing.T) {
	e, err := New(DefaultPolicy)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cases := []struct {
		roles    []string
		resource string
		action   string
		want     bool
	}{
		{[]string{"admin"}, "connections", "write", true},
		{[]string{"admin"}, "anything", "delete", true},
		{[]string{"operator"}, "integrations", "write", true},
		{[]string{"operator"}, "connections", "write", false},
		{[]string{"viewer"}, "executions", "read", true},
		{[]string{"viewer"}, "executions", "write", false},
		{[]string{"unknown"}, "integrations", "read", false},
		{nil, "integrations", "read", false},
	}
	for _, c := range cases {
		if got := e.Allow(c.roles, c.resource, c.action); got != c.want {
			t.Errorf("Allow(%v,%s,%s)=%v want %v", c.roles, c.resource, c.action, got, c.want)
		}
	}
}

func TestCustomPolicy(t *testing.T) {
	e, err := New("g, svc, svc\np, svc, reports, read\n")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !e.Allow([]string{"svc"}, "reports", "read") {
		t.Fatal("expected reports read to be allowed")
	}
	if e.Allow([]string{"svc"}, "reports", "write") {
		t.Fatal("expected reports write to be denied")
	}
}

func TestMalformedPolicy(t *testing.T) {
	if _, err := New("p, only-two\n"); err == nil {
		t.Fatal("expected error for malformed permission")
	}
	if _, err := New("x, a, b\n"); err == nil {
		t.Fatal("expected error for unknown policy type")
	}
}

func TestNilEnforcerDenies(t *testing.T) {
	var e *Enforcer
	if e.Allow([]string{"admin"}, "connections", "write") {
		t.Fatal("nil enforcer must deny")
	}
}
