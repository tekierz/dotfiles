#!/usr/bin/env bash

set -uo pipefail

test_dir=''
repo_root=''
tmp_root=''
if ! test_dir="$(CDPATH='' cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"; then
  printf 'test setup: cannot resolve test directory\n' >&2
  exit 1
fi
if ! repo_root="$(CDPATH='' cd -- "$test_dir/.." && pwd)"; then
  printf 'test setup: cannot resolve repository root\n' >&2
  exit 1
fi
checker_source="$repo_root/scripts/check-slice-scope.sh"
if [[ ! -f "$checker_source" || ! -r "$checker_source" ]]; then
  printf 'test setup: slice-scope checker is not readable\n' >&2
  exit 1
fi
if ! tmp_root="$(mktemp -d "${TMPDIR:-/tmp}/slice-scope-test.XXXXXX")"; then
  printf 'test setup: cannot create temporary root\n' >&2
  exit 1
fi
if [[ -z "$tmp_root" || ! -d "$tmp_root" || "${tmp_root##*/}" != slice-scope-test.* ]]; then
  printf 'test setup: temporary root failed verification\n' >&2
  exit 1
fi
if ! printf '%s\n' 'slice-scope-test-root' > "$tmp_root/.verified-test-root"; then
  printf 'test setup: cannot mark temporary root\n' >&2
  exit 1
fi
failures=0
test_count=0

cleanup() {
  if [[ -z "$tmp_root" ]]; then
    return 0
  fi
  if [[ ! -d "$tmp_root" || "${tmp_root##*/}" != slice-scope-test.* || ! -f "$tmp_root/.verified-test-root" ]]; then
    printf 'test cleanup: refusing unverified temporary root\n' >&2
    return 1
  fi
  rm -rf -- "$tmp_root" || {
    printf 'test cleanup: failed to remove temporary root\n' >&2
    return 1
  }
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM

write_contract() {
  local repo="$1"
  local slice_id="$2"
  local state="$3"

  printf '%s\n' \
    '# Disposable slice-scope policy fixture.' \
    'version=2' \
    "slice_id=$slice_id" \
    "state=$state" \
    'max_total_files=4' \
    'max_production_files=2' \
    'max_changed_lines=80' \
    'allow=internal/payload.go' \
    'allow=internal/payload_test.go' \
    'allow=tests/check-slice-scope_test.sh' \
    'allow=Makefile' \
    'test=internal/payload_test.go' \
    > "$repo/tasks/current-slice.scope" || return 1
}

write_v1_contract() {
  local repo="$1"
  local slice_id="$2"
  local state="$3"

  printf '%s\n' \
    '# Disposable v1 state-policy fixture.' \
    'version=1' \
    "slice_id=$slice_id" \
    "state=$state" \
    'max_total_files=1' \
    'max_production_files=1' \
    'max_changed_lines=20' \
    'allow=internal/payload.go' \
    > "$repo/tasks/current-slice.scope" || return 1
}

new_fixture() {
  local slice_id="$1"
  local state="$2"
  local version="${3:-2}"
  local repo

  repo="$(mktemp -d "$tmp_root/fixture.XXXXXX")" || return 1
  [[ -n "$repo" && -d "$repo" ]] || return 1
  mkdir -p "$repo/internal" "$repo/scripts" "$repo/tasks" "$repo/tests" || return 1
  cp "$checker_source" "$repo/scripts/check-slice-scope.sh" || return 1
  chmod +x "$repo/scripts/check-slice-scope.sh" || return 1
  if [[ "$version" == "1" ]]; then
    write_v1_contract "$repo" "$slice_id" "$state" || return 1
  else
    write_contract "$repo" "$slice_id" "$state" || return 1
  fi
  printf '%s\n' 'package payload' > "$repo/internal/payload.go" || return 1
  printf '%s\n' 'package payload_test' > "$repo/internal/payload_test.go" || return 1
  printf '%s\n' '# fixture test' > "$repo/tests/check-slice-scope_test.sh" || return 1
  printf '%s\n' 'fixture:' > "$repo/Makefile" || return 1

  (
    set -e
    cd "$repo"
    git init -q
    git config user.name 'Slice Scope Test'
    git config user.email 'slice-scope@example.invalid'
    git add scripts/check-slice-scope.sh tasks/current-slice.scope internal/payload.go internal/payload_test.go tests/check-slice-scope_test.sh Makefile
    git commit -q -m 'fixture baseline'
  ) || return 1

  printf '%s\n' "$repo" || return 1
}

stage_payload() {
  local repo="$1"

  printf '%s\n' 'package payload' '' 'const changed = true' > "$repo/internal/payload.go" || return 1
  git -C "$repo" add internal/payload.go || return 1
}

stage_contract() {
  local repo="$1"
  local slice_id="$2"
  local state="$3"

  write_contract "$repo" "$slice_id" "$state" || return 1
  git -C "$repo" add tasks/current-slice.scope || return 1
}

stage_v1_contract() {
  local repo="$1"
  local slice_id="$2"
  local state="$3"

  write_v1_contract "$repo" "$slice_id" "$state" || return 1
  git -C "$repo" add tasks/current-slice.scope || return 1
}

assert_rejected_exactly() {
  local label="$1"
  local expected="$2"
  local repo="$3"
  local mode="$4"
  local output
  local status

  test_count=$((test_count + 1))
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh "$mode" 2>&1)"
  status=$?

  if (( status == 0 )); then
    printf 'not ok %d - %s\n' "$test_count" "$label"
    printf '  expected rejection: %s\n' "$expected"
    printf '  actual: checker exited 0 and accepted the invalid policy state\n'
    failures=$((failures + 1))
    return
  fi

  if [[ "$output" != "$expected" ]]; then
    printf 'not ok %d - %s\n' "$test_count" "$label"
    printf '  expected: %s\n' "$expected"
    printf '  actual: %s\n' "$output"
    failures=$((failures + 1))
    return
  fi

  printf 'ok %d - %s\n' "$test_count" "$label"
}

assert_accepted() {
  local label="$1"
  local repo="$2"
  local mode="$3"
  local output status

  test_count=$((test_count + 1))
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh "$mode" 2>&1)"
  status=$?
  if (( status != 0 )); then
    printf 'not ok %d - %s\n' "$test_count" "$label"
    printf '  expected acceptance; actual: %s\n' "$output"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - %s\n' "$test_count" "$label"
}

assert_g1b_genesis_guardrail() {
  local bootstrap_repo="$1"
  local other_repo="$2"
  local existing_repo="$3"
  local missing_repo="$4" extra_repo="$5"
  local rejection='slice-check: guardrail candidate may contain only control-plane paths: tasks/slice-commit-ledger.tsv'
  local divergence='slice-check: staged candidate digest does not match verified scope trailer'
  local output status repo problem=''

  test_count=$((test_count + 1))
  output="$(cd "$bootstrap_repo" && bash scripts/check-slice-scope.sh --guardrail-candidate 2>&1)"
  status=$?
  if (( status != 0 )); then
    problem="expected G1B genesis acceptance; actual: $output"
  fi
  output="$(cd "$other_repo" && bash scripts/check-slice-scope.sh --guardrail-candidate 2>&1)"
  status=$?
  if (( status == 0 )) || [[ "$output" != "$rejection" ]]; then
    problem="${problem:+$problem; }other-slice genesis escaped: $output"
  fi
  output="$(cd "$existing_repo" && bash scripts/check-slice-scope.sh --guardrail-candidate 2>&1)"
  status=$?
  if (( status == 0 )) || [[ "$output" != "$rejection" ]]; then
    problem="${problem:+$problem; }existing ledger escaped: $output"
  fi
  for repo in "$missing_repo" "$extra_repo"; do
    output="$(cd "$repo" && bash scripts/check-slice-scope.sh --guardrail-candidate 2>&1)"
    status=$?
    if (( status == 0 )) || [[ "$output" != "$divergence" ]]; then
      problem="${problem:+$problem; }inexact staged paths escaped: $output"
    fi
  done
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - G1B checker and ledger genesis guardrail\n' "$test_count"
    printf '  %s\n' "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - G1B checker and ledger genesis guardrail\n' "$test_count"
}

assert_canonical_genesis_mutations() {
  local expected='slice-check: staged G1B ledger genesis does not match canonical bootstrap history'
  local mutation repo output status problem=''

  test_count=$((test_count + 1))
  for mutation in header record-type slice-id commit digest reason missing-row extra-row; do
    repo="$(new_genesis_guardrail_fixture g1b-ledger-closure 0 "$mutation")" || return 1
    output="$(cd "$repo" && bash scripts/check-slice-scope.sh --guardrail-candidate 2>&1)"
    status=$?
    if (( status == 0 )) || [[ "$output" != "$expected" ]]; then
      problem="${problem:+$problem; }$mutation escaped: $output"
    fi
  done
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - G1B genesis is byte-exact canonical history\n' "$test_count"
    printf '  %s\n' "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - G1B genesis is byte-exact canonical history\n' "$test_count"
}

