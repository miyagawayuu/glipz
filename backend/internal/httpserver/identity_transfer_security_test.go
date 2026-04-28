package httpserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"glipz.io/backend/internal/repo"
)

func TestIdentityBundleV2EncryptDecrypt(t *testing.T) {
	enc, err := encryptIdentityPrivateKey("correct horse battery staple", "PRIVATE_KEY_BYTES")
	if err != nil {
		t.Fatalf("encryptIdentityPrivateKey: %v", err)
	}
	got, err := decryptIdentityPrivateKey("correct horse battery staple", enc)
	if err != nil {
		t.Fatalf("decryptIdentityPrivateKey: %v", err)
	}
	if got != "PRIVATE_KEY_BYTES" {
		t.Fatalf("decrypted private key = %q", got)
	}
	if _, err := decryptIdentityPrivateKey("wrong horse battery staple", enc); err == nil {
		t.Fatal("decryptIdentityPrivateKey with wrong passphrase succeeded")
	}
}

func TestIdentityBundleV2EncryptDecryptsToValidPortableIdentity(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	enc := base64.RawURLEncoding
	sum := sha256.Sum256(pub)
	identity := repo.PortableIdentity{
		PortableID:                 repo.PortableIDPrefix + enc.EncodeToString(sum[:]),
		AccountPublicKey:           enc.EncodeToString(pub),
		AccountPrivateKeyEncrypted: enc.EncodeToString(priv),
	}

	bundleKey, err := encryptIdentityPrivateKey("correct horse battery staple", identity.AccountPrivateKeyEncrypted)
	if err != nil {
		t.Fatalf("encryptIdentityPrivateKey: %v", err)
	}
	decryptedPrivateKey, err := decryptIdentityPrivateKey("correct horse battery staple", bundleKey)
	if err != nil {
		t.Fatalf("decryptIdentityPrivateKey: %v", err)
	}
	identity.AccountPrivateKeyEncrypted = decryptedPrivateKey
	if _, err := repo.ValidatePortableIdentity(identity); err != nil {
		t.Fatalf("ValidatePortableIdentity: %v", err)
	}
}

func TestTransferTokenHashComparison(t *testing.T) {
	hash := hashTransferToken("token-value")
	if !constantEqualBase64Hash(hash, hashTransferToken("token-value")) {
		t.Fatal("same token did not compare equal")
	}
	if constantEqualBase64Hash(hash, hashTransferToken("other-token")) {
		t.Fatal("different token compared equal")
	}
}

func TestNormalizeTransferOriginRejectsUnsafeOrigins(t *testing.T) {
	if _, _, err := normalizeTransferOrigin("http://example.com"); err == nil {
		t.Fatal("non-local http origin accepted")
	}
	if _, _, err := normalizeTransferOrigin("https://example.com/path"); err == nil {
		t.Fatal("origin with path accepted")
	}
	if _, local, err := normalizeTransferOrigin("http://127.0.0.1:8080"); err != nil || !local {
		t.Fatalf("local development origin rejected: local=%v err=%v", local, err)
	}
}
