package httpserver

import (
	"expvar"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	httpRequestsTotal = expvar.NewMap("glipz_http_requests_total")
	httpRequestMs     = expvar.NewMap("glipz_http_request_duration_ms_total")
	httpInFlight      = expvar.NewInt("glipz_http_in_flight")

	operationTotal = expvar.NewMap("glipz_operation_total")
	operationMs    = expvar.NewMap("glipz_operation_duration_ms_total")
	operationErrs  = expvar.NewMap("glipz_operation_errors_total")

	sseActive = expvar.NewMap("glipz_sse_active")
	sseOpened = expvar.NewMap("glipz_sse_opened_total")
	sseClosed = expvar.NewMap("glipz_sse_closed_total")

	mediaProxyBytes = expvar.NewMap("glipz_media_proxy_bytes_total")

	federationDeliveryTotal = expvar.NewMap("glipz_federation_delivery_total")
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (r *statusRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func observeRequests(slowLogThreshold time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			httpInFlight.Add(1)
			rec := &statusRecorder{ResponseWriter: w}
			defer func() {
				httpInFlight.Add(-1)
				status := rec.status
				if status == 0 {
					status = http.StatusOK
				}
				route := ""
				if rc := chi.RouteContext(r.Context()); rc != nil {
					route = rc.RoutePattern()
				}
				if route == "" {
					route = r.URL.Path
				}
				key := metricKey(r.Method, route, fmt.Sprintf("%d", status))
				elapsed := time.Since(start)
				httpRequestsTotal.Add(key, 1)
				httpRequestMs.Add(key, elapsed.Milliseconds())
				if (slowLogThreshold > 0 && elapsed >= slowLogThreshold) || status >= 500 {
					log.Printf("http_request method=%s route=%q status=%d bytes=%d duration_ms=%d", r.Method, route, status, rec.bytes, elapsed.Milliseconds())
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

func metricKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.ReplaceAll(p, " ", "_")
		if p == "" {
			p = "unknown"
		}
		clean = append(clean, p)
	}
	return strings.Join(clean, "|")
}

func observeOperation(name string, started time.Time, err error) {
	elapsed := time.Since(started)
	operationTotal.Add(name, 1)
	operationMs.Add(name, elapsed.Milliseconds())
	if err != nil {
		operationErrs.Add(name, 1)
	}
	if elapsed >= 500*time.Millisecond || err != nil {
		log.Printf("operation name=%s duration_ms=%d error=%v", name, elapsed.Milliseconds(), err)
	}
}

func trackSSEOpen(name string) {
	sseActive.Add(name, 1)
	sseOpened.Add(name, 1)
}

func trackSSEClose(name string) {
	sseActive.Add(name, -1)
	sseClosed.Add(name, 1)
}

func addMediaProxyBytes(kind string, n int64) {
	if n > 0 {
		mediaProxyBytes.Add(kind, n)
	}
}

func observeFederationDelivery(status string, elapsed time.Duration) {
	federationDeliveryTotal.Add(status, 1)
	operationTotal.Add("federation_delivery."+status, 1)
	operationMs.Add("federation_delivery."+status, elapsed.Milliseconds())
}
