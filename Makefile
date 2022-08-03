SHELL := /bin/sh
BOLD := \033[1m
DIM := \033[2m
RESET := \033[0m

files_require_mocking = internal/workflow/session.go \
						internal/workflow/store.go \
						pkg/dipper/rpc.go \
						internal/api/request_context.go \
						drivers/cmd/gcloud-secret/main.go \
						drivers/cmd/datadog-emitter/statsd.go

ifneq (,$(wildcard ./.env))
	include ./.env
	export
endif

.PHONY: build lint run_mockgen unit-tests integration-tests test all clean run

build:
	@printf "$(BOLD)Buiding$(RESET)\n"
	@go install ./...

lint:
	@printf "$(BOLD)Linting source code$(RESET)\n"
	@golangci-lint run

.mockgen_installed:
	@printf "$(BOLD)Installing mockgen$(RESET)\n"
	@go get -d github.com/golang/mock/mockgen@v1.6.0
	@touch "$@"

.mockgen_files_generated: $(files_require_mocking)
	@for f in $?; do \
		basedir=`dirname $$f`; \
		mockdir="$$basedir"/mock_`basename $$basedir`; \
		mockfile="$$mockdir"/`basename $$f`; \
		mkdir -p "$$mockdir"; \
		printf "$(BOLD)Generating $$mockfile $(RESET)\n"; \
		mockgen -copyright_file=COPYRIGHT -source="$$f" -destination="$$mockfile"; \
	done
	@touch "$@"

run_mockgen: .mockgen_installed .mockgen_files_generated

unit-tests:
	@printf "$(BOLD)Running unit tests$(RESET)\n"
ifneq (,$(REPORT_TEST_COVERAGE))
	@go test -coverprofile=c.out ./...
else
	@go test ./...
endif

integration-tests:
	@printf "$(BOLD)Running integration tests$(RESET)\n"
	@go test -tags=integration ./cmd/honeydipper

test: unit-tests integration-tests

all: lint test build

clean:
	@for f in $(files_require_mocking); do \
		basedir=`dirname $$f`; \
		mockdir="$$basedir"/mock_`basename $$basedir`; \
		[[ -d "$$mockdir" ]] && rm -rf "$$mockdir" || true; \
	done
	@[[ -f ".mockgen_files_generated" ]] && rm -f .mockgen_files_generated || true
	@[[ -f ".mockgen_installed" ]] && rm -f .mockgen_installed || true

run: build
	@printf "$(BOLD)Starting the daemon$(RESET)\n"
	@REPO=$${REPO:-$${REPO_DIR}} $$(go env GOPATH)/bin/honeydipper
