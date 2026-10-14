package server

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCSPHashesOnlyExactInlineScriptBytes(t *testing.T) {
	script := "self.__next_f.push([1,\"hello 世界\"]);"
	policy := cspForHTML([]byte("<html><script>" + script + "</script><script src='/bundle.js'></script><div>other text</div></html>"))
	hash := sha256.Sum256([]byte(script))
	if !strings.Contains(policy, "'sha256-"+base64.StdEncoding.EncodeToString(hash[:])+"'") {
		t.Fatal("inline hydration not authorized")
	}
	if strings.Contains(policy[strings.Index(policy, "script-src"):], "unsafe-inline") {
		t.Fatal("arbitrary inline scripts authorized")
	}
	changed := sha256.Sum256([]byte(script + "alert(1)"))
	if strings.Contains(policy, base64.StdEncoding.EncodeToString(changed[:])) {
		t.Fatal("modified script authorized")
	}
}

func TestAuthRejectsURLAndCookieCredentials(t *testing.T) {
	key := []byte("test-security-signing-key-32-bytes")
	token, err := signJWT(key)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{jwtKey: key}
	h := s.requireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for _, path := range []string{"/api/tasks", "/api/events"} {
		for _, channel := range []string{"header", "query", "cookie"} {
			r := httptest.NewRequest("GET", path, nil)
			switch channel {
			case "header":
				r.Header.Set("Authorization", "Bearer "+token)
			case "query":
				r.URL.RawQuery = "token=" + url.QueryEscape(token)
			case "cookie":
				r.AddCookie(&http.Cookie{Name: "artex_token", Value: token})
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			want := 401
			if channel == "header" {
				want = 204
			}
			if w.Code != want {
				t.Fatalf("%s %s: got %d want %d", path, channel, w.Code, want)
			}
		}
	}
}

func TestLoginLimiterConcurrentAndExpiry(t *testing.T) {
	var l loginLimiter
	var allowed atomic.Int32
	var wg sync.WaitGroup
	now := time.Now()
	for range 100 {
		wg.Go(func() {
			if l.allow("127.0.0.1:1234", now) {
				allowed.Add(1)
			}
		})
	}
	wg.Wait()
	if allowed.Load() != 10 {
		t.Fatalf("admitted %d concurrent attempts", allowed.Load())
	}
	if l.allow("127.0.0.1:5678", now) {
		t.Fatal("new source port bypassed cap")
	}
	if !l.allow("127.0.0.1:5678", now.Add(time.Minute)) {
		t.Fatal("window did not reset")
	}
}

func TestLoginLimiterGlobalBound(t *testing.T) {
	var l loginLimiter
	now := time.Now()
	for i := range 60 {
		if !l.allow(string(rune(i+1000)), now) {
			t.Fatal("early global cap")
		}
	}
	if l.allow("another", now) {
		t.Fatal("global cap bypassed")
	}
	if len(l.clients) != 60 {
		t.Fatal("unexpected map size")
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	for _, path := range []string{"/login", "/function/tasks", "/api/auth/login", "/favicon.ico"} {
		r := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		for _, name := range []string{"Content-Security-Policy", "X-Frame-Options", "X-Content-Type-Options", "Referrer-Policy", "Permissions-Policy"} {
			if w.Header().Get(name) == "" {
				t.Fatalf("%s missing on %s", name, path)
			}
		}
		if w.Header().Get("Strict-Transport-Security") != "" {
			t.Fatal("HSTS on plaintext")
		}
		r.TLS = &tls.ConnectionState{}
		w = httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("Strict-Transport-Security") == "" {
			t.Fatal("HSTS missing on TLS")
		}
	}
}
