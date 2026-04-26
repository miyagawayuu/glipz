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

func newPublicOutboundHTTPClient(timeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = publicOutboundDialContext
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}
			_, err := validatePublicOutboundURL(req.Context(), req.URL.String(), true)
			return err
		},
	}
}

func validatePublicOutboundURL(ctx context.Context, raw string, allowHTTP bool) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return nil, fmt.Errorf("invalid url")
	}
	if u.User != nil || strings.TrimSpace(u.Host) == "" {
		return nil, fmt.Errorf("invalid host")
	}
	switch strings.ToLower(strings.TrimSpace(u.Scheme)) {
	case "https":
	case "http":
		if !allowHTTP {
			return nil, fmt.Errorf("http not allowed")
		}
	default:
		return nil, fmt.Errorf("unsupported scheme")
	}
	if err := ensurePublicOutboundHost(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

func ensurePublicOutboundHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(strings.Trim(host, "[]"))
	if host == "" {
		return fmt.Errorf("empty host")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") || strings.HasSuffix(lower, ".local") {
		return fmt.Errorf("local host not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if !isPublicOutboundIP(ip) {
			return fmt.Errorf("private ip not allowed")
		}
		return nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil || len(addrs) == 0 {
		return fmt.Errorf("dns lookup failed")
	}
	for _, addr := range addrs {
		if !isPublicOutboundIP(addr.IP) {
			return fmt.Errorf("private ip not allowed")
		}
	}
	return nil
}

func isPublicOutboundIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return !(ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsUnspecified())
}

func isSafeRemoteHost(host string) bool {
	h := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(strings.ToLower(host)), "https://"), "http://")
	h = strings.Trim(strings.TrimSuffix(h, "/"), "[]")
	if h == "" || h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".local") {
		return false
	}
	return net.ParseIP(h) == nil
}

func publicOutboundDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if err := ensurePublicOutboundHost(ctx, host); err != nil {
		return nil, err
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("dns lookup failed")
	}
	dialer := &net.Dialer{}
	var lastErr error
	for _, addr := range addrs {
		if !isPublicOutboundIP(addr.IP) {
			return nil, fmt.Errorf("private ip not allowed")
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("dial failed")
}
