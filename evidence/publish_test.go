package evidence

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishBody(t *testing.T) {
	dir := t.TempDir()
	source, destination := filepath.Join(dir, "staged"), filepath.Join(dir, "published")
	if err := os.WriteFile(source, []byte("complete evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := publishBody(source, destination); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatalf("staging file remains: %v", err)
	}
	if body, err := os.ReadFile(destination); err != nil || string(body) != "complete evidence" {
		t.Fatalf("published body=%q err=%v", body, err)
	}
	if err := publishBody(source, destination); err == nil {
		t.Fatal("publication with a missing source succeeded")
	}
	if body, err := os.ReadFile(destination); err != nil || string(body) != "complete evidence" {
		t.Fatalf("failed publication altered existing evidence: body=%q err=%v", body, err)
	}
}
