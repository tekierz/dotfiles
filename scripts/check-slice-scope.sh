#!/usr/bin/env bash

set -euo pipefail

mode="worktree"
case "${1:-}" in
  --candidate)
    mode="candidate"
    shift
    ;;
  --test-candidate)
    mode="test-candidate"
    shift
    ;;
  --guardrail-candidate)
    mode="guardrail-candidate"
    shift
    ;;
  --ledger-candidate)
    mode="ledger-candidate"
    shift
    ;;
  --contract-candidate)
    mode="contract-candidate"
    shift
    ;;
  --install)
    mode="install"
    shift
    ;;
esac

scope_file="${1:-tasks/current-slice.scope}"
ledger_file="tasks/slice-commit-ledger.tsv"
ledger_header=$'record_type\tslice_id\tcontract_frozen_commit\ttests_commit\tverified_scope_commit\tpayload_commit\tcandidate_sha256\treason'
canonical_g1b_genesis() {
  printf '%s\n' \
    "$ledger_header" \
    $'bootstrap-v1\tg0-plan-state-reset\t1aa1c36b3a21d9bc9e31362b21e536e57187e711\t-\t-\tee1b5847ad026dcdfb4871d41064e2b9742cedfd\t-\tpre-g1-unverified' \
    $'bootstrap-v1\tg1-slice-state-enforcement\t668f1acfe751970b205af6eed6bc83ad8bf5894f\t4675a1a235c35e6cd3a03aa3871088e003c1659b\t-\t-\t-\tsuperseded-at-tests-red' \
    $'bootstrap-v1\tg1-replan-catalog\t1bb4f9b6e56d7c1b09cc0602b4ddda29593f58e2\t-\t-\t493235a9a98f64e38cdae105144fcd71eefe8bce\t-\tpre-enforcement-replan-catalog' \
    $'bootstrap-v1\tg1a-candidate-authority\tdf0bd217ce0f8e36ae5f44e6d09a0a187c161d43\t770bf8d1ec2ebf28fdb9e91362723aa9ce90a6fd\ta62cc74e32fe38821a4524e1bf2d18884d1045eb\t3c6025adadde63fcc5d7289b06ae947e658a5b43\tf0727c52696ef06e4ff21eb10b5bcd8f3ca1f4404a5218963b495bd1670722ed\tcandidate-checker-self-upgrade-bootstrap'
}
repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

if [[ ! -f "$scope_file" ]]; then
  echo "slice-check: missing scope contract: $scope_file" >&2
  exit 1
fi

version=""
slice_id=""
state=""
max_total_files=""
max_production_files=""
max_changed_lines=""
exception=""
allows=()
ignores=()
tests=()
seen_version=0
seen_slice_id=0
seen_state=0
seen_max_total_files=0
seen_max_production_files=0
seen_max_changed_lines=0
seen_exception=0

duplicate_key() {
  echo "slice-check: duplicate singleton key: $1" >&2
  exit 1
}

while IFS= read -r line || [[ -n "$line" ]]; do
  [[ -z "$line" || "$line" == \#* ]] && continue
  if [[ "$line" != *=* ]]; then
    echo "slice-check: malformed contract line: $line" >&2
    exit 1
  fi

  key="${line%%=*}"
  value="${line#*=}"
  if [[ -z "$value" ]]; then
    echo "slice-check: empty value for $key" >&2
    exit 1
  fi

  case "$key" in
    version)
      (( seen_version == 0 )) || duplicate_key "$key"
      seen_version=1
      version="$value"
      ;;
    slice_id)
      (( seen_slice_id == 0 )) || duplicate_key "$key"
      seen_slice_id=1
      slice_id="$value"
      ;;
    state)
      (( seen_state == 0 )) || duplicate_key "$key"
      seen_state=1
      state="$value"
      ;;
    max_total_files)
      (( seen_max_total_files == 0 )) || duplicate_key "$key"
      seen_max_total_files=1
      max_total_files="$value"
      ;;
    max_production_files)
      (( seen_max_production_files == 0 )) || duplicate_key "$key"
      seen_max_production_files=1
      max_production_files="$value"
      ;;
    max_changed_lines)
      (( seen_max_changed_lines == 0 )) || duplicate_key "$key"
      seen_max_changed_lines=1
      max_changed_lines="$value"
      ;;
    exception)
      (( seen_exception == 0 )) || duplicate_key "$key"
      seen_exception=1
      exception="$value"
      ;;
    allow) allows+=("$value") ;;
    ignore) ignores+=("$value") ;;
    test) tests+=("$value") ;;
    *)
      echo "slice-check: unknown contract key: $key" >&2
      exit 1
      ;;
  esac
done < "$scope_file"

if [[ ( "$version" != "1" && "$version" != "2" ) || -z "$slice_id" || -z "$state" ]]; then
  echo "slice-check: contract requires version 1 or 2, slice_id, and state" >&2
  exit 1
fi

case "$state" in
  planned|contract-frozen|tests-red|implemented|reviewed|verified|committed) ;;
  *)
    echo "slice-check: invalid slice state: $state" >&2
    exit 1
    ;;
