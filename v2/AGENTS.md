# ZalGPS V2 agent execution contract

## Read order
1. Root AGENTS.md and SECURITY.md; this file.
2. TODO.md (authoritative task statuses and dependencies), ROADMAP.md.
3. docs/architecture.md, docs/deployment.md, docs/acceptance.md, docs/protocol-matrix.md.
4. Current git branch, HEAD, diff, CI and previous private handoff (authorized workspace only).

## Roles
Coordinator owns task sequencing and integration; Security reviewer owns threat model, tenant tests and disclosure; Core engineer owns DB/ingest; Protocol engineer owns codecs and differential fixtures; QA owns independent test and benchmark evidence; UI/mobile engineers own clients; Operations owns isolated staging, recovery and release; Product owner owns commercial/regulatory sign-off. Roles may be assigned to one agent sequentially. A reviewer should not rely on the implementer's claims alone.

## Work protocol
- Select the highest-priority READY task whose dependencies are DONE; claim one task in TODO or private coordination state. Only coordinator edits task statuses/merges overlapping changes. BLOCKED tasks do not authorize bypassing their gate.
- State task ID, scope, acceptance criteria, dependencies, intended files, baseline SHA and rollback. Smallest useful diff, no premature microservices or framework churn.
- Use dedicated worktrees for parallel non-overlapping work. Never share a writable worktree among agents. Use isolated disposable DB and synthetic fixtures. No real fleet data in logs or tests.
- Run gofmt, go mod verify, go test -race -count=1 ./..., go vet ./..., go build ./cmd/zalgpsd, applicable integration/fuzz/security tests. Ensure integration tests actually execute; SKIP is not PASS. Check dependency advisories and staged secret scan with redaction. Public CI is GitHub-hosted only; no credentials from fork code.
- Commit only reviewed allowlisted files, not git add -A in a mixed workspace. Preserve attribution/license. No force push. Before push, fetch and confirm expected branch head. Keep private evidence out of commit and PR. CI must be green on exact tested SHA before acceptance.
- Update task status, evidence and handoff. If blocked, state exact blocker, impact and next safe action. Do not create a new PR merely to restate a blocker. No auto merge/deploy without release gate and approval.
- Never mark hardware VERIFIED without model/firmware/transport tests; never label compliance CERTIFIED without applicable documentation. No stealth tracking or vehicle immobilization in MVP.

## Status
DONE = acceptance evidence for defined scope; READY = dependencies met; TODO = not started; IN_PROGRESS = assigned and active; BLOCKED = specific dependency/risk; VERIFIED = independently tested claim (not a task status). CI PASS is not production approval.

## Handoff format
Task ID / objective; baseline and resulting SHA; files changed; tests (commands, pass/fail/skip, evidence link or private reference); security and data impact; unresolved risks; rollback; next READY task. Never include secrets, raw traces or internal topology in public handoff.
