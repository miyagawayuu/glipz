package httpserver

import (
	"net"
	"net/url"
	"strings"
	"unicode"
)

func normalizeOAuthRedirectURI(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || strings.IndexFunc(trimmed, unicode.IsControl) >= 0 || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", false
	}
	u, err := url.Parse(trimmed)
	if err != nil || u == nil || u.IsAbs() == false || strings.TrimSpace(u.Host) == "" {
		return "", false
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return "", false
	}
	if u.User != nil || u.Fragment != "" {
		return "", false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host == "" {
		return "", false
	}
	if scheme == "http" && !isLocalOAuthRedirectHost(host) {
		return "", false
	}
	u.Scheme = scheme
	if port := strings.TrimSpace(u.Port()); port != "" {
		u.Host = net.JoinHostPort(host, port)
	} else {
		u.Host = host
	}
	return u.String(), true
}

func isLocalOAuthRedirectHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
