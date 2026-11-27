# Coordinator handoff

Baseline verified: 8c245ce2b47e05cf427e7c6e73de2277282766e1, GitHub Actions #1 success. Go V2 is a loopback native JSON/PostgreSQL pilot. Java remains untouched. No production deployment or device compatibility beyond tested native ingest.

Active queue: SEC-01 / SEC-02 / SEC-03; DB-01 integrated locally with tests, awaiting hosted CI. OPS-01 can run independently. DB-02 requires DB-01 and SEC-03 acceptance. Read TODO.md for authoritative dependencies, AGENTS.md for execution rules. Private security evidence and infrastructure inventory must be retrieved only through authorized local coordination channels, not inferred from public docs. Do not upload private reports.

## Agent dispatch template
Task ID and objective; required reading; baseline SHA; allowed files; dependencies; explicit acceptance gates; resource budget and isolated environment; no secret/real data publication; expected commit and evidence; rollback; stop conditions. Return result in AGENTS.md format. Coordinator validates before marking DONE. No extra PRs, no force-push, no merge or deploy without approval.

## Handoff maintenance
Update this summary only when milestone or active queue changes. TODO remains source of truth. Public handoff contains generic status only. Each agent checks current HEAD/CI before starting; never blindly reuse stale SHA.
