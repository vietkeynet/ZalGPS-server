# Agent instructions — ZalGPS

This repository is PUBLIC. Legacy Java/Traccar and Go V2 coexist. The active V2 delivery scope is v2/. Read [V2 agent guide](v2/AGENTS.md), [TODO](v2/TODO.md), [roadmap](v2/ROADMAP.md), [architecture](v2/docs/architecture.md), [deployment](v2/docs/deployment.md) and [security](SECURITY.md) before changes.

Never commit credentials, real GPS traces, device identifiers, internal hosts, private deployment instructions, scans or customer information. No untrusted PR execution on internal infrastructure. Only synthetic test data; private evidence stays outside Git. No automatic changes to visibility, secrets, branch rules, releases, production, other repositories, shared services or infrastructure.

Do not rewrite history, force-push, merge or deploy without approval and all relevant gates. Keep legacy compatibility and licenses. Work on one coherent task at a time; reuse the active development branch or existing PR; no speculative PRs. Do not modify unrelated Java files unless a specific approved compatibility task requires it. Read current branch head and worktree status before editing.