esac

for value in "$max_total_files" "$max_production_files" "$max_changed_lines"; do
  if [[ ! "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "slice-check: budgets must be positive integers" >&2
    exit 1
  fi
done

if (( max_total_files > 16 || max_production_files > 6 || max_changed_lines > 800 )); then
  if [[ -z "$exception" ]]; then
    echo "slice-check: above-default budgets require a recorded exception" >&2
    exit 1
  fi
fi

listed() {
  local wanted="$1"
  shift
  local candidate
  for candidate in "$@"; do
    [[ "$candidate" == "$wanted" ]] && return 0
  done
  return 1
}

validate_path() {
  local path="$1"
  case "$path" in
    /*|./*|../*|*/../*|*/..|*//* )
      echo "slice-check: contract paths must be clean repo-relative paths: $path" >&2
      exit 1
      ;;
  esac
}

is_production_path() {
  local path="$1"
  [[ ( "$path" == cmd/* || "$path" == internal/* || "$path" == scripts/* ) && "$path" != *_test.go ]]
}

validate_path "$scope_file"
if (( ${#allows[@]} == 0 )); then
  echo "slice-check: contract requires at least one allowed path" >&2
  exit 1
fi
if [[ "$version" == "2" ]] && (( ${#tests[@]} == 0 )); then
  echo "slice-check: version 2 contract requires at least one explicit test path" >&2
  exit 1
fi

install_paths=(
  "Makefile"
  "scripts/check-slice-scope.sh"
  "tasks/current-slice.scope"
  "tasks/lessons.md"
  "tasks/workflow-guardrails.md"
)
control_paths=(
  "Makefile"
  "scripts/check-slice-scope.sh"
  "tests/check-slice-scope_test.sh"
  "tasks/current-slice.scope"
  "tasks/lessons.md"
  "tasks/workflow-guardrails.md"
)

for ((i = 0; i < ${#allows[@]}; i++)); do
  path="${allows[$i]}"
  validate_path "$path"
  [[ "$path" != "$scope_file" ]] || {
    echo "slice-check: active contract may not allowlist itself" >&2
    exit 1
  }
  for ((j = i + 1; j < ${#allows[@]}; j++)); do
    [[ "$path" != "${allows[$j]}" ]] || {
      echo "slice-check: duplicate allowed path: $path" >&2
      exit 1
    }
  done
done

for ((i = 0; i < ${#tests[@]}; i++)); do
  path="${tests[$i]}"
  validate_path "$path"
  listed "$path" "${allows[@]}" || {
    echo "slice-check: test path must match an allowed path: $path" >&2
    exit 1
  }
  is_production_path "$path" && {
    echo "slice-check: production path may not be declared as a test: $path" >&2
    exit 1
  }
  if listed "$path" "${control_paths[@]}" && [[ "$path" != "tests/check-slice-scope_test.sh" ]]; then
    echo "slice-check: control-plane path may not be declared as a test: $path" >&2
    exit 1
  fi
  for ((j = i + 1; j < ${#tests[@]}; j++)); do
    [[ "$path" != "${tests[$j]}" ]] || {
      echo "slice-check: duplicate test path: $path" >&2
      exit 1
    }
  done
done

for ((i = 0; i < ${#ignores[@]}; i++)); do
  path="${ignores[$i]}"
  validate_path "$path"
  if [[ "$path" == "$scope_file" || "$path" == cmd/* || "$path" == internal/* || "$path" == scripts/* ]] || listed "$path" "${control_paths[@]}"; then
    echo "slice-check: production/control paths may not be ignored: $path" >&2
    exit 1
  fi
  listed "$path" "${allows[@]}" && {
    echo "slice-check: path cannot be both allowed and ignored: $path" >&2
    exit 1
  }
  for ((j = i + 1; j < ${#ignores[@]}; j++)); do
    [[ "$path" != "${ignores[$j]}" ]] || {
      echo "slice-check: duplicate ignored path: $path" >&2
      exit 1
    }
  done
done

if [[ "$mode" == "install" ]]; then
  for path in "scripts/check-slice-scope.sh" "tasks/current-slice.scope"; do
    if git cat-file -e "HEAD:$path" 2>/dev/null; then
      echo "slice-check: --install is only valid before the guardrails exist in HEAD" >&2
      exit 1
    fi
  done
fi

case "$mode" in
  candidate|guardrail-candidate|ledger-candidate)
    [[ "$state" == "verified" ]] || {
      echo "slice-check: --$mode requires state=verified; found: $state" >&2
      exit 1
    }
    ;;
  test-candidate)
    [[ "$state" == "contract-frozen" ]] || {
      echo "slice-check: --test-candidate requires state=contract-frozen; found: $state" >&2
      exit 1
    }
    ;;
esac

tmp_dir="$(mktemp -d "${TMPDIR:-/tmp}/slice-check.XXXXXX")"
[[ -n "$tmp_dir" && -d "$tmp_dir" && "${tmp_dir##*/}" == slice-check.* ]] || {
  echo "slice-check: temporary directory creation failed" >&2
  exit 1
}
cleanup() {
  [[ -n "$tmp_dir" && -d "$tmp_dir" && "${tmp_dir##*/}" == slice-check.* ]] || return 1
  rm -rf -- "$tmp_dir"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
status_file="$tmp_dir/status"
numstat_file="$tmp_dir/numstat"
if ! git -c status.renames=false status --porcelain=v1 -z --untracked-files=all > "$status_file"; then
  echo "slice-check: git status failed" >&2
  exit 1
fi

changed_paths=()
index_states=()
worktree_states=()
while IFS= read -r -d '' entry; do
  status="${entry:0:2}"
  path="${entry:3}"
  if [[ "$path" == *$'\t'* || "$path" == *$'\r'* || "$path" == *$'\n'* ]]; then
    echo "slice-check: repository paths may not contain TAB, CR, or LF" >&2
    exit 1
  fi
  if [[ "$status" == *R* || "$status" == *C* ]]; then
    echo "slice-check: rename/copy status requires a new frozen contract: $path" >&2
    exit 1
  fi
  index_state="${status:0:1}"
  worktree_state="${status:1:1}"
  case "$index_state" in
    " "|M|A|D|"?") ;;
    *)
      echo "slice-check: unrecognized index status '$index_state': $path" >&2
      exit 1
      ;;
  esac
  case "$worktree_state" in
    " "|M|D|"?") ;;
    *)
      echo "slice-check: unrecognized worktree status '$worktree_state': $path" >&2
      exit 1
      ;;
  esac
  if [[ ( "$index_state" == "?" || "$worktree_state" == "?" ) && "$status" != "??" ]]; then
    echo "slice-check: malformed untracked status '$status': $path" >&2
    exit 1
  fi
  changed_paths+=("$path")
  index_states+=("${status:0:1}")
  worktree_states+=("${status:1:1}")
done < "$status_file"

scoped_paths=()
scoped_index_states=()
contract_change_found=0
test_candidate_paths=0
guardrail_paths=0
g1b_genesis=0
g1b_genesis_attempt=0
if [[ "$mode" == "guardrail-candidate" && "$slice_id" == "g1b-ledger-closure" ]] && \
    ! git cat-file -e "HEAD:$ledger_file" 2>/dev/null; then
  genesis_ledger=0
  for ((i = 0; i < ${#changed_paths[@]}; i++)); do
    path="${changed_paths[$i]}"
    index_state="${index_states[$i]}"
    worktree_state="${worktree_states[$i]}"
    if (( ${#ignores[@]} > 0 )) && listed "$path" "${ignores[@]}"; then
      continue
    fi
    if [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
      continue
    fi
    [[ "$path" == "$ledger_file" && "$index_state" == "A" ]] && genesis_ledger=1
  done
  if (( genesis_ledger == 1 )); then
    g1b_genesis_attempt=1
    canonical_g1b_genesis > "$tmp_dir/canonical-genesis"
    git show ":$ledger_file" > "$tmp_dir/staged-genesis" 2>/dev/null || true
    cmp -s "$tmp_dir/canonical-genesis" "$tmp_dir/staged-genesis" && g1b_genesis=1
  fi
fi
for ((i = 0; i < ${#changed_paths[@]}; i++)); do
  path="${changed_paths[$i]}"
  index_state="${index_states[$i]}"
  worktree_state="${worktree_states[$i]}"

  if (( ${#ignores[@]} > 0 )) && listed "$path" "${ignores[@]}"; then
    if [[ "$index_state" != " " && "$index_state" != "?" ]]; then
      echo "slice-check: ignored/user-owned path must never be staged: $path" >&2
      exit 1
    fi
    continue
  fi

  if [[ "$index_state" != " " && "$index_state" != "?" && "$worktree_state" != " " ]]; then
    echo "slice-check: mixed staged/unstaged payload is forbidden: $path" >&2
    exit 1
  fi

  if [[ "$mode" == "test-candidate" && "$path" != "$scope_file" ]]; then
    listed "$path" "${tests[@]}" || {
      echo "slice-check: test candidate contains undeclared test path: $path" >&2
      exit 1
    }
    if [[ "$index_state" == "D" ]]; then
      echo "slice-check: explicit test candidate may not delete: $path" >&2
      exit 1
    fi
    test_candidate_paths=$((test_candidate_paths + 1))
  fi

  if [[ "$mode" == "ledger-candidate" && "$path" != "$scope_file" && "$path" != "$ledger_file" ]]; then
    echo "slice-check: ledger candidate may contain only $ledger_file: $path" >&2
    exit 1
  fi

  if [[ "$path" != "$scope_file" ]] && listed "$path" "${control_paths[@]}"; then
    case "$mode" in
      candidate)
        echo "slice-check: control-plane path requires --guardrail-candidate: $path" >&2
        exit 1
        ;;
      guardrail-candidate) guardrail_paths=$((guardrail_paths + 1)) ;;
    esac
  elif [[ "$mode" == "guardrail-candidate" && "$path" == "$ledger_file" && "$g1b_genesis_attempt" == "1" ]]; then
    if [[ "$g1b_genesis" != "1" ]]; then
      echo "slice-check: staged G1B ledger genesis does not match canonical bootstrap history" >&2
      exit 1
    fi
    guardrail_paths=$((guardrail_paths + 1))
  elif [[ "$mode" == "guardrail-candidate" && "$path" != "$scope_file" ]]; then
    echo "slice-check: guardrail candidate may contain only control-plane paths: $path" >&2
    exit 1
  fi

  if [[ "$path" != "$scope_file" ]] && ! listed "$path" "${allows[@]}" && { [[ "$mode" != "install" ]] || ! listed "$path" "${install_paths[@]}"; }; then
    echo "slice-check: path outside frozen allowlist: $path" >&2
    exit 1
  fi

  case "$mode" in
    worktree)
      [[ "$path" != "$scope_file" ]] || {
        echo "slice-check: active contract changed; commit it separately before slice work" >&2
        exit 1
      }
      if [[ "$index_state" != " " && "$index_state" != "?" ]]; then
        echo "slice-check: staged payload requires --candidate verification: $path" >&2
        exit 1
      fi
      ;;
    candidate|test-candidate|guardrail-candidate|ledger-candidate)
      [[ "$path" != "$scope_file" ]] || {
        echo "slice-check: product candidate may not modify its scope contract" >&2
        exit 1
      }
      if [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
        echo "slice-check: commit candidate is not exactly staged: $path" >&2
        exit 1
      fi
      ;;
    contract-candidate)
      if [[ "$path" != "$scope_file" ]]; then
        if [[ "$index_state" != " " && "$index_state" != "?" ]]; then
          echo "slice-check: contract candidate contains staged payload: $path" >&2
          exit 1
        fi
      elif [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
        echo "slice-check: contract candidate is not exactly staged" >&2
        exit 1
      else
        contract_change_found=1
      fi
      ;;
    install)
      if listed "$path" "${install_paths[@]}"; then
        if [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
          echo "slice-check: guardrail install path is not exactly staged: $path" >&2
          exit 1
        fi
      elif [[ "$index_state" != " " && "$index_state" != "?" ]]; then
        echo "slice-check: guardrail install contains staged product work: $path" >&2
        exit 1
      fi
      ;;
  esac

  if [[ "$mode" != "contract-candidate" || "$path" != "$scope_file" ]]; then
    scoped_paths+=("$path")
    scoped_index_states+=("$index_state")
  fi
done

if [[ "$mode" == "test-candidate" && "$test_candidate_paths" == "0" ]]; then
  echo "slice-check: test candidate contains no staged tests" >&2
  exit 1
fi
if [[ "$mode" == "guardrail-candidate" && "$guardrail_paths" == "0" ]]; then
  echo "slice-check: guardrail candidate contains no control-plane path" >&2
  exit 1
fi
if [[ "$mode" == "candidate" && ${#scoped_paths[@]} -eq 0 ]]; then
  echo "slice-check: candidate contains no staged payload" >&2
  exit 1
fi
if [[ "$mode" == "ledger-candidate" && ( ${#scoped_paths[@]} -ne 1 || "${scoped_paths[0]:-}" != "$ledger_file" ) ]]; then
  echo "slice-check: ledger candidate must contain exactly $ledger_file" >&2
  exit 1
fi

if [[ "$mode" == "contract-candidate" && "$contract_change_found" != "1" ]]; then
  echo "slice-check: contract candidate does not contain a staged contract change" >&2
  exit 1
fi

validate_tests_predecessor() {
  local delta="$tmp_dir/tests-predecessor" path count=0 invalid=0
  git rev-parse HEAD^ >/dev/null 2>&1 || invalid=1
  if (( invalid == 0 )); then
    git diff-tree --no-commit-id --name-only -r --no-renames -z HEAD^ HEAD > "$delta"
    while IFS= read -r -d '' path; do
      count=$((count + 1))
      if ! listed "$path" "${tests[@]}"; then
        if is_production_path "$path"; then
          echo "slice-check: tests commit contains non-test path: $path" >&2
          return 1
        fi
        invalid=1
      elif ! git cat-file -e "HEAD:$path" 2>/dev/null; then
        echo "slice-check: tests commit may not delete explicit test: $path" >&2
        return 1
      fi
    done < "$delta"
  fi
  if (( count == 0 || invalid == 1 )); then
    echo "slice-check: tests-red transition requires a nonempty immediately prior test commit" >&2
    return 1
  fi
}

validate_ledger_predecessor() {
  local delta="$tmp_dir/ledger-predecessor" path count=0 last_slice slice_count
  git rev-parse HEAD^ >/dev/null 2>&1 || count=99
  if (( count == 0 )); then
    git diff-tree --no-commit-id --name-only -r --no-renames -z HEAD^ HEAD > "$delta"
    while IFS= read -r -d '' path; do
      count=$((count + 1))
      [[ "$path" == "$ledger_file" ]] || count=99
    done < "$delta"
  fi
  if (( count != 1 )); then
    echo "slice-check: committed transition requires an immediately prior ledger-only commit" >&2
    return 1
  fi
  git show "HEAD:$ledger_file" > "$tmp_dir/closing-ledger" 2>/dev/null || {
    echo "slice-check: committed transition requires an immediately prior ledger-only commit" >&2
    return 1
  }
  last_slice="$(awk -F '\t' 'END {print $2}' "$tmp_dir/closing-ledger")"
  slice_count="$(awk -F '\t' -v slice="$slice_id" 'NR > 1 && $2 == slice {n++} END {print n + 0}' "$tmp_dir/closing-ledger")"
  if [[ "$last_slice" != "$slice_id" || "$slice_count" != "1" ]]; then
    echo "slice-check: last ledger row does not close active slice: $last_slice" >&2
    return 1
  fi
}

ledger_closure_pending=0
if [[ "$mode" == "contract-candidate" ]]; then
  head_scope="$tmp_dir/head-scope"
  git show "HEAD:$scope_file" > "$head_scope" 2>/dev/null || {
    echo "slice-check: committed scope contract is unavailable" >&2
    exit 1
  }
  head_version="$(sed -n 's/^version=//p' "$head_scope")"
  head_slice_id="$(sed -n 's/^slice_id=//p' "$head_scope")"
  head_state="$(sed -n 's/^state=//p' "$head_scope")"
  if [[ -z "$head_version" || -z "$head_slice_id" || -z "$head_state" ]]; then
    echo "slice-check: committed scope contract is malformed" >&2
    exit 1
  fi

  if [[ "$head_slice_id" != "$slice_id" ]]; then
    if [[ "$head_state" != "committed" || "$state" != "planned" ]]; then
      echo "slice-check: cannot start $slice_id before $head_slice_id reaches committed; found: $head_state" >&2
      exit 1
    fi
    if [[ "$version" != "2" ]]; then
      echo "slice-check: new slice contracts require version=2; found: $version" >&2
      exit 1
    fi
    if [[ "$head_slice_id" == "g1a-candidate-authority" && "$slice_id" != "g1b-ledger-closure" ]]; then
      echo "slice-check: G1A bootstrap permits only g1b-ledger-closure; found: $slice_id" >&2
      exit 1
    fi
    if [[ "$head_slice_id" == "g1b-ledger-closure" && "$slice_id" != "g1c-guardrail-adoption" ]]; then
      echo "slice-check: G1B bootstrap permits only g1c-guardrail-adoption; found: $slice_id" >&2
      exit 1
    fi
  else
    expected_state=""
    case "$head_state" in
      planned) expected_state="contract-frozen" ;;
      contract-frozen) expected_state="tests-red" ;;
      tests-red) expected_state="implemented" ;;
      implemented) expected_state="reviewed" ;;
      reviewed) expected_state="verified" ;;
      verified) expected_state="committed" ;;
    esac
    if [[ "$state" != "$expected_state" ]]; then
      echo "slice-check: illegal state transition for $slice_id: $head_state -> $state" >&2
      exit 1
    fi
    if [[ "$head_version" == "2" && "$version" != "2" ]]; then
      echo "slice-check: version 2 contract may not downgrade; found: $version" >&2
      exit 1
    fi
    if [[ "$head_state" == "verified" && "$state" == "committed" ]]; then
      validate_ledger_predecessor || exit 1
      ledger_closure_pending=1
    fi

    head_frozen="$tmp_dir/head-frozen"
    candidate_frozen="$tmp_dir/candidate-frozen"
    if [[ "$head_version" == "1" && "$slice_id" == "g1a-candidate-authority" && "$head_state" == "tests-red" && "$state" == "implemented" ]]; then
      if [[ "$version" != "2" || ${#tests[@]} -ne 1 || "${tests[0]}" != "tests/check-slice-scope_test.sh" ]]; then
        echo "slice-check: G1A migration requires exactly test=tests/check-slice-scope_test.sh" >&2
        exit 1
      fi
      grep -Ev '^(#|$|version=|state=|test=)' "$head_scope" > "$head_frozen"
      grep -Ev '^(#|$|version=|state=|test=)' "$scope_file" > "$candidate_frozen"
    elif [[ "$head_state" != "planned" ]]; then
      grep -Ev '^(#|$|state=)' "$head_scope" > "$head_frozen"
      grep -Ev '^(#|$|state=)' "$scope_file" > "$candidate_frozen"
    fi
    if [[ "$head_state" != "planned" ]] && ! cmp -s "$head_frozen" "$candidate_frozen"; then
      echo "slice-check: frozen contract fields changed after contract-frozen" >&2
      exit 1
    fi
    if [[ "$head_state" == "contract-frozen" && "$state" == "tests-red" ]]; then
      validate_tests_predecessor || exit 1
    fi
  fi
fi

if [[ "$mode" == "install" ]]; then
  for required in "${install_paths[@]}"; do
    found=0
    for ((i = 0; i < ${#changed_paths[@]}; i++)); do
      if [[ "${changed_paths[$i]}" == "$required" && "${index_states[$i]}" != " " && "${index_states[$i]}" != "?" && "${worktree_states[$i]}" == " " ]]; then
        found=1
        break
      fi
    done
    (( found == 1 )) || {
      echo "slice-check: required guardrail install path is not staged: $required" >&2
      exit 1
    }
  done
fi

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{print $1}'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$file" | awk '{print $NF}'
  else
    echo "slice-check: no SHA-256 implementation is available" >&2
    return 1
  fi
}

verified_trailer() {
  local message="$tmp_dir/verified-message"
  local trailers="$tmp_dir/verified-trailers"
  local commit="${1:-HEAD}"
  local line value="" count=0

  git show -s --format=%B "$commit" > "$message"
  git interpret-trailers --parse < "$message" > "$trailers"
  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in
      "Candidate-SHA256: "*)
        count=$((count + 1))
        value="${line#Candidate-SHA256: }"
        ;;
    esac
  done < "$trailers"
  if (( count != 1 )) || [[ ! "$value" =~ ^[0-9a-f]{64}$ ]]; then
    echo "slice-check: verified scope commit requires exactly one valid Candidate-SHA256 trailer" >&2
    return 1
  fi
  printf '%s\n' "$value"
}

verify_candidate_authority() {
  local delta="$tmp_dir/verified-delta"
  local parent_scope="$tmp_dir/reviewed-scope"
  local verified_scope="$tmp_dir/verified-scope"
  local path count=0 parent_state verified_state parent_slice verified_slice

  git rev-parse HEAD^ >/dev/null 2>&1 || {
    echo "slice-check: verified candidate authority must be the reviewed-to-verified scope commit" >&2
    return 1
  }
  git diff-tree --no-commit-id --name-only -r --no-renames -z HEAD^ HEAD > "$delta"
  while IFS= read -r -d '' path; do
    count=$((count + 1))
    [[ "$path" == "$scope_file" ]] || count=99
  done < "$delta"
  git show "HEAD^:$scope_file" > "$parent_scope" 2>/dev/null || count=99
  git show "HEAD:$scope_file" > "$verified_scope" 2>/dev/null || count=99
  parent_state="$(sed -n 's/^state=//p' "$parent_scope")"
  verified_state="$(sed -n 's/^state=//p' "$verified_scope")"
  parent_slice="$(sed -n 's/^slice_id=//p' "$parent_scope")"
  verified_slice="$(sed -n 's/^slice_id=//p' "$verified_scope")"
  if (( count != 1 )) || [[ "$parent_state" != "reviewed" || "$verified_state" != "verified" || "$parent_slice" != "$verified_slice" || "$verified_slice" != "$slice_id" ]]; then
    echo "slice-check: verified candidate authority must be the reviewed-to-verified scope commit" >&2
    return 1
  fi
  grep -Ev '^(#|$|state=)' "$parent_scope" > "$tmp_dir/reviewed-frozen"
  grep -Ev '^(#|$|state=)' "$verified_scope" > "$tmp_dir/verified-frozen"
  if ! cmp -s "$tmp_dir/reviewed-frozen" "$tmp_dir/verified-frozen"; then
    echo "slice-check: verified candidate authority must be the reviewed-to-verified scope commit" >&2
    return 1
  fi
  verified_trailer
}

staged_candidate_digest() {
  local manifest="$tmp_dir/staged-manifest"
  local entry scope_mode scope_blob

  entry="$(git ls-tree HEAD -- "$scope_file")"
  read -r scope_mode _ scope_blob _ <<< "$entry"
  [[ -n "$scope_mode" && -n "$scope_blob" ]] || {
    echo "slice-check: verified scope tree entry is unavailable" >&2
    return 1
  }
  printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest"
  git diff --cached --raw --no-renames --no-abbrev -z HEAD -- "${scoped_paths[@]}" >> "$manifest"
  sha256_file "$manifest"
}

committed_candidate_digest() {
  local verified="$1" payload="$2" manifest="$tmp_dir/committed-manifest"
  local entry scope_mode scope_blob
  entry="$(git ls-tree "$verified" -- "$scope_file")" || return 1
  read -r scope_mode _ scope_blob _ <<< "$entry"
  [[ -n "$scope_mode" && -n "$scope_blob" ]] || return 1
  printf 'scope\0%s\0%s\0' "$scope_mode" "$scope_blob" > "$manifest"
  git diff --raw --no-renames --no-abbrev -z "$verified" "$payload" >> "$manifest"
  sha256_file "$manifest"
}

validate_ledger_candidate() {
  local base_ref="${1:-HEAD}" candidate_ref="${2:-index}" authority_parent
  local head_ledger="$tmp_dir/head-ledger" candidate_ledger="$tmp_dir/candidate-ledger"
  local prefix="$tmp_dir/ledger-prefix" row record row_slice frozen tests_commit verified payload digest reason
  local head_lines candidate_lines commit role_state role_slice parent_state path trailer computed count=0 delta_count=0

  git show "$base_ref:$ledger_file" > "$head_ledger" 2>/dev/null || {
    echo "slice-check: ledger history is append-only" >&2
    return 1
  }
  if [[ "$candidate_ref" == "index" ]]; then
    git show ":$ledger_file" > "$candidate_ledger" 2>/dev/null || return 1
  else
    git show "$candidate_ref:$ledger_file" > "$candidate_ledger" 2>/dev/null || return 1
  fi
  authority_parent="$(git rev-parse "$base_ref")" || return 1
  head_lines="$(wc -l < "$head_ledger" | tr -d ' ')"
  candidate_lines="$(wc -l < "$candidate_ledger" | tr -d ' ')"
  head -n "$head_lines" "$candidate_ledger" > "$prefix"
  if ! cmp -s "$head_ledger" "$prefix"; then
    echo "slice-check: ledger history is append-only" >&2
    return 1
  fi
  if (( candidate_lines != head_lines + 1 )); then
    echo "slice-check: ledger candidate must append exactly one row" >&2
    return 1
  fi
  if [[ "$(sed -n '1p' "$candidate_ledger")" != "$ledger_header" ]]; then
    echo "slice-check: ledger header must be: $ledger_header" >&2
    return 1
  fi
  row="$(tail -n 1 "$candidate_ledger")"
  if ! printf '%s\n' "$row" | awk -F '\t' 'NF != 8 {exit 1} {for (i=1;i<=8;i++) if ($i=="") exit 1}'; then
    echo "slice-check: ledger row must contain exactly 8 nonempty tab-separated fields" >&2
    return 1
  fi
  IFS=$'\t' read -r record row_slice frozen tests_commit verified payload digest reason <<< "$row"
  if awk -F '\t' -v slice="$row_slice" 'NR > 1 && $2 == slice {found=1} END {exit !found}' "$head_ledger"; then
    echo "slice-check: duplicate ledger slice_id: $row_slice" >&2
    return 1
  fi
  if [[ "$row_slice" != "$slice_id" ]]; then
    echo "slice-check: ledger row slice_id does not match active slice: $row_slice" >&2
    return 1
  fi
  if [[ "$record" == "normal-v1" ]]; then
    echo "slice-check: normal-v1 ledger rows remain disabled until G1C" >&2
    return 1
  fi
  if [[ "$record" != "bootstrap-v1" ]]; then
    echo "slice-check: unsupported ledger record_type: $record" >&2
    return 1
  fi
  if [[ ! "$digest" =~ ^[0-9a-f]{64}$ ]]; then
    echo "slice-check: ledger candidate_sha256 must be 64 lowercase hexadecimal characters" >&2
    return 1
  fi
  for commit in "$frozen" "$tests_commit" "$verified" "$payload"; do
    if [[ ! "$commit" =~ ^[0-9a-f]{40}([0-9a-f]{24})?$ ]] || ! git cat-file -e "$commit^{commit}" 2>/dev/null; then
      echo "slice-check: ledger evidence commit is unavailable: $commit" >&2
      return 1
    fi
  done
  role_state="$(git show "$frozen:$scope_file" 2>/dev/null | sed -n 's/^state=//p')"
  if [[ "$role_state" != "contract-frozen" ]]; then
    echo "slice-check: ledger contract_frozen_commit does not record contract-frozen state" >&2
    return 1
  fi
  role_slice="$(git show "$frozen:$scope_file" 2>/dev/null | sed -n 's/^slice_id=//p')"
  if [[ "$role_slice" != "$row_slice" ]]; then
    echo "slice-check: ledger contract_frozen_commit records another slice" >&2
    return 1
  fi
  role_state="$(git show "$verified:$scope_file" 2>/dev/null | sed -n 's/^state=//p')"
  if [[ "$role_state" != "verified" ]]; then
    echo "slice-check: ledger verified_scope_commit does not record verified state" >&2
    return 1
  fi
  role_slice="$(git show "$verified:$scope_file" 2>/dev/null | sed -n 's/^slice_id=//p')"
  if [[ "$payload" != "$authority_parent" ]]; then
    echo "slice-check: ledger payload_commit must equal ledger candidate parent HEAD" >&2
    return 1
  fi
  git rev-list --first-parent "$verified" > "$tmp_dir/ledger-first-parent"
  if [[ "$frozen" == "$tests_commit" || "$tests_commit" == "$verified" || "$verified" == "$payload" ]] || \
      [[ "$(git rev-parse "$tests_commit^")" != "$frozen" || "$(git rev-parse "$payload^")" != "$verified" ]] || \
      ! grep -Fxq "$tests_commit" "$tmp_dir/ledger-first-parent"; then
    echo "slice-check: ledger evidence commits are not in required first-parent order" >&2
    return 1
  fi
  parent_state="$(git show "$verified^:$scope_file" 2>/dev/null | sed -n 's/^state=//p')"
  git diff-tree --no-commit-id --name-only -r --no-renames -z "$verified^" "$verified" > "$tmp_dir/verified-ledger-delta"
  while IFS= read -r -d '' path; do
    delta_count=$((delta_count + 1))
    [[ "$path" == "$scope_file" ]] || delta_count=99
  done < "$tmp_dir/verified-ledger-delta"
  git show "$verified^:$scope_file" | grep -Ev '^(#|$|state=)' > "$tmp_dir/ledger-reviewed-frozen"
  git show "$verified:$scope_file" | grep -Ev '^(#|$|state=)' > "$tmp_dir/ledger-verified-frozen"
  if [[ "$role_slice" != "$row_slice" || "$parent_state" != "reviewed" || "$delta_count" != "1" ]]; then
    echo "slice-check: ledger verified_scope_commit is not exact reviewed-to-verified authority" >&2
    return 1
  fi
  if ! cmp -s "$tmp_dir/ledger-reviewed-frozen" "$tmp_dir/ledger-verified-frozen"; then
    echo "slice-check: ledger verified_scope_commit changed frozen authority" >&2
    return 1
  fi
  git diff-tree --no-commit-id --name-only -r --no-renames -z "$tests_commit^" "$tests_commit" > "$tmp_dir/ledger-tests"
  while IFS= read -r -d '' path; do
    count=$((count + 1))
    if ! listed "$path" "${tests[@]}"; then
      echo "slice-check: ledger tests_commit contains non-test path: $path" >&2
      return 1
    elif ! git cat-file -e "$tests_commit:$path" 2>/dev/null; then
      echo "slice-check: ledger tests_commit may not delete explicit test: $path" >&2
      return 1
    fi
  done < "$tmp_dir/ledger-tests"
  (( count > 0 )) || {
    echo "slice-check: ledger tests_commit contains no test path" >&2
    return 1
  }
  git diff-tree --no-commit-id --name-only -r --no-renames -z "$payload^" "$payload" > "$tmp_dir/ledger-payload"
  while IFS= read -r -d '' path; do
    if ! listed "$path" "${allows[@]}"; then
      echo "slice-check: ledger payload commit contains path outside frozen allowlist: $path" >&2
      return 1
    fi
  done < "$tmp_dir/ledger-payload"
  trailer="$(verified_trailer "$verified")" || return 1
  computed="$(committed_candidate_digest "$verified" "$payload")" || return 1
  if ! [[ "$digest" == "$trailer" && "$digest" == "$computed" ]]; then
    echo "slice-check: ledger candidate digest does not match verified authority" >&2
    return 1
  fi
  if [[ "$row_slice" == "g1b-ledger-closure" && "$reason" != "ledger-checker-self-upgrade-bootstrap" ]]; then
    echo "slice-check: G1B bootstrap reason must be ledger-checker-self-upgrade-bootstrap" >&2
    return 1
  fi
}

if [[ "$mode" == "ledger-candidate" ]]; then
  validate_ledger_candidate || exit 1
fi
if (( ledger_closure_pending == 1 )); then
  validate_ledger_candidate HEAD^ HEAD || exit 1
fi

total_files="${#scoped_paths[@]}"
production_files=0
changed_lines=0

for ((i = 0; i < ${#scoped_paths[@]}; i++)); do
  path="${scoped_paths[$i]}"
  index_state="${scoped_index_states[$i]}"
  if is_production_path "$path"; then
    production_files=$((production_files + 1))
  fi

  if [[ "$index_state" != " " && "$index_state" != "?" ]]; then
    if ! git diff --cached --numstat HEAD -- "$path" > "$numstat_file"; then
      echo "slice-check: staged diff accounting failed: $path" >&2
      exit 1
    fi
  elif [[ "$index_state" == "?" ]]; then
    if [[ ! -f "$path" ]]; then
      echo "slice-check: unsupported untracked path type: $path" >&2
      exit 1
    fi
    lines="$(wc -l < "$path")"
    changed_lines=$((changed_lines + lines))
    continue
  elif ! git diff --numstat HEAD -- "$path" > "$numstat_file"; then
    echo "slice-check: worktree diff accounting failed: $path" >&2
    exit 1
  fi

  while IFS=$'\t' read -r added deleted _; do
    if [[ "$added" == "-" || "$deleted" == "-" ]]; then
      echo "slice-check: binary change requires an explicit separate slice: $path" >&2
      exit 1
    fi
    changed_lines=$((changed_lines + added + deleted))
  done < "$numstat_file"
done

failed=0
if (( total_files > max_total_files )); then
  echo "slice-check: $total_files files exceed ceiling $max_total_files" >&2
  failed=1
fi
if (( production_files > max_production_files )); then
  echo "slice-check: $production_files production files exceed ceiling $max_production_files" >&2
  failed=1
fi
if (( changed_lines > max_changed_lines )); then
  echo "slice-check: $changed_lines changed lines exceed ceiling $max_changed_lines" >&2
  failed=1
fi
(( failed == 0 )) || exit 1

if [[ "$mode" == "candidate" || "$mode" == "guardrail-candidate" ]]; then
  expected_digest="$(verify_candidate_authority)"
  computed_digest="$(staged_candidate_digest)"
  if [[ "$computed_digest" != "$expected_digest" ]]; then
    echo "slice-check: staged candidate digest does not match verified scope trailer" >&2
    exit 1
  fi
fi

echo "slice-check: PASS slice=$slice_id state=$state mode=$mode files=$total_files production=$production_files lines=$changed_lines"
if [[ -n "$exception" ]]; then
  echo "slice-check: recorded exception: $exception"
fi
