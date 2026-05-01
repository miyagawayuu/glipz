package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

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

func TestLoadSMTPSettings(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://glipz:glipz_dev@127.0.0.1:5432/glipz?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
	t.Setenv("JWT_SECRET", "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ__")
	t.Setenv("GLIPZ_STORAGE_MODE", "local")
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "587")
	t.Setenv("SMTP_USERNAME", "smtp-user")
	t.Setenv("SMTP_PASSWORD", "smtp-pass")
	t.Setenv("SMTP_TLS", "starttls")
	t.Setenv("MAIL_FROM_EMAIL", "")
	t.Setenv("MAIL_FROM_NAME", "")
	t.Setenv("SMTP_FROM_EMAIL", "no-reply@example.com")
	t.Setenv("SMTP_FROM_NAME", "Glipz")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != "587" {
		t.Fatalf("SMTP host/port = %q/%q, want smtp.example.com/587", cfg.SMTPHost, cfg.SMTPPort)
	}
	if cfg.SMTPUser != "smtp-user" || cfg.SMTPPass != "smtp-pass" || cfg.SMTPTLS != "starttls" {
		t.Fatalf("SMTP auth/tls not loaded as expected")
	}
	if cfg.MailFromEmail != "no-reply@example.com" || cfg.MailFromName != "Glipz" {
		t.Fatalf("mail sender = %q/%q, want SMTP aliases", cfg.MailFromEmail, cfg.MailFromName)
	}
}

func TestLoadFederationKeyAndTrustedProxyCIDRs(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://glipz:glipz_dev@127.0.0.1:5432/glipz?sslmode=disable")
	t.Setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
	t.Setenv("JWT_SECRET", "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ__")
	t.Setenv("GLIPZ_STORAGE_MODE", "local")
	t.Setenv("GLIPZ_FEDERATION_KEY_SEED", base64.StdEncoding.EncodeToString([]byte(strings.Repeat("a", 32))))
	t.Setenv("GLIPZ_TRUSTED_PROXY_CIDRS", "203.0.113.0/24, 2001:db8::/32")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FederationKeySeed == "" {
		t.Fatal("FederationKeySeed was not loaded")
	}
	if got := cfg.TrustedProxyCIDRs; len(got) != 2 || got[0] != "203.0.113.0/24" || got[1] != "2001:db8::/32" {
		t.Fatalf("TrustedProxyCIDRs = %#v, want two parsed CIDRs", got)
	}
}

func TestLoadRejectsInvalidFederationKeyAndTrustedProxyCIDR(t *testing.T) {
	baseEnv := func() {
		t.Setenv("DATABASE_URL", "postgres://glipz:glipz_dev@127.0.0.1:5432/glipz?sslmode=disable")
		t.Setenv("REDIS_URL", "redis://127.0.0.1:6379/0")
		t.Setenv("JWT_SECRET", "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ__")
		t.Setenv("GLIPZ_STORAGE_MODE", "local")
	}

	t.Run("bad key length", func(t *testing.T) {
		baseEnv()
		t.Setenv("GLIPZ_FEDERATION_KEY_SEED", base64.StdEncoding.EncodeToString([]byte("short")))
		if _, err := Load(); err == nil {
			t.Fatal("Load() accepted short federation key seed")
		}
	})

	t.Run("bad cidr", func(t *testing.T) {
		baseEnv()
		t.Setenv("GLIPZ_TRUSTED_PROXY_CIDRS", "not-a-cidr")
		if _, err := Load(); err == nil {
			t.Fatal("Load() accepted invalid trusted proxy CIDR")
		}
	})
}
