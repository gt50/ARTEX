package traffic

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func startPolicyProxy(t *testing.T, targets []string, targetCA []byte) (*Traffic, *http.Client) {
	t.Helper()
	rules, _ := json.Marshal(targets)
	t.Setenv("ARTEX_PROXY_ALLOWLIST", string(rules))
	t.Setenv("ARTEX_CA_DIR", t.TempDir())
	t.Setenv("ARTEX_PROXY_CA_FILE", "")
	if targetCA != nil {
		file := filepath.Join(t.TempDir(), "target.pem")
		if err := os.WriteFile(file, targetCA, 0600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ARTEX_PROXY_CA_FILE", file)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	tr, err := Open(filepath.Join(t.TempDir(), "traffic"), addr)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- tr.Start() }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = tr.proxy.Shutdown(ctx)
		<-done
		tr.Close()
	})
	for i := 0; i < 100; i++ {
		c, err := net.DialTimeout("tcp", addr, time.Millisecond*20)
		if err == nil {
			c.Close()
			break
		}
		if i == 99 {
			t.Fatal("proxy did not start")
		}
		time.Sleep(time.Millisecond * 10)
	}
	public, err := os.ReadFile(tr.CACertPath())
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AppendCertsFromPEM(public)
	proxyURL, _ := url.Parse(tr.ProxyAddr())
	transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), TLSClientConfig: &tls.Config{RootCAs: roots}}
	t.Cleanup(transport.CloseIdleConnections)
	return tr, &http.Client{Transport: transport, Timeout: time.Second * 5}
}

func TestProxyPolicyHTTPAndCONNECT(t *testing.T) {
	var hits atomic.Int32
	var deniedConnections atomic.Int32
	blocked := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("denied target received request") }))
	blocked.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			deniedConnections.Add(1)
		}
	}
	blocked.Start()
	defer blocked.Close()
	blockedURL, _ := url.Parse(blocked.URL)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Header.Get("Proxy-Authorization") != "" {
			t.Error("proxy credential forwarded")
		}
		w.Write([]byte("allowed"))
	}))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	tr, client := startPolicyProxy(t, []string{u.Host}, nil)
	r, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || string(b) != "allowed" {
		t.Fatal("approved HTTP failed")
	}
	for _, dest := range []string{"http://169.254.169.254/", blocked.URL} {
		r, err := client.Get(dest)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != 403 {
			t.Fatalf("unapproved HTTP status %d", r.StatusCode)
		}
	}
	c, err := net.Dial("tcp", tr.addr)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	req, _ := http.NewRequest("CONNECT", "http://"+blockedURL.Host, nil)
	req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("artex:"+tr.proxyToken)))
	if err := req.Write(c); err != nil {
		t.Fatal(err)
	}
	response, err := http.ReadResponse(bufio.NewReader(c), req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("unapproved CONNECT status %d", response.StatusCode)
	}
	if hits.Load() != 1 {
		t.Fatal("denied request reached target")
	}
	if deniedConnections.Load() != 0 {
		t.Fatal("denied destination was dialed")
	}
}

func TestProxyTLSValidationCannotDowngrade(t *testing.T) {
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("verified")) }))
	target.Config.ErrorLog = log.New(io.Discard, "", 0)
	target.StartTLS()
	defer target.Close()
	u, _ := url.Parse(target.URL)
	_, client := startPolicyProxy(t, []string{u.Host}, nil)
	for i := 0; i < 3; i++ {
		if r, err := client.Get(target.URL); err == nil {
			r.Body.Close()
			t.Fatal("untrusted upstream certificate accepted")
		}
	}
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.Certificate().Raw})
	_, trusted := startPolicyProxy(t, []string{u.Host}, ca)
	r, err := trusted.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || string(body) != "verified" {
		t.Fatal("explicitly trusted upstream failed")
	}
}

func TestProxyTLSCannotOverrideTargetIdentityWithSNI(t *testing.T) {
	target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Error("hostname-mismatched TLS reached HTTP handler") }))
	target.Config.ErrorLog = log.New(io.Discard, "", 0)
	target.StartTLS()
	defer target.Close()
	if !strings.Contains(strings.Join(target.Certificate().DNSNames, ","), "example.com") {
		t.Fatal("fixture must cover example.com")
	}
	u, _ := url.Parse(target.URL)
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: target.Certificate().Raw})
	tr, client := startPolicyProxy(t, []string{u.Host}, ca)
	_, port, _ := net.SplitHostPort(u.Host)
	alias := net.JoinHostPort("wrong.test", port)
	tr.policy.pinned[alias] = u.Host
	client.Transport.(*http.Transport).TLSClientConfig.ServerName = "example.com"
	if r, err := client.Get("https://" + alias); err == nil {
		r.Body.Close()
		t.Fatal("client SNI bypassed upstream hostname verification")
	}
}

func TestProxyScopePinsDestinationThroughUpstreamProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("scoped")) }))
	defer target.Close()
	u, _ := url.Parse(target.URL)
	var connects atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connects.Add(1)
		if r.Method != "CONNECT" || r.Host != u.Host {
			t.Errorf("upstream received unpinned destination %s %s", r.Method, r.Host)
			w.WriteHeader(403)
			return
		}
		remote, err := net.Dial("tcp", u.Host)
		if err != nil {
			t.Error(err)
			w.WriteHeader(502)
			return
		}
		client, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			remote.Close()
			t.Error(err)
			return
		}
		io.WriteString(client, "HTTP/1.1 200 Connection Established\r\n\r\n")
		go func() { defer client.Close(); defer remote.Close(); io.Copy(remote, client) }()
		go func() { defer client.Close(); defer remote.Close(); io.Copy(client, remote) }()
	}))
	defer upstream.Close()
	tr, client := startPolicyProxy(t, []string{u.Host}, nil)
	_, port, _ := net.SplitHostPort(u.Host)
	alias := net.JoinHostPort("target.test", port)
	tr.policy.pinned[alias] = u.Host
	if err := tr.SetUpstreamProxy(upstream.URL); err != nil {
		t.Fatal(err)
	}
	r, err := client.Get("http://" + alias)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	if r.StatusCode != 200 || string(body) != "scoped" {
		t.Fatal("scoped upstream forwarding failed")
	}
	r, err = client.Get("http://169.254.169.254/")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 403 || connects.Load() != 1 {
		t.Fatal("denied request reached upstream proxy")
	}
}
