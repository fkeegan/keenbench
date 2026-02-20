package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContextRPCDirectEditLifecycle(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	seedContextCategory(t, eng, workbenchID, contextCategoryCompany, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "---\nname: company-context\ndescription: Keep company priorities in mind.\n---\n\n# Company Context\n\nSee [summary](references/summary.md).\n",
		},
		{
			Path:    "references/summary.md",
			Content: "- Company: KeenBench\n- Audience: B2B operators",
		},
	})

	notifications := []string{}
	eng.SetNotifier(func(method string, params any) {
		notifications = append(notifications, method)
	})

	listResp, errInfo := eng.ContextList(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("list: %v", errInfo)
	}
	items, ok := listResp.(map[string]any)["items"].([]contextListItem)
	if !ok {
		t.Fatalf("expected []contextListItem payload")
	}
	if len(items) != len(contextCategories) {
		t.Fatalf("expected %d categories, got %d", len(contextCategories), len(items))
	}
	company := contextListItem{}
	for _, item := range items {
		if item.Category == contextCategoryCompany {
			company = item
			break
		}
	}
	if company.Status != "active" {
		t.Fatalf("expected active company context, got %q", company.Status)
	}
	if company.Summary != "Keep company priorities in mind." {
		t.Fatalf("unexpected company summary: %q", company.Summary)
	}

	artifactResp, errInfo := eng.ContextGetArtifact(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
	}))
	if errInfo != nil {
		t.Fatalf("get artifact: %v", errInfo)
	}
	artifactPayload := artifactResp.(map[string]any)
	if artifactPayload["has_direct_edits"] != false {
		t.Fatalf("expected has_direct_edits=false")
	}
	artifactFiles, ok := artifactPayload["files"].([]contextArtifactFile)
	if !ok || len(artifactFiles) != 2 {
		t.Fatalf("expected 2 artifact files, got %#v", artifactPayload["files"])
	}

	updateResp, errInfo := eng.ContextUpdateDirect(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"files": []map[string]any{
			{
				"path":    "SKILL.md",
				"content": "---\nname: company-context\ndescription: Updated direct edit description.\n---\n\n# Company Context\n\nSee [summary](references/summary.md).\n",
			},
			{
				"path":    "references/summary.md",
				"content": "Updated summary after direct edit.",
			},
		},
	}))
	if errInfo != nil {
		t.Fatalf("update direct: %v", errInfo)
	}
	updatedItem, ok := updateResp.(map[string]any)["item"].(*contextItem)
	if !ok {
		t.Fatalf("expected contextItem payload")
	}
	if !updatedItem.HasDirectEdits {
		t.Fatalf("expected direct-edit flag to be true")
	}
	if strings.TrimSpace(updatedItem.LastDirectEdit) == "" {
		t.Fatalf("expected last_direct_edit timestamp")
	}
	if updatedItem.Summary != "Updated direct edit description." {
		t.Fatalf("expected summary from edited SKILL frontmatter, got %q", updatedItem.Summary)
	}

	if _, errInfo := eng.ContextDelete(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
	})); errInfo != nil {
		t.Fatalf("delete: %v", errInfo)
	}

	getResp, errInfo := eng.ContextGet(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
	}))
	if errInfo != nil {
		t.Fatalf("get after delete: %v", errInfo)
	}
	var deletedItem contextItem
	raw, err := json.Marshal(getResp.(map[string]any)["item"])
	if err != nil {
		t.Fatalf("marshal deleted item: %v", err)
	}
	if err := json.Unmarshal(raw, &deletedItem); err != nil {
		t.Fatalf("unmarshal deleted item: %v", err)
	}
	if deletedItem.Status != "empty" {
		t.Fatalf("expected empty status after delete, got %q", deletedItem.Status)
	}

	if countMethod(notifications, "ContextChanged") < 2 {
		t.Fatalf("expected context change notifications, got %#v", notifications)
	}
	if countMethod(notifications, "WorkbenchClutterChanged") < 2 {
		t.Fatalf("expected clutter notifications, got %#v", notifications)
	}
}

