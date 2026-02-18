package engine

import (
	"context"
	"errors"
	"net"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
)

func mapLLMError(phase, providerID string, err error) *errinfo.ErrorInfo {
	if errors.Is(err, llm.ErrUnauthorized) {
		info := errinfo.ProviderAuthFailed(phase)
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrEgressBlocked) {
		info := errinfo.EgressBlocked(phase, "provider endpoint not allowed")
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrUnavailable) {
		info := errinfo.ProviderUnavailable(phase, err.Error())
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrRateLimited) {
		info := errinfo.ProviderUnavailable(phase, err.Error())
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		info := errinfo.NetworkUnavailable(phase, err.Error())
		info.ProviderID = providerID
		return info
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		info := errinfo.NetworkUnavailable(phase, err.Error())
		info.ProviderID = providerID
		return info
	}
	info := errinfo.ValidationFailed(phase, err.Error())
	info.ProviderID = providerID
	return info
}
