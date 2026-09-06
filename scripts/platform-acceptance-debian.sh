#!/usr/bin/env bash
# Run ONLY as the bootstrap entrypoint in the disposable CI Debian container.
set -euo pipefail
if [[ ${CI:-} != true || ${GITHUB_ACTIONS:-} != true || ${DOTFILES_PLATFORM_ACCEPTANCE:-} != 1 ]] ||
   [[ $(uname -s) != Linux || $(id -u) != 0 || ! -f /.dockerenv || ! -f /inputs/dotfiles ]]; then
  echo 'Debian acceptance requires its explicitly authorized disposable CI container' >&2
  exit 125
fi
# shellcheck source=/dev/null
source /etc/os-release
[[ $ID == debian && $VERSION_ID == 13 ]]
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y --no-install-recommends sudo ca-certificates curl zsh jq passwd util-linux
if dpkg-query -W -f='${Status}' fzf 2>/dev/null | grep -q 'ok installed'; then
  echo 'fzf already installed; absent-package precondition failed' >&2
  exit 1
fi
if command -v fzf; then exit 1; fi
useradd --create-home --uid 1001 --shell /bin/bash qa
chmod 700 /home/qa
install -d -m 0755 /opt/dotfiles-platform-qa
install -m 0555 /inputs/platformqa /opt/dotfiles-platform-qa/platformqa
install -m 0555 /inputs/dotfiles /opt/dotfiles-platform-qa/dotfiles
install -m 0555 /inputs/tools.test /opt/dotfiles-platform-qa/tools.test
printf 'dotfiles-platform-qa-debian-v1\n' > /opt/dotfiles-platform-qa/authorized
chmod 0444 /opt/dotfiles-platform-qa/authorized
# Only the two reviewed binaries can become root. Their private dispatcher
# applies the production trusted apt/pacman grammar and fixed environment.
printf 'qa ALL=(root) NOPASSWD: /opt/dotfiles-platform-qa/dotfiles, /opt/dotfiles-platform-qa/platformqa\n' > /etc/sudoers.d/dotfiles-platform-qa
chmod 0440 /etc/sudoers.d/dotfiles-platform-qa
visudo -cf /etc/sudoers.d/dotfiles-platform-qa
install -d -o qa -g qa -m 0700 /home/qa/.config /home/qa/.cache /home/qa/.local /home/qa/.local/state /home/qa/tmp
printf 'acceptance sentinel\n' > /home/qa/.zshrc
chown qa:qa /home/qa/.zshrc
chmod 0600 /home/qa/.zshrc
sha256sum /home/qa/.zshrc > /evidence/config-before.sha256
find /home/qa/.config -type f -print > /evidence/config-before.files
run_qa() {
  runuser -u qa -- env -i HOME=/home/qa XDG_CONFIG_HOME=/home/qa/.config \
    XDG_STATE_HOME=/home/qa/.local/state XDG_CACHE_HOME=/home/qa/.cache TMPDIR=/home/qa/tmp \
    PATH=/usr/bin:/bin:/usr/sbin:/sbin LANG=C LC_ALL=C CI=true GITHUB_ACTIONS=true \
    DOTFILES_PLATFORM_ACCEPTANCE=1 "$@"
}
bin=/opt/dotfiles-platform-qa/dotfiles
qa=/opt/dotfiles-platform-qa/platformqa
sha256sum "$bin" "$qa" /opt/dotfiles-platform-qa/tools.test > /evidence/binaries.sha256
cat /etc/os-release > /evidence/os-release
uname -a > /evidence/kernel.txt
apt --version > /evidence/apt-version.txt
run_qa "$bin" plan --json --tool fzf > /evidence/plan.json 2> /evidence/plan.stderr
[[ ! -s /evidence/plan.stderr ]]
jq -se 'length == 1 and (.[0] | .kind == "dotfiles.plan" and .status == "ready" and .manager == "apt" and .intent.tools == ["fzf"] and .summary.apply == 1 and .summary.backup_targets == 0 and (.actions | length) == 1)' /evidence/plan.json
jq -e '.actions[0].install | .manager == "apt" and (.steps | length) == 1 and .steps[0].kind == "package_manager" and .steps[0].provider == "apt" and .steps[0].packages == ["fzf"] and (.steps[0].casks | length) == 0 and (.steps[0].arguments | length) == 0 and .detector.kind == "package_receipt" and .detector.values == ["fzf"]' /evidence/plan.json
plan_hash=$(jq -er '.authority.plan_hash | select(test("^[a-f0-9]{64}$"))' /evidence/plan.json)
wrong_hash="${plan_hash:0:63}0"
[[ $wrong_hash != "$plan_hash" ]] || wrong_hash="${plan_hash:0:63}1"
set +e
run_qa "$bin" apply --yes --plan-hash "$wrong_hash" --tool fzf > /evidence/wrong.stdout 2> /evidence/wrong.stderr
wrong_exit=$?
set -e
[[ $wrong_exit == 2 && ! -s /evidence/wrong.stdout ]]
[[ $(cat /evidence/wrong.stderr) == 'installation plan changed; run dotfiles plan --json again' ]]
if dpkg-query -W -f='${Status}' fzf 2>/dev/null | grep -q 'ok installed'; then exit 1; fi
run_qa "$bin" apply --yes --plan-hash "$plan_hash" --tool fzf > /evidence/apply.stdout 2> /evidence/apply.stderr
[[ ! -s /evidence/apply.stderr && $(wc -l < /evidence/apply.stdout) == 1 ]]
grep -Eq "^installation applied: operation=[a-zA-Z0-9_.-]+ plan_hash=${plan_hash} succeeded=1 failed=0$" /evidence/apply.stdout
run_qa "$qa" receipt > /evidence/receipt.log
apt-mark hold fzf
run_qa "$qa" receipt-held > /evidence/receipt-held.log
[[ $(apt-mark showhold) == fzf ]]
apt-mark unhold fzf
run_qa "$qa" update > /evidence/update.log 2> /evidence/update.stderr
[[ ! -s /evidence/update.stderr ]]
grep -Fx 'DOTFILES_DEBIAN_QA_EXECUTED mode=update' /evidence/update.log
grep -F 'update evidence=no-op version=' /evidence/update.log
run_qa "$bin" plan --json --tool fzf > /evidence/after.json 2> /evidence/after.stderr
[[ ! -s /evidence/after.stderr ]]
jq -se 'length == 1 and (.[0] | .status == "no_changes" and (.authority | has("plan_hash") | not) and .capabilities.apply == "not_available")' /evidence/after.json
set +e
run_qa "$bin" apply --yes --plan-hash "$plan_hash" --tool fzf > /evidence/stale.stdout 2> /evidence/stale.stderr
stale_exit=$?
set -e
[[ $stale_exit == 2 && ! -s /evidence/stale.stdout ]]
[[ $(cat /evidence/stale.stderr) == 'installation plan is not ready; run dotfiles plan --json again' ]]
shopt -s nullglob
records=(/home/qa/.local/state/dotfiles/operations/*.json)
[[ ${#records[@]} == 1 ]]
jq -e --arg hash "$plan_hash" '.status == "succeeded" and .plan_hash == $hash and (.actions | length) == 1 and .actions[0].status == "succeeded" and (.backup // "") == "" and (.rollback // null) == null and (.warnings | index("package-only operation has no filesystem mutations; no filesystem rollback point was created")) != null' "${records[0]}"
[[ $(stat -c '%a' "${records[0]}") == 600 ]]
cp "${records[0]}" /evidence/operation.json
# This artifact contains only synthetic CI state; preserve private source mode.
chmod 0644 /evidence/operation.json
sha256sum -c /evidence/config-before.sha256
find /home/qa/.config -type f -print > /evidence/config-after.files
cmp /evidence/config-before.files /evidence/config-after.files
run_qa /opt/dotfiles-platform-qa/tools.test \
  -test.run='^TestGeneratedOptionsAreAcceptedByInstalledFzf$' -test.v -test.timeout=30s > /evidence/fzf-config.log
grep -F -- '--- PASS: TestGeneratedOptionsAreAcceptedByInstalledFzf ' /evidence/fzf-config.log
if grep -F -- '--- SKIP:' /evidence/fzf-config.log; then exit 1; fi
# Exercise an actual repository upgrade from an immutable older Debian package.
# Downgrade is an operator/package-manager recovery operation, not a promise
# that dotfiles filesystem rollback reverses installed packages.
case $(dpkg --print-architecture) in
  amd64) old_hash=5dddf8e2bd85956dc037711fbc9ee081cac81254cf706895c625cf9393140005 ;;
  arm64) old_hash=c9e9a7565618fb3095b3d213ead0ec7ae6e3fbe200abc5786b482a2f9cce39fe ;;
  *) exit 125 ;;
esac
old_package=/opt/dotfiles-platform-qa/fzf-old.deb
old_url="https://deb.debian.org/debian/pool/main/f/fzf/fzf_0.38.0-1+b1_$(dpkg --print-architecture).deb"
curl --fail --silent --show-error --location "$old_url" --output "$old_package"
printf '%s  %s\n' "$old_hash" "$old_package" | sha256sum --check
printf '%s\n' "$old_url" > /evidence/old-package-url.txt
sha256sum "$old_package" > /evidence/old-package.sha256
dpkg-query -W -f='${Version}\n' fzf > /evidence/current-version.txt
sha256sum /usr/bin/fzf > /evidence/current-binary.sha256
dpkg -i "$old_package" > /evidence/seed-old.log 2>&1
[[ $(dpkg-query -W -f='${Version}' fzf) == 0.38.0-1+b1 ]]
run_qa "$qa" receipt > /evidence/old-receipt.log
sha256sum /usr/bin/fzf > /evidence/old-binary.sha256
stat -c '%a %u %g' /usr/bin/fzf > /evidence/old-binary-mode.txt
run_qa "$qa" upgrade > /evidence/upgrade.log 2> /evidence/upgrade.stderr
[[ ! -s /evidence/upgrade.stderr ]]
grep -Fx 'DOTFILES_DEBIAN_QA_EXECUTED mode=upgrade' /evidence/upgrade.log
grep -F 'update evidence=version-change before=0.38.0-1+b1 after=' /evidence/upgrade.log
dpkg-query -W -f='${Version}\n' fzf > /evidence/upgraded-version.txt
cmp /evidence/current-version.txt /evidence/upgraded-version.txt
sha256sum --check /evidence/current-binary.sha256
dpkg -i "$old_package" > /evidence/operator-rollback.log 2>&1
[[ $(dpkg-query -W -f='${Version}' fzf) == 0.38.0-1+b1 ]]
run_qa "$qa" receipt > /evidence/rollback-receipt.log
sha256sum --check /evidence/old-binary.sha256
stat -c '%a %u %g' /usr/bin/fzf > /evidence/rollback-binary-mode.txt
cmp /evidence/old-binary-mode.txt /evidence/rollback-binary-mode.txt
sha256sum --check /evidence/config-before.sha256
find /home/qa/.config -type f -print > /evidence/config-after-rollback.files
cmp /evidence/config-before.files /evidence/config-after-rollback.files
printf 'DOTFILES_DEBIAN_PLATFORM_EXECUTED cases=10 update=version-change rollback=operator-package\n' | tee /evidence/result.txt
# Container deletion, not package uninstall, is the cleanup boundary. This job
# proves real Debian userspace/package transactions on the host Ubuntu kernel.
