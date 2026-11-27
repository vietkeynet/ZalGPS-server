# Public V2 security policy

Experimental pilot only. Do not publish private deployment configuration, internal network details, customer identifiers, real GPS captures, credentials, database exports or logs. Report vulnerabilities privately via GitHub private vulnerability reporting if enabled or a private maintainer channel. Rotate exposed credentials; deleting a file does not remove Git history.

Public CI uses GitHub-hosted runners only, read-only repository access and no deployment secrets. Never run untrusted PR code on internal runners. Production deployment, actual device test evidence and commercial customer data belong in a separate private environment. Synthetic fixtures only.

Use a dedicated database, TLS for remote database connections and 32 cryptographically random bytes per device token. Store only SHA-256 digests. Rotation and revocation require operator authorization. The device API only reads its own latest fix. HTTP errors are generic. Bind to loopback only.

Remaining gates: least-privilege DB roles/RLS, user authentication, distributed rate limits, backup/restore, crash recovery, audit, TLS termination, vulnerability scanning, retention and physical hardware validation.
