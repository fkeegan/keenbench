package errinfo

import "testing"

func TestProviderNotConfigured(t *testing.T) {
	err := ProviderNotConfigured(PhaseSettings)
	if err.ErrorCode != CodeProviderNotConfigured {
		t.Fatalf("expected provider not configured")
	}
	if len(err.Actions) == 0 || err.Actions[0] != ActionOpenSettings {
		t.Fatalf("expected open_settings action")
	}
}

func TestEgressConsentRequired(t *testing.T) {
	err := EgressConsentRequired(PhaseWorkshop, "openai", "scope123")
	if err.ErrorCode != CodeEgressConsentRequired {
		t.Fatalf("expected consent required")
	}
	if err.ProviderID != "openai" || err.ScopeHash != "scope123" {
		t.Fatalf("expected provider/scope to be set")
	}
}

func TestValidationHelpers(t *testing.T) {
	auth := ProviderAuthFailed(PhaseSettings)
	if auth.ErrorCode != CodeProviderAuthFailed {
		t.Fatalf("expected provider auth failed")
	}
	validation := ValidationFailed(PhaseWorkshop, "bad")
	if validation.ErrorCode != CodeValidationFailed {
		t.Fatalf("expected validation failed")
	}
	sandbox := SandboxViolation(PhaseWorkbench, "escape")
	if sandbox.ErrorCode != CodeSandboxViolation {
		t.Fatalf("expected sandbox violation")
	}
	empty := AgentEmptyTerminalResponse(PhaseWorkshop, "empty")
	if empty.ErrorCode != CodeAgentEmptyTerminal {
		t.Fatalf("expected empty terminal error code")
	}
	if !empty.Retryable {
		t.Fatalf("expected empty terminal error to be retryable")
	}
	if len(empty.Actions) == 0 || empty.Actions[0] != ActionRetry {
		t.Fatalf("expected retry action on empty terminal error")
	}
}
