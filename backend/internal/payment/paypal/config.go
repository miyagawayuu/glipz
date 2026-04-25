package paypal

import (
	"strings"
)

type Env string

const (
	EnvSandbox Env = "sandbox"
	EnvLive    Env = "live"
)

type Config struct {
	ClientID     string
	ClientSecret string
	WebhookID    string
	Env          Env
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.ClientID) != "" && strings.TrimSpace(c.ClientSecret) != "" && strings.TrimSpace(c.WebhookID) != ""
}

func (c Config) apiBase() string {
	if strings.EqualFold(string(c.Env), string(EnvLive)) {
		return "https://api-m.paypal.com"
	}
	return "https://api-m.sandbox.paypal.com"
}

