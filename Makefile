SHELL := /bin/sh
BOLD := \033[1m
DIM := \033[2m
RESET := \033[0m

files_require_mocking = internal/workflow/session.go \
						internal/workflow/store.go \
						pkg/dipper/rpc.go \
						internal/api/request_context.go

.PHONY: build lint run_mockgen unit-tests integration-tests test all clean

build:
	@echo -e "$(BOLD)Buiding$(RESET)"
	@go install ./...

lint:
	@echo -e "$(BOLD)Linting source code$(RESET)"
	@golangci-lint run

.mockgen_installed:
	@echo -e "$(BOLD)Installing mockgen$(RESET)"
	@go get github.com/golang/mock/mockgen@v1.4.4
	@touch "$@"

.mockgen_files_generated: $(files_require_mocking)
	@for f in $?; do \
		basedir=`dirname $$f`; \
		mockdir="$$basedir"/mock_`basename $$basedir`; \
		mockfile="$$mockdir"/`basename $$f`; \
		mkdir -p "$$mockdir"; \
		echo -e "$(BOLD)Generating $$mockfile $(RESET)"; \
		mockgen -copyright_file=COPYRIGHT -source="$$f" -destination="$$mockfile"; \
	done
	@touch "$@"

run_mockgen: .mockgen_installed .mockgen_files_generated

unit-tests: run_mockgen
	@echo -e "$(BOLD)Running unit tests$(RESET)"
	@go test ./...

integration-tests: run_mockgen
	@echo -e "$(BOLD)Running integration tests$(RESET)"
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
