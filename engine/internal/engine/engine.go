package engine

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"keenbench/engine/internal/anthropic"
	"keenbench/engine/internal/appdirs"
	"keenbench/engine/internal/diff"
	"keenbench/engine/internal/envutil"
	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/gemini"
	"keenbench/engine/internal/llm"
	"keenbench/engine/internal/logging"
	"keenbench/engine/internal/mistral"
	"keenbench/engine/internal/openai"
	"keenbench/engine/internal/secrets"
	"keenbench/engine/internal/settings"
	"keenbench/engine/internal/toolworker"
	"keenbench/engine/internal/workbench"
)

const (
	EngineVersion = "0.1.0"
	APIVersion    = "1"
)

const (
	maxProposalContentBytes = 1024 * 1024
	maxProposalWrites       = 10
	maxProposalOpsPerFile   = 100
	maxProposalOpsTotal     = 500
)

const (
	workbenchMaxFiles     = 10
	workbenchMaxFileBytes = 25 * 1024 * 1024
)

const (
	oauthFlowStatusPending      = "pending"
	oauthFlowStatusCodeReceived = "code_received"
	oauthFlowStatusCompleted    = "completed"
	oauthFlowStatusFailed       = "failed"
	oauthFlowStatusExpired      = "expired"
	defaultOAuthFlowTTL         = 10 * time.Minute
)

const (
	rateLimitRetryMaxAttempts = 5
	rateLimitRetryBaseDelay   = 10 * time.Second
	rateLimitRetryMaxDelay    = 4 * time.Minute
)

var workbenchEditableExtensions = []string{
	".txt", ".csv", ".md", ".json", ".xml", ".yaml", ".yml",
	".html", ".js", ".ts", ".py", ".java", ".go", ".rb", ".rs",
	".c", ".cpp", ".h", ".css", ".sql",
}

type Notifier func(method string, params any)

type LLMClient interface {
	ValidateKey(ctx context.Context, apiKey string) error
	Chat(ctx context.Context, apiKey, model string, messages []llm.Message) (string, error)
	StreamChat(ctx context.Context, apiKey, model string, messages []llm.Message, onDelta func(string)) (string, error)
	ChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool) (llm.ChatResponse, error)
	StreamChatWithTools(ctx context.Context, apiKey, model string, messages []llm.ChatMessage, tools []llm.Tool, onDelta func(string)) (llm.ChatResponse, error)
}

type codexOAuthClient interface {
	BuildAuthorizeURL(state, codeChallenge string) (string, error)
	ParseRedirectURL(redirectURL string) (string, string, error)
	ExchangeAuthorizationCode(ctx context.Context, code, codeVerifier string) (openai.CodexOAuthToken, error)
	RefreshAccessToken(ctx context.Context, refreshToken string) (openai.CodexOAuthToken, error)
	ExtractChatGPTAccountID(idToken string) string
}

type providerOAuthFlow struct {
	FlowID       string
	ProviderID   string
	State        string
	Verifier     string
	AuthorizeURL string
	Code         string
	Status       string
	LastError    string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type workshopRunHandle struct {
	runID  string
	cancel context.CancelFunc
}

type Engine struct {
	dataDir             string
	settings            *settings.Store
	secrets             *secrets.Store
	workbenches         *workbench.Manager
	providers           map[string]LLMClient
	toolWorker          toolworker.Client
	notify              Notifier
	logger              *slog.Logger
	sessionConsent      map[string]workbench.WorkshopConsent
	clutterMu           sync.Mutex
	pendingClutter      map[string]int
	clutterEmitAt       map[string]time.Time
	codexOAuth          codexOAuthClient
	oauthFlowTTL        time.Duration
	now                 func() time.Time
	newOAuthFlowID      func() (string, error)
	newCodexPKCE        func() (openai.CodexPKCEValues, error)
	oauthMu             sync.Mutex
	oauthFlows          map[string]*providerOAuthFlow
	oauthFlowByState    map[string]string
	oauthCallbackServer *http.Server
	runMu               sync.Mutex
	workshopRuns        map[string]workshopRunHandle
	sleep               func(context.Context, time.Duration) error
}

type Option func(*Engine)

func WithLogger(logger *slog.Logger) Option {
	return func(e *Engine) {
		if logger != nil {
			e.logger = logger
		}
	}
}

func WithToolWorker(worker toolworker.Client) Option {
	return func(e *Engine) {
		if worker != nil {
			e.toolWorker = worker
		}
	}
}

func New(opts ...Option) (*Engine, error) {
	engine := &Engine{logger: logging.Nop()}
	for _, opt := range opts {
		opt(engine)
	}
	dataDir, err := appdirs.DataDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	workbenchesDir := appdirs.WorkbenchesDir(dataDir)
	settingsPath := filepath.Join(dataDir, "settings.json")
	secretsPath := filepath.Join(dataDir, "secrets.enc")
	masterKeyPath := filepath.Join(dataDir, "master.key")
	mgr := workbench.NewManager(workbenchesDir)
	if err := mgr.Init(); err != nil {
		return nil, err
	}
	providers := map[string]LLMClient{
		ProviderOpenAI:      openai.NewClient(),
		ProviderOpenAICodex: openai.NewCodexClient(),
		ProviderAnthropic:   anthropic.NewClient(),
		ProviderGoogle:      gemini.NewClient(),
		ProviderMistral:     mistral.NewClient(),
	}
	if envutil.Bool("KEENBENCH_FAKE_OPENAI") {
		fake := newFakeOpenAI()
		for id := range providers {
			providers[id] = fake
		}
	}
	if engine.toolWorker == nil {
		if envutil.Bool("KEENBENCH_FAKE_TOOL_WORKER") {
			engine.toolWorker = toolworker.NewFake(workbenchesDir)
		} else {
			worker := toolworker.New(workbenchesDir, engine.logger.With("component", "toolworker"))
			if err := worker.Start(); err != nil {
				return nil, fmt.Errorf("tool worker failed to start: %w (run 'make package-worker' to set up dependencies)", err)
			}
			// Verify the worker is actually responsive
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := worker.HealthCheck(ctx); err != nil {
				worker.Close()
				return nil, fmt.Errorf("tool worker health check failed: %w (run 'make package-worker' to set up dependencies)", err)
			}
			engine.toolWorker = worker
		}
	}
	engine.dataDir = dataDir
	engine.settings = settings.NewStore(settingsPath)
	engine.secrets = secrets.NewStore(secretsPath, masterKeyPath)
	engine.workbenches = mgr
	engine.providers = providers
	engine.sessionConsent = make(map[string]workbench.WorkshopConsent)
	engine.pendingClutter = make(map[string]int)
	engine.clutterEmitAt = make(map[string]time.Time)
	engine.codexOAuth = openai.NewCodexOAuth()
	engine.oauthFlowTTL = defaultOAuthFlowTTL
	engine.now = time.Now
	engine.newOAuthFlowID = generateOAuthFlowID
	engine.newCodexPKCE = openai.GenerateCodexPKCE
	engine.oauthFlows = make(map[string]*providerOAuthFlow)
	engine.oauthFlowByState = make(map[string]string)
	engine.workshopRuns = make(map[string]workshopRunHandle)
	engine.sleep = sleepWithContext
	engine.logger.Debug("engine.init", "data_dir", dataDir, "workbenches_dir", workbenchesDir, "fake_openai", envutil.Bool("KEENBENCH_FAKE_OPENAI"))
	return engine, nil
}

func (e *Engine) SetNotifier(notify Notifier) {
	e.notify = notify
}

func (e *Engine) EngineGetInfo(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	return map[string]any{
		"engine_version": EngineVersion,
		"api_version":    APIVersion,
	}, nil
}

func (e *Engine) ToolWorkerGetStatus(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	if e.toolWorker == nil {
		return map[string]any{
			"available": false,
			"error":     "tool worker not initialized",
		}, nil
	}
	// Try a health check
	healthErr := e.toolWorker.HealthCheck(ctx)
	if healthErr != nil {
		e.logger.Warn("toolworker.health_check_failed", "error", healthErr.Error())
		return map[string]any{
			"available": false,
			"error":     healthErr.Error(),
		}, nil
	}
	return map[string]any{
		"available": true,
	}, nil
}

func (e *Engine) ProvidersGetStatus(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	settingsData, err := e.settings.Load()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	status := []map[string]any{}
	providers := []struct {
		id       string
		name     string
		authMode string
	}{
		{ProviderOpenAI, "OpenAI", "api_key"},
		{ProviderOpenAICodex, "OpenAI Codex", "oauth"},
		{ProviderAnthropic, "Anthropic", "api_key"},
		{ProviderGoogle, "Google", "api_key"},
		{ProviderMistral, "Mistral", "api_key"},
	}
	for _, provider := range providers {
		entry := settingsData.Providers[provider.id]
		item := map[string]any{
			"provider_id":  provider.id,
			"display_name": provider.name,
			"enabled":      entry.Enabled,
			"models":       modelsForProvider(provider.id),
			"auth_mode":    provider.authMode,
		}
		if supportsRPIReasoningEffortProvider(provider.id) {
			item["rpi_reasoning"] = map[string]any{
				"research_effort":  normalizeProviderRPIReasoningEffort(provider.id, entry.RPIResearchReasoningEffort),
				"plan_effort":      normalizeProviderRPIReasoningEffort(provider.id, entry.RPIPlanReasoningEffort),
				"implement_effort": normalizeProviderRPIReasoningEffort(provider.id, entry.RPIImplementReasoningEffort),
			}
		}
		if provider.authMode == "oauth" {
			creds, err := e.secrets.GetOpenAICodexOAuthCredentials()
			if err != nil {
				return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
			}
			connected := creds != nil && strings.TrimSpace(creds.RefreshToken) != ""
			item["configured"] = connected
			item["oauth_connected"] = connected
			if connected {
				expired := false
				if !creds.ExpiresAt.IsZero() {
					item["oauth_expires_at"] = creds.ExpiresAt.UTC().Format(time.RFC3339)
					now := time.Now()
					if e.now != nil {
						now = e.now()
					}
					expired = now.After(creds.ExpiresAt)
				}
				item["oauth_expired"] = expired
				if label := strings.TrimSpace(creds.AccountLabel); label != "" {
					item["oauth_account_label"] = label
				}
			}
		} else {
			key, errInfo := e.providerKey(ctx, provider.id)
			if errInfo != nil {
				return nil, errInfo
			}
			item["configured"] = strings.TrimSpace(key) != ""
		}
		status = append(status, item)
	}
	return map[string]any{"providers": status}, nil
}

func (e *Engine) ProvidersSetApiKey(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
		APIKey     string `json:"api_key"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	e.logger.Debug("providers.set_api_key", "provider_id", req.ProviderID, "api_key", logging.RedactValue(req.APIKey))
	if errInfo := e.setProviderKey(req.ProviderID, req.APIKey); errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{}, nil
}

func (e *Engine) ProvidersClearApiKey(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if errInfo := e.clearProviderKey(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{}, nil
}

func (e *Engine) ProvidersValidate(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	e.logger.Debug("providers.validate", "provider_id", req.ProviderID, "fake_openai", envutil.Bool("KEENBENCH_FAKE_OPENAI"))
	if errInfo := e.validateProviderKey(ctx, req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{"ok": true}, nil
}

func (e *Engine) ProvidersSetEnabled(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
		Enabled    bool   `json:"enabled"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if _, errInfo := e.clientForProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	e.logger.Info("providers.set_enabled", "provider_id", req.ProviderID, "enabled", req.Enabled)
	_, err := e.settings.Update(func(s *settings.Settings) {
		entry := s.Providers[req.ProviderID]
		entry.Enabled = req.Enabled
		s.Providers[req.ProviderID] = entry
	})
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
	}
	return map[string]any{}, nil
}

func (e *Engine) ProvidersSetReasoningEffort(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID      string `json:"provider_id"`
		ResearchEffort  string `json:"research_effort"`
		PlanEffort      string `json:"plan_effort"`
		ImplementEffort string `json:"implement_effort"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}

	providerID := strings.TrimSpace(req.ProviderID)
	if !supportsRPIReasoningEffortProvider(providerID) {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported provider"), providerID)
	}

	researchEffort, ok := validateProviderRPIReasoningEffort(providerID, req.ResearchEffort)
	if !ok {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid research_effort"), providerID)
	}
	planEffort, ok := validateProviderRPIReasoningEffort(providerID, req.PlanEffort)
	if !ok {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid plan_effort"), providerID)
	}
	implementEffort, ok := validateProviderRPIReasoningEffort(providerID, req.ImplementEffort)
	if !ok {
		return nil, withProviderID(errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid implement_effort"), providerID)
	}

	_, err := e.settings.Update(func(s *settings.Settings) {
		entry := s.Providers[providerID]
		entry.RPIResearchReasoningEffort = researchEffort
		entry.RPIPlanReasoningEffort = planEffort
		entry.RPIImplementReasoningEffort = implementEffort
		s.Providers[providerID] = entry
	})
	if err != nil {
		return nil, withProviderID(errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error()), providerID)
	}

	return map[string]any{}, nil
}

func (e *Engine) ProvidersOAuthStart(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if errInfo := validateOpenAICodexProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	if e.codexOAuth == nil || e.newOAuthFlowID == nil || e.newCodexPKCE == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth support unavailable")
	}
	pkce, err := e.newCodexPKCE()
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, err.Error())
	}
	flowID, err := e.newOAuthFlowID()
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, err.Error())
	}
	authorizeURL, err := e.codexOAuth.BuildAuthorizeURL(pkce.State, pkce.CodeChallenge)
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, err.Error())
	}
	now := time.Now().UTC()
	if e.now != nil {
		now = e.now().UTC()
	}
	flow := &providerOAuthFlow{
		FlowID:       flowID,
		ProviderID:   req.ProviderID,
		State:        pkce.State,
		Verifier:     pkce.CodeVerifier,
		AuthorizeURL: authorizeURL,
		Status:       oauthFlowStatusPending,
		CreatedAt:    now,
		ExpiresAt:    now.Add(e.oauthFlowTTL),
	}

	e.oauthMu.Lock()
	e.cleanupOAuthFlowsLocked(now)
	e.oauthFlows[flowID] = flow
	e.oauthFlowByState[flow.State] = flowID
	e.oauthMu.Unlock()

	callbackListening := e.ensureOAuthCallbackServer()
	return map[string]any{
		"provider_id":        req.ProviderID,
		"flow_id":            flowID,
		"authorize_url":      authorizeURL,
		"status":             flow.Status,
		"expires_at":         flow.ExpiresAt.Format(time.RFC3339),
		"callback_listening": callbackListening,
	}, nil
}

func (e *Engine) ProvidersOAuthStatus(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
		FlowID     string `json:"flow_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if errInfo := validateOpenAICodexProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	flow, errInfo := e.getOAuthFlow(req.ProviderID, req.FlowID)
	if errInfo != nil {
		return nil, errInfo
	}
	resp := map[string]any{
		"provider_id":   flow.ProviderID,
		"flow_id":       flow.FlowID,
		"status":        flow.Status,
		"expires_at":    flow.ExpiresAt.Format(time.RFC3339),
		"authorize_url": flow.AuthorizeURL,
	}
	if strings.TrimSpace(flow.Code) != "" {
		resp["code_captured"] = true
	}
	if strings.TrimSpace(flow.LastError) != "" {
		resp["error"] = flow.LastError
	}
	return resp, nil
}

func (e *Engine) ProvidersOAuthComplete(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID  string `json:"provider_id"`
		FlowID      string `json:"flow_id"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if errInfo := validateOpenAICodexProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	if e.codexOAuth == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth support unavailable")
	}
	flow, errInfo := e.getOAuthFlow(req.ProviderID, req.FlowID)
	if errInfo != nil {
		return nil, errInfo
	}

	code := strings.TrimSpace(flow.Code)
	if strings.TrimSpace(req.RedirectURL) != "" {
		parsedCode, state, err := e.codexOAuth.ParseRedirectURL(req.RedirectURL)
		if err != nil {
			return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, err.Error())
		}
		if state != flow.State {
			return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth state mismatch")
		}
		code = parsedCode
		e.oauthMu.Lock()
		if mutableFlow, ok := e.oauthFlows[flow.FlowID]; ok {
			mutableFlow.Code = code
			mutableFlow.Status = oauthFlowStatusCodeReceived
			flow = *mutableFlow
		}
		e.oauthMu.Unlock()
	}
	if strings.TrimSpace(code) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "authorization code not available")
	}

	token, err := e.codexOAuth.ExchangeAuthorizationCode(ctx, code, flow.Verifier)
	if err != nil {
		e.oauthMu.Lock()
		if mutableFlow, ok := e.oauthFlows[flow.FlowID]; ok {
			mutableFlow.Status = oauthFlowStatusFailed
			mutableFlow.LastError = err.Error()
		}
		e.oauthMu.Unlock()
		return nil, mapOAuthError(err, req.ProviderID)
	}
	if strings.TrimSpace(token.RefreshToken) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth token response missing refresh_token")
	}
	creds := &secrets.OpenAICodexOAuthCredentials{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		IDToken:      token.IDToken,
		ExpiresAt:    token.ExpiresAt.UTC(),
	}
	label := strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(token.IDToken))
	if label == "" {
		label = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(token.AccessToken))
	}
	if label != "" {
		creds.AccountLabel = label
	}
	if err := e.secrets.SetOpenAICodexOAuthCredentials(creds); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseSettings, err.Error())
	}
	e.oauthMu.Lock()
	if mutableFlow, ok := e.oauthFlows[flow.FlowID]; ok {
		mutableFlow.Status = oauthFlowStatusCompleted
		mutableFlow.LastError = ""
	}
	e.oauthMu.Unlock()
	return map[string]any{
		"provider_id":         req.ProviderID,
		"oauth_connected":     true,
		"oauth_account_label": creds.AccountLabel,
		"oauth_expires_at":    creds.ExpiresAt.Format(time.RFC3339),
	}, nil
}

func (e *Engine) ProvidersOAuthDisconnect(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid params")
	}
	if errInfo := validateOpenAICodexProvider(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	if errInfo := e.clearProviderKey(req.ProviderID); errInfo != nil {
		return nil, errInfo
	}
	return map[string]any{}, nil
}

func validateOpenAICodexProvider(providerID string) *errinfo.ErrorInfo {
	if strings.TrimSpace(providerID) != ProviderOpenAICodex {
		return errinfo.ValidationFailed(errinfo.PhaseSettings, "unsupported provider")
	}
	return nil
}

type providerRPIReasoningEffort struct {
	ResearchEffort  string
	PlanEffort      string
	ImplementEffort string
}

const (
	reasoningEffortNone   = "none"
	reasoningEffortLow    = "low"
	reasoningEffortMedium = "medium"
	reasoningEffortHigh   = "high"
	reasoningEffortXHigh  = "xhigh"
)

func supportsRPIReasoningEffortProvider(providerID string) bool {
	switch strings.TrimSpace(providerID) {
	case ProviderOpenAI, ProviderOpenAICodex:
		return true
	default:
		return false
	}
}

func normalizeRPIReasoningEffortLiteral(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validateProviderRPIReasoningEffort(providerID, effort string) (string, bool) {
	effort = normalizeRPIReasoningEffortLiteral(effort)
	switch strings.TrimSpace(providerID) {
	case ProviderOpenAI:
		switch effort {
		case reasoningEffortNone, reasoningEffortLow, reasoningEffortMedium, reasoningEffortHigh:
			return effort, true
		}
	case ProviderOpenAICodex:
		switch effort {
		case reasoningEffortLow, reasoningEffortMedium, reasoningEffortHigh, reasoningEffortXHigh:
			return effort, true
		}
	}
	return "", false
}

func normalizeProviderRPIReasoningEffort(providerID, effort string) string {
	if normalized, ok := validateProviderRPIReasoningEffort(providerID, effort); ok {
		return normalized
	}
	return reasoningEffortMedium
}

func (e *Engine) loadProviderRPIReasoningEffort(providerID string) (providerRPIReasoningEffort, *errinfo.ErrorInfo) {
	if !supportsRPIReasoningEffortProvider(providerID) {
		return providerRPIReasoningEffort{}, nil
	}
	settingsData, err := e.settings.Load()
	if err != nil {
		return providerRPIReasoningEffort{}, withProviderID(errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error()), providerID)
	}
	entry := settingsData.Providers[providerID]
	return providerRPIReasoningEffort{
		ResearchEffort:  normalizeProviderRPIReasoningEffort(providerID, entry.RPIResearchReasoningEffort),
		PlanEffort:      normalizeProviderRPIReasoningEffort(providerID, entry.RPIPlanReasoningEffort),
		ImplementEffort: normalizeProviderRPIReasoningEffort(providerID, entry.RPIImplementReasoningEffort),
	}, nil
}

func withRPIReasoningEffortProfile(ctx context.Context, providerID, effort string) context.Context {
	if !supportsRPIReasoningEffortProvider(providerID) {
		return ctx
	}
	normalizedEffort := normalizeProviderRPIReasoningEffort(providerID, effort)
	return llm.WithRequestProfile(ctx, llm.RequestProfile{ReasoningEffort: normalizedEffort})
}

func (e *Engine) getOAuthFlow(providerID, flowID string) (providerOAuthFlow, *errinfo.ErrorInfo) {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return providerOAuthFlow{}, errinfo.ValidationFailed(errinfo.PhaseSettings, "invalid flow id")
	}
	now := time.Now().UTC()
	if e.now != nil {
		now = e.now().UTC()
	}
	e.oauthMu.Lock()
	defer e.oauthMu.Unlock()
	e.cleanupOAuthFlowsLocked(now)
	flow, ok := e.oauthFlows[flowID]
	if !ok || flow.ProviderID != providerID {
		return providerOAuthFlow{}, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth flow not found")
	}
	if flow.Status == oauthFlowStatusExpired {
		return providerOAuthFlow{}, errinfo.ValidationFailed(errinfo.PhaseSettings, "oauth flow expired")
	}
	return *flow, nil
}

func (e *Engine) cleanupOAuthFlowsLocked(now time.Time) {
	retainAfter := 60 * time.Minute
	for flowID, flow := range e.oauthFlows {
		if now.After(flow.ExpiresAt) && flow.Status != oauthFlowStatusCompleted {
			flow.Status = oauthFlowStatusExpired
		}
		if now.After(flow.ExpiresAt.Add(retainAfter)) {
			delete(e.oauthFlows, flowID)
			delete(e.oauthFlowByState, flow.State)
		}
	}
}

func (e *Engine) ensureOAuthCallbackServer() bool {
	e.oauthMu.Lock()
	if e.oauthCallbackServer != nil {
		e.oauthMu.Unlock()
		return true
	}
	listenAddr, callbackPath, err := codexOAuthCallbackEndpoint()
	if err != nil {
		e.oauthMu.Unlock()
		e.logger.Warn("providers.oauth.callback_config_invalid", "error", err.Error())
		return false
	}
	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, e.handleOAuthCallback)
	server := &http.Server{
		Addr:              listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		e.oauthMu.Unlock()
		e.logger.Warn("providers.oauth.callback_listen_failed", "addr", listenAddr, "error", err.Error())
		return false
	}
	e.oauthCallbackServer = server
	e.oauthMu.Unlock()
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			e.logger.Warn("providers.oauth.callback_server_failed", "error", serveErr.Error())
		}
	}()
	return true
}

func codexOAuthCallbackEndpoint() (string, string, error) {
	parsed, err := url.Parse(openai.CodexOAuthRedirectURI)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", "", errors.New("oauth redirect URI missing host")
	}
	callbackPath := parsed.Path
	if strings.TrimSpace(callbackPath) == "" {
		callbackPath = "/"
	}
	return parsed.Host, callbackPath, nil
}

func (e *Engine) handleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query := r.URL.Query()
	state := strings.TrimSpace(query.Get("state"))
	code := strings.TrimSpace(query.Get("code"))
	oauthErr := strings.TrimSpace(query.Get("error"))
	oauthErrDesc := strings.TrimSpace(query.Get("error_description"))

	if state == "" {
		http.Error(w, "missing state", http.StatusBadRequest)
		return
	}
	e.oauthMu.Lock()
	flowID, ok := e.oauthFlowByState[state]
	if !ok {
		e.oauthMu.Unlock()
		http.Error(w, "unknown or expired oauth flow", http.StatusBadRequest)
		return
	}
	flow, ok := e.oauthFlows[flowID]
	if !ok {
		e.oauthMu.Unlock()
		http.Error(w, "unknown or expired oauth flow", http.StatusBadRequest)
		return
	}
	if oauthErr != "" {
		flow.Status = oauthFlowStatusFailed
		if oauthErrDesc != "" {
			flow.LastError = oauthErr + ": " + oauthErrDesc
		} else {
			flow.LastError = oauthErr
		}
		e.oauthMu.Unlock()
		http.Error(w, flow.LastError, http.StatusBadRequest)
		return
	}
	if code == "" {
		flow.Status = oauthFlowStatusFailed
		flow.LastError = "missing code in callback"
		e.oauthMu.Unlock()
		http.Error(w, "missing code", http.StatusBadRequest)
		return
	}
	flow.Code = code
	flow.Status = oauthFlowStatusCodeReceived
	flow.LastError = ""
	e.oauthMu.Unlock()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("Authorization received. You can return to KeenBench.")) // best-effort response
}

func generateOAuthFlowID() (string, error) {
	raw := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func (e *Engine) WorkbenchCreate(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(params, &req)
	settingsData, err := e.settings.Load()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseSettings, err.Error())
	}
	defaultModel := settingsData.UserDefaultModelID
	if _, ok := getModel(defaultModel); !ok {
		defaultModel = ModelOpenAIID
	}
	wb, err := e.workbenches.Create(req.Name, defaultModel)
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Info("workbench.create", "workbench_id", wb.ID, "name", wb.Name)
	return map[string]any{"workbench_id": wb.ID}, nil
}

func (e *Engine) WorkbenchList(ctx context.Context, _ json.RawMessage) (any, *errinfo.ErrorInfo) {
	items, err := e.workbenches.List()
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Debug("workbench.list", "count", len(items))
	return map[string]any{"workbenches": items}, nil
}

func (e *Engine) WorkbenchOpen(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	wb, err := e.workbenches.Open(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Info("workbench.open", "workbench_id", wb.ID, "name", wb.Name)
	return map[string]any{"workbench": wb}, nil
}

func (e *Engine) WorkbenchFilesList(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	files, err := e.workbenches.FilesList(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Debug("workbench.files_list", "workbench_id", req.WorkbenchID, "count", len(files))
	return map[string]any{"files": files}, nil
}

func (e *Engine) WorkbenchGetScope(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if req.WorkbenchID == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid workbench id")
	}
	if _, err := e.workbenches.Open(req.WorkbenchID); err != nil {
		if errors.Is(err, workbench.ErrInvalidPath) || strings.Contains(err.Error(), "invalid workbench id") {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if os.IsNotExist(err) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "workbench not found")
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkbench, err.Error())
	}
	sandboxRoot := filepath.Join(e.workbenchesRoot(), req.WorkbenchID)
	return map[string]any{
		"limits": map[string]any{
			"max_files":      workbenchMaxFiles,
			"max_file_bytes": workbenchMaxFileBytes,
		},
		"supported_types": map[string]any{
			"editable_extensions": workbenchEditableExtensions,
			"readable_kinds": []string{
				workbench.FileKindText,
				workbench.FileKindDocx,
				workbench.FileKindOdt,
				workbench.FileKindXlsx,
				workbench.FileKindPptx,
				workbench.FileKindPdf,
				workbench.FileKindImage,
			},
		},
		"sandbox_root": sandboxRoot,
	}, nil
}

func (e *Engine) WorkbenchFilesAdd(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string   `json:"workbench_id"`
		SourcePaths []string `json:"source_paths"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	results, err := e.workbenches.FilesAdd(req.WorkbenchID, req.SourcePaths)
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Info("workbench.files_add", "workbench_id", req.WorkbenchID, "count", len(req.SourcePaths))
	e.logger.Debug("workbench.files_add_results", "workbench_id", req.WorkbenchID, "results", logging.RedactAny(results))
	added := make([]string, 0, len(results))
	updated := make([]string, 0)
	for _, result := range results {
		switch result.Status {
		case "added":
			if result.FileName != "" {
				added = append(added, result.FileName)
			}
		case "updated":
			if result.FileName != "" {
				updated = append(updated, result.FileName)
			}
		}
	}
	e.notifyWorkbenchFilesChanged(req.WorkbenchID, added, nil, updated)
	e.emitClutterChanged(req.WorkbenchID)
	return map[string]any{"add_results": results}, nil
}

