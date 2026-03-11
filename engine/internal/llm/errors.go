package llm

import (
	"errors"
	"strings"
)

var (
	ErrUnauthorized    = errors.New("llm unauthorized")
	ErrPaymentRequired = errors.New("llm payment required")
	ErrUnavailable     = errors.New("llm unavailable")
	ErrEgressBlocked   = errors.New("egress blocked")
	ErrRateLimited     = errors.New("llm rate limited")
)

type APIError struct {
	Kind       error
	StatusCode int
	Detail     string
}

func (e *APIError) Error() string {
	if detail := strings.TrimSpace(e.Detail); detail != "" {
		return detail
	}
	if e.Kind != nil {
		return e.Kind.Error()
	}
	return "llm api error"
}

func (e *APIError) Unwrap() error {
	return e.Kind
}

func WrapAPIError(kind error, statusCode int, detail string) error {
	if kind == nil {
		kind = ErrUnavailable
	}
	if strings.TrimSpace(detail) == "" && statusCode == 0 {
		return kind
	}
	return &APIError{
		Kind:       kind,
		StatusCode: statusCode,
		Detail:     strings.TrimSpace(detail),
	}
}

func Detail(err error) string {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return strings.TrimSpace(apiErr.Detail)
	}
	return ""
}
