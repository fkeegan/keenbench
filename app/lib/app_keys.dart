import 'package:flutter/widgets.dart';

class AppKeys {
  static const homeScreen = Key('home_screen');
  static const homeSettingsButton = Key('home_settings_button');
  static const homeNewWorkbenchButton = Key('home_new_workbench_button');
  static const homeWorkbenchGrid = Key('home_workbench_grid');
  static const homeEmptyState = Key('home_empty_state');
  static const homeDeleteWorkbenchDialog = Key('home_delete_workbench_dialog');
  static const homeDeleteWorkbenchConfirm = Key(
    'home_delete_workbench_confirm',
  );
  static const homeDeleteWorkbenchCancel = Key('home_delete_workbench_cancel');

  static const newWorkbenchDialog = Key('new_workbench_dialog');
  static const newWorkbenchNameField = Key('new_workbench_name');
  static const newWorkbenchCreateButton = Key('new_workbench_create');
  static const newWorkbenchCancelButton = Key('new_workbench_cancel');

  static const workbenchScreen = Key('workbench_screen');
  static const workbenchSettingsButton = Key('workbench_settings_button');
  static const workbenchAddFilesButton = Key('workbench_add_files');
  static const workbenchAddContextButton = Key('workbench_add_context');
  static const workbenchFileList = Key('workbench_file_list');
  static const workbenchMessageList = Key('workbench_message_list');
  static const workbenchComposerField = Key('workbench_message_input');
  static const workbenchSendButton = Key('workbench_send_button');
  static const workbenchProposeButton = Key('workbench_propose_button');
  static const workbenchApplyButton = Key('workbench_apply_button');
  static const workbenchDismissButton = Key('workbench_dismiss_button');
  static const workbenchDraftBanner = Key('workbench_draft_banner');
  static const workbenchDraftMetadata = Key('workbench_draft_metadata');
  static const workbenchReviewButton = Key('workbench_review_button');
  static const workbenchDiscardButton = Key('workbench_discard_button');
  static const workbenchCheckpointsButton = Key('workbench_checkpoints_button');
  static const workbenchClutterBar = Key('workbench_clutter_bar');
  static const workbenchSkipToMainLink = Key('workbench_skip_to_main_content');
  static const workbenchSkipToComposerLink = Key('workbench_skip_to_composer');
  static const workbenchMainContentRegion = Key(
    'workbench_main_content_region',
  );
  static const workbenchModelSelectorSemantics = Key(
    'workbench_model_selector_semantics',
  );
  static const workbenchErrorSummary = Key('workbench_error_summary');
  static const workbenchScopeBadge = Key('workbench_scope_badge');
  static const workbenchScopeLimits = Key('workbench_scope_limits');
  static const workbenchContextWarning = Key('workbench_context_warning');
  static const workbenchChatModeToggle = Key('workbench_chat_mode_toggle');
  static const workbenchPhaseStatus = Key('workbench_phase_status');
  static const workbenchToolStatus = Key('workbench_tool_status');
  static const workbenchRateLimitWarning = Key('workbench_rate_limit_warning');
  static const workbenchRemoveFileDialog = Key('workbench_remove_file_dialog');
  static const workbenchRemoveFileConfirm = Key(
    'workbench_remove_file_confirm',
  );
  static const workbenchRemoveFileCancel = Key('workbench_remove_file_cancel');

  static const proposalCard = Key('proposal_card');
  static const proposalSummary = Key('proposal_summary');
  static const proposalWrites = Key('proposal_writes');
  static const proposalWarnings = Key('proposal_warnings');

  static const settingsScreen = Key('settings_screen');
  static const settingsProviderStatus = Key('settings_provider_status');
  static const settingsProviderToggle = Key('settings_provider_toggle');
  static const settingsApiKeyField = Key('settings_api_key');
  static const settingsSaveButton = Key('settings_save_button');
  static const settingsConsentModeToggle = Key('settings_consent_mode_toggle');
  static const settingsConsentAllowAllDialog = Key(
    'settings_consent_allow_all_dialog',
  );
  static const settingsConsentAllowAllConfirm = Key(
    'settings_consent_allow_all_confirm',
  );
  static const settingsConsentAllowAllCancel = Key(
    'settings_consent_allow_all_cancel',
  );
  static ValueKey<String> settingsOAuthStatusText(String providerId) =>
      ValueKey<String>('settings_oauth_status_$providerId');
  static ValueKey<String> settingsOAuthConnectButton(String providerId) =>
      ValueKey<String>('settings_oauth_connect_$providerId');
  static ValueKey<String> settingsOAuthDisconnectButton(String providerId) =>
      ValueKey<String>('settings_oauth_disconnect_$providerId');
  static ValueKey<String> settingsOAuthProgressDialog(String providerId) =>
      ValueKey<String>('settings_oauth_progress_$providerId');
  static ValueKey<String> settingsOAuthProgressCancelButton(
    String providerId,
  ) => ValueKey<String>('settings_oauth_progress_cancel_$providerId');
  static ValueKey<String> settingsOAuthRedirectField(String providerId) =>
      ValueKey<String>('settings_oauth_redirect_$providerId');
  static ValueKey<String> settingsOAuthCompleteButton(String providerId) =>
      ValueKey<String>('settings_oauth_complete_$providerId');
  static ValueKey<String> settingsReasoningResearchDropdown(
    String providerId,
  ) => ValueKey<String>('settings_reasoning_research_$providerId');
  static ValueKey<String> settingsReasoningPlanDropdown(String providerId) =>
      ValueKey<String>('settings_reasoning_plan_$providerId');
  static ValueKey<String> settingsReasoningImplementDropdown(
    String providerId,
  ) => ValueKey<String>('settings_reasoning_implement_$providerId');

