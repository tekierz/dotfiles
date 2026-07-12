# Working Lessons

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
