package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Config struct {
	Port             string
	DatabaseURL      string
	RedisURL         string
	JWTSecret        string
	StorageMode      string
	LocalStoragePath string
	S3Endpoint       string
	S3PublicEndpoint string
	S3Region         string
	S3AccessKey      string
	S3SecretKey      string
	S3Bucket         string
	S3UsePathStyle   bool
	// Primary frontend origin, derived from the first FRONTEND_ORIGIN entry.
	FrontendOrigin string
	// Full frontend origin list for CORS. The first entry matches FrontendOrigin.
	FrontendOrigins []string
	// Public base URL used by Glipz federation clients, for example https://api.example.com.
	// When empty, discovery and federation endpoints are not mounted.
	GlipzProtocolPublicOrigin string
	// Host shown in `@user@host`. Defaults to the host from GlipzProtocolPublicOrigin.
	GlipzProtocolHost string
	// Base URL used to build public media URLs, without a trailing slash.
	GlipzProtocolMediaPublicBase string
	// Vue build output directory containing index.html. When set and present, serve static files and the SPA on :PORT.
	StaticWebRoot string
	// Optional directory containing operator-editable Markdown legal documents.
	LegalDocsDir string
	// Software version label exposed in NodeInfo and similar outputs. Defaults to "dev".
	GlipzVersion string
	// Optional short federation policy summary exposed in nodeinfo.metadata.
	FederationPolicySummary string
	// Comma-separated site admin user IDs. Admin federation APIs stay unavailable when empty.
	AdminUserIDs []uuid.UUID
	// Time-to-live for email verification links.
	RegistrationVerifyTTL  time.Duration
	MailgunDomain          string
	MailgunAPIKey          string
	MailgunAPIBase         string
	MailFromEmail          string
	MailFromName           string
	WebPushVAPIDPublicKey  string
	WebPushVAPIDPrivateKey string
	WebPushVAPIDSubject    string
	// Patreon (fan club). Disabled by default; requires PatreonEnabled and OAuth credentials.
	PatreonEnabled      bool
	PatreonClientID     string
	PatreonClientSecret string
	// Optional override. Default: {GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/fanclub/patreon/callback
	PatreonRedirectURI string
	// Gumroad (fan club). Disabled by default; uses Gumroad's public license verification endpoint.
	GumroadEnabled bool
	// PayPal (payment). Disabled by default; requires PayPalEnabled and API credentials.
	PayPalEnabled      bool
	PayPalClientID     string
	PayPalClientSecret string
	PayPalWebhookID    string
	// "sandbox" (default) or "live"
	PayPalEnv string
	// Optional lightweight expvar metrics endpoint at /debug/vars.
	MetricsEnabled bool
	// Optional per-request access logs. Slow requests are still logged by metrics middleware when disabled.
	AccessLogEnabled bool
	// Optional slow request logging threshold in milliseconds. Zero disables slow request logs.
	SlowRequestLogMs int
	// Trust X-Real-IP / X-Forwarded-For from a reverse proxy. Enable only when the proxy overwrites them.
	TrustProxyHeaders bool
	// When true, auth rate limit backend errors reject attempts instead of failing open.
	AuthRateLimitFailClosed bool
	// Number of feed items returned by authenticated feed endpoints.
	FeedPageSize int
	// "proxy" (default) keeps serving local media through the API; "direct" redirects to S3/CDN public URLs.
	MediaProxyMode string
	// Maximum bytes streamed by the public remote media proxy. Defaults to 50 MiB.
	RemoteMediaProxyMaxBytes int64
	// Maximum public remote media proxy requests per IP per 15 minutes.
	RemoteMediaProxyRateLimitMax int
	// When true, remote media proxy rate limit backend errors reject requests instead of failing open.
	RemoteMediaProxyRateLimitFailClosed bool
	// Maximum public link-preview requests per IP/user per 15 minutes.
	LinkPreviewRateLimitMax int
	// When true, link-preview rate limit backend errors reject requests instead of failing open.
	LinkPreviewRateLimitFailClosed bool
	// When true, federation inbox rate limit backend errors reject requests instead of failing open.
	FederationInboxRateLimitFailClosed bool
	// Maximum bytes streamed by the federated DM attachment proxy. Defaults to 50 MiB.
	FederationDMAttachmentMaxBytes int64
	// Federation delivery worker tuning.
	FederationDeliveryBatchSize   int
	FederationDeliveryConcurrency int
	FederationDeliveryTickSeconds int
}

