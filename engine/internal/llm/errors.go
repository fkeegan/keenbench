package llm

import "errors"

var (
	ErrUnauthorized  = errors.New("llm unauthorized")
	ErrUnavailable   = errors.New("llm unavailable")
	ErrEgressBlocked = errors.New("egress blocked")
	ErrRateLimited   = errors.New("llm rate limited")
)