func (e *Engine) WorkbenchFilesRemove(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID    string   `json:"workbench_id"`
		WorkbenchPaths []string `json:"workbench_paths"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	results, err := e.workbenches.FilesRemove(req.WorkbenchID, req.WorkbenchPaths)
	if err != nil {
		if err.Error() == "draft exists" {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "draft exists; review or discard before continuing")
		}
		if errors.Is(err, workbench.ErrInvalidPath) || strings.Contains(err.Error(), "invalid workbench id") {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if os.IsNotExist(err) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "workbench not found")
		}
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Info("workbench.files_remove", "workbench_id", req.WorkbenchID, "count", len(req.WorkbenchPaths))
	e.logger.Debug("workbench.files_remove_results", "workbench_id", req.WorkbenchID, "results", logging.RedactAny(results))
	removed := make([]string, 0, len(results))
	updated := make([]string, 0)
	for _, result := range results {
		switch result.Status {
		case "removed":
			if result.Path != "" {
				removed = append(removed, result.Path)
			}
		case "updated":
			if result.Path != "" {
				updated = append(updated, result.Path)
			}
		}
	}
	e.notifyWorkbenchFilesChanged(req.WorkbenchID, nil, removed, updated)
	e.emitClutterChanged(req.WorkbenchID)
	return map[string]any{"remove_results": results}, nil
}

func (e *Engine) WorkbenchFilesExtract(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID    string   `json:"workbench_id"`
		DestinationDir string   `json:"destination_dir"`
		WorkbenchPaths []string `json:"workbench_paths"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	results, err := e.workbenches.FilesExtract(req.WorkbenchID, req.DestinationDir, req.WorkbenchPaths)
	if err != nil {
		if err.Error() == "draft exists" {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "draft exists; review or discard before continuing")
		}
		if errors.Is(err, workbench.ErrInvalidPath) ||
			errors.Is(err, workbench.ErrInvalidDestination) ||
			strings.Contains(err.Error(), "invalid workbench id") ||
			strings.Contains(err.Error(), "destination") {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if os.IsNotExist(err) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "workbench not found")
		}
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	e.logger.Info("workbench.files_extract", "workbench_id", req.WorkbenchID, "count", len(results))
	e.logger.Debug("workbench.files_extract_results", "workbench_id", req.WorkbenchID, "results", logging.RedactAny(results))
	return map[string]any{"extract_results": results}, nil
}

func (e *Engine) WorkbenchDelete(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid params")
	}
	if req.WorkbenchID == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "invalid workbench id")
	}
	if err := e.workbenches.Delete(req.WorkbenchID); err != nil {
		if err.Error() == "draft exists" {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "draft exists; review or discard before continuing")
		}
		if errors.Is(err, workbench.ErrInvalidPath) || strings.Contains(err.Error(), "invalid workbench id") {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, err.Error())
		}
		if os.IsNotExist(err) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkbench, "workbench not found")
		}
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkbench, err.Error())
	}
	delete(e.sessionConsent, req.WorkbenchID)
	e.logger.Info("workbench.delete", "workbench_id", req.WorkbenchID)
	return map[string]any{}, nil
}

func (e *Engine) EgressGetConsentStatus(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	scope, err := e.workbenches.ComputeScopeHash(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	consent, err := e.workbenches.ReadConsent(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	persistedConsent := consent.Workshop.ScopeHash != "" && consent.Workshop.ScopeHash == scope && consent.Workshop.ProviderID == model.ProviderID && consent.Workshop.ModelID == modelID
	sessionConsent := e.sessionConsent[req.WorkbenchID]
	sessionOk := sessionConsent.ScopeHash != "" && sessionConsent.ScopeHash == scope && sessionConsent.ProviderID == model.ProviderID && sessionConsent.ModelID == modelID
	consented := persistedConsent || sessionOk
	e.logger.Debug("egress.get_consent", "workbench_id", req.WorkbenchID, "consented", consented)
	return map[string]any{
		"consented":   consented,
		"provider_id": model.ProviderID,
		"model_id":    modelID,
		"scope_hash":  scope,
		"persisted":   persistedConsent,
	}, nil
}

func (e *Engine) EgressGrantWorkshopConsent(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ProviderID  string `json:"provider_id"`
		ModelID     string `json:"model_id"`
		ScopeHash   string `json:"scope_hash"`
		Persist     bool   `json:"persist"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	model, ok := getModel(req.ModelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	if req.ProviderID != model.ProviderID {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported provider")
	}
	currentScope, err := e.workbenches.ComputeScopeHash(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if req.ScopeHash == "" || req.ScopeHash != currentScope {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "scope hash mismatch")
	}
	consent := workbench.WorkshopConsent{ProviderID: req.ProviderID, ModelID: req.ModelID, ScopeHash: req.ScopeHash, ConsentedAt: time.Now().UTC().Format(time.RFC3339), Persisted: req.Persist}
	if req.Persist {
		payload := &workbench.Consent{Workshop: consent}
		if err := e.workbenches.WriteConsent(req.WorkbenchID, payload); err != nil {
			return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
	} else {
		e.sessionConsent[req.WorkbenchID] = consent
	}
	e.logger.Info("egress.grant_consent", "workbench_id", req.WorkbenchID, "provider_id", req.ProviderID, "persist", req.Persist)
	return map[string]any{}, nil
}

func (e *Engine) WorkshopGetState(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	state, _ := e.readWorkshopState(req.WorkbenchID)
	draft, _ := e.workbenches.DraftState(req.WorkbenchID)
	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	wb, _ := e.workbenches.Open(req.WorkbenchID)
	e.logger.Debug("workshop.get_state", "workbench_id", req.WorkbenchID, "has_draft", draft != nil)
	return map[string]any{
		"active_model_id":     modelID,
		"default_model_id":    wb.DefaultModelID,
		"has_draft":           draft != nil,
		"pending_proposal_id": state.PendingProposalID,
	}, nil
}

func (e *Engine) WorkshopGetConversation(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	items, err := e.readConversation(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	e.logger.Debug("workshop.get_conversation", "workbench_id", req.WorkbenchID, "count", len(items))
	return map[string]any{"messages": items}, nil
}

func (e *Engine) WorkshopUndoToMessage(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		MessageID   string `json:"message_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if strings.TrimSpace(req.WorkbenchID) == "" || strings.TrimSpace(req.MessageID) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "workbench_id and message_id are required")
	}
	revisionID, _, errInfo := e.undoWorkshopToMessage(req.WorkbenchID, req.MessageID)
	if errInfo != nil {
		return nil, errInfo
	}
	if err := e.clearRPIState(req.WorkbenchID); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if e.notify != nil {
		payload := map[string]any{
			"workbench_id":         req.WorkbenchID,
			"conversation_head_id": req.MessageID,
		}
		if revisionID != "" {
			payload["draft_revision_id"] = revisionID
		}
		e.notify("WorkshopUndoCompleted", payload)
	}
	resp := map[string]any{}
	if revisionID != "" {
		resp["draft_revision_id"] = revisionID
	}
	return resp, nil
}

func (e *Engine) WorkshopRegenerate(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		MessageID   string `json:"message_id,omitempty"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if strings.TrimSpace(req.WorkbenchID) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "workbench_id is required")
	}
	targetUserMessageID, errInfo := e.resolveRegenerateUserMessage(req.WorkbenchID, req.MessageID)
	if errInfo != nil {
		return nil, errInfo
	}
	if e.notify != nil {
		e.notify("WorkshopRegenerateStarted", map[string]any{
			"workbench_id":      req.WorkbenchID,
			"from_message_id":   req.MessageID,
			"target_message_id": targetUserMessageID,
		})
	}
	if _, _, errInfo := e.undoWorkshopToMessage(req.WorkbenchID, targetUserMessageID); errInfo != nil {
		return nil, errInfo
	}
	if err := e.clearRPIState(req.WorkbenchID); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	runParams, err := json.Marshal(map[string]any{
		"workbench_id": req.WorkbenchID,
		"message_id":   targetUserMessageID,
	})
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	runResp, runErr := e.WorkshopRunAgent(ctx, runParams)
	if runErr != nil {
		return nil, runErr
	}
	messageID, _ := runResp.(map[string]any)["message_id"].(string)
	if e.notify != nil {
		e.notify("WorkshopRegenerateCompleted", map[string]any{
			"workbench_id": req.WorkbenchID,
			"message_id":   messageID,
		})
	}
	return map[string]any{"message_id": messageID}, nil
}

func (e *Engine) WorkshopSendUserMessage(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Text        string `json:"text"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if err := e.ensureWorkshopUnlocked(req.WorkbenchID); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Text) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "empty message")
	}
	id := fmt.Sprintf("u-%d", time.Now().UnixNano())
	e.logger.Debug("workshop.send_user_message", "workbench_id", req.WorkbenchID, "message_id", id, "text", req.Text)
	entry := conversationMessage{
		Type:      "user_message",
		MessageID: id,
		Role:      "user",
		Text:      req.Text,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := e.appendConversation(req.WorkbenchID, entry); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if err := e.clearRPIState(req.WorkbenchID); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	return map[string]any{"message_id": id}, nil
}

func (e *Engine) WorkshopCancelRun(_ context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if strings.TrimSpace(req.WorkbenchID) == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "workbench_id is required")
	}
	cancelRequested := e.cancelWorkshopRun(req.WorkbenchID)
	if cancelRequested && e.notify != nil {
		e.notify("WorkshopRunCancelRequested", map[string]any{
			"workbench_id": req.WorkbenchID,
		})
	}
	return map[string]any{
		"cancel_requested": cancelRequested,
	}, nil
}

func (e *Engine) WorkshopStreamAssistantReply(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		MessageID   string `json:"message_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if err := e.ensureWorkshopUnlocked(req.WorkbenchID); err != nil {
		return nil, err
	}
	runCtx, runID, errInfo := e.beginWorkshopRun(ctx, req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	defer e.endWorkshopRun(req.WorkbenchID, runID)
	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	if err := e.ensureProviderReadyFor(runCtx, model.ProviderID); err != nil {
		return nil, err
	}
	if err := e.ensureConsent(req.WorkbenchID); err != nil {
		return nil, err
	}
	messages, errInfo := e.buildChatMessages(runCtx, req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	apiKey, errInfo := e.providerKey(runCtx, model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	client, errInfo := e.clientForProvider(model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	assistantID := fmt.Sprintf("a-%d", time.Now().UnixNano())
	e.logger.Debug("provider.stream_chat", "workbench_id", req.WorkbenchID, "message_id", assistantID, "model", modelID, "api_key", logging.RedactValue(apiKey), "messages", messages)
	var fullResponse strings.Builder
	_, err := e.streamChatWithRateLimitRetry(
		runCtx,
		req.WorkbenchID,
		client,
		apiKey,
		modelID,
		messages,
		func(delta string) {
			fullResponse.WriteString(delta)
			if e.notify != nil {
				e.notify("WorkshopAssistantStreamDelta", map[string]any{
					"workbench_id": req.WorkbenchID,
					"message_id":   assistantID,
					"token_delta":  delta,
				})
			}
		},
		model.ProviderID,
		"stream",
	)
	if err != nil {
		e.logger.Warn("provider.stream_chat_failed", "workbench_id", req.WorkbenchID, "error", err.Error())
		return nil, mapLLMError(errinfo.PhaseWorkshop, model.ProviderID, err)
	}
	if scope, err := e.workbenches.ComputeScopeHash(req.WorkbenchID); err == nil {
		e.appendEgressEvent(req.WorkbenchID, "workshop_chat", model.ProviderID, modelID, scope)
	}
	e.logger.Debug("provider.stream_chat_complete", "workbench_id", req.WorkbenchID, "message_id", assistantID, "response_length", len(fullResponse.String()))
	entry := conversationMessage{
		Type:      "assistant_message",
		MessageID: assistantID,
		Role:      "assistant",
		Text:      fullResponse.String(),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := e.appendConversation(req.WorkbenchID, entry); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if e.notify != nil {
		e.notify("WorkshopAssistantMessageComplete", map[string]any{
			"workbench_id": req.WorkbenchID,
			"message_id":   assistantID,
		})
	}
	return map[string]any{}, nil
}

func (e *Engine) WorkshopProposeChanges(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	if err := e.ensureWorkshopUnlocked(req.WorkbenchID); err != nil {
		return nil, err
	}
	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	if err := e.ensureProviderReadyFor(ctx, model.ProviderID); err != nil {
		return nil, err
	}
	if err := e.ensureConsent(req.WorkbenchID); err != nil {
		return nil, err
	}
	prompt, errInfo := e.buildProposalPrompt(ctx, req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	apiKey, errInfo := e.providerKey(ctx, model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	client, errInfo := e.clientForProvider(model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	messages := []llm.Message{
		{Role: "system", Content: proposalSystemPrompt},
		{Role: "user", Content: prompt},
	}
	e.logger.Debug("provider.chat", "workbench_id", req.WorkbenchID, "model", modelID, "api_key", logging.RedactValue(apiKey), "messages", messages)
	response, err := client.Chat(ctx, apiKey, providerModelName(modelID), messages)
	if err != nil {
		e.logger.Warn("provider.chat_failed", "workbench_id", req.WorkbenchID, "error", err.Error())
		return nil, mapLLMError(errinfo.PhaseWorkshop, model.ProviderID, err)
	}
	e.logger.Debug("provider.chat_complete", "workbench_id", req.WorkbenchID, "response", response)
	proposal, errInfo := e.parseProposal(response)
	if errInfo != nil && isProposalRepairable(errInfo) {
		e.logger.Warn("workshop.proposal_parse_failed", "workbench_id", req.WorkbenchID, "detail", errInfo.Detail)
		strictPrompt := fmt.Sprintf("The previous response was invalid: %s\n\n%s", errInfo.Detail, prompt)
		strictMessages := []llm.Message{
			{Role: "system", Content: proposalSystemPromptStrict},
			{Role: "user", Content: strictPrompt},
		}
		e.logger.Debug("provider.chat_retry", "workbench_id", req.WorkbenchID, "model", modelID, "api_key", logging.RedactValue(apiKey), "messages", strictMessages)
		response, err = client.Chat(ctx, apiKey, providerModelName(modelID), strictMessages)
		if err != nil {
			e.logger.Warn("provider.chat_retry_failed", "workbench_id", req.WorkbenchID, "error", err.Error())
			return nil, mapLLMError(errinfo.PhaseWorkshop, model.ProviderID, err)
		}
		e.logger.Debug("provider.chat_retry_complete", "workbench_id", req.WorkbenchID, "response", response)
		proposal, errInfo = e.parseProposal(response)
		if errInfo != nil && isProposalRepairable(errInfo) {
			e.logger.Warn("workshop.proposal_fallback", "workbench_id", req.WorkbenchID, "detail", errInfo.Detail)
			proposal = fallbackProposal(errInfo.Detail)
			errInfo = nil
		}
	}
	if errInfo != nil {
		return nil, errInfo
	}
	if scope, err := e.workbenches.ComputeScopeHash(req.WorkbenchID); err == nil {
		e.appendEgressEvent(req.WorkbenchID, "workshop_proposal", model.ProviderID, modelID, scope)
	}
	proposalID := fmt.Sprintf("p-%d", time.Now().UnixNano())
	proposal.ProposalID = proposalID
	if err := e.writeProposal(req.WorkbenchID, proposal); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	e.logger.Info("workshop.proposal_created", "workbench_id", req.WorkbenchID, "proposal_id", proposalID)
	state, _ := e.readWorkshopState(req.WorkbenchID)
	if proposal.NoChanges {
		state.PendingProposalID = ""
		_ = e.writeWorkshopState(req.WorkbenchID, state)
		return map[string]any{"proposal_id": proposalID, "no_changes": true}, nil
	}
	state.PendingProposalID = proposalID
	_ = e.writeWorkshopState(req.WorkbenchID, state)
	if e.notify != nil {
		e.notify("WorkshopProposalReady", map[string]any{
			"workbench_id": req.WorkbenchID,
			"proposal_id":  proposalID,
		})
	}
	return map[string]any{"proposal_id": proposalID, "no_changes": false}, nil
}

func (e *Engine) WorkshopGetProposal(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ProposalID  string `json:"proposal_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	proposal, err := e.readProposal(req.WorkbenchID, req.ProposalID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	e.logger.Debug("workshop.get_proposal", "workbench_id", req.WorkbenchID, "proposal_id", req.ProposalID)
	return map[string]any{"proposal": proposal}, nil
}

func (e *Engine) WorkshopDismissProposal(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ProposalID  string `json:"proposal_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	stateInfo, err := e.readWorkshopState(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	stateInfo.PendingProposalID = ""
	if err := e.writeWorkshopState(req.WorkbenchID, stateInfo); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	e.logger.Info("workshop.proposal_dismissed", "workbench_id", req.WorkbenchID, "proposal_id", req.ProposalID)
	return map[string]any{"cleared": true}, nil
}

func (e *Engine) WorkshopApplyProposal(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		ProposalID  string `json:"proposal_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	proposal, err := e.readProposal(req.WorkbenchID, req.ProposalID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if proposal.NoChanges {
		stateInfo, _ := e.readWorkshopState(req.WorkbenchID)
		stateInfo.PendingProposalID = ""
		_ = e.writeWorkshopState(req.WorkbenchID, stateInfo)
		return map[string]any{"no_changes": true}, nil
	}
	if errInfo := e.validateProposalForApply(req.WorkbenchID, proposal); errInfo != nil {
		return nil, errInfo
	}
	existingDraft, _ := e.workbenches.DraftState(req.WorkbenchID)
	state, err := e.workbenches.CreateDraft(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	createdDraft := existingDraft == nil
	cleanupDraft := func() {
		if createdDraft {
			_ = e.workbenches.DiscardDraft(req.WorkbenchID)
		}
	}
	if existingDraft == nil {
		if err := e.ensureDraftBaseline(ctx, req.WorkbenchID, state.DraftID); err != nil {
			cleanupDraft()
			if errInfo := mapToolWorkerError(errinfo.PhaseWorkshop, err); errInfo != nil {
				return nil, errInfo
			}
			return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
		}
	}
	e.logger.Info("workshop.apply_proposal", "workbench_id", req.WorkbenchID, "proposal_id", req.ProposalID)
	stagingName := fmt.Sprintf("draft.%s.staging", req.ProposalID)
	if err := e.workbenches.CreateDraftStaging(req.WorkbenchID, stagingName); err != nil {
		cleanupDraft()
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	applied := 0
	failures := 0
	e.logger.Debug("workshop.apply_proposal_writes", "workbench_id", req.WorkbenchID, "writes", logging.RedactAny(proposal.Writes))
	for _, write := range proposal.Writes {
		if len(write.Content) > maxProposalContentBytes {
			failures++
			continue
		}
		if err := e.workbenches.ApplyWriteToArea(req.WorkbenchID, stagingName, write.Path, write.Content); err != nil {
			if errors.Is(err, workbench.ErrSandboxViolation) {
				_ = e.workbenches.RemoveDraftStaging(req.WorkbenchID, stagingName)
				cleanupDraft()
				return nil, errinfo.SandboxViolation(errinfo.PhaseWorkshop, err.Error())
			}
			if errors.Is(err, workbench.ErrInvalidPath) {
				_ = e.workbenches.RemoveDraftStaging(req.WorkbenchID, stagingName)
				cleanupDraft()
				return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, err.Error())
			}
			failures++
			continue
		}
		applied++
	}

	summaries := make(map[string]string)
	focusHints := make(map[string]map[string]any)
	for _, entry := range proposal.Ops {
		if err := e.applyOfficeOps(ctx, req.WorkbenchID, stagingName, entry); err != nil {
			if errInfo := mapToolWorkerError(errinfo.PhaseWorkshop, err); errInfo != nil {
				_ = e.workbenches.RemoveDraftStaging(req.WorkbenchID, stagingName)
				cleanupDraft()
				return nil, errInfo
			}
			failures++
			continue
		}
		applied++
		summaries[entry.Path] = entry.Summary
		switch strings.ToLower(entry.Kind) {
		case workbench.FileKindXlsx:
			if hint := buildXlsxFocusHint(entry.Ops); hint != nil {
				focusHints[entry.Path] = hint
			}
		case workbench.FileKindDocx:
			if hint := buildDocxFocusHint(entry.Ops); hint != nil {
				focusHints[entry.Path] = hint
			}
		case workbench.FileKindPptx:
			if hint := buildPptxFocusHint(entry.Ops); hint != nil {
				focusHints[entry.Path] = hint
			}
		}
	}

	if applied == 0 && failures > 0 {
		_ = e.workbenches.RemoveDraftStaging(req.WorkbenchID, stagingName)
		cleanupDraft()
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "no proposal changes applied")
	}

	if err := e.workbenches.CommitDraftStaging(req.WorkbenchID, stagingName); err != nil {
		_ = e.workbenches.RemoveDraftStaging(req.WorkbenchID, stagingName)
		cleanupDraft()
		return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if len(summaries) > 0 {
		if err := e.writeProposalSummaries(req.WorkbenchID, state.DraftID, summaries); err != nil {
			return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
	}
	if len(focusHints) > 0 {
		if err := e.writeProposalFocusHints(req.WorkbenchID, state.DraftID, focusHints); err != nil {
			return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
	}
	stateInfo, _ := e.readWorkshopState(req.WorkbenchID)
	stateInfo.PendingProposalID = ""
	_ = e.writeWorkshopState(req.WorkbenchID, stateInfo)
	e.notifyDraftState(req.WorkbenchID, state)
	return map[string]any{"draft_id": state.DraftID}, nil
}

const (
	maxAgentTurns       = 200
	maxToolCallsPerTurn = 50
	rpiResearchMaxTurns = 30
	rpiPlanMaxTurns     = 10
	rpiItemMaxTurns     = 30
	rpiMaxPlanInflation = 2

	rpiStatusPending = "pending"
	rpiStatusDone    = "done"
	rpiStatusFailed  = "failed"

	rpiResearchFile = "research.md"
	rpiPlanFile     = "plan.md"

	// Loop detection constants
	loopDetectionWindow  = 10 // sliding window of recent tool calls
	loopDetectionWarning = 3  // identical calls in window to trigger warning
	loopDetectionStop    = 5  // identical calls in window to hard-stop
)

// toolCallHash computes a hash of tool name + arguments for loop detection.
func toolCallHash(call llm.ToolCall) string {
	key := call.Function.Name + ":" + toolCallFingerprint(call)
	h := sha256.Sum256([]byte(key))
	return hex.EncodeToString(h[:8])
}

func toolCallFingerprint(call llm.ToolCall) string {
	name := strings.TrimSpace(call.Function.Name)
	args := map[string]any{}
	if err := json.Unmarshal([]byte(call.Function.Arguments), &args); err != nil {
		return strings.TrimSpace(call.Function.Arguments)
	}

	switch name {
	case "read_file":
		return marshalToolFingerprint(map[string]any{
			"path":        strings.TrimSpace(toString(args["path"])),
			"sheet":       strings.TrimSpace(toString(args["sheet"])),
			"range":       strings.TrimSpace(toString(args["range"])),
			"section":     strings.TrimSpace(toString(args["section"])),
			"slide_index": toIntDefault(args["slide_index"], 0),
			"pages":       strings.TrimSpace(toString(args["pages"])),
			"line_start":  toIntDefault(args["line_start"], 0),
			"line_count":  toIntDefault(args["line_count"], 0),
		})
	case "table_read_rows":
		return marshalToolFingerprint(map[string]any{
			"path":      strings.TrimSpace(toString(args["path"])),
			"row_start": toIntDefault(args["row_start"], 0),
			"row_count": toIntDefault(args["row_count"], 0),
			"columns":   normalizeStringList(args["columns"]),
		})
	case "table_query":
		return marshalToolFingerprint(map[string]any{
			"path":          strings.TrimSpace(toString(args["path"])),
			"query":         normalizeSQLForLoop(toString(args["query"])),
			"window_rows":   toIntDefault(args["window_rows"], 0),
			"window_offset": toIntDefault(args["window_offset"], 0),
		})
	case "table_export":
		return marshalToolFingerprint(map[string]any{
			"path":        strings.TrimSpace(toString(args["path"])),
			"query":       normalizeSQLForLoop(toString(args["query"])),
			"target_path": strings.TrimSpace(toString(args["target_path"])),
			"format":      strings.ToLower(strings.TrimSpace(toString(args["format"]))),
			"sheet":       strings.TrimSpace(toString(args["sheet"])),
		})
	case "table_update_from_export":
		return marshalToolFingerprint(map[string]any{
			"path":                       strings.TrimSpace(toString(args["path"])),
			"query":                      normalizeSQLForLoop(toString(args["query"])),
			"target_path":                strings.TrimSpace(toString(args["target_path"])),
			"sheet":                      strings.TrimSpace(toString(args["sheet"])),
			"mode":                       strings.ToLower(strings.TrimSpace(toString(args["mode"]))),
			"start_cell":                 strings.ToUpper(strings.TrimSpace(toString(args["start_cell"]))),
			"include_header":             toBoolDefault(args["include_header"], false),
			"create_workbook_if_missing": toBoolDefault(args["create_workbook_if_missing"], false),
			"create_sheet_if_missing":    toBoolDefault(args["create_sheet_if_missing"], false),
			"clear_target_range":         toBoolDefault(args["clear_target_range"], false),
		})
	case "write_text_file":
		return marshalToolFingerprint(map[string]any{
			"path": strings.TrimSpace(toString(args["path"])),
		})
	case "xlsx_operations", "docx_operations", "pptx_operations":
		ops := normalizeOps(args["operations"])
		firstOp := ""
		firstSheet := ""
		if len(ops) > 0 {
			firstOp = strings.TrimSpace(toString(ops[0]["op"]))
			firstSheet = strings.TrimSpace(toString(ops[0]["sheet"]))
		}
		return marshalToolFingerprint(map[string]any{
			"path":          strings.TrimSpace(toString(args["path"])),
			"operations":    len(ops),
			"first_op_type": firstOp,
			"first_sheet":   firstSheet,
		})
	default:
		return marshalToolFingerprint(args)
	}
}

func marshalToolFingerprint(payload map[string]any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(data)
}

func normalizeStringList(raw any) []string {
	values, ok := raw.([]any)
	if !ok {
		if typed, ok := raw.([]string); ok {
			copied := make([]string, 0, len(typed))
			for _, value := range typed {
				trimmed := strings.TrimSpace(value)
				if trimmed != "" {
					copied = append(copied, trimmed)
				}
			}
			return copied
		}
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(toString(value))
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func normalizeSQLForLoop(query string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(query))), " ")
}

func normalizeOps(raw any) []map[string]any {
	typed, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(typed))
	for _, item := range typed {
		operation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, operation)
	}
	return out
}

