package httpserver

import "net/http"

const defaultContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob: https:; media-src 'self' blob: https:; connect-src 'self'; frame-src https://www.youtube-nocookie.com https://player.vimeo.com https://www.dailymotion.com https://www.loom.com https://streamable.com https://fast.wistia.net https://player.bilibili.com https://www.tiktok.com https://store.steampowered.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setHeaderIfEmpty(w, "Content-Security-Policy", defaultContentSecurityPolicy)
		setHeaderIfEmpty(w, "Referrer-Policy", "strict-origin-when-cross-origin")
		setHeaderIfEmpty(w, "Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		setHeaderIfEmpty(w, "X-Content-Type-Options", "nosniff")
		setHeaderIfEmpty(w, "X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func setHeaderIfEmpty(w http.ResponseWriter, key, value string) {
	if w.Header().Get(key) == "" {
		w.Header().Set(key, value)
	}
}