func Load() (Config, error) {
	c := Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		StorageMode:      strings.ToLower(strings.TrimSpace(getEnv("GLIPZ_STORAGE_MODE", "s3"))),
		LocalStoragePath: strings.TrimSpace(getEnv("GLIPZ_LOCAL_STORAGE_PATH", "data/media")),
		S3Endpoint:       os.Getenv("S3_ENDPOINT"),
		S3PublicEndpoint: os.Getenv("S3_PUBLIC_ENDPOINT"),
		S3Region:         getEnv("S3_REGION", "us-east-1"),
		S3AccessKey:      os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:      os.Getenv("S3_SECRET_KEY"),
		S3Bucket:         os.Getenv("S3_BUCKET"),
	}
	if c.DatabaseURL == "" {
		return c, fmt.Errorf("DATABASE_URL is required")
	}
	if c.RedisURL == "" {
		return c, fmt.Errorf("REDIS_URL is required")
	}
	if err := validateJWTSecret(c.JWTSecret); err != nil {
		return c, err
	}
	if c.StorageMode != "local" {
		c.StorageMode = "s3"
	}
	if c.StorageMode == "s3" && (c.S3Endpoint == "" || c.S3AccessKey == "" || c.S3SecretKey == "" || c.S3Bucket == "") {
		return c, fmt.Errorf("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET are required")
	}
	if c.S3PublicEndpoint == "" {
		c.S3PublicEndpoint = c.S3Endpoint
	}
	c.S3UsePathStyle = strings.EqualFold(os.Getenv("S3_USE_PATH_STYLE"), "true") ||
		strings.HasPrefix(c.S3Endpoint, "http://minio") ||
		strings.Contains(strings.ToLower(c.S3Endpoint), ".r2.cloudflarestorage.com")
	if v := os.Getenv("S3_USE_PATH_STYLE"); v != "" {
		c.S3UsePathStyle, _ = strconv.ParseBool(v)
	}
	c.StaticWebRoot = strings.TrimSpace(os.Getenv("STATIC_WEB_ROOT"))
	c.LegalDocsDir = strings.TrimSpace(os.Getenv("LEGAL_DOCS_DIR"))
	fe := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN"))
	if fe == "" {
		if c.StaticWebRoot != "" {
			fe = "http://127.0.0.1:" + strings.TrimSpace(c.Port)
		} else {
			fe = "http://localhost:5173"
		}
	}
	primary, origins := parseCommaOrigins(fe)
	if primary == "" {
		return c, fmt.Errorf("FRONTEND_ORIGIN must contain at least one valid origin")
	}
	c.FrontendOrigin = primary
	c.FrontendOrigins = origins
	c.GlipzProtocolPublicOrigin = strings.TrimSuffix(strings.TrimSpace(os.Getenv("GLIPZ_PROTOCOL_PUBLIC_ORIGIN")), "/")
	c.GlipzProtocolHost = strings.TrimSpace(strings.ToLower(os.Getenv("GLIPZ_PROTOCOL_HOST")))
	c.GlipzProtocolMediaPublicBase = strings.TrimSuffix(strings.TrimSpace(os.Getenv("GLIPZ_PROTOCOL_MEDIA_PUBLIC_BASE")), "/")

	legacyFederationPublicOrigin := strings.TrimSuffix(strings.TrimSpace(os.Getenv("GLIPZ_FEDERATION_PUBLIC_ORIGIN")), "/")
	legacyFederationHost := strings.TrimSpace(strings.ToLower(os.Getenv("GLIPZ_FEDERATION_HOST")))
	legacyMediaPublicBase := strings.TrimSuffix(strings.TrimSpace(os.Getenv("MEDIA_PUBLIC_BASE")), "/")
	legacyProtocolPublicOrigin := strings.TrimSuffix(strings.TrimSpace(os.Getenv("ACTIVITYPUB_PUBLIC_ORIGIN")), "/")
	legacyProtocolHost := strings.TrimSpace(strings.ToLower(os.Getenv("ACTIVITYPUB_WEBFINGER_HOST")))
	legacyProtocolMediaPublicBase := strings.TrimSuffix(strings.TrimSpace(os.Getenv("ACTIVITYPUB_MEDIA_PUBLIC_BASE")), "/")

	if c.GlipzProtocolPublicOrigin == "" {
		c.GlipzProtocolPublicOrigin = legacyFederationPublicOrigin
	}
	if c.GlipzProtocolPublicOrigin == "" {
		c.GlipzProtocolPublicOrigin = legacyProtocolPublicOrigin
	}
	if c.GlipzProtocolHost == "" {
		c.GlipzProtocolHost = legacyFederationHost
	}
	if c.GlipzProtocolHost == "" {
		c.GlipzProtocolHost = legacyProtocolHost
	}
	if c.GlipzProtocolMediaPublicBase == "" {
		c.GlipzProtocolMediaPublicBase = legacyMediaPublicBase
	}
	if c.GlipzProtocolMediaPublicBase == "" {
		c.GlipzProtocolMediaPublicBase = legacyProtocolMediaPublicBase
	}
	c.GlipzVersion = strings.TrimSpace(os.Getenv("GLIPZ_VERSION"))
	if c.GlipzVersion == "" {
		c.GlipzVersion = "dev"
	}
	c.FederationPolicySummary = strings.TrimSpace(os.Getenv("FEDERATION_POLICY_SUMMARY"))
	c.RegistrationVerifyTTL = 30 * time.Minute
	if ttlRaw := strings.TrimSpace(os.Getenv("REGISTRATION_VERIFY_TTL")); ttlRaw != "" {
		ttl, err := time.ParseDuration(ttlRaw)
		if err != nil || ttl <= 0 {
			return c, fmt.Errorf("REGISTRATION_VERIFY_TTL must be a positive duration")
		}
		c.RegistrationVerifyTTL = ttl
	}
	c.MailgunDomain = strings.TrimSpace(os.Getenv("MAILGUN_DOMAIN"))
	c.MailgunAPIKey = strings.TrimSpace(os.Getenv("MAILGUN_API_KEY"))
	c.MailgunAPIBase = strings.TrimSpace(os.Getenv("MAILGUN_API_BASE"))
	c.MailFromEmail = strings.TrimSpace(os.Getenv("MAIL_FROM_EMAIL"))
	c.MailFromName = strings.TrimSpace(os.Getenv("MAIL_FROM_NAME"))
	c.WebPushVAPIDPublicKey = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PUBLIC_KEY"))
	c.WebPushVAPIDPrivateKey = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PRIVATE_KEY"))
	c.WebPushVAPIDSubject = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_SUBJECT"))
	c.PatreonEnabled, _ = strconv.ParseBool(getEnv("PATREON_ENABLED", "false"))
	c.PatreonClientID = strings.TrimSpace(os.Getenv("PATREON_CLIENT_ID"))
	c.PatreonClientSecret = strings.TrimSpace(os.Getenv("PATREON_CLIENT_SECRET"))
	c.PatreonRedirectURI = strings.TrimSpace(os.Getenv("PATREON_REDIRECT_URI"))
	c.GumroadEnabled, _ = strconv.ParseBool(getEnv("GUMROAD_ENABLED", "false"))
	c.PayPalEnabled, _ = strconv.ParseBool(getEnv("PAYPAL_ENABLED", "false"))
	c.PayPalClientID = strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_ID"))
	c.PayPalClientSecret = strings.TrimSpace(os.Getenv("PAYPAL_CLIENT_SECRET"))
	c.PayPalWebhookID = strings.TrimSpace(os.Getenv("PAYPAL_WEBHOOK_ID"))
	c.PayPalEnv = strings.ToLower(strings.TrimSpace(getEnv("PAYPAL_ENV", "sandbox")))
	c.MetricsEnabled, _ = strconv.ParseBool(getEnv("GLIPZ_METRICS_ENABLED", "false"))
	c.AccessLogEnabled, _ = strconv.ParseBool(getEnv("GLIPZ_ACCESS_LOG_ENABLED", "false"))
	c.SlowRequestLogMs = positiveIntEnv("GLIPZ_SLOW_REQUEST_LOG_MS", 0, 0, 600000)
	c.TrustProxyHeaders, _ = strconv.ParseBool(getEnv("GLIPZ_TRUST_PROXY_HEADERS", "false"))
	c.AuthRateLimitFailClosed, _ = strconv.ParseBool(getEnv("GLIPZ_AUTH_RATE_LIMIT_FAIL_CLOSED", "false"))
	c.FeedPageSize = positiveIntEnv("GLIPZ_FEED_PAGE_SIZE", 30, 10, 100)
	c.MediaProxyMode = strings.ToLower(strings.TrimSpace(getEnv("GLIPZ_MEDIA_PROXY_MODE", "proxy")))
	if c.MediaProxyMode != "direct" {
		c.MediaProxyMode = "proxy"
	}
	if c.StorageMode == "local" && c.MediaProxyMode == "direct" && c.GlipzProtocolMediaPublicBase == "" {
		c.MediaProxyMode = "proxy"
	}
	c.RemoteMediaProxyMaxBytes = positiveInt64Env("GLIPZ_REMOTE_MEDIA_PROXY_MAX_BYTES", 50<<20, 1<<20, 512<<20)
	c.RemoteMediaProxyRateLimitMax = positiveIntEnv("GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_MAX", 120, 10, 10000)
	c.RemoteMediaProxyRateLimitFailClosed, _ = strconv.ParseBool(getEnv("GLIPZ_REMOTE_MEDIA_PROXY_RATE_LIMIT_FAIL_CLOSED", "false"))
	c.LinkPreviewRateLimitMax = positiveIntEnv("GLIPZ_LINK_PREVIEW_RATE_LIMIT_MAX", 60, 10, 10000)
	c.LinkPreviewRateLimitFailClosed, _ = strconv.ParseBool(getEnv("GLIPZ_LINK_PREVIEW_RATE_LIMIT_FAIL_CLOSED", "false"))
	c.FederationInboxRateLimitFailClosed, _ = strconv.ParseBool(getEnv("GLIPZ_FEDERATION_INBOX_RATE_LIMIT_FAIL_CLOSED", "false"))
	c.FederationDMAttachmentMaxBytes = positiveInt64Env("GLIPZ_FEDERATION_DM_ATTACHMENT_MAX_BYTES", 50<<20, 1<<20, 512<<20)
	c.FederationDeliveryBatchSize = positiveIntEnv("GLIPZ_FEDERATION_DELIVERY_BATCH_SIZE", 25, 1, 200)
	c.FederationDeliveryConcurrency = positiveIntEnv("GLIPZ_FEDERATION_DELIVERY_CONCURRENCY", 1, 1, 32)
	c.FederationDeliveryTickSeconds = positiveIntEnv("GLIPZ_FEDERATION_DELIVERY_TICK_SECONDS", 8, 1, 300)
	for _, part := range strings.Split(os.Getenv("GLIPZ_ADMIN_USER_IDS"), ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		id, err := uuid.Parse(p)
		if err != nil {
			continue
		}
		c.AdminUserIDs = append(c.AdminUserIDs, id)
	}
	return c, nil
}

