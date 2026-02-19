package errinfo

// ErrorInfo follows ADR-0006 for structured error data.
type ErrorInfo struct {
	ErrorCode   string   `json:"error_code"`
	Phase       string   `json:"phase,omitempty"`
	Subphase    string   `json:"subphase,omitempty"`
	Retryable   bool     `json:"retryable"`
	Actions     []string `json:"actions,omitempty"`
	ProviderID  string   `json:"provider_id,omitempty"`
	ModelID     string   `json:"model_id,omitempty"`
	WorkbenchID string   `json:"workbench_id,omitempty"`
	ScopeHash   string   `json:"scope_hash,omitempty"`
	Detail      string   `json:"detail,omitempty"`
	DetailRef   string   `json:"detail_ref,omitempty"`
}

const (
	CodeEgressConsentRequired = "EGRESS_CONSENT_REQUIRED"
	CodeEgressBlocked         = "EGRESS_BLOCKED_BY_POLICY"
	CodeProviderNotConfigured = "PROVIDER_NOT_CONFIGURED"
	CodeProviderAuthFailed    = "PROVIDER_AUTH_FAILED"
	CodeProviderUnavailable   = "PROVIDER_UNAVAILABLE"
	CodeNetworkUnavailable    = "NETWORK_UNAVAILABLE"
	CodeSandboxViolation      = "SANDBOX_VIOLATION"
	CodeValidationFailed      = "VALIDATION_FAILED"
	CodeFileReadFailed        = "FILE_READ_FAILED"
	CodeFileWriteFailed       = "FILE_WRITE_FAILED"
	CodeDiskFull              = "DISK_FULL"
	CodeUserCanceled          = "USER_CANCELED"
	CodeConflictPublished     = "CONFLICT_PUBLISHED_CHANGED"
	CodeToolWorkerUnavailable = "TOOL_WORKER_UNAVAILABLE"
	CodeAgentLoopDetected     = "AGENT_LOOP_DETECTED"
	CodeStyleSkillLoadFailed  = "STYLE_SKILL_LOAD_FAILED"
	CodeStyleMergeFailed      = "STYLE_MERGE_FAILED"
)

const (
	ActionRetry        = "retry"
	ActionOpenSettings = "open_settings"
	ActionReviewDraft  = "review_draft"
	ActionDiscardDraft = "discard_draft"
)

const (
	PhaseWorkbench = "workbench"
	PhaseWorkshop  = "workshop"
	PhaseReview    = "review"
	PhasePublish   = "publish"
	PhaseSettings  = "settings"
)

const (
	SubphaseAddFiles     = "add_files"
	SubphaseStream       = "stream"
	SubphaseProposal     = "proposal"
	SubphaseApply        = "apply"
	SubphaseRPIResearch  = "rpi_research"
	SubphaseRPIPlan      = "rpi_plan"
	SubphaseRPIImplement = "rpi_implement"
	SubphaseRPISummary   = "rpi_summary"
)

func ProviderNotConfigured(phase string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeProviderNotConfigured,
		Phase:     phase,
		Retryable: false,
		Actions:   []string{ActionOpenSettings},
	}
}

func ProviderAuthFailed(phase string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeProviderAuthFailed,
		Phase:     phase,
		Retryable: false,
		Actions:   []string{ActionOpenSettings},
	}
}

func EgressConsentRequired(phase, providerID, scopeHash string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode:  CodeEgressConsentRequired,
		Phase:      phase,
		Retryable:  true,
		Actions:    []string{ActionRetry},
		ProviderID: providerID,
		ScopeHash:  scopeHash,
	}
}

func ValidationFailed(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeValidationFailed,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func SandboxViolation(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeSandboxViolation,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func FileReadFailed(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeFileReadFailed,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func FileWriteFailed(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeFileWriteFailed,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func ToolWorkerUnavailable(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeToolWorkerUnavailable,
		Phase:     phase,
		Retryable: true,
		Actions:   []string{ActionRetry},
		Detail:    detail,
	}
}

func EgressBlocked(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeEgressBlocked,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func AgentLoopDetected(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeAgentLoopDetected,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func ProviderUnavailable(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeProviderUnavailable,
		Phase:     phase,
		Retryable: true,
		Actions:   []string{ActionRetry},
		Detail:    detail,
	}
}

func UserCanceled(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeUserCanceled,
		Phase:     phase,
		Retryable: false,
		Detail:    detail,
	}
}

func NetworkUnavailable(phase, detail string) *ErrorInfo {
	return &ErrorInfo{
		ErrorCode: CodeNetworkUnavailable,
		Phase:     phase,
		Retryable: true,
		Actions:   []string{ActionRetry},
		Detail:    detail,
	}
}
