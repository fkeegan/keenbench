# KeenBench Provider & Model Selection Test Plan (Short)

## Test Plan Overview

**Document Version:** 1.0
**Date:** 2026-03-11
**Application:** KeenBench Desktop Application
**Test Scope:** Provider/model selection, OpenRouter catalog handling, and provider-specific error messaging
**Test Type:** Manual Black Box Testing

## Test Objectives

1. Validate the user default provider/model is applied to new Workbenches.
2. Verify Workbench provider changes switch to a valid default model for the selected provider.
3. Confirm OpenRouter models are shown only inside the OpenRouter model picker, with free models first.
4. Verify large model lists support search.
5. Confirm configured providers can still answer prompts after switching.
6. Confirm provider-specific failures are surfaced clearly instead of collapsing into the wrong error state.

Assumptions for this short plan:
- The app is already running.
- Credentials are already configured for OpenRouter, Anthropic Claude, and OpenAI Codex.
- The request referenced "opencode"; this plan uses the current in-app provider label `OpenRouter`.
- The default provider is `OpenRouter` and the default model is `openrouter:openrouter/free`.
- Before starting, choose a short unique suffix for this test run, referred to below as `<RUN_ID>`. Use either a 4-digit random string such as `4821` or the current `DDHHMMSS` time such as `11153045`.
- Use a simple prompt such as `Reply with READY only.` when a test asks for a successful response.

---

## 1. Default Selection

### Test Case: TC-PM-001 - New Workbench Inherits the Default OpenRouter Selection
**Priority:** Critical
**Type:** Functional
**Preconditions:**
1. The Home screen is visible.
2. In Settings, `Default Provider` is `OpenRouter`.
3. In Settings, `Default Model` is `openrouter:openrouter/free`.
4. A short unique suffix has been chosen for this test run as `<RUN_ID>`.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Verify the Home screen shows the `Workbenches` heading and the `New Workbench` button | The Home screen is ready for workbench creation |
| 2 | Click `New Workbench` | A dialog titled `New Workbench` opens |
| 3 | Click the `Workbench name` field | The text field gains focus and a cursor appears |
| 4 | Enter `PM Default OpenRouter-<RUN_ID>` | The name appears in the field |
| 5 | Click `Create` | The dialog closes and the new Workbench is displayed |
| 6 | Verify the Workbench title is `PM Default OpenRouter-<RUN_ID>` | The Workbench screen is visible with the new name in the title bar |
| 7 | Verify the left sidebar shows `Workbench Files` and the composer is visible at the bottom | The Workbench layout is fully loaded |
| 8 | Read the `Provider` selector in the top-right area of the header | `OpenRouter` is selected |
| 9 | Click the `Model` selector | The `Select model` dialog opens |
| 10 | Read the selected row in the model list | `openrouter:openrouter/free` is selected and marked `Free` |
| 11 | Click the selected `openrouter:openrouter/free` row to close the picker | The model picker closes and the Workbench is visible again |
| 12 | Click `Ask` if `Agent` is currently selected | `Ask` is the active composer mode |
| 13 | Click the composer field | The composer gains focus |
| 14 | Enter `Reply with READY only.` | The prompt text is visible in the composer |
| 15 | Click `Send` | If a provider consent dialog appears, it is shown for `OpenRouter`; otherwise the request starts immediately |
| 16 | If a provider consent dialog appears, approve the request | The dialog closes and the assistant run starts |
| 17 | Wait for the response to finish | One assistant reply is shown and the run succeeds |

---

## 2. Default Settings Persistence

### Test Case: TC-PM-002 - Changing the User Default Affects New Workbenches Only
**Priority:** High
**Type:** Functional
**Preconditions:**
1. The Workbench `PM Default OpenRouter-<RUN_ID>` from TC-PM-001 exists.
2. `Anthropic Claude` is configured and enabled.
3. The same `<RUN_ID>` chosen earlier is available.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Click the Back arrow in the top-left corner of the Workbench | The Home screen is displayed |
| 2 | Click `Settings` in the Home screen app bar | The `Settings` screen opens |
| 3 | Scroll to the `Default Provider` section if needed | The default provider and default model controls are visible |
| 4 | Open the `Default provider` dropdown | The list of available providers opens |
| 5 | Select `Anthropic Claude` | The dropdown closes and `Anthropic Claude` is shown as the selected default provider |
| 6 | Click the `Default model` field | The `Select default model` dialog opens |
| 7 | Read the selected row in the model list | `anthropic-claude:claude-sonnet-4-6` is selected |
| 8 | Click the selected `anthropic-claude:claude-sonnet-4-6` row to close the picker | The picker closes and the `Default model` field remains set |
| 9 | Click the Back arrow in the top-left corner | The Home screen is displayed |
| 10 | Click `New Workbench` | The `New Workbench` dialog opens |
| 11 | Enter `PM Anthropic Default-<RUN_ID>` in the `Workbench name` field | The new name is visible in the field |
| 12 | Click `Create` | The new Workbench opens |
| 13 | Read the `Provider` selector | `Anthropic Claude` is selected |
| 14 | Click the `Model` selector | The `Select model` dialog opens |
| 15 | Read the selected row | `anthropic-claude:claude-sonnet-4-6` is selected |
| 16 | Click the Back arrow in the top-left corner | The Home screen is displayed |
| 17 | Click the `PM Default OpenRouter-<RUN_ID>` Workbench tile | The original Workbench opens |
| 18 | Read the `Provider` selector in the original Workbench | `OpenRouter` is still selected |