func toString(raw any) string {
	switch typed := raw.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		if raw == nil {
			return ""
		}
		return fmt.Sprintf("%v", raw)
	}
}

func toIntDefault(raw any, fallback int) int {
	switch typed := raw.(type) {
	case float64:
		return int(typed)
	case float32:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case int32:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
		if parsed, err := typed.Float64(); err == nil {
			return int(parsed)
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return fallback
		}
		if parsed, err := strconv.Atoi(trimmed); err == nil {
			return parsed
		}
	}
	return fallback
}

func toBoolDefault(raw any, fallback bool) bool {
	switch typed := raw.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case float32:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case int32:
		return typed != 0
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed != 0
		}
		if parsed, err := typed.Float64(); err == nil {
			return parsed != 0
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "t", "1", "yes", "y":
			return true
		case "false", "f", "0", "no", "n":
			return false
		case "":
			return fallback
		}
	}
	return fallback
}

// checkLoopDetection checks a sliding window of recent tool call hashes for
// repeated identical calls. Returns ("warn", hash) if warning threshold hit,
// ("stop", hash) if hard-stop threshold hit, or ("", "") if OK.
func checkLoopDetection(window []string) (string, string) {
	counts := make(map[string]int, len(window))
	for _, h := range window {
		counts[h]++
	}
	for h, count := range counts {
		if count >= loopDetectionStop {
			return "stop", h
		}
		if count >= loopDetectionWarning {
			return "warn", h
		}
	}
	return "", ""
}

type agentLoopConfig struct {
	workbenchID           string
	client                LLMClient
	apiKey                string
	modelID               string
	messages              []llm.ChatMessage
	tools                 []llm.Tool
	maxTurns              int
	handler               *ToolHandler
	phaseName             string
	persistToConversation bool
	emitStreamDeltas      bool
	toolLogSeqStart       int
}

type agentLoopResult struct {
	finalText     string
	toolLogSeqEnd int
	turnCount     int
	toolCallCount int
	err           *errinfo.ErrorInfo
}

func withSubphase(errInfo *errinfo.ErrorInfo, subphase string) *errinfo.ErrorInfo {
	if errInfo == nil {
		return nil
	}
	copied := *errInfo
	copied.Subphase = subphase
	return &copied
}

