package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const identityTransferHTTPTimeout = 20 * time.Second

func normalizeTransferOrigin(raw string) (string, bool, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false, err
	}
	if u.Scheme == "" || u.Host == "" || u.User != nil {
		return "", false, fmt.Errorf("invalid origin")
	}
	if u.Path != "" && u.Path != "/" || u.RawQuery != "" || u.Fragment != "" {
		return "", false, fmt.Errorf("origin must not include path, query, or fragment")
	}
	host := u.Hostname()
	isLocal := isLoopbackHost(host)
	switch strings.ToLower(u.Scheme) {
	case "https":
	case "http":
		if !isLocal {
			return "", false, fmt.Errorf("http origin is only allowed for local development")
		}
	default:
		return "", false, fmt.Errorf("unsupported origin scheme")
	}
	if !isLocal {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := rejectUnsafeResolvedHost(ctx, host); err != nil {
			return "", false, err
		}
	}
	u.Path, u.RawQuery, u.Fragment = "", "", ""
	return strings.TrimRight(u.String(), "/"), isLocal, nil
}

func transferURL(origin, path string) (string, error) {
	base, err := url.Parse(strings.TrimRight(strings.TrimSpace(origin), "/"))
	if err != nil {
		return "", err
	}
	if base.Scheme == "" || base.Host == "" {
		return "", fmt.Errorf("invalid origin")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	base.Path = path
	base.RawQuery = ""
	base.Fragment = ""
	return base.String(), nil
}

func newIdentityTransferHTTPClient() *http.Client {
	return &http.Client{
		Timeout: identityTransferHTTPTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func validateTransferRequestURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	origin := &url.URL{Scheme: u.Scheme, Host: u.Host}
	_, _, err = normalizeTransferOrigin(origin.String())
	return err
}

func rejectUnsafeResolvedHost(ctx context.Context, host string) error {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("host has no addresses")
	}
	for _, addr := range ips {
		if isUnsafeRemoteIP(addr.IP) {
			return fmt.Errorf("origin resolves to a private or reserved address")
		}
	}
	return nil
}

func isLoopbackHost(host string) bool {
	h := strings.Trim(strings.ToLower(host), "[]")
	if h == "localhost" {
		return true
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func isUnsafeRemoteIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		ip.IsMulticast()
}
