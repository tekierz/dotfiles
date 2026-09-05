# Audit evidence and reproduction

This directory belongs to the [September 5 audit](../../codebase-git-audit-2026-09-05.md), whose baseline is `d77c0f790a3694df21dfc8e784eef66dfb2614b2`. Findings describe that checkout unless explicitly compared with a newer branch.

The three focused reviews contain source references and the original temporary reproduction paths. Durable probe sources are saved here with `.go.txt` extensions so they are not compiled into the product or ordinary tests. Logs use `.log.txt` so repository ignore rules do not hide the evidence. No credential values or binary failure dumps are included.

- `product-probes.go.txt`: seven tests asserting desired behavior; they intentionally **fail** on the audited code, thereby demonstrating the defects. `product-probes.log.txt` contains their output.
- `recovery-probes.go.txt`: two tests asserting the observed defects; they **pass** when the catalog retains payload bytes and restore accepts changed content.
- `runner-probes.go.txt`, `apt-probes.go.txt`, `apt-deadlock-probes.go.txt`: three fake-process/package tests asserting the observed defects; they **pass** when those defects are reproduced. They never invoke a real package mutation.
- `verification.json`: exit codes for individual audit checks, including initial sandbox failures. Final normal/race results are the `*-unsandboxed` entries. Zero-length Staticcheck output indicates success.
- `remote-evidence.json`: selected read-only GitHub API results observed September 5, including exact branches, protection, and CI run heads. `homebrew-formula.rb.txt` is the external formula observed at the same time.
- `tracked-inventory.tsv`: initial tracked-path inventory, including line/byte counts. `secret-pattern-scan.json` records the separate history-scan coverage and limitations.

To recreate the two Go overlays without editing compiled repository files, run from the audited repository root with the intended Go toolchain available:

```python
import json
import pathlib
import subprocess
import tempfile

repo = pathlib.Path.cwd()
evidence = repo / "tasks/audit-work/2026-09-05"
scratch = pathlib.Path(tempfile.mkdtemp(prefix="dotfiles-audit-probes-"))
for name, package in (("product", "ui"), ("recovery", "backup")):
    overlay = scratch / f"{name}.json"
    overlay.write_text(json.dumps({"Replace": {
        str(repo / f"internal/{package}/audit_reproduction_test.go"):
        str(evidence / f"{name}-probes.go.txt")
    }}))
    result = subprocess.run([
        "go", "test", "-overlay", str(overlay), f"./internal/{package}",
        "-run", "^TestAudit", "-count=1", "-v"
    ])
    print(name, "exit", result.returncode)
```

For the execution probes, copy the baseline's non-test `.go` files from `internal/runner` and `internal/pkg`, plus `go.mod` and `go.sum`, into a disposable directory preserving those paths. Add `runner-probes.go.txt` as `internal/runner/audit_test.go`, and the two APT files as separate `_test.go` files under `internal/pkg`. Run `go test ./internal/runner ./internal/pkg -run TestAudit -count=1 -v` there. Do not add these defect-asserting tests as permanent acceptance tests without rewriting the assertions to require corrected behavior.

Use temporary writable Go/lint caches in a restricted environment. The exact descendant-cancellation baseline test required execution outside the macOS sandbox in this session; its initial failure to create a PID fixture was not counted as a product defect. Baseline tests were run with a minimal inherited environment because existing failure diagnostics can print fixture environments.

These probes operate on temporary fixtures. They do not substitute for real platform, package-manager, owner-hardware, terminal, or release verification.