func sleepWithContext(ctx context.Context, wait time.Duration) error {
	if wait <= 0 {
		return nil
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func rateLimitBackoffDuration(attempt int) time.Duration {
	if attempt <= 0 {
		return 0
	}
	wait := rateLimitRetryBaseDelay * time.Duration(1<<uint(attempt-1))
	if wait > rateLimitRetryMaxDelay {
		return rateLimitRetryMaxDelay
	}
	return wait
}

func isRateLimitErrorInfo(errInfo *errinfo.ErrorInfo) bool {
	if errInfo == nil {
		return false
	}
	if errInfo.ErrorCode != errinfo.CodeProviderUnavailable {
		return false
	}
	detail := strings.ToLower(strings.TrimSpace(errInfo.Detail))
	return strings.Contains(detail, "rate limit") ||
		strings.Contains(detail, "rate-limited") ||
		strings.Contains(detail, "too many requests") ||
		strings.Contains(detail, "429")
}

func (e *Engine) notifyRateLimitWarning(workbenchID, providerID, modelID, phase string, attempt int, wait time.Duration) {
	if e.notify == nil {
		return
	}
	e.notify("WorkshopRateLimitWarning", map[string]any{
		"workbench_id":  workbenchID,
		"provider_id":   providerID,
		"model_id":      modelID,
		"phase":         strings.TrimSpace(phase),
		"retry_attempt": attempt,
		"retry_max":     rateLimitRetryMaxAttempts,
		"wait_ms":       wait.Milliseconds(),
		"warning_message": fmt.Sprintf(
			"Rate limit reached. Retrying in %d ms (%d/%d).",
			wait.Milliseconds(),
			attempt,
			rateLimitRetryMaxAttempts,
		),
	})
}

func (e *Engine) beginWorkshopRun(parent context.Context, workbenchID string) (context.Context, string, *errinfo.ErrorInfo) {
	runCtx, cancel := context.WithCancel(parent)
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())

	e.runMu.Lock()
	defer e.runMu.Unlock()
	if _, exists := e.workshopRuns[workbenchID]; exists {
		cancel()
		return nil, "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "workshop run already in progress")
	}
	e.workshopRuns[workbenchID] = workshopRunHandle{
		runID:  runID,
		cancel: cancel,
	}
	return runCtx, runID, nil
}

func (e *Engine) endWorkshopRun(workbenchID, runID string) {
	var cancel context.CancelFunc

	e.runMu.Lock()
	handle, ok := e.workshopRuns[workbenchID]
	if ok && handle.runID == runID {
		cancel = handle.cancel
		delete(e.workshopRuns, workbenchID)
	}
	e.runMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (e *Engine) cancelWorkshopRun(workbenchID string) bool {
	e.runMu.Lock()
	handle, ok := e.workshopRuns[workbenchID]
	e.runMu.Unlock()
	if !ok || handle.cancel == nil {
		return false
	}
	handle.cancel()
	return true
}

func (e *Engine) streamChatWithToolsWithRateLimitRetry(
	ctx context.Context,
	cfg agentLoopConfig,
	messages []llm.ChatMessage,
	onDelta func(string),
	turn int,
	providerID string,
	logPrefix string,
) (llm.ChatResponse, error) {
	var lastErr error
	for attempt := 0; attempt <= rateLimitRetryMaxAttempts; attempt++ {
		resp, err := cfg.client.StreamChatWithTools(
			ctx,
			cfg.apiKey,
			providerModelName(cfg.modelID),
			messages,
			cfg.tools,
			onDelta,
		)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !errors.Is(err, llm.ErrRateLimited) {
			return llm.ChatResponse{}, err
		}
		if attempt == rateLimitRetryMaxAttempts {
			return llm.ChatResponse{}, err
		}
		retryAttempt := attempt + 1
		wait := rateLimitBackoffDuration(retryAttempt)
		e.logger.Warn(
			logPrefix+"_rate_limited",
			"turn", turn,
			"retry_attempt", retryAttempt,
			"retry_max", rateLimitRetryMaxAttempts,
			"retry_in_ms", wait.Milliseconds(),
		)
		e.notifyRateLimitWarning(cfg.workbenchID, providerID, cfg.modelID, cfg.phaseName, retryAttempt, wait)
		if err := e.sleep(ctx, wait); err != nil {
			return llm.ChatResponse{}, err
		}
	}
	if lastErr != nil {
		return llm.ChatResponse{}, lastErr
	}
	return llm.ChatResponse{}, errors.New("rate-limit retry failed")
}

func (e *Engine) streamChatWithRateLimitRetry(
	ctx context.Context,
	workbenchID string,
	client LLMClient,
	apiKey string,
	modelID string,
	messages []llm.Message,
	onDelta func(string),
	providerID string,
	phase string,
) (string, error) {
	var lastErr error
	for attempt := 0; attempt <= rateLimitRetryMaxAttempts; attempt++ {
		resp, err := client.StreamChat(ctx, apiKey, providerModelName(modelID), messages, onDelta)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !errors.Is(err, llm.ErrRateLimited) {
			return "", err
		}
		if attempt == rateLimitRetryMaxAttempts {
			return "", err
		}
		retryAttempt := attempt + 1
		wait := rateLimitBackoffDuration(retryAttempt)
		e.logger.Warn(
			"workshop.stream_rate_limited",
			"phase", strings.TrimSpace(phase),
			"retry_attempt", retryAttempt,
			"retry_max", rateLimitRetryMaxAttempts,
			"retry_in_ms", wait.Milliseconds(),
		)
		e.notifyRateLimitWarning(workbenchID, providerID, modelID, phase, retryAttempt, wait)
		if err := e.sleep(ctx, wait); err != nil {
			return "", err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("rate-limit retry failed")
}

func (e *Engine) runAgentLoop(ctx context.Context, cfg agentLoopConfig) agentLoopResult {
	result := agentLoopResult{
		toolLogSeqEnd: cfg.toolLogSeqStart,
	}
	if cfg.maxTurns <= 0 {
		cfg.maxTurns = maxAgentTurns
	}
	if cfg.handler == nil {
		cfg.handler = NewToolHandler(e, cfg.workbenchID, ctx)
	}
	messages := append([]llm.ChatMessage{}, cfg.messages...)
	assistantID := fmt.Sprintf("a-%d", time.Now().UnixNano())
	providerID := ""
	if model, ok := getModel(cfg.modelID); ok {
		providerID = model.ProviderID
	}

	logPrefix := "workshop.agent"
	if phase := strings.TrimSpace(cfg.phaseName); phase != "" {
		logPrefix = "workshop." + phase + ".agent"
	}
	e.logger.Info(logPrefix+"_start", "workbench_id", cfg.workbenchID, "message_id", assistantID)

	var fullResponse strings.Builder
	var finalAssistantText string
	var loopWindow []string
	agentStartTime := time.Now()
	totalToolCalls := 0

	var onDelta func(string)
	if cfg.emitStreamDeltas {
		onDelta = func(delta string) {
			fullResponse.WriteString(delta)
			if e.addPendingClutter(cfg.workbenchID, delta) {
				e.emitClutterChanged(cfg.workbenchID)
			}
			if e.notify != nil {
				e.notify("WorkshopAssistantStreamDelta", map[string]any{
					"workbench_id": cfg.workbenchID,
					"message_id":   assistantID,
					"token_delta":  delta,
				})
			}
		}
	}

	for turn := 0; turn < cfg.maxTurns; turn++ {
		payloadBytes := estimatePayloadBytes(messages)
		e.logger.Info(logPrefix+"_api_request", "turn", turn, "messages", len(messages), "payload_bytes_approx", payloadBytes)

		apiStart := time.Now()
		resp, err := e.streamChatWithToolsWithRateLimitRetry(
			ctx,
			cfg,
			messages,
			onDelta,
			turn,
			providerID,
			logPrefix,
		)
		if err != nil {
			if providerID == ProviderOpenAICodex && errors.Is(err, llm.ErrUnauthorized) {
				e.logCodexUnauthorizedDiagnostics()
			}
			e.logger.Warn(
				logPrefix+"_api_error",
				"turn", turn,
				"provider_id", providerID,
				"model_id", cfg.modelID,
				"error", err.Error(),
			)
			result.err = mapLLMError(errinfo.PhaseWorkshop, providerID, err)
			return result
		}
		if scope, err := e.workbenches.ComputeScopeHash(cfg.workbenchID); err == nil {
			e.appendEgressEvent(cfg.workbenchID, "workshop_agent_turn", providerID, cfg.modelID, scope)
		}

		apiElapsed := time.Since(apiStart)
		e.logger.Info(logPrefix+"_api_response", "turn", turn, "elapsed_ms", apiElapsed.Milliseconds(),
			"tool_call_count", len(resp.ToolCalls), "finish_reason", resp.FinishReason, "content_length", len(resp.Content))

		if len(resp.ToolCalls) == 0 {
			finalAssistantText = strings.TrimSpace(resp.Content)
			result.turnCount = turn + 1
			e.logger.Info(logPrefix+"_complete", "total_turns", turn+1, "total_tool_calls", totalToolCalls,
				"total_elapsed_ms", time.Since(agentStartTime).Milliseconds())
			break
		}

		toolCalls := resp.ToolCalls
		if len(toolCalls) > maxToolCallsPerTurn {
			toolCalls = toolCalls[:maxToolCallsPerTurn]
		}

		messages = append(messages, llm.ChatMessage{
			Role:      "assistant",
			Content:   resp.Content,
			ToolCalls: toolCalls,
		})

		if cfg.persistToConversation {
			e.clearPendingClutter(cfg.workbenchID)
			assistantConvEntry := conversationMessage{
				Type:      "assistant_message",
				MessageID: fmt.Sprintf("a-%d", time.Now().UnixNano()),
				Role:      "assistant",
				Text:      resp.Content,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			}
			if len(toolCalls) > 0 {
				tcData, _ := json.Marshal(toolCalls)
				assistantConvEntry.Metadata = map[string]any{"tool_calls": json.RawMessage(tcData)}
			}
			_ = e.appendConversation(cfg.workbenchID, assistantConvEntry)
		}

		turnToolCount := 0
		turnToolMS := int64(0)
		for _, call := range toolCalls {
			argsSummary := call.Function.Arguments
			if len(argsSummary) > 200 {
				argsSummary = argsSummary[:200] + "..."
			}
			e.logger.Info("workshop.tool_start", "tool", call.Function.Name, "entry_id", result.toolLogSeqEnd+1, "args_summary", argsSummary)

			callHash := toolCallHash(call)
			loopWindow = append(loopWindow, callHash)
			if len(loopWindow) > loopDetectionWindow {
				loopWindow = loopWindow[len(loopWindow)-loopDetectionWindow:]
			}

			action, _ := checkLoopDetection(loopWindow)
			if action == "stop" {
				e.logger.Warn(logPrefix+"_loop_hard_stop", "tool", call.Function.Name, "turn", turn)
				result.err = errinfo.AgentLoopDetected(
					errinfo.PhaseWorkshop,
					fmt.Sprintf(
						"repeated identical tool call detected: %s (hard stop after %d+ identical calls in last %d)",
						call.Function.Name,
						loopDetectionStop,
						loopDetectionWindow,
					),
				)
				return result
			}
			if action == "warn" {
				e.logger.Warn(logPrefix+"_loop_warning", "tool", call.Function.Name, "turn", turn)
				if e.notify != nil {
					e.notify("WorkshopAgentLoopWarning", map[string]any{
						"workbench_id": cfg.workbenchID,
						"tool_name":    call.Function.Name,
						"turn":         turn,
					})
				}
			}

			if e.notify != nil {
				e.notify("WorkshopToolExecuting", map[string]any{
					"workbench_id": cfg.workbenchID,
					"tool_name":    call.Function.Name,
					"tool_call_id": call.ID,
				})
			}

			toolStart := time.Now()
			toolResult, toolErr := cfg.handler.Execute(call)
			toolElapsed := time.Since(toolStart)
			if toolErr != nil {
				toolResult = fmt.Sprintf("Error: %s", toolErr.Error())
				e.logger.Warn("workshop.tool_error", "tool", call.Function.Name, "entry_id", result.toolLogSeqEnd, "error", toolErr.Error())
			}

			result.toolLogSeqEnd++
			logEntry := toolLogEntry{
				ID:        result.toolLogSeqEnd,
				Tool:      call.Function.Name,
				Args:      call.Function.Arguments,
				Result:    toolResult,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				ElapsedMS: toolElapsed.Milliseconds(),
			}
			if toolErr != nil {
				logEntry.Error = toolErr.Error()
			}

			receipt := buildToolReceipt(call.Function.Name, toolResult, result.toolLogSeqEnd)
			logEntry.Receipt = receipt
			if err := e.appendToolLog(cfg.workbenchID, logEntry); err != nil {
				e.logger.Warn("workshop.tool_log_write_error", "error", err.Error())
			}

			e.logger.Info("workshop.tool_complete", "tool", call.Function.Name, "entry_id", result.toolLogSeqEnd,
				"elapsed_ms", toolElapsed.Milliseconds(), "result_bytes", len(toolResult), "receipt_bytes", len(receipt))
			turnToolCount++
			turnToolMS += toolElapsed.Milliseconds()
			totalToolCalls++

			messages = append(messages, llm.ChatMessage{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    receipt,
			})

			if cfg.persistToConversation {
				toolResultEntry := conversationMessage{
					Type:      "tool_result",
					MessageID: fmt.Sprintf("tr-%d", time.Now().UnixNano()),
					Role:      "tool",
					Text:      receipt,
					CreatedAt: time.Now().UTC().Format(time.RFC3339),
					Metadata: map[string]any{
						"tool_call_id":   call.ID,
						"tool_name":      call.Function.Name,
						"success":        toolErr == nil,
						"tool_log_entry": result.toolLogSeqEnd,
					},
				}
				_ = e.appendConversation(cfg.workbenchID, toolResultEntry)
			}

			if e.notify != nil {
				e.notify("WorkshopToolComplete", map[string]any{
					"workbench_id": cfg.workbenchID,
					"tool_name":    call.Function.Name,
					"tool_call_id": call.ID,
					"success":      toolErr == nil,
				})
			}
		}

		e.logger.Info(logPrefix+"_turn_summary", "turn", turn, "tools_called", turnToolCount, "total_tool_ms", turnToolMS, "message_count", len(messages))
		result.turnCount = turn + 1
	}

	result.toolCallCount = totalToolCalls

	if finalAssistantText == "" && fullResponse.Len() == 0 {
		e.logger.Warn(logPrefix+"_max_turns_exhausted", "max_turns", cfg.maxTurns,
			"total_tool_calls", totalToolCalls, "total_elapsed_ms", time.Since(agentStartTime).Milliseconds())
		result.err = errinfo.AgentLoopDetected(errinfo.PhaseWorkshop,
			fmt.Sprintf("agent reached maximum turn limit (%d) without completing", cfg.maxTurns))
		return result
	}

	assistantText := fullResponse.String()
	if strings.TrimSpace(assistantText) == "" {
		assistantText = finalAssistantText
	}
	result.finalText = assistantText

	if cfg.persistToConversation {
		e.clearPendingClutter(cfg.workbenchID)
		entry := conversationMessage{
			Type:      "assistant_message",
			MessageID: assistantID,
			Role:      "assistant",
			Text:      assistantText,
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
		}
		if err := e.appendConversation(cfg.workbenchID, entry); err != nil {
			result.err = errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
			return result
		}
		if e.notify != nil {
			e.notify("WorkshopAssistantMessageComplete", map[string]any{
				"workbench_id": cfg.workbenchID,
				"message_id":   assistantID,
			})
		}
	}

	return result
}

func (e *Engine) logCodexUnauthorizedDiagnostics() {
	creds, err := e.secrets.GetOpenAICodexOAuthCredentials()
	if err != nil {
		e.logger.Warn("providers.oauth.codex_unauthorized_diag_failed", "error", err.Error())
		return
	}
	if creds == nil {
		e.logger.Warn("providers.oauth.codex_unauthorized_diag", "credentials_present", false)
		return
	}
	now := time.Now()
	if e.now != nil {
		now = e.now()
	}
	expiresAt := ""
	expired := false
	if !creds.ExpiresAt.IsZero() {
		expiresAt = creds.ExpiresAt.UTC().Format(time.RFC3339)
		expired = now.After(creds.ExpiresAt)
	}
	accessAccountPresent := false
	idAccountPresent := false
	if e.codexOAuth != nil {
		accessAccountPresent = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(creds.AccessToken)) != ""
		idAccountPresent = strings.TrimSpace(e.codexOAuth.ExtractChatGPTAccountID(creds.IDToken)) != ""
	}
	e.logger.Warn(
		"providers.oauth.codex_unauthorized_diag",
		"credentials_present", true,
		"has_access_token", strings.TrimSpace(creds.AccessToken) != "",
		"has_refresh_token", strings.TrimSpace(creds.RefreshToken) != "",
		"has_id_token", strings.TrimSpace(creds.IDToken) != "",
		"expires_at", expiresAt,
		"expired", expired,
		"access_account_present", accessAccountPresent,
		"id_account_present", idAccountPresent,
		"account_label_present", strings.TrimSpace(creds.AccountLabel) != "",
	)
}

// WorkshopRunAgent runs the Research → Plan → Implement workflow and then emits
// a single user-visible summary response.
func (e *Engine) WorkshopRunAgent(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		MessageID   string `json:"message_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	e.clearPendingClutter(req.WorkbenchID)
	defer e.clearPendingClutter(req.WorkbenchID)
	if err := e.ensureWorkshopUnlocked(req.WorkbenchID); err != nil {
		return nil, err
	}
	runCtx, runID, runErr := e.beginWorkshopRun(ctx, req.WorkbenchID)
	if runErr != nil {
		return nil, runErr
	}
	defer e.endWorkshopRun(req.WorkbenchID, runID)
	modelID, errInfo := e.resolveActiveModel(req.WorkbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	if err := e.ensureProviderReadyFor(runCtx, model.ProviderID); err != nil {
		return nil, err
	}
	if err := e.ensureConsent(req.WorkbenchID); err != nil {
		return nil, err
	}
	runStartedAt := time.Now()
	if e.now != nil {
		runStartedAt = e.now()
	}

	apiKey, errInfo := e.providerKey(runCtx, model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	client, errInfo := e.clientForProvider(model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}
	reasoningEffort, errInfo := e.loadProviderRPIReasoningEffort(model.ProviderID)
	if errInfo != nil {
		return nil, errInfo
	}

	userPrompt, errInfo := e.resolveRPIUserPrompt(req.WorkbenchID, req.MessageID)
	if errInfo != nil {
		return nil, errInfo
	}

	state := e.readRPIState(req.WorkbenchID)
	toolLogSeq := e.currentToolLogSeq(req.WorkbenchID)
	var focusHints map[string]map[string]any

	if !state.HasResearch {
		e.notifyPhase(req.WorkbenchID, "research")
		toolLogSeq, errInfo = e.runResearchPhase(
			withRPIReasoningEffortProfile(runCtx, model.ProviderID, reasoningEffort.ResearchEffort),
			req.WorkbenchID,
			userPrompt,
			client,
			apiKey,
			modelID,
			toolLogSeq,
		)
		if errInfo != nil {
			return nil, errInfo
		}
		e.notifyPhaseComplete(req.WorkbenchID, "research")
		state = e.readRPIState(req.WorkbenchID)
	}

	if state.HasResearch && !state.HasPlan {
		e.notifyPhase(req.WorkbenchID, "plan")
		toolLogSeq, errInfo = e.runPlanPhase(
			withRPIReasoningEffortProfile(runCtx, model.ProviderID, reasoningEffort.PlanEffort),
			req.WorkbenchID,
			userPrompt,
			client,
			apiKey,
			modelID,
			toolLogSeq,
		)
		if errInfo != nil {
			return nil, errInfo
		}
		e.notifyPhaseComplete(req.WorkbenchID, "plan")
		state = e.readRPIState(req.WorkbenchID)
	}

	if state.HasPlan && !state.AllDone {
		e.notifyPhase(req.WorkbenchID, "implement")
		var hints map[string]map[string]any
		toolLogSeq, hints, errInfo = e.runImplementPhase(
			withRPIReasoningEffortProfile(runCtx, model.ProviderID, reasoningEffort.ImplementEffort),
			req.WorkbenchID,
			userPrompt,
			client,
			apiKey,
			modelID,
			toolLogSeq,
		)
		if errInfo != nil {
			return nil, errInfo
		}
		if len(hints) > 0 {
			focusHints = hints
		}
		e.notifyPhaseComplete(req.WorkbenchID, "implement")
	}

	assistantID, summaryText, errInfo := e.runSummaryPhase(runCtx, req.WorkbenchID, client, apiKey, modelID)
	if errInfo != nil {
		return nil, errInfo
	}

	hasDraft := false
	if ds, _ := e.workbenches.DraftState(req.WorkbenchID); ds != nil {
		hasDraft = true
		now := time.Now()
		if e.now != nil {
			now = e.now()
		}
		elapsedMS := now.Sub(runStartedAt).Milliseconds()
		if elapsedMS < 0 {
			elapsedMS = 0
		}
		if err := e.updateConversationMetadata(req.WorkbenchID, assistantID, map[string]any{
			"job_elapsed_ms": elapsedMS,
		}); err != nil {
			return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
		e.notifyDraftState(req.WorkbenchID, ds)
		if strings.TrimSpace(summaryText) != "" {
			if err := e.writeDraftSummary(req.WorkbenchID, ds.DraftID, strings.TrimSpace(summaryText)); err != nil {
				return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
			}
		}
		if len(focusHints) > 0 {
			if err := e.writeProposalFocusHints(req.WorkbenchID, ds.DraftID, focusHints); err != nil {
				return nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
			}
		}
	}

	return map[string]any{
		"message_id": assistantID,
		"has_draft":  hasDraft,
	}, nil
}

func (e *Engine) runResearchPhase(ctx context.Context, workbenchID, userPrompt string, client LLMClient, apiKey, modelID string, toolLogSeqStart int) (int, *errinfo.ErrorInfo) {
	messages, errInfo := e.buildRPIResearchMessages(ctx, workbenchID, userPrompt)
	if errInfo != nil {
		return toolLogSeqStart, withSubphase(errInfo, errinfo.SubphaseRPIResearch)
	}
	result := e.runAgentLoop(ctx, agentLoopConfig{
		workbenchID:           workbenchID,
		client:                client,
		apiKey:                apiKey,
		modelID:               modelID,
		messages:              messages,
		tools:                 ResearchTools,
		maxTurns:              rpiResearchMaxTurns,
		handler:               NewToolHandler(e, workbenchID, ctx),
		phaseName:             "research",
		persistToConversation: false,
		emitStreamDeltas:      false,
		toolLogSeqStart:       toolLogSeqStart,
	})
	if result.err != nil {
		return result.toolLogSeqEnd, withSubphase(result.err, errinfo.SubphaseRPIResearch)
	}
	if err := e.writeRPIArtifact(workbenchID, rpiResearchFile, strings.TrimSpace(result.finalText)); err != nil {
		return result.toolLogSeqEnd, withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIResearch)
	}
	return result.toolLogSeqEnd, nil
}

func (e *Engine) runPlanPhase(ctx context.Context, workbenchID, userPrompt string, client LLMClient, apiKey, modelID string, toolLogSeqStart int) (int, *errinfo.ErrorInfo) {
	research, err := e.readRPIArtifact(workbenchID, rpiResearchFile)
	if err != nil {
		return toolLogSeqStart, withSubphase(errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIPlan)
	}
	messages, errInfo := e.buildRPIPlanMessages(ctx, workbenchID, strings.TrimSpace(research), userPrompt)
	if errInfo != nil {
		return toolLogSeqStart, withSubphase(errInfo, errinfo.SubphaseRPIPlan)
	}
	result := e.runAgentLoop(ctx, agentLoopConfig{
		workbenchID:           workbenchID,
		client:                client,
		apiKey:                apiKey,
		modelID:               modelID,
		messages:              messages,
		tools:                 PlanTools,
		maxTurns:              rpiPlanMaxTurns,
		handler:               NewToolHandler(e, workbenchID, ctx),
		phaseName:             "plan",
		persistToConversation: false,
		emitStreamDeltas:      false,
		toolLogSeqStart:       toolLogSeqStart,
	})
	if result.err != nil {
		return result.toolLogSeqEnd, withSubphase(result.err, errinfo.SubphaseRPIPlan)
	}
	planText := strings.TrimSpace(result.finalText)
	planCount := countPlanChecklistItems(planText)
	if planCount == 0 {
		return result.toolLogSeqEnd, withSubphase(
			errinfo.ValidationFailed(errinfo.PhaseWorkshop, "plan phase produced no actionable checklist items"),
			errinfo.SubphaseRPIPlan,
		)
	}
	planWithMeta := fmt.Sprintf("<!-- original_count: %d -->\n%s\n", planCount, planText)
	if err := e.writeRPIArtifact(workbenchID, rpiPlanFile, planWithMeta); err != nil {
		return result.toolLogSeqEnd, withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIPlan)
	}
	return result.toolLogSeqEnd, nil
}

func (e *Engine) runImplementPhase(ctx context.Context, workbenchID, userPrompt string, client LLMClient, apiKey, modelID string, toolLogSeqStart int) (int, map[string]map[string]any, *errinfo.ErrorInfo) {
	toolLogSeq := toolLogSeqStart
	handler := NewToolHandler(e, workbenchID, ctx)
	for {
		state := e.readRPIState(workbenchID)
		if !state.HasPlan || state.AllDone {
			break
		}

		currentIdx := -1
		var currentItem rpiPlanItem
		for i, item := range state.PlanItems {
			if item.Status != rpiStatusPending {
				continue
			}
			currentIdx = i
			currentItem = item
			break
		}
		if currentIdx < 0 {
			break
		}
		e.notifyImplementProgress(workbenchID, currentIdx+1, len(state.PlanItems), currentItem.Label)

		messages, errInfo := e.buildRPIImplementMessages(ctx, workbenchID, currentItem, userPrompt)
		if errInfo != nil {
			return toolLogSeq, handler.FocusHints(), withSubphase(errInfo, errinfo.SubphaseRPIImplement)
		}
		result := e.runAgentLoop(ctx, agentLoopConfig{
			workbenchID:           workbenchID,
			client:                client,
			apiKey:                apiKey,
			modelID:               modelID,
			messages:              messages,
			tools:                 WorkshopTools,
			maxTurns:              rpiItemMaxTurns,
			handler:               handler,
			phaseName:             "implement",
			persistToConversation: false,
			emitStreamDeltas:      false,
			toolLogSeqStart:       toolLogSeq,
		})
		toolLogSeq = result.toolLogSeqEnd
		if result.err != nil {
			if result.err.ErrorCode == errinfo.CodeUserCanceled || isRateLimitErrorInfo(result.err) {
				return toolLogSeq, handler.FocusHints(), withSubphase(result.err, errinfo.SubphaseRPIImplement)
			}
			reason := compactRPIErrorReason(result.err)
			retryMessages, retryErrInfo := e.buildRPIImplementRetryMessages(ctx, workbenchID, currentItem, userPrompt, reason)
			if retryErrInfo != nil {
				return toolLogSeq, handler.FocusHints(), withSubphase(retryErrInfo, errinfo.SubphaseRPIImplement)
			}
			retryResult := e.runAgentLoop(ctx, agentLoopConfig{
				workbenchID:           workbenchID,
				client:                client,
				apiKey:                apiKey,
				modelID:               modelID,
				messages:              retryMessages,
				tools:                 WorkshopTools,
				maxTurns:              rpiItemMaxTurns,
				handler:               handler,
				phaseName:             "implement",
				persistToConversation: false,
				emitStreamDeltas:      false,
				toolLogSeqStart:       toolLogSeq,
			})
			toolLogSeq = retryResult.toolLogSeqEnd
			if retryResult.err != nil {
				if retryResult.err.ErrorCode == errinfo.CodeUserCanceled || isRateLimitErrorInfo(retryResult.err) {
					return toolLogSeq, handler.FocusHints(), withSubphase(retryResult.err, errinfo.SubphaseRPIImplement)
				}
				failReason := compactRPIErrorReason(retryResult.err)
				if err := e.markPlanItem(workbenchID, currentIdx, rpiStatusFailed, failReason); err != nil {
					return toolLogSeq, handler.FocusHints(), withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIImplement)
				}
				continue
			}
			result = retryResult
		}

		if err := e.markPlanItem(workbenchID, currentIdx, rpiStatusDone, ""); err != nil {
			return toolLogSeq, handler.FocusHints(), withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIImplement)
		}

		newItems := extractNewPlanItems(result.finalText)
		if len(newItems) > 0 {
			if err := e.appendPlanItems(workbenchID, newItems); err != nil {
				return toolLogSeq, handler.FocusHints(), withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPIImplement)
			}
		}
	}
	return toolLogSeq, handler.FocusHints(), nil
}

func (e *Engine) runSummaryPhase(ctx context.Context, workbenchID string, client LLMClient, apiKey, modelID string) (string, string, *errinfo.ErrorInfo) {
	e.notifyPhase(workbenchID, "summary")
	messages, errInfo := e.buildRPISummaryMessages(ctx, workbenchID)
	if errInfo != nil {
		return "", "", withSubphase(errInfo, errinfo.SubphaseRPISummary)
	}

	providerID := ""
	if model, ok := getModel(modelID); ok {
		providerID = model.ProviderID
	}

	assistantID := fmt.Sprintf("a-%d", time.Now().UnixNano())
	var fullResponse strings.Builder
	resp, err := e.streamChatWithRateLimitRetry(
		ctx,
		workbenchID,
		client,
		apiKey,
		modelID,
		messages,
		func(delta string) {
			fullResponse.WriteString(delta)
			if e.addPendingClutter(workbenchID, delta) {
				e.emitClutterChanged(workbenchID)
			}
			if e.notify != nil {
				e.notify("WorkshopAssistantStreamDelta", map[string]any{
					"workbench_id": workbenchID,
					"message_id":   assistantID,
					"token_delta":  delta,
				})
			}
		},
		providerID,
		"summary",
	)
	if err != nil {
		return "", "", withSubphase(mapLLMError(errinfo.PhaseWorkshop, providerID, err), errinfo.SubphaseRPISummary)
	}
	if scope, err := e.workbenches.ComputeScopeHash(workbenchID); err == nil {
		e.appendEgressEvent(workbenchID, "workshop_rpi_summary", providerID, modelID, scope)
	}

	assistantText := fullResponse.String()
	if strings.TrimSpace(assistantText) == "" {
		assistantText = strings.TrimSpace(resp)
	}

	e.clearPendingClutter(workbenchID)
	entry := conversationMessage{
		Type:      "assistant_message",
		MessageID: assistantID,
		Role:      "assistant",
		Text:      assistantText,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := e.appendConversation(workbenchID, entry); err != nil {
		return "", "", withSubphase(errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error()), errinfo.SubphaseRPISummary)
	}

	if e.notify != nil {
		e.notify("WorkshopAssistantMessageComplete", map[string]any{
			"workbench_id": workbenchID,
			"message_id":   assistantID,
		})
	}
	e.notifyPhaseComplete(workbenchID, "summary")
	return assistantID, assistantText, nil
}

func (e *Engine) resolveRPIUserPrompt(workbenchID, messageID string) (string, *errinfo.ErrorInfo) {
	userMessageID, errInfo := e.resolveRegenerateUserMessage(workbenchID, messageID)
	if errInfo != nil {
		return "", errInfo
	}
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	for _, item := range items {
		if item.MessageID != userMessageID || item.Role != "user" {
			continue
		}
		return strings.TrimSpace(item.Text), nil
	}
	return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "user prompt not found")
}

func (e *Engine) buildRPIResearchMessages(ctx context.Context, workbenchID, userPrompt string) ([]llm.ChatMessage, *errinfo.ErrorInfo) {
	manifest, errInfo := e.buildRPIManifest(ctx, workbenchID, false)
	if errInfo != nil {
		return nil, errInfo
	}
	systemContent := RPIResearchSystemPrompt + "\n\n" + manifest
	if contextBlock, _ := e.buildWorkbenchContextInjection(workbenchID); contextBlock != "" {
		systemContent += "\n\nWorkbench context:\n" + contextBlock
	}
	prompt := strings.TrimSpace(userPrompt)
	if prompt == "" {
		prompt = "Analyze the available files and prepare research findings."
	}
	return []llm.ChatMessage{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: prompt},
	}, nil
}

func (e *Engine) buildRPIPlanMessages(ctx context.Context, workbenchID, research, userPrompt string) ([]llm.ChatMessage, *errinfo.ErrorInfo) {
	manifest, errInfo := e.buildRPIManifest(ctx, workbenchID, true)
	if errInfo != nil {
		return nil, errInfo
	}
	systemContent := RPIPlanSystemPrompt
	if contextBlock, _ := e.buildWorkbenchContextInjection(workbenchID); contextBlock != "" {
		systemContent += "\n\nWorkbench context:\n" + contextBlock
	}
	content := strings.TrimSpace(fmt.Sprintf(
		"User request:\n%s\n\nResearch summary:\n%s\n\nCurrent file manifest:\n%s",
		strings.TrimSpace(userPrompt),
		strings.TrimSpace(research),
		strings.TrimSpace(manifest),
	))
	return []llm.ChatMessage{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: content},
	}, nil
}

func (e *Engine) buildRPIImplementMessages(ctx context.Context, workbenchID string, item rpiPlanItem, userPrompt string) ([]llm.ChatMessage, *errinfo.ErrorInfo) {
	planContent, err := e.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	manifest, errInfo := e.buildRPIManifest(ctx, workbenchID, false)
	if errInfo != nil {
		return nil, errInfo
	}
	systemContent := fmt.Sprintf(RPIImplementSystemPrompt, item.RawLine, planContent)
	systemContent += "\n\nCurrent file manifest:\n" + manifest
	if contextBlock, _ := e.buildWorkbenchContextInjection(workbenchID); contextBlock != "" {
		systemContent += "\n\nWorkbench context:\n" + contextBlock
	}
	userContent := strings.TrimSpace(userPrompt)
	if userContent == "" {
		userContent = "Execute the current plan item."
	}
	return []llm.ChatMessage{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: userContent},
	}, nil
}

func (e *Engine) buildRPIImplementRetryMessages(ctx context.Context, workbenchID string, item rpiPlanItem, userPrompt, failureReason string) ([]llm.ChatMessage, *errinfo.ErrorInfo) {
	messages, errInfo := e.buildRPIImplementMessages(ctx, workbenchID, item, userPrompt)
	if errInfo != nil {
		return nil, errInfo
	}
	retryContext := "Previous attempt failed."
	if strings.TrimSpace(failureReason) != "" {
		retryContext = fmt.Sprintf("Previous attempt failed with: %s", strings.TrimSpace(failureReason))
	}
	retryContext += "\nRetry this same item with a different approach."
	messages = append(messages, llm.ChatMessage{Role: "user", Content: retryContext})
	return messages, nil
}

func (e *Engine) buildRPISummaryMessages(ctx context.Context, workbenchID string) ([]llm.Message, *errinfo.ErrorInfo) {
	planContent, err := e.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
		}
		planContent = "# Execution Plan\n\n(No plan artifact found.)"
	}
	manifest, errInfo := e.buildRPIManifest(ctx, workbenchID, true)
	if errInfo != nil {
		return nil, errInfo
	}
	systemContent := fmt.Sprintf(RPISummarySystemPrompt, strings.TrimSpace(planContent), strings.TrimSpace(manifest))
	return []llm.Message{
		{Role: "system", Content: systemContent},
		{Role: "user", Content: "Provide the final summary now."},
	}, nil
}

func (e *Engine) buildRPIManifest(ctx context.Context, workbenchID string, lightweight bool) (string, *errinfo.ErrorInfo) {
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, "failed to assemble file manifest: "+err.Error())
	}
	area := "published"
	if ds, _ := e.workbenches.DraftState(workbenchID); ds != nil {
		area = "draft"
	}

	var manifest strings.Builder
	manifest.WriteString("Current workbench files:\n\n")
	for _, f := range files {
		manifest.WriteString(fmt.Sprintf("## %s\n", f.Path))
		manifest.WriteString(fmt.Sprintf("- Type: %s, Size: %d bytes", f.FileKind, f.Size))
		if f.IsOpaque {
			manifest.WriteString(" (opaque)")
		}
		manifest.WriteString("\n")
		if lightweight {
			manifest.WriteString("\n")
			continue
		}

		switch f.FileKind {
		case workbench.FileKindXlsx, workbench.FileKindDocx, workbench.FileKindPptx, workbench.FileKindPdf:
			mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
			if mapResult != "" {
				manifest.WriteString("- Map:\n```json\n")
				manifest.WriteString(mapResult)
				manifest.WriteString("\n```\n")
			} else {
				manifest.WriteString("- Map: unavailable (use get_file_map tool to retrieve)\n")
			}
		case workbench.FileKindText:
			if isCSVWorkbenchPath(f.Path) {
				mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
				if mapResult != "" {
					manifest.WriteString("- Map:\n```json\n")
					manifest.WriteString(mapResult)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString("- CSV map unavailable (use table_get_map tool to retrieve)\n")
				}
				break
			}
			if f.Size <= smallTextFileThreshold {
				content, readErr := e.workbenches.ReadFile(workbenchID, area, f.Path)
				if readErr == nil {
					manifest.WriteString("- Content:\n```\n")
					manifest.WriteString(content)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString("- Content: unavailable\n")
				}
			} else {
				mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
				if mapResult != "" {
					manifest.WriteString("- Map:\n```json\n")
					manifest.WriteString(mapResult)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString(fmt.Sprintf("- Large text file (%d bytes). Use read_file with line_start/line_count.\n", f.Size))
				}
			}
		case workbench.FileKindImage:
			manifest.WriteString("- Image file (read-only, use get_file_info for metadata)\n")
		default:
			if f.IsOpaque {
				manifest.WriteString("- Opaque/binary file (content not directly readable)\n")
			} else {
				manifest.WriteString("- Use read_file to access content\n")
			}
		}
		manifest.WriteString("\n")
	}
	return manifest.String(), nil
}

func (e *Engine) notifyPhase(workbenchID, phase string) {
	if e.notify == nil {
		return
	}
	e.notify("WorkshopPhaseStarted", map[string]any{
		"workbench_id": workbenchID,
		"phase":        phase,
	})
}

func (e *Engine) notifyPhaseComplete(workbenchID, phase string) {
	if e.notify == nil {
		return
	}
	e.notify("WorkshopPhaseCompleted", map[string]any{
		"workbench_id": workbenchID,
		"phase":        phase,
	})
}

func (e *Engine) notifyImplementProgress(workbenchID string, current, total int, label string) {
	if e.notify == nil {
		return
	}
	e.notify("WorkshopImplementProgress", map[string]any{
		"workbench_id": workbenchID,
		"current_item": current,
		"total_items":  total,
		"item_label":   strings.TrimSpace(label),
	})
}

func compactRPIErrorReason(errInfo *errinfo.ErrorInfo) string {
	if errInfo == nil {
		return ""
	}
	reason := strings.TrimSpace(errInfo.Detail)
	if reason == "" {
		reason = strings.TrimSpace(errInfo.ErrorCode)
	}
	reason = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(reason, "\n", " "), "\r", " "))
	if len(reason) > 220 {
		reason = reason[:220] + "..."
	}
	return reason
}

func countPlanChecklistItems(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	count := 0
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if rpiPlanItemLinePattern.MatchString(line) {
			count++
		}
	}
	return count
}

// buildAgentMessages creates the initial messages for the agentic workshop.
// It builds a map-first context with structural maps for each file.
func (e *Engine) buildAgentMessages(ctx context.Context, workbenchID string) ([]llm.ChatMessage, *errinfo.ErrorInfo) {
	// Get file manifest - fail closed if manifest cannot be assembled
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, "failed to assemble file manifest: "+err.Error())
	}

	area := "published"
	if ds, _ := e.workbenches.DraftState(workbenchID); ds != nil {
		area = "draft"
	}

	// Build manifest with structural maps
	var manifest strings.Builder
	manifest.WriteString("Current workbench files:\n\n")
	for _, f := range files {
		manifest.WriteString(fmt.Sprintf("## %s\n", f.Path))
		manifest.WriteString(fmt.Sprintf("- Type: %s, Size: %d bytes", f.FileKind, f.Size))
		if f.IsOpaque {
			manifest.WriteString(" (opaque)")
		}
		manifest.WriteString("\n")

		// Include structural map per file type
		switch f.FileKind {
		case workbench.FileKindXlsx, workbench.FileKindDocx, workbench.FileKindPptx, workbench.FileKindPdf:
			mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
			if mapResult != "" {
				manifest.WriteString("- Map:\n```json\n")
				manifest.WriteString(mapResult)
				manifest.WriteString("\n```\n")
			} else {
				manifest.WriteString("- Map: unavailable (use get_file_map tool to retrieve)\n")
			}

		case workbench.FileKindText:
			if isCSVWorkbenchPath(f.Path) {
				mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
				if mapResult != "" {
					manifest.WriteString("- Map:\n```json\n")
					manifest.WriteString(mapResult)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString("- CSV map unavailable (use table_get_map tool to retrieve)\n")
				}
				break
			}
			// For small text files, inline content; for large ones, include map
			if f.Size <= smallTextFileThreshold {
				content, err := e.workbenches.ReadFile(workbenchID, area, f.Path)
				if err == nil {
					manifest.WriteString("- Content:\n```\n")
					manifest.WriteString(content)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString("- Content: unavailable\n")
				}
			} else {
				mapResult := e.getFileMapForContext(ctx, workbenchID, area, f.FileKind, f.Path)
				if mapResult != "" {
					manifest.WriteString("- Map:\n```json\n")
					manifest.WriteString(mapResult)
					manifest.WriteString("\n```\n")
				} else {
					manifest.WriteString(fmt.Sprintf("- Large text file (%d bytes). Use read_file with line_start/line_count.\n", f.Size))
				}
			}

		case workbench.FileKindImage:
			manifest.WriteString("- Image file (read-only, use get_file_info for metadata)\n")

		default:
			if f.IsOpaque {
				manifest.WriteString("- Opaque/binary file (content not directly readable)\n")
			} else {
				manifest.WriteString("- Use read_file to access content\n")
			}
		}
		manifest.WriteString("\n")
	}

	systemContent := AgentSystemPrompt + "\n\n" + manifest.String()
	if contextBlock, _ := e.buildWorkbenchContextInjection(workbenchID); contextBlock != "" {
		systemContent += "\n\nWorkbench context:\n" + contextBlock
	}

	// Get conversation history
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}

	messages := []llm.ChatMessage{
		{Role: "system", Content: systemContent},
	}

	// Add conversation history including tool calls and tool results
	for _, item := range items {
		switch {
		case item.Type == "tool_result" && item.Metadata != nil:
			toolCallID, _ := item.Metadata["tool_call_id"].(string)
			if toolCallID != "" {
				text := item.Text
				// Backward compat: truncate large pre-migration tool results
				if len(text) > receiptSizeThreshold {
					text = truncateHistoricalToolResult(text)
				}
				messages = append(messages, llm.ChatMessage{
					Role:       "tool",
					ToolCallID: toolCallID,
					Content:    text,
				})
			}
		case item.Role == "assistant" && item.Metadata != nil:
			msg := llm.ChatMessage{
				Role:    "assistant",
				Content: item.Text,
			}
			// Restore tool_calls from metadata if present
			if tcRaw, ok := item.Metadata["tool_calls"]; ok {
				if tcJSON, err := json.Marshal(tcRaw); err == nil {
					var toolCalls []llm.ToolCall
					if err := json.Unmarshal(tcJSON, &toolCalls); err == nil {
						msg.ToolCalls = toolCalls
					}
				}
			}
			messages = append(messages, msg)
		case item.Role == "user" || item.Role == "assistant":
			messages = append(messages, llm.ChatMessage{
				Role:    item.Role,
				Content: item.Text,
			})
		}
	}

	return messages, nil
}

