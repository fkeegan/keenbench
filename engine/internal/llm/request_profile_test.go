package llm

import (
	"context"
	"testing"
)

func TestRequestProfileFromContext(t *testing.T) {
	base := context.Background()
	ctx := WithRequestProfile(base, RequestProfile{
		ReasoningEffort: "xhigh",
	})

	profile, ok := RequestProfileFromContext(ctx)
	if !ok {
		t.Fatalf("expected request profile in context")
	}
	if profile.ReasoningEffort != "xhigh" {
		t.Fatalf("expected reasoning effort xhigh, got %q", profile.ReasoningEffort)
	}
}

func TestRequestProfileFromContextMissing(t *testing.T) {
	profile, ok := RequestProfileFromContext(context.Background())
	if ok {
		t.Fatalf("expected missing profile, got %+v", profile)
	}
}

func TestWithRequestProfileHandlesNilContext(t *testing.T) {
	ctx := WithRequestProfile(nil, RequestProfile{ReasoningEffort: "none"})
	profile, ok := RequestProfileFromContext(ctx)
	if !ok {
		t.Fatalf("expected request profile in context")
	}
	if profile.ReasoningEffort != "none" {
		t.Fatalf("expected reasoning effort none, got %q", profile.ReasoningEffort)
	}
}