  static const consentDialog = Key('consent_dialog');
  static const consentFileList = Key('consent_file_list');
  static const consentScopeHash = Key('consent_scope_hash');
  static const consentCancelButton = Key('consent_cancel');
  static const consentContinueButton = Key('consent_continue');

  static const providerRequiredDialog = Key('provider_required_dialog');
  static const providerRequiredOpenSettings = Key(
    'provider_required_open_settings',
  );
  static const providerRequiredCancel = Key('provider_required_cancel');

  static const proposalFailedDialog = Key('proposal_failed_dialog');
  static const proposalFailedRetry = Key('proposal_failed_retry');
  static const proposalFailedDismiss = Key('proposal_failed_dismiss');

  static const reviewScreen = Key('review_screen');
  static const reviewPublishButton = Key('review_publish_button');
  static const reviewDiscardButton = Key('review_discard_button');
  static const reviewChangeList = Key('review_change_list');
  static const reviewDiffList = Key('review_diff_list');
  static const reviewSkipToFileList = Key('review_skip_to_file_list');
  static const reviewSkipToMainContent = Key('review_skip_to_main_content');
  static const reviewFileListFocusTarget = Key('review_file_list_focus_target');
  static const reviewDetailFocusTarget = Key('review_detail_focus_target');
  static const reviewErrorSummary = Key('review_error_summary');

  static const checkpointsScreen = Key('checkpoints_screen');
  static const checkpointsCreateButton = Key('checkpoints_create_button');
  static const checkpointsList = Key('checkpoints_list');

  static const contextOverviewScreen = Key('context_overview_screen');
  static const contextModeTextRadio = Key('context_mode_text_radio');
  static const contextModeFileRadio = Key('context_mode_file_radio');
  static const contextTextField = Key('context_text_field');
  static const contextFilePathField = Key('context_file_path_field');
  static const contextNoteField = Key('context_note_field');
  static const contextProcessButton = Key('context_process_button');
  static const contextCancelButton = Key('context_cancel_button');
  static const contextDeleteConfirm = Key('context_delete_confirm');
  static const contextDeleteCancel = Key('context_delete_cancel');
  static const contextDirectEditToggle = Key('context_direct_edit_toggle');
  static const contextDirectSaveButton = Key('context_direct_save_button');
  static const contextReprocessButton = Key('context_reprocess_button');
  static const contextProcessingIndicator = Key('context_processing_indicator');
  static const contextManualOverwriteConfirm = Key(
    'context_manual_overwrite_confirm',
  );
  static const contextManualOverwriteCancel = Key(
    'context_manual_overwrite_cancel',
  );

  static ValueKey<String> workbenchTile(String id) =>
      ValueKey<String>('workbench_tile_$id');
  static ValueKey<String> workbenchTileMenu(String id) =>
      ValueKey<String>('workbench_tile_menu_$id');
  static ValueKey<String> workbenchTileDelete(String id) =>
      ValueKey<String>('workbench_tile_delete_$id');

  static ValueKey<String> workbenchFileRow(String name) =>
      ValueKey<String>('workbench_file_$name');
  static ValueKey<String> workbenchFileExtractButton(String name) =>
      ValueKey<String>('workbench_file_extract_$name');
  static ValueKey<String> workbenchFileRemoveButton(String name) =>
      ValueKey<String>('workbench_file_remove_$name');
  static ValueKey<String> workbenchMessageCopyButton(String id) =>
      ValueKey<String>('workbench_message_copy_$id');
  static ValueKey<String> workbenchMessageRewindButton(String id) =>
      ValueKey<String>('workbench_message_rewind_$id');
  static ValueKey<String> workbenchMessageRegenerateButton(String id) =>
      ValueKey<String>('workbench_message_regenerate_$id');
  static ValueKey<String> contextCategoryCard(String category) =>
      ValueKey<String>('context_category_card_$category');
  static ValueKey<String> contextCategoryAddButton(String category) =>
      ValueKey<String>('context_category_add_$category');
  static ValueKey<String> contextCategoryEditButton(String category) =>
      ValueKey<String>('context_category_edit_$category');
  static ValueKey<String> contextCategoryDeleteButton(String category) =>
      ValueKey<String>('context_category_delete_$category');
  static ValueKey<String> contextCategoryInspectButton(String category) =>
      ValueKey<String>('context_category_inspect_$category');
  static ValueKey<String> contextArtifactField(String path) =>
      ValueKey<String>('context_artifact_field_$path');
  static ValueKey<String> workbenchCheckpointEventCard(String id) =>
      ValueKey<String>('workbench_checkpoint_event_$id');
  static ValueKey<String> workbenchCheckpointRestoreButton(String id) =>
      ValueKey<String>('workbench_checkpoint_restore_$id');
}
