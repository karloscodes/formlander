#!/usr/bin/env bash
# The test of install.sh, the script behind `curl -fsSL
# https://formlander.com/install | sudo bash`: on a fresh machine it installs
# Chasen and deploys the image of this commit, then this checks /up, a real
# login with the password the script printed, the nightly updates, a second
# run that updates and keeps the data, and that a server of the older
# installer is left alone.
#
# It needs Docker, sudo, and the ports 80 and 443, and it changes the
# machine: run it on a CI runner or in a throwaway VM, not on your computer.
#   e2e/installer.sh
set -euo pipefail
cd "$(dirname "$0")/.."

registry=localhost:5000 # a registry with no login, like Docker Hub for a public image
image=$registry/formlander:e2e
host=formlander.localhost

pass() { echo "ok   - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
web() { curl -s -H "Host: $host" "$@"; } # through the proxy of Chasen
install() { sudo FORMLANDER_IMAGE="$image" bash install.sh "$host"; } # the domain as the docs pass it: bash -s <domain>

docker rm -f formlander-e2e-registry >/dev/null 2>&1 || true
docker run -d --name formlander-e2e-registry -p 127.0.0.1:5000:5000 registry:2 >/dev/null
trap 'docker rm -f formlander-e2e-registry >/dev/null 2>&1 || true' EXIT

echo "--- the image of this commit"
# TARGETARCH, which BuildKit sets and the legacy builder does not.
case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; *) arch=$(uname -m) ;; esac
docker build -q --build-arg TARGETARCH="$arch" -t "$image" . >/dev/null
docker push -q "$image" >/dev/null

echo "--- the installer, on a fresh machine"
out="$(install 2>&1)" || { echo "$out"; fail "install.sh failed"; }
echo "$out" | grep -v "with the password" | tail -8 # the password stays out of a public CI log

[[ "$(web -o /dev/null -w '%{http_code}' http://127.0.0.1/up)" == 200 ]] || fail "/up does not answer 200"
pass "/up answers 200"
status="$(sudo chasen-server status formlander)" # not in a pipe: grep -q stops early, and pipefail fails the status
grep -q "Updates:  each night" <<<"$status" || fail "the nightly updates are off"
pass "Formlander updates itself each night"

password="$(echo "$out" | grep -o 'with the password [^ .]*' | cut -d' ' -f4)"
[[ -n "$password" ]] || fail "install.sh printed no first password"
login() {
	web -o /dev/null -w '%{http_code} %{redirect_url}' -d "email=admin@formlander.local" --data-urlencode "password=$1" http://127.0.0.1/admin/login
}
[[ "$(login "$password")" == 30?" "*/admin ]] || fail "the printed password does not log in: $(login "$password")"
pass "the password that install.sh printed logs in"

echo "--- the installer again: an update that keeps the data"
again="$(install 2>&1)" || { echo "$again"; fail "the second run failed"; }
grep -q "Formlander is up to date" <<<"$again" || fail "the second run did not update: $again"
[[ "$(login "$password")" == 30?" "*/admin ]] || fail "the login fails after the second run"
pass "a second run updates, and the data stays"

echo "--- a server of the older installer"
echo "0 3 * * * root /usr/local/bin/formlander update" | sudo tee /etc/cron.d/formlander-update >/dev/null
old="$(install 2>&1)"
sudo rm /etc/cron.d/formlander-update
grep -q "Nothing changed" <<<"$old" || fail "install.sh did not leave a server of the older installer alone: $old"
pass "install.sh leaves a server of the older installer alone"

echo "All checks passed."
