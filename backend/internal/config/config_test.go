package config

import "testing"

func TestValidateJWTSecretRejectsWeakValues(t *testing.T) {
	for _, secret := range []string{
		"",
		"short-secret",
		"replace-with-a-long-random-secret",
		"your-very-long-random-secret",
		"replace-with-" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	} {
		if err := validateJWTSecret(secret); err == nil {
			t.Fatalf("validateJWTSecret(%q) = nil, want error", secret)
		}
	}
}

func TestValidateJWTSecretAcceptsLongRandomLookingValue(t *testing.T) {
	secret := "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ__"
	if err := validateJWTSecret(secret); err != nil {
		t.Fatalf("validateJWTSecret(long secret) = %v, want nil", err)
	}
}
