SHELL := /bin/bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

BIN := bin/lil.bin
CONFIG ?= config.toml
VERSION := $(shell git describe --tags --always --dirty)
COMMIT := $(shell git rev-parse --short HEAD)
BUILDSTR := $(VERSION) ($(COMMIT))
DEV_SOCKET := $(CURDIR)/.dev/tmux.sock
export LIL_TMUX_SOCKET := $(DEV_SOCKET)

.PHONY: help install build-ui build run test lint check dev dev-api dev-ui dev-stop dev-logs dev-attach dev-restart clean

help: ## Show available commands
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  %-16s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

install: ## Install locked frontend dependencies
	cd ui && pnpm install --frozen-lockfile

build-ui: install ## Build the embedded Vue UI
	cd ui && pnpm build

build: build-ui ## Build the Go binary and embedded UI
	mkdir -p bin
	CGO_ENABLED=0 go build -o $(BIN) -ldflags="-X 'main.buildString=$(BUILDSTR)'" .

run: build ## Run the production-style binary (CONFIG=config.toml)
	./$(BIN) --config="$(CONFIG)"

test: build-ui ## Run backend tests with the race detector
	go test -race ./...

lint: build-ui ## Run Go vet/staticcheck, Vue type checks and ESLint
	go vet ./...
	staticcheck ./...
	cd ui && pnpm typecheck && pnpm lint

check: test lint ## Run all checks

dev: build ## Start API and hot-reloading Vue UI in isolated tmux
	@mkdir -p .dev
	@if tmux -S "$$LIL_TMUX_SOCKET" has-session -t lil-dev 2>/dev/null; then \
		echo 'Already running. Use make dev-restart to rebuild the API.'; \
	else \
		tmux -S "$$LIL_TMUX_SOCKET" new-session -d -s lil-dev -n api -c "$(CURDIR)" '$(MAKE) dev-api'; \
		tmux -S "$$LIL_TMUX_SOCKET" new-window -t lil-dev -n ui -c "$(CURDIR)" '$(MAKE) dev-ui'; \
	fi
	@echo 'Admin: http://localhost:5173/admin/ (dev mode, signed in as dev@example.com)'
	@echo 'API/redirects: http://localhost:17000 | Database: .dev/urls.db'
	@echo 'Use make dev-logs, make dev-attach, or make dev-stop.'

dev-api: ## Run the local API in foreground
	mkdir -p .dev
	./$(BIN) --config=dev/local.toml 2>&1 | tee .dev/api.log

dev-ui: ## Run Vite with hot reload in foreground
	mkdir -p .dev
	cd ui && API_URL=http://127.0.0.1:17000 pnpm dev --host 127.0.0.1 --port 5173 --strictPort 2>&1 | tee ../.dev/ui.log

dev-stop: ## Stop only this project's local dev session
	@if tmux -S "$$LIL_TMUX_SOCKET" has-session -t lil-dev 2>/dev/null; then \
		tmux -S "$$LIL_TMUX_SOCKET" list-panes -s -t lil-dev -F '#{pane_id}' | while read -r pane; do \
			tmux -S "$$LIL_TMUX_SOCKET" send-keys -t "$$pane" C-c; \
		done; \
	fi

dev-restart: ## Stop, rebuild and restart local dev
	$(MAKE) dev-stop
	@for i in $$(seq 1 50); do \
		if ! tmux -S "$$LIL_TMUX_SOCKET" has-session -t lil-dev 2>/dev/null; then break; fi; \
		sleep 0.1; \
	done
	$(MAKE) dev

dev-logs: ## Show recent local API and UI logs
	tail -n 60 .dev/api.log .dev/ui.log

dev-attach: ## Attach to local dev terminals (Ctrl-b d detaches)
	tmux -S "$$LIL_TMUX_SOCKET" attach-session -t lil-dev

clean: ## Remove build outputs (keeps local database)
	rm -rf bin ui/dist
