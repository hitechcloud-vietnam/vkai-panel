package acme

import (
	"crypto/x509"
	"encoding/json"
	"testing"
)

func TestNewOrderMarshalling(t *testing.T) {
	tests := []struct {
		name    string
		request newOrderRequest
		want    string
	}{
		{
			name:    "dns identifier without a profile",
			request: newOrderRequest{Identifiers: []Identifier{DNSIdentifier("panel.example.com")}},
			want:    `{"identifiers":[{"type":"dns","value":"panel.example.com"}]}`,
		},
		{
			name: "dns identifier with a profile",
			request: newOrderRequest{
				Identifiers: []Identifier{DNSIdentifier("panel.example.com")},
				Profile:     ProfileClassic,
			},
			want: `{"identifiers":[{"type":"dns","value":"panel.example.com"}],"profile":"classic"}`,
		},
		{
			name:    "ip identifier without a profile",
			request: newOrderRequest{Identifiers: []Identifier{IPIdentifier("203.0.113.10")}},
			want:    `{"identifiers":[{"type":"ip","value":"203.0.113.10"}]}`,
		},
		{
			name: "ip identifier with the shortlived profile",
			request: newOrderRequest{
				Identifiers: []Identifier{IPIdentifier("203.0.113.10")},
				Profile:     ProfileShortLived,
			},
			want: `{"identifiers":[{"type":"ip","value":"203.0.113.10"}],"profile":"shortlived"}`,
		},
		{
			name: "mixed identifiers keep their order",
			request: newOrderRequest{
				Identifiers: []Identifier{
					DNSIdentifier("panel.example.com"),
					IPIdentifier("2001:db8::1"),
				},
				Profile: ProfileTLSServer,
			},
			want: `{"identifiers":[{"type":"dns","value":"panel.example.com"},{"type":"ip","value":"2001:db8::1"}],"profile":"tlsserver"}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.request)
			if err != nil {
				t.Fatalf("marshal newOrder request: %v", err)
			}
			if string(got) != tc.want {
				t.Fatalf("newOrder JSON mismatch\n got: %s\nwant: %s", got, tc.want)
			}
		})
	}
}

func TestValidateIdentifiers(t *testing.T) {
	tests := []struct {
		name    string
		ids     []Identifier
		wantErr bool
	}{
		{"dns", []Identifier{DNSIdentifier("panel.example.com")}, false},
		{"ipv4", []Identifier{IPIdentifier("203.0.113.10")}, false},
		{"ipv6", []Identifier{IPIdentifier("2001:db8::1")}, false},
		{"empty dns value", []Identifier{DNSIdentifier("  ")}, true},
		{"ip value is a host name", []Identifier{IPIdentifier("panel.example.com")}, true},
		{"unknown type", []Identifier{{Type: "email", Value: "ops@example.com"}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateIdentifiers(tc.ids)
			if tc.wantErr != (err != nil) {
				t.Fatalf("validateIdentifiers(%v) error = %v, wantErr %v", tc.ids, err, tc.wantErr)
			}
		})
	}
}

func TestProfilesUnmarshal(t *testing.T) {
	t.Run("object form as Let's Encrypt sends it", func(t *testing.T) {
		var meta DirectoryMeta
		body := `{"termsOfService":"https://ca.example/tos","profiles":{"classic":"The old ways","shortlived":"About six days","tlsserver":"TLS server only"}}`
		if err := json.Unmarshal([]byte(body), &meta); err != nil {
			t.Fatalf("unmarshal directory meta: %v", err)
		}
		want := []string{ProfileClassic, ProfileShortLived, ProfileTLSServer}
		got := meta.Profiles.Names()
		if len(got) != len(want) {
			t.Fatalf("profiles = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("profiles = %v, want %v", got, want)
			}
		}
		if !meta.Profiles.Has(ProfileShortLived) {
			t.Fatal("shortlived profile must be reported as available")
		}
		if meta.Profiles.Has("nonexistent") {
			t.Fatal("an unadvertised profile must not be reported as available")
		}
		if desc := meta.Profiles[ProfileShortLived]; desc != "About six days" {
			t.Fatalf("profile description = %q", desc)
		}
	})

	t.Run("array form", func(t *testing.T) {
		var meta DirectoryMeta
		if err := json.Unmarshal([]byte(`{"profiles":["classic","shortlived"]}`), &meta); err != nil {
			t.Fatalf("unmarshal directory meta: %v", err)
		}
		if !meta.Profiles.Has(ProfileClassic) || !meta.Profiles.Has(ProfileShortLived) {
			t.Fatalf("profiles = %v", meta.Profiles.Names())
		}
	})

	t.Run("absent", func(t *testing.T) {
		var meta DirectoryMeta
		if err := json.Unmarshal([]byte(`{"website":"https://ca.example"}`), &meta); err != nil {
			t.Fatalf("unmarshal directory meta: %v", err)
		}
		if len(meta.Profiles) != 0 {
			t.Fatalf("expected no profiles, got %v", meta.Profiles.Names())
		}
	})
}

func TestBuildCSRCarriesBothIdentifierTypes(t *testing.T) {
	key, err := generateAccountKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	der, err := buildCSR(key, []Identifier{
		DNSIdentifier("panel.example.com"),
		IPIdentifier("203.0.113.10"),
	})
	if err != nil {
		t.Fatalf("buildCSR: %v", err)
	}
	csr, err := x509.ParseCertificateRequest(der)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature: %v", err)
	}
	if len(csr.DNSNames) != 1 || csr.DNSNames[0] != "panel.example.com" {
		t.Fatalf("CSR DNS names = %v", csr.DNSNames)
	}
	if len(csr.IPAddresses) != 1 || csr.IPAddresses[0].String() != "203.0.113.10" {
		t.Fatalf("CSR IP addresses = %v", csr.IPAddresses)
	}
}
