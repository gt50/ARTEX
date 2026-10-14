package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var browserTrustMu sync.Mutex

const browserCANickname = "ARTEX recording proxy"

// The container opts in using its ephemeral service-user home. Native installs
// must opt in explicitly; never modify an operator's browser trust implicitly.
// Chromium M146+ uses ~/.local/share/pki/nssdb unless a legacy NSS DB exists:
// https://chromium.googlesource.com/chromium/src/+/main/docs/linux/cert_management.md
func syncChromiumCATrust(certPath string) error {
	if os.Getenv("ARTEX_CHROMIUM_CA_TRUST") != "1" {
		return nil
	}
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic Chromium CA trust is supported only on Linux")
	}
	browserTrustMu.Lock()
	defer browserTrustMu.Unlock()
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".local", "share", "pki", "nssdb")
	legacy := filepath.Join(home, ".pki", "nssdb")
	if info, err := os.Stat(legacy); err == nil && info.IsDir() {
		dir = legacy
	}
	_, statErr := os.Stat(filepath.Join(dir, "cert9.db"))
	if certPath == "" && os.IsNotExist(statErr) {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	run := func(args ...string) ([]byte, error) {
		out, err := exec.CommandContext(ctx, "/usr/bin/certutil", append([]string{"-d", "sql:" + dir}, args...)...).CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("Chromium certificate store: %w: %s", err, strings.TrimSpace(string(out)))
		}
		return out, nil
	}
	if os.IsNotExist(statErr) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if _, err := run("-N", "--empty-password"); err != nil {
			return err
		}
	} else if statErr != nil {
		return statErr
	}
	listing, err := run("-L")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(listing), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 1 && strings.Join(fields[:len(fields)-1], " ") == browserCANickname {
			if _, err := run("-D", "-n", browserCANickname); err != nil {
				return err
			}
			break
		}
	}
	if certPath != "" {
		// Import only the public certificate as an SSL CA. Other certificate
		// authorities remain intact, and TLS verification stays enabled.
		_, err = run("-A", "-n", browserCANickname, "-t", "C,,", "-i", certPath)
		return err
	}
	return nil
}
