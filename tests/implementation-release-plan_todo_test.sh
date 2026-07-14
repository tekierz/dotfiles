#!/usr/bin/env bash
set -eu

plan=tasks/implementation-release-plan-2026-07-13.md
todo=tasks/todo.md
failures=0

fail() { printf 'FAIL: %s\n' "$1" >&2; failures=$((failures + 1)); }
require_fixed() { rg -Fq -- "$1" "$2" || fail "$3"; }
require_regex() { rg -q -- "$1" "$todo" || fail "$2"; }
reject_regex() { ! rg -q -- "$1" "$todo" || fail "$2"; }

require_regex 'G3A.*0182fe7' 'todo must bind closed G3A to 0182fe7'
require_fixed '- The stopped G3B control attempt has no ledger row, candidate digest, or execution authority.' "$todo" 'todo must record stopped non-authoritative G3B'
require_regex 'G3C.*b74c799' 'todo must bind closed G3C to b74c799'
require_fixed "- [ ] Complete G3D's todo-only control projection through its normal-v1 ledger row and committed scope state." "$todo" 'todo must keep G3D active and conditional'
require_regex 'RK1UO.*19fabd7' 'todo must bind RK1UO to 19fabd7'
require_regex 'BA1A.*161671b' 'todo must bind BA1A to 161671b'
require_regex 'BA1SF1.*351fb1c' 'todo must bind BA1SF1 to 351fb1c'
require_regex '45 (catalog )?slices remain incomplete.*19 safety slices remain' 'todo must record live 45/19 accounting'
require_fixed 'RK1ULK -> RK1ULA -> RK1P' "$todo" 'todo must name the active runner chain'
require_fixed 'BA1SF2F -> BA1SF2D -> BA1B2 -> BA1C' "$todo" 'todo must name the active restore chain'

g2_rule="Branch-local Wave 1 product work may proceed while G2 is open, but no closed Wave 1 product candidate may integrate until G2's draft PR and remote CI baseline close."
require_fixed "$g2_rule" "$plan" 'committed plan must retain the G2 integration block'
require_fixed "$g2_rule" "$todo" 'todo must retain the G2 integration block'
require_regex 'pi-agent-integration-spec[.]md.*user-owned, untracked, and untouched' 'todo must preserve Pi ownership'

rg0_rule='RG0 remains open until G3D reaches committed through its normal-v1 ledger row; earlier G3D states do not satisfy the gate.'
require_fixed "$rg0_rule" "$todo" 'todo must retain the durable conditional RG0 rule'
reject_regex '^- \[x\].*G3D' 'todo must not claim G3D closure before its ledger row'
reject_regex 'G3D.*(is closed|closed at|is committed|committed at)' 'todo must keep G3D conditional rather than claim closure'
reject_regex 'G3B.*(active|in progress).*control|G3B.*control.*(active|in progress)' 'todo must not revive the stopped G3B control attempt'
reject_regex 'RG0[^.\n]*(is complete|is closed)' 'todo must not claim RG0 closure while G3D is active'
reject_regex '(^|[^[:alnum:]])BA1SF2([^FD[:alnum:]]|$)' 'stopped combined BA1SF2 remains in todo'
reject_regex 'RK1UL([^KA[:alnum:]]|$)' 'stopped combined RK1UL remains in todo'
reject_regex '44 (catalog )?slices remain incomplete|18 safety slices remain' 'stale 44/18 accounting remains in todo'
reject_regex 'Pass RG0 local plan/state freeze|Open parallel Wave 1A|No RK1 or BA1 product candidate has started' 'obsolete RG0 or Wave 1 projection remains in todo'

if [ "$failures" -ne 0 ]; then
  printf '%s todo projection assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release todo checks passed\n'
