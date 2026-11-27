# ZalGPS V2 — security threat model

**Scope:** current V2 pilot only: loopback HTTP JSON ingest and latest-position read, PostgreSQL storage, and the repository CI/dependency evidence listed below. This is a code-and-workflow review, not a production deployment assessment, penetration test, compliance opinion, or statement that planned controls are implemented. No private topology, credentials, or operational configuration is included.

## Scope and trust boundaries

- **Device/client → HTTP listener:** bearer token and position JSON cross an input boundary. Listener binds `127.0.0.1:18082`; a local reverse proxy or other local process, if configured, is outside this code's proven boundary. Loopback is not equivalent to an internet-facing TLS endpoint.
- **Ingest handler → storage:** handler authenticates through storage, validates bounded JSON, then delegates persistence; request context limits store work. The service has no user, membership, or management API in this pilot.
- **Service → PostgreSQL:** database connection is configured externally. SQL uses parameters and authentication selects device and tenant from token hash; position queries use both. Database host, network policy, encryption, privileges, backups, and actual DSN handling in deployment are not established here.
- **Repository → CI and release workflows:** V2 CI is separate from release publication. CI configuration evidence below does not establish repository-level branch rules or secret availability.

## Assets

- Device bearer tokens and their SHA-256 hashes; enrollment, rotation, and revocation state.
- Location fixes, timestamps, tenant/device association, and latest position.
- Integrity and availability of PostgreSQL data and the V2 binary/build artifacts.
- CI credentials/permissions and release artifacts or publication credentials (existence and availability not assessed).

## Threat actors and abuse cases

- **Unauthenticated remote or local caller:** probes endpoints, submits malformed/oversized JSON, or floods requests to consume service capacity.
- **Caller with stolen/guessed device token:** submits fabricated locations or reads latest location for token's device. Bearer possession is the authorization basis; device/user identity beyond enrollment is not established by request payload.
- **Malicious or compromised enrolled device:** replays or fabricates plausible position data, attempts cross-tenant/device access, or submits duplicate/late fixes.
- **Compromised local process, proxy, host, or database credential:** reads or alters data, bypasses assumptions at trust boundaries, or captures bearer tokens. Loopback binding alone does not mitigate local compromise.
- **Malicious contribution or compromised CI dependency/action:** attempts to execute code in CI or affect published artifacts; scope depends on workflow permissions, dependency integrity, and release workflow controls.
- **Availability abuse:** occupies all in-process handler slots or exhausts database capacity; limits observed in code are not a measured capacity guarantee.

## Evidence — implemented behavior observed

- Ingest hashes the Bearer token with SHA-256 before storage authentication; database stores token hash, and rotation/revocation functions are present.
- HTTP server binds loopback and configures read-header, read, write, idle, and header-size limits.
- Ingest limits request body to 4096 bytes, rejects unknown JSON fields and trailing JSON, validates position, uses a three-second handler context timeout, and bounds concurrent in-process requests with 64 slots. Saturation returns 503 with `Retry-After`.
- Storage uses parameterized SQL. Authentication resolves tenant/device from a non-revoked token hash; latest-position filtering and deduplication include tenant/device scope.
- Existing ingest tests cover missing token, invalid/missing coordinates, spoofed tenant field, trailing JSON, oversized body, success, duplicate, unauthorized storage response, generic storage failure response without diagnostic leakage. PostgreSQL integration test covers duplicate token rejection, per-tenant same-device behavior, latest read isolation, rotation, and revocation when a disposable DB is configured.
- V2 CI declares `contents: read`, uses `ubuntu-latest`, pins checkout and setup-go to commit SHAs with persisted credentials disabled, installs Go 1.26, and runs formatting, module verification, race-enabled Go tests, vet, and build. Workflow has no deploy secret references and does not use `pull_request_target`.
- Dependency review evidence supplied for this task: `GOPROXY=off GOSUMDB=off go mod verify` passed using cached modules. Local Trivy offline scan passed with no vulnerability rows for the scanned V2 module only. These results do not prove absence of vulnerabilities.

## Risks not established or not verified

