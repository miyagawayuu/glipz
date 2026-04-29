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
