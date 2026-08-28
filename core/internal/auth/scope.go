package auth

// The API key scope model.
//
// An API key used to be all-or-nothing: whoever held one held everything the
// account behind it could do. That is the wrong shape for the thing API keys
// are actually used for - a monitoring probe that lists websites, a billing
// integration that reads usage, a deployment robot that restarts one service.
// None of those needs to be able to drop a database, and every one of them
// keeps its key in a file on a machine the panel does not control.
//
// A scope is three answers to three questions, written as one string:
//
//	[<tenant>/]<module>:<action>
//
//	tenant  - which tenant's data. Omitted means "the tenant the key belongs
//	          to", which is what almost every key wants and is the safe
//	          default. "*" means any tenant the key's owner can reach, which
//	          only makes sense for a platform-level integration.
//	module  - which part of the panel: website, database, dns, ... The
//	          catalogue below is exactly the RBAC resource list, so a scope and
//	          a permission name the same thing and an operator does not have to
//	          learn two vocabularies. "*" means every module.
//	action  - "read" or "write". "write" implies "read", because an integration
//	          that can create a website can obviously see it. "*" is a synonym
//	          for write.
//
// Examples:
//
//	website:read                  list and inspect websites, change nothing
//	*:read                        a read-only key over the whole panel
//	database:write                full control of databases, nothing else
//	dns:read dns:write            equivalent to dns:write
//	*/monitoring:read             read monitoring for every tenant in reach
//	*:*                           the old all-or-nothing key, said out loud
//
// Two rules make this fail closed:
//
//   - A key with no scopes authorises nothing. Not "everything, because the
//     list is empty" - nothing. Creating a key without scopes is refused.
//   - A scope string that does not parse authorises nothing, and does not
//     invalidate the rest of the key. A typo narrows a key; it never widens it.
//
// The scope set is only ever half the answer. Authority is the intersection of
// the key's scopes and the RBAC permissions of the user the key belongs to: a
// key cannot do what its owner cannot do, however it was written.

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/google/uuid"
)

// Actions. Read is a strict subset of write.
const (
	ActionRead  = "read"
	ActionWrite = "write"
)

// Wildcards.
const (
	// AnyModule in the module position matches every module.
	AnyModule = "*"
	// AnyTenant in the tenant position matches every tenant.
	AnyTenant = "*"
	// anyAction is accepted in the action position and means write (which
	// implies read). It is not a third action.
	anyAction = "*"
)

// MaxScopesPerKey caps how many scopes one key may carry. A key that needs
// more than this is not a scoped key, it is a full key written out long hand;
// the cap keeps the row, the audit entry and the evaluation loop bounded.
const MaxScopesPerKey = 64

// ErrNoScopes is returned when a key would be created with nothing in it.
var ErrNoScopes = errors.New("an API key must declare at least one scope")

// modules is the catalogue. The names are the RBAC resource names used by
// middleware.RequirePermission in the router, deliberately: "website:read" as a
// scope and "website.read" as a permission are the same authority expressed to
// the two systems that have to agree about it.
var modules = []string{
	"audit",
	"backup",
	"cluster",
	"database",
	"dns",
	"docker",
	"firewall",
	"logs",
	"monitoring",
	"nodeapp",
	"notifications",
	"php",
	"reverseproxy",
	"server",
	"settings",
	"ssl",
	"terminal",
	"tenant",
	"user",
	"website",
	"wordpress",
}

var moduleSet = func() map[string]bool {
	set := make(map[string]bool, len(modules))
	for _, m := range modules {
		set[m] = true
	}
	return set
}()

// Modules returns the module catalogue, for the endpoint that tells an
// operator what they may write in a scope.
func Modules() []string {
	out := make([]string, len(modules))
	copy(out, modules)
	return out
}

// KnownModule reports whether a module name is in the catalogue.
func KnownModule(name string) bool {
	return moduleSet[strings.ToLower(strings.TrimSpace(name))]
}

// Scope is one parsed scope string.
type Scope struct {
	// Tenant is "" for "the key's own tenant", AnyTenant for every tenant, or
	// a tenant UUID in canonical string form.
	Tenant string
	// Module is a catalogue name or AnyModule.
	Module string
	// Action is ActionRead or ActionWrite. The "*" written by an operator is
	// canonicalised to ActionWrite here, so nothing downstream has to know
	// about a third value.
	Action string
}

// String returns the canonical text of a scope. Parsing the output of String
// yields the same Scope.
func (s Scope) String() string {
	if s.Tenant == "" {
		return s.Module + ":" + s.Action
	}
	return s.Tenant + "/" + s.Module + ":" + s.Action
}

