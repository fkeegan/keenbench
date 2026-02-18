package toolworker

import "errors"

const CodeToolWorkerUnavailable = "TOOL_WORKER_UNAVAILABLE"

var ErrUnavailable = errors.New("tool worker unavailable")

type RemoteError struct {
	Code    string
	Message string
}

func (e *RemoteError) Error() string {
	if e == nil {
		return ""
	}
	if e.Code == "" {
		return e.Message
	}
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}
