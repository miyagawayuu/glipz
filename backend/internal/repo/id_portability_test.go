package repo

import (
	"strings"
	"testing"
)

func TestNewPortableIdentityUsesPublicKeyFingerprint(t *testing.T) {
	identity, err := newPortableIdentity()
	if err != nil {
		t.Fatalf("newPortableIdentity: %v", err)
	}
	if !strings.HasPrefix(identity.PortableID, PortableIDPrefix) {
		t.Fatalf("portable id prefix = %q", identity.PortableID)
	}
	if identity.AccountPublicKey == "" {
		t.Fatal("missing account public key")
	}
	if identity.AccountPrivateKeyEncrypted == "" {
		t.Fatal("missing account private key")
	}
}

func TestPortableIDForRemoteFallsBackToLegacyAcct(t *testing.T) {
	got := PortableIDForRemote("Alice@Example.COM", "")
	want := "legacy:alice@example.com"
	if got != want {
		t.Fatalf("PortableIDForRemote fallback = %q, want %q", got, want)
	}
}

func TestNormalizeFederationTargetAcctPreservesPortableIDFingerprintCase(t *testing.T) {
	got := NormalizeFederationTargetAcct(" glipz:id:AbCdEF123 ")
	want := "glipz:id:AbCdEF123"
	if got != want {
		t.Fatalf("NormalizeFederationTargetAcct = %q, want %q", got, want)
	}
}
