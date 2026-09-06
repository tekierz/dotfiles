#!/usr/bin/env bash
# Run ONLY as the bootstrap entrypoint in the disposable CI Arch container.
set -euo pipefail
if [[ ${CI:-} != true || ${GITHUB_ACTIONS:-} != true || ${DOTFILES_PLATFORM_ACCEPTANCE:-} != 1 ]] ||
   [[ $(uname -s) != Linux || $(id -u) != 0 || ! -f /.dockerenv || ! -f /inputs/dotfiles ]]; then
  echo 'Arch acceptance requires its explicitly authorized disposable CI container' >&2
  exit 125
fi
# shellcheck source=/dev/null
source /etc/os-release
[[ $ID == arch ]]
# The minimal official image has no local signing secret. Initialize its
# disposable keyring before package hooks refresh Arch's trusted keys.
pacman-key --init
pacman-key --populate archlinux
pacman -Syu --noconfirm --needed sudo ca-certificates zsh jq shadow util-linux diffutils
if pacman -Q fzf >/dev/null 2>&1; then
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
printf 'dotfiles-platform-qa-arch-v1\n' > /opt/dotfiles-platform-qa/authorized
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
pacman --version > /evidence/pacman-version.txt
run_qa "$bin" plan --json --tool fzf > /evidence/plan.json 2> /evidence/plan.stderr
[[ ! -s /evidence/plan.stderr ]]
jq -se 'length == 1 and (.[0] | .kind == "dotfiles.plan" and .status == "ready" and .manager == "pacman" and .intent.tools == ["fzf"] and .summary.apply == 1 and .summary.backup_targets == 0 and (.actions | length) == 1)' /evidence/plan.json
jq -e '.actions[0].install | .manager == "pacman" and (.steps | length) == 1 and .steps[0].kind == "package_manager" and .steps[0].provider == "pacman" and .steps[0].packages == ["fzf"] and (.steps[0].casks | length) == 0 and (.steps[0].arguments | length) == 0 and .detector.kind == "package_receipt" and .detector.values == ["fzf"]' /evidence/plan.json
plan_hash=$(jq -er '.authority.plan_hash | select(test("^[a-f0-9]{64}$"))' /evidence/plan.json)
wrong_hash="${plan_hash:0:63}0"
[[ $wrong_hash != "$plan_hash" ]] || wrong_hash="${plan_hash:0:63}1"
set +e
run_qa "$bin" apply --yes --plan-hash "$wrong_hash" --tool fzf > /evidence/wrong.stdout 2> /evidence/wrong.stderr
wrong_exit=$?
set -e
[[ $wrong_exit == 2 && ! -s /evidence/wrong.stdout ]]
[[ $(cat /evidence/wrong.stderr) == 'installation plan changed; run dotfiles plan --json again' ]]
if pacman -Q fzf >/dev/null 2>&1; then exit 1; fi
run_qa "$bin" apply --yes --plan-hash "$plan_hash" --tool fzf > /evidence/apply.stdout 2> /evidence/apply.stderr
[[ ! -s /evidence/apply.stderr && $(wc -l < /evidence/apply.stdout) == 1 ]]
grep -Eq "^installation applied: operation=[a-zA-Z0-9_.-]+ plan_hash=${plan_hash} succeeded=1 failed=0$" /evidence/apply.stdout
run_qa "$qa" receipt > /evidence/receipt.log
run_qa "$qa" update > /evidence/update.log 2> /evidence/update.stderr
[[ ! -s /evidence/update.stderr ]]
grep -Fx 'DOTFILES_ARCH_QA_EXECUTED mode=update' /evidence/update.log
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
printf 'DOTFILES_ARCH_PLATFORM_EXECUTED cases=7 update=no-op\n' | tee /evidence/result.txt
# Container deletion is the cleanup boundary. This is real Arch userspace and
# pacman execution on the hosted Ubuntu kernel, without AUR/paru coverage.
