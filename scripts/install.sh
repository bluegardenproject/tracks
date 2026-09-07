#!/bin/bash
#
# Installs tracks: downloads the matching binary from the latest GitHub
# release, drops it in ~/.tracks, and adds that dir to PATH in your shell
# rc. Re-run it any time to upgrade — a running daemon from the previous
# version auto-restarts on the next `tracks` invocation.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/bluegardenproject/tracks/main/scripts/install.sh | bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

REPO="bluegardenproject/tracks"
INSTALL_DIR="$HOME/.tracks"
BINARY_NAME="tracks"

echo -e "${BOLD}${BLUE}tracks Installer${NC}"
echo -e "Installing to: ${YELLOW}$INSTALL_DIR${NC}"
echo

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case $ARCH in
    x86_64) ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

case $OS in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    *)
        echo -e "${RED}Error: Unsupported OS: $OS${NC}"
        echo -e "${YELLOW}tracks needs tmux and is supported on Linux and macOS only.${NC}"
        exit 1
        ;;
esac

echo -e "Detected: ${GREEN}$OS-$ARCH${NC}"

echo -e "${BLUE}Creating installation directory...${NC}"
mkdir -p "$INSTALL_DIR"

echo -e "${BLUE}Fetching latest release...${NC}"
RELEASE_URL="https://api.github.com/repos/$REPO/releases/latest"
ASSET="tracks-$OS-$ARCH"
RELEASE_JSON=$(curl -s "$RELEASE_URL")

# `[^"]*` keeps a match inside one JSON string — the API answers on a
# single line, so a greedy `.*` would splice the first URL in the response
# onto the last asset name and yield a URL that 404s. Requiring
# `/releases/download/` narrows it to release assets, though a download
# URL quoted in the release notes still matches; what picks the right one
# is `head -1` plus GitHub listing `assets` ahead of `body`. The digest
# check below is what makes a wrong pick safe rather than fatal.
# `|| true` so a missing asset (e.g. before the first release exists,
# when /releases/latest 404s) falls through to the friendly guard below
# instead of aborting on grep's exit 1 under `set -e`.
DOWNLOAD_URL=$(printf '%s' "$RELEASE_JSON" |
    grep -o "https://[^\"]*/releases/download/[^\"]*$ASSET" | head -1 || true)
CHECKSUMS_URL=$(printf '%s' "$RELEASE_JSON" |
    grep -o "https://[^\"]*/releases/download/[^\"]*SHA256SUMS" | head -1 || true)

if [ -z "$DOWNLOAD_URL" ]; then
    echo -e "${RED}Error: Could not find binary for $OS-$ARCH${NC}"
    echo -e "${YELLOW}Available releases: https://github.com/$REPO/releases${NC}"
    exit 1
fi

echo -e "Download URL: ${GREEN}$DOWNLOAD_URL${NC}"

echo -e "${BLUE}Downloading tracks...${NC}"
TEMP_FILE=$(mktemp)
# -f: fail (non-zero exit) on an HTTP error instead of saving the error
# body as if it were the binary.
curl -fL -o "$TEMP_FILE" "$DOWNLOAD_URL"

# --- Verify the download against the release's published digests ---
# A checksums file that disagrees with the download always aborts. A
# release that publishes none only warns: releases before this check
# existed have no SHA256SUMS, and refusing those would break the
# documented install path for anyone landing on an older one.
sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | awk '{print $1}'
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 "$1" | awk '{print $1}'
    else
        return 1
    fi
}

echo -e "${BLUE}Verifying checksum...${NC}"
if [ -z "$CHECKSUMS_URL" ]; then
    # TODO: make this a hard failure once a release published *after* this
    # change is the latest one. /releases/latest always serves the newest
    # release, so from that point on there is no legitimate way to land on
    # a release without SHA256SUMS — while stripping the file is exactly
    # what someone with release-write access would do.
    echo -e "${YELLOW}  This release publishes no SHA256SUMS — skipping.${NC}"
elif ! SUMS=$(curl -fsSL "$CHECKSUMS_URL"); then
    rm -f "$TEMP_FILE"
    echo -e "${RED}Error: could not fetch SHA256SUMS — refusing to install.${NC}"
    echo -e "${RED}  $CHECKSUMS_URL${NC}"
    exit 1
elif ! EXPECTED=$(printf '%s\n' "$SUMS" |
    awk -v a="$ASSET" '$2 == a || $2 == "*" a {print $1}' | head -1) ||
    [ -z "$EXPECTED" ]; then
    rm -f "$TEMP_FILE"
    echo -e "${RED}Error: SHA256SUMS does not list $ASSET — refusing to install.${NC}"
    exit 1
elif ! ACTUAL=$(sha256_of "$TEMP_FILE"); then
    echo -e "${YELLOW}  No sha256sum or shasum on PATH — skipping.${NC}"
elif [ "$EXPECTED" != "$ACTUAL" ]; then
    rm -f "$TEMP_FILE"
    echo -e "${RED}Error: checksum mismatch for $ASSET — refusing to install.${NC}"
    echo -e "${RED}  expected $EXPECTED${NC}"
    echo -e "${RED}  actual   $ACTUAL${NC}"
    exit 1
