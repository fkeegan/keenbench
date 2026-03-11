package openrouter

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"keenbench/engine/internal/egress"
	"keenbench/engine/internal/llm"
)

type mockRT struct {
	roundTrip func(req *http.Request) (*http.Response, error)
}

func (m *mockRT) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTrip(req)
}

func response(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestAllowlistRoundTripper(t *testing.T) {
	called := false
	rt := egress.NewAllowlistRoundTripper(&mockRT{
		roundTrip: func(req *http.Request) (*http.Response, error) {
			called = true
			return response(http.StatusOK, "{}"), nil
		},
	}, []string{"openrouter.ai"})

	req, _ := http.NewRequest(http.MethodGet, "https://openrouter.ai/api/v1/models", nil)
	if _, err := rt.RoundTrip(req); err != nil {
		t.Fatalf("round trip failed: %v", err)
	}
	if !called {
		t.Fatalf("expected allowlisted request to reach base transport")
	}

	blockedReq, _ := http.NewRequest(http.MethodGet, "https://example.com/v1/models", nil)
	if _, err := rt.RoundTrip(blockedReq); err != llm.ErrEgressBlocked {
		t.Fatalf("expected egress blocked error, got %v", err)
	}
}

func TestValidateKey(t *testing.T) {
	client := &Client{
		baseURL: "https://openrouter.ai/api/v1",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path != "/api/v1/models" {
					t.Fatalf("expected /api/v1/models, got %s", req.URL.Path)
				}
				if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
					t.Fatalf("unexpected authorization header: %q", got)
				}
				if got := req.Header.Get("HTTP-Referer"); got != "https://keenbench.app" {
					t.Fatalf("unexpected HTTP-Referer header: %q", got)
				}
				if got := req.Header.Get("X-Title"); got != "KeenBench" {
					t.Fatalf("unexpected X-Title header: %q", got)
				}
				return response(http.StatusOK, `{"data":[]}`), nil
			},
		}},
	}
	if err := client.ValidateKey(context.Background(), "sk-test"); err != nil {
		t.Fatalf("validate key failed: %v", err)
	}
}

func TestValidateKeyUnauthorized(t *testing.T) {
	client := &Client{
		baseURL: "https://openrouter.ai/api/v1",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				return response(http.StatusUnauthorized, `{"error":"unauthorized"}`), nil
			},
		}},
	}
	if err := client.ValidateKey(context.Background(), "sk-test"); err != llm.ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestFetchModels(t *testing.T) {
	client := &Client{
		baseURL: "https://openrouter.ai/api/v1",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				body := `{"data":[{"id":"meta-llama/llama-3.3-70b-instruct","name":"Llama 3.3","context_length":128000,"supported_parameters":["tools"]}]}`
				return response(http.StatusOK, body), nil
			},
		}},
	}
	models, err := client.FetchModels(context.Background(), "sk-test")
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	m := models[0]
	if m.ID != "meta-llama/llama-3.3-70b-instruct" {
		t.Errorf("unexpected model ID: %q", m.ID)
	}
	if m.Name != "Llama 3.3" {
		t.Errorf("unexpected model name: %q", m.Name)
	}
	if m.ContextLength != 128000 {
		t.Errorf("unexpected context length: %d", m.ContextLength)
	}
	if len(m.SupportedParameters) != 1 || m.SupportedParameters[0] != "tools" {
		t.Errorf("unexpected supported_parameters: %v", m.SupportedParameters)
	}
}

func TestFetchModels_NoTools(t *testing.T) {
	client := &Client{
		baseURL: "https://openrouter.ai/api/v1",
		client: &http.Client{Transport: &mockRT{
			roundTrip: func(req *http.Request) (*http.Response, error) {
				body := `{"data":[{"id":"some/model","name":"Some Model","context_length":4096,"supported_parameters":[]}]}`
				return response(http.StatusOK, body), nil
			},
		}},
	}
	models, err := client.FetchModels(context.Background(), "sk-test")
	if err != nil {
		t.Fatalf("FetchModels failed: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	m := models[0]
	toolsSupported := false
	for _, p := range m.SupportedParameters {
		if p == "tools" {
			toolsSupported = true
		}
	}
	if toolsSupported {
		t.Errorf("expected model without tools support to not include tools in supported_parameters")
	}
}
