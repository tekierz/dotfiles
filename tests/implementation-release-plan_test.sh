#!/usr/bin/env bash
set -eu

plan=tasks/implementation-release-plan-2026-07-13.md
todo=tasks/todo.md
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
  'RK1UO:G1C:4/3/280' \
  'RK1UL:RK1UO:5/3/800' \
  'RK1P:RK1UL:8/5/800' \
  'BA1A:G1C:2/1/250' \
  'BA1SF1:BA1A:5/3/760' \
  'BA1SF2:BA1SF1:4/2/780' \
  'BA1B2:BA1SF2:2/1/800' \
  'BA1C:BA1B2:2/1/360'; do
  id=${entry%%:*}
  rest=${entry#*:}
  dependency=${rest%%:*}
  budget=${rest#*:}
  expect_dep "$id" "$dependency"
  expect_budget "$id" "$budget"
done

expect_dep RS1 RK1P
expect_dep CX1 RK1P
expect_dep NP3 'RK1P, NP1'
expect_dep BC1 BA1C
expect_dep BC2 BA1C
expect_dep BR3 'BR1, BA1C'

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
expect_wave 1B 'RK1UL + BA1SF1' 'Ordinary lifecycle and restore-parent authority'
expect_wave 1C 'RK1P + BA1SF2' 'Privileged supervisor and exact restore leaves'
expect_wave 1D 'RS1 + BA1B2' 'Sequential streaming and immutable catalog kernel'
expect_wave 1E 'UR1 + BA1C' 'Provider domain and opaque restore authority'
expect_wave 1F 'AP1 + BC1' 'Debian adapter and CLI restore'
expect_wave 1G 'UR2 + BC2' 'TUI update routing and TUI restore'
expect_wave 1H 'CX1 + BU1' 'Cancellation propagation and uninstall recovery'
expect_wave 1I 'BR1 + SH1' 'Bounded-read policy and embedded helper'
expect_wave 1J 'BR2 + BR3' 'Config and backup input limits'
wave_count=$(awk -F '|' '$2 ~ /^[[:space:]]*1[A-Z][[:space:]]*$/ { count++ } END { print count+0 }' "$plan")
expect_count "$wave_count" 10 'Wave 1 row count'

if ! rg -q 'RK1UO[[:space:]]*->[[:space:]]*RK1UL[[:space:]]*->[[:space:]]*RK1P' "$plan"; then
  fail 'runner serialization must order RK1UO -> RK1UL -> RK1P'
fi
if ! rg -Fq 'UR2 --> MB1' "$plan"; then
  fail 'dependency graph must route UR2 to MB1'
fi
if ! rg -Fq 'cmd/dotfiles/main.go: RK1P -> BC1 -> BU1 -> CL1 -> PO2 -> NP5.' "$plan"; then
  fail 'cmd serialization must include RK1P before BC1'
fi
if ! rg -Fq 'internal/safefile restore authority: BA1A -> BA1SF1 -> BA1SF2.' "$plan"; then
  fail 'safefile serialization must end at BA1SF2'
fi
if ! rg -Fq 'internal/backup restore authority: BA1B2 -> BA1C -> BR3.' "$plan"; then
  fail 'backup serialization must start at BA1B2'
fi

safety=$(awk '/^### Runtime and update safety/ { on=1 } on && /^\| SP1 \|/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
wave2=$(awk '/^\| SP1 \|/ { on=1 } on && /^### External repositories/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
external=$(awk '/^### External repositories/ { on=1 } on && /^## 6[.]/ { on=0 } on && !/^\| ID \|/ && /^\| [A-Z][A-Z0-9]+ \|/ { count++ } END { print count+0 }' "$plan")
gates=$(awk '/^## 10[.] Release gates/ { on=1 } on && /^## 11[.]/ { on=0 } on && /^\| RG[0-9] / { count++ } END { print count+0 }' "$plan")
catalog=$((1 + safety + wave2 + external))
expect_count "$safety" 20 'safety slice count'
expect_count "$wave2" 20 'Wave 2 engineering slice count'
expect_count "$catalog" 46 'fixed catalog count'
expect_count "$((catalog - 2))" 44 'remaining incomplete slice count'
expect_count "$gates" 10 'release gate count'
if ! rg -Fq '18 safety slices remain' "$plan" "$todo"; then
  fail 'roadmap must state exactly: 18 safety slices remain'
fi

if ! rg -q '^\| RG0 Plan freeze \|.*G3 committed' "$plan"; then
  fail 'RG0 evidence must include committed G3 reconciliation'
fi
todo_active=$(sed -n '1,40p' "$todo")
case $todo_active in *G3*RK1UO*BA1A*) ;; *) fail 'active todo milestones must name G3 and the RK1UO + BA1A opening pair' ;; esac
g2_rule="Branch-local Wave 1 work may proceed while G2 is open, but no closed candidate may integrate until G2's draft PR and remote CI baseline close."
for roadmap in "$plan" "$todo"; do
  if ! rg -Fq "$g2_rule" "$roadmap"; then
    fail "$roadmap must state the branch-local G2 integration rule"
  fi
done

for stale in \
  '| RK1 |' '| BA1 |' '| 1A | RK1 + BA1 |' 'RK1 intentionally absorbs' \
  'internal/backup files: BA1 -> BR3.' 'Requires RK1,' \
  "Open parallel Wave 1A (\`RK1\` plus \`BA1\`)." \
  'No RK1 or BA1 product candidate has started'; do
  if rg -Fq "$stale" "$plan" "$todo"; then
    fail "stale single-slice text remains: $stale"
  fi
done

if [ "$failures" -ne 0 ]; then
  printf '%s release-plan assertion(s) failed\n' "$failures" >&2
  exit 1
fi
printf 'implementation release plan checks passed\n'
