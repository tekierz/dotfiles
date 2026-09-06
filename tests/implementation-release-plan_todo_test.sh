#!/usr/bin/env bash
set -eu

plan=tasks/implementation-release-plan-2026-07-13.md
todo=tasks/todo.md
ledger=tasks/slice-commit-ledger.tsv
failures=0

fail() { printf 'FAIL: %s\n' "$1" >&2; failures=$((failures + 1)); }
require_fixed() { rg -Fq -- "$1" "$2" || fail "$3"; }
require_regex() { rg -q -- "$1" "$todo" || fail "$2"; }
reject_regex() { ! rg -q -- "$1" "$todo" || fail "$2"; }

expected_active_projection_blob=fcb43832f97ae9a5660a660b66d3e022ed21aa95
active_projection_blob=$(awk '/^## Implementation and release plan reset — 2026-07-13$/ { exit } { print }' "$todo" | git hash-object --stdin)
[ "$active_projection_blob" = "$expected_active_projection_blob" ] || fail 'todo active projection must match the exact frozen G3F blob'

require_regex 'G3A.*0182fe7' 'todo must bind closed G3A to 0182fe7'
require_fixed '- The stopped G3B control attempt has no ledger row, candidate digest, or execution authority.' "$todo" 'todo must record stopped non-authoritative G3B'
require_regex 'G3C.*b74c799' 'todo must bind closed G3C to b74c799'
require_fixed "- [x] Close G3D's todo-only control projection at \`ed921d1\` through its normal-v1 ledger row and committed scope state." "$todo" 'todo must bind closed G3D to ed921d1'
require_fixed "- [x] Close G3E's plan accounting and mode replan at \`46735ed\` through the \`g3e-plan-accounting-mode-replan-v4\` normal-v1 ledger row." "$todo" 'todo must bind closed G3E to 46735ed and its ledger row'
g2_rows=$(awk '$0 == "- [ ] Complete G2 authorized worktrees, draft PR, and remote CI baseline." { count++ } END { print count+0 }' "$todo")
[ "$g2_rows" -eq 1 ] || fail 'todo must contain exactly one literal unchecked G2 milestone'
require_regex 'RK1UO.*19fabd7' 'todo must bind RK1UO to 19fabd7'
require_regex 'BA1A.*161671b' 'todo must bind BA1A to 161671b'
require_regex 'BA1SF1.*351fb1c' 'todo must bind BA1SF1 to 351fb1c'
require_regex 'BA1SF2F.*6c58f00' 'todo must bind BA1SF2F to 6c58f00'
require_regex 'BA1SF2D.*75efdc0' 'todo must bind BA1SF2D to 75efdc0'
require_fixed 'None is integrated through G2.' "$todo" 'todo must keep branch-local closures unintegrated'
require_regex 'fixed catalog contains 49 slices including 23 safety slices' 'todo must record fixed 49/23 accounting'
require_regex '44 catalog slices remain' 'todo must record live 44 incomplete accounting'
require_regex '18 safety slices remain before MB1' 'todo must record live 18 safety accounting'
require_fixed 'RK1ULK -> RK1ULA -> RK1P -> RS1' "$todo" 'todo must name the full remaining runner serialization'
require_fixed 'BA1SF2D -> (BA1SF2M + BA1B2) -> BA1C' "$todo" 'todo must name the remaining restore fork/join'
require_regex 'Wave 1A is complete branch-locally; ten barriers remain from 1B through 1K' 'todo must record the exact remaining Wave 1 barriers'
require_fixed "- Wave 1B remains open on RK1ULK; its BA1SF2F half has branch-local evidence at \`6c58f00\`." "$todo" 'todo must keep Wave 1B open on RK1ULK'
require_fixed "- Wave 1C remains open on RK1ULA; its BA1SF2D half has branch-local evidence at \`75efdc0\`." "$todo" 'todo must keep Wave 1C open on RK1ULA'

g3e_row=$'normal-v1\tg3e-plan-accounting-mode-replan-v4\tcdd8b2f63379fd594374955b4719b5a3f5c109c8\tcf8e373ac49a2bf02f4bd2e1049f043730e9dc17\t29656e2902f55c89240af0ff7f0f945280b869c1\ta9354e7987fe9fdbe29f0263355599a18cede950\ta05c7aa264cf8034f364d3273e77aa0f602ec34b7ab3405b0a9a8b036e1e591f\t-'
g3e_rows=$(awk -v wanted="$g3e_row" '$0 == wanted { count++ } END { print count+0 }' "$ledger")
[ "$g3e_rows" -eq 1 ] || fail 'ledger must contain exactly one exact G3E v4 normal-v1 row'

g2_rule="Branch-local Wave 1 product work may proceed while G2 is open, but no closed Wave 1 product candidate may integrate until G2's draft PR and remote CI baseline close."
require_fixed "$g2_rule" "$plan" 'committed plan must retain the G2 integration block'
require_fixed "$g2_rule" "$todo" 'todo must retain the G2 integration block'
require_regex 'pi-agent-integration-spec[.]md.*user-owned, untracked, and untouched' 'todo must preserve Pi ownership'

rg0_rule='RG0 is closed through the prior normal-v1 RG0 roadmap reconciliation plus the G1C, G3A,'
require_fixed "$rg0_rule" "$todo" 'todo must record closed RG0 governance authority'
require_fixed "G3C, and G3D governance rows; G3E closed the reconciled plan at \`46735ed\` without" "$todo" 'todo must bind G3D/G3E governance authority'
require_fixed 'integrating any Wave 1 product candidate.' "$todo" 'todo must separate G3E governance from product integration'
reject_regex 'G3B.*(active|in progress).*control|G3B.*control.*(active|in progress)' 'todo must not revive the stopped G3B control attempt'
reject_regex 'earlier G3D states do not satisfy the gate' 'todo must reject stale conditional RG0 state'
reject_regex '^- \[ \].*G3[DE]' 'todo must not leave closed G3D or G3E milestones open'
reject_regex '^- \[x\] Complete G2 authorized worktrees, draft PR, and remote CI baseline[.]$' 'todo must not mark G2 complete'
reject_regex '(^|[^[:alnum:]])BA1SF2([^FD[:alnum:]]|$)' 'stopped combined BA1SF2 remains in todo'
reject_regex 'RK1UL([^KA[:alnum:]]|$)' 'stopped combined RK1UL remains in todo'
reject_regex '45 (catalog )?slices remain incomplete|19 safety slices remain|43 (catalog )?slices remain incomplete|17 safety slices remain|fixed catalog contains 48 slices|including 22 safety slices' 'stale catalog or safety accounting remains in todo'
reject_regex 'Active (runner|restore|backup) chain|BA1SF2F -> BA1SF2D -> BA1B2 -> BA1C' 'stale active chain remains in todo'
reject_regex 'Pass RG0 local plan/state freeze|Open parallel Wave 1A|No RK1 or BA1 product candidate has started' 'obsolete RG0 or Wave 1 projection remains in todo'

if [ "$failures" -ne 0 ]; then
  printf '%s todo projection assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release todo checks passed\n'
