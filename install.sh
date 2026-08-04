#!/usr/bin/env bash
#
# FastShip installer.
#
# Turns a fresh Ubuntu/Debian machine into a working FastShip host: the
# container platform (containerd, buildkit, CNI), the network config, the
# fastship + shipd binaries, and shipd as a managed system service.
#
# It is IDEMPOTENT — every step checks whether the work is already done and
# skips it if so. Safe to run on a machine that already has some pieces.
#
# This version builds the binaries from source, so it assumes Go and the
# FastShip repo are present. A release version would download prebuilt
# binaries instead.
#
# Usage:  sudo ./install.sh

set -euo pipefail

# --- helpers -----------------------------------------------------------

# say prints a step header so the user can follow along.
say()  { printf "\n\033[1;34m==>\033[0m %s\n" "$1"; }
ok()   { printf "  \033[1;32m✓\033[0m %s\n" "$1"; }
skip() { printf "  \033[1;33m•\033[0m %s (already done)\n" "$1"; }

# require_root ensures we can do the privileged setup.
require_root() {
  if [ "$(id -u)" -ne 0 ]; then
    echo "This installer needs root. Run: sudo ./install.sh" >&2
    exit 1
  fi
}

# have checks whether a command exists.
have() { command -v "$1" >/dev/null 2>&1; }

# --- versions (pin so installs are reproducible) -----------------------

BUILDKIT_VERSION="v0.18.2"
CNI_VERSION="v1.6.2"

# The user who invoked sudo — the one to add to the fastship group. When
# run via sudo, SUDO_USER holds the real user; fall back to the login name.
TARGET_USER="${SUDO_USER:-$(logname 2>/dev/null || echo "")}"

# --- 1. containerd -----------------------------------------------------

install_containerd() {
  say "containerd"
  if have containerd; then
    skip "containerd installed"
  else
    apt-get update -qq
    apt-get install -y -qq containerd
    ok "containerd installed"
  fi

  # Make sure it is running and starts on boot.
  systemctl enable --now containerd >/dev/null 2>&1 || true
  ok "containerd running"
}

# --- 2. buildkit -------------------------------------------------------

install_buildkit() {
  say "buildkit"
  if have buildkitd && have buildctl; then
    skip "buildkit binaries present"
  else
    local tarball="buildkit-${BUILDKIT_VERSION}.linux-amd64.tar.gz"
    local url="https://github.com/moby/buildkit/releases/download/${BUILDKIT_VERSION}/${tarball}"
    curl -fsSL "$url" -o "/tmp/${tarball}"
    tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    ok "buildkit installed"
  fi

  # Install buildkitd as a systemd service, backed by containerd so built
  # images land in containerd's store where the engine can run them.
  if [ -f /etc/systemd/system/buildkit.service ]; then
    skip "buildkit service present"
  else
    cat > /etc/systemd/system/buildkit.service <<'EOF'
[Unit]
Description=BuildKit (containerd worker)
After=containerd.service
Wants=containerd.service

[Service]
ExecStart=/usr/local/bin/buildkitd --oci-worker=false --containerd-worker=true --containerd-worker-namespace=fastship
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
    systemctl enable --now buildkit
    ok "buildkit service installed and running"
  fi
}

# --- 3. CNI plugins ----------------------------------------------------

install_cni() {
  say "CNI plugins"
  if [ -f /opt/cni/bin/bridge ]; then
    skip "CNI plugins present"
  else
    local tarball="cni-plugins-linux-amd64-${CNI_VERSION}.tgz"
    local url="https://github.com/containernetworking/plugins/releases/download/${CNI_VERSION}/${tarball}"
    mkdir -p /opt/cni/bin
    curl -fsSL "$url" -o "/tmp/${tarball}"
    tar -C /opt/cni/bin -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
    ok "CNI plugins installed"
  fi
}

# --- 4. network config -------------------------------------------------

write_network_config() {
  say "FastShip network"
  if [ -f /etc/cni/net.d/10-fastship.conflist ]; then
    skip "network config present"
  else
    mkdir -p /etc/cni/net.d
    cat > /etc/cni/net.d/10-fastship.conflist <<'EOF'
{
  "cniVersion": "1.0.0",
  "name": "fastship",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "fastship0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16",
        "routes": [ { "dst": "0.0.0.0/0" } ]
      }
    },
    { "type": "loopback" }
  ]
}
EOF
    ok "network config written"
  fi
}

# --- 5. build and install binaries -------------------------------------

install_binaries() {
  say "FastShip binaries"
  if ! have go; then
    echo "  Go is required to build from source but was not found." >&2
    echo "  Install Go, or use a release build of FastShip." >&2
    exit 1
  fi

  # Build both binaries. Run from the repo root (where this script lives).
  local repo_root
  repo_root="$(cd "$(dirname "$0")" && pwd)"

  ( cd "$repo_root" && go build -o /tmp/fastship ./cmd/fastship )
  ( cd "$repo_root" && go build -o /tmp/shipd ./cmd/shipd )

  # Install atomically (mv over any running binary avoids "text file busy").
  mv /tmp/fastship /usr/local/bin/fastship
  # shipd may be running as a service; stop, replace, restart below.
  mv /tmp/shipd /usr/local/bin/shipd.new
  ok "binaries built and staged"
}

# --- 6. fastship group -------------------------------------------------

setup_group() {
  say "fastship group"
  if getent group fastship >/dev/null; then
    skip "group exists"
  else
    groupadd fastship
    ok "group created"
  fi

  if [ -n "$TARGET_USER" ]; then
    if id -nG "$TARGET_USER" | grep -qw fastship; then
      skip "$TARGET_USER already in group"
    else
      usermod -aG fastship "$TARGET_USER"
      ok "added $TARGET_USER to fastship group"
      echo "  (log out and back in for the group to take effect)"
    fi
  fi
}

# --- 7. shipd service --------------------------------------------------

install_shipd_service() {
  say "shipd service"

  # Put the new binary in place (handles the running-binary case).
  if systemctl is-active --quiet shipd; then
    systemctl stop shipd
  fi
  mv /usr/local/bin/shipd.new /usr/local/bin/shipd
  ok "shipd binary installed"

  if [ -f /etc/systemd/system/shipd.service ]; then
    skip "shipd service file present"
  else
    cat > /etc/systemd/system/shipd.service <<'EOF'
[Unit]
Description=FastShip daemon
After=network.target containerd.service buildkit.service
Wants=containerd.service buildkit.service

[Service]
Type=simple
ExecStart=/usr/local/bin/shipd
Restart=always
RestartSec=2
User=root

[Install]
WantedBy=multi-user.target
EOF
    ok "shipd service file written"
  fi

  systemctl daemon-reload
  systemctl enable --now shipd
  ok "shipd running"
}

# --- main --------------------------------------------------------------

main() {
  require_root
  say "Installing FastShip"

  install_containerd
  install_buildkit
  install_cni
  write_network_config
  install_binaries
  setup_group
  install_shipd_service

  say "Done"
  echo "FastShip is installed. Try:"
  echo "  fastship run <your-app>"
  echo
  echo "If you were just added to the fastship group, log out and back in first."
}

main "$@"