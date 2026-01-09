#!/bin/bash

# Formlander Installer Script
# Usage: curl -fsSL https://raw.githubusercontent.com/karloscodes/formlander/master/install.sh | sudo bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Verify running as root
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}Error: This script requires root privileges. Use 'sudo su' and then re-run the installation command.${NC}"
    exit 1
fi

# Handle piped execution by creating a self-contained script
if [ ! -t 0 ]; then
    echo "Detected piped execution. Creating temporary installer for interactive mode..."
    TEMP_SCRIPT=$(mktemp /tmp/formlander-install-XXXXXX.sh)

    # Write the entire script to temp file, but without the pipe detection
    cat > "$TEMP_SCRIPT" << 'EOF'
#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

# Verify running as root
if [ "$(id -u)" -ne 0 ]; then
    echo -e "${RED}Error: This script requires root privileges. Use 'sudo su' and then re-run the installation command.${NC}"
    exit 1
fi

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH. Only amd64 and arm64 are supported.${NC}"
        exit 1
        ;;
esac

# Define installation directory and paths
INSTALL_DIR="/usr/local/bin"
BINARY_PATH="$INSTALL_DIR/formlander"
TEMP_FILE="/tmp/formlander-$ARCH"

# GitHub repository info
GITHUB_REPO="karloscodes/formlander"

NEED_UPDATE=false
if ! command -v jq >/dev/null 2>&1; then
    NEED_UPDATE=true
fi
if ! command -v file >/dev/null 2>&1; then
    NEED_UPDATE=true
fi
if [ "$NEED_UPDATE" = true ]; then
    apt-get update -qq > /dev/null 2>&1
fi
if ! command -v jq >/dev/null 2>&1; then
    apt-get install -y -qq jq > /dev/null 2>&1 || {
        echo -e "${RED}Error: Failed to install jq. This script requires jq to parse GitHub API responses.${NC}"
        exit 1
    }
fi
if ! command -v file >/dev/null 2>&1; then
    apt-get install -y -qq file > /dev/null 2>&1 || {
        echo -e "${RED}Error: Failed to install 'file'. Binary verification will be skipped.${NC}"
    }
fi

# Fetch the latest release information
echo "Fetching latest release information..."
RELEASE_INFO=$(curl -fsSL "https://api.github.com/repos/$GITHUB_REPO/releases/latest")

# Check for rate limit or other API errors
if echo "$RELEASE_INFO" | grep -q "API rate limit exceeded"; then
    echo -e "${RED}Error: GitHub API rate limit exceeded. Please try again later.${NC}"
    exit 1
fi

if echo "$RELEASE_INFO" | grep -q "Not Found"; then
    echo -e "${RED}Error: No releases found in $GITHUB_REPO. Please check the repository for releases.${NC}"
    exit 1
fi

# Extract the latest version using jq
LATEST_VERSION=$(echo "$RELEASE_INFO" | jq -r '.tag_name' | sed 's/^v//')

if [ -z "$LATEST_VERSION" ]; then
    echo -e "${RED}Error: Could not determine latest version.${NC}"
    exit 1
fi

echo "Latest version: $LATEST_VERSION"

# Look for the correct asset name
ASSET_NAME="formlander-v$LATEST_VERSION-$ARCH"
if ! echo "$RELEASE_INFO" | jq -r '.assets[].name' | grep -q "$ASSET_NAME"; then
    echo -e "${RED}Error: No binary found for $ARCH in release v$LATEST_VERSION.${NC}"
    echo "Available assets:"
    echo "$RELEASE_INFO" | jq -r '.assets[].name'
    exit 1
fi

# Construct the download URL
BINARY_URL="https://github.com/$GITHUB_REPO/releases/download/v$LATEST_VERSION/$ASSET_NAME"
echo "Download URL: $BINARY_URL"

# Download the binary
echo "Downloading Formlander v$LATEST_VERSION for $ARCH..."
curl -L --fail --progress-bar -o "$TEMP_FILE" "$BINARY_URL" || {
    echo -e "${RED}Error: Failed to download binary.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
}

# Verify the download
if [ ! -s "$TEMP_FILE" ]; then
    echo -e "${RED}Error: Downloaded file is empty.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
fi

# Check file type
if command -v file >/dev/null 2>&1; then
    FILE_TYPE=$(file -b "$TEMP_FILE" | cut -d',' -f1-2)
    echo "Verifying file: $FILE_TYPE"
    if ! echo "$FILE_TYPE" | grep -q "ELF"; then
        echo -e "${RED}Error: Downloaded file is not a valid binary.${NC}"
        rm -f "$TEMP_FILE"
        exit 1
    fi
fi

# Install the binary
echo "Installing to $BINARY_PATH..."
mv "$TEMP_FILE" "$BINARY_PATH" && chmod +x "$BINARY_PATH" || {
    echo -e "${RED}Error: Failed to install binary.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
}

# Run the installer interactively
echo -e "${GREEN}Running Formlander installer...${NC}"
if "$BINARY_PATH" install; then
    echo -e "${GREEN}Installation complete!${NC}"
