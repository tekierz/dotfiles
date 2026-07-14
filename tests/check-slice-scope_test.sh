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

if (( test_count != 27 )); then
  printf 'test harness error: expected 27 assertions, ran %d\n' "$test_count" >&2
  exit 1
fi

if (( failures != 0 )); then
  printf '%d of %d slice-scope policy assertions failed\n' "$failures" "$test_count"
  exit 1
fi

printf 'all %d slice-scope policy assertions passed\n' "$test_count"
