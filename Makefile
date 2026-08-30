.DEFAULT_GOAL := help
SHELL := /bin/bash

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -X main.buildVersion=$(VERSION) -X main.buildCommit=$(COMMIT)

.PHONY: help
help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-26s\033[0m %s\n", $$1, $$2}'

# --- development ---------------------------------------------------------

.PHONY: up
up: ## Start postgres, redis and minio
	docker compose -f docker-compose.dev.yml up -d
	@echo "waiting for postgres..." && until docker compose -f docker-compose.dev.yml exec -T postgres pg_isready -U dawa24 -d dawa24_store >/dev/null 2>&1; do sleep 1; done
	@echo "ready"

.PHONY: down
down: ## Stop local infrastructure
	docker compose -f docker-compose.dev.yml down

.PHONY: reset
reset: ## Destroy local data and rebuild the schema from scratch
	docker compose -f docker-compose.dev.yml down -v
	rm -rf .data
	$(MAKE) up
	$(MAKE) migrate

.PHONY: run
run: ## Run the HTTP server
	go run -ldflags="$(LDFLAGS)" ./cmd/server

.PHONY: worker
worker: ## Run the background worker
	go run ./cmd/worker

# --- database ------------------------------------------------------------

.PHONY: migrate
migrate: ## Apply pending migrations
	go run ./cmd/cli migrate

.PHONY: migrate-status
migrate-status: ## List migrations and show how many are pending
	go run ./cmd/cli migrate-status

# --- quality gates -------------------------------------------------------
# These are exactly what CI runs. A green `make check` locally means a green
# pipeline, so failures are found before a push rather than after one.

.PHONY: check
check: fmt-check vet lint test check-provider-isolation check-prompt-version check-file-size check-inline-styles check-error-swallow check-no-cdn check-file-size-count check-hardcoded-arabic check-emoji check-unused-components ## Run every gate

.PHONY: check-error-swallow
check-error-swallow: ## Fail if a service error is silently discarded
	@echo "==> checking for swallowed errors"
	@bad=$$( \
	  { grep -rn 'err == nil {' internal/ui/*.go internal/modules/*/*.go 2>/dev/null | grep -v '_test.go'; \
	    grep -rnE '[a-zA-Z]+, _ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go 2>/dev/null | grep -v '_test.go'; \
	    grep -rn '_ = pages\.' internal/ui/*.go 2>/dev/null | grep -v '_test.go'; \
	  } | grep -v 'nolint:errswallow' | wc -l ); \
	if [ "$$bad" -ne 0 ]; then \
	  echo "FAIL: $$bad swallowed-error site(s):"; \
	  { grep -rn 'err == nil {' internal/ui/*.go internal/modules/*/*.go; \
	    grep -rnE '[a-zA-Z]+, _ = h\.[a-zA-Z]+Svc\.' internal/ui/*.go; \
	    grep -rn '_ = pages\.' internal/ui/*.go; } | grep -v '_test.go' | grep -v 'nolint:errswallow'; \
	  echo ""; \
	  echo "Each site must surface the error (see docs/PLAN_V6/04_PHASE_D_SILENCE.md §D.1.4)."; \
	  echo "If silence is genuinely correct, annotate the line with // nolint:errswallow and say why."; \
	  exit 1; \
	fi; \
	echo "OK: no swallowed errors"

.PHONY: fmt
fmt: ## Format the codebase
	gofmt -w .

.PHONY: fmt-check
fmt-check: ## Fail if anything is unformatted
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "unformatted files:"; echo "$$out"; exit 1; fi

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: lint
lint: ## Run golangci-lint (enforces module boundaries via depguard)
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed: https://golangci-lint.run/welcome/install/"; exit 1; }
	golangci-lint run

.PHONY: test
test: ## Run tests with race detection
	go test -race -count=1 ./...

.PHONY: test-short
test-short: ## Run unit tests only, skipping anything needing containers
	go test -short -count=1 ./...

.PHONY: test-integration
test-integration: ## Run database integration tests
	go test -v -count=1 ./test/integration/...


# --- architectural invariants --------------------------------------------

.PHONY: check-provider-isolation
check-provider-isolation: ## Fail if an AI provider name leaks outside platform/gateway
	@echo "checking provider isolation..."
	@if grep -riE '\b(openai|anthropic|deepseek|gemini|groq|openrouter)\b' \
	     --include='*.go' --include='*.templ' --include='*.sql' \
	     ./cmd ./internal ./db 2>/dev/null \
	     | grep -v '^./internal/platform/gateway/' \
	     | grep -v '_test.go'; then \
	  echo ""; \
	  echo "ERROR: an AI provider name appears outside internal/platform/gateway/."; \
	  echo "The Store must be provider-agnostic: ask the Gateway for a capability,"; \
	  echo "never for a named provider or model. See docs/adr/0005-ai-gateway.md."; \
	  exit 1; \
	fi
	@echo "  ok: no provider names outside the gateway package"

.PHONY: check-prompt-version
check-prompt-version: ## Fail if the match-prompt version is declared outside aicapabilities
	@echo "checking prompt version isolation..."
	@if grep -rn 'sm-enh-' --include='*.go' ./cmd ./internal 2>/dev/null 	     | grep -v '^./internal/shared/matchflow/' 	     | grep -v '_test.go'; then 	  echo ""; 	  echo "ERROR: the match-enhancement prompt version appears outside"; 	  echo "internal/shared/matchflow/. It is the decision-cache key: two copies"; 	  echo "drift, and a drifted key silently splits one cache in two."; 	  echo "Import matchflow.PromptVersion instead of restating it."; 	  exit 1; 	fi
	@echo "  ok: one prompt version, declared once"

.PHONY: check-file-size
check-file-size: ## Fail on Go files over 400 lines
	@echo "checking file sizes..."
	@fail=0; \
	while read -r count file; do \
	  if [ "$$count" -gt 400 ]; then echo "  $$file: $$count lines (limit 400)"; fail=1; fi; \
	done < <(find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v ' total$$'); \
	if [ "$$fail" = "1" ]; then \
	  echo ""; \
	  echo "ERROR: files above the 400-line limit. Split them by concern."; \
	  echo "The limit exists so a whole file fits in one read - for humans and"; \
	  echo "for AI agents. See AGENTS.md."; \
	  exit 1; \
	fi
	@echo "  ok: all files within limit"

# --- build ---------------------------------------------------------------

.PHONY: build
build: ## Build all binaries into ./bin
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/server ./cmd/server
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/worker ./cmd/worker
	go build -trimpath -ldflags="$(LDFLAGS)" -o bin/cli    ./cmd/cli

.PHONY: docker
docker: ## Build the container image
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t dawa24-store:$(VERSION) .

check-inline-styles: ## Fail if inline style attributes grow past the current ceiling
	@echo "==> checking inline styles"
	@n=$$(grep -oh 'style="' internal/ui/pages/*.templ internal/ui/layouts/*.templ | wc -l | tr -d ' '); 	if [ "$$n" -gt 3996 ]; then 	  echo "FAIL: $$n inline style attributes (ceiling 3996)."; 	  echo ""; 	  echo "Inline styles bypass the tokens in app.css, which is why the design"; 	  echo "drifted: a fix on one page never generalises. Use a class from"; 	  echo "components.css, or add one there. Genuinely one-off positioning may"; 	  echo "stay inline, but must use var(--token) values, never literals."; 	  echo ""; 	  echo "This is a ratchet: lower the ceiling in the Makefile as it drops."; 	  exit 1; 	fi; 	echo "OK: $$n inline styles (ceiling 3996)"

# --- ratchets ------------------------------------------------------------
# Each number below may only go down. When a change improves one, lower its
# ceiling in the same commit: that is what stops an improvement eroding a page
# at a time, which is how the design and the translations drifted in the first
# place. These are the measured values on 30 August 2026.

.PHONY: check-no-cdn
check-no-cdn: ## Fail if a template loads a script or stylesheet from a CDN
	@echo "==> checking for third-party asset hosts"
	@if grep -rn --include='*.templ' -E '(src|href)="https?://(unpkg|cdnjs|cdn\.jsdelivr)' internal/ui; then echo ""; echo "ERROR: a template loads an asset from a CDN."; echo "htmx, Alpine and Leaflet are served from internal/ui/static/vendor;"; echo "a CDN tag puts a host we do not run on every page render and forces"; echo "script-src to name it in the CSP."; exit 1; fi
	@echo "  ok: no CDN asset hosts"

.PHONY: check-file-size-count
check-file-size-count: ## Fail if the number of oversized Go files grows
	@n=$$(find ./cmd ./internal -name '*.go' -not -name '*_templ.go' -exec wc -l {} + | grep -v " total$$" | awk '$$1>400' | wc -l | tr -d " "); if [ "$$n" -gt 93 ]; then echo "FAIL: $$n Go files over 400 lines (ceiling 93). check-file-size lists them."; exit 1; fi; echo "  ok: $$n oversized files (ceiling 93)"

.PHONY: check-hardcoded-arabic
check-hardcoded-arabic: ## Fail if Arabic string literals in Go grow
	@pat=$$(printf '\330'); n=$$(LC_ALL=C grep -rc "\"[^\"]*$$pat[^\"]*\"" --include='*.go' internal/ui internal/modules cmd 2>/dev/null | grep -v "_test\|_templ\|/i18n/" | awk -F: '{s+=$$2} END{print s+0}'); if [ "$$n" -gt 1246 ]; then echo "FAIL: $$n Arabic literals in Go (ceiling 1246). A string in a handler cannot be shown in English -- add a key to internal/shared/i18n and call i18n.T(lang, key)."; exit 1; fi; echo "  ok: $$n Arabic literals in Go (ceiling 1246)"

.PHONY: check-emoji
check-emoji: ## Fail if emoji in templates grow
	@pat=$$(printf '\360\237'); n=$$(LC_ALL=C grep -ro "$$pat" --include='*.templ' internal/ui | wc -l | tr -d " "); if [ "$$n" -gt 9 ]; then echo "FAIL: $$n emoji in templates (ceiling 9). Use an icon from internal/ui/components/icons.templ, or nothing."; exit 1; fi; echo "  ok: $$n emoji in templates (ceiling 9)"

.PHONY: check-unused-components
check-unused-components: ## Fail if components no page uses grow
	@n=0; for f in internal/ui/components/*.templ; do for fn in $$(grep -oE "^templ [A-Za-z0-9_]+" "$$f" | awk '{print $$2}'); do p=$$(grep -rho "components\.$$fn" --include='*.templ' internal/ui/pages internal/ui/layouts 2>/dev/null | wc -l); i=$$(grep -rho "@$$fn(" --include='*.templ' internal/ui/components 2>/dev/null | wc -l); if [ "$$p" -eq 0 ] && [ "$$i" -eq 0 ]; then n=$$((n+1)); echo "  unused: $$fn"; fi; done; done; if [ "$$n" -gt 0 ]; then echo "FAIL: $$n components nothing references (ceiling 0). Either a page should use it, or it should not exist."; exit 1; fi; echo "  ok: every component is referenced"
