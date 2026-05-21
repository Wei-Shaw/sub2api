# LDAP Sync Policies

Read this file when you need merge priorities or release guardrails beyond the short rules in `SKILL.md`.

## Merge Priorities

1. Start from latest official upstream.
2. Re-apply only the LDAP customization required for this fork to remain functional and releasable.
3. Do not preserve unrelated fork-only changes unless the user explicitly asks for them.
4. When upstream and LDAP behavior conflict, prefer the LDAP-required behavior.

## Required Fork Behavior

- Preserve the Gemini customization that disables platform-side Gemini rate limiting.
- Upstream 429 passthrough may remain, but local Gemini precheck, local persistence, scheduling exclusion, and ops-side "rate limited" display must stay disabled unless the user explicitly asks otherwise.
- This fork must not guide admins to self-update from upstream GitHub releases inside the web UI.
- Server-side upgrades remain script-driven.

## Deployment Documentation Constraints

- Keep `deploy/README_LDAP_ENTERPRISE.md` as the single LDAP operations document unless the user explicitly asks for a broader docs change.
- Do not edit upstream `README*.md` or `deploy/README.md` unless the user explicitly asks.
- `deploy/README_LDAP_ENTERPRISE.md` should stay copy-first and operationally minimal.
- Fresh deploy and upgrade flows should each keep a single-line command block that works as written.

## Repository Hygiene

- Start with a clean worktree.
- Do not commit backup files, caches, or package-manager noise.
- Commit generated artifacts if the sync flow regenerates them.
- Treat GitHub publication as part of the done state unless the user explicitly asks for local-only work.
- Push only when the resulting branch state actually changed.
