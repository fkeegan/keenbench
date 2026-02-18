ENGINE_BIN=engine/bin/keenbench-engine
TOOL_WORKER_BIN=engine/bin/keenbench-tool-worker
FLUTTER_BIN ?= flutter
DART_BIN ?= dart

ROOT_DIR := $(abspath .)
ENGINE_BIN_ABS := $(abspath $(ENGINE_BIN))
TOOL_WORKER_BIN_ABS := $(abspath $(TOOL_WORKER_BIN))

.DEFAULT_GOAL := help

.PHONY: help run run-macos engine fmt test package-worker deps stop clean check-worker package-macos package-macos-universal notarize-macos notarize-macos-universal toolworker-macos toolworker-macos-universal iconset-macos linux-desktop-dev

LINUX_APP_ID ?= com.keenbench.app
LINUX_APP_BINARY ?= keenbench
LINUX_APP_DISPLAY_NAME ?= KeenBench
LINUX_APP_ICON_NAME ?= keenbench

help: ## Show available make targets
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make <target>\n\nTargets:\n"} /^[a-zA-Z0-9_-]+:.*##/ {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## Fetch Go/Flutter dependencies
	cd engine && go mod download
	cd app && $(FLUTTER_BIN) pub get

package-worker: ## Package the Python tool worker (creates venv and installs deps)
	@if [ ! -f engine/tools/pyworker/.venv/bin/python ] || [ ! -f engine/bin/keenbench-tool-worker ]; then \
		echo "Setting up tool worker venv..."; \
		scripts/package_worker.sh; \
	else \
		echo "Tool worker venv already exists, checking deps..."; \
		engine/tools/pyworker/.venv/bin/pip install -q -r engine/tools/pyworker/requirements.txt; \
	fi

check-worker: package-worker ## Verify tool worker can start and respond
	@echo "Verifying tool worker health..."
	@echo '{"jsonrpc":"2.0","id":1,"method":"WorkerGetInfo","params":{}}' | \
		KEENBENCH_WORKBENCHES_DIR=/tmp $(TOOL_WORKER_BIN) | \
		grep -q '"ok": true' && echo "Tool worker OK" || \
		(echo "ERROR: Tool worker health check failed!" && exit 1)

linux-desktop-dev: ## Install local Linux desktop metadata/icons so GNOME maps app ID to name/icon in dev runs
	@mkdir -p "$(HOME)/.local/share/applications"
	@sed \
		-e 's|@APP_DISPLAY_NAME@|$(LINUX_APP_DISPLAY_NAME)|g' \
		-e 's|@BINARY_NAME@|$(LINUX_APP_BINARY)|g' \
		-e 's|@APP_ICON_NAME@|$(LINUX_APP_ICON_NAME)|g' \
		-e 's|@APPLICATION_ID@|$(LINUX_APP_ID)|g' \
		app/linux/keenbench.desktop.in > "$(HOME)/.local/share/applications/$(LINUX_APP_ID).desktop"
	@for size in 16 32 48 64 128 256 512; do \
		install -Dm644 "app/linux/runner/resources/$(LINUX_APP_ICON_NAME)_$${size}.png" "$(HOME)/.local/share/icons/hicolor/$${size}x$${size}/apps/$(LINUX_APP_ICON_NAME).png"; \
	done
	@update-desktop-database "$(HOME)/.local/share/applications" >/dev/null 2>&1 || true
	@gtk-update-icon-cache -f -t "$(HOME)/.local/share/icons/hicolor" >/dev/null 2>&1 || true

run: deps engine package-worker linux-desktop-dev ## Run the Flutter desktop app (builds engine, sets up worker, fetches deps)
	cd app && KEENBENCH_ENV_PATH=$(ROOT_DIR)/.env KEENBENCH_ENGINE_PATH=$(ENGINE_BIN_ABS) KEENBENCH_TOOL_WORKER_PATH=$(TOOL_WORKER_BIN_ABS) $(FLUTTER_BIN) run -d linux

run-macos: deps engine package-worker ## Run the Flutter desktop app on macOS (builds engine, sets up worker, fetches deps)
	cd app && KEENBENCH_ENV_PATH=$(ROOT_DIR)/.env KEENBENCH_ENGINE_PATH=$(ENGINE_BIN_ABS) KEENBENCH_TOOL_WORKER_PATH=$(TOOL_WORKER_BIN_ABS) $(FLUTTER_BIN) run -d macos

engine: ## Build the Go engine binary
	cd engine && go build -o ../$(ENGINE_BIN) ./cmd/keenbench-engine

fmt: ## Format Go + Dart code
	cd engine && gofmt -w .
	cd app && $(DART_BIN) format .

test: ## Run Go + Flutter tests
	cd engine && go test ./... -coverprofile=coverage.out
	cd app && $(FLUTTER_BIN) test

stop: ## Stop running app/engine/tool-worker processes for this repo
	@echo "Stopping KeenBench processes..."
	-@pkill -f "$(ENGINE_BIN_ABS)" >/dev/null 2>&1 || true
	-@pkill -f "$(TOOL_WORKER_BIN_ABS)" >/dev/null 2>&1 || true
	-@pkill -f "$(ROOT_DIR)/app/build/linux" >/dev/null 2>&1 || true
	-@pkill -f "$(ROOT_DIR)/app/.dart_tool" >/dev/null 2>&1 || true
	-@pkill -f "flutter run -d linux" >/dev/null 2>&1 || true

clean: stop ## Stop processes and clean build artifacts
	cd engine && go clean -cache -testcache
	rm -f engine/coverage.out $(ENGINE_BIN) $(TOOL_WORKER_BIN)
	cd app && $(FLUTTER_BIN) clean

toolworker-macos: deps ## Build a standalone macOS tool worker binary (PyInstaller) for bundling into .app
	scripts/build_toolworker_macos.sh

toolworker-macos-universal: deps ## Build a universal2 macOS tool worker binary (requires Rosetta/x86_64 Python)
	scripts/build_toolworker_macos.sh universal2

package-macos: deps ## Build and package a macOS .dmg (unsigned)
	scripts/package_macos.sh

package-macos-universal: deps ## Build and package a macOS universal2 .dmg (requires Rosetta/x86_64 Python)
	KEENBENCH_MACOS_UNIVERSAL=1 scripts/package_macos.sh

notarize-macos: package-macos ## Notarize + staple the macOS .dmg (requires KEENBENCH_NOTARY_PROFILE)
	scripts/notarize_macos.sh "dist/$${KEENBENCH_DMG_NAME:-KeenBench-macos.dmg}"

notarize-macos-universal: package-macos-universal ## Notarize + staple the macOS universal2 .dmg (requires KEENBENCH_NOTARY_PROFILE)
	scripts/notarize_macos.sh "dist/$${KEENBENCH_DMG_NAME:-KeenBench-macos-universal2.dmg}"

iconset-macos: ## Regenerate the macOS AppIcon.appiconset PNGs from the current SVG mark
	python3 scripts/generate_macos_appiconset.py
