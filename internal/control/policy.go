package control

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

var ErrTargetNotAllowed = errors.New("target is not allowed")

type TargetPolicy struct {
	hosts    map[string]struct{}
	prefixes []netip.Prefix
}

func ParseTargetPolicy(spec string) (*TargetPolicy, error) {
	policy := &TargetPolicy{hosts: make(map[string]struct{})}
	for _, raw := range strings.Split(spec, ",") {
		token := strings.TrimSpace(raw)
		if token == "" {
			continue
		}
		if prefix, err := netip.ParsePrefix(token); err == nil {
			policy.prefixes = append(policy.prefixes, prefix.Masked())
			continue
		}
		if addr, err := netip.ParseAddr(token); err == nil {
			bits := 32
			if addr.Is6() {
				bits = 128
			}
			policy.prefixes = append(policy.prefixes, netip.PrefixFrom(addr, bits))
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(token, "."))
		if strings.ContainsAny(host, "/:@") {
			return nil, fmt.Errorf("invalid target allowlist entry %q", token)
		}
		policy.hosts[host] = struct{}{}
	}
	return policy, nil
}

func (p *TargetPolicy) Authorize(rawTarget string) error {
	if p == nil {
		return ErrTargetNotAllowed
	}
	parsed, err := url.Parse(rawTarget)
	if err != nil || parsed.Hostname() == "" {
		return fmt.Errorf("%w: invalid URL", ErrTargetNotAllowed)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrTargetNotAllowed)
	}
	if parsed.User != nil {
		return fmt.Errorf("%w: URL credentials are not supported", ErrTargetNotAllowed)
	}

	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if addr, err := netip.ParseAddr(host); err == nil {
		for _, prefix := range p.prefixes {
			if prefix.Contains(addr) {
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrTargetNotAllowed, host)
	}
	if _, ok := p.hosts[host]; ok {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrTargetNotAllowed, host)
}
