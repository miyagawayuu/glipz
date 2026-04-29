package mailer

import (
	"strings"
	"testing"
)

func TestEnabledAcceptsSMTPWithoutMailgun(t *testing.T) {
	cfg := Config{
		SMTPHost:  "smtp.example.com",
		FromEmail: "no-reply@example.com",
	}
	if !cfg.Enabled() {
		t.Fatalf("Enabled() = false, want true")
	}
	if !cfg.SMTPEnabled() {
		t.Fatalf("SMTPEnabled() = false, want true")
	}
	if cfg.MailgunEnabled() {
		t.Fatalf("MailgunEnabled() = true, want false")
	}
}

func TestSMTPTLSModeDefaults(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		port string
		want string
	}{
		{name: "auto starttls", raw: "", port: "587", want: "auto"},
		{name: "implicit tls on 465", raw: "", port: "465", want: "tls"},
		{name: "explicit none", raw: "none", port: "587", want: "none"},
		{name: "explicit starttls", raw: "starttls", port: "25", want: "starttls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smtpTLSMode(tt.raw, tt.port); got != tt.want {
				t.Fatalf("smtpTLSMode(%q, %q) = %q, want %q", tt.raw, tt.port, got, tt.want)
			}
		})
	}
}

func TestBuildTextMessageEncodesUTF8(t *testing.T) {
	msg := string(buildTextMessage("Glipz <no-reply@example.com>", "user@example.com", "Glipz メール認証", "本文です"))
	for _, want := range []string{
		"From: Glipz <no-reply@example.com>",
		"To: user@example.com",
		"Subject: =?utf-8?q?Glipz_=E3=83=A1=E3=83=BC=E3=83=AB=E8=AA=8D=E8=A8=BC?=",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q:\n%s", want, msg)
		}
	}
}
