package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"golang.org/x/net/html"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const cspBase = "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'; form-action 'self'; script-src 'self'"

// Static Next hydration code is approved by its exact bytes, rather than allowing
// every inline script. Compute once per embedded document at the serving layer.
func cspForHTML(document []byte) string {
	policy := cspBase
	z := html.NewTokenizer(bytes.NewReader(document))
	inScript := false
	seen := map[string]bool{}
	for {
		switch z.Next() {
		case html.ErrorToken:
			return policy
		case html.StartTagToken:
			name, _ := z.TagName()
			inScript = strings.EqualFold(string(name), "script")
		case html.EndTagToken:
			inScript = false
		case html.TextToken:
			if inScript {
				hash := sha256.Sum256(z.Raw())
				value := " 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
				if !seen[value] {
					policy += value
					seen[value] = true
				}
			}
		}
	}
}

type loginWindow struct {
	start time.Time
	count int
}

// Both limits count admissions, including malformed requests, unknown accounts,
// and in-flight password checks. The global cap also bounds the IP map. RemoteAddr
// is supplied by net/http; untrusted X-Forwarded-For cannot evade the limiter.
type loginLimiter struct {
	mu      sync.Mutex
	global  loginWindow
	clients map[string]loginWindow
}

func (l *loginLimiter) allow(remote string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.global.start.IsZero() || now.Sub(l.global.start) >= time.Minute {
		l.global = loginWindow{start: now}
		l.clients = make(map[string]loginWindow)
	}
	if l.global.count >= 60 {
		return false
	}
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	w := l.clients[host]
	if w.count >= 10 {
		return false
	}
	w.count++
	l.clients[host] = w
	l.global.count++
	return true
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", cspBase)
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if len(r.URL.Path) >= 5 && r.URL.Path[:5] == "/api/" {
			w.Header().Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}