func (c Config) WebPushEnabled() bool {
	return c.WebPushVAPIDPublicKey != "" && c.WebPushVAPIDPrivateKey != "" && c.WebPushVAPIDSubject != ""
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func validateJWTSecret(secret string) error {
	trimmed := strings.TrimSpace(secret)
	if len(trimmed) < 64 {
		return fmt.Errorf("JWT_SECRET must be set and at least 64 characters")
	}
	placeholder := strings.ToLower(trimmed)
	switch placeholder {
	case "replace-with-a-long-random-secret",
		"your-very-long-random-secret",
		"change-me",
		"changeme":
		return fmt.Errorf("JWT_SECRET must be replaced with a real random secret")
	}
	if strings.Contains(placeholder, "replace-with") || strings.Contains(placeholder, "your-very-long") {
		return fmt.Errorf("JWT_SECRET must be replaced with a real random secret")
	}
	return nil
}

func positiveIntEnv(key string, def, min, max int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v < min || v > max {
		return def
	}
	return v
}

func positiveInt64Env(key string, def, min, max int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < min || v > max {
		return def
	}
	return v
}

// parseCommaOrigins splits values such as "https://a.example, https://b.example".
// It returns the first valid origin as the primary origin for email links and similar uses.
func parseCommaOrigins(raw string) (primary string, all []string) {
	for _, part := range strings.Split(raw, ",") {
		o := strings.TrimSpace(strings.TrimSuffix(part, "/"))
		if o == "" {
			continue
		}
		all = append(all, o)
	}
	if len(all) > 0 {
		primary = all[0]
	}
	return primary, all
}
