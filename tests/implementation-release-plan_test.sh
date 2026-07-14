#!/usr/bin/env bash
set -eu

plan=tasks/implementation-release-plan-2026-07-13.md
failures=0

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  failures=$((failures + 1))
}

expect_dep() {
  id=$1
  dependency=$2
  if ! awk -F '|' -v wanted_id="$id" -v wanted_dep="$dependency" '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }
    trim($2) == wanted_id && trim($(NF - 1)) == wanted_dep { matches++ }
    END { exit matches == 1 ? 0 : 1 }
  ' "$plan"; then
    fail "$id must have exactly one catalog row depending on $dependency"
  fi
}

expect_budget() {
  id=$1
  budget=$2
  if ! awk -F '|' -v wanted_id="$id" -v wanted_budget="$budget" '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }
    trim($2) == wanted_id {
      row_match=0
      for (field=3; field<NF; field++)
        if (trim($field) == wanted_budget) row_match=1
      matches += row_match
    }
    END { exit matches == 1 ? 0 : 1 }
  ' "$plan"; then
    fail "$id must have exactly one budget row with exact field $budget"
  fi
}

expect_outcome() {
  id=$1
  outcome=$2
  if ! awk -F '|' -v wanted_id="$id" -v wanted_outcome="$outcome" '
    function trim(value) {
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      return value
    }
    trim($2) == wanted_id && trim($3) == wanted_outcome { matches++ }
    END { exit matches == 1 ? 0 : 1 }
  ' "$plan"; then
    fail "$id must have exactly one catalog row with outcome: $outcome"
  fi
}

expect_count() {
  actual=$1
  expected=$2
  label=$3
  if [ "$actual" -ne "$expected" ]; then
    fail "$label must be $expected (found $actual)"
  fi
}

for entry in \
  'G3A:G1C' \
  'G3C:G3A' \
  'G3D:G3C' \
  'G2:G3D and user authorization' \
  'RK1UO:G1C' \
  'RK1ULK:RK1UO' \
  'RK1ULA:RK1ULK' \
  'RK1P:RK1ULA' \
  'BA1A:G1C' \
  'BA1SF1:BA1A' \
  'BA1SF2F:BA1SF1' \
  'BA1SF2D:BA1SF2F' \
  'BA1B2:BA1SF2D' \
  'BA1C:BA1B2' \
  'RS1:RK1P' \
  'CX1:RK1P' \
  'NP3:RK1P, NP1' \
  'BC1:BA1C' \
  'BC2:BA1C' \
  'BU1:BC1' \
  'BR3:BR1, BA1C' \
  'UR2:UR1, RS1'; do
  expect_dep "${entry%%:*}" "${entry#*:}"
done

for entry in \
  'RK1UO:4/3/280' \
  'RK1ULK:2/1/1150' \
  'RK1ULA:4/2/690' \
  'RK1P:8/5/800' \
  'BA1A:2/1/250' \
  'BA1SF1:5/3/760' \
  'BA1SF2F:4/2/650' \
  'BA1SF2D:4/2/780' \
  'BA1B2:2/1/800' \
  'BA1C:2/1/360'; do
  expect_budget "${entry%%:*}" "${entry#*:}"
done

expect_outcome BA1SF2F 'Restore exact captured file bytes and mode, return the exact installed Revision, remove only the exact accepted file leaf, and reject invalid source authority before parent creation'
expect_outcome BA1SF2D 'Restore exact captured recursive directory names, bytes, and modes, return exact installed recursive evidence, remove only the exact accepted directory leaf, and reject invalid source authority before parent creation'

expect_wave() {
  id=$1
  pair=$2
  barrier=$3
  if ! awk -F '|' -v wanted_id="$id" -v wanted_pair="$pair" -v wanted_barrier="$barrier" '
    function trim(value) { gsub(/^[[:space:]]+|[[:space:]]+$/, "", value); return value }
    trim($2) == wanted_id && trim($3) == wanted_pair && trim($4) == wanted_barrier { matches++ }
    END { exit matches == 1 ? 0 : 1 }
  ' "$plan"; then
    fail "$id must pair $pair with barrier: $barrier"
  fi
}

