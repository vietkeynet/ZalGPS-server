# Acceptance and evidence gates

| Gate | Minimum evidence |
|---|---|
| G0 source | clean worktree baseline, scoped diff, license/notice, review, exact SHA, public-safe secret scan |
| G1 CI | gofmt, go mod verify, go test -race -count=1 ./..., go vet, build; integration must execute, no skip-as-pass |
| G2 security | threat model; invalid/missing/revoked tokens, spoofing, tenant A/B isolation through SQL/API/websocket/cache/jobs, DB least privilege, no raw secrets/locations in logs; dependency scan |
| G3 durability | commit-before-ACK (where protocol allows), duplicate/late/replay, concurrency, crash between receive/commit/ACK/worker, DB outage, bounded retry, recovery counts |
| G4 migrations | fresh/upgrade from pilot/parallel migrator, migration checksums, backup/restore rehearsal, schema/binary compatibility |
| G5 protocol | exact model/firmware/transport, golden Java comparison, frame split/merge, max size, invalid CRC, fuzz, heartbeat, ACK and unsupported commands; physical test only for HARDWARE VERIFIED |
| G6 UI/API | OpenAPI contract, E2E tenant isolation, map coordinate accuracy, realtime reconnect, pagination/exports, accessibility, mobile consent/offline |
| G7 performance | same resource profile Java vs Go; 1k devices at 15s, 5x short bursts, reconnect storm, DB fail/restart; RSS, CPU, p95/p99, queue, disk, loss accounting; no unbounded production stress |
| G8 operations | staging-only TLS and firewall, least privilege, separate backup destination, readiness/metrics, alerts, signed artifact/SBOM, rollback rehearsal and approvals |
| G9 release | private security/compliance review, documented physical hardware, support plan, owner acceptance; no critical unresolved finding |

Pilot proposed targets (not measured): <=256 MiB core RSS at 1k/15s, <=500 ms p95 ingest-to-durable; measure protocol ACK deadline independently. No acknowledged durable loss under specified crash tests. Explicitly count received/accepted/duplicate/rejected/retried/persisted. Define RPO/RTO with owner; do not claim zero-loss outside tested durability assumptions. Current CI #1 establishes G1 for pilot only, not all gates.

## Test record template
Task ID, commit SHA, environment label and resource budget, fixture origin (synthetic/public licensed only), command, expected/actual, PASS/FAIL/SKIP, CI link or private evidence ID, reviewer, remaining blocker and next task. Never attach packet dumps or secret scan output publicly. Failed acceptance cannot be waived silently.
