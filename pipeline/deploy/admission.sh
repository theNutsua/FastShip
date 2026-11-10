# deploy/admission.sh Act 1: A1 toolchain, A2 lockfile, A3 clean tree
# Sourced by lib.sh. Not executable on its own.

# A1 toolchain
# Fails unless `go version` matches the pinned toolchain in go.mod.
a1_toolchain() {
  local want got
  if [[ ! -f go.mod ]]; then
    log_fail "A1 toolchain: no go.mod found in $(pwd)" \
             "run this from the repo root, or create go.mod"
    return 1
  fi
  want=$(awk '/^toolchain /{print $2; exit}' go.mod)
  if [[ -z "$want" ]]; then
    log_fail "A1 toolchain: go.mod has no 'toolchain' directive" \
             "add: toolchain go1.22.4"
    return 1
  fi
  got=$(go version | awk '{print $3}')   # e.g. go1.22.4
  if [[ "$got" != "$want" ]]; then
    log_fail "A1 toolchain: expected $want, observed $got" \
             "install $want or update the toolchain directive"
    return 1
  fi
  log_ok "A1 toolchain: $got"
}

# A2 lockfile
a2_lockfile() {
  if [[ ! -f go.sum ]]; then
    log_fail "A2 lockfile: go.sum is missing" \
             "run 'go mod tidy' and commit go.sum"
    return 1
  fi
  if [[ ! -s go.sum ]]; then
    log_fail "A2 lockfile: go.sum is empty" \
             "run 'go mod tidy' and commit go.sum"
    return 1
  fi
  log_ok "A2 lockfile: go.sum present ($(wc -l < go.sum) lines)"
}

# A3 clean tree
a3_clean_tree() {
  local dirty
  dirty=$(git status --porcelain)
  if [[ -n "$dirty" ]]; then
    log_fail "A3 clean tree: working tree is dirty" \
             "commit or stash changes; 'git status --porcelain' shows:"$'\n'"$dirty"
    return 1
  fi
  log_ok "A3 clean tree: $(git rev-parse --short HEAD)"
}

# Fingerprint
# F = SHA256(go version, GOOS, GOARCH, go.sum, git SHA)
a_fingerprint() {
  {
    go version
    go env GOOS GOARCH
    sha256sum go.sum
    git rev-parse HEAD
  } | sha256sum | awk '{print $1}'
}