---

## 3. Provider Switching

### Test Case: TC-PM-003 - Switching Providers Selects the Correct Default Model
**Priority:** Critical
**Type:** Functional
**Preconditions:**
1. Any Workbench is open.
2. `Anthropic Claude` is configured and enabled.
3. `OpenAI Codex` is configured and enabled.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Click the `Provider` selector in the Workbench header | The provider dropdown opens |
| 2 | Select `Anthropic Claude` | The dropdown closes and the selector updates to `Anthropic Claude` |
| 3 | Click the `Model` selector | The `Select model` dialog opens |
| 4 | Read the selected row | `anthropic-claude:claude-sonnet-4-6` is selected |
| 5 | Click the selected Anthropic row to close the picker | The picker closes and the Workbench is visible again |
| 6 | Click `Ask` if `Agent` is currently selected | `Ask` is the active composer mode |
| 7 | Click the composer field | The composer gains focus |
| 8 | Enter `Reply with READY only.` | The prompt text is visible in the composer |
| 9 | Click `Send` | If a consent dialog appears, it is for `Anthropic Claude`; otherwise the run starts immediately |
| 10 | If a provider consent dialog appears, approve the request | The dialog closes and the assistant run starts |
| 11 | Wait for the response to finish | One assistant reply is shown |
| 12 | Click the `Provider` selector again | The provider dropdown opens |
| 13 | Select `OpenAI Codex` | The dropdown closes and the selector updates to `OpenAI Codex` |
| 14 | Click the `Model` selector | The `Select model` dialog opens |
| 15 | Read the selected row | `openai-codex:gpt-5.4` is selected |
| 16 | Click the selected OpenAI Codex row to close the picker | The picker closes and the Workbench is visible again |
| 17 | Click `Ask` if `Agent` is currently selected | `Ask` is the active composer mode |
| 18 | Click the composer field | The composer gains focus |
| 19 | Enter `Reply with READY only.` | The prompt text is visible in the composer |
| 20 | Click `Send` | If a consent dialog appears, it is for `OpenAI Codex`; otherwise the run starts immediately |
| 21 | If a provider consent dialog appears, approve the request | The dialog closes and the assistant run starts |
| 22 | Wait for the response to finish | One assistant reply is shown and the provider remains `OpenAI Codex` |

---

## 4. OpenRouter Catalog UX

### Test Case: TC-PM-004 - OpenRouter Model Picker Is Provider-Scoped, Searchable, and Free-First
**Priority:** High
**Type:** Functional
**Preconditions:**
1. A Workbench is open.
2. `OpenRouter` is configured and enabled.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Click the `Provider` selector in the Workbench header | The provider dropdown opens |
| 2 | Select `OpenRouter` | The dropdown closes and the selector updates to `OpenRouter` |
| 3 | Click the `Model` selector | The `Select model` dialog opens |
| 4 | Verify the dialog contains a `Search models` field | The search field is visible at the top of the picker |
| 5 | Read the model IDs shown in the first several visible rows | Each visible model ID starts with `openrouter:` |
| 6 | Continue reading rows until the first entry without a `Free` badge appears | Every visible entry before that point is marked `Free` |
| 7 | Click the `Search models` field | The search field gains focus |
| 8 | Enter `:free` | The list filters immediately |
| 9 | Read the visible filtered results | Each visible result contains `:free` in the model ID and no Anthropic or OpenAI Codex models are shown |
| 10 | Clear the search field | The full OpenRouter list returns |
| 11 | Click `Cancel` | The picker closes and the Workbench is visible again |

---

