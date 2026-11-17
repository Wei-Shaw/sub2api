# Account Gateway ownership rules

This fork is the single owner of provider logic for Account Gateway.

- Provider HTTP, parsing, protocol codecs, model catalogs and provider-specific request admission live here. The Gateway (`agent-teams-ai/account-gateway`) keeps contracts, identity, fences and policy; ReviewRouter consumes contracts and keeps repository/model policy and run admission.
- Reuse existing Sub2API code (`backend/internal/pkg/*`, `backend/internal/service/*`) before writing new code. The Gateway and ReviewRouter must not copy provider logic; if a capability is missing, extend this fork.
- Each protected profile has exactly one model catalog source here: a public provider catalogue (OpenRouter), a provider read bound to the account (Codex), or a static lower bound plus the provider's own `/models` list: read through the existing quota credential lease for the API-key coding plans (GLM, MiniMax, Kimi), and read without any key for NVIDIA Build, whose list is public. For these profiles the provider's list decides by owner choice, filtered in one place to models that can review code and bounded to 512 models; a model joins the static lower bound only with evidence it belongs to the plan.
- A key is decoded only by the private dispatch seam and the read-only quota lease. Dispatch never reads the network and never takes the lease: it admits the anchor, the static models or the last good live list, read from memory.

Design and rationale: `ai-docs/architecture/64-gateway-native-ownership-and-model-catalog.md` and, for the live lists, `65-live-coding-plan-model-catalog.md` and `68-nvidia-live-model-catalog.md` in the ReviewRouter repository (`777genius/review-router-ai`).