// getFileMapForContext retrieves the structural map for a file for inclusion in
// the agent's initial context. Returns empty string on failure (degraded to placeholder).
func (e *Engine) getFileMapForContext(ctx context.Context, workbenchID, area, kind, path string) string {
	if e.toolWorker == nil {
		return ""
	}
	method := ""
	switch kind {
	case workbench.FileKindXlsx:
		method = "XlsxGetMap"
	case workbench.FileKindDocx:
		method = "DocxGetMap"
	case workbench.FileKindPptx:
		method = "PptxGetMap"
	case workbench.FileKindPdf:
		method = "PdfGetMap"
	case workbench.FileKindText:
		if isCSVWorkbenchPath(path) {
			method = "TabularGetMap"
		} else {
			method = "TextGetMap"
		}
	default:
		return ""
	}
	params := map[string]any{
		"workbench_id": workbenchID,
		"path":         path,
		"root":         area,
	}
	var resp json.RawMessage
	if err := e.toolWorker.Call(ctx, method, params, &resp); err != nil {
		e.logger.Warn("workshop.map_context_failed", "path", path, "kind", kind, "error", err.Error())
		return ""
	}
	return string(resp)
}

func (e *Engine) ReviewGetChangeSet(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	draft, err := e.workbenches.DraftState(req.WorkbenchID)
	if err != nil || draft == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "draft not found")
	}
	changes, err := e.workbenches.ChangeSet(req.WorkbenchID)
	if err != nil {
		if errors.Is(err, workbench.ErrDeletionDetected) {
			return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "deletions are not allowed in draft")
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	manifestFiles, _ := e.workbenches.FilesList(req.WorkbenchID)
	manifestIndex := make(map[string]workbench.FileEntry, len(manifestFiles))
	for _, file := range manifestFiles {
		manifestIndex[strings.ToLower(file.Path)] = file
	}
	type changeItem struct {
		Path        string         `json:"path"`
		ChangeType  string         `json:"change_type"`
		FileKind    string         `json:"file_kind"`
		PreviewKind string         `json:"preview_kind"`
		MimeType    string         `json:"mime_type"`
		IsOpaque    bool           `json:"is_opaque"`
		FocusHint   map[string]any `json:"focus_hint,omitempty"`
		Summary     string         `json:"summary,omitempty"`
		SizeBytes   int64          `json:"size_bytes,omitempty"`
	}
	var enriched []changeItem
	for _, change := range changes {
		item := changeItem{Path: change.Path, ChangeType: change.ChangeType}
		file, ok := manifestIndex[strings.ToLower(change.Path)]
		if ok {
			item.FileKind = file.FileKind
			item.MimeType = file.MimeType
			item.IsOpaque = file.IsOpaque
		} else {
			kind, opaque := workbench.FileKindForPath(change.Path)
			item.FileKind = kind
			item.IsOpaque = opaque
			item.MimeType = mimeTypeForPath(change.Path)
		}
		item.PreviewKind = previewKindForFile(item.FileKind, item.IsOpaque)
		if info, err := e.workbenches.StatFile(req.WorkbenchID, "draft", change.Path); err == nil {
			item.SizeBytes = info.Size()
		}
		if draft != nil {
			if summary, err := e.readProposalSummary(req.WorkbenchID, draft.DraftID, change.Path); err == nil {
				item.Summary = summary
			}
			if hint, err := e.readFocusHint(req.WorkbenchID, draft.DraftID, change.Path); err == nil && hint != nil {
				item.FocusHint = hint
			}
		}
		enriched = append(enriched, item)
	}
	e.logger.Debug("review.get_change_set", "workbench_id", req.WorkbenchID, "count", len(enriched))
	resp := map[string]any{"draft_id": draft.DraftID, "changes": enriched}
	if draftSummary, err := e.readDraftSummary(req.WorkbenchID, draft.DraftID); err == nil && strings.TrimSpace(draftSummary) != "" {
		resp["draft_summary"] = draftSummary
	}
	return resp, nil
}

func (e *Engine) ReviewGetTextDiff(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Path        string `json:"path"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	draft, err := e.workbenches.DraftState(req.WorkbenchID)
	if err != nil || draft == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "draft not found")
	}
	files, _ := e.workbenches.FilesList(req.WorkbenchID)
	var entry *workbench.FileEntry
	for i := range files {
		if strings.EqualFold(files[i].Path, req.Path) {
			entry = &files[i]
			break
		}
	}
	fileKind := ""
	isOpaque := false
	if entry != nil {
		fileKind = entry.FileKind
		isOpaque = entry.IsOpaque
	}
	if fileKind == "" {
		fileKind, isOpaque = workbench.FileKindForPath(req.Path)
	}
	if isOpaque || fileKind == workbench.FileKindBinary || fileKind == workbench.FileKindImage {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "no diff available for this file type")
	}

	if fileKind == workbench.FileKindText {
		before, err := e.workbenches.ReadFile(req.WorkbenchID, "published", req.Path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				before = ""
			} else if errors.Is(err, workbench.ErrInvalidPath) {
				return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
			} else if errors.Is(err, workbench.ErrSandboxViolation) {
				return nil, errinfo.SandboxViolation(errinfo.PhaseReview, err.Error())
			} else {
				return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
			}
		}
		after, err := e.workbenches.ReadFile(req.WorkbenchID, "draft", req.Path)
		if err != nil {
			if errors.Is(err, workbench.ErrInvalidPath) {
				return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
			}
			if errors.Is(err, workbench.ErrSandboxViolation) {
				return nil, errinfo.SandboxViolation(errinfo.PhaseReview, err.Error())
			}
			return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
		}
		hunks, tooLarge := diff.TextDiffWithLimit(before, after, diff.MaxDiffLines)
		return map[string]any{"hunks": hunks, "too_large": tooLarge}, nil
	}

	text, err := e.extractText(ctx, req.WorkbenchID, "draft", fileKind, req.Path)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	before, hasBaseline, err := e.readBaselineText(req.WorkbenchID, draft.DraftID, req.Path)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	if hasBaseline {
		hunks, tooLarge := diff.TextDiffWithLimit(before, text, diff.MaxDiffLines)
		return map[string]any{
			"hunks":            hunks,
			"too_large":        tooLarge,
			"baseline_missing": false,
			"reference_source": "draft_start_snapshot",
		}, nil
	}

	publishedExists, errInfo := e.publishedFileExists(req.WorkbenchID, req.Path)
	if errInfo != nil {
		return nil, errInfo
	}
	if !publishedExists {
		return map[string]any{
			"hunks":             []diff.Hunk{},
			"too_large":         false,
			"baseline_missing":  true,
			"reference_source":  "none",
			"reference_warning": reviewReferenceWarningUnavailable,
		}, nil
	}
	publishedText, err := e.extractText(ctx, req.WorkbenchID, "published", fileKind, req.Path)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			if errInfo.ErrorCode == errinfo.CodeValidationFailed || errInfo.ErrorCode == errinfo.CodeFileReadFailed {
				return map[string]any{
					"hunks":             []diff.Hunk{},
					"too_large":         false,
					"baseline_missing":  true,
					"reference_source":  "none",
					"reference_warning": reviewReferenceWarningUnavailable,
				}, nil
			}
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	hunks, tooLarge := diff.TextDiffWithLimit(publishedText, text, diff.MaxDiffLines)
	return map[string]any{
		"hunks":             hunks,
		"too_large":         tooLarge,
		"baseline_missing":  false,
		"reference_source":  "published_current_fallback",
		"reference_warning": reviewReferenceWarningPublishedFallback,
	}, nil
}

type docxSectionContent struct {
	SectionIndex int            `json:"section_index"`
	SectionCount int            `json:"section_count"`
	Section      map[string]any `json:"section"`
}

type pptxSlideContent struct {
	SlideIndex int            `json:"slide_index"`
	SlideCount int            `json:"slide_count"`
	Slide      map[string]any `json:"slide"`
}

func (e *Engine) ReviewGetDocxContentDiff(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID  string `json:"workbench_id"`
		Path         string `json:"path"`
		SectionIndex *int   `json:"section_index"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	sectionIndex := 0
	if req.SectionIndex != nil {
		sectionIndex = *req.SectionIndex
	}
	if sectionIndex < 0 {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid section_index")
	}

	draftContent, err := e.readDocxSectionContent(ctx, req.WorkbenchID, req.Path, "draft", sectionIndex)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}

	draft, err := e.workbenches.DraftState(req.WorkbenchID)
	if err != nil || draft == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "draft not found")
	}
	referenceSource := "draft_start_snapshot"
	referenceWarning := ""
	_, hasBaseline, err := e.readBaselineText(req.WorkbenchID, draft.DraftID, req.Path)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	if !hasBaseline {
		referenceSource = "published_current_fallback"
		referenceWarning = reviewReferenceWarningPublishedFallback
	}

	publishedExists, errInfo := e.publishedFileExists(req.WorkbenchID, req.Path)
	if errInfo != nil {
		return nil, errInfo
	}
	if !publishedExists {
		return map[string]any{
			"baseline":          nil,
			"draft":             draftContent.Section,
			"section_count":     draftContent.SectionCount,
			"baseline_missing":  true,
			"reference_source":  "none",
			"reference_warning": reviewReferenceWarningUnavailable,
		}, nil
	}

	baselineContent, err := e.readDocxSectionContent(ctx, req.WorkbenchID, req.Path, "published", sectionIndex)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			if errInfo.ErrorCode == errinfo.CodeValidationFailed || errInfo.ErrorCode == errinfo.CodeFileReadFailed {
				return map[string]any{
					"baseline":          nil,
					"draft":             draftContent.Section,
					"section_count":     draftContent.SectionCount,
					"baseline_missing":  true,
					"reference_source":  "none",
					"reference_warning": reviewReferenceWarningUnavailable,
				}, nil
			}
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}

	sectionCount := draftContent.SectionCount
	if baselineContent.SectionCount > sectionCount {
		sectionCount = baselineContent.SectionCount
	}
	resp := map[string]any{
		"baseline":         baselineContent.Section,
		"draft":            draftContent.Section,
		"section_count":    sectionCount,
		"baseline_missing": false,
		"reference_source": referenceSource,
	}
	if referenceWarning != "" {
		resp["reference_warning"] = referenceWarning
	}
	return resp, nil
}

func (e *Engine) ReviewGetPptxContentDiff(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Path        string `json:"path"`
		SlideIndex  *int   `json:"slide_index"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	slideIndex := 0
	if req.SlideIndex != nil {
		slideIndex = *req.SlideIndex
	}
	if slideIndex < 0 {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid slide_index")
	}

	draftContent, err := e.readPptxSlideContent(ctx, req.WorkbenchID, req.Path, "draft", slideIndex)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}

	draft, err := e.workbenches.DraftState(req.WorkbenchID)
	if err != nil || draft == nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "draft not found")
	}
	referenceSource := "draft_start_snapshot"
	referenceWarning := ""
	_, hasBaseline, err := e.readBaselineText(req.WorkbenchID, draft.DraftID, req.Path)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	if !hasBaseline {
		referenceSource = "published_current_fallback"
		referenceWarning = reviewReferenceWarningPublishedFallback
	}

	publishedExists, errInfo := e.publishedFileExists(req.WorkbenchID, req.Path)
	if errInfo != nil {
		return nil, errInfo
	}
	if !publishedExists {
		return map[string]any{
			"baseline":          nil,
			"draft":             draftContent.Slide,
			"slide_count":       draftContent.SlideCount,
			"baseline_missing":  true,
			"reference_source":  "none",
			"reference_warning": reviewReferenceWarningUnavailable,
		}, nil
	}

	baselineContent, err := e.readPptxSlideContent(ctx, req.WorkbenchID, req.Path, "published", slideIndex)
	if err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			if errInfo.ErrorCode == errinfo.CodeValidationFailed || errInfo.ErrorCode == errinfo.CodeFileReadFailed {
				return map[string]any{
					"baseline":          nil,
					"draft":             draftContent.Slide,
					"slide_count":       draftContent.SlideCount,
					"baseline_missing":  true,
					"reference_source":  "none",
					"reference_warning": reviewReferenceWarningUnavailable,
				}, nil
			}
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}

	slideCount := draftContent.SlideCount
	if baselineContent.SlideCount > slideCount {
		slideCount = baselineContent.SlideCount
	}
	resp := map[string]any{
		"baseline":         baselineContent.Slide,
		"draft":            draftContent.Slide,
		"slide_count":      slideCount,
		"baseline_missing": false,
		"reference_source": referenceSource,
	}
	if referenceWarning != "" {
		resp["reference_warning"] = referenceWarning
	}
	return resp, nil
}

func (e *Engine) ReviewGetPdfPreviewPage(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	return e.reviewPreviewPage(ctx, params, "PdfRenderPage")
}

func (e *Engine) ReviewGetDocxPreviewPage(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	return e.reviewPreviewPage(ctx, params, "DocxRenderPage")
}

func (e *Engine) ReviewGetOdtPreviewPage(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	return e.reviewPreviewPage(ctx, params, "OdtRenderPage")
}

func (e *Engine) ReviewGetPptxPreviewSlide(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string  `json:"workbench_id"`
		Path        string  `json:"path"`
		Version     string  `json:"version"`
		SlideIndex  int     `json:"slide_index"`
		Scale       float64 `json:"scale"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	root, err := normalizePreviewRoot(req.Version)
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
	}
	scale := clampPreviewScale(req.Scale)
	payload := map[string]any{
		"workbench_id": req.WorkbenchID,
		"path":         req.Path,
		"root":         root,
		"slide_index":  req.SlideIndex,
		"scale":        scale,
	}
	var resp struct {
		BytesBase64 string `json:"bytes_base64"`
		SlideCount  int    `json:"slide_count"`
		MimeType    string `json:"mime_type"`
		ScaledDown  bool   `json:"scaled_down"`
	}
	if e.toolWorker == nil {
		return nil, errinfo.ToolWorkerUnavailable(errinfo.PhaseReview, "tool worker unavailable")
	}
	if err := e.toolWorker.Call(ctx, "PptxRenderSlide", payload, &resp); err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	return map[string]any{
		"bytes_base64": resp.BytesBase64,
		"slide_count":  resp.SlideCount,
		"mime_type":    resp.MimeType,
		"scaled_down":  resp.ScaledDown,
	}, nil
}

func (e *Engine) ReviewGetXlsxPreviewGrid(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Path        string `json:"path"`
		Version     string `json:"version"`
		Sheet       string `json:"sheet"`
		RowStart    int    `json:"row_start"`
		RowCount    int    `json:"row_count"`
		ColStart    int    `json:"col_start"`
		ColCount    int    `json:"col_count"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	root, err := normalizePreviewRoot(req.Version)
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
	}
	rowCount := clampGridCount(req.RowCount, 200)
	colCount := clampGridCount(req.ColCount, 50)
	payload := map[string]any{
		"workbench_id": req.WorkbenchID,
		"path":         req.Path,
		"root":         root,
		"sheet":        req.Sheet,
		"row_start":    req.RowStart,
		"row_count":    rowCount,
		"col_start":    req.ColStart,
		"col_count":    colCount,
	}
	var resp struct {
		Sheets   []string           `json:"sheets"`
		RowCount int                `json:"row_count"`
		ColCount int                `json:"col_count"`
		Cells    [][]map[string]any `json:"cells"`
	}
	if e.toolWorker == nil {
		return nil, errinfo.ToolWorkerUnavailable(errinfo.PhaseReview, "tool worker unavailable")
	}
	if err := e.toolWorker.Call(ctx, "XlsxRenderGrid", payload, &resp); err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	return map[string]any{
		"sheets":    resp.Sheets,
		"row_count": resp.RowCount,
		"col_count": resp.ColCount,
		"cells":     resp.Cells,
	}, nil
}

func (e *Engine) ReviewGetImagePreview(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
		Path        string `json:"path"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	if e.toolWorker == nil {
		return nil, errinfo.ToolWorkerUnavailable(errinfo.PhaseReview, "tool worker unavailable")
	}
	draftPayload := map[string]any{
		"workbench_id": req.WorkbenchID,
		"path":         req.Path,
		"root":         "draft",
	}
	var draftRender struct {
		BytesBase64 string `json:"bytes_base64"`
		MimeType    string `json:"mime_type"`
		ScaledDown  bool   `json:"scaled_down"`
	}
	var draftMeta map[string]any
	if err := e.toolWorker.Call(ctx, "ImageRender", draftPayload, &draftRender); err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	if err := e.toolWorker.Call(ctx, "ImageGetMetadata", draftPayload, &draftMeta); err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	result := map[string]any{
		"draft": map[string]any{
			"mime_type":    draftRender.MimeType,
			"bytes_base64": draftRender.BytesBase64,
			"metadata":     draftMeta,
		},
		"has_published": false,
	}

	_, err := e.workbenches.ReadFileBytes(req.WorkbenchID, "published", req.Path)
	if err == nil {
		publishedPayload := map[string]any{
			"workbench_id": req.WorkbenchID,
			"path":         req.Path,
			"root":         "published",
		}
		var pubRender struct {
			BytesBase64 string `json:"bytes_base64"`
			MimeType    string `json:"mime_type"`
			ScaledDown  bool   `json:"scaled_down"`
		}
		var pubMeta map[string]any
		if err := e.toolWorker.Call(ctx, "ImageRender", publishedPayload, &pubRender); err != nil {
			if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
				return nil, errInfo
			}
			return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
		}
		if err := e.toolWorker.Call(ctx, "ImageGetMetadata", publishedPayload, &pubMeta); err != nil {
			if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
				return nil, errInfo
			}
			return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
		}
		result["published"] = map[string]any{
			"mime_type":    pubRender.MimeType,
			"bytes_base64": pubRender.BytesBase64,
			"metadata":     pubMeta,
		}
		result["has_published"] = true
	} else if errors.Is(err, workbench.ErrInvalidPath) {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
	} else if errors.Is(err, workbench.ErrSandboxViolation) {
		return nil, errinfo.SandboxViolation(errinfo.PhaseReview, err.Error())
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	return result, nil
}

func (e *Engine) reviewPreviewPage(ctx context.Context, params json.RawMessage, method string) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string  `json:"workbench_id"`
		Path        string  `json:"path"`
		Version     string  `json:"version"`
		PageIndex   int     `json:"page_index"`
		Scale       float64 `json:"scale"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, "invalid params")
	}
	if errInfo := e.ensureDraftExists(req.WorkbenchID); errInfo != nil {
		return nil, errInfo
	}
	root, err := normalizePreviewRoot(req.Version)
	if err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
	}
	scale := clampPreviewScale(req.Scale)
	payload := map[string]any{
		"workbench_id": req.WorkbenchID,
		"path":         req.Path,
		"root":         root,
		"page_index":   req.PageIndex,
		"scale":        scale,
	}
	if e.toolWorker == nil {
		return nil, errinfo.ToolWorkerUnavailable(errinfo.PhaseReview, "tool worker unavailable")
	}
	var resp struct {
		BytesBase64 string `json:"bytes_base64"`
		PageCount   int    `json:"page_count"`
		MimeType    string `json:"mime_type"`
		ScaledDown  bool   `json:"scaled_down"`
	}
	if err := e.toolWorker.Call(ctx, method, payload, &resp); err != nil {
		if errInfo := mapToolWorkerError(errinfo.PhaseReview, err); errInfo != nil {
			return nil, errInfo
		}
		return nil, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
	}
	return map[string]any{
		"bytes_base64": resp.BytesBase64,
		"page_count":   resp.PageCount,
		"mime_type":    resp.MimeType,
		"scaled_down":  resp.ScaledDown,
	}, nil
}

func (e *Engine) readDocxSectionContent(ctx context.Context, workbenchID, path, root string, sectionIndex int) (*docxSectionContent, error) {
	if e.toolWorker == nil {
		return nil, toolworker.ErrUnavailable
	}
	params := map[string]any{
		"workbench_id":  workbenchID,
		"path":          path,
		"root":          root,
		"section_index": sectionIndex,
	}
	var resp docxSectionContent
	if err := e.toolWorker.Call(ctx, "DocxGetSectionContent", params, &resp); err != nil {
		return nil, err
	}
	if resp.Section == nil {
		resp.Section = map[string]any{}
	}
	return &resp, nil
}

func (e *Engine) readPptxSlideContent(ctx context.Context, workbenchID, path, root string, slideIndex int) (*pptxSlideContent, error) {
	if e.toolWorker == nil {
		return nil, toolworker.ErrUnavailable
	}
	params := map[string]any{
		"workbench_id": workbenchID,
		"path":         path,
		"root":         root,
		"slide_index":  slideIndex,
		"detail":       "positioned",
	}
	var resp pptxSlideContent
	if err := e.toolWorker.Call(ctx, "PptxGetSlideContent", params, &resp); err != nil {
		if !shouldFallbackPptxSlideLegacy(err) {
			return nil, err
		}
		legacyParams := map[string]any{
			"workbench_id": workbenchID,
			"path":         path,
			"root":         root,
			"slide_index":  slideIndex,
		}
		if legacyErr := e.toolWorker.Call(ctx, "PptxGetSlideContent", legacyParams, &resp); legacyErr != nil {
			return nil, legacyErr
		}
	}
	if resp.Slide == nil {
		resp.Slide = map[string]any{}
	}
	return &resp, nil
}

func shouldFallbackPptxSlideLegacy(err error) bool {
	if err == nil {
		return false
	}
	var remote *toolworker.RemoteError
	if !errors.As(err, &remote) {
		return false
	}
	if remote.Code != errinfo.CodeValidationFailed {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(remote.Message))
	if strings.Contains(msg, "detail") || strings.Contains(msg, "positioned") || strings.Contains(msg, "unsupported") {
		return true
	}
	return false
}

func (e *Engine) ensureDraftExists(workbenchID string) *errinfo.ErrorInfo {
	draft, err := e.workbenches.DraftState(workbenchID)
	if err != nil || draft == nil {
		return errinfo.ValidationFailed(errinfo.PhaseReview, "draft not found")
	}
	return nil
}

func normalizePreviewRoot(version string) (string, error) {
	switch strings.ToLower(version) {
	case "", "draft":
		return "draft", nil
	case "published":
		return "published", nil
	default:
		return "", fmt.Errorf("invalid version")
	}
}

func clampPreviewScale(scale float64) float64 {
	if scale <= 0 {
		scale = 1.0
	}
	if scale < 0.25 {
		return 0.25
	}
	if scale > 2.0 {
		return 2.0
	}
	return scale
}

func clampGridCount(value int, max int) int {
	if value <= 0 {
		return max
	}
	if value > max {
		return max
	}
	return value
}

func (e *Engine) DraftGetState(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid params")
	}
	draft, err := e.workbenches.DraftState(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if draft == nil {
		return map[string]any{"has_draft": false}, nil
	}
	e.logger.Debug("draft.get_state", "workbench_id", req.WorkbenchID, "draft_id", draft.DraftID)
	resp := map[string]any{
		"has_draft":  true,
		"draft_id":   draft.DraftID,
		"created_at": draft.CreatedAt,
	}
	source := map[string]any{}
	if draft.SourceKind != "" {
		resp["source_kind"] = draft.SourceKind
		source["kind"] = draft.SourceKind
	}
	if draft.SourceRef != "" {
		resp["source_ref"] = draft.SourceRef
		source["ref"] = draft.SourceRef
		source["job_id"] = draft.SourceRef
	}
	if len(source) > 0 {
		resp["source"] = source
	}
	return resp, nil
}

func (e *Engine) DraftPublish(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhasePublish, "invalid params")
	}
	checkpointID, err := e.workbenches.CheckpointCreate(req.WorkbenchID, "publish", "Before publish")
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhasePublish, err.Error())
	}
	checkpointCreatedAt := time.Now().UTC().Format(time.RFC3339)
	if meta, err := e.workbenches.CheckpointGet(req.WorkbenchID, checkpointID); err == nil && meta != nil && meta.CreatedAt != "" {
		checkpointCreatedAt = meta.CreatedAt
	}
	publishedAt, err := e.workbenches.PublishDraft(req.WorkbenchID)
	if err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhasePublish, err.Error())
	}
	e.appendCheckpointConversationEvent(req.WorkbenchID, conversationMessage{
		Type:         "system_event",
		MessageID:    fmt.Sprintf("s-%d", time.Now().UnixNano()),
		Role:         "system",
		Text:         "Created publish checkpoint.",
		CreatedAt:    checkpointCreatedAt,
		EventKind:    "checkpoint_publish",
		CheckpointID: checkpointID,
		Reason:       "publish",
		Timestamp:    checkpointCreatedAt,
	})
	e.logger.Info("draft.publish", "workbench_id", req.WorkbenchID, "published_at", publishedAt.Format(time.RFC3339))
	if e.notify != nil {
		e.notify("CheckpointCreated", map[string]any{
			"workbench_id":  req.WorkbenchID,
			"checkpoint_id": checkpointID,
			"reason":        "publish",
			"created_at":    checkpointCreatedAt,
			"description":   "Before publish",
		})
	}
	e.notifyDraftState(req.WorkbenchID, nil)
	stateInfo, _ := e.readWorkshopState(req.WorkbenchID)
	stateInfo.PendingProposalID = ""
	_ = e.writeWorkshopState(req.WorkbenchID, stateInfo)
	return map[string]any{
		"published_at":  publishedAt.Format(time.RFC3339),
		"checkpoint_id": checkpointID,
	}, nil
}

func (e *Engine) DraftDiscard(ctx context.Context, params json.RawMessage) (any, *errinfo.ErrorInfo) {
	var req struct {
		WorkbenchID string `json:"workbench_id"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhasePublish, "invalid params")
	}
	if err := e.workbenches.DiscardDraft(req.WorkbenchID); err != nil {
		return nil, errinfo.FileWriteFailed(errinfo.PhasePublish, err.Error())
	}
	e.logger.Info("draft.discard", "workbench_id", req.WorkbenchID)
	e.notifyDraftState(req.WorkbenchID, nil)
	stateInfo, _ := e.readWorkshopState(req.WorkbenchID)
	stateInfo.PendingProposalID = ""
	_ = e.writeWorkshopState(req.WorkbenchID, stateInfo)
	return map[string]any{}, nil
}

