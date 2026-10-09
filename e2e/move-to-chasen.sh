#!/usr/bin/env bash
# The test of moving a server of the older installer to Chasen: it sets one
# up with the released formlander binary, as install.sh did before, then
# runs the steps of the docs and checks that the data, the password, and the
# domain stay, and that Chasen updates Formlander each night from then on.
#
# It needs Docker, sudo, and the ports 80 and 443, and it changes the
# machine: run it on a CI runner or in a throwaway VM, not on your computer.
#   e2e/move-to-chasen.sh
set -euo pipefail

host=formlander.localhost
# The install script of this commit, or the one on GitHub when the test runs alone.
installer="${FORMLANDER_INSTALLER:-https://raw.githubusercontent.com/karloscodes/formlander/main/install.sh}"
pass() { echo "ok   - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
web() { curl -s -H "Host: $host" "$@"; }
login() {
	web -o /dev/null -w '%{http_code} %{redirect_url}' -d "email=admin@formlander.local" --data-urlencode "password=$1" http://127.0.0.1/admin/login
}

echo "--- a server of the older installer"
case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64) arch=arm64 ;; *) fail "no build for $(uname -m)" ;; esac
curl -fsSL -o /tmp/formlander "https://github.com/karloscodes/formlander/releases/latest/download/formlander-linux-$arch"
sudo install -m 755 /tmp/formlander /usr/local/bin/formlander
printf '%s\ny\n' "$host" | sudo formlander install >/tmp/old-install.log 2>&1 || { cat /tmp/old-install.log; fail "the older installer failed"; }
for _ in $(seq 30); do [[ "$(web -o /dev/null -w '%{http_code}' http://127.0.0.1/_health)" == 200 ]] && break; sleep 2; done
password="$(sudo cat /var/matcha/formlander/storage/initial-admin-password)"
[[ "$(login "$password")" == 30?" "*/admin ]] || fail "the older install does not log in"
[[ -f /etc/cron.d/formlander-update ]] || fail "the older installer made no nightly update"
pass "the older installer runs Formlander, and its password logs in"

echo "--- the move, as the docs say"
curl -fsSL https://chasenhq.com/server | sudo sh >/dev/null
sudo chasen-server setup >/dev/null
sudo chasen-server adopt formlander
sudo rm /etc/cron.d/formlander-update /usr/local/bin/formlander
curl -fsSL "$installer" | sudo bash

[[ "$(web -o /dev/null -w '%{http_code}' http://127.0.0.1/up)" == 200 ]] || fail "/up does not answer after the move"
pass "/up answers after the move"
[[ "$(login "$password")" == 30?" "*/admin ]] || fail "the old password does not log in after the move"
pass "the data and the old password stay"
# The status in a variable: grep -q in a pipe stops early, and pipefail
# then counts the cut-off status as a failure.
status="$(sudo chasen-server status formlander)"
grep -q "Updates:  each night" <<<"$status" || fail "the nightly updates of Chasen are off"
[[ ! -f /etc/cron.d/formlander-update ]] || fail "the nightly update of the older installer is still there"
pass "Chasen updates it each night, and the older nightly update is gone"
grep -qE "^URL: +.*$host" <<<"$status" || fail "the domain changed: $status"
pass "the domain stays"

echo "--- the install line on the moved server: an update"
again="$(curl -fsSL "$installer" | sudo bash 2>&1)" || { echo "$again"; fail "the install line failed on the moved server"; }
grep -q "Formlander is up to date" <<<"$again" || fail "the install line did not update the moved server: $again"
[[ "$(login "$password")" == 30?" "*/admin ]] || fail "the login fails after the update"
pass "the install line updates the moved server"

echo "All checks passed."