func TestContextMutationBlockedWhenDraftExists(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	seedContextCategory(t, eng, workbenchID, contextCategoryCompany, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "---\nname: company-context\ndescription: Company context.\n---\n\n# Company Context\n\nSee [summary](references/summary.md).\n",
		},
		{
			Path:    "references/summary.md",
			Content: "Summary.",
		},
	})

	if _, err := eng.workbenches.CreateDraftWithSource(workbenchID, "workshop", "assistant"); err != nil {
		t.Fatalf("create draft: %v", err)
	}

	_, errInfo := eng.ContextUpdateDirect(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"files": []map[string]any{
			{
				"path":    "SKILL.md",
				"content": "---\nname: company-context\ndescription: blocked\n---\n",
			},
		},
	}))
	if errInfo == nil {
		t.Fatalf("expected update direct to be blocked by draft")
	}
	if errInfo.ErrorCode != "VALIDATION_FAILED" || !strings.Contains(strings.ToLower(errInfo.Detail), "draft exists") {
		t.Fatalf("unexpected error: %#v", errInfo)
	}

	_, errInfo = eng.ContextDelete(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
	}))
	if errInfo == nil {
		t.Fatalf("expected delete to be blocked by draft")
	}
	if errInfo.ErrorCode != "VALIDATION_FAILED" || !strings.Contains(strings.ToLower(errInfo.Detail), "draft exists") {
		t.Fatalf("unexpected delete error: %#v", errInfo)
	}
}

func TestContextProcessEmitsAddedThenUpdatedAction(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	eng.providers[ProviderOpenAI] = &testOpenAI{
		chatResponse: `{"summary":"Company context summary","files":[{"path":"SKILL.md","content":"---\nname: company-context\ndescription: Company context skill.\n---\n\n# Company Context\n\nSee [summary](references/summary.md)."},{"path":"references/summary.md","content":"Company summary facts."}]}`,
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"enabled":     true,
	})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}
	status, errInfo := eng.EgressGetConsentStatus(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("consent status: %v", errInfo)
	}
	statusMap := status.(map[string]any)
	if _, errInfo := eng.EgressGrantWorkshopConsent(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"provider_id":  statusMap["provider_id"],
		"model_id":     statusMap["model_id"],
		"scope_hash":   statusMap["scope_hash"],
		"persist":      false,
	})); errInfo != nil {
		t.Fatalf("grant consent: %v", errInfo)
	}

	contextActions := []string{}
	eng.SetNotifier(func(method string, params any) {
		if method != "ContextChanged" {
			return
		}
		payload, ok := params.(map[string]any)
		if !ok {
			return
		}
		action, _ := payload["action"].(string)
		if strings.TrimSpace(action) != "" {
			contextActions = append(contextActions, action)
		}
	})

	if _, errInfo := eng.ContextProcess(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"input": map[string]any{
			"mode": "text",
			"text": "Initial company context source",
		},
	})); errInfo != nil {
		t.Fatalf("first process: %v", errInfo)
	}

	if _, errInfo := eng.ContextProcess(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"input": map[string]any{
			"mode": "text",
			"text": "Updated company context source",
		},
	})); errInfo != nil {
		t.Fatalf("second process: %v", errInfo)
	}

	if len(contextActions) < 2 {
		t.Fatalf("expected at least 2 context actions, got %#v", contextActions)
	}
	if contextActions[0] != "added" {
		t.Fatalf("expected first action=added, got %q", contextActions[0])
	}
	if contextActions[1] != "updated" {
		t.Fatalf("expected second action=updated, got %q", contextActions[1])
	}
}

func TestContextProcessRequiresConsentWhenAskMode(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	eng.providers[ProviderOpenAI] = &testOpenAI{
		chatResponse: `{"summary":"Company context summary","files":[{"path":"SKILL.md","content":"---\nname: company-context\ndescription: Company context skill.\n---\n\n# Company Context\n\nSee [summary](references/summary.md)."},{"path":"references/summary.md","content":"Company summary facts."}]}`,
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"enabled":     true,
	})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}

	_, errInfo := eng.ContextProcess(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"input": map[string]any{
			"mode": "text",
			"text": "Initial company context source",
		},
	}))
	if errInfo == nil || errInfo.ErrorCode != "EGRESS_CONSENT_REQUIRED" {
		t.Fatalf("expected consent required error, got %#v", errInfo)
	}
}

