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
    'max_total_files=3' \
    'max_production_files=1' \
    'max_changed_lines=40' \
    'ignore=notes.local' \
    'allow=internal/payload.go' \
    'allow=internal/production.go' \
    'test=internal/payload.go' \
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
  printf '%s\n' 'package production' > "$repo/internal/production.go" || return 1
  printf '%s\n' \
    $'version\tslice_id\tcontract_commit\tpayload_commit\tcandidate_sha256\treason' \
    $'bootstrap-v1\tg0-plan-state-reset\t1aa1c36b3a21d9bc9e31362b21e536e57187e711\tee1b5847ad026dcdfb4871d41064e2b9742cedfd\t-\tpre-g1-unverified' \
    > "$repo/tasks/slice-commit-ledger.tsv" || return 1

  (
    set -e
    cd "$repo"
    git init -q
    git config user.name 'Slice Scope Test'
    git config user.email 'slice-scope@example.invalid'
    git add scripts/check-slice-scope.sh tasks/current-slice.scope tasks/slice-commit-ledger.tsv internal/payload.go internal/production.go
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
    printf '  actual: %s\n' "$output"
    failures=$((failures + 1))
    return
  fi
  printf 'ok %d - %s\n' "$test_count" "$label"
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

fixture_digest() {
  local repo="$1"
  shift
  local index_file manifest_file
  index_file="$(mktemp "$tmp_root/index.XXXXXX")" || return 1
  manifest_file="$(mktemp "$tmp_root/manifest.XXXXXX")" || return 1
  rm -f -- "$index_file" || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" read-tree HEAD || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" add -A -- "$@" || return 1
  GIT_INDEX_FILE="$index_file" git -C "$repo" diff --cached --raw --no-abbrev -z HEAD -- "$@" > "$manifest_file" || return 1
  hash_file "$manifest_file"
}

prepare_verified_payload() {
  local repo="$1"
  local path="${2:-internal/payload.go}"
  local digest

  printf '%s\n' 'package payload' '' 'const changed = true' > "$repo/$path" || return 1
  digest="$(fixture_digest "$repo" "$path")" || return 1
  write_contract "$repo" 'slice-one' 'verified' || return 1
  if [[ "$path" == "Makefile" ]]; then
    printf '%s\n' 'allow=Makefile' >> "$repo/tasks/current-slice.scope" || return 1
  fi
  git -C "$repo" add tasks/current-slice.scope || return 1
  git -C "$repo" commit -q -m 'verified scope' -m "Candidate-SHA256: $digest" || return 1
  git -C "$repo" add "$path" || return 1
  printf '%s\n' "$digest"
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

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
prepare_verified_payload "$repo" >/dev/null || exit 1
assert_accepted 'verified normal candidate' "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
stage_payload "$repo" || exit 1
assert_accepted 'declared red-test candidate' "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
printf '%s\n' 'package production' '' 'const changed = true' > "$repo/internal/production.go" || exit 1
git -C "$repo" add internal/production.go || exit 1
assert_rejected_exactly \
  'undeclared test path' \
  'slice-check: test candidate contains undeclared test path: internal/production.go' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
prepare_verified_payload "$repo" >/dev/null || exit 1
printf '%s\n' '// shadow' >> "$repo/internal/payload.go" || exit 1
assert_rejected_exactly \
  'staged payload shadow' \
  'slice-check: mixed staged/unstaged payload is forbidden: internal/payload.go' \
  "$repo" '--candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
printf '%s\n' 'private' > "$repo/notes.local" || exit 1
git -C "$repo" add notes.local || exit 1
assert_rejected_exactly \
  'ignored path staged' \
  'slice-check: ignored/user-owned path must never be staged: notes.local' \
  "$repo" '--test-candidate'

repo="$(new_fixture 'slice-one' 'planned')" || exit 1
stage_contract "$repo" 'slice-one' 'contract-frozen' || exit 1
assert_accepted 'adjacent planned to contract-frozen transition' "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'contract-frozen')" || exit 1
write_contract "$repo" 'slice-one' 'tests-red' || exit 1
awk '{sub(/^max_changed_lines=40$/, "max_changed_lines=41"); print}' \
  "$repo/tasks/current-slice.scope" > "$repo/tasks/scope.next" || exit 1
mv "$repo/tasks/scope.next" "$repo/tasks/current-slice.scope" || exit 1
git -C "$repo" add tasks/current-slice.scope || exit 1
assert_rejected_exactly \
  'frozen budget mutation' \
  'slice-check: frozen contract fields changed after contract-frozen' \
  "$repo" '--contract-candidate'

repo="$(new_fixture 'slice-one' 'reviewed')" || exit 1
prepare_verified_payload "$repo" 'Makefile' >/dev/null || exit 1
assert_rejected_exactly \
  'control path in normal candidate' \
  'slice-check: control-plane path requires --guardrail-candidate: Makefile' \
  "$repo" '--candidate'
assert_accepted 'verified guardrail candidate' "$repo" '--guardrail-candidate'

prepare_ledger_candidate() {
  local variant="$1"
  local repo digest contract_commit payload_commit

  repo="$(new_fixture 'slice-one' 'reviewed')" || return 1
  digest="$(prepare_verified_payload "$repo")" || return 1
  git -C "$repo" commit -q -m 'payload' || return 1
  payload_commit="$(git -C "$repo" rev-parse HEAD)" || return 1
  contract_commit="$(git -C "$repo" rev-parse HEAD^)" || return 1
  case "$variant" in
    ancestry) contract_commit="$(git -C "$repo" rev-parse HEAD^^)" || return 1 ;;
    digest) digest='0000000000000000000000000000000000000000000000000000000000000000' ;;
    rewrite)
      awk '{sub(/pre-g1-unverified$/, "rewritten"); print}' \
        "$repo/tasks/slice-commit-ledger.tsv" > "$repo/tasks/ledger.next" || return 1
      mv "$repo/tasks/ledger.next" "$repo/tasks/slice-commit-ledger.tsv" || return 1
      ;;
  esac
  printf 'v1\tslice-one\t%s\t%s\t%s\t-\n' \
    "$contract_commit" "$payload_commit" "$digest" >> "$repo/tasks/slice-commit-ledger.tsv" || return 1
  git -C "$repo" add tasks/slice-commit-ledger.tsv || return 1
  printf '%s\n' "$repo"
}

