package rpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestServerHandlesRequest(t *testing.T) {
	input := "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"Ping\",\"api_version\":\"1\"}\n"
	reader := strings.NewReader(input)
	var output bytes.Buffer
	server := NewServer("1", reader, &output, nil)
	server.Register("Ping", func(ctx context.Context, params json.RawMessage) (any, *Error) {
		return map[string]any{"pong": true}, nil
	})

	if err := server.Serve(context.Background()); err != nil {
		t.Fatalf("serve: %v", err)
	}
	var respLine string
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		respLine = strings.TrimSpace(output.String())
		if respLine != "" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if respLine == "" {
		t.Fatalf("expected response")
	}
	var resp Response
	if err := json.Unmarshal([]byte(respLine), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	if result["pong"] != true {
		t.Fatalf("expected pong true")
	}
}
