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

if [[ "$version" != "2" || -z "$slice_id" || -z "$state" ]]; then
  echo "slice-check: contract requires version=2, slice_id, and state" >&2
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
  if [[ "$path" == *$'\t'* || "$path" == *$'\r'* || "$path" == *$'\n'* ]]; then
    echo "slice-check: contract paths may not contain TAB, CR, or LF" >&2
    exit 1
  fi
  case "$path" in
    /*|./*|../*|*/../*|*/..|*//* )
      echo "slice-check: contract paths must be clean repo-relative paths: $path" >&2
      exit 1
      ;;
  esac
}

validate_path "$scope_file"
if (( ${#allows[@]} == 0 )); then
  echo "slice-check: contract requires at least one allowed path" >&2
  exit 1
fi
if (( ${#tests[@]} == 0 )); then
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
  "tasks/slice-commit-ledger.tsv"
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
    if [[ "$state" != "verified" ]]; then
      echo "slice-check: --${mode} requires state=verified; found: $state" >&2
      exit 1
    fi
    ;;
  test-candidate)
    if [[ "$state" != "contract-frozen" ]]; then
      echo "slice-check: --test-candidate requires state=contract-frozen; found: $state" >&2
      exit 1
    fi
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
if ! git status --porcelain=v1 -z --untracked-files=all > "$status_file"; then
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
candidate_has_control=0
ledger_path="tasks/slice-commit-ledger.tsv"
for ((i = 0; i < ${#changed_paths[@]}; i++)); do
  path="${changed_paths[$i]}"
  index_state="${index_states[$i]}"
  worktree_state="${worktree_states[$i]}"

  if listed "$path" "${ignores[@]}"; then
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

  if [[ "$path" != "$scope_file" ]] && listed "$path" "${control_paths[@]}"; then
    case "$mode" in
      candidate|test-candidate)
        echo "slice-check: control-plane path requires --guardrail-candidate: $path" >&2
        exit 1
        ;;
      guardrail-candidate)
        if [[ "$path" == "$ledger_path" ]] && \
          { [[ "$slice_id" != "g1-slice-state-enforcement" || "$index_state" != "A" ]] || git cat-file -e "HEAD:$ledger_path" 2>/dev/null; }; then
          echo "slice-check: ledger changes require --ledger-candidate" >&2
          exit 1
        fi
        candidate_has_control=1
        ;;
    esac
  fi

  if [[ "$path" != "$scope_file" ]] && ! listed "$path" "${allows[@]}" && \
    { [[ "$mode" != "install" ]] || ! listed "$path" "${install_paths[@]}"; } && \
    { [[ "$mode" != "ledger-candidate" || "$path" != "$ledger_path" ]]; }; then
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
      if [[ "$mode" == "test-candidate" ]] && ! listed "$path" "${tests[@]}"; then
        echo "slice-check: test candidate contains undeclared test path: $path" >&2
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
    ledger-candidate)
      if [[ "$path" != "$ledger_path" ]]; then
        echo "slice-check: ledger candidate must be isolated; found: $path" >&2
        exit 1
      fi
      if [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
        echo "slice-check: ledger candidate is not exactly staged" >&2
        exit 1
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

if [[ "$mode" == "guardrail-candidate" && "$candidate_has_control" != "1" ]]; then
  echo "slice-check: guardrail candidate contains no control-plane path" >&2
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

    if [[ "$head_state" != "planned" ]]; then
      head_frozen="$tmp_dir/head-frozen"
      candidate_frozen="$tmp_dir/candidate-frozen"
      if [[ "$head_version" == "1" && "$slice_id" == "g1-slice-state-enforcement" && "$head_state" == "tests-red" && "$state" == "implemented" ]]; then
        grep -Ev '^(#|$|version=|state=|test=)' "$head_scope" > "$head_frozen"
        grep -Ev '^(#|$|version=|state=|test=)' "$scope_file" > "$candidate_frozen"
      else
        grep -Ev '^(#|$|state=)' "$head_scope" > "$head_frozen"
        grep -Ev '^(#|$|state=)' "$scope_file" > "$candidate_frozen"
      fi
      if ! cmp -s "$head_frozen" "$candidate_frozen"; then
        echo "slice-check: frozen contract fields changed after contract-frozen" >&2
        exit 1
      fi
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

candidate_digest() {
  local index_file="$tmp_dir/candidate-index"
  local manifest_file="$tmp_dir/candidate-manifest"
  rm -f -- "$index_file"
  GIT_INDEX_FILE="$index_file" git read-tree HEAD
  GIT_INDEX_FILE="$index_file" git add -A -- "$@"
  GIT_INDEX_FILE="$index_file" git diff --cached --raw --no-abbrev -z HEAD -- "$@" > "$manifest_file"
  [[ -s "$manifest_file" ]] || {
    echo "slice-check: candidate contains no staged payload" >&2
    return 1
  }
  sha256_file "$manifest_file"
}

candidate_trailer() {
  local commit="$1"
  local message_file="$tmp_dir/commit-message"
  local trailers_file="$tmp_dir/commit-trailers"
  local trailer_count=0
  local trailer_value=""
  local trailer_line

  git show -s --format=%B "$commit" > "$message_file"
  git interpret-trailers --parse < "$message_file" > "$trailers_file"
  while IFS= read -r trailer_line || [[ -n "$trailer_line" ]]; do
    case "$trailer_line" in
      "Candidate-SHA256: "*)
        trailer_count=$((trailer_count + 1))
        trailer_value="${trailer_line#Candidate-SHA256: }"
        ;;
    esac
  done < "$trailers_file"
  if (( trailer_count != 1 )) || [[ ! "$trailer_value" =~ ^[0-9a-f]{64}$ ]]; then
    echo "slice-check: verified scope commit requires exactly one valid Candidate-SHA256 trailer" >&2
    return 1
  fi
  printf '%s\n' "$trailer_value"
}

validate_ledger_candidate() {
  local old_ledger="$tmp_dir/old-ledger"
  local staged_ledger="$tmp_dir/staged-ledger"
  local prefix="$tmp_dir/ledger-prefix"
  local old_lines new_lines row schema row_slice contract_commit payload_commit digest reason extra
  local head_commit parent_commit contract_scope contract_state contract_slice computed path
  local commit_allows=()
  local commit_manifest="$tmp_dir/commit-manifest"

  git show "HEAD:$ledger_path" > "$old_ledger" 2>/dev/null || {
    echo "slice-check: committed closure ledger is unavailable" >&2
    return 1
  }
  git show ":$ledger_path" > "$staged_ledger" 2>/dev/null || {
    echo "slice-check: ledger removal is forbidden" >&2
    return 1
  }
  if [[ "$(sed -n '1p' "$old_ledger")" != $'version\tslice_id\tcontract_commit\tpayload_commit\tcandidate_sha256\treason' ]]; then
    echo "slice-check: committed closure ledger header is invalid" >&2
    return 1
  fi
  old_lines="$(awk 'END {print NR}' "$old_ledger")"
  new_lines="$(awk 'END {print NR}' "$staged_ledger")"
  if (( new_lines != old_lines + 1 )); then
    echo "slice-check: ledger candidate must append exactly one row" >&2
    return 1
  fi
  sed -n "1,${old_lines}p" "$staged_ledger" > "$prefix"
  if ! cmp -s "$old_ledger" "$prefix"; then
    echo "slice-check: closure ledger is append-only" >&2
    return 1
  fi
  row="$(sed -n "${new_lines}p" "$staged_ledger")"
  IFS=$'\t' read -r schema row_slice contract_commit payload_commit digest reason extra <<< "$row"
  if [[ -z "$schema" || -z "$row_slice" || -z "$contract_commit" || -z "$payload_commit" || -z "$digest" || -z "$reason" || -n "${extra:-}" ]]; then
    echo "slice-check: appended ledger row is malformed" >&2
    return 1
  fi
  head_commit="$(git rev-parse HEAD)"
  if [[ "$payload_commit" != "$head_commit" ]] || ! git cat-file -e "${contract_commit}^{commit}" 2>/dev/null; then
    echo "slice-check: ledger commits do not identify the current closure" >&2
    return 1
  fi
  parent_commit="$(git rev-parse "${payload_commit}^")"
  if [[ "$parent_commit" != "$contract_commit" ]]; then
    echo "slice-check: ledger contract is not the payload parent" >&2
    return 1
  fi

  contract_scope="$tmp_dir/ledger-contract-scope"
  git show "${contract_commit}:$scope_file" > "$contract_scope" 2>/dev/null || {
    echo "slice-check: ledger contract scope is unavailable" >&2
    return 1
  }
  contract_state="$(sed -n 's/^state=//p' "$contract_scope")"
  contract_slice="$(sed -n 's/^slice_id=//p' "$contract_scope")"
  if [[ "$contract_state" != "verified" || "$contract_slice" != "$row_slice" ]]; then
    echo "slice-check: ledger contract is not the verified slice authority" >&2
    return 1
  fi

  if [[ "$schema" == "bootstrap-v1" ]]; then
    if [[ "$row_slice" != "g1-slice-state-enforcement" || "$digest" != "-" || "$reason" != "self-upgrade-not-self-authenticating" ]] || \
      grep -q $'^bootstrap-v1\tg1-slice-state-enforcement\t' "$old_ledger"; then
      echo "slice-check: invalid G1 bootstrap closure row" >&2
      return 1
    fi
    return 0
  fi
  if [[ "$schema" != "v1" || "$reason" != "-" || ! "$digest" =~ ^[0-9a-f]{64}$ ]]; then
    echo "slice-check: normal ledger row requires v1 digest evidence" >&2
    return 1
  fi

  while IFS= read -r line || [[ -n "$line" ]]; do
    case "$line" in allow=*) commit_allows+=("${line#allow=}") ;; esac
  done < "$contract_scope"
  (( ${#commit_allows[@]} > 0 )) || {
    echo "slice-check: ledger contract has no allowed payload" >&2
    return 1
  }
  while IFS= read -r -d '' path; do
    listed "$path" "${commit_allows[@]}" || {
      echo "slice-check: payload commit escaped its verified allowlist: $path" >&2
      return 1
    }
  done < <(git diff --name-only -z "$contract_commit" "$payload_commit")
  git diff --raw --no-abbrev -z "$contract_commit" "$payload_commit" -- "${commit_allows[@]}" > "$commit_manifest"
  computed="$(sha256_file "$commit_manifest")"
  if [[ "$(candidate_trailer "$contract_commit")" != "$digest" || "$computed" != "$digest" ]]; then
    echo "slice-check: ledger candidate digest does not match the verified payload" >&2
    return 1
  fi
}

total_files="${#scoped_paths[@]}"
production_files=0
changed_lines=0

for ((i = 0; i < ${#scoped_paths[@]}; i++)); do
  path="${scoped_paths[$i]}"
  index_state="${scoped_index_states[$i]}"
  if [[ ( "$path" == cmd/* || "$path" == internal/* || "$path" == scripts/* ) && "$path" != *_test.go ]]; then
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

case "$mode" in
  candidate|guardrail-candidate)
    computed_digest="$(candidate_digest "${scoped_paths[@]}")"
    expected_digest="$(candidate_trailer HEAD)"
    if [[ "$computed_digest" != "$expected_digest" ]]; then
      echo "slice-check: staged candidate digest does not match verified scope trailer" >&2
      exit 1
    fi
    ;;
  ledger-candidate)
    validate_ledger_candidate
    ;;
esac

echo "slice-check: PASS slice=$slice_id state=$state mode=$mode files=$total_files production=$production_files lines=$changed_lines"
if [[ -n "$exception" ]]; then
  echo "slice-check: recorded exception: $exception"
fi
