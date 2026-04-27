package httpserver

import "testing"

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
