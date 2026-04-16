package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestLimiter(rate int, window time.Duration) *RateLimiter {
	// Same as NewRateLimiter but without background goroutine for tests.
	return &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
}

func TestAllowUpToLimit(t *testing.T) {
	rl := newTestLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
}

func TestAllowBlocksAfterLimit(t *testing.T) {
	rl := newTestLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		rl.Allow("1.2.3.4")
	}
	if rl.Allow("1.2.3.4") {
		t.Fatal("request after limit should be denied")
	}
}

func TestWindowResetAllowsRequests(t *testing.T) {
	rl := newTestLimiter(2, 50*time.Millisecond)
	rl.Allow("1.2.3.4")
	rl.Allow("1.2.3.4")
	if rl.Allow("1.2.3.4") {
		t.Fatal("should be denied before window reset")
	}
	time.Sleep(60 * time.Millisecond)
	if !rl.Allow("1.2.3.4") {
		t.Fatal("should be allowed after window reset")
	}
}

func TestIndependentIPLimits(t *testing.T) {
	rl := newTestLimiter(1, time.Minute)
	if !rl.Allow("10.0.0.1") {
		t.Fatal("first IP first request should be allowed")
	}
	if rl.Allow("10.0.0.1") {
		t.Fatal("first IP second request should be denied")
	}
	if !rl.Allow("10.0.0.2") {
		t.Fatal("second IP first request should be allowed")
	}
}

func TestMiddleware429(t *testing.T) {
	rl := newTestLimiter(1, time.Minute)

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request — allowed.
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "5.5.5.5:12345"
	rec := httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Second request — rate limited.
	rec = httptest.NewRecorder()
	handler(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Fatalf("unexpected error: %v", body["error"])
	}
	if _, ok := body["retry_after_sec"]; !ok {
		t.Fatal("expected retry_after_sec in body")
	}
}

func TestMiddlewareXForwardedFor(t *testing.T) {
	rl := newTestLimiter(1, time.Minute)

	handler := rl.Middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use X-Forwarded-For so two requests from different proxied IPs are independent.
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	req1.RemoteAddr = "127.0.0.1:9999"
	req1.Header.Set("X-Forwarded-For", "8.8.8.8, 127.0.0.1")

	rec := httptest.NewRecorder()
	handler(rec, req1)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// Same XFF IP — should be limited.
	rec = httptest.NewRecorder()
	handler(rec, req1)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}

	// Different XFF IP — should be allowed.
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.RemoteAddr = "127.0.0.1:9999"
	req2.Header.Set("X-Forwarded-For", "9.9.9.9")
	rec = httptest.NewRecorder()
	handler(rec, req2)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for different XFF IP, got %d", rec.Code)
	}
}