expect_wave 1A 'RK1UO + BA1A' 'Native observer and snapshot extraction foundations'
expect_wave 1B 'RK1ULK + BA1SF2F' 'Private lifecycle kernel and exact file restore leaves'
expect_wave 1C 'RK1ULA + BA1SF2D' 'Public streaming adapters and exact directory restore leaves'
expect_wave 1D 'RK1P + BA1B2' 'Privileged supervisor and immutable catalog kernel'
expect_wave 1E 'RS1 + BA1C' 'Sequential streaming and opaque restore authority'
expect_wave 1F 'UR1 + BC1' 'Provider domain and CLI restore'
expect_wave 1G 'AP1 + BC2' 'Debian adapter and TUI restore'
expect_wave 1H 'UR2 + BU1' 'TUI update routing and uninstall recovery'
expect_wave 1I 'CX1 + BR1' 'Cancellation propagation and bounded-read policy'
expect_wave 1J 'BR2 + SH1' 'Config input limits and embedded helper'
expect_wave 1K 'BR3' 'Backup input limits'
wave_count=$(awk -F '|' '$2 ~ /^[[:space:]]*1[A-Z][[:space:]]*$/ { count++ } END { print count+0 }' "$plan")
expect_count "$wave_count" 11 'Wave 1 row count'
remaining_wave_count=$(awk -F '|' '$2 ~ /^[[:space:]]*1[B-K][[:space:]]*$/ { count++ } END { print count+0 }' "$plan")
expect_count "$remaining_wave_count" 10 'remaining Wave 1 barrier count'

for chain in \
  'G1C --> G3A' \
  'G3A --> G3C' \
  'G3C --> G3D' \
  'G3D --> G2' \
  'internal/runner lifecycle: RK1UO -> RK1ULK -> RK1ULA -> RK1P -> RS1.' \
  'internal/safefile restore authority: BA1A -> BA1SF1 -> BA1SF2F -> BA1SF2D.' \
  'internal/backup restore authority: BA1B2 -> BA1C -> BR3.' \
  'cmd/dotfiles/main.go: RK1P -> BC1 -> BU1 -> CL1 -> PO2 -> NP5.' \
  'internal/ui/installation.go: UR2 -> CX1 -> SP2 -> NP6.' \
  'release workflow: CI1 -> CI2 -> CI3 -> CI4.'; do
  if ! rg -Fq "$chain" "$plan"; then
    fail "missing dependency or serialization chain: $chain"
  fi
done

