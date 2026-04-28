package repo

import (
	"crypto/ed25519"
	"encoding/base64"
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

func TestSetUserPortableIdentityNormalizesLocalPlaceholderID(t *testing.T) {
	identity, err := newPortableIdentity()
	if err != nil {
		t.Fatalf("newPortableIdentity: %v", err)
	}
	pub, err := base64.RawURLEncoding.DecodeString(identity.AccountPublicKey)
	if err != nil {
		t.Fatalf("public key decode: %v", err)
	}
	if got := portableIDForPublicKey(ed25519.PublicKey(pub)); got != identity.PortableID {
		t.Fatalf("portableIDForPublicKey = %q, want %q", got, identity.PortableID)
	}
	if !isLocalPlaceholderPortableID("glipz:id:local-123") {
		t.Fatal("local placeholder was not detected")
	}
}

func TestValidatePortableIdentityRejectsMismatchedPortableID(t *testing.T) {
	identity, err := newPortableIdentity()
	if err != nil {
		t.Fatalf("newPortableIdentity: %v", err)
	}
	identity.PortableID = PortableIDPrefix + "wrong"
	if _, err := ValidatePortableIdentity(identity); err == nil {
		t.Fatal("mismatched portable id was accepted")
	}
}

func TestValidatePortableIdentityRejectsMismatchedPrivateKey(t *testing.T) {
	identity, err := newPortableIdentity()
	if err != nil {
		t.Fatalf("newPortableIdentity: %v", err)
	}
	other, err := newPortableIdentity()
	if err != nil {
		t.Fatalf("newPortableIdentity other: %v", err)
	}
	identity.AccountPrivateKeyEncrypted = other.AccountPrivateKeyEncrypted
	if _, err := ValidatePortableIdentity(identity); err == nil {
		t.Fatal("mismatched private key was accepted")
	}
}
