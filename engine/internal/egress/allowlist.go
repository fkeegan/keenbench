package egress

import (
	"net"
	"net/http"
	"strings"

	"keenbench/engine/internal/llm"
)

// AllowlistRoundTripper enforces HTTPS-only requests to a fixed host allowlist.
type AllowlistRoundTripper struct {
	Base      http.RoundTripper
	Allowlist map[string]bool
}

// NewAllowlistRoundTripper returns a RoundTripper that enforces a host allowlist.
func NewAllowlistRoundTripper(base http.RoundTripper, hosts []string) *AllowlistRoundTripper {
	allowlist := make(map[string]bool, len(hosts))
	for _, host := range hosts {
		allowlist[strings.ToLower(host)] = true
	}
	return &AllowlistRoundTripper{Base: base, Allowlist: allowlist}
}

func (rt *AllowlistRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL == nil {
		return nil, llm.ErrEgressBlocked
	}
	if req.URL.Scheme != "https" {
		return nil, llm.ErrEgressBlocked
	}
	host := req.URL.Hostname()
	if host == "" {
		return nil, llm.ErrEgressBlocked
	}
	if ip := net.ParseIP(host); ip != nil {
		return nil, llm.ErrEgressBlocked
	}
	if !rt.Allowlist[strings.ToLower(host)] {
		return nil, llm.ErrEgressBlocked
	}
	base := rt.Base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(req)
}