assert_test_deletion_rejections() {
  local staged_repo="$1" red_repo="$2" ledger_repo="$3" repo mode expected output status problem=''

  test_count=$((test_count + 1))
  for repo in "$staged_repo" "$red_repo" "$ledger_repo"; do
    case "$repo" in
      "$staged_repo") expected='slice-check: explicit test candidate may not delete: internal/payload_test.go'; mode=--test-candidate ;;
      "$red_repo") expected='slice-check: tests commit may not delete explicit test: internal/payload_test.go'; mode=--contract-candidate ;;
      *) expected='slice-check: ledger tests_commit may not delete explicit test: internal/payload_test.go'; mode=--ledger-candidate ;;
    esac
    output="$(cd "$repo" && bash scripts/check-slice-scope.sh "$mode" 2>&1)"
    status=$?
    if (( status == 0 )) || [[ "$output" != "$expected" ]]; then
      problem="${problem:+$problem; }$repo: $output"
    fi
  done
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - explicit test deletion is forbidden at every evidence boundary\n  %s\n' "$test_count" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - explicit test deletion is forbidden at every evidence boundary\n' "$test_count"
}

replace_contract_line() {
  local repo="$1"
  local prefix="$2"
  local replacement="$3"
  local next="$repo/tasks/scope.next"

  awk -v prefix="$prefix" -v replacement="$replacement" \
    'index($0, prefix) == 1 {print replacement; next} {print}' \
    "$repo/tasks/current-slice.scope" > "$next" || return 1
  mv "$next" "$repo/tasks/current-slice.scope" || return 1
}

remove_contract_lines() {
  local repo="$1"
  local prefix="$2"
  local next="$repo/tasks/scope.next"

  awk -v prefix="$prefix" 'index($0, prefix) != 1 {print}' \
    "$repo/tasks/current-slice.scope" > "$next" || return 1
  mv "$next" "$repo/tasks/current-slice.scope" || return 1
}

hash_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  else
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  fi
}

expected_worktree_digest() {
  local repo="$1"
  shift
  local index_file manifest_file objects entry scope_mode scope_blob

  entry="$(git -C "$repo" ls-files -s -- tasks/current-slice.scope)" || return 1
  read -r scope_mode scope_blob _ <<< "$entry"
  index_file="$(mktemp "$tmp_root/digest-index.XXXXXX")" || return 1
  manifest_file="$(mktemp "$tmp_root/digest-manifest.XXXXXX")" || return 1
  objects="$(mktemp -d "$tmp_root/digest-objects.XXXXXX")" || return 1
  rm -f -- "$index_file" || return 1
  GIT_INDEX_FILE="$index_file" GIT_OBJECT_DIRECTORY="$objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$repo/.git/objects" \
    git -C "$repo" read-tree HEAD || return 1
  GIT_INDEX_FILE="$index_file" GIT_OBJECT_DIRECTORY="$objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$repo/.git/objects" \
    git -C "$repo" update-index --add --cacheinfo \
    "$scope_mode" "$scope_blob" tasks/current-slice.scope || return 1
  GIT_INDEX_FILE="$index_file" GIT_OBJECT_DIRECTORY="$objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$repo/.git/objects" \
    git -C "$repo" add -A -- "$@" || return 1
  printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest_file" || return 1
  GIT_INDEX_FILE="$index_file" GIT_OBJECT_DIRECTORY="$objects" GIT_ALTERNATE_OBJECT_DIRECTORIES="$repo/.git/objects" \
    git -C "$repo" diff --cached --raw --no-renames --no-abbrev -z HEAD -- "$@" \
    >> "$manifest_file" || return 1
  hash_file "$manifest_file"
}

assert_digest_equivalence() {
  local repo="$1" digest="$2" mode="$3" label="$4"
  shift 4
  local before after before_index after_index before_status before_refs before_objects before_locks output repeat status problem=''
  local trailer="Candidate-SHA256: $digest"

  test_count=$((test_count + 1))
  git -C "$repo" status --short >/dev/null || return 1
  before="$(git -C "$repo" ls-files -s)" || return 1
  before_index="$(hash_file "$repo/.git/index")" || return 1
  before_status="$(git -C "$repo" -c status.renames=false status --porcelain=v1 --untracked-files=all)" || return 1
  before_refs="$(git -C "$repo" show-ref)" || return 1
  before_objects="$(find "$repo/.git/objects" -type f -print | sort)" || return 1
  before_locks="$(find "$repo/.git" -name '*.lock' -print | sort)" || return 1
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh --candidate-digest 2>&1)"
  status=$?
  repeat="$(cd "$repo" && bash scripts/check-slice-scope.sh --candidate-digest 2>&1)"
  after="$(git -C "$repo" ls-files -s)" || return 1
  after_index="$(hash_file "$repo/.git/index")" || return 1
  (( status == 0 )) && [[ "$output" == "$trailer" ]] || problem="digest output: $output"
  [[ "$repeat" == "$output" ]] || problem="${problem:+$problem; }digest is nondeterministic: $repeat"
  [[ "$before" == "$after" ]] || problem="${problem:+$problem; }real index mutated"
  [[ "$before_index" == "$after_index" ]] || problem="${problem:+$problem; }index bytes mutated"
  [[ "$before_status" == "$(git -C "$repo" -c status.renames=false status --porcelain=v1 --untracked-files=all)" ]] || problem="${problem:+$problem; }status mutated"
  [[ "$before_refs" == "$(git -C "$repo" show-ref)" ]] || problem="${problem:+$problem; }refs mutated"
  [[ "$before_objects" == "$(find "$repo/.git/objects" -type f -print | sort)" ]] || problem="${problem:+$problem; }object database leaked"
  [[ "$before_locks" == "$(find "$repo/.git" -name '*.lock' -print | sort)" ]] || problem="${problem:+$problem; }lock leaked"
  git -C "$repo" commit -q -m 'verified scope' -m "$trailer" || return 1
  git -C "$repo" add -A -- "$@" || return 1
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh "$mode" 2>&1)"
  status=$?
  (( status == 0 )) || problem="${problem:+$problem; }candidate equivalence: $output"
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - %s\n  %s\n' "$test_count" "$label" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - %s\n' "$test_count" "$label"
}

assert_make_candidate_targets() {
  local specs target expected output status problem=''
  specs=$'slice-check\tbash scripts/check-slice-scope.sh\n'
  specs+=$'slice-check-test\tbash tests/check-slice-scope_test.sh\n'
  specs+=$'slice-check-contract-candidate\tbash scripts/check-slice-scope.sh --contract-candidate\n'
  specs+=$'slice-check-test-candidate\tbash scripts/check-slice-scope.sh --test-candidate\n'
  specs+=$'slice-check-guardrail-candidate\tbash scripts/check-slice-scope.sh --guardrail-candidate\n'
  specs+=$'slice-check-ledger-candidate\tbash scripts/check-slice-scope.sh --ledger-candidate\n'
  specs+=$'slice-check-candidate-digest\tbash scripts/check-slice-scope.sh --candidate-digest\n'
  specs+=$'slice-check-candidate\tbash scripts/check-slice-scope.sh --candidate'

  test_count=$((test_count + 1))
  while IFS=$'\t' read -r target expected; do
    output="$(make -s -n -C "$repo_root" "$target" 2>/dev/null)"
    status=$?
    (( status == 0 )) && [[ "$output" == "$expected" ]] || problem="${problem:+$problem; }$target: $output"
    awk -v target="$target" '$1 == ".PHONY:" {for (i=2;i<=NF;i++) if ($i==target) found=1} END {exit !found}' \
      "$repo_root/Makefile" || problem="${problem:+$problem; }$target is not phony"
  done <<< "$specs"
  output="$(make -s -n -C "$repo_root" slice-check-candidate MODE=guardrail-candidate 2>/dev/null)"
  [[ "$output" == 'bash scripts/check-slice-scope.sh --candidate' ]] || \
    problem="${problem:+$problem; }MODE override: $output"
  output="$(make -s -C "$repo_root" help 2>/dev/null)"
  while read -r target; do
    [[ "$output" == *"  $target "* ]] || problem="${problem:+$problem; }help omits $target"
  done < <(printf '%s\n' slice-check slice-check-test slice-check-contract-candidate \
    slice-check-test-candidate slice-check-guardrail-candidate slice-check-ledger-candidate \
    slice-check-candidate-digest slice-check-candidate)
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - Make maps every candidate mode and marks it phony\n  %s\n' "$test_count" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - Make maps every candidate mode and marks it phony\n' "$test_count"
}