func TestContextProcessAllowsWhenGlobalConsentModeEnabled(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	eng.providers[ProviderOpenAI] = &testOpenAI{
		chatResponse: `{"summary":"Company context summary","files":[{"path":"SKILL.md","content":"---\nname: company-context\ndescription: Company context skill.\n---\n\n# Company Context\n\nSee [summary](references/summary.md)."},{"path":"references/summary.md","content":"Company summary facts."}]}`,
	}

	if _, errInfo := eng.ProvidersSetApiKey(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"api_key":     "sk-test",
	})); errInfo != nil {
		t.Fatalf("set key: %v", errInfo)
	}
	if _, errInfo := eng.ProvidersSetEnabled(ctx, mustJSON(t, map[string]any{
		"provider_id": "openai",
		"enabled":     true,
	})); errInfo != nil {
		t.Fatalf("set enabled: %v", errInfo)
	}
	if _, errInfo := eng.UserSetConsentMode(ctx, mustJSON(t, map[string]any{
		"mode":     "allow_all",
		"approved": true,
	})); errInfo != nil {
		t.Fatalf("set consent mode: %v", errInfo)
	}

	if _, errInfo := eng.ContextProcess(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"category":     contextCategoryCompany,
		"input": map[string]any{
			"mode": "text",
			"text": "Initial company context source",
		},
	})); errInfo != nil {
		t.Fatalf("process with allow_all mode: %v", errInfo)
	}
}

func TestParseContextModelOutputRecoversFromJSONFence(t *testing.T) {
	output := "Here is the result:\n```json\n{\n  \"summary\": \"Company summary\",\n  \"files\": [\n    {\"path\": \"SKILL.md\", \"content\": \"---\\nname: company-context\\ndescription: Company context skill.\\n---\\n\\nBody\"},\n    {\"path\": \"references/summary.md\", \"content\": \"Facts\"}\n  ]\n}\n```\n"

	parsed, err := parseContextModelOutput(output)
	if err != nil {
		t.Fatalf("expected fenced json to parse, got %v", err)
	}
	if parsed.Summary != "Company summary" {
		t.Fatalf("unexpected summary: %q", parsed.Summary)
	}
	if len(parsed.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(parsed.Files))
	}
}

func TestParseContextModelOutputRecoversFromJSONStringWrapper(t *testing.T) {
	output := `"{\"summary\":\"Company summary\",\"files\":[{\"path\":\"SKILL.md\",\"content\":\"---\\nname: company-context\\ndescription: Company context skill.\\n---\\n\\nBody\"},{\"path\":\"references/summary.md\",\"content\":\"Facts\"}]}"` //nolint:lll

	parsed, err := parseContextModelOutput(output)
	if err != nil {
		t.Fatalf("expected wrapped json string to parse, got %v", err)
	}
	if parsed.Summary != "Company summary" {
		t.Fatalf("unexpected summary: %q", parsed.Summary)
	}
	if len(parsed.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(parsed.Files))
	}
}

func TestParseContextModelOutputRecoversFromArrayWrapper(t *testing.T) {
	output := `[{"summary":"Company summary","files":[{"path":"SKILL.md","content":"---\nname: company-context\ndescription: Company context skill.\n---\n\nBody"},{"path":"references/summary.md","content":"Facts"}]}]`

	parsed, err := parseContextModelOutput(output)
	if err != nil {
		t.Fatalf("expected single-item array to parse, got %v", err)
	}
	if parsed.Summary != "Company summary" {
		t.Fatalf("unexpected summary: %q", parsed.Summary)
	}
	if len(parsed.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(parsed.Files))
	}
}

func TestParseContextModelOutputRejectsMissingFiles(t *testing.T) {
	if _, err := parseContextModelOutput(`{"summary":"only summary"}`); err == nil {
		t.Fatalf("expected missing files to fail")
	} else if !strings.Contains(err.Error(), "processing output missing files") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateContextArtifactsDocumentStyleRequiresStyleRules(t *testing.T) {
	err := validateContextArtifacts(contextCategoryDocumentStyle, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "---\nname: document-style\ndescription: Style rules.\n---\n\n# Style\nUse concise writing.\n",
		},
	})
	if err == nil {
		t.Fatalf("expected document-style validation error when style-rules is missing")
	}
	if !strings.Contains(err.Error(), "references/style-rules.md is required") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestExtractContextInputTextCleansStagingDirectory(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)

	source := filepath.Join(t.TempDir(), "input.docx")
	if err := os.WriteFile(source, []byte("docx source content"), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	text, filename, errInfo := eng.extractContextInputText(ctx, workbenchID, source)
	if errInfo != nil {
		t.Fatalf("extract context input: %v", errInfo)
	}
	if text != "docx source content" {
		t.Fatalf("unexpected extracted text: %q", text)
	}
	if filename != "input.docx" {
		t.Fatalf("unexpected source filename: %q", filename)
	}

	workbenchRoot := filepath.Join(eng.workbenchesRoot(), workbenchID)
	entries, err := os.ReadDir(workbenchRoot)
	if err != nil {
		t.Fatalf("read workbench root: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "contexttmp-") {
			t.Fatalf("staging directory was not cleaned up: %s", entry.Name())
		}
	}
}

