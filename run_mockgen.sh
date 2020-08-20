#!/bin/bash

mkdir -p internal/workflow/mock_workflow
mockgen -copyright_file=COPYRIGHT -source=internal/workflow/session.go -destination=internal/workflow/mock_workflow/session.go
mockgen -copyright_file=COPYRIGHT -source=internal/workflow/store.go -destination=internal/workflow/mock_workflow/store.go

mkdir -p pkg/dipper/mock_dipper
mockgen -copyright_file=COPYRIGHT -source=pkg/dipper/rpc.go -destination=pkg/dipper/mock_dipper/rpc.go

mkdir -p internal/api/mock_api
mockgen -copyright_file=COPYRIGHT -source=internal/api/request_context.go -destination=internal/api/mock_api/request_context.go