assert_rejection_matrix() {
  local label="$1" repo mode expected output status problem=''
  shift
  test_count=$((test_count + 1))
  while (( $# > 0 )); do
    repo="$1"
    mode="$2"
    expected="$3"
    shift 3
    output="$(cd "$repo" && bash scripts/check-slice-scope.sh "$mode" 2>&1)"
    status=$?
    if (( status == 0 )) || [[ "$output" != "$expected" ]]; then
      problem="${problem:+$problem; }$mode $expected => $output"
    fi
  done
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - %s\n  %s\n' "$test_count" "$label" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - %s\n' "$test_count" "$label"
}

assert_rg0_roadmap_truth() {
  local todo="$repo_root/tasks/todo.md" ledger="$repo_root/tasks/slice-commit-ledger.tsv"
  local ledger_truth expected problem=''

  test_count=$((test_count + 1))
  ledger_truth="$(awk -F '\t' '$2 == "g1c-guardrail-adoption" {rows++; if ($1 == "normal-v1" && $8 == "-") valid++} END {print rows + 0 ":" valid + 0}' "$ledger")"
  [[ "$ledger_truth" == "1:1" ]] || problem="G1C ledger truth is $ledger_truth"
  while IFS= read -r expected; do
    [[ "$(grep -Fxc -- "$expected" "$todo")" == "1" ]] || problem="${problem:+$problem; }missing or duplicate: $expected"
  done <<'EOF'
- [x] Deliver G1C Make targets and workflow adoption through the first normal closure.
- [x] Pass RG0 local plan/state freeze.
- [ ] Complete G2 authorized worktrees, draft PR, and remote CI baseline.
- [ ] Open parallel Wave 1A (`RK1` plus `BA1`).
- G1C closed through the first normal ledger row; RG0 local plan/state freeze is complete.
- No RK1 or BA1 product candidate has started; G2 and Wave 1A remain open.
EOF
  grep -Fq -- 'Pass RG0 and open parallel Wave 1A' "$todo" && problem="${problem:+$problem; }stale combined RG0/Wave 1A wording remains"
  grep -Fq -- 'blocked until G1C closes' "$todo" && problem="${problem:+$problem; }stale G1C blocking wording remains"
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - RG0 roadmap matches committed G1C evidence\n  %s\n' "$test_count" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - RG0 roadmap matches committed G1C evidence\n' "$test_count"
}

assert_normal_closure_successor() {
  local repo="$1" output status problem=''

  test_count=$((test_count + 1))
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh --ledger-candidate 2>&1)"
  status=$?
  (( status == 0 )) || problem="ledger: $output"
  git -C "$repo" commit -q -m 'normal ledger closure' || return 1
  stage_g1b_contract "$repo" committed g1c-guardrail-adoption || return 1
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh --contract-candidate 2>&1)"
  status=$?
  (( status == 0 )) || problem="${problem:+$problem; }committed: $output"
  git -C "$repo" commit -q -m 'committed G1C scope' || return 1
  stage_contract "$repo" rk1-runner-kernel planned || return 1
  output="$(cd "$repo" && bash scripts/check-slice-scope.sh --contract-candidate 2>&1)"
  status=$?
  (( status == 0 )) || problem="${problem:+$problem; }successor: $output"
  if [[ -n "$problem" ]]; then
    printf 'not ok %d - normal ledger closure authorizes G1C successor\n  %s\n' "$test_count" "$problem"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - normal ledger closure authorizes G1C successor\n' "$test_count"
}

prepare_verified_candidate() {
  local repo="$1"
  local scope_source="$2"
  shift 2
  local index_file manifest_file entry scope_mode scope_blob digest

  replace_contract_line "$repo" 'state=' 'state=verified' || return 1
  git -C "$repo" add tasks/current-slice.scope || return 1
  entry="$(git -C "$repo" ls-files -s -- tasks/current-slice.scope)" || return 1
  read -r scope_mode scope_blob _ <<< "$entry"
  if [[ "$scope_source" == "parent" ]]; then
    entry="$(git -C "$repo" ls-tree HEAD -- tasks/current-slice.scope)" || return 1
    read -r scope_mode scope_blob _ <<< "$entry"
  fi
  index_file="$(mktemp "$tmp_root/index.XXXXXX")" || return 1
  manifest_file="$(mktemp "$tmp_root/manifest.XXXXXX")" || return 1
  rm -f -- "$index_file" || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" read-tree HEAD || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" add -A -- "$@" || return 1
  printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest_file" || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" diff --cached --raw --no-renames --no-abbrev -z HEAD -- "$@" \
    >> "$manifest_file" || return 1
  digest="$(hash_file "$manifest_file")" || return 1
  git -C "$repo" commit -q -m 'verified scope' -m "Candidate-SHA256: $digest" || return 1
  git -C "$repo" add -A -- "$@" || return 1
  printf '%s\n' "$digest"
}

write_g1a_migration_contract() {
  local repo="$1"
  local version="$2"
  local state="$3"
  shift 3

  printf '%s\n' \
    '# G1A schema migration fixture.' \
    "version=$version" \
    'slice_id=g1a-candidate-authority' \
    "state=$state" \
    'max_total_files=2' \
    'max_production_files=1' \
    'max_changed_lines=800' \
    'allow=scripts/check-slice-scope.sh' \
    'allow=tests/check-slice-scope_test.sh' \
    'allow=internal/payload_test.go' \
    "$@" \
    > "$repo/tasks/current-slice.scope" || return 1
}

ledger_header=$'record_type\tslice_id\tcontract_frozen_commit\ttests_commit\tverified_scope_commit\tpayload_commit\tcandidate_sha256\treason'

write_canonical_genesis() {
  local file="$1"
  printf '%s\n' \
    "$ledger_header" \
    $'bootstrap-v1\tg0-plan-state-reset\t1aa1c36b3a21d9bc9e31362b21e536e57187e711\t-\t-\tee1b5847ad026dcdfb4871d41064e2b9742cedfd\t-\tpre-g1-unverified' \
    $'bootstrap-v1\tg1-slice-state-enforcement\t668f1acfe751970b205af6eed6bc83ad8bf5894f\t4675a1a235c35e6cd3a03aa3871088e003c1659b\t-\t-\t-\tsuperseded-at-tests-red' \
    $'bootstrap-v1\tg1-replan-catalog\t1bb4f9b6e56d7c1b09cc0602b4ddda29593f58e2\t-\t-\t493235a9a98f64e38cdae105144fcd71eefe8bce\t-\tpre-enforcement-replan-catalog' \
    $'bootstrap-v1\tg1a-candidate-authority\tdf0bd217ce0f8e36ae5f44e6d09a0a187c161d43\t770bf8d1ec2ebf28fdb9e91362723aa9ce90a6fd\ta62cc74e32fe38821a4524e1bf2d18884d1045eb\t3c6025adadde63fcc5d7289b06ae947e658a5b43\tf0727c52696ef06e4ff21eb10b5bcd8f3ca1f4404a5218963b495bd1670722ed\tcandidate-checker-self-upgrade-bootstrap' \
    > "$file" || return 1
}
g1b_frozen=''
g1b_tests=''
g1b_reviewed=''
g1b_verified=''
g1b_payload=''
g1b_digest=''

new_g1b_fixture() {
  local state="$1"
  local repo

  repo="$(new_fixture 'g1b-ledger-closure' "$state")" || return 1
  printf '%s\n' "$ledger_header" > "$repo/tasks/slice-commit-ledger.tsv" || return 1
  printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$repo/tasks/current-slice.scope" || return 1
  git -C "$repo" add tasks/current-slice.scope tasks/slice-commit-ledger.tsv || return 1
  git -C "$repo" commit -q -m 'G1B fixture contract' || return 1
  printf '%s\n' "$repo"
}

new_g1c_fixture() {
  local state="$1" repo
  repo="$(new_fixture g1c-guardrail-adoption "$state")" || return 1
  cp "$repo_root/tasks/slice-commit-ledger.tsv" "$repo/tasks/slice-commit-ledger.tsv" || return 1
  printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$repo/tasks/current-slice.scope" || return 1
  git -C "$repo" add tasks/current-slice.scope tasks/slice-commit-ledger.tsv || return 1
  git -C "$repo" commit -q -m 'G1C fixture contract' || return 1
  printf '%s\n' "$repo"
}

new_planning_fixture() {
  local duplicate="${1:-}" repo
  repo="$(new_fixture g1c-guardrail-adoption committed)" || return 1
  printf '%s\n' "$ledger_header" \
    $'normal-v1\tg1c-guardrail-adoption\t-\t-\t-\t-\t-\t-' \
    > "$repo/tasks/slice-commit-ledger.tsv" || return 1
  if [[ -n "$duplicate" ]]; then
    printf '%s\t%s\t-\t-\t-\t-\t-\t-\n' normal-v1 "$duplicate" >> "$repo/tasks/slice-commit-ledger.tsv" || return 1
  fi
  printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$repo/tasks/current-slice.scope" || return 1
  git -C "$repo" add tasks/current-slice.scope tasks/slice-commit-ledger.tsv || return 1
  git -C "$repo" commit -q -m 'committed ledger reservation' || return 1
  printf '%s\n' "$repo"
}

