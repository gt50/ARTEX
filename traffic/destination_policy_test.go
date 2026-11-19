package traffic

import (
	"context"
	"net/netip"
	"testing"
)

func TestDestinationPolicyDefaultDenyAndPinning(t *testing.T) {
	lookups := 0
	lookup := func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("192.0.2.20")}, nil
	}
	p, err := loadDestinationPolicy(`["target.test:443","127.0.0.1:8080","[::1]:8443"]`, lookup)
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"target.test:443", "TARGET.TEST.:443"} {
		got, err := p.resolve(context.Background(), address)
		if err != nil || got != "192.0.2.20:443" {
			t.Fatalf("%q: %q %v", address, got, err)
		}
	}
	if lookups != 1 {
		t.Fatal("destination re-resolved after policy load")
	}
	for _, address := range []string{"target.test:444", "other.test:443", "192.0.2.20:443", "169.254.169.254:80", "127.0.0.1:8081", "[::1]:80"} {
		if _, err := p.resolve(context.Background(), address); err == nil {
			t.Fatalf("allowed %q", address)
		}
	}
	for _, raw := range []string{"", "[]"} {
		p, err := loadDestinationPolicy(raw, lookup)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.resolve(context.Background(), "127.0.0.1:80"); err == nil {
			t.Fatal("empty policy allowed destination")
		}
	}
}

func TestDestinationPolicyRejectsInvalidConfiguration(t *testing.T) {
	for _, raw := range []string{`["*.test:443"]`, `["https://target.test"]`, `["target.test:0"]`, `["target.test:65536"]`, `["0.0.0.0:80"]`, `["[::]:80"]`, `["224.0.0.1:80"]`, `["[fe80::1%eth0]:80"]`, `{}`, `[1]`} {
		_, err := loadDestinationPolicy(raw, func(context.Context, string) ([]netip.Addr, error) { return nil, nil })
		if err == nil {
			t.Fatalf("accepted invalid policy %s", raw)
		}
	}
}
