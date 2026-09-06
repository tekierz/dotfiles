#!/usr/bin/env bash
# Public installer and credential-free startup acceptance on disposable macOS.
set -euo pipefail
if [[ ${GITHUB_ACTIONS:-} != true || ${RUNNER_ENVIRONMENT:-} != github-hosted ||
      ${RUNNER_OS:-} != macOS || ${DOTFILES_RELEASE_ACCEPTANCE:-} != 1 || $(uname -s) != Darwin ]]; then
  echo 'npm release acceptance requires its disposable hosted macOS job' >&2
  exit 125
fi
: "${RUNNER_TEMP:?}"
: "${GITHUB_SHA:?}"
: "${GITHUB_WORKSPACE:?}"
: "${RELEASE_VERSION:?}"
[[ $GITHUB_SHA =~ ^[a-f0-9]{40}$ && $(git -C "$GITHUB_WORKSPACE" rev-parse HEAD) == "$GITHUB_SHA" ]]
[[ $RELEASE_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
git -C "$GITHUB_WORKSPACE" diff --quiet HEAD -- cmd internal go.mod go.sum
trial_root=$(mktemp -d "$RUNNER_TEMP/dotfiles-npm.XXXXXX")
trial_home="$trial_root/home"
trial_runtime="$trial_root/node"
trial_evidence="$RUNNER_TEMP/dotfiles-npm-evidence"
[[ ! -e $trial_evidence ]]
mkdir -m 700 "$trial_home" "$trial_evidence"
mkdir -p "$trial_runtime/bin" "$trial_runtime/lib/node_modules" "$trial_home/tmp"

# The production launcher intentionally discards npm_config_* and ignores npmrc.
# Copy the real runtime so npm's intrinsic global prefix belongs to this trial.
trial_node=$(node -p 'process.execPath')
trial_npm=$(node -e 'console.log(require("fs").realpathSync(process.argv[1]))' "$(command -v npm)")
trial_npm_root=$(dirname "$(dirname "$trial_npm")")
[[ -f $trial_npm_root/package.json && $(basename "$trial_npm") == npm-cli.js ]]
cp "$trial_node" "$trial_runtime/bin/node"
cp -R "$trial_npm_root" "$trial_runtime/lib/node_modules/npm"
ln -s ../lib/node_modules/npm/bin/npm-cli.js "$trial_runtime/bin/npm"
ln -s ../lib/node_modules/npm/bin/npx-cli.js "$trial_runtime/bin/npx"
trial_brew_prefix=$(brew --prefix)
trial_path="$trial_runtime/bin:$trial_brew_prefix/bin:/usr/bin:/bin:/usr/sbin:/sbin"
run_trial() {
  env -i HOME="$trial_home" XDG_CONFIG_HOME="$trial_home/.config" \
    XDG_STATE_HOME="$trial_home/.local/state" XDG_CACHE_HOME="$trial_home/.cache" \
    TMPDIR="$trial_home/tmp" PATH="$trial_path" LANG=en_US.UTF-8 CI=true \
    HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTOREMOVE=1 HOMEBREW_NO_ANALYTICS=1 \
    NPM_CONFIG_USERCONFIG=/dev/null NPM_CONFIG_GLOBALCONFIG=/dev/null/dotfiles-global-npmrc "$@"
}
[[ $(run_trial npm prefix -g) == "$trial_runtime" ]]
run_trial node -e 'const [major,minor]=process.versions.node.split(".").map(Number); if(major<22 || (major===22 && minor<19)) process.exit(1)'
run_trial node --version > "$trial_evidence/node-version.txt"
run_trial npm --version > "$trial_evidence/npm-version.txt"
printf '%s\n' "$trial_runtime" > "$trial_evidence/npm-prefix.txt"
shasum -a 256 "$trial_runtime/bin/node" "$trial_runtime/lib/node_modules/npm/bin/npm-cli.js" > "$trial_evidence/runtime.sha256"
for tool in codex pi; do
  # shellcheck disable=SC2016 # The child shell receives the tool as positional input.
  if run_trial /bin/bash -c 'command -v "$1"' _ "$tool"; then
    echo "Preexisting tool on trial PATH: $tool" >&2
    exit 125
  fi
done
trial_bin="$trial_root/dotfiles"
(cd "$GITHUB_WORKSPACE" && GOTOOLCHAIN=go1.26.8 go build -trimpath -ldflags "-s -w -X main.version=$RELEASE_VERSION" -o "$trial_bin" ./cmd/dotfiles)
go version -m "$trial_bin" > "$trial_evidence/candidate-build.txt"
grep -F 'go1.26.8' "$trial_evidence/candidate-build.txt"
grep -F "vcs.revision=$GITHUB_SHA" "$trial_evidence/candidate-build.txt"
shasum -a 256 "$trial_bin" > "$trial_evidence/candidate.sha256"
printf '%s\n' "$GITHUB_SHA" > "$trial_evidence/candidate-commit.txt"
(cd "$GITHUB_WORKSPACE" && DOTFILES_REAL_NPM_TEST=1 GOTOOLCHAIN=go1.26.8 \
  go test ./internal/pkg -run '^TestNPMExecutionIdentityInstalledNPMConfigNeutralization$' -count=1 -v) \
  > "$trial_evidence/npm-config-regression.txt"
grep -F -- '--- PASS: TestNPMExecutionIdentityInstalledNPMConfigNeutralization ' "$trial_evidence/npm-config-regression.txt"
cd "$trial_home"
: > "$trial_evidence/reviewed-plans.jsonl"
for tool in codex pi; do
  case $tool in
    codex) package=@openai/codex; version=0.144.1; arguments='["install","-g","@openai/codex@0.144.1"]' ;;
    pi) package=@earendil-works/pi-coding-agent; version=0.80.3; arguments='["install","-g","--ignore-scripts","@earendil-works/pi-coding-agent@0.80.3"]' ;;
  esac
  for attempt in 1 2; do
    plan="$trial_evidence/$tool-plan-$attempt.json"
    run_trial "$trial_bin" plan --json --tool "$tool" > "$plan"
    jq -e --arg tool "$tool" '.schema_version == 2 and .status == "ready" and .intent.tools == [$tool] and .summary.apply == 1 and .summary.backup_targets == 0' "$plan"
    phase=$(jq -er '.phase.kind' "$plan")
    if [[ $phase == prerequisite ]]; then
      [[ $attempt == 1 ]]
      jq -e '.phase.index == 1 and .phase.next == "replan_required" and .actions[0].install.steps == [{"kind":"package_manager","provider":"brew","packages":["node"],"casks":[],"arguments":[]}]' "$plan"
    else
      [[ $phase == npm ]]
      jq -e --argjson arguments "$arguments" '.phase.index == 2 and .phase.next == "complete" and .actions[0].install.steps == [{"kind":"npm_global","provider":"npm","packages":[],"casks":[],"arguments":$arguments}]' "$plan"
    fi
    hash=$(jq -er '.authority.plan_hash | select(test("^[a-f0-9]{64}$"))' "$plan")
    set +e
    run_trial "$trial_bin" apply --yes --plan-hash "$hash" --tool "$tool" > "$trial_evidence/$tool-apply-$attempt.stdout" 2> "$trial_evidence/$tool-apply-$attempt.stderr"
    result=$?
    set -e
    if [[ $phase == prerequisite ]]; then
      [[ $result == 3 && ! -s $trial_evidence/$tool-apply-$attempt.stderr ]]
      grep -Fx 'installation phase complete; replan required; run dotfiles plan --json again' "$trial_evidence/$tool-apply-$attempt.stdout"
      jq -cn --arg hash "$hash" --arg tool "$tool" '{hash:$hash,tool:$tool,status:"phase_complete"}' >> "$trial_evidence/reviewed-plans.jsonl"
      continue
    fi
    [[ $result == 0 && ! -s $trial_evidence/$tool-apply-$attempt.stderr ]]
    grep -Eq "^installation applied: operation=[a-zA-Z0-9_.-]+ plan_hash=${hash} succeeded=1 failed=0$" "$trial_evidence/$tool-apply-$attempt.stdout"
    jq -cn --arg hash "$hash" --arg tool "$tool" '{hash:$hash,tool:$tool,status:"succeeded"}' >> "$trial_evidence/reviewed-plans.jsonl"
    break
  done
  [[ $phase == npm && -x $trial_runtime/bin/$tool ]]
  run_trial node -e 'const p=require(process.argv[1]); if(p.version!==process.argv[2]) process.exit(1); console.log(JSON.stringify({name:p.name,version:p.version}))' \
    "$trial_runtime/lib/node_modules/$package/package.json" "$version" > "$trial_evidence/$tool-package.json"
  run_trial "$trial_runtime/bin/$tool" --version > "$trial_evidence/$tool-version.txt"
  grep -F "$version" "$trial_evidence/$tool-version.txt"
  run_trial "$trial_runtime/bin/$tool" --help > "$trial_evidence/$tool-help.txt"
  run_trial "$trial_bin" plan --json --tool "$tool" > "$trial_evidence/$tool-after.json"
  jq -e '.status == "no_changes" and (.authority | has("plan_hash") | not)' "$trial_evidence/$tool-after.json"