new_digest_rejection_fixture() {
  local shape="$1" state=reviewed repo
  [[ "$shape" != nonreviewed ]] || state=implemented
  repo="$(new_fixture slice-one "$state")" || return 1
  case "$shape" in
    ledger)
      printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$repo/tasks/current-slice.scope" || return 1
      printf '%s\n' "$ledger_header" > "$repo/tasks/slice-commit-ledger.tsv" || return 1
      git -C "$repo" add tasks/current-slice.scope tasks/slice-commit-ledger.tsv || return 1
      git -C "$repo" commit -q -m 'ledger digest authority' || return 1
      printf '%s\n' '# worktree ledger' >> "$repo/tasks/slice-commit-ledger.tsv" || return 1
      ;;
    staged-ignored)
      printf '%s\n' 'ignore=tasks/pi-agent-integration-spec.md' >> "$repo/tasks/current-slice.scope" || return 1
      git -C "$repo" add tasks/current-slice.scope || return 1
      git -C "$repo" commit -q -m 'ignored digest authority' || return 1
      printf '%s\n' user-owned > "$repo/tasks/pi-agent-integration-spec.md" || return 1
      git -C "$repo" add -f tasks/pi-agent-integration-spec.md || return 1
      ;;
    over-files)
      printf '%s\n' 'allow=scripts/check-slice-scope.sh' 'allow=tasks/todo.md' 'allow=tasks/workflow-guardrails.md' >> "$repo/tasks/current-slice.scope" || return 1
      printf '%s\n' todo > "$repo/tasks/todo.md" && printf '%s\n' workflow > "$repo/tasks/workflow-guardrails.md" || return 1
      git -C "$repo" add tasks/current-slice.scope tasks/todo.md tasks/workflow-guardrails.md && git -C "$repo" commit -q -m 'digest file budget authority' || return 1
      for path in Makefile scripts/check-slice-scope.sh tests/check-slice-scope_test.sh tasks/todo.md tasks/workflow-guardrails.md; do printf '%s\n' '# budget payload' >> "$repo/$path" || return 1; done
      ;;
  esac
  if [[ "$shape" != empty && "$shape" != ledger && "$shape" != staged-ignored && "$shape" != over-files ]]; then
    stage_payload "$repo" || return 1
    git -C "$repo" reset -q internal/payload.go || return 1
  fi
  replace_contract_line "$repo" state= state=verified || return 1
  [[ "$shape" != frozen ]] || replace_contract_line "$repo" max_changed_lines= max_changed_lines=81 || return 1
  [[ "$shape" == unstaged-scope ]] || git -C "$repo" add tasks/current-slice.scope || return 1
  case "$shape" in
    staged-payload) git -C "$repo" add internal/payload.go || return 1 ;;
    mixed-xy) git -C "$repo" add internal/payload.go && printf '%s\n' '// worktree shadow' >> "$repo/internal/payload.go" || return 1 ;;
    mixed-control-product) printf '%s\n' '# control worktree' >> "$repo/Makefile" || return 1 ;;
    over-lines) for ((i = 0; i < 90; i++)); do printf '%s\n' '# budget line' >> "$repo/internal/payload.go" || return 1; done ;;
    outside) mkdir -p "$repo/docs" && printf '%s\n' outside > "$repo/docs/outside.md" || return 1 ;;
    symlink) rm -f "$repo/internal/payload.go" && ln -s payload_test.go "$repo/internal/payload.go" || return 1 ;;
    fifo) rm -f "$repo/internal/payload.go" && mkfifo "$repo/internal/payload.go" || return 1 ;;
  esac
  printf '%s\n' "$repo"
}

stage_g1b_contract() {
  local repo="$1"
  local state="$2"
  local slice="${3:-g1b-ledger-closure}"

  write_contract "$repo" "$slice" "$state" || return 1
  printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$repo/tasks/current-slice.scope" || return 1
  git -C "$repo" add tasks/current-slice.scope || return 1
}

commit_fixture_change() {
  local repo="$1"
  local path="$2"
  local text="$3"

  mkdir -p "$(dirname "$repo/$path")" || return 1
  printf '%s\n' "$text" >> "$repo/$path" || return 1
  git -C "$repo" add "$path" || return 1
  git -C "$repo" commit -q -m "fixture change $path" || return 1
}

prepare_g1b_chain() {
  local repo="$1"
  local test_path="${2:-internal/payload_test.go}"
  local payload_path="${3:-internal/payload.go}"
  local test_action="${4:-modify}"
  local slice="${5:-g1b-ledger-closure}"
  local shape="${6:-}"
  local state

  g1b_frozen="$(git -C "$repo" rev-parse HEAD)" || return 1
  if [[ "$test_action" == "delete" ]]; then
    git -C "$repo" rm -q "$test_path" || return 1
    git -C "$repo" commit -q -m 'delete red evidence' || return 1
  else
    commit_fixture_change "$repo" "$test_path" '# red evidence' || return 1
  fi
  g1b_tests="$(git -C "$repo" rev-parse HEAD)" || return 1
  for state in tests-red implemented reviewed; do
    [[ "$shape" != skipped || "$state" != implemented ]] || continue
    stage_g1b_contract "$repo" "$state" "$slice" || return 1
    if [[ "$shape" == contaminated && "$state" == implemented ]]; then
      printf '%s\n' '# contaminated state' >> "$repo/internal/payload.go" || return 1
      git -C "$repo" add internal/payload.go || return 1
    fi
    git -C "$repo" commit -q -m "scope $state" || return 1
    if [[ "$state" == "reviewed" ]]; then
      g1b_reviewed="$(git -C "$repo" rev-parse HEAD)" || return 1
    fi
  done
  if [[ "$shape" == empty ]]; then
    replace_contract_line "$repo" state= state=verified || return 1
    git -C "$repo" add tasks/current-slice.scope || return 1
    entry="$(git -C "$repo" ls-files -s -- tasks/current-slice.scope)" || return 1
    read -r scope_mode scope_blob _ <<< "$entry"
    manifest_file="$(mktemp "$tmp_root/empty-payload.XXXXXX")" || return 1
    printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest_file" || return 1
    g1b_digest="$(hash_file "$manifest_file")" || return 1
    git -C "$repo" commit -q -m 'verified scope' -m "Candidate-SHA256: $g1b_digest" || return 1
    g1b_verified="$(git -C "$repo" rev-parse HEAD)" || return 1
    git -C "$repo" commit -q --allow-empty -m 'empty payload' || return 1
    g1b_payload="$(git -C "$repo" rev-parse HEAD)" || return 1
    return
  fi
  mkdir -p "$(dirname "$repo/$payload_path")" || return 1
  printf '%s\n' '# payload candidate' >> "$repo/$payload_path" || return 1
  g1b_digest="$(prepare_verified_candidate "$repo" actual "$payload_path")" || return 1
  g1b_verified="$(git -C "$repo" rev-parse HEAD)" || return 1
  git -C "$repo" commit -q -m 'payload candidate' || return 1
  g1b_payload="$(git -C "$repo" rev-parse HEAD)" || return 1
}

append_g1b_row() {
  local repo="$1"
  local record_type="$2"
  local slice="$3"
  local frozen="$4"
  local tests_commit="$5"
  local verified="$6"
  local payload="$7"
  local digest="$8"
  local reason="$9"

  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$record_type" "$slice" "$frozen" "$tests_commit" "$verified" "$payload" "$digest" "$reason" \
    >> "$repo/tasks/slice-commit-ledger.tsv" || return 1
  git -C "$repo" add tasks/slice-commit-ledger.tsv || return 1
}

append_current_g1b_row() {
  append_g1b_row "$1" "${2:-normal-v1}" "${3:-g1b-ledger-closure}" \
    "$g1b_frozen" "$g1b_tests" "$g1b_verified" "$g1b_payload" "$g1b_digest" \
    "${4:--}"
}