- No evidence here verifies TLS, a public ingress/proxy configuration, firewalling, rate limiting, or production deployment. Do not expose this loopback pilot directly to untrusted networks.
- Current evidence is not proof of SQL row-level security or restricted runtime DB privileges. DB-02 is outstanding; SQL scoping in these code paths does not prove isolation against direct database access or future callers.
- In-process 64-slot cap does not limit distributed clients, request rate, aggregate replicas, or all resource use. No load/DoS test evidence supplied.
- Tests listed above do not establish fuzz coverage, concurrent race behavior of handler/storage under adversarial load, malformed Authorization edge cases, token brute-force resistance, log/telemetry privacy, backup/restore security, or production key lifecycle controls.
- Legacy Gradle workflow has no `permissions` declaration. Review and least-privilege hardening remain appropriate; workflow content alone does not prove exploitable exposure. Legacy dependency scan was skipped: Gradle/Maven unavailable, Java 17 environment differs from target Java 21, and wrapper/dependency download was not cached. No clean legacy vulnerability result is claimed.
- Separate release workflow references publication secrets and artifact push. This does not establish whether any referenced secret is configured or available. Its action pinning, permissions, artifact provenance, and publication controls need independent review; no report of actual secret values or access is included.
- CI module verification and offline scan are bounded to available cached V2 dependencies; neither replaces online/current advisory review, lockfile/SBOM review, or review of transitive and legacy dependencies.

## Negative-test checklist

Start from tests in `v2/internal/ingest/ingest_test.go` and `v2/internal/storage/postgres_test.go`; add cases before treating these as covered:

- [ ] Missing, malformed, wrong-scheme, altered, rotated, and revoked Bearer tokens fail closed; no storage access for invalid token.
- [ ] Spoofed tenant/device fields, unknown fields, concatenated/trailing JSON values, malformed JSON, empty body, and body at/over 4096-byte boundary are rejected as intended.
- [ ] Invalid coordinates (including NaN/Infinity JSON representations), missing coordinates, invalid/future timestamps, and invalid speed never persist.
- [ ] Tenant A token cannot read or affect tenant B's device/history; same device ID across tenants remains isolated; parameterized input cannot alter SQL behavior.
- [ ] DB timeout/failure returns generic client error without SQL, DSN, token, or internal diagnostic disclosure.
- [ ] Concurrent requests beyond 64 occupied slots reject excess with 503 and release slots after completion/error; verify bounded behavior under cancellation/timeouts.
- [ ] HTTP method/content-type/path checks reject unsupported requests; verify health endpoint reveals no sensitive operational state.
- [ ] Run PostgreSQL integration cases with disposable DB configured; skipped integration test is not a pass.

## Small hardening priorities

1. Keep V2 listener loopback-only; if an external access path is proposed, require separately reviewed TLS/authenticated proxy and ingress controls first.
2. Complete DB-02: separate runtime and migration identities, least privilege, and direct-SQL/cross-tenant negative tests; do not treat application query filters as a substitute.
3. Review legacy and release workflow permissions/action pins; explicitly constrain workflow token permissions where compatible, then validate artifact and publication path. No repository settings change is implied here.
4. Extend negative tests above, especially overload/cancellation, malformed auth, and configured PostgreSQL tenant isolation; ensure integration execution is visible rather than skipped.
5. Establish repeatable dependency/advisory review for V2 and legacy with supported toolchains and current data; document scan scope, date, and skips.

## CI and dependency review notes

V2 CI has a narrow read-only repository permission declaration, SHA-pinned checkout, disabled checkout credential persistence, and no deploy secrets or `pull_request_target` observed. Workflow inspection is not a review of GitHub repository settings, runner isolation, or secret availability. Legacy Gradle workflow's missing permissions declaration and separate release workflow merit follow-up review. V2 offline verification and Trivy result are scoped to cached V2 modules; legacy scan did not run for the environment reasons stated above. No broad claim of dependency safety is warranted.

## Caveats

This document separates observed source/workflow/test evidence from unverified risk. It does not assert that every listed test passed in this task, that production controls exist, that the system is secure, or that the product is ready for deployment. Reassess on changes to listener exposure, auth, storage, workflows, dependencies, or deployment design. Public-safe summary only; keep private findings and environment-specific evidence out of this document.
