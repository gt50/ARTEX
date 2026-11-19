package proxy

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

	log "github.com/sirupsen/logrus"
)

func TestRequestErrorLogOmitsCredentialURL(t *testing.T) {
	var output bytes.Buffer
	logger := log.New()
	logger.SetOutput(&output)
	logErr(log.NewEntry(logger), &url.Error{Op: "Get", URL: "https://user:SECRET@target.test/SECRET?token=SECRET", Err: errors.New("TLS verification failed")})
	if strings.Contains(output.String(), "SECRET") || !strings.Contains(output.String(), "TLS verification failed") {
		t.Fatalf("unsafe error log: %s", output.String())
	}
}
