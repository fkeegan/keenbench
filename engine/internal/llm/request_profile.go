package llm

import "context"

// RequestProfile carries optional per-request model execution preferences.
type RequestProfile struct {
	ReasoningEffort string
}

type requestProfileContextKey struct{}

// WithRequestProfile stores a request profile on the provided context.
func WithRequestProfile(ctx context.Context, profile RequestProfile) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, requestProfileContextKey{}, profile)
}

// RequestProfileFromContext retrieves a request profile from context if present.
func RequestProfileFromContext(ctx context.Context) (RequestProfile, bool) {
	if ctx == nil {
		return RequestProfile{}, false
	}
	profile, ok := ctx.Value(requestProfileContextKey{}).(RequestProfile)
	if !ok {
		return RequestProfile{}, false
	}
	return profile, true
}
