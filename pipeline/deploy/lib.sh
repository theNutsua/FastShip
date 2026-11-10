#!/usr/bin/env bash
# deploy/lib.sh all operations as shell functions. Sourced by the Makefile.
set -euo pipefail

# Logging primitives
log_ok()   { printf '  \033[32mOK\033[0m   %s\n' "$*" >&2; }
log_fail() {
  printf '  \033[31mFAIL\033[0m %s\n' "$1" >&2
  [[ -n "${2:-}" ]] && printf '         fix: %s\n' "$2" >&2
}
log_info() { printf '  ..   %s\n' "$*" >&2; }

# Source the acts

# shellcheck source=deploy/admission.sh
source "$(dirname "${BASH_SOURCE[0]}")/admission.sh"

# The check-env operation
op_check_env() {
  local rc=0
  log_info "admission gate: A1 ∧ A2 ∧ A3"
  a1_toolchain    || rc=1
  a2_lockfile     || rc=1
  a3_clean_tree   || rc=1
  if (( rc == 0 )); then
    log_ok "admitted; fingerprint $(a_fingerprint)"
  else
    log_fail "admission denied" "fix the predicates above, then re-run 'make check-env'"
  fi
  return $rc
}