func (e *Engine) ensureConsent(workbenchID string) *errinfo.ErrorInfo {
	scope, err := e.workbenches.ComputeScopeHash(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	modelID, errInfo := e.resolveActiveModel(workbenchID)
	if errInfo != nil {
		return errInfo
	}
	model, ok := getModel(modelID)
	if !ok {
		return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported model")
	}
	consent, err := e.workbenches.ReadConsent(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	persisted := consent.Workshop.ProviderID == model.ProviderID && consent.Workshop.ModelID == modelID && consent.Workshop.ScopeHash == scope
	session := e.sessionConsent[workbenchID]
	sessionOk := session.ProviderID == model.ProviderID && session.ModelID == modelID && session.ScopeHash == scope
	if !persisted && !sessionOk {
		errInfo := errinfo.EgressConsentRequired(errinfo.PhaseWorkshop, model.ProviderID, scope)
		errInfo.ModelID = modelID
		return errInfo
	}
	return nil
}

func (e *Engine) ensureWorkshopUnlocked(workbenchID string) *errinfo.ErrorInfo {
	draft, err := e.workbenches.DraftState(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if draft != nil {
		return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "draft exists; review or discard before continuing")
	}
	return nil
}

func (e *Engine) undoWorkshopToMessage(workbenchID, messageID string) (string, *workbench.DraftState, *errinfo.ErrorInfo) {
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return "", nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	targetIndex := findConversationMessageIndex(items, messageID)
	if targetIndex < 0 {
		return "", nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "message not found")
	}
	if checkpointID := rewindPublishedCheckpointID(items, targetIndex); checkpointID != "" {
		preRestoreID, err := e.workbenches.CheckpointCreate(workbenchID, "pre_restore", "Before rewind restore")
		if err != nil {
			return "", nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
		if err := e.workbenches.CheckpointRestorePublished(workbenchID, checkpointID); err != nil {
			return "", nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
		}
		e.logger.Info(
			"workshop.undo_restore_published",
			"workbench_id",
			workbenchID,
			"message_id",
			messageID,
			"checkpoint_id",
			checkpointID,
			"pre_restore_checkpoint_id",
			preRestoreID,
		)
	}
	if err := e.writeConversation(workbenchID, items[:targetIndex+1]); err != nil {
		return "", nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	revisionID, err := e.restoreDraftRevisionSnapshot(workbenchID, messageID)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "draft revision not found for message")
		}
		return "", nil, errinfo.FileWriteFailed(errinfo.PhaseWorkshop, err.Error())
	}
	draftState, err := e.workbenches.DraftState(workbenchID)
	if err != nil {
		return "", nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	e.notifyDraftState(workbenchID, draftState)
	stateInfo, _ := e.readWorkshopState(workbenchID)
	stateInfo.PendingProposalID = ""
	_ = e.writeWorkshopState(workbenchID, stateInfo)
	return revisionID, draftState, nil
}

func (e *Engine) resolveRegenerateUserMessage(workbenchID, requestedMessageID string) (string, *errinfo.ErrorInfo) {
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	if len(items) == 0 {
		return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "conversation is empty")
	}
	targetIndex := -1
	if strings.TrimSpace(requestedMessageID) == "" {
		for i := len(items) - 1; i >= 0; i-- {
			if items[i].Role == "user" || items[i].Role == "assistant" {
				targetIndex = i
				break
			}
		}
		if targetIndex < 0 {
			return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "no message available to regenerate")
		}
	} else {
		targetIndex = findConversationMessageIndex(items, requestedMessageID)
		if targetIndex < 0 {
			return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "message not found")
		}
	}
	target := items[targetIndex]
	if target.Role == "user" {
		return target.MessageID, nil
	}
	if target.Role != "assistant" {
		return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "regenerate target must be a user or assistant message")
	}
	for i := targetIndex - 1; i >= 0; i-- {
		if items[i].Role == "user" {
			return items[i].MessageID, nil
		}
	}
	return "", errinfo.ValidationFailed(errinfo.PhaseWorkshop, "assistant message has no preceding user message")
}

func mapToolWorkerError(phase string, err error) *errinfo.ErrorInfo {
	if err == nil {
		return nil
	}
	if errors.Is(err, toolworker.ErrUnavailable) {
		return errinfo.ToolWorkerUnavailable(phase, "tool worker unavailable")
	}
	var remote *toolworker.RemoteError
	if errors.As(err, &remote) {
		switch remote.Code {
		case errinfo.CodeValidationFailed:
			return errinfo.ValidationFailed(phase, remote.Message)
		case errinfo.CodeSandboxViolation:
			return errinfo.SandboxViolation(phase, remote.Message)
		case errinfo.CodeFileReadFailed:
			return errinfo.FileReadFailed(phase, remote.Message)
		case errinfo.CodeFileWriteFailed:
			return errinfo.FileWriteFailed(phase, remote.Message)
		case toolworker.CodeToolWorkerUnavailable:
			return errinfo.ToolWorkerUnavailable(phase, remote.Message)
		}
	}
	return nil
}

const workshopContextSystemPrompt = `You are KeenBench, a local file assistant.
The Workbench is the local sandbox of files the user has provided.
The file manifest and contents below are already available to you.

Capabilities:
- You can create or update Workbench files directly.
- Office files (.docx, .xlsx, .pptx) can be modified by the system using structured operations.
- Do not offer external scripts or claim you cannot write files in this environment.

Do not ask the user to upload or re-send files that appear in the manifest.
If you need more detail from a file, ask the user to specify the file and section (for example: sheet name + row range, or a pasted excerpt).

When a request is feasible, respond directly and assume the system can create or update Workbench files.
If a request is not feasible within the Workbench sandbox or supported operations, say so and suggest a workable alternative.`

const workshopContentUnavailable = "File exists but content unavailable."
const workshopContentOmitted = "Content omitted to stay within context limits."
const workshopContentTruncated = "Content truncated to stay within context limits."
const conversationTruncatedNote = "Note: Earlier messages omitted to stay within context limits."
const contextCompressedEvent = "Context compressed to stay within model limits (older messages summarized)."

const (
	maxContextCharsTotal    = 24000
	maxContextCharsPerFile  = 8000
	maxContextLinesPerFile  = 200
	maxConversationMessages = 8
	maxConversationChars    = 6000
)

func trimConversationItems(items []conversationMessage) ([]conversationMessage, bool) {
	var filtered []conversationMessage
	for _, item := range items {
		if item.Role == "user" || item.Role == "assistant" {
			filtered = append(filtered, item)
		}
	}
	if len(filtered) == 0 {
		return nil, false
	}
	var trimmed []conversationMessage
	truncated := false
	totalChars := 0
	for i := len(filtered) - 1; i >= 0; i-- {
		item := filtered[i]
		if len(trimmed) == 0 {
			trimmed = append(trimmed, item)
			totalChars += len(item.Text)
			continue
		}
		if maxConversationMessages > 0 && len(trimmed) >= maxConversationMessages {
			truncated = true
			continue
		}
		if maxConversationChars > 0 && totalChars+len(item.Text) > maxConversationChars {
			truncated = true
			continue
		}
		trimmed = append(trimmed, item)
		totalChars += len(item.Text)
	}
	for i, j := 0, len(trimmed)-1; i < j; i, j = i+1, j-1 {
		trimmed[i], trimmed[j] = trimmed[j], trimmed[i]
	}
	if len(trimmed) < len(filtered) {
		truncated = true
	}
	return trimmed, truncated
}

func truncateToLines(text string, maxLines int) (string, bool) {
	if maxLines <= 0 {
		return text, false
	}
	lines := 0
	for i, r := range text {
		if r == '\n' {
			lines++
			if lines >= maxLines {
				return strings.TrimRight(text[:i], "\n"), true
			}
		}
	}
	return text, false
}

func truncateToChars(text string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return "", true
	}
	if len(text) <= maxChars {
		return text, false
	}
	trimmed := text[:maxChars]
	if idx := strings.LastIndex(trimmed, "\n"); idx > 0 {
		trimmed = trimmed[:idx]
	}
	trimmed = strings.TrimRight(trimmed, "\n")
	return trimmed, true
}

func limitContextForFile(kind string, content string, remaining int) (string, bool, bool) {
	if remaining <= 0 {
		return "", false, true
	}
	truncated := false
	content, cut := truncateToLines(content, maxContextLinesPerFile)
	truncated = truncated || cut
	content, cut = truncateToChars(content, maxContextCharsPerFile)
	truncated = truncated || cut
	if remaining > 0 && len(content) > remaining {
		content, _ = truncateToChars(content, remaining)
		truncated = true
	}
	return content, truncated, false
}

func contextDetailRequestHint(kind string) string {
	switch kind {
	case workbench.FileKindXlsx:
		return "Ask the user for the sheet name and row/column range."
	case workbench.FileKindPptx:
		return "Ask the user for the slide number or the specific text to focus on."
	case workbench.FileKindDocx, workbench.FileKindOdt, workbench.FileKindPdf:
		return "Ask the user for the page/section or to paste the relevant excerpt."
	default:
		return "Ask the user to paste or specify the relevant section."
	}
}

func contextTruncationHint(kind string, omitted bool) string {
	base := workshopContentTruncated
	if omitted {
		base = workshopContentOmitted
	}
	return base + " " + contextDetailRequestHint(kind)
}

func (e *Engine) buildWorkshopFileContext(ctx context.Context, workbenchID string) (string, *errinfo.ErrorInfo) {
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	area := "published"
	if draft, _ := e.workbenches.DraftState(workbenchID); draft != nil {
		area = "draft"
	}
	var manifest strings.Builder
	manifest.WriteString("Manifest:\n")
	if len(files) == 0 {
		manifest.WriteString("- (no files)\n")
	} else {
		for _, file := range files {
			kind := file.FileKind
			isOpaque := file.IsOpaque
			if kind == "" {
				kind, isOpaque = workbench.FileKindForPath(file.Path)
			}
			mimeType := file.MimeType
			if mimeType == "" {
				mimeType = mimeTypeForPath(file.Path)
			}
			manifest.WriteString(fmt.Sprintf("- %s | kind=%s | size=%d | mime=%s | opaque=%t\n", file.Path, kind, file.Size, mimeType, isOpaque))
		}
	}
	manifest.WriteString("\nFile contents:\n")
	remaining := maxContextCharsTotal
	for _, file := range files {
		kind := file.FileKind
		isOpaque := file.IsOpaque
		if kind == "" {
			kind, isOpaque = workbench.FileKindForPath(file.Path)
		}
		mimeType := file.MimeType
		if mimeType == "" {
			mimeType = mimeTypeForPath(file.Path)
		}
		manifest.WriteString("\n# " + file.Path)
		if kind != "" {
			manifest.WriteString(" (" + kind + ")")
		}
		if isOpaque {
			manifest.WriteString(" [opaque]")
		}
		manifest.WriteString("\n")
		switch kind {
		case workbench.FileKindText:
			if isCSVWorkbenchPath(file.Path) {
				mapResult := e.getFileMapForContext(ctx, workbenchID, area, kind, file.Path)
				if mapResult != "" {
					manifest.WriteString("Use table_get_map/table_describe/table_stats/table_read_rows/table_query/table_export/table_update_from_export.\n")
					manifest.WriteString("Map:\n")
					manifest.WriteString(mapResult)
					manifest.WriteString("\n")
				} else {
					manifest.WriteString(workshopContentUnavailable + "\n")
				}
				continue
			}
			content, err := e.workbenches.ReadFile(workbenchID, area, file.Path)
			if err != nil {
				manifest.WriteString(workshopContentUnavailable + "\n")
				e.logger.Warn("workshop.context_text_unavailable", "workbench_id", workbenchID, "path", file.Path, "error", err.Error())
				continue
			}
			content, truncated, omitted := limitContextForFile(kind, content, remaining)
			if omitted {
				manifest.WriteString(contextTruncationHint(kind, true) + "\n")
				continue
			}
			manifest.WriteString(content)
			if truncated {
				manifest.WriteString("\n" + contextTruncationHint(kind, false))
			}
			manifest.WriteString("\n")
			remaining -= len(content)
		case workbench.FileKindDocx, workbench.FileKindOdt, workbench.FileKindXlsx, workbench.FileKindPptx, workbench.FileKindPdf:
			text, err := e.extractText(ctx, workbenchID, area, kind, file.Path)
			if err != nil {
				manifest.WriteString(workshopContentUnavailable + "\n")
				e.logger.Warn("workshop.context_extract_unavailable", "workbench_id", workbenchID, "path", file.Path, "error", err.Error())
				continue
			}
			text, truncated, omitted := limitContextForFile(kind, text, remaining)
			if omitted {
				manifest.WriteString(contextTruncationHint(kind, true) + "\n")
				continue
			}
			manifest.WriteString(text)
			if truncated {
				manifest.WriteString("\n" + contextTruncationHint(kind, false))
			}
			manifest.WriteString("\n")
			remaining -= len(text)
		case workbench.FileKindImage:
			manifest.WriteString(fmt.Sprintf("Metadata: size=%d bytes, mime=%s. %s\n", file.Size, mimeType, workshopContentUnavailable))
		default:
			if isOpaque || kind == workbench.FileKindBinary {
				manifest.WriteString(fmt.Sprintf("Metadata: size=%d bytes, mime=%s. %s\n", file.Size, mimeType, workshopContentUnavailable))
			} else {
				manifest.WriteString(workshopContentUnavailable + "\n")
			}
		}
	}
	return manifest.String(), nil
}

func (e *Engine) buildChatMessages(ctx context.Context, workbenchID string) ([]llm.Message, *errinfo.ErrorInfo) {
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return nil, errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	contextBlock, errInfo := e.buildWorkshopFileContext(ctx, workbenchID)
	if errInfo != nil {
		return nil, errInfo
	}
	trimmed, truncated := trimConversationItems(items)
	if truncated {
		needsEvent := true
		if len(items) > 0 {
			last := items[len(items)-1]
			if last.Type == "system_event" && last.Text == contextCompressedEvent {
				needsEvent = false
			}
		}
		if needsEvent {
			_ = e.appendConversation(workbenchID, conversationMessage{
				Type:      "system_event",
				MessageID: fmt.Sprintf("s-%d", time.Now().UnixNano()),
				Role:      "system",
				Text:      contextCompressedEvent,
				CreatedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
	systemContent := workshopContextSystemPrompt + "\n\n" + contextBlock
	if contextInjection, _ := e.buildWorkbenchContextInjection(workbenchID); contextInjection != "" {
		systemContent += "\n\nWorkbench context:\n" + contextInjection
	}
	if truncated {
		systemContent += "\n\n" + conversationTruncatedNote
	}
	messages := []llm.Message{
		{Role: "system", Content: systemContent},
	}
	for _, item := range trimmed {
		if item.Role == "user" || item.Role == "assistant" {
			messages = append(messages, llm.Message{Role: item.Role, Content: item.Text})
		}
	}
	return messages, nil
}

const proposalSystemPrompt = "You are KeenBench. Return a single JSON object that matches Proposal schema v2. Output only JSON (no markdown or code fences).\nRequired shape:\n{\"schema_version\":2,\"summary\":\"...\",\"no_changes\":false,\"writes\":[{\"path\":\"file.md\",\"content\":\"...\"}],\"ops\":[{\"path\":\"report.docx\",\"kind\":\"docx\",\"summary\":\"...\",\"ops\":[{\"op\":\"set_paragraphs\",\"paragraphs\":[{\"text\":\"...\",\"style\":\"Heading1\"}]}]}],\"warnings\":[]}\nRules:\n- summary must be non-empty.\n- If no file edits are needed, set \"no_changes\": true and leave writes/ops empty.\n- Otherwise, either writes or ops (or both) must be present.\n- No delete operations.\n- Paths must be flat (no folders).\n- Writes allowed only for text/code extensions: .md, .txt, .csv, .json, .xml, .yaml, .yml, .html, .js, .ts, .py, .java, .go, .rb, .rs, .c, .cpp, .h, .css, .sql.\n- Ops are only for .docx/.xlsx/.pptx with kind docx/xlsx/pptx matching the extension.\n- Max 10 writes, max 100 ops per file, max 500 ops total.\n- Each ops entry must include a per-file summary.\nAllowed ops:\nDocx: set_paragraphs, append_paragraph, replace_text.\nXlsx: ensure_sheet, set_cells, set_range, set_column_widths, set_row_heights, freeze_panes.\nPptx: add_slide, set_slide_text, append_bullets."

const proposalSystemPromptStrict = "You are KeenBench. Return a single JSON object matching Proposal schema v2 exactly:\n{\"schema_version\":2,\"summary\":\"...\",\"no_changes\":false,\"writes\":[{\"path\":\"summary.md\",\"content\":\"...\"}],\"ops\":[],\"warnings\":[]}\nRules:\n- Output only JSON (no markdown or code fences).\n- summary must be non-empty.\n- If no file edits are needed, set \"no_changes\": true and leave writes/ops empty.\n- Otherwise, either writes or ops must be present.\n- Allowed write extensions: .md, .txt, .csv, .json, .xml, .yaml, .yml, .html, .js, .ts, .py, .java, .go, .rb, .rs, .c, .cpp, .h, .css, .sql.\n- No delete operations."

func (e *Engine) buildProposalPrompt(ctx context.Context, workbenchID string) (string, *errinfo.ErrorInfo) {
	items, err := e.readConversation(workbenchID)
	if err != nil {
		return "", errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	var convo strings.Builder
	trimmed, truncated := trimConversationItems(items)
	if truncated {
		convo.WriteString(conversationTruncatedNote + "\n")
	}
	for _, item := range trimmed {
		if item.Role == "user" || item.Role == "assistant" {
			convo.WriteString(item.Role + ": " + item.Text + "\n")
		}
	}
	contextBlock, errInfo := e.buildWorkshopFileContext(ctx, workbenchID)
	if errInfo != nil {
		return "", errInfo
	}
	injection := ""
	if contextInjection, _ := e.buildWorkbenchContextInjection(workbenchID); contextInjection != "" {
		injection = "\n\nWorkbench context:\n" + contextInjection
	}
	prompt := fmt.Sprintf("Conversation:\n%s\n\nWorkbench files:\n%s%s\n\nRemember: Files listed are already available; do not ask to upload. If content is truncated, ask the user to specify the relevant section. Return proposal JSON (schema_version=2).", convo.String(), contextBlock, injection)
	return prompt, nil
}

func (e *Engine) extractText(ctx context.Context, workbenchID, root, kind, path string) (string, error) {
	if e.toolWorker == nil {
		return "", toolworker.ErrUnavailable
	}
	method := ""
	switch kind {
	case workbench.FileKindDocx:
		method = "DocxExtractText"
	case workbench.FileKindOdt:
		method = "OdtExtractText"
	case workbench.FileKindXlsx:
		method = "XlsxExtractText"
	case workbench.FileKindPptx:
		method = "PptxExtractText"
	case workbench.FileKindPdf:
		method = "PdfExtractText"
	default:
		return "", fmt.Errorf("unsupported extract kind: %s", kind)
	}
	params := map[string]any{
		"workbench_id": workbenchID,
		"path":         path,
		"root":         root,
	}
	var resp struct {
		Text string `json:"text"`
	}
	if err := e.toolWorker.Call(ctx, method, params, &resp); err != nil {
		return "", err
	}
	return resp.Text, nil
}

type Proposal struct {
	ProposalID    string          `json:"proposal_id"`
	SchemaVersion int             `json:"schema_version,omitempty"`
	Summary       string          `json:"summary"`
	NoChanges     bool            `json:"no_changes,omitempty"`
	Writes        []ProposalWrite `json:"writes,omitempty"`
	Ops           []ProposalOp    `json:"ops,omitempty"`
	Warnings      []string        `json:"warnings,omitempty"`
}

type ProposalWrite struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type ProposalOp struct {
	Path    string           `json:"path"`
	Kind    string           `json:"kind"`
	Summary string           `json:"summary"`
	Ops     []map[string]any `json:"ops"`
}

const reviewReferenceWarningPublishedFallback = "Draft-start reference unavailable; comparing against current Published."
const reviewReferenceWarningUnavailable = "Reference content unavailable for this file."

func isProposalRepairable(errInfo *errinfo.ErrorInfo) bool {
	if errInfo == nil {
		return false
	}
	if errInfo.ErrorCode != errinfo.CodeValidationFailed || errInfo.Phase != errinfo.PhaseWorkshop {
		return false
	}
	if strings.Contains(strings.ToLower(errInfo.Detail), "delete") {
		return false
	}
	return true
}

func fallbackProposal(reason string) *Proposal {
	proposal := &Proposal{
		Summary:   "No draft changes.",
		NoChanges: true,
	}
	if reason != "" {
		proposal.Warnings = []string{fmt.Sprintf("Generated fallback proposal (%s).", reason)}
	}
	return proposal
}

func truncateUTF8(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	if maxBytes <= 0 {
		return ""
	}
	count := 0
	for _, r := range value {
		size := utf8.RuneLen(r)
		if size <= 0 {
			continue
		}
		if count+size > maxBytes {
			break
		}
		count += size
	}
	if count <= 0 {
		return ""
	}
	return value[:count]
}

func (e *Engine) parseProposal(response string) (*Proposal, *errinfo.ErrorInfo) {
	trimmed := strings.TrimSpace(response)
	payload := trimmed
	if !json.Valid([]byte(payload)) {
		start := strings.Index(payload, "{")
		end := strings.LastIndex(payload, "}")
		if start >= 0 && end > start {
			payload = payload[start : end+1]
		} else {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid proposal json")
		}
	}
	var raw any
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid proposal json")
	}
	if containsDeleteKeys(raw) {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "delete operations are not allowed")
	}
	var probe struct {
		SchemaVersion int `json:"schema_version"`
	}
	if err := json.Unmarshal([]byte(payload), &probe); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid proposal json")
	}
	if probe.SchemaVersion > 0 && probe.SchemaVersion != 2 {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported proposal schema")
	}

	var proposal Proposal
	if err := json.Unmarshal([]byte(payload), &proposal); err != nil {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "invalid proposal json")
	}

	if proposal.Summary == "" {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "proposal missing summary")
	}

	if probe.SchemaVersion >= 2 {
		proposal.SchemaVersion = 2
		if proposal.NoChanges {
			if len(proposal.Writes) > 0 || len(proposal.Ops) > 0 {
				return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "no_changes must not include writes or ops")
			}
			return &proposal, nil
		}
		if len(proposal.Writes) == 0 && len(proposal.Ops) == 0 {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "proposal missing writes or ops")
		}
		if len(proposal.Writes) > maxProposalWrites {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "too many writes in proposal")
		}
		for _, write := range proposal.Writes {
			if err := validateProposalWrite(write); err != nil {
				return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, err.Error())
			}
		}
		if err := validateProposalOps(proposal.Ops); err != nil {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, err.Error())
		}
		return &proposal, nil
	}

	if len(proposal.Writes) == 0 {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "proposal missing writes")
	}
	if len(proposal.Writes) > maxProposalWrites {
		return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, "too many writes in proposal")
	}
	for _, write := range proposal.Writes {
		if err := validateProposalWrite(write); err != nil {
			return nil, errinfo.ValidationFailed(errinfo.PhaseWorkshop, err.Error())
		}
	}
	return &proposal, nil
}

