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
  local repo

  repo="$(mktemp -d "$tmp_root/fixture.XXXXXX")" || return 1
  [[ -n "$repo" && -d "$repo" ]] || return 1
  mkdir -p "$repo/internal" "$repo/scripts" "$repo/tasks" || return 1
  cp "$checker_source" "$repo/scripts/check-slice-scope.sh" || return 1
  chmod +x "$repo/scripts/check-slice-scope.sh" || return 1
  write_contract "$repo" "$slice_id" "$state" || return 1
  printf '%s\n' 'package payload' > "$repo/internal/payload.go" || return 1

  (
    set -e
    cd "$repo"
    git init -q
    git config user.name 'Slice Scope Test'
    git config user.email 'slice-scope@example.invalid'
    git add scripts/check-slice-scope.sh tasks/current-slice.scope internal/payload.go
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

candidate_must_be_verified() {
  local state="$1"
  local repo

  repo="$(new_fixture 'slice-one' "$state")" || return 1
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

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
stage_contract "$repo" 'slice-one' 'verified' || exit 1
assert_rejected_exactly \
  'skipped contract-frozen to verified transition' \
  'slice-check: illegal state transition for slice-one: contract-frozen -> verified' \
  "$repo" \
  '--contract-candidate'

repo="$(new_fixture 'slice-one' 'verified')" || exit 1
stage_contract "$repo" 'slice-one' 'reviewed' || exit 1
assert_rejected_exactly \
  'backward verified to reviewed transition' \
  'slice-check: illegal state transition for slice-one: verified -> reviewed' \
  "$repo" \
  '--contract-candidate'

repo="$(new_fixture 'slice-one' 'verified')" || exit 1
stage_contract "$repo" 'slice-two' 'planned' || exit 1
assert_rejected_exactly \
  'new slice before previous slice committed' \
  'slice-check: cannot start slice-two before slice-one reaches committed; found: verified' \
  "$repo" \
  '--contract-candidate'

if (( test_count != 7 )); then
  printf 'test harness error: expected 7 assertions, ran %d\n' "$test_count" >&2
  exit 1
fi

if (( failures != 0 )); then
  printf '%d of %d slice-scope policy assertions failed\n' "$failures" "$test_count"
  exit 1
fi

printf 'all %d slice-scope policy assertions passed\n' "$test_count"