new_genesis_guardrail_fixture() {
  local slice="$1"
  local ledger_exists="$2"
  local mutation="${3:-}"
  local repo

  repo="$(new_fixture "$slice" reviewed)" || return 1
  replace_contract_line "$repo" 'test=' 'test=tests/check-slice-scope_test.sh' || return 1
  printf '%s\n' 'allow=scripts/check-slice-scope.sh' 'allow=tasks/slice-commit-ledger.tsv' \
    >> "$repo/tasks/current-slice.scope" || return 1
  git -C "$repo" add tasks/current-slice.scope || return 1
  git -C "$repo" commit -q -m 'guardrail authority' || return 1
  if [[ "$ledger_exists" == "1" ]]; then
    write_canonical_genesis "$repo/tasks/slice-commit-ledger.tsv" || return 1
    git -C "$repo" add tasks/slice-commit-ledger.tsv || return 1
    git -C "$repo" commit -q -m 'existing ledger' || return 1
  fi
  printf '%s\n' '# checker candidate' >> "$repo/scripts/check-slice-scope.sh" || return 1
  printf '%s\n' '# explicit test candidate' >> "$repo/tests/check-slice-scope_test.sh" || return 1
  if [[ "$ledger_exists" == "1" ]]; then
    printf '%s\n' '# staged ledger change' >> "$repo/tasks/slice-commit-ledger.tsv" || return 1
  else
    write_canonical_genesis "$repo/tasks/slice-commit-ledger.tsv" || return 1
    case "$mutation" in
      header) sed '1s/record_type/record-kind/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      record-type) sed '2s/bootstrap-v1/bootstrap/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      slice-id) sed '2s/g0-plan-state-reset/g0-wrong/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      commit) sed '2s/1aa1c36b/0aa1c36b/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      digest) sed '5s/f0727c52/00727c52/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      reason) sed '2s/pre-g1-unverified/wrong-reason/' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      missing-row) sed '2d' "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" ;;
      extra-row) cp "$repo/tasks/slice-commit-ledger.tsv" "$repo/tasks/ledger.next" && printf '%s\n' 'bootstrap-v1\textra\t-\t-\t-\t-\t-\textra' >> "$repo/tasks/ledger.next" ;;
    esac
    if [[ -n "$mutation" && "$mutation" != "missing-test" && "$mutation" != "extra-control" ]]; then
      mv "$repo/tasks/ledger.next" "$repo/tasks/slice-commit-ledger.tsv" || return 1
    fi
  fi
  prepare_verified_candidate "$repo" actual scripts/check-slice-scope.sh tests/check-slice-scope_test.sh \
    tasks/slice-commit-ledger.tsv >/dev/null || return 1
  if [[ "$mutation" == "missing-test" ]]; then
    git -C "$repo" show HEAD:tests/check-slice-scope_test.sh > "$repo/tests/check-slice-scope_test.sh" || return 1
    git -C "$repo" add tests/check-slice-scope_test.sh || return 1
  elif [[ "$mutation" == "extra-control" ]]; then
    printf '%s\n' '# extra control' >> "$repo/Makefile" || return 1
    git -C "$repo" add Makefile || return 1
  fi
  printf '%s\n' "$repo"
}

candidate_must_be_verified() {
  local state="$1"
  local repo

  repo="$(new_fixture 'slice-one' "$state" 1)" || return 1
  stage_payload "$repo" || return 1
  assert_rejected_exactly \
    "production candidate at $state" \
    "slice-check: --candidate requires state=verified; found: $state" \
    "$repo" \
    '--candidate'
}

candidate_must_be_verified 'contract-frozen' || exit 1
candidate_must_be_verified 'implemented' || exit 1
candidate_must_be_verified 'reviewed' || exit 1
candidate_must_be_verified 'committed' || exit 1

repo="$(new_fixture 'slice-one' 'contract-frozen' 1)" || exit 1
stage_v1_contract "$repo" 'slice-one' 'verified' || exit 1
assert_rejected_exactly \
  'skipped contract-frozen to verified transition' \
  'slice-check: illegal state transition for slice-one: contract-frozen -> verified' \
  "$repo" \
  '--contract-candidate'

repo="$(new_fixture 'slice-one' 'verified' 1)" || exit 1
stage_v1_contract "$repo" 'slice-one' 'reviewed' || exit 1
assert_rejected_exactly \
  'backward verified to reviewed transition' \
  'slice-check: illegal state transition for slice-one: verified -> reviewed' \
  "$repo" \
  '--contract-candidate'

repo="$(new_fixture 'slice-one' 'verified' 1)" || exit 1
stage_v1_contract "$repo" 'slice-two' 'planned' || exit 1
assert_rejected_exactly \
  'new slice before previous slice committed' \
  'slice-check: cannot start slice-two before slice-one reaches committed; found: verified' \
  "$repo" \
  '--contract-candidate'

repo="$(new_fixture 'g1a-candidate-authority' 'committed')" || exit 1
stage_contract "$repo" 'g1b-ledger-closure' 'planned' || exit 1
assert_accepted 'G1A bootstrap starts G1B ledger closure' "$repo" '--contract-candidate'

repo="$(new_fixture 'g1a-candidate-authority' 'committed')" || exit 1
stage_contract "$repo" 'rk1-runner-kernel' 'planned' || exit 1
assert_rejected_exactly \
  'G1A bootstrap rejects other next slice' \
  'slice-check: G1A bootstrap permits only g1b-ledger-closure; found: rk1-runner-kernel' \
  "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
replace_contract_line "$repo" 'test=' 'test=tests/check-slice-scope_test.sh' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'control test authority' || exit 1
printf '%s\n' '# changed red test' > "$repo/tests/check-slice-scope_test.sh" || exit 1
git -C "$repo" add tests/check-slice-scope_test.sh || exit 1
assert_accepted 'explicit test control path' "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
remove_contract_lines "$repo" 'test=' || exit 1
assert_rejected_exactly \
  'missing explicit test path' \
  'slice-check: version 2 contract requires at least one explicit test path' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
printf '%s\n' 'test=internal/payload_test.go' >> "$repo/tasks/current-slice.scope" || exit 1
assert_rejected_exactly \
  'duplicate explicit test path' \
  'slice-check: duplicate test path: internal/payload_test.go' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
replace_contract_line "$repo" 'test=' 'test=tests/not-allowed.sh' || exit 1
assert_rejected_exactly \
  'non-allowlisted explicit test path' \
  'slice-check: test path must match an allowed path: tests/not-allowed.sh' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
replace_contract_line "$repo" 'test=' 'test=internal/payload.go' || exit 1
assert_rejected_exactly \
  'production path declared as test' \
  'slice-check: production path may not be declared as a test: internal/payload.go' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
assert_rejected_exactly \
  'empty test candidate' \
  'slice-check: test candidate contains no staged tests' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
replace_contract_line "$repo" 'test=' 'test=tests/check-slice-scope_test.sh' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'control test authority' || exit 1
printf '%s\n' 'package payload_test' '' 'func TestChanged() {}' > "$repo/internal/payload_test.go" || exit 1
git -C "$repo" add internal/payload_test.go || exit 1
assert_rejected_exactly \
  'test-looking undeclared path' \
  'slice-check: test candidate contains undeclared test path: internal/payload_test.go' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'verified')" || exit 1
printf '%s\n' 'fixture:' '' $'\t@true' > "$repo/Makefile" || exit 1
stage_payload "$repo" || exit 1
git -C "$repo" add Makefile || exit 1
assert_rejected_exactly \
  'mixed guardrail candidate' \
  'slice-check: guardrail candidate may contain only control-plane paths: internal/payload.go' \
  "$repo" '--guardrail-candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
stage_payload "$repo" || exit 1
git -C "$repo" reset -q internal/payload.go || exit 1
digest="$(prepare_verified_candidate "$repo" actual internal/payload.go)" || exit 1
index_file="$(mktemp "$tmp_root/intervening-index.XXXXXX")" || exit 1
rm -f -- "$index_file" || exit 1
GIT_INDEX_FILE="$index_file" git -C "$repo" read-tree HEAD || exit 1
GIT_INDEX_FILE="$index_file" git -C "$repo" commit -q --allow-empty \
  -m 'intervening authority' -m "Candidate-SHA256: $digest" || exit 1
assert_rejected_exactly \
  'intervening trailer commit' \
  'slice-check: verified candidate authority must be the reviewed-to-verified scope commit' \
  "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
stage_payload "$repo" || exit 1
git -C "$repo" reset -q internal/payload.go || exit 1
prepare_verified_candidate "$repo" parent internal/payload.go >/dev/null || exit 1
assert_rejected_exactly \
  'candidate digest scope blob drift' \
  'slice-check: staged candidate digest does not match verified scope trailer' \
  "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
printf '%s\n' 'internal/payload.go filter=canonical' > "$repo/.gitattributes" || exit 1
git -C "$repo" config filter.canonical.clean "sed 's/worktree/staged/g'" || exit 1
git -C "$repo" config filter.canonical.smudge cat || exit 1
git -C "$repo" add .gitattributes internal/payload.go || exit 1
git -C "$repo" commit -q -m 'filter baseline' || exit 1
printf '%s\n' 'worktree' > "$repo/internal/payload.go" || exit 1
prepare_verified_candidate "$repo" actual internal/payload.go >/dev/null || exit 1
git -C "$repo" config filter.canonical.clean "sed 's/worktree/restaged/g'" || exit 1
git -C "$repo" update-index --assume-unchanged internal/payload.go || exit 1
assert_accepted 'candidate reads real staged index blob' "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
printf '%s\n' 'old' > "$repo/internal/old.txt" || exit 1
printf '%s\n' 'allow=internal/old.txt' 'allow=internal/new.txt' >> "$repo/tasks/current-slice.scope" || exit 1
git -C "$repo" add internal/old.txt tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'rename baseline' || exit 1
mv "$repo/internal/old.txt" "$repo/internal/new.txt" || exit 1
git -C "$repo" config diff.renames true || exit 1
prepare_verified_candidate "$repo" actual internal/old.txt internal/new.txt >/dev/null || exit 1
git -C "$repo" config diff.renames false || exit 1
assert_accepted 'candidate manifest ignores rename configuration' "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
write_g1a_migration_contract "$repo" 1 tests-red || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'G1A v1 baseline' || exit 1
write_g1a_migration_contract "$repo" 2 implemented 'test=tests/check-slice-scope_test.sh' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
assert_accepted 'exact G1A v1 to v2 migration' "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
write_g1a_migration_contract "$repo" 1 tests-red || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'G1A v1 baseline' || exit 1
write_g1a_migration_contract "$repo" 2 implemented 'test=internal/payload_test.go' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
assert_rejected_exactly \
  'G1A migration alternate test' \
  'slice-check: G1A migration requires exactly test=tests/check-slice-scope_test.sh' \
  "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
