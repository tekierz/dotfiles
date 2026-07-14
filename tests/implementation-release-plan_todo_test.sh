#!/usr/bin/env bash
set -eu

plan=tasks/implementation-release-plan-2026-07-13.md
todo=tasks/todo.md
failures=0

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}
require_fixed() {
  if ! rg -Fq -- "$1" "$2"; then
    fail "$3"
  fi
}
require_regex() {
  if ! rg -q -- "$1" "$todo"; then
    fail "$2"
  fi
}
reject_regex() {
  if rg -q -- "$1" "$todo"; then
    fail "$2"
  fi
}

require_fixed 'G3A' "$todo" 'todo must name closed G3A plan authority'
require_fixed 'G3B' "$todo" 'todo must name active G3B control sync'
require_regex 'RK1UO.*19fabd7' 'todo must bind RK1UO to close commit 19fabd7'
require_regex 'BA1A.*161671b' 'todo must bind BA1A to close commit 161671b'
require_regex 'BA1SF1.*351fb1c' 'todo must bind BA1SF1 to close commit 351fb1c'
require_regex '44 (catalog )?slices remain incomplete.*18 safety slices remain' 'todo must record live 44/18 accounting'
require_fixed 'RK1ULK -> RK1ULA -> RK1P' "$todo" 'todo must name the active runner chain'
require_fixed 'BA1SF2 -> BA1B2 -> BA1C' "$todo" 'todo must name the active restore chain'

g2_rule="Branch-local Wave 1 product work may proceed while G2 is open, but no closed Wave 1 product candidate may integrate until G2's draft PR and remote CI baseline close."
require_fixed "$g2_rule" "$plan" 'committed plan must retain the G2 product-integration block'
require_fixed "$g2_rule" "$todo" 'todo must retain the G2 product-integration block'
require_regex 'pi-agent-integration-spec[.]md.*user-owned, untracked, and untouched' 'todo must preserve user Pi ownership'

rg0_rule='RG0 remains open until G3B reaches committed through its normal-v1 ledger row; earlier G3B states do not satisfy the gate.'
require_fixed "$rg0_rule" "$todo" 'todo must use the durable conditional RG0 wording'
reject_regex '^- \[x\].*G3B' 'todo must not check off G3B before its later closure commit'
reject_regex 'RG0[^.\n]*(is complete|is closed)' 'todo must not claim RG0 closure in the G3B payload'

reject_regex '(^|[^[:alnum:]])G3([^AB[:alnum:]]|$)' 'rejected aggregate G3 name remains in todo'
reject_regex 'RK1UL([^KA[:alnum:]]|$)' 'stopped RK1UL name remains in todo'
reject_regex '45 (catalog )?slices remain incomplete|45 incomplete slices' 'stale 45-incomplete accounting remains in todo'
reject_regex '19 safety slices remain' 'stale 19-safety accounting remains in todo'
reject_regex 'Open parallel Wave 1A|No RK1 or BA1 product candidate has started' 'pre-foundation Wave 1 status remains in todo'

if [ "$failures" -ne 0 ]; then
  printf '%s todo projection assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release todo checks passed\n'
