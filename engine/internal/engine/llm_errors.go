package engine

import (
	"context"
	"errors"
	"net"

	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/llm"
)

func mapLLMError(phase, providerID string, err error) *errinfo.ErrorInfo {
	detail := llm.Detail(err)
	if detail == "" {
		detail = err.Error()
	}
	if errors.Is(err, llm.ErrUnauthorized) {
		info := errinfo.ProviderAuthFailed(phase)
		info.ProviderID = providerID
		if llm.Detail(err) != "" {
			info.Detail = detail
		}
		return info
	}
	if errors.Is(err, llm.ErrPaymentRequired) {
		info := errinfo.ProviderPaymentRequired(phase, detail)
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrEgressBlocked) {
		info := errinfo.EgressBlocked(phase, "provider endpoint not allowed")
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrUnavailable) {
		info := errinfo.ProviderUnavailable(phase, detail)
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, llm.ErrRateLimited) {
		info := errinfo.ProviderUnavailable(phase, detail)
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, context.Canceled) {
		info := errinfo.UserCanceled(phase, "run canceled")
		info.ProviderID = providerID
		return info
	}
	if errors.Is(err, context.DeadlineExceeded) {
		info := errinfo.NetworkUnavailable(phase, detail)
		info.ProviderID = providerID
		return info
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		info := errinfo.NetworkUnavailable(phase, detail)
		info.ProviderID = providerID
		return info
	}
	info := errinfo.ValidationFailed(phase, detail)
	info.ProviderID = providerID
	return info
}