write_g1a_migration_contract "$repo" 1 tests-red || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
git -C "$repo" commit -q -m 'G1A v1 baseline' || exit 1
write_g1a_migration_contract "$repo" 2 implemented \
  'test=tests/check-slice-scope_test.sh' 'test=internal/payload_test.go' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
assert_rejected_exactly \
  'G1A migration additional test' \
  'slice-check: G1A migration requires exactly test=tests/check-slice-scope_test.sh' \
  "$repo" '--contract-candidate'

for mutation in test allow budget; do
  repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
  replace_contract_line "$repo" 'state=' 'state=tests-red' || exit 1
  case "$mutation" in
    test) replace_contract_line "$repo" 'test=' 'test=tests/check-slice-scope_test.sh' || exit 1 ;;
    allow) printf '%s\n' 'allow=tests/extra.sh' >> "$repo/tasks/current-slice.scope" || exit 1 ;;
    budget) replace_contract_line "$repo" 'max_changed_lines=' 'max_changed_lines=81' || exit 1 ;;
  esac
  git -C "$repo" add tasks/current-slice.scope || exit 1
  assert_rejected_exactly \
    "frozen $mutation mutation" \
    'slice-check: frozen contract fields changed after contract-frozen' \
    "$repo" '--contract-candidate'
done

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
replace_contract_line "$repo" 'state=' 'state=verified' || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
entry="$(git -C "$repo" ls-files -s -- tasks/current-slice.scope)" || exit 1
read -r scope_mode scope_blob _ <<< "$entry"
manifest_file="$(mktemp "$tmp_root/empty-manifest.XXXXXX")" || exit 1
printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest_file" || exit 1
digest="$(hash_file "$manifest_file")" || exit 1
git -C "$repo" commit -q -m 'verified empty scope' -m "Candidate-SHA256: $digest" || exit 1
assert_rejected_exactly \
  'empty normal candidate' \
  'slice-check: candidate contains no staged payload' \
  "$repo" '--candidate'

repo="$(new_fixture 'g1a-candidate-authority' 'committed')" || exit 1
stage_v1_contract "$repo" 'g1b-ledger-closure' 'planned' || exit 1
assert_rejected_exactly \
  'G1B successor requires schema v2' \
  'slice-check: new slice contracts require version=2; found: 1' \
  "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
replace_contract_line "$repo" 'test=' 'test=Makefile' || exit 1
assert_rejected_exactly \
  'control-plane Makefile declared as test' \
  'slice-check: control-plane path may not be declared as a test: Makefile' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'g1b-ledger-closure' 'planned')" || exit 1
stage_v1_contract "$repo" 'g1b-ledger-closure' 'contract-frozen' || exit 1
assert_rejected_exactly \
  'version 2 contract cannot downgrade' \
  'slice-check: version 2 contract may not downgrade; found: 1' \
  "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
stage_g1b_contract "$repo" 'tests-red' || exit 1
assert_rejected_exactly 'tests-red without test predecessor' \
  'slice-check: tests-red transition requires a nonempty immediately prior test commit' \
  "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
commit_fixture_change "$repo" internal/payload.go '# not a test' || exit 1
stage_g1b_contract "$repo" 'tests-red' || exit 1
assert_rejected_exactly 'tests-red predecessor contains production' \
  'slice-check: tests commit contains non-test path: internal/payload.go' \
  "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
commit_fixture_change "$repo" internal/payload_test.go '# red evidence' || exit 1
git -C "$repo" commit -q --allow-empty -m 'intervening commit' || exit 1
stage_g1b_contract "$repo" 'tests-red' || exit 1
assert_rejected_exactly 'tests-red predecessor has wrong ancestry' \
  'slice-check: tests-red transition requires a nonempty immediately prior test commit' \
  "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
commit_fixture_change "$repo" internal/payload_test.go '# red evidence' || exit 1
stage_g1b_contract "$repo" 'tests-red' || exit 1
assert_accepted 'tests-red accepts exact test-only predecessor' "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
stage_g1b_contract "$repo" 'committed' || exit 1
assert_rejected_exactly 'committed without ledger predecessor' \
  'slice-check: committed transition requires an immediately prior ledger-only commit' \
  "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
head_commit="$(git -C "$repo" rev-parse HEAD)" || exit 1
append_g1b_row "$repo" bootstrap-v1 slice-one "$head_commit" "$head_commit" "$head_commit" "$head_commit" \
  '0000000000000000000000000000000000000000000000000000000000000000' isolation || exit 1
stage_payload "$repo" || exit 1
assert_rejected_exactly 'ledger candidate is isolated' \
  'slice-check: ledger candidate may contain only tasks/slice-commit-ledger.tsv: internal/payload.go' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
head_commit="$(git -C "$repo" rev-parse HEAD)" || exit 1
append_g1b_row "$repo" bootstrap-v1 historical-slice "$head_commit" "$head_commit" "$head_commit" "$head_commit" \
  '0000000000000000000000000000000000000000000000000000000000000000' historical || exit 1
git -C "$repo" commit -q -m 'historical ledger row' || exit 1
printf '%s\n' "$ledger_header" > "$repo/tasks/slice-commit-ledger.tsv" || exit 1
git -C "$repo" add tasks/slice-commit-ledger.tsv || exit 1
assert_rejected_exactly 'ledger history removal' \
  'slice-check: ledger history is append-only' "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
head_commit="$(git -C "$repo" rev-parse HEAD)" || exit 1
for slice in slice-one slice-two; do
  append_g1b_row "$repo" bootstrap-v1 "$slice" "$head_commit" "$head_commit" "$head_commit" "$head_commit" \
    '0000000000000000000000000000000000000000000000000000000000000000' two-rows || exit 1
done
assert_rejected_exactly 'ledger appends two rows' \
  'slice-check: ledger candidate must append exactly one row' "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
printf '%s\n' 'record_type slice_id wrong' > "$repo/tasks/slice-commit-ledger.tsv" || exit 1
git -C "$repo" add tasks/slice-commit-ledger.tsv || exit 1
git -C "$repo" commit -q -m 'malformed ledger baseline' || exit 1
head_commit="$(git -C "$repo" rev-parse HEAD)" || exit 1
append_g1b_row "$repo" bootstrap-v1 slice-one "$head_commit" "$head_commit" "$head_commit" "$head_commit" \
  '0000000000000000000000000000000000000000000000000000000000000000' bad-header || exit 1
assert_rejected_exactly 'ledger canonical header' \
  "slice-check: ledger header must be: $ledger_header" "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'verified')" || exit 1
printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' bootstrap-v1 slice-one a b c d e \
  >> "$repo/tasks/slice-commit-ledger.tsv" || exit 1
git -C "$repo" add tasks/slice-commit-ledger.tsv || exit 1
assert_rejected_exactly 'ledger row missing evidence' \
  'slice-check: ledger row must contain exactly 8 nonempty tab-separated fields' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_g1b_row "$repo" normal-v1 g1b-ledger-closure "$g1b_frozen" "$g1b_tests" "$g1b_verified" \
  "$g1b_payload" not-a-digest - || exit 1
assert_rejected_exactly 'ledger malformed digest' \
  'slice-check: ledger candidate_sha256 must be 64 lowercase hexadecimal characters' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_g1b_row "$repo" normal-v1 g1b-ledger-closure "$g1b_frozen" "$g1b_tests" "$g1b_verified" \
  "$g1b_payload" '0000000000000000000000000000000000000000000000000000000000000000' \
  - || exit 1
assert_rejected_exactly 'ledger digest differs from verified authority' \
  'slice-check: ledger candidate digest does not match verified authority' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'planned')" || exit 1
wrong_frozen="$(git -C "$repo" rev-parse HEAD)" || exit 1
stage_g1b_contract "$repo" 'contract-frozen' || exit 1
git -C "$repo" commit -q -m 'scope contract-frozen' || exit 1
prepare_g1b_chain "$repo" || exit 1
append_g1b_row "$repo" normal-v1 g1b-ledger-closure "$wrong_frozen" "$g1b_tests" "$g1b_verified" \
  "$g1b_payload" "$g1b_digest" - || exit 1