repo="$(prepare_ledger_candidate pass)" || exit 1
assert_accepted 'append-only ledger closure' "$repo" '--ledger-candidate'

repo="$(prepare_ledger_candidate rewrite)" || exit 1
assert_rejected_exactly 'ledger rewrite' 'slice-check: closure ledger is append-only' "$repo" '--ledger-candidate'

repo="$(new_fixture 'slice-one' 'verified')" || exit 1
git -C "$repo" rm -q tasks/slice-commit-ledger.tsv || exit 1
assert_rejected_exactly 'ledger removal' 'slice-check: ledger removal is forbidden' "$repo" '--ledger-candidate'

repo="$(prepare_ledger_candidate ancestry)" || exit 1
assert_rejected_exactly \
  'ledger bad ancestry' \
  'slice-check: ledger contract is not the payload parent' \
  "$repo" '--ledger-candidate'

repo="$(prepare_ledger_candidate digest)" || exit 1
assert_rejected_exactly \
  'ledger bad digest' \
  'slice-check: ledger candidate digest does not match the verified payload' \
  "$repo" '--ledger-candidate'

if (( test_count != 21 )); then
  printf 'test harness error: expected 21 assertions, ran %d\n' "$test_count" >&2
  exit 1
fi

if (( failures != 0 )); then
  printf '%d of %d slice-scope policy assertions failed\n' "$failures" "$test_count"
  exit 1
fi

printf 'all %d slice-scope policy assertions passed\n' "$test_count"
