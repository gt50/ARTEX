package traffic

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"
)

type destinationPolicy struct{ pinned map[string]string }

func canonicalDestination(address string) (string, string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, "%/\\@ \t\r\n") {
		return "", "", fmt.Errorf("destination must be an exact host:port")
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", "", fmt.Errorf("invalid destination port")
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		host = ip.Unmap().String()
	} else {
		if host == "" || len(host) > 253 {
			return "", "", fmt.Errorf("invalid destination hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", "", fmt.Errorf("invalid destination hostname")
			}
			for _, ch := range label {
				if ch != '-' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
					return "", "", fmt.Errorf("invalid destination hostname")
				}
			}
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), host, nil
}

func loadDestinationPolicy(raw string, lookup func(context.Context, string) ([]netip.Addr, error)) (*destinationPolicy, error) {
	var rules []string
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return nil, fmt.Errorf("ARTEX_PROXY_ALLOWLIST must be a JSON array of host:port strings: %w", err)
		}
	}
	if len(rules) > 256 {
		return nil, fmt.Errorf("proxy allowlist exceeds 256 destinations")
	}
	p := &destinationPolicy{pinned: make(map[string]string)}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, rule := range rules {
		canonical, host, err := canonicalDestination(rule)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy allowlist entry %q: %w", rule, err)
		}
		var ips []netip.Addr
		if ip, err := netip.ParseAddr(host); err == nil {
			ips = []netip.Addr{ip}
		} else {
			ips, err = lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve proxy allowlist host %q: %w", host, err)
			}
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("proxy allowlist host %q has no addresses", host)
		}
		for _, ip := range ips {
			if !ip.IsValid() || ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
				return nil, fmt.Errorf("invalid proxy destination address")
			}
		}
		_, port, _ := net.SplitHostPort(canonical)
		// Never re-resolve on dial: remote proxies also receive this exact IP.
		p.pinned[canonical] = net.JoinHostPort(ips[0].Unmap().String(), port)
	}
	return p, nil
}

func (p *destinationPolicy) resolve(_ context.Context, address string) (string, error) {
	canonical, _, err := canonicalDestination(address)
	if err != nil {
		return "", err
	}
	if p != nil {
		if pinned, ok := p.pinned[canonical]; ok {
			return pinned, nil
		}
	}
	return "", fmt.Errorf("proxy destination is outside the operator allowlist")
}