func TestWorkbenchContextInjectionAndClutterMetrics(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)

	seedContextCategory(t, eng, workbenchID, contextCategoryCompany, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "---\nname: company-context\ndescription: Company directives.\n---\n\n# Company Context\n\nSee [summary](references/summary.md).\n",
		},
		{
			Path:    "references/summary.md",
			Content: "Company facts.",
		},
	})
	seedContextCategory(t, eng, workbenchID, contextCategorySituation, []contextArtifactFile{
		{
			Path:    "context.md",
			Content: "## Situation\n\n- Audience: board\n- Deadline: Friday\n" + strings.Repeat("token ", 90000),
		},
	})

	injection, estimatedTokens := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-situation>") {
		t.Fatalf("expected situation block in injection")
	}
	if !strings.Contains(injection, "<workbench-skill name=\"company-context\">") {
		t.Fatalf("expected company skill block in injection")
	}
	if !strings.Contains(injection, "## references/summary.md") {
		t.Fatalf("expected inlined reference content in injection")
	}
	if estimatedTokens <= 0 {
		t.Fatalf("expected non-zero token estimate")
	}

	messages, errInfo := eng.buildChatMessages(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build chat messages: %v", errInfo)
	}
	if len(messages) == 0 || !strings.Contains(messages[0].Content, "Workbench context:") {
		t.Fatalf("expected workbench context in chat system prompt")
	}

	proposalPrompt, errInfo := eng.buildProposalPrompt(ctx, workbenchID)
	if errInfo != nil {
		t.Fatalf("build proposal prompt: %v", errInfo)
	}
	if !strings.Contains(proposalPrompt, "Workbench context:") {
		t.Fatalf("expected workbench context in proposal prompt")
	}

	clutterResp, errInfo := eng.WorkbenchGetClutter(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
	}))
	if errInfo != nil {
		t.Fatalf("clutter: %v", errInfo)
	}
	clutterPayload := clutterResp.(map[string]any)
	if clutterPayload["context_items_weight"].(float64) <= 0 {
		t.Fatalf("expected positive context_items_weight")
	}
	if clutterPayload["context_share"].(float64) <= 0.35 {
		t.Fatalf("expected context_share > 0.35, got %v", clutterPayload["context_share"])
	}
	if clutterPayload["context_warning"] != true {
		t.Fatalf("expected context_warning=true, got %v", clutterPayload["context_warning"])
	}
}

func TestWorkbenchContextInjectionKeepsStandaloneDocumentStyleWithoutFormatSignals(t *testing.T) {
	eng, workbenchID := newContextTestEngine(t)
	seedValidDocumentStyleContext(t, eng, workbenchID, "Use brand color 0B3A5B for heading text.")

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"document-style\">") {
		t.Fatalf("expected standalone document-style skill when no format is relevant")
	}
	if strings.Contains(injection, "xlsx-style-skill") || strings.Contains(injection, "docx-style-skill") || strings.Contains(injection, "pptx-style-skill") {
		t.Fatalf("did not expect format style skill injection without manifest or intent")
	}
}

