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
	// TURN (coturn shared-secret / time-limited credentials).
	// When set, DM calls can use TURN.
	TurnHost         string
	TurnPort         int
	TurnsPort        int
	TurnSharedSecret string
	TurnTTLSeconds   int
	WebPushVAPIDPublicKey  string
	WebPushVAPIDPrivateKey string
	WebPushVAPIDSubject    string
	// Patreon (fan club). Optional. When PatreonClientID is empty, Patreon API routes are unavailable.
	PatreonClientID     string
	PatreonClientSecret string
	// Optional override. Default: {GLIPZ_PROTOCOL_PUBLIC_ORIGIN}/api/v1/fanclub/patreon/callback
	PatreonRedirectURI string
}

func Load() (Config, error) {
	c := Config{
		Port:             getEnv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		RedisURL:         os.Getenv("REDIS_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
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
	if c.JWTSecret == "" || len(c.JWTSecret) < 16 {
		return c, fmt.Errorf("JWT_SECRET must be set and at least 16 characters")
	}
	if c.S3Endpoint == "" || c.S3AccessKey == "" || c.S3SecretKey == "" || c.S3Bucket == "" {
		return c, fmt.Errorf("S3_ENDPOINT, S3_ACCESS_KEY, S3_SECRET_KEY, S3_BUCKET are required")
	}
	if c.S3PublicEndpoint == "" {
		c.S3PublicEndpoint = c.S3Endpoint
	}
	c.S3UsePathStyle = strings.EqualFold(os.Getenv("S3_USE_PATH_STYLE"), "true") ||
		strings.HasPrefix(c.S3Endpoint, "http://minio")
	if v := os.Getenv("S3_USE_PATH_STYLE"); v != "" {
		c.S3UsePathStyle, _ = strconv.ParseBool(v)
	}
	c.StaticWebRoot = strings.TrimSpace(os.Getenv("STATIC_WEB_ROOT"))
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
	c.TurnHost = strings.TrimSpace(os.Getenv("TURN_HOST"))
	c.TurnPort = 3478
	if raw := strings.TrimSpace(os.Getenv("TURN_PORT")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 65535 {
			c.TurnPort = v
		}
	}
	c.TurnsPort = 443
	if raw := strings.TrimSpace(os.Getenv("TURNS_PORT")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 65535 {
			c.TurnsPort = v
		}
	}
	c.TurnSharedSecret = strings.TrimSpace(os.Getenv("TURN_SHARED_SECRET"))
	c.TurnTTLSeconds = 600
	if raw := strings.TrimSpace(os.Getenv("TURN_TTL_SECONDS")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			c.TurnTTLSeconds = v
		}
	}
	c.WebPushVAPIDPublicKey = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PUBLIC_KEY"))
	c.WebPushVAPIDPrivateKey = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_PRIVATE_KEY"))
	c.WebPushVAPIDSubject = strings.TrimSpace(os.Getenv("WEB_PUSH_VAPID_SUBJECT"))
	c.PatreonClientID = strings.TrimSpace(os.Getenv("PATREON_CLIENT_ID"))
	c.PatreonClientSecret = strings.TrimSpace(os.Getenv("PATREON_CLIENT_SECRET"))
	c.PatreonRedirectURI = strings.TrimSpace(os.Getenv("PATREON_REDIRECT_URI"))
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
