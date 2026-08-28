package auth

import (
	"net/http"
	"testing"
)

func TestDeviceFingerprintSurvivesABrowserUpdate(t *testing.T) {
	// The one legitimate way a User-Agent changes inside a session: the
	// browser updates itself. If that read as a different device, every
	// operator would be signed out on patch Tuesday and the control would be
	// switched off within a week.
	before := DeviceFingerprint("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.6478.127 Safari/537.36")
	after := DeviceFingerprint("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.6533.72 Safari/537.36")
	if before != after {
		t.Fatal("a browser version bump changed the device fingerprint; every user would be signed out when their browser updates")
	}

	// A different program is a different device, which is the case that
	// matters: a token lifted out of a browser and replayed by tooling.
	if DeviceFingerprint("curl/8.5.0") == before {
		t.Fatal("curl and a browser produced the same fingerprint")
	}
	if DeviceFingerprint("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Firefox/128.0") == before {
		t.Fatal("two different browsers produced the same fingerprint")
	}
}

func TestNetworkOf(t *testing.T) {
	for in, want := range map[string]string{
		"203.0.113.7":     "203.0.113.0/24",
		"203.0.113.250":   "203.0.113.0/24",
		"198.51.100.4":    "198.51.100.0/24",
		"2001:db8:1:2::5": "2001:db8:1::/48",
		"not an address":  "",
	} {
		if got := NetworkOf(in); got != want {
			t.Errorf("NetworkOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBindingPolicyEvaluate(t *testing.T) {
	const boundIP = "203.0.113.7"
	bound := Binding{
		IP:          boundIP,
		Network:     NetworkOf(boundIP),
		Fingerprint: DeviceFingerprint("Mozilla/5.0 Chrome/126.0.0.0"),
	}
	sameDevice := DeviceFingerprint("Mozilla/5.0 Chrome/126.0.0.0")
	otherDevice := DeviceFingerprint("curl/8.5.0")

	cases := []struct {
		name   string
		policy BindingPolicy
		seen   Observation
		want   Verdict
		why    string
	}{
		{
			name:   "same address, same device",
			policy: DefaultBindingPolicy(),
			seen:   Observation{IP: boundIP, Fingerprint: sameDevice, Method: http.MethodPost},
			want:   VerdictAllow,
		},
		{
			name:   "a neighbouring address in the same NAT pool",
			policy: DefaultBindingPolicy(),
			seen:   Observation{IP: "203.0.113.201", Fingerprint: sameDevice, Method: http.MethodPost},
			want:   VerdictAllow,
			why:    "carrier NAT and egress clusters move a client inside their own block constantly",
		},
		{
			name:   "a different network, reading",
			policy: DefaultBindingPolicy(),
			seen:   Observation{IP: "198.51.100.4", Fingerprint: sameDevice, Method: http.MethodGet},
			want:   VerdictAllowChanged,
			why:    "a phone moving from wifi to mobile data must not lose the dashboard",
		},
		{
			name:   "a different network, writing",
			policy: DefaultBindingPolicy(),
			seen:   Observation{IP: "198.51.100.4", Fingerprint: sameDevice, Method: http.MethodDelete},
			want:   VerdictReauthenticate,
			why:    "the same phone must not delete a database until the password is proved again",
		},
		{
			name:   "a different device",
			policy: DefaultBindingPolicy(),
			seen:   Observation{IP: boundIP, Fingerprint: otherDevice, Method: http.MethodGet},
			want:   VerdictRefuse,
			why:    "a device change is never a network artefact; it is another program holding the token",
		},
		{
			name:   "strict mode, one address away",
			policy: BindingPolicy{IPMode: IPBindingStrict, DeviceBinding: true},
			seen:   Observation{IP: "203.0.113.8", Fingerprint: sameDevice, Method: http.MethodGet},
			want:   VerdictRefuse,
			why:    "strict is what an operator picks when they know their address does not move",
		},
		{
			name:   "warn mode never refuses",
			policy: BindingPolicy{IPMode: IPBindingWarn, DeviceBinding: true},
			seen:   Observation{IP: "198.51.100.4", Fingerprint: sameDevice, Method: http.MethodDelete},
			want:   VerdictAllowChanged,
		},
		{
			name:   "off mode ignores the address entirely",
			policy: BindingPolicy{IPMode: IPBindingOff, DeviceBinding: true},
			seen:   Observation{IP: "198.51.100.4", Fingerprint: sameDevice, Method: http.MethodDelete},
			want:   VerdictAllow,
		},
		{
			name:   "device binding off still allows a device change",
			policy: BindingPolicy{IPMode: IPBindingNetwork, DeviceBinding: false},
			seen:   Observation{IP: boundIP, Fingerprint: otherDevice, Method: http.MethodPost},
			want:   VerdictAllow,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.policy.Evaluate(bound, tc.seen)
			if got.Verdict != tc.want {
				t.Fatalf("verdict = %v, want %v (%s)", got.Verdict, tc.want, tc.why)
			}
		})
	}
}

func TestBindingPolicyFromEnvFallsBackToTheDefaultOnATypo(t *testing.T) {
	t.Setenv(EnvSessionIPBinding, "netwrok")
	policy := BindingPolicyFromEnv()
	if policy.IPMode != IPBindingNetwork {
		t.Fatalf("a misspelt mode resolved to %q; a typo must never disable the control", policy.IPMode)
	}
	if !policy.DeviceBinding {
		t.Fatal("device binding defaulted to off")
	}

	t.Setenv(EnvSessionIPBinding, "strict")
	t.Setenv(EnvSessionDeviceBinding, "off")
	policy = BindingPolicyFromEnv()
	if policy.IPMode != IPBindingStrict || policy.DeviceBinding {
		t.Fatalf("explicit configuration was not honoured: %+v", policy)
	}
}

func TestAddressInCIDRs(t *testing.T) {
	if !AddressInCIDRs("203.0.113.7", nil) {
		t.Fatal("an empty allow list must mean no restriction")
	}
	if !AddressInCIDRs("203.0.113.7", []string{"203.0.113.0/24"}) {
		t.Fatal("an address inside the allowed block was refused")
	}
	if AddressInCIDRs("198.51.100.4", []string{"203.0.113.0/24"}) {
		t.Fatal("an address outside the allowed block was accepted")
	}
	if !AddressInCIDRs("203.0.113.7", []string{"203.0.113.7"}) {
		t.Fatal("a bare address in the list was not honoured")
	}
	if AddressInCIDRs("203.0.113.7", []string{"not a network"}) {
		t.Fatal("an unparseable entry granted access; a typo in an allow list must grant nothing")
	}
	if AddressInCIDRs("", []string{"203.0.113.0/24"}) {
		t.Fatal("an unknown source address satisfied a restriction")
	}
}

func TestValidateCIDRs(t *testing.T) {
	got, err := ValidateCIDRs([]string{" 203.0.113.0/24 ", "198.51.100.4"})
	if err != nil {
		t.Fatalf("ValidateCIDRs: %v", err)
	}
	if len(got) != 2 || got[0] != "203.0.113.0/24" || got[1] != "198.51.100.4" {
		t.Fatalf("ValidateCIDRs = %v", got)
	}
	if _, err := ValidateCIDRs([]string{"203.0.113.0/24", "nonsense"}); err == nil {
		t.Fatal("a bad entry was accepted; it would have silently locked the key out")
	}
}
