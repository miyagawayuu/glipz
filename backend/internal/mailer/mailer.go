package mailer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mailgun/mailgun-go/v4"
)

type Config struct {
	Domain    string
	APIKey    string
	APIBase   string
	FromEmail string
	FromName  string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Domain) != "" && strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.FromEmail) != ""
}

func (c Config) fromHeader() string {
	name := strings.TrimSpace(c.FromName)
	if name == "" {
		return c.FromEmail
	}
	return fmt.Sprintf("%s <%s>", name, c.FromEmail)
}

func SendText(cfg Config, to, subject, body string) error {
	mg := mailgun.NewMailgun(strings.TrimSpace(cfg.Domain), strings.TrimSpace(cfg.APIKey))
	if apiBase := strings.TrimSpace(cfg.APIBase); apiBase != "" {
		mg.SetAPIBase(apiBase)
	}
	msg := mg.NewMessage(cfg.fromHeader(), subject, body, strings.TrimSpace(to))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, _, err := mg.Send(ctx, msg)
	return err
}
