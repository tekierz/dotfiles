# Working Lessons

- If a slice crosses a second delivery surface or a default scope ceiling, stop and split it
  before production grows. Repeated reviewer expansion becomes a named child slice, and
  completion is recorded only in the delivering commit.
- Verification is commit-candidate evidence, not a reusable historical claim. Any production
  edit expires it; run the frozen acceptance gates again before marking or committing work.
- For this remediation program, use a Sol-named agent to orchestrate and review
  each bounded slice and a Terra-named agent to implement it; the root agent
  coordinates verification and logical commits rather than bypassing that split.
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
- If two consecutive Terra implementation turns fail to persist an explicitly requested
  tests-first checkpoint, Sol may make the smallest tests-only persistence change after root
  approval. Production work must return to the Sol-orchestrator/Terra-implementer split, and
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