// ParseScope parses one scope string. It is strict: an unknown module, an
// unknown action or a tenant that is not a UUID is an error rather than
// something quietly ignored, because a scope that is quietly ignored is a
// permission an operator believes they granted.
func ParseScope(raw string) (Scope, error) {
	text := strings.ToLower(strings.TrimSpace(raw))
	if text == "" {
		return Scope{}, errors.New("empty scope")
	}

	var scope Scope

	if slash := strings.Index(text, "/"); slash >= 0 {
		tenant := strings.TrimSpace(text[:slash])
		text = strings.TrimSpace(text[slash+1:])
		switch {
		case tenant == AnyTenant:
			scope.Tenant = AnyTenant
		case tenant == "":
			return Scope{}, fmt.Errorf("scope %q: empty tenant qualifier", raw)
		default:
			id, err := uuid.Parse(tenant)
			if err != nil {
				return Scope{}, fmt.Errorf("scope %q: tenant qualifier is not a UUID", raw)
			}
			scope.Tenant = id.String()
		}
	}

	module, action, found := strings.Cut(text, ":")
	if !found {
		return Scope{}, fmt.Errorf("scope %q: expected <module>:<action>", raw)
	}
	module = strings.TrimSpace(module)
	action = strings.TrimSpace(action)

	if module != AnyModule && !moduleSet[module] {
		return Scope{}, fmt.Errorf("scope %q: unknown module %q", raw, module)
	}
	scope.Module = module

	switch action {
	case ActionRead:
		scope.Action = ActionRead
	case ActionWrite, anyAction:
		scope.Action = ActionWrite
	default:
		return Scope{}, fmt.Errorf("scope %q: action must be %q or %q", raw, ActionRead, ActionWrite)
	}

	return scope, nil
}

// ScopeSet is a key's whole grant.
type ScopeSet []Scope

// ParseScopeSet parses every scope in a list, ignoring nothing.
//
// It is used where the input is operator supplied and a mistake must be
// reported: creating or updating a key. The request-path counterpart is
// ParseScopeSetLenient.
func ParseScopeSet(raw []string) (ScopeSet, error) {
	if len(raw) == 0 {
		return nil, ErrNoScopes
	}
	if len(raw) > MaxScopesPerKey {
		return nil, fmt.Errorf("an API key may carry at most %d scopes", MaxScopesPerKey)
	}

	set := make(ScopeSet, 0, len(raw))
	for _, entry := range raw {
		scope, err := ParseScope(entry)
		if err != nil {
			return nil, err
		}
		set = append(set, scope)
	}
	return set.canonical(), nil
}

// ParseScopeSetLenient parses the scopes stored on a key, dropping the ones
// that no longer parse and reporting them.
//
// This is the request path. A row written by an older build, or by the second
// key-minting path in service/multi_user.go which does no validation at all,
// must not turn into an authentication error that reads like a wrong key - and
// it must certainly not turn into a grant. Anything unparseable is dropped, so
// a bad entry can only ever narrow the key, and the caller logs what it lost.
func ParseScopeSetLenient(raw []string) (ScopeSet, []string) {
	set := make(ScopeSet, 0, len(raw))
	var dropped []string
	for _, entry := range raw {
		scope, err := ParseScope(entry)
		if err != nil {
			dropped = append(dropped, entry)
			continue
		}
		set = append(set, scope)
	}
	return set.canonical(), dropped
}

// canonical sorts and de-duplicates, so two equivalent grants read the same in
// an audit entry and in the API.
func (set ScopeSet) canonical() ScopeSet {
	seen := make(map[string]bool, len(set))
	out := make(ScopeSet, 0, len(set))
	for _, scope := range set {
		text := scope.String()
		if seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, scope)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

// Strings returns the canonical text of every scope, which is what is stored
// in the database and shown in the API.
func (set ScopeSet) Strings() []string {
	out := make([]string, 0, len(set))
	for _, scope := range set {
		out = append(out, scope.String())
	}
	return out
}

// Access is what an endpoint demands: this module, this action, on this
// tenant's data.
type Access struct {
	// Tenant is the tenant whose data the request touches. For every route in
	// this panel that is the authenticated caller's tenant.
	Tenant uuid.UUID
	Module string
	Action string
}

// Allows reports whether the grant covers the demand.
//
// keyTenant is the tenant the key belongs to; it is what an unqualified scope
// resolves to. An empty scope set allows nothing - that is the whole point of
// the type.
func (set ScopeSet) Allows(keyTenant uuid.UUID, want Access) bool {
	if len(set) == 0 {
		return false
	}
	if want.Module == "" || want.Action == "" {
		// A route that forgot to say what it needs is refused rather than
		// allowed. The alternative fails open on a typo.
		return false
	}

	wantModule := strings.ToLower(strings.TrimSpace(want.Module))
	wantAction := strings.ToLower(strings.TrimSpace(want.Action))
	if wantAction != ActionRead && wantAction != ActionWrite {
		return false
	}

	for _, scope := range set {
		if scope.Module != AnyModule && scope.Module != wantModule {
			continue
		}
		// write implies read; read does not imply write.
		if wantAction == ActionWrite && scope.Action != ActionWrite {
			continue
		}
		switch scope.Tenant {
		case AnyTenant:
			return true
		case "":
			if want.Tenant == keyTenant && keyTenant != uuid.Nil {
				return true
			}
		default:
			if want.Tenant != uuid.Nil && scope.Tenant == want.Tenant.String() {
				return true
			}
		}
	}
	return false
}

// ActionForMethod maps an HTTP method onto the action it needs. It is the same
// mapping middleware.RequirePermission uses, so a route that is a read for RBAC
// is a read for scopes.
func ActionForMethod(method string) string {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return ActionRead
	default:
		return ActionWrite
	}
}

// PermissionForScope returns the RBAC permission name that corresponds to a
// module and action, so the owner's permissions and the key's scopes can be
// checked against the same demand.
func PermissionForScope(module, action string) string {
	return strings.ToLower(module) + "." + strings.ToLower(action)
}
