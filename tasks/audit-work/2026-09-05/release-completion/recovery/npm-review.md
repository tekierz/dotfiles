# Independent npm acceptance review — 2026-09-05

Reviewed scripts/release-acceptance-npm.sh against internal/pkg/npm_execution_identity.go and internal/operation/journal.go. No product or harness edits by reviewer.

Conditional acceptance: workflow must select an official standalone Node distribution with actions/setup-node before running this script. An arbitrary Homebrew runtime cannot be copied this way. Current local Homebrew Node 26.8.1 reproduced dyld abort (-6) after copying bin/node alone because @rpath/libnode.147.dylib was not copied; current Homebrew npm also includes builtin npmrc containing prefix=/opt/homebrew. The script's pre-install prefix assertion fails closed, so neither issue enables installation into an unexpected prefix, but either stops acceptance. These findings were sent to the root and author; author/root selected official setup-node instead of changing runtime payloads.

The copied npm symlink and native Node binary match the product's accepted identity model: both are independently observed, canonical paths are revalidated, and the product starts canonical Node with canonical npm-cli.js. The clean child environment contains no login tokens or owner npm config, and product execution resets user/global npmrc configuration while discarding npm_config_* overrides. The copied runtime must retain its intrinsic isolated global prefix.

Each live install uses a freshly collected public plan, exact intended tool, pinned expected package arguments, zero filesystem backup targets, full plan hash, phase-aware output/exit checks, exact installed package manifest version, version/help startup, and a subsequent no_changes plan. No authenticated session, saved Claude configuration, package rollback, or owner snapshot recovery is demonstrated.

Suggested evidence strengthening: bind every persisted operation to its reviewed plan hash, verify exactly one successful action, and reject any fabricated backup/rollback fields. Current terminal-status/private-file checks alone establish weaker journal evidence. Root and author notified.

The MCP client sends initialize, awaits its result, sends notifications/initialized, and then tools/list; it checks the expected sequentialthinking tool and terminates the entire subprocess group with bounded waits. This is credential-free stdio discovery acceptance only. Output buffering and response waits are bounded; stderr is a local hosted-runner evidence file. Bash syntax and ShellCheck passed during independent review. No live npm installation was attempted on the owner machine.