func TestWorkbenchContextInjectionFormatGatedByManifestSuppressesStandaloneDocumentStyle(t *testing.T) {
	ctx := context.Background()
	eng, workbenchID := newContextTestEngine(t)
	seedContextCategory(t, eng, workbenchID, contextCategoryCompany, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "---\nname: company-context\ndescription: Company directives.\n---\n\n# Company Context\n\nSee [summary](references/summary.md).\n",
		},
		{
			Path:    "references/summary.md",
			Content: "Company facts.",
		},
	})
	seedValidDocumentStyleContext(t, eng, workbenchID, "Use brand color 0B3A5B for header fills.")

	xlsxPath := filepath.Join(t.TempDir(), "budget.xlsx")
	if err := os.WriteFile(xlsxPath, []byte("fake xlsx bytes"), 0o600); err != nil {
		t.Fatalf("write xlsx source: %v", err)
	}
	if _, errInfo := eng.WorkbenchFilesAdd(ctx, mustJSON(t, map[string]any{
		"workbench_id": workbenchID,
		"source_paths": []string{xlsxPath},
	})); errInfo != nil {
		t.Fatalf("add xlsx file: %v", errInfo)
	}

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"company-context\">") {
		t.Fatalf("expected existing company context to remain injected")
	}
	if !strings.Contains(injection, "<workbench-skill name=\"xlsx-style-custom\">") {
		t.Fatalf("expected merged xlsx custom style skill when document-style is present")
	}
	if strings.Contains(injection, "<workbench-skill name=\"document-style\">") {
		t.Fatalf("expected standalone document-style to be suppressed when format style is injected")
	}
}

func TestWorkbenchContextInjectionFormatGatedByIntent(t *testing.T) {
	eng, workbenchID := newContextTestEngine(t)
	if err := eng.appendConversation(workbenchID, conversationMessage{
		Type:      "user_message",
		MessageID: "u-intent-1",
		Role:      "user",
		Text:      "Please create a slide deck presentation for the board update.",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("append conversation: %v", err)
	}

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"pptx-style-skill\">") {
		t.Fatalf("expected pptx bundled style skill from conversation intent")
	}
	if strings.Contains(injection, "<workbench-skill name=\"pptx-style-custom\">") {
		t.Fatalf("did not expect custom merge without document-style context")
	}
}

func TestWorkbenchContextInjectionDocumentStyleMergeSuccessPreservesCatalog(t *testing.T) {
	eng, workbenchID := newContextTestEngine(t)
	seedValidDocumentStyleContext(t, eng, workbenchID, "Use brand color 0B3A5B for header fills.")
	if err := eng.appendConversation(workbenchID, conversationMessage{
		Type:      "user_message",
		MessageID: "u-intent-2",
		Role:      "user",
		Text:      "Build an excel spreadsheet for operating metrics.",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("append conversation: %v", err)
	}

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"xlsx-style-custom\">") {
		t.Fatalf("expected merged xlsx-style-custom skill")
	}
	if strings.Contains(injection, "<workbench-skill name=\"document-style\">") {
		t.Fatalf("expected standalone document-style suppression during merged format injection")
	}
	if !strings.Contains(injection, "Use only parameters in this catalog when calling spreadsheet write operations.") {
		t.Fatalf("expected generic tool capability catalog section to be preserved")
	}
	if !strings.Contains(injection, "Use brand color 0B3A5B for header fills.") {
		t.Fatalf("expected user style rule to be merged into injected format skill")
	}
}

func TestWorkbenchContextInjectionDocumentStyleMergeFailureFallsBackToGeneric(t *testing.T) {
	eng, workbenchID := newContextTestEngine(t)
	seedContextCategory(t, eng, workbenchID, contextCategoryDocumentStyle, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: "name: broken-document-style\nmissing frontmatter fence",
		},
		{
			Path:    "references/style-rules.md",
			Content: "Use sharp contrast for section headers.",
		},
	})
	if err := eng.appendConversation(workbenchID, conversationMessage{
		Type:      "user_message",
		MessageID: "u-intent-3",
		Role:      "user",
		Text:      "Create a spreadsheet for staffing plans.",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("append conversation: %v", err)
	}

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"xlsx-style-skill\">") {
		t.Fatalf("expected fallback generic xlsx style skill when merge fails")
	}
	if strings.Contains(injection, "<workbench-skill name=\"xlsx-style-custom\">") {
		t.Fatalf("did not expect custom style skill when merge fails")
	}
	if strings.Contains(injection, "<workbench-skill name=\"document-style\">") {
		t.Fatalf("expected standalone document-style suppression when fallback generic format skill is injected")
	}
	assertStyleNotice(t, eng, workbenchID, "STYLE_MERGE_FAILED", "xlsx")
}

