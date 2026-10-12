# BUAGA v0.2.8 merge notes

Base: production `4263dac5edc6d645266eb4becfc249bf45805778`.
Upstream: stable `v0.2.8`, `fd80b08c90b55edcad5b00171b53f08721d30da1`.

- Preserve custom Grok video ownership, durable task lookup and atomic billing instead of replacing them with upstream handlers.
- Preserve DeepSeek reasoning replay in Chat, Messages and Responses routes, cancellation checks, profit controls and account exclusion loops.
- Merge upstream sticky-hit observability, Gemini transport failover and final-request effort extraction into those custom paths.
- Preserve the custom display-only model list. Upstream allowlist validation must not be applied to the display-only list. Keep the existing backend allowlist validation.
- Keep the custom manifest controls and expose their checked state for the upstream interaction regression test.
- Keep BUAGA's real 5xx classification and temporary cooldown; adapt the new upstream test's generic-502 expectation. Authentication classification remains false for a 500.
- Keep the deleted upstream deployment entry points deleted; only the authoritative Git archive release chain is supported. Preserve the custom attribution document.
- Merge the additive plugin protocol and scoped account directory; only declared OpenAI OAuth outbound capability receives that account scope.

Only three new migrations are allowed. `upgrade-v028-database.py` performs an online backup, restores schema and affected configuration into an isolated database, rehearses the exact migrations and compares normalized prices plus all group/channel/settings fields. Production uses NOWAIT locks and short statement timeouts, compares configuration again before committing and records the exact migration checksums. Existing migrations are immutable. Old application rollback keeps the additive schema and never restores the database over live writes.

`upgrade-v028-deploy.py` requires the migration proof, test proof tied to the release commit, verified custom base image and additive static assets before a graceful Nginx switch. The old application remains running for active requests and rollback. The pricing monitor must be rebound and verified against the new application.
