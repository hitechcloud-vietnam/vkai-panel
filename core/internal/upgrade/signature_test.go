package upgrade

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"strings"
	"testing"
)

// signManifest returns m with a signature made by key, the way the release
// tooling is expected to produce one.
func signManifest(t *testing.T, key ed25519.PrivateKey, m Manifest) Manifest {
	t.Helper()
	m.Signature = hex.EncodeToString(ed25519.Sign(key, ManifestSigningPayload(m)))
	return m
}

func releaseKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return pub, priv
}

// With release keys configured, the feed host stops being able to choose what
// this machine installs as root: it can serve anything it likes, and anything
// not signed by the release key is refused before a byte is downloaded.
func TestSignedManifestsAreRequiredWhenKeysAreConfigured(t *testing.T) {
	t.Parallel()
	pub, priv := releaseKeypair(t)
	_, otherPriv := releaseKeypair(t)
	hexKey := hex.EncodeToString(pub)

	withKeys := func(c *Config) { c.ReleasePublicKeys = []string{hexKey} }

	t.Run("a signed release is installed", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", withKeys)
		m := env.publish("1.1.0", "")
		env.publishManifests(signManifest(t, priv, m))

		res, err := env.u.Check(context.Background())
		if err != nil {
			t.Fatalf("Check: %v", err)
		}
		if !res.SignaturesChecked {
			t.Error("CheckResult does not report that signatures were verified")
		}
		if res.Target.Version != "1.1.0" {
			t.Errorf("target = %+v", res.Target)
		}
	})

	t.Run("an unsigned release is refused", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", withKeys)
		env.publish("1.1.0", "")

		if _, err := env.u.Check(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "not signed") {
			t.Fatalf("Check error = %v, want the unsigned release to be refused", err)
		}
	})

	t.Run("a signature by the wrong key is refused", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", withKeys)
		m := env.publish("1.1.0", "")
		env.publishManifests(signManifest(t, otherPriv, m))

		if _, err := env.u.Check(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "not made by any configured release key") {
			t.Fatalf("Check error = %v, want the wrong key to be refused", err)
		}
	})

	t.Run("a tampered field invalidates the signature", func(t *testing.T) {
		t.Parallel()
		// The interesting tamper is the one that matters: same version,
		// same URL, a different tarball digest.
		for _, tamper := range []func(*Manifest){
			func(m *Manifest) { m.SHA256 = sha256Hex([]byte("a different release")) },
			func(m *Manifest) { m.TarballURL = testTarBase + "elsewhere.tar.gz" },
			func(m *Manifest) { m.MinUpgradeFrom = "" },
			func(m *Manifest) { m.Version = "1.2.0" },
			func(m *Manifest) { m.SizeBytes += 1 },
		} {
			env := newTestEnv(t, "1.0.0", withKeys)
			m := env.publish("1.1.0", "1.0.0")
			signed := signManifest(t, priv, m)
			tamper(&signed)
			env.publishManifests(signed)

			if _, err := env.u.Check(context.Background()); err == nil {
				t.Errorf("Check accepted a manifest whose signature no longer covers it")
			}
		}
	})

	t.Run("every listed release has to be signed, not only the target", func(t *testing.T) {
		t.Parallel()
		env := newTestEnv(t, "1.0.0", withKeys)
		target := signManifest(t, priv, manifestFor("1.1.0", ""))
		// An unsigned entry is not inert: min_upgrade_from on any release
		// in the way decides whether the target is installable at all.
		env.publishManifests(target, manifestFor("1.0.5", "0.9.0"))

		if _, err := env.u.Check(context.Background()); err == nil {
			t.Fatal("Check accepted a feed with an unsigned entry beside the signed target")
		}
	})
}

// The payload is a contract with the release tooling: if this changes, every
// published signature stops verifying. It is pinned here on purpose.
func TestManifestSigningPayloadIsStable(t *testing.T) {
	t.Parallel()
	m := manifestFor("1.1.0", "1.0.0")
	m.SizeBytes = 4096
	want := strings.Join([]string{
		"vkai-panel-release-manifest-v1",
		"1.1.0",
		"2026-02-01T00:00:00Z",
		"1.0.0",
		testTarBase + "vkai-panel-1.1.0.tar.gz",
		m.SHA256,
		"4096",
		"https://docs.example.test/changelog/1.1.0",
	}, "\n") + "\n"

	if got := string(ManifestSigningPayload(m)); got != want {
		t.Errorf("ManifestSigningPayload =\n%q\nwant\n%q", got, want)
	}
}

func TestNewRejectsMalformedReleaseKeys(t *testing.T) {
	t.Parallel()
	for _, key := range []string{"nothex", hex.EncodeToString([]byte("too short"))} {
		if _, err := New(Config{
			Root: "/vkai-panel", FeedURL: "https://example.test/feed.json",
			ReleasePublicKeys: []string{key},
		}, Deps{}); err == nil {
			t.Errorf("New accepted the malformed release key %q", key)
		}
	}
}

// With no keys configured the feed is trusted on TLS alone. That is the default
// and it is a known risk, not an oversight - so it is at least reported.
func TestUnsignedFeedIsReportedAsUnverified(t *testing.T) {
	t.Parallel()
	env := newTestEnv(t, "1.0.0", nil)
	env.publish("1.1.0", "")

	res, err := env.u.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if res.SignaturesChecked {
		t.Error("CheckResult claims signatures were checked with no keys configured")
	}
}