### Test Case: TC-PM-005 - A Different Free OpenRouter Model Can Be Selected and Used
**Priority:** High
**Type:** Functional
**Preconditions:**
1. A Workbench is open.
2. `OpenRouter` is configured and enabled.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Click the `Provider` selector in the Workbench header | The provider dropdown opens |
| 2 | Select `OpenRouter` | The dropdown closes and `OpenRouter` becomes the active provider |
| 3 | Click the `Model` selector | The `Select model` dialog opens |
| 4 | Click the `Search models` field | The search field gains focus |
| 5 | Enter `openrouter:arcee-ai/trinity-mini:free` | The list filters to matching results |
| 6 | Click the row for `openrouter:arcee-ai/trinity-mini:free` | The picker closes and the model selector updates |
| 7 | Read the model selector label in the Workbench header | It shows the newly selected free OpenRouter model |
| 8 | Click `Ask` if `Agent` is currently selected | `Ask` is the active composer mode |
| 9 | Click the composer field | The composer gains focus |
| 10 | Enter `Reply with READY only.` | The prompt text is visible in the composer |
| 11 | Click `Send` | If a provider consent dialog appears, it is for `OpenRouter`; otherwise the run starts immediately |
| 12 | If a provider consent dialog appears, approve the request | The dialog closes and the assistant run starts |
| 13 | Wait for the response to finish | One assistant reply is shown and the run succeeds |

---

## 5. Error Handling

### Test Case: TC-PM-006 - Invalid OpenRouter Credential Shows a Provider-Specific Validation Error
**Priority:** Critical
**Type:** Negative
**Preconditions:**
1. `OpenRouter` is currently configured with a known-working key.
2. The tester can restore the original OpenRouter key after the test.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Click the Back arrow until the Home screen is visible | The Home screen is displayed |
| 2 | Click `Settings` | The `Settings` screen opens |
| 3 | Scroll to the `OpenRouter` provider card | The OpenRouter credential controls are visible |
| 4 | Click into the OpenRouter API key field | The field gains focus |
| 5 | Replace the saved key with `invalid-openrouter-key-for-test` | The invalid value is visible in the field |
| 6 | Click `Save & Validate` | Validation starts |
| 7 | Wait for validation to finish | A provider-specific validation error is shown; it is not collapsed into a generic missing-key message |
| 8 | Click the OpenRouter API key field again | The field gains focus |
| 9 | Replace the invalid value with the original working OpenRouter key | The original key is visible in the field |
| 10 | Click `Save & Validate` again | Validation succeeds and OpenRouter returns to the configured state |

---

### Test Case: TC-PM-007 - Missing OpenRouter Credential Shows the Provider-Required Dialog
**Priority:** Critical
**Type:** Negative
**Preconditions:**
1. `OpenRouter` is currently configured with a known-working key.
2. The tester can restore the original OpenRouter key after the test.
3. A Workbench exists and can be opened from the Home screen.

| Step | Action | Expected Result |
|------|--------|-----------------|
| 1 | Open `Settings` from the Home screen | The `Settings` screen opens |
| 2 | Scroll to the `OpenRouter` provider card | The OpenRouter controls are visible |
| 3 | Click `Clear Key` in the OpenRouter card | The saved OpenRouter credential is removed |
| 4 | Read the OpenRouter status text | The provider shows the not-configured state |
| 5 | Click the Back arrow to return to the Home screen | The Home screen is displayed |
| 6 | Open any existing Workbench | The Workbench screen opens |
| 7 | Click the `Provider` selector | The provider dropdown opens |
| 8 | Select `OpenRouter` | `OpenRouter` becomes the selected provider |
| 9 | Click `Ask` if `Agent` is currently selected | `Ask` is the active composer mode |
| 10 | Click the composer field | The composer gains focus |
| 11 | Enter `Reply with READY only.` | The prompt text is visible in the composer |
| 12 | Click `Send` | A provider-required dialog appears instead of starting a model run |
| 13 | Read the dialog body and available actions | The dialog tells the tester to configure OpenRouter in Settings and includes `Open Settings` |
| 14 | Click `Open Settings` in the dialog | The `Settings` screen opens |
| 15 | Click the OpenRouter API key field | The field gains focus |
| 16 | Enter the original valid OpenRouter key | The original key is visible in the field |
| 17 | Click `Save & Validate` | Validation succeeds and OpenRouter returns to the configured state |

---

## Test Execution Summary

| Test Case | Priority | Status |
|-----------|----------|--------|
| TC-PM-001 | Critical | Not Run |
| TC-PM-002 | High | Not Run |
| TC-PM-003 | Critical | Not Run |
| TC-PM-004 | High | Not Run |
| TC-PM-005 | High | Not Run |
| TC-PM-006 | Critical | Not Run |
| TC-PM-007 | Critical | Not Run |

---

*End of Test Plan Document*
