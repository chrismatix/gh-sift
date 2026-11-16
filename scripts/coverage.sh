#!/usr/bin/env bash
# Writes coverdata/coverage.html, coverage-details.html and the coverage.svg
# badge. -coverpkg credits a package for what any package's tests run in it.
set -euo pipefail
cd "$(dirname "$0")/.."
rm -rf coverdata && mkdir coverdata
go test -coverpkg=./... -coverprofile=coverdata/coverage_all.out ./...
# The report tool is not part of sift.
grep -v '/tools/' coverdata/coverage_all.out >coverdata/coverage.out
go tool cover -html=coverdata/coverage.out -o=coverdata/coverage-details.html
go run ./tools/coveragereport coverdata/coverage.out coverdata