else
    echo -e "${GREEN}  ok — $ASSET matches SHA256SUMS${NC}"
fi

echo -e "${BLUE}Installing binary...${NC}"
mv "$TEMP_FILE" "$INSTALL_DIR/$BINARY_NAME"
chmod +x "$INSTALL_DIR/$BINARY_NAME"

echo -e "${BLUE}Adding to PATH...${NC}"

# Append a PATH line to a POSIX-shell rc file, idempotently. We grep
# the file itself (not $PATH) so re-running the installer in a shell
# that hasn't yet sourced its rc doesn't produce duplicate entries.
add_path_posix() {
    local rc="$1"
    local marker="# tracks (auto-added by install.sh)"
    local line="export PATH=\"$INSTALL_DIR:\$PATH\""

    mkdir -p "$(dirname "$rc")"
    [ -f "$rc" ] || touch "$rc"

    if grep -Fq "$INSTALL_DIR" "$rc" 2>/dev/null; then
        echo -e "${YELLOW}$INSTALL_DIR already referenced in $rc${NC}"
        return
    fi

    {
        echo ""
        echo "$marker"
        echo "$line"
    } >> "$rc"
    echo -e "${GREEN}Added $INSTALL_DIR to PATH in $rc${NC}"
}

# fish uses a different syntax and a per-shell config dir. Drop a tiny
# conf.d snippet so it loads on every interactive fish session.
add_path_fish() {
    local conf_dir="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d"
    local conf="$conf_dir/tracks.fish"

    mkdir -p "$conf_dir"
    if [ -f "$conf" ] && grep -Fq "$INSTALL_DIR" "$conf"; then
        echo -e "${YELLOW}$INSTALL_DIR already referenced in $conf${NC}"
        return
    fi

    cat > "$conf" <<EOF
# tracks (auto-added by install.sh)
fish_add_path -gP $INSTALL_DIR
EOF
    echo -e "${GREEN}Added $INSTALL_DIR to PATH in $conf${NC}"
}

SHELL_NAME="$(basename "${SHELL:-}")"
SHELL_CONFIG=""

case "$SHELL_NAME" in
    zsh)
        SHELL_CONFIG="${ZDOTDIR:-$HOME}/.zshrc"
        add_path_posix "$SHELL_CONFIG"
        ;;
    bash)
        SHELL_CONFIG="$HOME/.bashrc"
        add_path_posix "$SHELL_CONFIG"
        # Login shells on macOS read .bash_profile, not .bashrc, so we
        # also append there if it exists. We don't create it from
        # nothing — that can hide an intentional .profile setup.
        if [ "$(uname -s)" = "Darwin" ] && [ -f "$HOME/.bash_profile" ]; then
            add_path_posix "$HOME/.bash_profile"
        fi
        ;;
    fish)
        add_path_fish
        SHELL_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/fish/conf.d/tracks.fish"
        ;;
    *)
        SHELL_CONFIG="$HOME/.profile"
        add_path_posix "$SHELL_CONFIG"
        echo -e "${YELLOW}Unrecognized shell '$SHELL_NAME' — wrote to $SHELL_CONFIG.${NC}"
        echo -e "${YELLOW}If your shell doesn't source that file, add $INSTALL_DIR to PATH manually.${NC}"
        ;;
esac

# Make tracks callable in *this* installer process too, so the verify
# step below works regardless of which rc file we touched.
case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *) export PATH="$INSTALL_DIR:$PATH" ;;
esac

echo -e "${BLUE}Verifying installation...${NC}"
if "$INSTALL_DIR/$BINARY_NAME" version >/dev/null 2>&1; then
    echo -e "${GREEN}Installation successful!${NC}"
else
    echo -e "${YELLOW}Installation completed, but verification failed${NC}"
    echo -e "${YELLOW}  You may need to restart your terminal${NC}"
fi

echo
echo -e "${BOLD}${GREEN}Installation Complete!${NC}"
echo
echo -e "${BOLD}Usage:${NC}"
echo -e "  ${GREEN}tracks${NC}          - Start the tmux session + dashboard"
echo -e "  ${GREEN}tracks version${NC}  - Show the installed version"
echo -e "  ${GREEN}tracks --help${NC}   - Show all commands"
echo
echo -e "${YELLOW}Requires ${BOLD}git${NC}${YELLOW}, ${BOLD}tmux${NC}${YELLOW}, and the ${BOLD}claude${NC}${YELLOW} CLI on your PATH.${NC}"
echo -e "${YELLOW}If a tracks daemon from an older version is running, it restarts${NC}"
echo -e "${YELLOW}automatically the next time you run ${BOLD}tracks${NC}${YELLOW}.${NC}"
echo
echo -e "${YELLOW}Note: You may need to restart your terminal.${NC}"
if [ -n "$SHELL_CONFIG" ]; then
    if [ "$SHELL_NAME" = "fish" ]; then
        echo -e "  ${BLUE}source $SHELL_CONFIG${NC}    # fish"
    else
        echo -e "  ${BLUE}source $SHELL_CONFIG${NC}"
    fi
fi
echo