func TestWorkbenchContextInjectionStyleSkillLoadFailureKeepsStandaloneDocumentStyle(t *testing.T) {
	t.Setenv("KEENBENCH_BUNDLED_SKILLS_DIR", filepath.Join(t.TempDir(), "missing-bundled-skills"))

	eng, workbenchID := newContextTestEngine(t)
	seedValidDocumentStyleContext(t, eng, workbenchID, "Use subtle borders for data grids.")
	if err := eng.appendConversation(workbenchID, conversationMessage{
		Type:      "user_message",
		MessageID: "u-intent-4",
		Role:      "user",
		Text:      "Please create a spreadsheet model.",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("append conversation: %v", err)
	}

	injection, _ := eng.buildWorkbenchContextInjection(workbenchID)
	if !strings.Contains(injection, "<workbench-skill name=\"document-style\">") {
		t.Fatalf("expected standalone document-style when bundled format skill cannot be loaded")
	}
	if strings.Contains(injection, "xlsx-style-skill") || strings.Contains(injection, "xlsx-style-custom") {
		t.Fatalf("did not expect xlsx format style skill when bundled skill load fails")
	}
	assertStyleNotice(t, eng, workbenchID, "STYLE_SKILL_LOAD_FAILED", "xlsx")
}

func newContextTestEngine(t *testing.T) (*Engine, string) {
	t.Helper()
	t.Setenv("KEENBENCH_DATA_DIR", t.TempDir())
	t.Setenv("KEENBENCH_FAKE_TOOL_WORKER", "1")

	eng, err := New()
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	createResp, errInfo := eng.WorkbenchCreate(context.Background(), mustJSON(t, map[string]any{
		"name": "Context Test",
	}))
	if errInfo != nil {
		t.Fatalf("create workbench: %v", errInfo)
	}
	workbenchID := createResp.(map[string]any)["workbench_id"].(string)
	return eng, workbenchID
}

func seedContextCategory(t *testing.T, eng *Engine, workbenchID, category string, files []contextArtifactFile) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	source := &contextSourceMetadata{
		Mode:            contextInputModeText,
		Text:            "seed source",
		CreatedAt:       now,
		LastProcessedAt: now,
		ModelID:         ModelOpenAIID,
		HasDirectEdits:  false,
	}
	if err := eng.writeContextCategory(workbenchID, category, source, files, "", false); err != nil {
		t.Fatalf("seed context category %s: %v", category, err)
	}
}

func seedValidDocumentStyleContext(t *testing.T, eng *Engine, workbenchID, styleRule string) {
	t.Helper()
	skill := "---\nname: document-style\ndescription: Team styling rules for generated office files.\n---\n\n# Document Style\n\n## Formatting Rules\n- Keep visual hierarchy consistent.\n- Prefer concise labels and readable typography.\n- See [style rules](references/style-rules.md).\n\n## Worked Examples\n- Use emphasized headers and restrained accent colors.\n"
	rules := "- Use 11pt body text for narrative content.\n"
	if strings.TrimSpace(styleRule) != "" {
		rules += "- " + strings.TrimSpace(styleRule) + "\n"
	}
	seedContextCategory(t, eng, workbenchID, contextCategoryDocumentStyle, []contextArtifactFile{
		{
			Path:    "SKILL.md",
			Content: skill,
		},
		{
			Path:    "references/style-rules.md",
			Content: rules,
		},
	})
}

func assertStyleNotice(t *testing.T, eng *Engine, workbenchID, code, format string) {
	t.Helper()
	items, err := eng.readConversation(workbenchID)
	if err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	for _, item := range items {
		if item.Type != "system_event" || item.EventKind != "style_notice" {
			continue
		}
		if item.Metadata == nil {
			continue
		}
		gotCode := strings.TrimSpace(stringValue(item.Metadata["error_code"]))
		gotFormat := strings.TrimSpace(strings.ToLower(stringValue(item.Metadata["format"])))
		if gotCode == strings.TrimSpace(code) && gotFormat == strings.TrimSpace(strings.ToLower(format)) {
			return
		}
	}
	t.Fatalf("expected style notice code=%s format=%s", code, format)
}

func countMethod(methods []string, method string) int {
	count := 0
	for _, value := range methods {
		if value == method {
			count++
		}
	}
	return count
}