else
    INSTALL_EXIT_CODE=$?
    echo -e "${RED}Installation failed with exit code $INSTALL_EXIT_CODE.${NC}"
    exit $INSTALL_EXIT_CODE
fi
EOF

    # Make temp script executable and run it with proper TTY
    chmod +x "$TEMP_SCRIPT"
    bash "$TEMP_SCRIPT" < /dev/tty
    rm -f "$TEMP_SCRIPT"
    exit $?
fi

# If we reach here, we're running normally (not piped)
# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Unsupported architecture: $ARCH. Only amd64 and arm64 are supported.${NC}"
        exit 1
        ;;
esac

# Define installation directory and paths
INSTALL_DIR="/usr/local/bin"
BINARY_PATH="$INSTALL_DIR/formlander"
TEMP_FILE="/tmp/formlander-$ARCH"

# GitHub repository info
GITHUB_REPO="karloscodes/formlander"

NEED_UPDATE=false
if ! command -v jq >/dev/null 2>&1; then
    NEED_UPDATE=true
fi
if ! command -v file >/dev/null 2>&1; then
    NEED_UPDATE=true
fi
if [ "$NEED_UPDATE" = true ]; then
    apt-get update -qq > /dev/null 2>&1
fi
if ! command -v jq >/dev/null 2>&1; then
    apt-get install -y -qq jq > /dev/null 2>&1 || {
        echo -e "${RED}Error: Failed to install jq. This script requires jq to parse GitHub API responses.${NC}"
        exit 1
    }
fi
if ! command -v file >/dev/null 2>&1; then
    apt-get install -y -qq file > /dev/null 2>&1 || {
        echo -e "${RED}Error: Failed to install 'file'. Binary verification will be skipped.${NC}"
    }
fi

# Fetch the latest release information
echo "Fetching latest release information..."
RELEASE_INFO=$(curl -fsSL "https://api.github.com/repos/$GITHUB_REPO/releases/latest")

# Check for rate limit or other API errors
if echo "$RELEASE_INFO" | grep -q "API rate limit exceeded"; then
    echo -e "${RED}Error: GitHub API rate limit exceeded. Please try again later.${NC}"
    exit 1
fi

if echo "$RELEASE_INFO" | grep -q "Not Found"; then
    echo -e "${RED}Error: No releases found in $GITHUB_REPO. Please check the repository for releases.${NC}"
    exit 1
fi

# Extract the latest version using jq
LATEST_VERSION=$(echo "$RELEASE_INFO" | jq -r '.tag_name' | sed 's/^v//')

if [ -z "$LATEST_VERSION" ]; then
    echo -e "${RED}Error: Could not determine latest version.${NC}"
    exit 1
fi

echo "Latest version: $LATEST_VERSION"

# Look for the correct asset name
ASSET_NAME="formlander-v$LATEST_VERSION-$ARCH"
if ! echo "$RELEASE_INFO" | jq -r '.assets[].name' | grep -q "$ASSET_NAME"; then
    echo -e "${RED}Error: No binary found for $ARCH in release v$LATEST_VERSION.${NC}"
    echo "Available assets:"
    echo "$RELEASE_INFO" | jq -r '.assets[].name'
    exit 1
fi

# Construct the download URL
BINARY_URL="https://github.com/$GITHUB_REPO/releases/download/v$LATEST_VERSION/$ASSET_NAME"
echo "Download URL: $BINARY_URL"

# Download the binary
echo "Downloading Formlander v$LATEST_VERSION for $ARCH..."
curl -L --fail --progress-bar -o "$TEMP_FILE" "$BINARY_URL" || {
    echo -e "${RED}Error: Failed to download binary.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
}

# Verify the download
if [ ! -s "$TEMP_FILE" ]; then
    echo -e "${RED}Error: Downloaded file is empty.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
fi

# Check file type
if command -v file >/dev/null 2>&1; then
    FILE_TYPE=$(file -b "$TEMP_FILE" | cut -d',' -f1-2)
    echo "Verifying file: $FILE_TYPE"
    if ! echo "$FILE_TYPE" | grep -q "ELF"; then
        echo -e "${RED}Error: Downloaded file is not a valid binary.${NC}"
        rm -f "$TEMP_FILE"
        exit 1
    fi
fi

# Install the binary
echo "Installing to $BINARY_PATH..."
mv "$TEMP_FILE" "$BINARY_PATH" && chmod +x "$BINARY_PATH" || {
    echo -e "${RED}Error: Failed to install binary.${NC}"
    rm -f "$TEMP_FILE"
    exit 1
}

# Run the installer interactively
echo -e "${GREEN}Running Formlander installer...${NC}"
if "$BINARY_PATH" install; then
    echo -e "${GREEN}Installation complete!${NC}"
    echo -e "${GREEN}Formlander has been successfully installed and configured.${NC}"
else
    INSTALL_EXIT_CODE=$?
    echo -e "${RED}Installation failed with exit code $INSTALL_EXIT_CODE.${NC}"
    exit $INSTALL_EXIT_CODE
fi