assert_rejected_exactly 'ledger frozen evidence has wrong scope state' \
  'slice-check: ledger contract_frozen_commit does not record contract-frozen state' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_g1b_row "$repo" normal-v1 g1b-ledger-closure "$g1b_frozen" "$g1b_tests" "$g1b_reviewed" \
  "$g1b_payload" "$g1b_digest" - || exit 1
assert_rejected_exactly 'ledger verified evidence has wrong scope state' \
  'slice-check: ledger verified_scope_commit does not record verified state' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_current_g1b_row "$repo" normal-v1 g1b-ledger-closure wrong || exit 1
assert_rejected_exactly 'normal ledger reason is exact' \
  'slice-check: normal-v1 ledger reason must be -' \
  "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
missing_commit='ffffffffffffffffffffffffffffffffffffffff'
append_g1b_row "$repo" normal-v1 g1b-ledger-closure "$g1b_frozen" "$missing_commit" "$g1b_verified" \
  "$g1b_payload" "$g1b_digest" - || exit 1
assert_rejected_exactly 'ledger missing evidence commit' \
  "slice-check: ledger evidence commit is unavailable: $missing_commit" "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
head_commit="$(git -C "$repo" rev-parse HEAD)" || exit 1
append_g1b_row "$repo" bootstrap-v1 g1b-ledger-closure "$head_commit" "$head_commit" "$head_commit" "$head_commit" \
  '0000000000000000000000000000000000000000000000000000000000000000' historical || exit 1
git -C "$repo" commit -q -m 'historical ledger row' || exit 1
prepare_g1b_chain "$repo" || exit 1
append_current_g1b_row "$repo" || exit 1
assert_rejected_exactly 'ledger duplicate historical slice' \
  'slice-check: duplicate ledger slice_id: g1b-ledger-closure' "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" internal/payload_test.go docs/outside.md || exit 1
append_current_g1b_row "$repo" || exit 1
assert_rejected_exactly 'ledger payload escapes allowlist' \
  'slice-check: ledger payload commit contains path outside frozen allowlist: docs/outside.md' \
  "$repo" '--ledger-candidate'

skipped_chain_repo='' contaminated_chain_repo='' empty_chain_repo=''
for shape in skipped contaminated empty; do
  repo="$(new_g1c_fixture contract-frozen)" || exit 1
  prepare_g1b_chain "$repo" internal/payload_test.go internal/payload.go modify g1c-guardrail-adoption "$shape" || exit 1
  append_g1b_row "$repo" normal-v1 g1c-guardrail-adoption "$g1b_frozen" "$g1b_tests" \
    "$g1b_verified" "$g1b_payload" "$g1b_digest" - || exit 1
  case "$shape" in skipped) skipped_chain_repo="$repo" ;; contaminated) contaminated_chain_repo="$repo" ;; empty) empty_chain_repo="$repo" ;; esac
done
cp -R "$empty_chain_repo" "$tmp_root/committed-invalid" || exit 1
git -C "$tmp_root/committed-invalid" commit -q -m 'invalid ledger row' || exit 1
stage_g1b_contract "$tmp_root/committed-invalid" committed g1c-guardrail-adoption || exit 1
assert_rejection_matrix 'normal ledger chain rejects skipped, contaminated, and empty evidence' \
  "$skipped_chain_repo" --ledger-candidate 'slice-check: ledger evidence state sequence is incomplete: implemented' \
  "$contaminated_chain_repo" --ledger-candidate 'slice-check: ledger implemented commit contains non-scope path: internal/payload.go' \
  "$empty_chain_repo" --ledger-candidate 'slice-check: ledger payload_commit contains no payload path' \
  "$tmp_root/committed-invalid" --contract-candidate 'slice-check: ledger payload_commit contains no payload path'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_current_g1b_row "$repo" bootstrap-v1 g1b-ledger-closure bootstrap || exit 1
assert_rejected_exactly 'bootstrap ledger rows are reserved' \
  'slice-check: bootstrap-v1 is reserved for pre-G1C closure' "$repo" '--ledger-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_current_g1b_row "$repo" || exit 1
assert_accepted 'exact normal ledger candidate' "$repo" '--ledger-candidate'

git -C "$repo" commit -q -m 'normal ledger closure' || exit 1
stage_g1b_contract "$repo" 'committed' || exit 1
assert_accepted 'committed accepts exact normal closing predecessor' "$repo" '--contract-candidate'

repo="$(new_g1b_fixture 'contract-frozen')" || exit 1
prepare_g1b_chain "$repo" || exit 1
append_current_g1b_row "$repo" normal-v1 other-slice - || exit 1
git -C "$repo" commit -q -m 'wrong ledger closure' || exit 1
stage_g1b_contract "$repo" 'committed' || exit 1
assert_rejected_exactly 'committed last row closes other slice' \
  'slice-check: last ledger row does not close active slice: other-slice' \
  "$repo" '--contract-candidate'

repo="$(new_fixture 'g1b-ledger-closure' 'committed')" || exit 1
stage_contract "$repo" 'g1c-guardrail-adoption' 'planned' || exit 1
assert_accepted 'G1B phase lock starts G1C' "$repo" '--contract-candidate'

repo="$(new_fixture 'g1b-ledger-closure' 'committed')" || exit 1
stage_contract "$repo" 'rk1-runner-kernel' 'planned' || exit 1
assert_rejected_exactly 'G1B phase lock rejects other successor' \
  'slice-check: G1B bootstrap permits only g1c-guardrail-adoption; found: rk1-runner-kernel' \
  "$repo" '--contract-candidate'

bootstrap_repo="$(new_genesis_guardrail_fixture g1b-ledger-closure 0)" || exit 1
other_repo="$(new_genesis_guardrail_fixture other-slice 0)" || exit 1
existing_repo="$(new_genesis_guardrail_fixture g1b-ledger-closure 1)" || exit 1
missing_repo="$(new_genesis_guardrail_fixture g1b-ledger-closure 0 missing-test)" || exit 1
extra_repo="$(new_genesis_guardrail_fixture g1b-ledger-closure 0 extra-control)" || exit 1
assert_g1b_genesis_guardrail "$bootstrap_repo" "$other_repo" "$existing_repo" "$missing_repo" "$extra_repo"
assert_canonical_genesis_mutations

staged_delete_repo="$(new_fixture slice-one contract-frozen)" || exit 1
git -C "$staged_delete_repo" rm -q internal/payload_test.go || exit 1
red_delete_repo="$(new_g1b_fixture contract-frozen)" || exit 1
git -C "$red_delete_repo" rm -q internal/payload_test.go || exit 1
git -C "$red_delete_repo" commit -q -m 'delete red test' || exit 1
stage_g1b_contract "$red_delete_repo" tests-red || exit 1
ledger_delete_repo="$(new_g1b_fixture contract-frozen)" || exit 1
prepare_g1b_chain "$ledger_delete_repo" internal/payload_test.go internal/payload.go delete || exit 1
append_current_g1b_row "$ledger_delete_repo" || exit 1
assert_test_deletion_rejections "$staged_delete_repo" "$red_delete_repo" "$ledger_delete_repo"

digest_repo="$(new_fixture slice-one reviewed)" || exit 1
printf '%s\n' 'allow=internal/new.txt' 'ignore=tasks/pi-agent-integration-spec.md' >> "$digest_repo/tasks/current-slice.scope" || exit 1
git -C "$digest_repo" add tasks/current-slice.scope || exit 1
git -C "$digest_repo" commit -q -m 'digest authority' || exit 1
printf '%s\n' user-owned > "$digest_repo/tasks/pi-agent-integration-spec.md" || exit 1
stage_payload "$digest_repo" || exit 1
git -C "$digest_repo" reset -q internal/payload.go || exit 1
printf '%s\n' untracked > "$digest_repo/internal/new.txt" || exit 1
replace_contract_line "$digest_repo" state= state=verified || exit 1
git -C "$digest_repo" add tasks/current-slice.scope || exit 1
digest="$(expected_worktree_digest "$digest_repo" internal/new.txt internal/payload.go)" || exit 1
assert_digest_equivalence "$digest_repo" "$digest" --candidate \
  'candidate digest is exact, index-neutral, and equivalent' internal/payload.go internal/new.txt

delete_digest_repo="$(new_fixture slice-one reviewed)" || exit 1
rm "$delete_digest_repo/internal/payload.go" || exit 1
replace_contract_line "$delete_digest_repo" state= state=verified || exit 1
git -C "$delete_digest_repo" add tasks/current-slice.scope || exit 1
delete_digest="$(expected_worktree_digest "$delete_digest_repo" internal/payload.go)" || exit 1
assert_digest_equivalence "$delete_digest_repo" "$delete_digest" --candidate \
  'tracked deletion digest is equivalent to staged candidate' internal/payload.go

