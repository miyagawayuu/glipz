package mailer

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
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
	SMTPHost  string
	SMTPPort  string
	SMTPUser  string
	SMTPPass  string
	SMTPTLS   string
}

func (c Config) Enabled() bool {
	return c.MailgunEnabled() || c.SMTPEnabled()
}

func (c Config) MailgunEnabled() bool {
	return strings.TrimSpace(c.Domain) != "" && strings.TrimSpace(c.APIKey) != "" && strings.TrimSpace(c.FromEmail) != ""
}

func (c Config) SMTPEnabled() bool {
	return strings.TrimSpace(c.SMTPHost) != "" && strings.TrimSpace(c.FromEmail) != ""
}

func (c Config) fromHeader() string {
	name := strings.TrimSpace(c.FromName)
	if name == "" {
		return c.FromEmail
	}
	return (&mail.Address{Name: name, Address: c.FromEmail}).String()
}

func SendText(cfg Config, to, subject, body string) error {
	if cfg.MailgunEnabled() {
		return sendTextMailgun(cfg, to, subject, body)
	}
	if cfg.SMTPEnabled() {
		return sendTextSMTP(cfg, to, subject, body)
	}
	return fmt.Errorf("mailer is not configured")
}

func sendTextMailgun(cfg Config, to, subject, body string) error {
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

func sendTextSMTP(cfg Config, to, subject, body string) error {
	host := strings.TrimSpace(cfg.SMTPHost)
	port := smtpPort(cfg.SMTPPort)
	addr := net.JoinHostPort(host, port)
	tlsMode := smtpTLSMode(cfg.SMTPTLS, port)

	dialer := &net.Dialer{Timeout: 30 * time.Second}
	var conn net.Conn
	var err error
	if tlsMode == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, smtpTLSConfig(host))
	} else {
		conn, err = dialer.DialContext(context.Background(), "tcp", addr)
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		return err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return err
	}
	defer client.Close()

	if tlsMode == "starttls" || tlsMode == "auto" {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(smtpTLSConfig(host)); err != nil {
				return err
			}
		} else if tlsMode == "starttls" {
			return fmt.Errorf("smtp server does not advertise STARTTLS")
		}
	}

	user := strings.TrimSpace(cfg.SMTPUser)
	pass := strings.TrimSpace(cfg.SMTPPass)
	if user != "" || pass != "" {
		if err := client.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
			return err
		}
	}

	fromAddr, err := parseEmailAddress(cfg.FromEmail)
	if err != nil {
		return fmt.Errorf("invalid from email: %w", err)
	}
	toAddr, err := parseEmailAddress(to)
	if err != nil {
		return fmt.Errorf("invalid recipient email: %w", err)
	}
	if err := client.Mail(fromAddr); err != nil {
		return err
	}
	if err := client.Rcpt(toAddr); err != nil {
		return err
	}
	w, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(buildTextMessage(cfg.fromHeader(), toAddr, subject, body)); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return client.Quit()
}

func smtpPort(raw string) string {
	port := strings.TrimSpace(raw)
	if port == "" {
		return "587"
	}
	if _, err := strconv.Atoi(port); err != nil {
		return "587"
	}
	return port
}

func smtpTLSMode(raw, port string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "none", "false", "off", "disable", "disabled":
		return "none"
	case "tls", "ssl", "implicit":
		return "tls"
	case "starttls", "true", "on", "enable", "enabled", "required":
		return "starttls"
	default:
		if port == "465" {
			return "tls"
		}
		return "auto"
	}
}

func smtpTLSConfig(host string) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: host,
	}
}

func parseEmailAddress(raw string) (string, error) {
	addr, err := mail.ParseAddress(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	return addr.Address, nil
}

func buildTextMessage(from, to, subject, body string) []byte {
	var encodedBody bytes.Buffer
	qp := quotedprintable.NewWriter(&encodedBody)
	_, _ = qp.Write([]byte(body))
	_ = qp.Close()

	headers := []string{
		"From: " + from,
		"To: " + to,
		"Subject: " + mime.QEncoding.Encode("utf-8", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: quoted-printable",
	}
	return []byte(strings.Join(headers, "\r\n") + "\r\n\r\n" + encodedBody.String() + "\r\n")
}
