package openai

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

type stubRT struct {
	called bool
}

func (s *stubRT) RoundTrip(req *http.Request) (*http.Response, error) {
	s.called = true
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("{}")),
		Header:     make(http.Header),
	}, nil
}

func TestAllowlistRoundTripper(t *testing.T) {
	stub := &stubRT{}
	rt := egress.NewAllowlistRoundTripper(stub, []string{"api.openai.com"})
	req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
	_, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !stub.called {
		t.Fatalf("expected base to be called")
	}

	reqBad, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	_, err = rt.RoundTrip(reqBad)
	if err != llm.ErrEgressBlocked {
		t.Fatalf("expected egress blocked")
	}
}
