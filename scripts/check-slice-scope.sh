#!/usr/bin/env bash

set -euo pipefail

mode="worktree"
case "${1:-}" in
  --candidate)
    mode="candidate"
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
    *)
      echo "slice-check: unknown contract key: $key" >&2
      exit 1
      ;;
  esac
done < "$scope_file"

if [[ "$version" != "1" || -z "$slice_id" || -z "$state" ]]; then
  echo "slice-check: contract requires version=1, slice_id, and state" >&2
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

validate_path "$scope_file"
if (( ${#allows[@]} == 0 )); then
  echo "slice-check: contract requires at least one allowed path" >&2
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

status_file="$(mktemp)"
numstat_file="$(mktemp)"
trap 'rm -f "$status_file" "$numstat_file"' EXIT
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

  if [[ "$mode" != "install" && "$path" != "$scope_file" ]] && listed "$path" "${control_paths[@]}"; then
    echo "slice-check: control-plane path requires a separate guardrail commit: $path" >&2
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
    candidate)
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
        echo "slice-check: contract candidate must be isolated; found: $path" >&2
        exit 1
      fi
      if [[ "$index_state" == " " || "$index_state" == "?" || "$worktree_state" != " " ]]; then
        echo "slice-check: contract candidate is not exactly staged" >&2
        exit 1
      fi
      contract_change_found=1
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

  scoped_paths+=("$path")
  scoped_index_states+=("$index_state")
done

if [[ "$mode" == "contract-candidate" && "$contract_change_found" != "1" ]]; then
  echo "slice-check: contract candidate does not contain a staged contract change" >&2
  exit 1
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

echo "slice-check: PASS slice=$slice_id state=$state mode=$mode files=$total_files production=$production_files lines=$changed_lines"
if [[ -n "$exception" ]]; then
  echo "slice-check: recorded exception: $exception"
fi
