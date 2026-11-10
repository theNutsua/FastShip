# Makefile  the invocation surface.
SHELL   := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

LIB := deploy/lib.sh

.PHONY: help check-env fingerprint

help:  ## Show this help.
	@grep -E '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
	 awk 'BEGIN{FS=":.*?## "}{printf "  %-18s %s\n", $$1, $$2}'

check-env:  ## Admit: A1 toolchain, A2 lockfile, A3 clean tree.
	@bash -c 'source $(LIB) && op_check_env'

fingerprint:  ## Print F = SHA256(toolchain, GOOS, GOARCH, go.sum, git SHA).
	@bash -c 'source $(LIB) && a_fingerprint'