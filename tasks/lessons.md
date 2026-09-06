# Working Lessons

- If a slice crosses a second delivery surface or a default scope ceiling, stop and split it
  before production grows. Repeated reviewer expansion becomes a named child slice, and
  completion is recorded only in the delivering commit.
- Verification is commit-candidate evidence, not a reusable historical claim. Any production
  edit expires it; run the frozen acceptance gates again before marking or committing work.
- Use gpt-6-astra for every subagent. Vary reasoning effort by task difficulty,
  not model family: low for straightforward tasks, medium for ordinary work,
  high or higher for complex implementation and safety review. The user confirmed
  this on 2026-09-05; old Sol/Terra and global mixed-model rules are superseded.
- When the user asks for persistent multi-agent execution, keep at least one bounded
  reviewer or implementation agent visibly active whenever concurrency permits. Check
  agent state before reporting progress; an errored or completed thread is not active work.
- A release-readiness task is complete only when the tracked remediation plan, adversarial
  findings, automated gates, and deployment evidence all agree. Passing a focused test
  batch is a checkpoint, not a release verdict.
- Keep logical commits flowing after each independently verified layer, but never mix the
  user's untracked planning files into staging.
- During long architectural remediation, commit each reviewed contract, neutral foundation,
  adapter adoption, and command surface separately before beginning the next layer; a green
  worktree checkpoint is part of traceability, not cleanup deferred to the end.
- After converting an if/else chain to a condition switch, run `gofmt` on that
  file immediately before touching another file; a partial mechanical rewrite
  must never leave the shared multi-agent tree syntactically invalid.
- Before adding a linter-requested lifecycle call such as `t.Parallel`, inspect
  the whole function for an existing platform-guarded call; duplicate lifecycle
  calls can compile cleanly yet panic at runtime.
- After extracting a large function into helpers, run the package compile before
  any other edit so obsolete locals and signatures cannot destabilize the shared
  tree, even briefly.
- If an implementation agent violates an explicit small, test-first checkpoint
  twice, interrupt that turn and replace it with a fresh tests-only agent. Do not
  permit more production growth until the complete failing matrix is visible and
  independently reviewed.
- Registry presence is not feature reachability. For every added integration, test the full
  user path through Manage, the deep-dive selector, immutable planning, confirmation, and
  typed execution; a tool that exists only in registry metadata is still a missing feature.
- Terminal category styling must preserve meaning without color or decorative glyphs. Treat
  narrow layouts, plain-ASCII terminals, and reduced-color environments as first-class
  acceptance cases rather than visual polish deferred until release.
- If two consecutive implementation-agent turns fail to persist an explicitly requested
  tests-first checkpoint, the Astra reviewer may make the smallest tests-only persistence change after root
  approval. Production work must return to the Astra reviewer/implementer ownership split, and
  the directly persisted contract still requires independent adversarial review.
- When the user asks to finish implementation, measure and report progress in runnable product
  code first. Governance documents, policy tests, scope transitions, and ledger commits are
  prerequisites or evidence, not implementation progress, and must not dominate the work or
  the status report after the governing contract is already sound.
- Root-owned staging must never leave implementation agents waiting at a state-machine gate.
  Commit planned, frozen, and red-test checkpoints immediately; otherwise agents may either
  idle without product output or begin code before authority exists. If code appears early,
  preserve it outside the worktree, restore the exact parent without destructive Git commands,
  and reapply it only after the correct tests-first checkpoint.
- When the user explicitly prioritizes a working dogfood build, optimize for complete runnable
  behavior and useful operational logging. Use existing tests plus the smallest compile/smoke
  checks needed to keep the application testable; defer expanded adversarial, race, policy, and
  cross-platform matrices until the end-to-end feature path exists. Do not let per-slice release
  ceremony delay a build the user can install and exercise manually.

- Read current implementation before declaring a feature blocked from historical todo prose.
  The September integration implements phased npm authority even though older checklist
  sections still describe the superseded temporary disable.
- Pin GOTOOLCHAIN=go1.26.8 for release verification and candidate Homebrew builds;
  the host default can be newer, and Homebrew superenv overrides ambient GOTOOLCHAIN.
- Package-manager inventory commands can return nonzero for an expected state:
  named `brew outdated` exits 1 when an upgrade exists. Capture its status and
  validate the structured result before classifying the command as a failure.
- Verify subprocess environment neutralization against the real tool as well as a native
  stand-in. npm 11 rejects identical user/global config paths, even when both point
  to the null device; mocks that only echo the environment cannot detect this.
- Check executable-identity size limits against actual supported upstream binaries.
  Official Node 24 standalone binaries exceed 64 MiB even though Homebrew split-library
  launchers are smaller; keep a bounded real-size regression and cap-plus-one refusal.
- A successful CodeQL workflow means analysis completed, not that its findings gate
  passed. Inspect the separate CodeQL PR check and open branch alerts before promotion.

- Live installation acceptance must bridge the real installation observer into public plan/apply. Synthetic authoritative receipt fixtures hid two failures: legacy boolean absence was unknown, and Node prerequisite receipts were incorrectly required to establish CLI presence. Give supported recipe-backed CLIs typed binary evidence and evaluate prerequisite receipts only in their matching manager namespace; never loosen generic product-presence authority.

- Pinned Go 1.26.8 ignores linked-worktree `.git` files for embedded VCS metadata, even with `-buildvcs=true`. A clean ordinary local clone with a real `.git` directory restores exact `vcs.revision` and `vcs.modified=false`; use it for provenance-sensitive release rehearsals rather than weakening verification or changing product build flags.

- Validate release-evidence links against Git’s tracked/indexed files, not only filesystem existence. Repository-wide log ignores can silently omit bounded synthetic test evidence; explicitly stage only the reviewed evidence files instead of changing global ignore rules.
