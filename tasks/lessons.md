# Working Lessons

- When the user asks for persistent multi-agent execution, keep at least one bounded
  reviewer or implementation agent visibly active whenever concurrency permits. Check
  agent state before reporting progress; an errored or completed thread is not active work.
- A release-readiness task is complete only when the tracked remediation plan, adversarial
  findings, automated gates, and deployment evidence all agree. Passing a focused test
  batch is a checkpoint, not a release verdict.
- Keep logical commits flowing after each independently verified layer, but never mix the
  user's untracked planning files into staging.
