#!/bin/bash
# Installs Formlander on this server with Chasen (https://chasenhq.com):
# HTTPS, live backups of the database, and an update each night.
#   curl -fsSL https://formlander.com/install | sudo bash -s forms.example.com
#
# The domain is the first argument, or FORMLANDER_DOMAIN. Without either, it asks.
# FORMLANDER_IMAGE deploys another image than karloscodes/formlander:latest,
# for a test. Run it again to update Formlander now.
set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'
image="${FORMLANDER_IMAGE:-karloscodes/formlander:latest}"

fail() { echo -e "${RED}Error: $*${NC}" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || fail "run it as root: curl -fsSL https://formlander.com/install | sudo bash -s forms.example.com"
[ "$(uname -s)" = Linux ] || fail "Formlander installs on a Linux server. From your computer, deploy it with Chasen: https://formlander.com/docs/deployment/"

# A server that the older installer set up keeps it: it updates itself each
# night. This script never touches a Formlander that runs.
if [ -f /etc/cron.d/formlander-update ] || grep -qs '^ *formlander:' /etc/matcha/config.yml; then
	echo "This server runs Formlander from the older installer. It keeps updating itself each night,"
	echo "and its commands stay: formlander update, restore-db, change-admin-password."
	echo "Nothing changed. To move it to Chasen: https://formlander.com/docs/move-to-chasen/"
	exit 0
fi

# Formlander runs on Chasen already, also after a move from the older
# installer: deploy the newest image, keep the settings, and update it each
# night from now on.
if command -v chasen-server >/dev/null 2>&1 && chasen-server list 2>/dev/null | grep -q '^formlander '; then
	echo "Updating Formlander to the newest $image..."
	printf '{"image":"%s","keep_settings":true,"auto_update":true,"env":{}}\n' "$image" | chasen-server deploy formlander latest
	echo -e "${GREEN}Formlander is up to date.${NC}"
	exit 0
fi

domain="${1:-${FORMLANDER_DOMAIN:-}}"
if [ -z "$domain" ]; then
	[ -r /dev/tty ] || fail "no domain: curl -fsSL https://formlander.com/install | sudo bash -s forms.example.com"
	read -r -p "Domain for Formlander (e.g. forms.example.com): " domain </dev/tty
fi
domain="$(echo "$domain" | tr '[:upper:]' '[:lower:]')"
[[ "$domain" =~ ^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$ && "$domain" == *.* ]] || fail "\"$domain\" is not a domain"

echo "Installing Chasen..."
curl -fsSL https://chasenhq.com/server | sh
chasen-server setup >/dev/null # it installs Docker when the server has none

echo "Deploying Formlander at $domain..."
printf '{"image":"%s","domain":"%s","auto_update":true,"env":{}}\n' "$image" "$domain" | chasen-server deploy formlander latest

password="$(chasen-server run formlander cat /app/storage/initial-admin-password 2>/dev/null || true)"
echo
echo -e "${GREEN}Formlander runs at https://$domain${NC}"
echo "  Sign in as admin@formlander.local${password:+ with the password $password}. Change it in Settings."
echo "  Point an A record for $domain to this server: HTTPS comes on the first request."
echo "  It updates itself each night, with a backup of the database first."
echo
echo "Manage it from your computer:"
echo "  curl -fsSL https://chasenhq.com/cli | sh"
echo "  chasen add server root@<this server>"
echo "  chasen -a formlander status        # also: logs, backups, restore, rollback"
