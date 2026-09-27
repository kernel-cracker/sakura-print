#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
# Copyright (C) 2026 PulseRoot
#
# Runs every test: Go (with the race detector), the installer's build tuning, the photo engine, and the browser tests.
#   ./test.sh            everything
#   ./test.sh editor     Go + engine, and only the browser tests with "editor" in their name
set -e
cd "$(dirname "$0")"
echo "== Go"
gofmt -l . | grep . && { echo "gofmt wants to change the files above"; exit 1; }
go vet ./...
go test -race -count=1 ./...
echo "== installer"
tests/install_test.sh | tail -1
echo "== photo engine"
node develop_test.mjs | tail -1
echo "== browser"
node tests/run.mjs "$@"