func containsDeleteKeys(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, val := range typed {
			lower := strings.ToLower(key)
			if lower == "delete" || lower == "deletes" {
				return true
			}
			if containsDeleteKeys(val) {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if containsDeleteKeys(item) {
				return true
			}
		}
	}
	return false
}

func validateProposalWrite(write ProposalWrite) error {
	if write.Path == "" || write.Content == "" {
		return errors.New("proposal write missing path/content")
	}
	if err := validateFlatPath(write.Path); err != nil {
		return err
	}
	if !workbench.IsTextWritePath(write.Path) {
		return errors.New("unsupported write extension")
	}
	if len(write.Content) > maxProposalContentBytes {
		return errors.New("content too large")
	}
	return nil
}

func validateProposalOps(entries []ProposalOp) error {
	if len(entries) == 0 {
		return nil
	}
	totalOps := 0
	for _, entry := range entries {
		if entry.Path == "" || entry.Kind == "" || entry.Summary == "" {
			return errors.New("proposal op missing path/kind/summary")
		}
		if err := validateFlatPath(entry.Path); err != nil {
			return err
		}
		kind := strings.ToLower(entry.Kind)
		ext := strings.ToLower(filepath.Ext(entry.Path))
		expectedExt := map[string]string{
			"docx": ".docx",
			"xlsx": ".xlsx",
			"pptx": ".pptx",
		}[kind]
		if expectedExt == "" {
			return errors.New("unsupported op kind")
		}
		if ext != expectedExt {
			return errors.New("op kind does not match file extension")
		}
		if len(entry.Ops) == 0 {
			return errors.New("proposal op missing ops")
		}
		if len(entry.Ops) > maxProposalOpsPerFile {
			return errors.New("too many ops for file")
		}
		totalOps += len(entry.Ops)
		if totalOps > maxProposalOpsTotal {
			return errors.New("too many ops in proposal")
		}
		for _, op := range entry.Ops {
			if err := validateOpEntry(kind, op); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOpEntry(kind string, op map[string]any) error {
	name, ok := op["op"].(string)
	if !ok || name == "" {
		return errors.New("op missing op name")
	}
	switch kind {
	case "docx":
		switch name {
		case "set_paragraphs":
			if _, ok := op["paragraphs"].([]any); !ok {
				return errors.New("set_paragraphs requires paragraphs")
			}
		case "append_paragraph":
		case "replace_text":
			if _, ok := op["search"].(string); !ok {
				return errors.New("replace_text requires search")
			}
		default:
			return errors.New("unsupported docx op")
		}
	case "xlsx":
		switch name {
		case "ensure_sheet":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("ensure_sheet requires sheet")
			}
		case "set_cells":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("set_cells requires sheet")
			}
			if _, ok := op["cells"].([]any); !ok {
				return errors.New("set_cells requires cells")
			}
		case "set_range":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("set_range requires sheet")
			}
			if _, ok := op["start"].(string); !ok {
				return errors.New("set_range requires start")
			}
			if _, ok := op["values"].([]any); !ok {
				return errors.New("set_range requires values")
			}
		case "set_column_widths":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("set_column_widths requires sheet")
			}
			columns, ok := op["columns"].([]any)
			if !ok || len(columns) == 0 {
				return errors.New("set_column_widths requires columns")
			}
			for _, raw := range columns {
				entry, ok := raw.(map[string]any)
				if !ok {
					return errors.New("set_column_widths requires columns entries with column and width")
				}
				columnRaw, ok := entry["column"]
				if !ok {
					return errors.New("set_column_widths requires columns entries with column and width")
				}
				switch typed := columnRaw.(type) {
				case string:
					if strings.TrimSpace(typed) == "" {
						return errors.New("set_column_widths requires columns entries with column and width")
					}
				default:
					if idx, ok := intFromAny(columnRaw); !ok || idx <= 0 {
						return errors.New("set_column_widths requires columns entries with column and width")
					}
				}
				if !isNumericValue(entry["width"]) {
					return errors.New("set_column_widths requires columns entries with column and width")
				}
			}
		case "set_row_heights":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("set_row_heights requires sheet")
			}
			rows, ok := op["rows"].([]any)
			if !ok || len(rows) == 0 {
				return errors.New("set_row_heights requires rows")
			}
			for _, raw := range rows {
				entry, ok := raw.(map[string]any)
				if !ok {
					return errors.New("set_row_heights requires rows entries with row and height")
				}
				row, ok := intFromAny(entry["row"])
				if !ok || row <= 0 {
					return errors.New("set_row_heights requires rows entries with row and height")
				}
				if !isNumericValue(entry["height"]) {
					return errors.New("set_row_heights requires rows entries with row and height")
				}
			}
		case "freeze_panes":
			if _, ok := op["sheet"].(string); !ok {
				return errors.New("freeze_panes requires sheet")
			}
			hasRow := false
			if raw, exists := op["row"]; exists {
				row, ok := intFromAny(raw)
				if !ok || row < 0 {
					return errors.New("freeze_panes requires row to be >= 0")
				}
				hasRow = true
			}
			hasColumn := false
			if raw, exists := op["column"]; exists {
				col, ok := intFromAny(raw)
				if !ok || col < 0 {
					return errors.New("freeze_panes requires column to be >= 0")
				}
				hasColumn = true
			}
			if !hasRow && !hasColumn {
				return errors.New("freeze_panes requires row or column")
			}
		default:
			return errors.New("unsupported xlsx op")
		}
	case "pptx":
		switch name {
		case "add_slide":
		case "set_slide_text":
			if _, ok := op["index"].(float64); !ok {
				if _, ok := op["index"].(int); !ok {
					return errors.New("set_slide_text requires index")
				}
			}
		case "append_bullets":
			if _, ok := op["index"].(float64); !ok {
				if _, ok := op["index"].(int); !ok {
					return errors.New("append_bullets requires index")
				}
			}
			if _, ok := op["bullets"].([]any); !ok {
				return errors.New("append_bullets requires bullets")
			}
		default:
			return errors.New("unsupported pptx op")
		}
	default:
		return errors.New("unsupported op kind")
	}
	return nil
}

func isNumericValue(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return true
	default:
		return false
	}
}

func validateFlatPath(path string) error {
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) || strings.Contains(clean, "\\") {
		return errors.New("invalid write path")
	}
	if filepath.Dir(clean) != "." {
		return errors.New("nested paths are not allowed")
	}
	return nil
}

func (e *Engine) validateProposalForApply(workbenchID string, proposal *Proposal) *errinfo.ErrorInfo {
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return errinfo.FileReadFailed(errinfo.PhaseWorkshop, err.Error())
	}
	index := make(map[string]workbench.FileEntry, len(files))
	for _, file := range files {
		index[strings.ToLower(file.Path)] = file
	}
	for _, write := range proposal.Writes {
		if !workbench.IsTextWritePath(write.Path) {
			return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported write extension")
		}
		if file, ok := index[strings.ToLower(write.Path)]; ok {
			if file.IsOpaque {
				return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "cannot write opaque file")
			}
			if file.FileKind != workbench.FileKindText {
				return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "cannot write read-only file type")
			}
		} else {
			kind, opaque := workbench.FileKindForPath(write.Path)
			if opaque || kind != workbench.FileKindText {
				return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "unsupported write extension")
			}
		}
	}
	for _, entry := range proposal.Ops {
		if file, ok := index[strings.ToLower(entry.Path)]; ok {
			if file.FileKind == "" {
				fileKind, _ := workbench.FileKindForPath(file.Path)
				file.FileKind = fileKind
			}
			if file.IsOpaque {
				return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "cannot modify opaque file")
			}
			expectedKind := strings.ToLower(entry.Kind)
			if file.FileKind != expectedKind {
				return errinfo.ValidationFailed(errinfo.PhaseWorkshop, "op kind does not match existing file")
			}
		}
	}
	return nil
}

func (e *Engine) applyOfficeOps(ctx context.Context, workbenchID, root string, entry ProposalOp) error {
	if e.toolWorker == nil {
		return toolworker.ErrUnavailable
	}
	method := ""
	switch strings.ToLower(entry.Kind) {
	case workbench.FileKindDocx:
		method = "DocxApplyOps"
	case workbench.FileKindXlsx:
		method = "XlsxApplyOps"
	case workbench.FileKindPptx:
		method = "PptxApplyOps"
	default:
		return errors.New("unsupported op kind")
	}
	params := map[string]any{
		"workbench_id": workbenchID,
		"path":         entry.Path,
		"ops":          entry.Ops,
		"root":         root,
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	if err := e.toolWorker.Call(ctx, method, params, &resp); err != nil {
		return err
	}
	return nil
}

func (e *Engine) ensureDraftBaseline(ctx context.Context, workbenchID, draftID string) error {
	reviewRoot := e.reviewRoot(workbenchID, draftID)
	baselineDir := filepath.Join(reviewRoot, "baseline")
	if _, err := os.Stat(baselineDir); err == nil {
		return nil
	}
	if err := os.MkdirAll(baselineDir, 0o755); err != nil {
		return err
	}
	files, err := e.workbenches.FilesList(workbenchID)
	if err != nil {
		return err
	}
	for _, file := range files {
		kind := file.FileKind
		if kind == "" {
			kind, _ = workbench.FileKindForPath(file.Path)
		}
		switch kind {
		case workbench.FileKindDocx, workbench.FileKindOdt, workbench.FileKindXlsx, workbench.FileKindPptx, workbench.FileKindPdf:
			text, err := e.extractText(ctx, workbenchID, "draft", kind, file.Path)
			if err != nil {
				e.logger.Warn("review.baseline_extract_failed", "workbench_id", workbenchID, "path", file.Path, "error", err.Error())
				continue
			}
			if err := os.WriteFile(e.baselinePath(workbenchID, draftID, file.Path), []byte(text), 0o600); err != nil {
				e.logger.Warn("review.baseline_write_failed", "workbench_id", workbenchID, "path", file.Path, "error", err.Error())
				continue
			}
		}
	}
	return nil
}

func (e *Engine) writeProposalSummaries(workbenchID, draftID string, summaries map[string]string) error {
	root := e.reviewRoot(workbenchID, draftID)
	dir := filepath.Join(root, "summaries")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for path, summary := range summaries {
		if summary == "" {
			continue
		}
		if err := os.WriteFile(e.summaryPath(workbenchID, draftID, path), []byte(summary), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) writeDraftSummary(workbenchID, draftID, summary string) error {
	if strings.TrimSpace(summary) == "" {
		return nil
	}
	root := e.reviewRoot(workbenchID, draftID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	return os.WriteFile(e.draftSummaryPath(workbenchID, draftID), []byte(summary), 0o600)
}

func (e *Engine) readDraftSummary(workbenchID, draftID string) (string, error) {
	data, err := os.ReadFile(e.draftSummaryPath(workbenchID, draftID))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (e *Engine) writeProposalFocusHints(workbenchID, draftID string, hints map[string]map[string]any) error {
	root := e.reviewRoot(workbenchID, draftID)
	dir := filepath.Join(root, "focus_hints")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for path, hint := range hints {
		if hint == nil || len(hint) == 0 {
			continue
		}
		if err := writeJSON(e.focusHintPath(workbenchID, draftID, path), hint); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) summaryPath(workbenchID, draftID, path string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(path)))
	return filepath.Join(e.reviewRoot(workbenchID, draftID), "summaries", hex.EncodeToString(hash[:])+".txt")
}

func (e *Engine) draftSummaryPath(workbenchID, draftID string) string {
	return filepath.Join(e.reviewRoot(workbenchID, draftID), "draft_summary.txt")
}

func (e *Engine) focusHintPath(workbenchID, draftID, path string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(path)))
	return filepath.Join(e.reviewRoot(workbenchID, draftID), "focus_hints", hex.EncodeToString(hash[:])+".json")
}

func (e *Engine) readFocusHint(workbenchID, draftID, path string) (map[string]any, error) {
	data, err := os.ReadFile(e.focusHintPath(workbenchID, draftID, path))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hint map[string]any
	if err := json.Unmarshal(data, &hint); err != nil {
		return nil, err
	}
	if len(hint) == 0 {
		return nil, nil
	}
	return hint, nil
}

type xlsxFocusBounds struct {
	minRow int
	maxRow int
	minCol int
	maxCol int
}

func buildXlsxFocusHint(ops []map[string]any) map[string]any {
	type sheetEntry struct {
		bounds xlsxFocusBounds
		seen   bool
	}
	bySheet := make(map[string]*sheetEntry)
	order := make([]string, 0)
	lastTouchedSheet := ""

	ensureSheet := func(sheet string) {
		if sheet == "" {
			return
		}
		lastTouchedSheet = sheet
		if _, ok := bySheet[sheet]; ok {
			return
		}
		bySheet[sheet] = &sheetEntry{}
		order = append(order, sheet)
	}

	updateBounds := func(sheet string, row, col int) {
		if sheet == "" || row <= 0 || col <= 0 {
			return
		}
		ensureSheet(sheet)
		entry, ok := bySheet[sheet]
		if !ok {
			entry = &sheetEntry{
				bounds: xlsxFocusBounds{minRow: row, maxRow: row, minCol: col, maxCol: col},
				seen:   true,
			}
			bySheet[sheet] = entry
			order = append(order, sheet)
			return
		}
		if !entry.seen {
			entry.bounds = xlsxFocusBounds{minRow: row, maxRow: row, minCol: col, maxCol: col}
			entry.seen = true
			return
		}
		if row < entry.bounds.minRow {
			entry.bounds.minRow = row
		}
		if row > entry.bounds.maxRow {
			entry.bounds.maxRow = row
		}
		if col < entry.bounds.minCol {
			entry.bounds.minCol = col
		}
		if col > entry.bounds.maxCol {
			entry.bounds.maxCol = col
		}
	}

	updateRange := func(sheet string, startRow, startCol, endRow, endCol int) {
		if startRow <= 0 || startCol <= 0 || endRow <= 0 || endCol <= 0 {
			return
		}
		if endRow < startRow {
			startRow, endRow = endRow, startRow
		}
		if endCol < startCol {
			startCol, endCol = endCol, startCol
		}
		updateBounds(sheet, startRow, startCol)
		updateBounds(sheet, endRow, endCol)
	}

	for _, op := range ops {
		name, _ := op["op"].(string)
		sheet, _ := op["sheet"].(string)
		ensureSheet(sheet)
		switch name {
		case "set_cells":
			cells, ok := op["cells"].([]any)
			if !ok {
				continue
			}
			for _, entry := range cells {
				cellMap, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				cellRef, _ := cellMap["cell"].(string)
				row, col, ok := parseCellRef(cellRef)
				if !ok {
					continue
				}
				updateBounds(sheet, row, col)
			}
		case "set_range":
			startRef, _ := op["start"].(string)
			startRow, startCol, ok := parseCellRef(startRef)
			if !ok {
				continue
			}
			values, ok := op["values"].([]any)
			if !ok {
				continue
			}
			rowCount := len(values)
			if rowCount == 0 {
				continue
			}
			colCount := 0
			for _, row := range values {
				rowVals, ok := row.([]any)
				if !ok {
					continue
				}
				if len(rowVals) > colCount {
					colCount = len(rowVals)
				}
			}
			if colCount == 0 {
				continue
			}
			endRow := startRow + rowCount - 1
			endCol := startCol + colCount - 1
			updateRange(sheet, startRow, startCol, endRow, endCol)
		case "summarize_by_category", "ensure_sheet", "set_column_widths", "set_row_heights", "freeze_panes":
			// Sheet-level ops establish target context even without explicit cell bounds.
			continue
		}
	}

	if len(order) == 0 {
		return nil
	}
	var bestSheet string
	bestArea := -1
	for _, sheet := range order {
		entry := bySheet[sheet]
		if entry == nil || !entry.seen {
			continue
		}
		area := (entry.bounds.maxRow - entry.bounds.minRow + 1) * (entry.bounds.maxCol - entry.bounds.minCol + 1)
		if area > bestArea {
			bestArea = area
			bestSheet = sheet
		}
	}
	if bestSheet == "" {
		bestSheet = lastTouchedSheet
	}
	if bestSheet == "" && len(order) > 0 {
		bestSheet = order[len(order)-1]
	}
	if bestSheet == "" {
		return nil
	}
	entry := bySheet[bestSheet]
	if entry == nil || !entry.seen {
		return map[string]any{
			"sheet": bestSheet,
		}
	}
	bounds := entry.bounds
	return map[string]any{
		"sheet":     bestSheet,
		"row_start": bounds.minRow - 1,
		"row_end":   bounds.maxRow - 1,
		"col_start": bounds.minCol - 1,
		"col_end":   bounds.maxCol - 1,
	}
}

func buildDocxFocusHint(ops []map[string]any) map[string]any {
	explicitSectionIndex := -1
	hasDocxEdit := false
	for _, op := range ops {
		if idx, ok := firstIntField(op, "section_index", "section"); ok && idx >= 0 {
			if explicitSectionIndex < 0 || idx < explicitSectionIndex {
				explicitSectionIndex = idx
			}
			continue
		}
		name, _ := op["op"].(string)
		switch strings.ToLower(strings.TrimSpace(name)) {
		case "set_paragraphs", "append_paragraph", "replace_text":
			hasDocxEdit = true
		}
	}
	if explicitSectionIndex >= 0 {
		return map[string]any{"section_index": explicitSectionIndex}
	}
	if hasDocxEdit {
		return map[string]any{"section_index": 0}
	}
	return nil
}

func buildPptxFocusHint(ops []map[string]any) map[string]any {
	slideIndex := -1
	for _, op := range ops {
		name, _ := op["op"].(string)
		if strings.EqualFold(strings.TrimSpace(name), "add_slide") {
			// add_slide targets are resolved from resulting draft state
			// because pyworker currently appends and ignores op index.
			continue
		}
		if idx, ok := firstIntField(op, "index", "slide_index"); ok && idx >= 0 {
			if slideIndex < 0 || idx < slideIndex {
				slideIndex = idx
			}
		}
	}
	if slideIndex < 0 {
		return nil
	}
	return map[string]any{"slide_index": slideIndex}
}

func firstIntField(values map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		if out, ok := intFromAny(value); ok {
			return out, true
		}
	}
	return 0, false
}

func intFromAny(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int8:
		return int(typed), true
	case int16:
		return int(typed), true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint:
		return int(typed), true
	case uint8:
		return int(typed), true
	case uint16:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float32:
		out := int(typed)
		if float32(out) == typed {
			return out, true
		}
	case float64:
		out := int(typed)
		if float64(out) == typed {
			return out, true
		}
	case string:
		out, err := strconv.Atoi(strings.TrimSpace(typed))
		if err == nil {
			return out, true
		}
	case json.Number:
		out, err := typed.Int64()
		if err == nil {
			return int(out), true
		}
	}
	return 0, false
}

func parseCellRef(cell string) (int, int, bool) {
	trimmed := strings.TrimSpace(cell)
	if trimmed == "" {
		return 0, 0, false
	}
	i := 0
	for i < len(trimmed) && unicode.IsLetter(rune(trimmed[i])) {
		i++
	}
	if i == 0 || i == len(trimmed) {
		return 0, 0, false
	}
	colStr := strings.ToUpper(trimmed[:i])
	rowStr := trimmed[i:]
	row, err := strconv.Atoi(rowStr)
	if err != nil || row <= 0 {
		return 0, 0, false
	}
	col := 0
	for _, ch := range colStr {
		if ch < 'A' || ch > 'Z' {
			return 0, 0, false
		}
		col = col*26 + int(ch-'A'+1)
	}
	if col <= 0 {
		return 0, 0, false
	}
	return row, col, true
}

func (e *Engine) baselinePath(workbenchID, draftID, path string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(path)))
	return filepath.Join(e.reviewRoot(workbenchID, draftID), "baseline", hex.EncodeToString(hash[:])+".txt")
}

func (e *Engine) reviewRoot(workbenchID, draftID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "review", draftID)
}

func (e *Engine) readBaselineText(workbenchID, draftID, path string) (string, bool, error) {
	data, err := os.ReadFile(e.baselinePath(workbenchID, draftID, path))
	if err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	return string(data), true, nil
}

func (e *Engine) publishedFileExists(workbenchID, path string) (bool, *errinfo.ErrorInfo) {
	_, err := e.workbenches.ReadFileBytes(workbenchID, "published", path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if errors.Is(err, workbench.ErrInvalidPath) {
		return false, errinfo.ValidationFailed(errinfo.PhaseReview, err.Error())
	}
	if errors.Is(err, workbench.ErrSandboxViolation) {
		return false, errinfo.SandboxViolation(errinfo.PhaseReview, err.Error())
	}
	return false, errinfo.FileReadFailed(errinfo.PhaseReview, err.Error())
}

func (e *Engine) readProposalSummary(workbenchID, draftID, path string) (string, error) {
	data, err := os.ReadFile(e.summaryPath(workbenchID, draftID, path))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func previewKindForFile(kind string, isOpaque bool) string {
	if isOpaque || kind == workbench.FileKindBinary {
		return "none"
	}
	switch kind {
	case workbench.FileKindImage:
		return "image"
	case workbench.FileKindXlsx:
		return "grid"
	case workbench.FileKindText, workbench.FileKindDocx, workbench.FileKindOdt, workbench.FileKindPptx, workbench.FileKindPdf:
		return "diff"
	default:
		return "none"
	}
}

var mimeFallbacks = map[string]string{
	".md":   "text/markdown",
	".csv":  "text/csv",
	".yaml": "text/yaml",
	".yml":  "text/yaml",
	".json": "application/json",
	".xml":  "application/xml",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".odt":  "application/vnd.oasis.opendocument.text",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".pdf":  "application/pdf",
	".svg":  "image/svg+xml",
}

func mimeTypeForPath(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "application/octet-stream"
	}
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = mimeFallbacks[ext]
	}
	if mimeType == "" {
		return "application/octet-stream"
	}
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	return mimeType
}

func (e *Engine) proposalPath(workbenchID, proposalID string) (string, error) {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	if strings.ContainsAny(workbenchID, string(filepath.Separator)+"\\") {
		return "", errors.New("invalid workbench id")
	}
	if strings.ContainsAny(proposalID, string(filepath.Separator)+"\\") {
		return "", errors.New("invalid proposal id")
	}
	path := filepath.Join(root, "meta", "workshop", "proposals", proposalID+".json")
	return path, nil
}

func (e *Engine) writeProposal(workbenchID string, proposal *Proposal) error {
	path, err := e.proposalPath(workbenchID, proposal.ProposalID)
	if err != nil {
		return err
	}
	return writeJSON(path, proposal)
}

func (e *Engine) readProposal(workbenchID, proposalID string) (*Proposal, error) {
	path, err := e.proposalPath(workbenchID, proposalID)
	if err != nil {
		return nil, err
	}
	var proposal Proposal
	if err := readJSON(path, &proposal); err != nil {
		return nil, err
	}
	return &proposal, nil
}

func (e *Engine) workbenchesRoot() string {
	return appdirs.WorkbenchesDir(e.dataDir)
}

func (e *Engine) readWorkshopState(workbenchID string) (*workshopState, error) {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "workshop_state.json")
	var state workshopState
	if err := readJSON(path, &state); err != nil {
		if os.IsNotExist(err) {
			return &workshopState{}, nil
		}
		return nil, err
	}
	return &state, nil
}

func (e *Engine) writeWorkshopState(workbenchID string, state *workshopState) error {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "workshop_state.json")
	return writeJSON(path, state)
}

type workshopState struct {
	ActiveModelID     string `json:"active_model_id"`
	PendingProposalID string `json:"pending_proposal_id,omitempty"`
}

type rpiState struct {
	HasResearch   bool          `json:"has_research"`
	HasPlan       bool          `json:"has_plan"`
	PlanItems     []rpiPlanItem `json:"plan_items"`
	OriginalCount int           `json:"original_count"`
	AllDone       bool          `json:"all_done"`
}

type rpiPlanItem struct {
	Index   int    `json:"index"`
	Label   string `json:"label"`
	Status  string `json:"status"`
	RawLine string `json:"raw_line"`
}

var (
	rpiPlanItemLinePattern      = regexp.MustCompile(`^- \[([ x!])\]\s*(\d+)\.\s*(.+)$`)
	rpiUncheckedPlanLinePattern = regexp.MustCompile(`^- \[ \]\s*\d+\.\s*.+$`)
	rpiOriginalCountPattern     = regexp.MustCompile(`<!--\s*original_count:\s*(\d+)\s*-->`)
	rpiFailedSuffixPattern      = regexp.MustCompile(`\s+\[Failed:\s*.*\]\s*$`)
)

func (e *Engine) rpiDir(workbenchID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "workshop", "_rpi")
}

