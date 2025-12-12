#!/bin/bash
# Downloads vendor dependencies for embedding in the Go binary

set -e

VENDOR_DIR="web/static/vendor"
BIN_DIR="bin"
mkdir -p "$VENDOR_DIR" "$BIN_DIR"

echo "Downloading htmx..."
curl -sL "https://unpkg.com/htmx.org@1.9.12/dist/htmx.min.js" -o "$VENDOR_DIR/htmx.min.js"

echo "Downloading highlight.js..."
curl -sL "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js" -o "$VENDOR_DIR/highlight.min.js"

echo "Downloading highlight.js GitHub Dark theme..."
curl -sL "https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css" -o "$VENDOR_DIR/highlight-github-dark.min.css"

# Download Tailwind CLI if not present (pinned to v3.4.17 for compatibility)
TAILWIND_VERSION="v3.4.17"
TAILWIND_BIN="$BIN_DIR/tailwindcss"
if [ ! -f "$TAILWIND_BIN" ]; then
    echo "Downloading Tailwind CSS CLI $TAILWIND_VERSION..."

    # Detect OS and architecture
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        darwin)
            case "$ARCH" in
                arm64) TAILWIND_ASSET="tailwindcss-macos-arm64" ;;
                x86_64) TAILWIND_ASSET="tailwindcss-macos-x64" ;;
                *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
            esac
            ;;
        linux)
            case "$ARCH" in
                aarch64|arm64) TAILWIND_ASSET="tailwindcss-linux-arm64" ;;
                x86_64) TAILWIND_ASSET="tailwindcss-linux-x64" ;;
                *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
            esac
            ;;
        *)
            echo "Unsupported OS: $OS"
            exit 1
            ;;
    esac

    curl -sL "https://github.com/tailwindlabs/tailwindcss/releases/download/$TAILWIND_VERSION/$TAILWIND_ASSET" -o "$TAILWIND_BIN"
    chmod +x "$TAILWIND_BIN"
    echo "Tailwind CLI $TAILWIND_VERSION downloaded to $TAILWIND_BIN"
else
    echo "Tailwind CLI already present at $TAILWIND_BIN"
fi

echo ""
echo "Done! Vendored files:"
ls -la "$VENDOR_DIR"