safety=$(awk '/^### Runtime and update safety/ { on=1 } on && /^\| SP1 \|/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
wave2=$(awk '/^\| SP1 \|/ { on=1 } on && /^### External repositories/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
external=$(awk '/^### External repositories/ { on=1 } on && /^## 6[.]/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
gates=$(awk '/^## 10[.] Release gates/ { on=1 } on && /^## 11[.]/ { on=0 } on && /^\| RG[0-9] / { count++ } END { print count+0 }' "$plan")
catalog=$((1 + safety + wave2 + external))
expect_count "$safety" 22 'safety slice count'
expect_count "$wave2" 20 'Wave 2 engineering slice count'
expect_count "$external" 5 'external slice count'
expect_count "$catalog" 48 'fixed catalog count including G2'
expect_count "$gates" 10 'release gate count'

for truth in \
  'RK1UO, BA1A, BA1SF1, BA1SF2F, and BA1SF2D are closed branch-locally' \
  "BA1SF2F closed branch-locally at \`6c58f00\` and BA1SF2D closed branch-locally at \`75efdc0\`; neither closure is integrated." \
  '43 catalog slices remain incomplete and 17 safety slices remain' \
  'Wave 1A is complete branch-locally; exactly ten Wave 1 barriers remain (1B-1K).' \
  'Wave 1B has completed its BA1SF2F restore half branch-locally but remains open on RK1ULK.' \
  'Wave 1C has completed its BA1SF2D restore half branch-locally but remains open on RK1ULA.' \
  'Active restore chain: BA1B2 -> BA1C.' \
  "RK1ULK's 2/1/1150 budget is a before-the-fact hard exception ceiling, not a target or permission to widen its two-file/one-production-file authority." \
  'Behavior-proven RK1ULK candidates must remain at or below 785 total changed lines.' \
  'At 325 production lines, RK1ULK must stop and re-plan rather than compress behavior or reviewability.' \
  'BA1SF2F and BA1SF2D are serialized because both modify internal/safefile/restore_session_unsupported.go and internal/safefile/unsupported_test.go.' \
  'Requires all 22 safety slices:' \
  'BA1SF1, BA1SF2F, BA1SF2D, BA1B2, BA1C' \
  '| RG0 Plan/control freeze |' \
  'normal-v1 closures for the prior RG0 roadmap reconciliation, G1C, G3A, G3C, and G3D; fixed 48-slice catalog; live 43 incomplete/17 safety accounting; no stale active state' \
  "Branch-local Wave 1 product work may proceed while G2 is open, but no closed Wave 1 product candidate may integrate until G2's draft PR and remote CI baseline close." \
  'The stopped G3B control attempt has no ledger row, candidate digest, or execution authority; G3C and G3D replace it without inheriting its candidate history.' \
  'G3C is plan-only and G3D is todo-only; neither candidate may contain or inherit the stopped G3B payload.' \
  'The serial G3C/G3D governance fast-forward re-closes RG0 and is not Wave 1 product integration.'; do
  if ! rg -Fq "$truth" "$plan"; then
    fail "missing exact release truth: $truth"
  fi
done

for stale in \
  '| RK1 |' \
  '| BA1 |' \
  '| RK1UL |' \
  '| 1A | RK1 + BA1 |' \
  'RK1 intentionally absorbs' \
  'internal/backup files: BA1 -> BR3.' \
  'RK1UO -> RK1UL -> RK1P' \
  'Requires RK1,' \
  '| BA1SF2 |' \
  'BA1SF1 --> BA1SF2' \
  'BA1SF2 --> BA1B2' \
  'BA1SF1 -> BA1SF2.' \
  '| 1C | RK1ULA + BA1SF2 |' \
  '47 slices' \
  '45 catalog slices remain incomplete' \
  '19 safety slices remain' \
  '44 catalog slices remain incomplete' \
  '18 safety slices remain' \
  '| RK1ULK | 2/1/790 |' \
  'BA1SF2F is integrated' \
  'BA1SF2D is integrated' \
  'BA1SF2F and BA1SF2D are integrated' \
  'Requires all 21 safety slices:' \
  'G3A --> G3B' \
  'G3B --> G2' \
  '| G3B | Synchronize the active control projection'; do
  if rg -Fq "$stale" "$plan"; then
    fail "stale aggregate name remains: $stale"
  fi
done

if rg -qi '1150[^.\n]*soft|soft[^.\n]*1150' "$plan"; then
  fail 'RK1ULK 1150 hard exception is falsely described as soft'
fi
if rg -qi '1150[^.\n]*(is|as) (an? )?(implementation )?target' "$plan"; then
  fail 'RK1ULK 1150 hard exception is falsely described as a target'
fi
if rg -qi 'Wave 1B (is|has been) (complete|closed)' "$plan"; then
  fail 'Wave 1B is falsely closed while RK1ULK remains open'
fi
if rg -qi 'Wave 1C (is|has been) (complete|closed)' "$plan"; then
  fail 'Wave 1C is falsely closed while RK1ULA remains open'
fi

if [ "$failures" -ne 0 ]; then
  printf '%s release-plan assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release plan checks passed\n'
