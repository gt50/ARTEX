package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestChromiumTrustRequiresExplicitOptIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("ARTEX_CHROMIUM_CA_TRUST", "")
	if err := syncChromiumCATrust("missing-public-certificate.pem"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Fatal("native browser trust was modified without opt-in")
	}
}

func TestChromiumTrustNeverInvokesWindowsCertutil(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux platform guard")
	}
	t.Setenv("ARTEX_CHROMIUM_CA_TRUST", "1")
	if err := syncChromiumCATrust("missing-public-certificate.pem"); err == nil {
		t.Fatal("platform guard missing")
	}
}