done

shopt -s nullglob
records=("$trial_home/.local/state/dotfiles/operations/"*.json)
[[ ${#records[@]} -ge 2 ]]
[[ ${#records[@]} == $(jq -s 'length' "$trial_evidence/reviewed-plans.jsonl") ]]
[[ ${#records[@]} == $(jq -s 'map(.plan_hash) | unique | length' "${records[@]}") ]]
mkdir "$trial_evidence/operations"
for record in "${records[@]}"; do
  [[ $(stat -f '%Lp' "$record") == 600 ]]
  jq -e --slurpfile expected "$trial_evidence/reviewed-plans.jsonl" '
    . as $record | [$expected[] | select(.hash == $record.plan_hash)] as $matches |
    ($matches | length) == 1 and .status == $matches[0].status and
    (.actions | length) == 1 and .actions[0].action_id == ("install:" + $matches[0].tool) and
    .actions[0].status == "succeeded" and (.backup // "") == "" and (.rollback // null) == null' "$record"
  cp "$record" "$trial_evidence/operations/"
done

# This exact stdio definition is part of config.AllMCPServers. The probe starts
# it as a client would; it does not claim Claude authentication or a saved config.
grep -F 'sequentialThinkingMCPPackage = "@modelcontextprotocol/server-sequential-thinking@2026.7.4"' "$GITHUB_WORKSPACE/internal/config/claude.go"
cat > "$trial_root/mcp.py" <<'PY'
import json
import os
import selectors
import signal
import subprocess
import sys
import time

evidence = sys.argv[1]
command = ["npx", "-y", "@modelcontextprotocol/server-sequential-thinking@2026.7.4"]
with open(os.path.join(evidence, "mcp-stderr.txt"), "w") as errors:
    process = subprocess.Popen(command, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=errors,
                               start_new_session=True)
    selector = selectors.DefaultSelector()
    selector.register(process.stdout, selectors.EVENT_READ)
    buffered = b""
    messages = []
    def send(message):
        process.stdin.write((json.dumps(message) + "\n").encode())
        process.stdin.flush()
    def receive(identifier):
        global buffered
        deadline = time.monotonic() + 180
        while time.monotonic() < deadline:
            if b"\n" in buffered:
                line, buffered = buffered.split(b"\n", 1)
                if not line.strip():
                    continue
                message = json.loads(line)
                messages.append(message)
                if message.get("id") == identifier:
                    assert "error" not in message, message
                    return message["result"]
                continue
            if not selector.select(timeout=1):
                continue
            chunk = os.read(process.stdout.fileno(), 65536)
            if not chunk:
                raise RuntimeError("MCP exited before response")
            buffered += chunk
            if len(buffered) > 1048576 or len(messages) > 100:
                raise RuntimeError("MCP response exceeded bounds")
        raise TimeoutError("MCP response timeout")
    try:
        send({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
            "protocolVersion": "2025-03-26", "capabilities": {},
            "clientInfo": {"name": "dotfiles-release-acceptance", "version": "1.0.0"}}})
        initialized = receive(1)
        assert initialized.get("protocolVersion") and initialized.get("serverInfo")
        send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        send({"jsonrpc": "2.0", "id": 2, "method": "tools/list", "params": {}})
        listing = receive(2)
        assert any(tool.get("name") == "sequentialthinking" for tool in listing["tools"])
        with open(os.path.join(evidence, "mcp-responses.json"), "w") as output:
            json.dump({"command": command, "messages": messages, "authentication": "not_required_for_probe"}, output, indent=2)
    finally:
        selector.close()
        try:
            os.killpg(process.pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            os.killpg(process.pid, signal.SIGKILL)
            process.wait(timeout=5)
PY
run_trial python3 "$trial_root/mcp.py" "$trial_evidence"
printf 'DOTFILES_NPM_RELEASE_ACCEPTANCE_PASSED codex=0.144.1 pi=0.80.3 mcp=initialize-and-tools-list authentication=not-tested source=%s\n' \
  "$GITHUB_SHA" | tee "$trial_evidence/result.txt"
# The hosted VM owns teardown. No login tokens are inherited or requested.