func (e *Engine) readRPIState(workbenchID string) rpiState {
	state := rpiState{
		PlanItems: make([]rpiPlanItem, 0),
	}
	if _, err := e.readRPIArtifact(workbenchID, rpiResearchFile); err == nil {
		state.HasResearch = true
	}
	planContent, err := e.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		return state
	}
	state.HasPlan = true
	lines := strings.Split(planContent, "\n")
	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		match := rpiPlanItemLinePattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		index, parseErr := strconv.Atoi(match[2])
		if parseErr != nil {
			continue
		}
		status := rpiStatusPending
		switch match[1] {
		case "x":
			status = rpiStatusDone
		case "!":
			status = rpiStatusFailed
		}
		state.PlanItems = append(state.PlanItems, rpiPlanItem{
			Index:   index,
			Label:   stripRPIFailureSuffix(match[3]),
			Status:  status,
			RawLine: line,
		})
	}
	state.OriginalCount = parseRPIOriginalCount(planContent)
	if state.OriginalCount <= 0 {
		state.OriginalCount = len(state.PlanItems)
	}
	state.AllDone = true
	for _, item := range state.PlanItems {
		if item.Status == rpiStatusPending {
			state.AllDone = false
			break
		}
	}
	return state
}

func (e *Engine) clearRPIState(workbenchID string) error {
	return os.RemoveAll(e.rpiDir(workbenchID))
}

func (e *Engine) writeRPIArtifact(workbenchID, filename, content string) error {
	path, err := e.rpiArtifactPath(workbenchID, filename)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

func (e *Engine) readRPIArtifact(workbenchID, filename string) (string, error) {
	path, err := e.rpiArtifactPath(workbenchID, filename)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (e *Engine) markPlanItem(workbenchID string, itemIndex int, status, reason string) error {
	marker := ""
	switch strings.TrimSpace(status) {
	case rpiStatusPending:
		marker = " "
	case rpiStatusDone:
		marker = "x"
	case rpiStatusFailed:
		marker = "!"
	default:
		return fmt.Errorf("invalid plan item status: %s", status)
	}
	planContent, err := e.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		return err
	}
	lines := strings.Split(planContent, "\n")
	targetLine := -1
	targetItemNumber := ""
	targetLabel := ""
	currentItem := 0
	for i, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		match := rpiPlanItemLinePattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		if currentItem == itemIndex {
			targetLine = i
			targetItemNumber = match[2]
			targetLabel = stripRPIFailureSuffix(match[3])
			break
		}
		currentItem++
	}
	if targetLine < 0 {
		return fmt.Errorf("plan item at position %d not found", itemIndex)
	}
	updatedLine := fmt.Sprintf("- [%s] %s. %s", marker, targetItemNumber, targetLabel)
	if strings.TrimSpace(status) == rpiStatusFailed {
		cleanReason := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(reason, "\r", " "), "\n", " "))
		if cleanReason != "" {
			updatedLine += " [Failed: " + cleanReason + "]"
		}
	}
	lines[targetLine] = updatedLine
	updatedContent := strings.Join(lines, "\n")
	if strings.HasSuffix(planContent, "\n") && !strings.HasSuffix(updatedContent, "\n") {
		updatedContent += "\n"
	}
	return e.writeRPIArtifact(workbenchID, rpiPlanFile, updatedContent)
}

func (e *Engine) currentToolLogSeq(workbenchID string) int {
	path := e.toolLogPath(workbenchID)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			e.logger.Warn("workshop.tool_log_read_error", "workbench_id", workbenchID, "error", err.Error())
		}
		return 0
	}
	maxID := 0
	for _, rawLine := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		var entry toolLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.ID > maxID {
			maxID = entry.ID
		}
	}
	return maxID
}

func (e *Engine) appendPlanItems(workbenchID string, newItems []string) error {
	if len(newItems) == 0 {
		return nil
	}
	planContent, err := e.readRPIArtifact(workbenchID, rpiPlanFile)
	if err != nil {
		return err
	}
	lines := strings.Split(planContent, "\n")
	currentCount := 0
	lastItemLine := -1
	for i, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		if rpiPlanItemLinePattern.MatchString(line) {
			currentCount++
			lastItemLine = i
		}
	}
	originalCount := parseRPIOriginalCount(planContent)
	if originalCount <= 0 {
		originalCount = currentCount
	}
	maxCount := originalCount * rpiMaxPlanInflation
	if maxCount <= 0 {
		maxCount = currentCount + len(newItems)
	}
	allowed := len(newItems)
	if currentCount+allowed > maxCount {
		allowed = maxCount - currentCount
		if allowed < 0 {
			allowed = 0
		}
		dropped := len(newItems) - allowed
		if dropped > 0 {
			e.logger.Warn("workshop.rpi_plan_inflation_capped",
				"workbench_id", workbenchID,
				"original_count", originalCount,
				"current_count", currentCount,
				"requested_new", len(newItems),
				"dropped", dropped,
				"max_count", maxCount,
			)
		}
	}
	if allowed == 0 {
		return nil
	}
	itemsToAppend := make([]string, 0, allowed)
	for _, rawItem := range newItems {
		if len(itemsToAppend) >= allowed {
			break
		}
		line := strings.TrimSpace(strings.TrimSuffix(rawItem, "\r"))
		if !rpiUncheckedPlanLinePattern.MatchString(line) {
			continue
		}
		itemsToAppend = append(itemsToAppend, line)
	}
	if len(itemsToAppend) == 0 {
		return nil
	}
	if len(lines) == 1 && lines[0] == "" {
		lines = append([]string{}, itemsToAppend...)
	} else if lastItemLine < 0 {
		lines = append(lines, itemsToAppend...)
	} else {
		tail := append([]string{}, lines[lastItemLine+1:]...)
		lines = append(lines[:lastItemLine+1], itemsToAppend...)
		lines = append(lines, tail...)
	}
	updatedContent := strings.Join(lines, "\n")
	if strings.HasSuffix(planContent, "\n") && !strings.HasSuffix(updatedContent, "\n") {
		updatedContent += "\n"
	}
	return e.writeRPIArtifact(workbenchID, rpiPlanFile, updatedContent)
}

func extractNewPlanItems(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	items := make([]string, 0)
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if rpiUncheckedPlanLinePattern.MatchString(line) {
			items = append(items, line)
		}
	}
	return items
}

func (e *Engine) rpiArtifactPath(workbenchID, filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if name == "" {
		return "", errors.New("artifact filename is required")
	}
	if filepath.Base(name) != name || name == "." || name == ".." {
		return "", errors.New("invalid artifact filename")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("invalid artifact filename")
	}
	return filepath.Join(e.rpiDir(workbenchID), name), nil
}

func parseRPIOriginalCount(planContent string) int {
	match := rpiOriginalCountPattern.FindStringSubmatch(planContent)
	if len(match) != 2 {
		return 0
	}
	count, err := strconv.Atoi(strings.TrimSpace(match[1]))
	if err != nil || count < 0 {
		return 0
	}
	return count
}

func stripRPIFailureSuffix(label string) string {
	trimmed := strings.TrimSpace(label)
	if trimmed == "" {
		return trimmed
	}
	return strings.TrimSpace(rpiFailedSuffixPattern.ReplaceAllString(trimmed, ""))
}

type conversationMessage struct {
	Type         string         `json:"type"`
	MessageID    string         `json:"message_id"`
	Role         string         `json:"role"`
	Text         string         `json:"text"`
	CreatedAt    string         `json:"created_at"`
	EventKind    string         `json:"event_kind,omitempty"`
	CheckpointID string         `json:"checkpoint_id,omitempty"`
	Reason       string         `json:"reason,omitempty"`
	Timestamp    string         `json:"timestamp,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

type draftRevisionSnapshot struct {
	RevisionID string `json:"revision_id"`
	MessageID  string `json:"message_id"`
	CreatedAt  string `json:"created_at"`
	HasDraft   bool   `json:"has_draft"`
	DraftID    string `json:"draft_id,omitempty"`
}

func (e *Engine) appendConversation(workbenchID string, entry conversationMessage) error {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "conversation.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	if entry.Role == "user" || entry.Role == "assistant" {
		if err := e.recordDraftRevisionSnapshot(workbenchID, entry.MessageID); err != nil {
			return err
		}
	}
	e.emitClutterChanged(workbenchID)
	return nil
}

func (e *Engine) writeConversation(workbenchID string, entries []conversationMessage) error {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "conversation.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var builder strings.Builder
	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(builder.String()), 0o600); err != nil {
		return err
	}
	e.emitClutterChanged(workbenchID)
	return nil
}

func (e *Engine) updateConversationMetadata(workbenchID, messageID string, metadata map[string]any) error {
	msgID := strings.TrimSpace(messageID)
	if msgID == "" || len(metadata) == 0 {
		return nil
	}
	entries, err := e.readConversation(workbenchID)
	if err != nil {
		return err
	}
	idx := findConversationMessageIndex(entries, msgID)
	if idx < 0 {
		return fmt.Errorf("message %q not found in conversation", msgID)
	}
	if entries[idx].Metadata == nil {
		entries[idx].Metadata = map[string]any{}
	}
	for key, value := range metadata {
		entries[idx].Metadata[key] = value
	}
	return e.writeConversation(workbenchID, entries)
}

func (e *Engine) appendCheckpointConversationEvent(workbenchID string, entry conversationMessage) {
	if strings.TrimSpace(entry.CreatedAt) == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if strings.TrimSpace(entry.Timestamp) == "" {
		entry.Timestamp = entry.CreatedAt
	}
	if strings.TrimSpace(entry.Type) == "" {
		entry.Type = "system_event"
	}
	if strings.TrimSpace(entry.Role) == "" {
		entry.Role = "system"
	}
	if strings.TrimSpace(entry.MessageID) == "" {
		entry.MessageID = fmt.Sprintf("s-%d", time.Now().UnixNano())
	}
	if err := e.appendConversation(workbenchID, entry); err != nil {
		e.logger.Warn(
			"workshop.append_checkpoint_event_failed",
			"workbench_id",
			workbenchID,
			"event_kind",
			entry.EventKind,
			"checkpoint_id",
			entry.CheckpointID,
			"error",
			err.Error(),
		)
	}
}

func (e *Engine) readConversation(workbenchID string) ([]conversationMessage, error) {
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	path := filepath.Join(root, "meta", "conversation.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []conversationMessage{}, nil
		}
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var entries []conversationMessage
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry conversationMessage
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		needsCompat := entry.MessageID == "" || entry.Text == "" || entry.CreatedAt == "" || entry.Type == ""
		if !needsCompat && (entry.Type == "system_event" || entry.Role == "system") {
			needsCompat = entry.EventKind == "" || entry.Metadata == nil
		}
		if needsCompat {
			var raw map[string]any
			if err := json.Unmarshal([]byte(line), &raw); err == nil {
				if entry.MessageID == "" {
					entry.MessageID = stringValue(raw["id"])
				}
				if entry.Text == "" {
					entry.Text = stringValue(raw["content"])
				}
				if entry.CreatedAt == "" {
					entry.CreatedAt = stringValue(raw["timestamp"])
				}
				if entry.Type == "" {
					entry.Type = stringValue(raw["kind"])
				}
				if entry.EventKind == "" {
					entry.EventKind = stringValue(raw["event"])
				}
				if entry.CheckpointID == "" {
					entry.CheckpointID = stringValue(raw["checkpoint_id"])
				}
				if entry.Reason == "" {
					entry.Reason = stringValue(raw["reason"])
				}
				if entry.Timestamp == "" {
					entry.Timestamp = stringValue(raw["timestamp"])
				}
				if entry.Metadata == nil {
					if metadata, ok := raw["metadata"].(map[string]any); ok && len(metadata) > 0 {
						entry.Metadata = metadata
					}
				}
			}
		}
		if entry.Type == "" {
			switch entry.Role {
			case "assistant":
				entry.Type = "assistant_message"
			case "user":
				entry.Type = "user_message"
			case "system":
				entry.Type = "system_event"
			}
		}
		if entry.Timestamp == "" {
			entry.Timestamp = entry.CreatedAt
		}
		if entry.MessageID == "" {
			entry.MessageID = fmt.Sprintf("legacy-%d", i)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func findConversationMessageIndex(items []conversationMessage, messageID string) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].MessageID == messageID {
			return i
		}
	}
	return -1
}

func rewindPublishedCheckpointID(items []conversationMessage, targetIndex int) string {
	for i := targetIndex + 1; i < len(items); i++ {
		entry := items[i]
		switch strings.TrimSpace(entry.EventKind) {
		case "checkpoint_publish":
			if checkpointID := strings.TrimSpace(entry.CheckpointID); checkpointID != "" {
				return checkpointID
			}
		case "checkpoint_restore":
			if entry.Metadata == nil {
				continue
			}
			if preRestoreID := strings.TrimSpace(stringValue(entry.Metadata["pre_restore_checkpoint_id"])); preRestoreID != "" {
				return preRestoreID
			}
		}
	}
	return ""
}

func workshopRevisionID(messageID string) string {
	sum := sha256.Sum256([]byte(messageID))
	return hex.EncodeToString(sum[:])
}

func (e *Engine) workshopRevisionsRoot(workbenchID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "workshop", "draft_revisions")
}

func (e *Engine) workshopRevisionDir(workbenchID, messageID string) string {
	return filepath.Join(e.workshopRevisionsRoot(workbenchID), workshopRevisionID(messageID))
}

func (e *Engine) recordDraftRevisionSnapshot(workbenchID, messageID string) error {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil
	}
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	state, err := e.workbenches.DraftState(workbenchID)
	if err != nil {
		return err
	}
	revisionID := workshopRevisionID(messageID)
	snapshot := draftRevisionSnapshot{
		RevisionID: revisionID,
		MessageID:  messageID,
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		HasDraft:   state != nil,
	}
	if state != nil {
		snapshot.DraftID = state.DraftID
	}

	revisionDir := e.workshopRevisionDir(workbenchID, messageID)
	stagingDir := revisionDir + ".staging"
	_ = os.RemoveAll(stagingDir)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return err
	}
	cleanup := func() {
		_ = os.RemoveAll(stagingDir)
	}

	if state != nil {
		draftSrc := filepath.Join(root, "draft")
		if err := snapshotDirLight(draftSrc, filepath.Join(stagingDir, "draft_snapshot")); err != nil {
			cleanup()
			return err
		}
		if err := snapshotFileLight(filepath.Join(root, "meta", "draft.json"), filepath.Join(stagingDir, "draft.json")); err != nil {
			cleanup()
			return err
		}
		reviewSrc := filepath.Join(root, "meta", "review", state.DraftID)
		if _, err := os.Stat(reviewSrc); err == nil {
			if err := snapshotDirLight(reviewSrc, filepath.Join(stagingDir, "review_snapshot")); err != nil {
				cleanup()
				return err
			}
		} else if !os.IsNotExist(err) {
			cleanup()
			return err
		}
	}
	if err := writeJSON(filepath.Join(stagingDir, "rev.json"), snapshot); err != nil {
		cleanup()
		return err
	}
	_ = os.RemoveAll(revisionDir)
	if err := os.Rename(stagingDir, revisionDir); err != nil {
		cleanup()
		return err
	}
	return nil
}

func (e *Engine) restoreDraftRevisionSnapshot(workbenchID, messageID string) (string, error) {
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return "", errors.New("message_id is required")
	}
	root := filepath.Join(e.workbenchesRoot(), workbenchID)
	metaRoot := filepath.Join(root, "meta")
	revisionDir := e.workshopRevisionDir(workbenchID, messageID)
	var snapshot draftRevisionSnapshot
	if err := readJSON(filepath.Join(revisionDir, "rev.json"), &snapshot); err != nil {
		return "", err
	}
	if snapshot.RevisionID == "" {
		snapshot.RevisionID = workshopRevisionID(messageID)
	}
	if !snapshot.HasDraft {
		_ = os.RemoveAll(filepath.Join(root, "draft"))
		_ = os.Remove(filepath.Join(metaRoot, "draft.json"))
		_ = os.RemoveAll(filepath.Join(metaRoot, "review"))
		return snapshot.RevisionID, nil
	}

	draftSrc := filepath.Join(revisionDir, "draft_snapshot")
	if _, err := os.Stat(draftSrc); err != nil {
		return "", err
	}
	draftTmp := filepath.Join(root, "draft.restore_tmp")
	_ = os.RemoveAll(draftTmp)
	if err := snapshotDirLight(draftSrc, draftTmp); err != nil {
		return "", err
	}
	_ = os.RemoveAll(filepath.Join(root, "draft"))
	if err := os.Rename(draftTmp, filepath.Join(root, "draft")); err != nil {
		_ = os.RemoveAll(draftTmp)
		return "", err
	}
	if err := snapshotFileLight(filepath.Join(revisionDir, "draft.json"), filepath.Join(metaRoot, "draft.json")); err != nil {
		return "", err
	}
	reviewRoot := filepath.Join(metaRoot, "review")
	_ = os.RemoveAll(reviewRoot)
	if snapshot.DraftID != "" {
		reviewSrc := filepath.Join(revisionDir, "review_snapshot")
		if _, err := os.Stat(reviewSrc); err == nil {
			if err := snapshotDirLight(reviewSrc, filepath.Join(reviewRoot, snapshot.DraftID)); err != nil {
				return "", err
			}
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	return snapshot.RevisionID, nil
}

func snapshotDirLight(src, dest string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		destPath := filepath.Join(dest, entry.Name())
		if entry.IsDir() {
			if err := snapshotDirLight(srcPath, destPath); err != nil {
				return err
			}
			continue
		}
		if err := snapshotFileLight(srcPath, destPath); err != nil {
			return err
		}
	}
	return nil
}

func snapshotFileLight(src, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	_ = os.Remove(dest)
	if err := os.Link(src, dest); err == nil {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o600)
}

func (e *Engine) notifyWorkbenchFilesChanged(workbenchID string, added, removed, updated []string) {
	if e.notify == nil {
		return
	}
	if added == nil {
		added = []string{}
	}
	if removed == nil {
		removed = []string{}
	}
	if updated == nil {
		updated = []string{}
	}
	e.notify("WorkbenchFilesChanged", map[string]any{
		"workbench_id": workbenchID,
		"added":        added,
		"removed":      removed,
		"updated":      updated,
	})
}

func (e *Engine) notifyDraftState(workbenchID string, draft *workbench.DraftState) {
	if e.notify == nil {
		return
	}
	payload := map[string]any{
		"workbench_id": workbenchID,
		"has_draft":    draft != nil,
	}
	if draft != nil && draft.DraftID != "" {
		payload["draft_id"] = draft.DraftID
	}
	if draft != nil && draft.CreatedAt != "" {
		payload["created_at"] = draft.CreatedAt
	}
	source := map[string]any{}
	if draft != nil && draft.SourceKind != "" {
		payload["source_kind"] = draft.SourceKind
		source["kind"] = draft.SourceKind
	}
	if draft != nil && draft.SourceRef != "" {
		payload["source_ref"] = draft.SourceRef
		source["ref"] = draft.SourceRef
		source["job_id"] = draft.SourceRef
	}
	if len(source) > 0 {
		payload["source"] = source
	}
	e.notify("DraftStateChanged", payload)
	e.notify("WorkbenchDraftStateChanged", payload)
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func readJSON(path string, dest interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dest)
}

func writeJSON(path string, payload interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// --- Tool call log (JSONL) ---

// toolLogEntry represents a single tool call logged to the JSONL file.
type toolLogEntry struct {
	ID        int            `json:"id"`
	Tool      string         `json:"tool"`
	Args      string         `json:"args"`
	Result    string         `json:"result"`
	Receipt   string         `json:"receipt"`
	Timestamp string         `json:"ts"`
	ElapsedMS int64          `json:"elapsed_ms"`
	Error     string         `json:"error,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
}

func (e *Engine) toolLogPath(workbenchID string) string {
	return filepath.Join(e.workbenchesRoot(), workbenchID, "meta", "workshop", "tool_log.jsonl")
}

func (e *Engine) appendToolLog(workbenchID string, entry toolLogEntry) error {
	path := e.toolLogPath(workbenchID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func (e *Engine) readToolLogEntry(workbenchID string, entryID int) (toolLogEntry, error) {
	path := e.toolLogPath(workbenchID)
	data, err := os.ReadFile(path)
	if err != nil {
		return toolLogEntry{}, fmt.Errorf("tool log not found: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry toolLogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.ID == entryID {
			return entry, nil
		}
	}
	return toolLogEntry{}, fmt.Errorf("tool log entry %d not found", entryID)
}

// --- Receipt generation ---

// receiptSizeThreshold is the minimum result size (bytes) to trigger receipt
// generation. Results smaller than this are kept as-is.
const receiptSizeThreshold = 2048

// dataReturningTools is the set of tools whose results may be large and should
// be replaced with compact receipts when above the size threshold.
var dataReturningTools = map[string]bool{
	"table_query":     true,
	"table_read_rows": true,
	"table_describe":  true,
	"table_stats":     true,
	"table_get_map":   true,
	"read_file":       true,
	"get_file_map":    true,
	"get_file_info":   true,
	"list_files":      true,
	"xlsx_get_styles": true,
	"docx_get_styles": true,
	"pptx_get_styles": true,
}

// buildToolReceipt generates a compact receipt for a tool result. If the result
// is small enough or the tool is not data-returning, returns the original result.
func buildToolReceipt(toolName string, result string, logEntryID int) string {
	if len(result) <= receiptSizeThreshold {
		return result
	}
	if !dataReturningTools[toolName] {
		return result
	}

	// Try to parse as JSON and extract shape info
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		// Try as JSON array
		var arr []any
		if err := json.Unmarshal([]byte(result), &arr); err != nil {
			// Not JSON — generate a text receipt
			return buildTextReceipt(result, logEntryID)
		}
		return buildArrayReceipt(arr, logEntryID)
	}

	return buildObjectReceipt(toolName, parsed, logEntryID)
}

func buildTextReceipt(result string, logEntryID int) string {
	lines := strings.SplitN(result, "\n", 21)
	preview := lines
	if len(preview) > 20 {
		preview = preview[:20]
	}
	return fmt.Sprintf("[Receipt — %d bytes, %d lines]\nPreview (first %d lines):\n%s\n\nFull result in tool log entry #%d. Use recall_tool_result to retrieve.",
		len(result), len(strings.Split(result, "\n")), len(preview), strings.Join(preview, "\n"), logEntryID)
}

func buildArrayReceipt(arr []any, logEntryID int) string {
	preview := arr
	if len(preview) > 5 {
		preview = preview[:5]
	}
	previewJSON, _ := json.Marshal(preview)
	return fmt.Sprintf("[Receipt — array, %d items]\nPreview (first %d):\n%s\n\nFull result in tool log entry #%d. Use recall_tool_result to retrieve.",
		len(arr), len(preview), string(previewJSON), logEntryID)
}

func buildObjectReceipt(toolName string, parsed map[string]any, logEntryID int) string {
	var sb strings.Builder
	sb.WriteString("[Receipt")

	// Extract shape info common to tabular results
	if rowCount, ok := numericField(parsed, "row_count"); ok {
		sb.WriteString(fmt.Sprintf(" — %d rows", int(rowCount)))
	}
	if totalRows, ok := numericField(parsed, "total_rows"); ok {
		sb.WriteString(fmt.Sprintf(", %d total", int(totalRows)))
	}
	if hasMore, ok := parsed["has_more"].(bool); ok && hasMore {
		sb.WriteString(", has_more=true")
	}
	sb.WriteString("]\n")

	// Extract column names if present
	if cols, ok := parsed["columns"]; ok {
		if colJSON, err := json.Marshal(cols); err == nil {
			sb.WriteString("Columns: ")
			sb.WriteString(string(colJSON))
			sb.WriteString("\n")
		}
	}

	// Preview: first few rows if present
	if rows, ok := parsed["rows"].([]any); ok && len(rows) > 0 {
		previewRows := rows
		if len(previewRows) > 5 {
			previewRows = previewRows[:5]
		}
		previewJSON, _ := json.MarshalIndent(previewRows, "", "  ")
		sb.WriteString(fmt.Sprintf("Preview (%d of %d rows):\n", len(previewRows), len(rows)))
		sb.WriteString(string(previewJSON))
		sb.WriteString("\n")
	} else if data, ok := parsed["data"].([]any); ok && len(data) > 0 {
		previewData := data
		if len(previewData) > 5 {
			previewData = previewData[:5]
		}
		previewJSON, _ := json.MarshalIndent(previewData, "", "  ")
		sb.WriteString(fmt.Sprintf("Preview (%d of %d items):\n", len(previewData), len(data)))
		sb.WriteString(string(previewJSON))
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("\nFull result in tool log entry #%d. Use recall_tool_result to retrieve.", logEntryID))
	return sb.String()
}

// numericField extracts a numeric value from a map field, handling both
// float64 (from JSON) and int types.
func numericField(m map[string]any, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	}
	return 0, false
}

// --- Payload size estimation ---

// estimatePayloadBytes approximates the total payload size for the messages
// that will be sent to the LLM API.
func estimatePayloadBytes(messages []llm.ChatMessage) int {
	total := 0
	for _, m := range messages {
		total += len(m.Content)
		for _, tc := range m.ToolCalls {
			total += len(tc.Function.Name) + len(tc.Function.Arguments)
		}
	}
	return total
}

// truncateHistoricalToolResult generates a compact placeholder for old tool
// results that exceed the receipt threshold when replaying conversation history.
func truncateHistoricalToolResult(result string) string {
	return fmt.Sprintf("[Historical tool result, %d bytes. Data was logged at execution time.]", len(result))
}
