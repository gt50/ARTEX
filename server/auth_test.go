package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The return value of GetSetting for "key does not exist" and "read error" is only one error: in both cases, the value is
// Empty string. Once the password-related handler treats the error as "the password has not been set", it will
// Open the initialization entrance - authInit will allow an unauthenticated request to overwrite the existing administrator password.
// authStatus will send the front end directly to /setup to do this.
//
// These two tests turn off the handler's connection pool to cause read failures, asserting that both are fail closed.
func TestAuthStatusFailsClosedWhenDataSourceUnavailable(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	// Turn off the pool and let subsequent GetSetting return error instead of sql.ErrNoRows.
	if err := m.pg.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{m: m}
	w := httptest.NewRecorder()
	s.authStatus(w, httptest.NewRequest("GET", "/api/auth/status", nil))

	if w.Code != 503 {
		t.Fatalf("status=%d want 503 (read failure is treated as uninitialized and the user will be sent to /setup to overwrite the password); body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Initialized *bool `json:"initialized"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err == nil && payload.Initialized != nil {
		t.Fatalf("Should not answer initialized when read fails, get %v", *payload.Initialized)
	}
}

func TestValidateUsername(t *testing.T) {
	for _, username := range []string{"", "has space", "has\ttab", "has\nnewline", "has\u00a0space", strings.Repeat("a", 65)} {
		if validateUsername(username) == "" {
			t.Errorf("accepted invalid username %q", username)
		}
	}
	for _, username := range []string{"ARTEX", "gt50", "test.user", "用户", strings.Repeat("a", 64)} {
		if err := validateUsername(username); err != "" {
			t.Errorf("rejected %q: %s", username, err)
		}
	}
}

// Use only an explicitly supplied disposable database; this test changes account credentials.
func TestConfigurableAccount(t *testing.T) {
	dsn := os.Getenv("ARTEX_AUTH_TEST_DSN")
	if dsn == "" {
		t.Skip("requires disposable ARTEX_AUTH_TEST_DSN")
	}
	t.Setenv("ARTEX_PG_DSN", dsn)
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if _, err := m.pg.Exec(`DELETE FROM settings WHERE key IN ('auth.username', 'auth.password_hash')`); err != nil {
		t.Fatal(err)
	}
	s := &Server{m: m, jwtKey: []byte("test-account-signing-key-32-bytes!")}
	call := func(handler http.HandlerFunc, body, token string, want int) string {
		t.Helper()
		r := httptest.NewRequest("POST", "/api/auth/test", strings.NewReader(body))
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
		var response struct {
			Token string `json:"token"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &response)
		return response.Token
	}
	call(s.authInit, `{"username":"bad name","password":"original-pass"}`, "", 400)
	token := call(s.authInit, `{"username":"gt50","password":"original-pass"}`, "", 200)
	if !verifyJWT(token, s.jwtKey) {
		t.Fatal("setup did not issue a valid token")
	}
	call(s.authInit, `{"username":"other","password":"other-password"}`, "", 403)
	call(s.authLogin, `{"username":"ARTEX","password":"original-pass"}`, "", 401)
	call(s.authLogin, `{"username":"gt50","password":"original-pass"}`, "", 200)
	call(s.authAccount, "", "", 401)
	call(s.authChangePassword, `{"username":"new-name","old_password":"original-pass"}`, "", 401)
	call(s.authChangePassword, `{"username":"new-name","old_password":"wrong"}`, token, 401)
	call(s.authChangePassword, `{"username":"","old_password":"original-pass"}`, token, 400)
	call(s.authChangePassword, `{"username":"用户","old_password":"original-pass","new_password":""}`, token, 200)
	call(s.authLogin, `{"username":"gt50","password":"original-pass"}`, "", 401)
	call(s.authLogin, `{"username":"用户","password":"original-pass"}`, "", 200)
	username, err := s.accountUsername()
	if err != nil || username != "用户" {
		t.Fatalf("saved username=%q err=%v", username, err)
	}
	call(s.authChangePassword, `{"username":"new-name","old_password":"original-pass","new_password":"updated-pass"}`, token, 200)
	call(s.authLogin, `{"username":"new-name","password":"original-pass"}`, "", 401)
	call(s.authLogin, `{"username":"new-name","password":"updated-pass"}`, "", 200)
	updated, err := m.pg.UpdateAccount("bad-name", "stale-hash", "bad-hash")
	if err != nil || updated {
		t.Fatalf("stale credential update: updated=%v err=%v", updated, err)
	}
	call(s.authLogin, `{"username":"new-name","password":"updated-pass"}`, "", 200)
	if _, err := m.pg.Exec(`DELETE FROM settings WHERE key='auth.username'`); err != nil {
		t.Fatal(err)
	}
	call(s.authLogin, `{"username":"ARTEX","password":"updated-pass"}`, "", 200)
}

func TestAuthInitFailsClosedWhenDataSourceUnavailable(t *testing.T) {
	m, err := NewManager(t.TempDir(), "")
	if err != nil {
		t.Skipf("postgres unavailable (%v)", err)
	}
	defer m.Close()
	if err := m.pg.Close(); err != nil {
		t.Fatal(err)
	}

	s := &Server{m: m}
	w := httptest.NewRecorder()
	body := strings.NewReader(`{"password":"correct horse battery"}`)
	s.authInit(w, httptest.NewRequest("POST", "/api/auth/init", body))

	if w.Code != 503 {
		t.Fatalf("status=%d want 503 (letting it pass when the read fails will cause unauthenticated requests to overwrite the existing password); body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "token") {
		t.Fatalf("Token should not be issued when read fails: %s", w.Body.String())
	}
}

func TestValidatePassword(t *testing.T) {
	for _, tc := range []struct {
		name, pw string
		wantErr  bool
	}{
		{"empty", "", true},
		{"seven characters", "1234567", true},
		{"eight characters", "12345678", false},
		{"eight multibyte characters are measured in bytes", "…—€™…—€™", false},
		{"three Unicode punctuation characters occupy nine bytes", "…—€", true},
		{"72 bytes", strings.Repeat("a", 72), false},
		{"73 bytes exceed bcrypt limit", strings.Repeat("a", 73), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validatePassword(tc.pw); (got != "") != tc.wantErr {
				t.Fatalf("validatePassword(%q)=%q, wantErr=%v", tc.pw, got, tc.wantErr)
			}
		})
	}
}
