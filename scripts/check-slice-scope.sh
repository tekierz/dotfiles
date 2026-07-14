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
  candidate|guardrail-candidate)
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
    test_candidate_paths=$((test_candidate_paths + 1))
  fi

  if [[ "$path" != "$scope_file" ]] && listed "$path" "${control_paths[@]}"; then
    case "$mode" in
      candidate)
        echo "slice-check: control-plane path requires --guardrail-candidate: $path" >&2
        exit 1
        ;;
      guardrail-candidate) guardrail_paths=$((guardrail_paths + 1)) ;;
    esac
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
    candidate|test-candidate|guardrail-candidate)
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

if [[ "$mode" == "contract-candidate" && "$contract_change_found" != "1" ]]; then
  echo "slice-check: contract candidate does not contain a staged contract change" >&2
  exit 1
fi

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
  local line value="" count=0

  git show -s --format=%B HEAD > "$message"
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
