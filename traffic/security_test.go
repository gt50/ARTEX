package traffic

import (
	"encoding/base64"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCAStaysOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv("ARTEX_CA_DIR", filepath.Join(workspace, "private"))
	if tr, err := Open(filepath.Join(workspace, "traffic"), "127.0.0.1:0"); err == nil {
		tr.Close()
		t.Fatal("CA inside workspace allowed")
	}
	private := t.TempDir()
	t.Setenv("ARTEX_CA_DIR", private)
	tr, err := Open(filepath.Join(workspace, "traffic"), "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Close()
	if filepath.Dir(tr.CACertPath()) != private {
		t.Fatal("wrong CA location")
	}
	if _, err := os.Stat(filepath.Join(private, "mitmproxy-ca.pem")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "traffic", "_ca")); !os.IsNotExist(err) {
		t.Fatal("CA material created in workspace")
	}
}

func TestProxyAuthenticationBeforeForwarding(t *testing.T) {
	tr := &Traffic{proxyToken: strings.Repeat("a", 32), addr: "127.0.0.1:8788"}
	for _, method := range []string{"GET", "CONNECT"} {
		for _, value := range []string{"", "Basic invalid", "Basic " + base64.StdEncoding.EncodeToString([]byte("artex:wrong")), "Basic " + base64.StdEncoding.EncodeToString([]byte("artex:"+tr.proxyToken))} {
			r := httptest.NewRequest(method, "http://127.0.0.1:9999/", nil)
			r.Header.Set("Proxy-Authorization", value)
			ok, err := tr.authenticateProxy(httptest.NewRecorder(), r)
			want := strings.HasSuffix(value, base64.StdEncoding.EncodeToString([]byte("artex:"+tr.proxyToken)))
			if ok != want || (err == nil) != want {
				t.Fatalf("%s authentication: %v %v", method, ok, err)
			}
			if r.Header.Get("Proxy-Authorization") != "" {
				t.Fatal("credential not stripped")
			}
		}
	}
	u, err := url.Parse(tr.ProxyAddr())
	if err != nil {
		t.Fatal(err)
	}
	pw, _ := u.User.Password()
	if pw != tr.proxyToken || u.User.Username() != "artex" {
		t.Fatal("agent URL missing authentication")
	}
}

func TestCaptureHeadersRedactCredentials(t *testing.T) {
	head := headerLines(map[string][]string{"Authorization": {"Bearer secret"}, "cOoKiE": {"session=secret"}, "Set-Cookie": {"secret"}, "X-Api-Key": {"secret"}, "Content-Type": {"text/plain"}})
	if strings.Contains(head, "secret") {
		t.Fatal("credential persisted")
	}
	if !strings.Contains(head, "text/plain") || strings.Count(head, "[REDACTED]") != 4 {
		t.Fatal("unexpected redaction")
	}
}

func TestCaptureRedactsStructuredCredentials(t *testing.T) {
	u, _ := url.Parse("https://user:secret@example.test/api?access_token=secret&search=visible")
	clean := redactURL(u)
	if strings.Contains(clean.String(), "secret") || clean.Query().Get("search") != "visible" {
		t.Fatal("URL redaction failed")
	}
	if !strings.Contains(u.String(), "secret") {
		t.Fatal("live forwarding URL mutated")
	}
	for _, tc := range []struct{ body, media string }{
		{`{"token":"secret","items":[{"client_secret":"secret","name":"visible"}]}`, "application/json"},
		{"password=secret&name=visible", "application/x-www-form-urlencoded"},
	} {
		out := string(redactBody([]byte(tc.body), tc.media))
		if strings.Contains(out, `"secret"`) || strings.Contains(out, "=secret") || !strings.Contains(out, "visible") {
			t.Fatal("body redaction failed")
		}
	}
}

func TestCaptureRedactsProviderCredentialsButPreservesTokenMetrics(t *testing.T) {
	body := []byte(`{"env":{"OPENAI_API_KEY":"sensitive-value","AWS_SECRET_ACCESS_KEY":"sensitive-value"},"X-Session-Token":"sensitive-value","prompt_tokens":123,"max_tokens":456,"large_id":9007199254740993}`)
	clean := string(redactBody(body, "application/json"))
	if strings.Contains(clean, "sensitive-value") {
		t.Fatal("prefixed provider credential persisted")
	}
	if !strings.Contains(clean, `"prompt_tokens":123`) || !strings.Contains(clean, `"max_tokens":456`) {
		t.Fatal("token usage metrics were redacted")
	}
	if !strings.Contains(clean, `"large_id":9007199254740993`) {
		t.Fatal("redaction changed an unrelated numeric identifier")
	}
}

func TestStoredCredentialRedactionRebuildsSearchAndPages(t *testing.T) {
	tr, _ := openTraffic(t)
	tr.record(newFlow("example.test", "POST", "/login", []byte(`{"name":"visible"}`), []byte(`{"ok":true}`)))
	id := onlyExchangeID(t, tr)
	// Simulate an existing capture from before redaction was introduced.
	secret := "OLD_CAPTURE_SECRET_9137"
	_, err := tr.db.Exec(`UPDATE exchange_bodies SET req_head=?,req_body=?,resp_head=?,resp_body=? WHERE id=?`, "POST /login?token="+secret+" HTTP/1.1\nAuthorization: Bearer "+secret+"\nContent-Type: application/json\n", []byte(`{"password":"`+secret+`","name":"visible"}`), "HTTP 200\nSet-Cookie: token="+secret+"\nContent-Type: application/json\n", []byte(`{"token":"`+secret+`"}`), id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tr.db.Exec(`UPDATE exchanges SET url=? WHERE id=?`, "https://example.test/login?token="+secret, id); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.RedactStoredCredentials(); err != nil {
		t.Fatal(err)
	}
	req, resp, err := tr.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(req+resp, secret) || !strings.Contains(req, "visible") {
		t.Fatal("stored credentials not redacted correctly")
	}
	if tr.fts {
		hits, err := tr.query("example.test", "", secret, 0, 10)
		if err != nil || len(hits) != 0 {
			t.Fatal("credential remains searchable")
		}
	}
}
