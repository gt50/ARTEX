package server

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceSymlinksCannotExposeOrModifyPrivateFiles(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	private := filepath.Join(outside, "secret.txt")
	const sentinel = "PRIVATE_KEY_SENTINEL"
	if err := os.WriteFile(private, []byte(sentinel), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "jump")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(private, filepath.Join(workspace, "leaf")); err != nil {
		t.Fatal(err)
	}
	s := &Server{m: &Manager{dir: workspace}}
	for _, tc := range []struct {
		name, method, path, body string
		handler                  http.HandlerFunc
	}{
		{"read ancestor", "GET", "/?path=jump/secret.txt", "", s.wsRead},
		{"read leaf", "GET", "/?path=leaf", "", s.wsRead},
		{"download", "GET", "/?path=jump/secret.txt", "", s.wsDownload},
		{"list", "GET", "/?path=jump", "", s.wsList},
		{"write ancestor", "POST", "/", `{"path":"jump/secret.txt","content":"overwrite"}`, s.wsWrite},
		{"write leaf", "POST", "/", `{"path":"leaf","content":"overwrite"}`, s.wsWrite},
		{"mkdir", "POST", "/", `{"path":"jump/newdir"}`, s.wsMkdir},
		{"delete ancestor", "DELETE", "/?path=jump/secret.txt", "", s.wsDelete},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.handler(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
			if w.Code < 400 || strings.Contains(w.Body.String(), sentinel) {
				t.Fatalf("escaped workspace: status %d", w.Code)
			}
		})
	}
	for _, name := range []string{"leaf", "jump/secret.txt"} {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("file", filepath.Base(name))
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte("overwrite"))
		form.Close()
		path := "/?path="
		if strings.HasPrefix(name, "jump/") {
			path += "jump"
		}
		r := httptest.NewRequest("POST", path, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		s.wsUpload(w, r)
		if w.Code < 400 {
			t.Fatal("upload followed escaping symlink")
		}
	}
	data, err := os.ReadFile(private)
	if err != nil || string(data) != sentinel {
		t.Fatal("private file modified")
	}
	if _, err := os.Stat(filepath.Join(outside, "newdir")); !os.IsNotExist(err) {
		t.Fatal("directory created outside workspace")
	}
	w := httptest.NewRecorder()
	s.wsDelete(w, httptest.NewRequest("DELETE", "/?path=jump", nil))
	if w.Code != 200 {
		t.Fatalf("safe symlink removal: %d", w.Code)
	}
	if data, err := os.ReadFile(private); err != nil || string(data) != sentinel {
		t.Fatal("symlink deletion affected its target")
	}
}

func TestWorkspaceRootedOperationsPreserveNormalAccess(t *testing.T) {
	dir := t.TempDir()
	s := &Server{m: &Manager{dir: dir}}
	w := httptest.NewRecorder()
	s.wsWrite(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"path":"nested/file.txt","content":"visible"}`)))
	if w.Code != 200 {
		t.Fatalf("write: %d %s", w.Code, w.Body.String())
	}
	for _, handler := range []http.HandlerFunc{s.wsRead, s.wsDownload} {
		w := httptest.NewRecorder()
		handler(w, httptest.NewRequest("GET", "/?path=nested/file.txt", nil))
		if w.Code != 200 || !strings.Contains(w.Body.String(), "visible") {
			t.Fatalf("read/download: %d", w.Code)
		}
	}
	w = httptest.NewRecorder()
	s.wsDelete(w, httptest.NewRequest("DELETE", "/?path=nested", nil))
	if w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(dir, "nested")); !os.IsNotExist(err) {
		t.Fatal("directory was not removed")
	}
}

func TestChatUploadsStayInsideWorkspaceAndDoNotOverwrite(t *testing.T) {
	workspace := t.TempDir()
	s := &Server{m: &Manager{dir: workspace}}
	upload := func(id string) *httptest.ResponseRecorder {
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		file, err := form.CreateFormFile("file", "report.txt")
		if err != nil {
			t.Fatal(err)
		}
		file.Write([]byte("upload"))
		form.Close()
		r := httptest.NewRequest("POST", "/?scope=staging&id="+id, &body)
		r.Header.Set("Content-Type", form.FormDataContentType())
		w := httptest.NewRecorder()
		s.chatUpload(w, r)
		return w
	}
	for range 2 {
		if w := upload("normal"); w.Code != 200 {
			t.Fatalf("normal upload: %d %s", w.Code, w.Body.String())
		}
	}
	for _, name := range []string{"report.txt", "report-1.txt"} {
		if data, err := os.ReadFile(filepath.Join(workspace, "drafts", "normal", "uploads", name)); err != nil || string(data) != "upload" {
			t.Fatal("duplicate upload overwrote or lost content")
		}
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(workspace, "drafts", "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if w := upload("escape"); w.Code < 400 {
		t.Fatal("chat upload followed an external directory symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatal("chat upload modified an external directory")
	}
}
