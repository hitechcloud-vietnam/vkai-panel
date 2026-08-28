package auth

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseScope(t *testing.T) {
	tenant := uuid.New()

	cases := []struct {
		in      string
		want    string
		wantErr bool
		why     string
	}{
		{in: "website:read", want: "website:read"},
		{in: "  WEBSITE : READ ", want: "website:read", why: "case and spacing are an operator's business, not a grammar error"},
		{in: "database:*", want: "database:write", why: "* in the action position is write, which implies read"},
		{in: "*:read", want: "*:read"},
		{in: "*:*", want: "*:write", why: "the old all-or-nothing key, said out loud"},
		{in: "*/monitoring:read", want: "*/monitoring:read"},
		{in: tenant.String() + "/website:read", want: tenant.String() + "/website:read"},

		{in: "", wantErr: true},
		{in: "website", wantErr: true, why: "no action"},
		{in: "website:delete", wantErr: true, why: "there are two actions and delete is not one"},
		{in: "websites:read", wantErr: true, why: "not a module name; a typo must be refused, not ignored"},
		{in: "not-a-uuid/website:read", wantErr: true},
		{in: "/website:read", wantErr: true},
	}

	for _, tc := range cases {
		scope, err := ParseScope(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseScope(%q) was accepted and should not have been (%s)", tc.in, tc.why)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseScope(%q): %v", tc.in, err)
			continue
		}
		if scope.String() != tc.want {
			t.Errorf("ParseScope(%q) = %q, want %q", tc.in, scope.String(), tc.want)
		}
	}
}

func TestParseScopeSetRefusesAnEmptyGrant(t *testing.T) {
	if _, err := ParseScopeSet(nil); err == nil {
		t.Fatal("a key with no scopes was accepted; a key that authorises nothing must not be creatable")
	}
	if _, err := ParseScopeSet([]string{}); err == nil {
		t.Fatal("an empty scope list was accepted")
	}
}

func TestParseScopeSetLenientNarrowsButNeverWidens(t *testing.T) {
	set, dropped := ParseScopeSetLenient([]string{"website:read", "nonsense", "database:write"})

	if len(dropped) != 1 || dropped[0] != "nonsense" {
		t.Fatalf("dropped = %v, want exactly the unparseable entry", dropped)
	}
	got := set.Strings()
	want := []string{"database:write", "website:read"}
	if len(got) != len(want) {
		t.Fatalf("scopes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scopes = %v, want %v", got, want)
		}
	}
}

func TestScopeSetAllows(t *testing.T) {
	keyTenant := uuid.New()
	otherTenant := uuid.New()

	mustParse := func(t *testing.T, entries ...string) ScopeSet {
		t.Helper()
		set, err := ParseScopeSet(entries)
		if err != nil {
			t.Fatalf("ParseScopeSet(%v): %v", entries, err)
		}
		return set
	}

	cases := []struct {
		name   string
		scopes ScopeSet
		access Access
		want   bool
		why    string
	}{
		{
			name:   "the endpoint it was scoped for",
			scopes: mustParse(t, "website:read"),
			access: Access{Tenant: keyTenant, Module: "website", Action: ActionRead},
			want:   true,
		},
		{
			name:   "a different module",
			scopes: mustParse(t, "website:read"),
			access: Access{Tenant: keyTenant, Module: "database", Action: ActionRead},
			want:   false,
			why:    "an integration that lists websites must not reach databases",
		},
		{
			name:   "a write with a read-only scope",
			scopes: mustParse(t, "website:read"),
			access: Access{Tenant: keyTenant, Module: "website", Action: ActionWrite},
			want:   false,
			why:    "this is the whole point of read-only",
		},
		{
			name:   "a read with a write scope",
			scopes: mustParse(t, "website:write"),
			access: Access{Tenant: keyTenant, Module: "website", Action: ActionRead},
			want:   true,
			why:    "write implies read",
		},
		{
			name:   "a read-only key over everything",
			scopes: mustParse(t, "*:read"),
			access: Access{Tenant: keyTenant, Module: "database", Action: ActionRead},
			want:   true,
		},
		{
			name:   "a read-only key asked to write",
			scopes: mustParse(t, "*:read"),
			access: Access{Tenant: keyTenant, Module: "database", Action: ActionWrite},
			want:   false,
		},
		{
			name:   "another tenant's data with an unqualified scope",
			scopes: mustParse(t, "website:read"),
			access: Access{Tenant: otherTenant, Module: "website", Action: ActionRead},
			want:   false,
			why:    "an unqualified scope means the key's own tenant and nothing else",
		},
		{
			name:   "another tenant's data with a tenant wildcard",
			scopes: mustParse(t, "*/website:read"),
			access: Access{Tenant: otherTenant, Module: "website", Action: ActionRead},
			want:   true,
		},
		{
			name:   "a scope pinned to one tenant, used on another",
			scopes: mustParse(t, otherTenant.String()+"/website:read"),
			access: Access{Tenant: keyTenant, Module: "website", Action: ActionRead},
			want:   false,
		},
		{
			name:   "an empty grant",
			scopes: ScopeSet{},
			access: Access{Tenant: keyTenant, Module: "website", Action: ActionRead},
			want:   false,
			why:    "no scopes must mean nothing, never everything",
		},
		{
			name:   "an endpoint that declares nothing",
			scopes: mustParse(t, "*:write"),
			access: Access{Tenant: keyTenant},
			want:   false,
			why:    "a route that forgot to say what it needs is refused, not allowed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.scopes.Allows(keyTenant, tc.access); got != tc.want {
				t.Fatalf("Allows() = %v, want %v (%s)", got, tc.want, tc.why)
			}
		})
	}
}

func TestActionForMethod(t *testing.T) {
	for method, want := range map[string]string{
		"GET":     ActionRead,
		"HEAD":    ActionRead,
		"OPTIONS": ActionRead,
		"POST":    ActionWrite,
		"PUT":     ActionWrite,
		"PATCH":   ActionWrite,
		"DELETE":  ActionWrite,
	} {
		if got := ActionForMethod(method); got != want {
			t.Errorf("ActionForMethod(%q) = %q, want %q", method, got, want)
		}
	}
}
