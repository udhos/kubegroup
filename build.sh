#!/bin/bash

go install golang.org/x/vuln/cmd/govulncheck@latest
go install golang.org/x/tools/cmd/deadcode@latest
go install github.com/mgechev/revive@latest
go install honnef.co/go/tools/cmd/staticcheck@latest

gofmt -s -w .

revive ./...

staticcheck ./...

gocyclo -over 15 .

go mod tidy

govulncheck ./...

deadcode ./examples/*

go env -w CGO_ENABLED=1

go test -race ./...

go env -w CGO_ENABLED=0

go install ./...

go env -u CGO_ENABLED
