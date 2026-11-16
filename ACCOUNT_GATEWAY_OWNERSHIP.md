# Account Gateway ownership rules

This fork is the single owner of provider logic for Account Gateway.

- Provider HTTP, parsing, protocol codecs, model catalogs and provider-specific request admission live here. The Gateway (`agent-teams-ai/account-gateway`) keeps contracts, identity, fences and policy; ReviewRouter consumes contracts and keeps repository/model policy and run admission.
- Reuse existing Sub2API code (`backend/internal/pkg/*`, `backend/internal/service/*`) before writing new code. The Gateway and ReviewRouter must not copy provider logic; if a capability is missing, extend this fork.
- Each protected profile has exactly one qualified model catalog source here: a table owned by this fork, a public provider catalogue (OpenRouter) or a provider read bound to the account (Codex). For the API-key coding plans, a model joins a static catalog only with evidence it belongs to the plan (provider documentation or real qualification); an upstream `/models` answer alone is not evidence.
- For the API-key coding plans and their static catalogs, the private dispatch seam and the existing read-only quota lease are the only places a key is decoded; catalogue reads of a static catalog never decrypt it.

Design and rationale: `ai-docs/architecture/64-gateway-native-ownership-and-model-catalog.md` in the ReviewRouter repository (`777genius/review-router-ai`).
