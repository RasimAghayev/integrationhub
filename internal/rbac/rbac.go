// Package rbac enforces role-based access control with Casbin. Callers are
// mapped to roles (via JWT claims or API key records) and roles are granted
// resource/action permissions.
package rbac

import (
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

// ModelText is the Casbin RBAC model. The matcher supports the "*" wildcard for
// both resource and action.
const ModelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && (p.obj == "*" || r.obj == p.obj) && (p.act == "*" || r.act == p.act)
`

// DefaultPolicy grants sensible permissions to the built-in roles.
const DefaultPolicy = `
g, admin, admin
p, admin, *, *
g, operator, operator
p, operator, connections, read
p, operator, integrations, read
p, operator, integrations, write
p, operator, executions, read
p, operator, executions, write
g, viewer, viewer
p, viewer, connections, read
p, viewer, integrations, read
p, viewer, executions, read
`

// Enforcer answers permission questions.
type Enforcer struct {
	e *casbin.Enforcer
}

// New builds an Enforcer from a policy CSV. Lines starting with "p," define
// permissions and "g," defines role membership.
func New(policyCSV string) (*Enforcer, error) {
	m, err := model.NewModelFromString(ModelText)
	if err != nil {
		return nil, fmt.Errorf("rbac: model: %w", err)
	}
	e, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("rbac: enforcer: %w", err)
	}
	for _, line := range strings.Split(policyCSV, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) < 2 {
			return nil, fmt.Errorf("rbac: malformed policy line %q", line)
		}
		var ok bool
		switch parts[0] {
		case "p":
			if len(parts) != 4 {
				return nil, fmt.Errorf("rbac: malformed permission %q", line)
			}
			ok, err = e.AddPolicy(parts[1], parts[2], parts[3])
		case "g":
			if len(parts) != 3 {
				return nil, fmt.Errorf("rbac: malformed grouping %q", line)
			}
			ok, err = e.AddGroupingPolicy(parts[1], parts[2])
		default:
			return nil, fmt.Errorf("rbac: unknown policy type %q", parts[0])
		}
		if err != nil {
			return nil, fmt.Errorf("rbac: add policy %q: %w", line, err)
		}
		_ = ok
	}
	return &Enforcer{e: e}, nil
}

// Allow reports whether any of the roles may perform action on resource.
func (e *Enforcer) Allow(roles []string, resource, action string) bool {
	if e == nil || e.e == nil {
		return false
	}
	for _, role := range roles {
		if role == "" {
			continue
		}
		if ok, err := e.e.Enforce(role, resource, action); err == nil && ok {
			return true
		}
	}
	return false
}
