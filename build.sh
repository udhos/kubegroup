#!/bin/bash

go mod tidy

go test -race ./...

export CGO_ENABLED=0

go install ./...
