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
  'G3B:G3A' \
  'G2:G3B and user authorization' \
  'RK1UO:G1C' \
  'RK1ULK:RK1UO' \
  'RK1ULA:RK1ULK' \
  'RK1P:RK1ULA' \
  'BA1A:G1C' \
  'BA1SF1:BA1A' \
  'BA1SF2:BA1SF1' \
  'BA1B2:BA1SF2' \
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
  'RK1ULK:2/1/790' \
  'RK1ULA:4/2/690' \
  'RK1P:8/5/800' \
  'BA1A:2/1/250' \
  'BA1SF1:5/3/760' \
  'BA1SF2:4/2/780' \
  'BA1B2:2/1/800' \
  'BA1C:2/1/360'; do
  expect_budget "${entry%%:*}" "${entry#*:}"
done

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
expect_wave 1B 'RK1ULK + BA1SF1' 'Private lifecycle kernel and restore-parent authority'
expect_wave 1C 'RK1ULA + BA1SF2' 'Public streaming adapters and exact restore leaves'
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

for chain in \
  'G1C --> G3A' \
  'G3A --> G3B' \
  'G3B --> G2' \
  'internal/runner lifecycle: RK1UO -> RK1ULK -> RK1ULA -> RK1P -> RS1.' \
  'internal/safefile restore authority: BA1A -> BA1SF1 -> BA1SF2.' \
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
expect_count "$safety" 21 'safety slice count'
expect_count "$wave2" 20 'Wave 2 engineering slice count'
expect_count "$external" 5 'external slice count'
expect_count "$catalog" 47 'fixed catalog count including G2'
expect_count "$gates" 10 'release gate count'

for truth in \
  'RK1UO, BA1A, and BA1SF1 are closed branch-locally' \
  '44 catalog slices remain incomplete and 18 safety slices remain' \
  '| RG0 Plan/control freeze |' \
  "Branch-local Wave 1 product work may proceed while G2 is open, but no closed Wave 1 product candidate may integrate until G2's draft PR and remote CI baseline close." \
  'The serial G3A/G3B governance fast-forward re-closes RG0 and is not Wave 1 product integration.'; do
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
  'Requires RK1,'; do
  if rg -Fq "$stale" "$plan"; then
    fail "stale aggregate name remains: $stale"
  fi
done

if [ "$failures" -ne 0 ]; then
  printf '%s release-plan assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release plan checks passed\n'
