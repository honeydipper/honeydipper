#!/bin/bash

mkdir -p internal/workflow/mock_workflow
mockgen -copyright_file=COPYRIGHT -source=internal/workflow/session.go -destination=internal/workflow/mock_workflow/session.go
mockgen -copyright_file=COPYRIGHT -source=internal/workflow/store.go -destination=internal/workflow/mock_workflow/store.go
