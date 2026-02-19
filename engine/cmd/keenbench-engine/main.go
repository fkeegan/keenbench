package main

import (
	"context"
	"encoding/json"
	"log"
	"os"

	"keenbench/engine/internal/appdirs"
	"keenbench/engine/internal/engine"
	"keenbench/engine/internal/envfile"
	"keenbench/engine/internal/envutil"
	"keenbench/engine/internal/errinfo"
	"keenbench/engine/internal/logging"
	"keenbench/engine/internal/rpc"
)

func main() {
	envResult := envfile.Load()
	debug := envutil.Bool("KEENBENCH_DEBUG")
	dataDir, err := appdirs.DataDir()
	if err != nil {
		log.Fatalf("engine init failed: %v", err)
	}
	logSetup, logErr := logging.NewFileLogger(dataDir, debug)
	logger := logSetup.Logger
	if logger == nil {
		logger = logging.Nop()
	}
	logger = logger.With("component", "engine")
	if logSetup.Enabled {
		logger.Info("engine.logging_enabled", "path", logSetup.Path)
	}
	if envResult.Loaded {
		logger.Debug("engine.env_loaded", "path", envResult.Path, "keys", envResult.Keys)
	}
	if envResult.Err != nil {
		logger.Warn("engine.env_load_failed", "path", envResult.Path, "error", envResult.Err.Error())
	}
	if logErr != nil {
		logger.Warn("engine.log_setup_failed", "error", logErr.Error())
	}
	if logSetup.Close != nil {
		defer logSetup.Close()
	}

	eng, err := engine.New(engine.WithLogger(logger))
	if err != nil {
		logger.Error("engine.init_failed", "error", err.Error())
		log.Fatalf("engine init failed: %v", err)
	}
	server := rpc.NewServer(engine.APIVersion, os.Stdin, os.Stdout, logger)
	eng.SetNotifier(server.Notify)

	register := func(method string, fn func(context.Context, json.RawMessage) (any, *errinfo.ErrorInfo)) {
		server.Register(method, func(ctx context.Context, params json.RawMessage) (any, *rpc.Error) {
			result, errInfo := fn(ctx, params)
			if errInfo != nil {
				msg := errInfo.ErrorCode
				if errInfo.Detail != "" {
					msg = errInfo.Detail
				}
				return nil, &rpc.Error{Message: msg, Data: errInfo}
			}
			return result, nil
		})
	}

	register("EngineGetInfo", eng.EngineGetInfo)
	register("ToolWorkerGetStatus", eng.ToolWorkerGetStatus)
	register("ProvidersGetStatus", eng.ProvidersGetStatus)
	register("ProvidersSetApiKey", eng.ProvidersSetApiKey)
	register("ProvidersClearApiKey", eng.ProvidersClearApiKey)
	register("ProvidersValidate", eng.ProvidersValidate)
	register("ProvidersSetEnabled", eng.ProvidersSetEnabled)
	register("ProvidersSetReasoningEffort", eng.ProvidersSetReasoningEffort)
	register("ProvidersOAuthStart", eng.ProvidersOAuthStart)
	register("ProvidersOAuthStatus", eng.ProvidersOAuthStatus)
	register("ProvidersOAuthComplete", eng.ProvidersOAuthComplete)
	register("ProvidersOAuthDisconnect", eng.ProvidersOAuthDisconnect)
	register("ModelsListSupported", eng.ModelsListSupported)
	register("ModelsGetCapabilities", eng.ModelsGetCapabilities)
	register("UserGetDefaultModel", eng.UserGetDefaultModel)
	register("UserSetDefaultModel", eng.UserSetDefaultModel)
	register("WorkbenchGetDefaultModel", eng.WorkbenchGetDefaultModel)
	register("WorkbenchSetDefaultModel", eng.WorkbenchSetDefaultModel)

	register("WorkbenchCreate", eng.WorkbenchCreate)
	register("WorkbenchOpen", eng.WorkbenchOpen)
	register("WorkbenchList", eng.WorkbenchList)
	register("WorkbenchGetScope", eng.WorkbenchGetScope)
	register("WorkbenchFilesList", eng.WorkbenchFilesList)
	register("WorkbenchFilesAdd", eng.WorkbenchFilesAdd)
	register("WorkbenchFilesRemove", eng.WorkbenchFilesRemove)
	register("WorkbenchFilesExtract", eng.WorkbenchFilesExtract)
	register("WorkbenchDelete", eng.WorkbenchDelete)
	register("ContextList", eng.ContextList)
	register("ContextGet", eng.ContextGet)
	register("ContextProcess", eng.ContextProcess)
	register("ContextGetArtifact", eng.ContextGetArtifact)
	register("ContextUpdateDirect", eng.ContextUpdateDirect)
	register("ContextDelete", eng.ContextDelete)

	register("EgressGetConsentStatus", eng.EgressGetConsentStatus)
	register("EgressGrantWorkshopConsent", eng.EgressGrantWorkshopConsent)
	register("EgressListEvents", eng.EgressListEvents)

	register("WorkshopGetState", eng.WorkshopGetState)
	register("WorkshopGetConversation", eng.WorkshopGetConversation)
	register("WorkshopSetActiveModel", eng.WorkshopSetActiveModel)
	register("WorkshopSendUserMessage", eng.WorkshopSendUserMessage)
	register("WorkshopCancelRun", eng.WorkshopCancelRun)
	register("WorkshopStreamAssistantReply", eng.WorkshopStreamAssistantReply)
	register("WorkshopRunAgent", eng.WorkshopRunAgent)
	register("WorkshopUndoToMessage", eng.WorkshopUndoToMessage)
	register("WorkshopRegenerate", eng.WorkshopRegenerate)
	register("WorkshopProposeChanges", eng.WorkshopProposeChanges)
	register("WorkshopGetProposal", eng.WorkshopGetProposal)
	register("WorkshopDismissProposal", eng.WorkshopDismissProposal)
	register("WorkshopApplyProposal", eng.WorkshopApplyProposal)

	register("ReviewGetChangeSet", eng.ReviewGetChangeSet)
	register("ReviewGetTextDiff", eng.ReviewGetTextDiff)
	register("ReviewGetDocxContentDiff", eng.ReviewGetDocxContentDiff)
	register("ReviewGetPptxContentDiff", eng.ReviewGetPptxContentDiff)
	register("ReviewGetPdfPreviewPage", eng.ReviewGetPdfPreviewPage)
	register("ReviewGetDocxPreviewPage", eng.ReviewGetDocxPreviewPage)
	register("ReviewGetOdtPreviewPage", eng.ReviewGetOdtPreviewPage)
	register("ReviewGetPptxPreviewSlide", eng.ReviewGetPptxPreviewSlide)
	register("ReviewGetXlsxPreviewGrid", eng.ReviewGetXlsxPreviewGrid)
	register("ReviewGetImagePreview", eng.ReviewGetImagePreview)

	register("DraftGetState", eng.DraftGetState)
	register("DraftPublish", eng.DraftPublish)
	register("DraftDiscard", eng.DraftDiscard)

	register("CheckpointsList", eng.CheckpointsList)
	register("CheckpointGet", eng.CheckpointGet)
	register("CheckpointCreate", eng.CheckpointCreate)
	register("CheckpointRestore", eng.CheckpointRestore)

	register("WorkbenchGetClutter", eng.WorkbenchGetClutter)

	if err := server.Serve(context.Background()); err != nil {
		logger.Error("rpc.server_error", "error", err.Error())
		log.Fatalf("rpc server error: %v", err)
	}
}