staged_repo="$(new_digest_rejection_fixture staged-payload)" || exit 1
unstaged_scope_repo="$(new_digest_rejection_fixture unstaged-scope)" || exit 1
empty_repo="$(new_digest_rejection_fixture empty)" || exit 1
nonreviewed_repo="$(new_digest_rejection_fixture nonreviewed)" || exit 1
frozen_repo="$(new_digest_rejection_fixture frozen)" || exit 1
outside_repo="$(new_digest_rejection_fixture outside)" || exit 1
symlink_repo="$(new_digest_rejection_fixture symlink)" || exit 1
fifo_repo="$(new_digest_rejection_fixture fifo)" || exit 1
mixed_xy_repo="$(new_digest_rejection_fixture mixed-xy)" || exit 1
mixed_control_repo="$(new_digest_rejection_fixture mixed-control-product)" || exit 1
ledger_digest_repo="$(new_digest_rejection_fixture ledger)" || exit 1
ignored_digest_repo="$(new_digest_rejection_fixture staged-ignored)" || exit 1
line_budget_repo="$(new_digest_rejection_fixture over-lines)" || exit 1
file_budget_repo="$(new_digest_rejection_fixture over-files)" || exit 1
assert_rejection_matrix 'candidate digest rejects every unsafe shape' \
  "$staged_repo" --candidate-digest 'slice-check: candidate digest requires payload to remain unstaged: internal/payload.go' \
  "$unstaged_scope_repo" --candidate-digest 'slice-check: candidate digest requires exactly staged verified scope' \
  "$empty_repo" --candidate-digest 'slice-check: candidate digest contains no worktree payload' \
  "$nonreviewed_repo" --candidate-digest 'slice-check: --candidate-digest requires reviewed -> verified scope transition' \
  "$frozen_repo" --candidate-digest 'slice-check: frozen contract fields changed after reviewed' \
  "$outside_repo" --candidate-digest 'slice-check: path outside frozen allowlist: docs/outside.md' \
  "$symlink_repo" --candidate-digest 'slice-check: candidate digest rejects symlink payload: internal/payload.go' \
  "$fifo_repo" --candidate-digest 'slice-check: candidate digest rejects unsupported payload type: internal/payload.go' \
  "$mixed_xy_repo" --candidate-digest 'slice-check: mixed staged/unstaged payload is forbidden: internal/payload.go' \
  "$mixed_control_repo" --candidate-digest 'slice-check: candidate digest may not mix control-plane and production paths' \
  "$ledger_digest_repo" --candidate-digest 'slice-check: tasks/slice-commit-ledger.tsv requires --ledger-candidate' \
  "$ignored_digest_repo" --candidate-digest 'slice-check: ignored/user-owned path must never be staged: tasks/pi-agent-integration-spec.md' \
  "$line_budget_repo" --candidate-digest 'slice-check: 92 changed lines exceed ceiling 80' \
  "$file_budget_repo" --candidate-digest 'slice-check: 5 files exceed ceiling 4'

assert_make_candidate_targets

guardrail_repo="$(new_fixture g1c-guardrail-adoption reviewed)" || exit 1
replace_contract_line "$guardrail_repo" test= test=tests/check-slice-scope_test.sh || exit 1
replace_contract_line "$guardrail_repo" max_total_files= max_total_files=5 || exit 1
printf '%s\n' 'allow=scripts/check-slice-scope.sh' 'allow=tasks/todo.md' 'allow=tasks/workflow-guardrails.md' \
  >> "$guardrail_repo/tasks/current-slice.scope" || exit 1
printf '%s\n' todo > "$guardrail_repo/tasks/todo.md" || exit 1
printf '%s\n' workflow > "$guardrail_repo/tasks/workflow-guardrails.md" || exit 1
git -C "$guardrail_repo" add tasks/current-slice.scope tasks/todo.md tasks/workflow-guardrails.md || exit 1
git -C "$guardrail_repo" commit -q -m 'five-file guardrail baseline' || exit 1
for path in Makefile scripts/check-slice-scope.sh tests/check-slice-scope_test.sh tasks/todo.md tasks/workflow-guardrails.md; do
  printf '%s\n' '# guardrail payload' >> "$guardrail_repo/$path" || exit 1
done
replace_contract_line "$guardrail_repo" state= state=verified || exit 1
git -C "$guardrail_repo" add tasks/current-slice.scope || exit 1
guardrail_digest="$(expected_worktree_digest "$guardrail_repo" Makefile scripts/check-slice-scope.sh tests/check-slice-scope_test.sh tasks/todo.md tasks/workflow-guardrails.md)" || exit 1
assert_digest_equivalence "$guardrail_repo" "$guardrail_digest" --guardrail-candidate \
  'control digest is deterministic and authorizes exact five-file guardrail payload' Makefile scripts/check-slice-scope.sh tests/check-slice-scope_test.sh tasks/todo.md tasks/workflow-guardrails.md

reserved_repo="$(new_fixture slice-one reviewed)" || exit 1
printf '%s\n' 'allow=tasks/slice-commit-ledger.tsv' >> "$reserved_repo/tasks/current-slice.scope" || exit 1
printf '%s\n' "$ledger_header" > "$reserved_repo/tasks/slice-commit-ledger.tsv" || exit 1
git -C "$reserved_repo" add tasks/current-slice.scope tasks/slice-commit-ledger.tsv && git -C "$reserved_repo" commit -q -m 'ledger reservation authority' || exit 1
cp -R "$reserved_repo" "$tmp_root/test-reservation" || exit 1
replace_contract_line "$tmp_root/test-reservation" state= state=contract-frozen || exit 1
git -C "$tmp_root/test-reservation" add tasks/current-slice.scope && git -C "$tmp_root/test-reservation" commit -q -m 'test reservation authority' || exit 1
printf '%s\n' reserved >> "$reserved_repo/tasks/slice-commit-ledger.tsv" || exit 1
prepare_verified_candidate "$reserved_repo" actual tasks/slice-commit-ledger.tsv >/dev/null || exit 1
cp -R "$reserved_repo" "$tmp_root/contract-reservation" || exit 1
printf '%s\n' reserved >> "$tmp_root/test-reservation/tasks/slice-commit-ledger.tsv" && git -C "$tmp_root/test-reservation" add tasks/slice-commit-ledger.tsv || exit 1
replace_contract_line "$tmp_root/contract-reservation" state= state=committed && git -C "$tmp_root/contract-reservation" add tasks/current-slice.scope || exit 1
assert_rejection_matrix 'ledger path is reserved in every non-ledger candidate mode' \
  "$reserved_repo" --candidate 'slice-check: tasks/slice-commit-ledger.tsv requires --ledger-candidate' \
  "$reserved_repo" --guardrail-candidate 'slice-check: tasks/slice-commit-ledger.tsv requires --ledger-candidate' \
  "$tmp_root/test-reservation" --test-candidate 'slice-check: tasks/slice-commit-ledger.tsv requires --ledger-candidate' \
  "$tmp_root/contract-reservation" --contract-candidate 'slice-check: tasks/slice-commit-ledger.tsv requires --ledger-candidate'

normal_repo="$(new_g1c_fixture contract-frozen)" || exit 1
prepare_g1b_chain "$normal_repo" internal/payload_test.go internal/payload.go modify g1c-guardrail-adoption || exit 1
append_g1b_row "$normal_repo" normal-v1 g1c-guardrail-adoption "$g1b_frozen" "$g1b_tests" \
  "$g1b_verified" "$g1b_payload" "$g1b_digest" - || exit 1
assert_normal_closure_successor "$normal_repo"

missing_reservation_repo="$(new_fixture g1c-guardrail-adoption committed)" || exit 1
stage_contract "$missing_reservation_repo" rk1-runner-kernel planned || exit 1
assert_rejected_exactly 'successor requires committed ledger reservation' \
  'slice-check: cannot start rk1-runner-kernel without committed ledger closure for g1c-guardrail-adoption' \
  "$missing_reservation_repo" '--contract-candidate'

duplicate_plan_repo="$(new_planning_fixture rk1-runner-kernel)" || exit 1
stage_contract "$duplicate_plan_repo" rk1-runner-kernel planned || exit 1
assert_rejected_exactly 'duplicate ledger slice ID rejected at planning' \
  'slice-check: slice_id already exists in ledger: rk1-runner-kernel' "$duplicate_plan_repo" '--contract-candidate'

assert_rg0_roadmap_truth

if (( test_count != 69 )); then
  printf 'test harness error: expected 69 assertions, ran %d\n' "$test_count" >&2
  exit 1
fi

if (( failures != 0 )); then
  printf '%d of %d slice-scope policy assertions failed\n' "$failures" "$test_count"
  exit 1
fi

printf 'all %d slice-scope policy assertions passed\n' "$test_count"
