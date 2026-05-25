import { describe, expect, it } from "vitest";
import {
  PROFILE_TOGGLE_PRESETS,
  categoryDefaultProfile,
  deriveUpstreamCategory,
  overrideCount,
  resolveUpstreamPassthroughPolicy,
} from "@/utils/upstreamPassthrough";
import type {
  AccountUpstreamPassthroughOverride,
  UpstreamPassthroughDefaults,
} from "@/api/admin/settings";

describe("upstreamPassthrough TS resolver — parity with Go backend", () => {
  // ============================================================
  // deriveUpstreamCategory (5 rules + override + nil-safe)
  // ============================================================
  describe("deriveUpstreamCategory", () => {
    it("returns official for null account (defensive)", () => {
      expect(deriveUpstreamCategory(null)).toBe("official");
      expect(deriveUpstreamCategory(undefined)).toBe("official");
    });

    it("respects valid category_override (rule 0)", () => {
      expect(
        deriveUpstreamCategory({
          type: "upstream",
          extra: { upstream_passthrough: { category_override: "official" } },
        })
      ).toBe("official");
    });

    it("ignores invalid category_override and falls through", () => {
      expect(
        deriveUpstreamCategory({
          type: "upstream",
          extra: {
            // @ts-expect-error intentional invalid value
            upstream_passthrough: { category_override: "garbage" },
          },
        })
      ).toBe("relay"); // falls back to rule 1
    });

    it("rule 1: type=upstream → relay", () => {
      expect(deriveUpstreamCategory({ type: "upstream" })).toBe("relay");
    });

    it("rule 2: platform=antigravity → reverse", () => {
      expect(
        deriveUpstreamCategory({ type: "api_key", platform: "antigravity" })
      ).toBe("reverse");
    });

    it("rule 2: platform=kiro → reverse", () => {
      expect(
        deriveUpstreamCategory({ type: "api_key", platform: "kiro" })
      ).toBe("reverse");
    });

    it("rule 3: setup_token type → reverse", () => {
      expect(deriveUpstreamCategory({ type: "setup_token" })).toBe("reverse");
    });

    it("rule 4: OAuth on Anthropic → reverse", () => {
      expect(
        deriveUpstreamCategory({ type: "oauth", platform: "anthropic" })
      ).toBe("reverse");
    });

    it("rule 4: OAuth on OpenAI → reverse", () => {
      expect(
        deriveUpstreamCategory({ type: "oauth", platform: "openai" })
      ).toBe("reverse");
    });

    it("rule 4 boundary: OAuth on other platform (e.g. gemini) → official", () => {
      expect(
        deriveUpstreamCategory({ type: "oauth", platform: "gemini" })
      ).toBe("official");
    });

    it("rule 5: fallback for unknown combos → official", () => {
      expect(deriveUpstreamCategory({ type: "api_key" })).toBe("official");
      expect(
        deriveUpstreamCategory({ type: "api_key", platform: "anthropic" })
      ).toBe("official");
    });
  });

  // ============================================================
  // categoryDefaultProfile
  // ============================================================
  describe("categoryDefaultProfile", () => {
    it("relay → transparent", () => {
      expect(categoryDefaultProfile("relay")).toBe("transparent");
    });
    it("official → protected", () => {
      expect(categoryDefaultProfile("official")).toBe("protected");
    });
    it("reverse → strict", () => {
      expect(categoryDefaultProfile("reverse")).toBe("strict");
    });
  });

  // ============================================================
  // Profile presets — values must match Go's profileTogglesByName
  // ============================================================
  describe("PROFILE_TOGGLE_PRESETS", () => {
    it("transparent: all 7 true", () => {
      expect(PROFILE_TOGGLE_PRESETS.transparent).toEqual({
        forward_client_headers: true,
        forward_user_network_info: true,
        skip_body_scrub: true,
        skip_system_prompt_inject: true,
        forward_client_ua: true,
        forward_beta_flags: true,
        skip_model_rewrite: true,
      });
    });

    it("protected: only skip_system_prompt_inject true (user owns system prompt)", () => {
      expect(PROFILE_TOGGLE_PRESETS.protected).toEqual({
        forward_client_headers: false,
        forward_user_network_info: false,
        skip_body_scrub: false,
        skip_system_prompt_inject: true,
        forward_client_ua: false,
        forward_beta_flags: false,
        skip_model_rewrite: false,
      });
    });

    it("strict: all 7 false (mimic must inject everything)", () => {
      expect(PROFILE_TOGGLE_PRESETS.strict).toEqual({
        forward_client_headers: false,
        forward_user_network_info: false,
        skip_body_scrub: false,
        skip_system_prompt_inject: false,
        forward_client_ua: false,
        forward_beta_flags: false,
        skip_model_rewrite: false,
      });
    });
  });

  // ============================================================
  // resolveUpstreamPassthroughPolicy — 7-layer precedence
  // ============================================================
  describe("resolveUpstreamPassthroughPolicy", () => {
    it("kill switch force_strict short-circuits everything", () => {
      const r = resolveUpstreamPassthroughPolicy(
        { type: "upstream" }, // would derive to relay → transparent normally
        null,
        "force_strict"
      );
      expect(r.profileApplied).toBe("strict");
      expect(r.globalOverrideActive).toBe(true);
      expect(r.toggles).toEqual(PROFILE_TOGGLE_PRESETS.strict);
      expect(r.category).toBe("relay"); // still recorded for logs
    });

    it("kill switch force_transparent overrides protected account", () => {
      const r = resolveUpstreamPassthroughPolicy(
        { type: "api_key", platform: "anthropic" },
        null,
        "force_transparent"
      );
      expect(r.profileApplied).toBe("transparent");
      expect(r.toggles).toEqual(PROFILE_TOGGLE_PRESETS.transparent);
      expect(r.globalOverrideActive).toBe(true);
    });

    it("auto kill switch → per-category default", () => {
      const r = resolveUpstreamPassthroughPolicy(
        { type: "api_key", platform: "anthropic" },
        null,
        "auto"
      );
      expect(r.category).toBe("official");
      expect(r.profileApplied).toBe("protected");
      expect(r.globalOverrideActive).toBe(false);
    });

    it("nil defaults + nil override → uses code-baked defaults", () => {
      const r = resolveUpstreamPassthroughPolicy(
        { type: "upstream" },
        null,
        null
      );
      expect(r.category).toBe("relay");
      expect(r.profileApplied).toBe("transparent");
    });

    it("system defaults override code-baked defaults per category", () => {
      const defaults: UpstreamPassthroughDefaults = {
        relay: { profile: "transparent" },
        official: { profile: "protected", overrides: { skip_body_scrub: true } },
        reverse: { profile: "strict" },
      };
      const r = resolveUpstreamPassthroughPolicy(
        { type: "api_key", platform: "anthropic" },
        defaults,
        "auto"
      );
      // protected base + skip_body_scrub=true from system-default override
      expect(r.toggles.skip_body_scrub).toBe(true);
      expect(r.toggles.forward_client_headers).toBe(false); // protected default unchanged
    });

    it("account profile override replaces category slot (no inherit of system overrides)", () => {
      const defaults: UpstreamPassthroughDefaults = {
        relay: { profile: "transparent" },
        official: { profile: "protected", overrides: { skip_body_scrub: true } },
        reverse: { profile: "strict" },
      };
      const acctOverride: AccountUpstreamPassthroughOverride = {
        profile: "strict",
      };
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "api_key",
          platform: "anthropic",
          extra: { upstream_passthrough: acctOverride },
        },
        defaults,
        "auto"
      );
      expect(r.profileApplied).toBe("strict");
      // System-default's skip_body_scrub=true override is NOT inherited because
      // the account picked a different profile.
      expect(r.toggles.skip_body_scrub).toBe(false);
    });

    it("account sparse overrides win over profile preset", () => {
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "upstream",
          extra: {
            upstream_passthrough: {
              overrides: { skip_model_rewrite: false },
            },
          },
        },
        null,
        "auto"
      );
      // Transparent preset has skip_model_rewrite=true; account override flips it
      expect(r.profileApplied).toBe("transparent");
      expect(r.toggles.skip_model_rewrite).toBe(false);
      expect(r.toggles.forward_client_headers).toBe(true);
    });

    it("legacy anthropic_passthrough=true sets 3 toggles when no new fields", () => {
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "api_key",
          platform: "anthropic",
          extra: { anthropic_passthrough: true },
        },
        null,
        "auto"
      );
      // Category=official → profile=protected base. Legacy bumps three toggles:
      expect(r.toggles.forward_client_headers).toBe(true);
      expect(r.toggles.forward_beta_flags).toBe(true);
      expect(r.toggles.skip_body_scrub).toBe(true);
      // Conservative defaults preserved:
      expect(r.toggles.forward_user_network_info).toBe(false);
    });

    it("legacy fallback is suppressed when any new field is set", () => {
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "api_key",
          platform: "anthropic",
          extra: {
            anthropic_passthrough: true,
            upstream_passthrough: { profile: "strict" },
          },
        },
        null,
        "auto"
      );
      // profile=strict (which has all-false), legacy must NOT bump anything
      expect(r.profileApplied).toBe("strict");
      expect(r.toggles.forward_client_headers).toBe(false);
      expect(r.toggles.skip_body_scrub).toBe(false);
    });

    it("category_override forces a different category slot", () => {
      const defaults: UpstreamPassthroughDefaults = {
        relay: { profile: "transparent" },
        official: { profile: "protected" },
        reverse: { profile: "strict" },
      };
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "api_key",
          platform: "anthropic", // would be official
          extra: {
            upstream_passthrough: { category_override: "relay" },
          },
        },
        defaults,
        "auto"
      );
      expect(r.category).toBe("relay");
      expect(r.profileApplied).toBe("transparent");
    });
  });

  describe("overrideCount", () => {
    it("returns 0 when toggles match preset exactly", () => {
      const r = resolveUpstreamPassthroughPolicy(
        { type: "upstream" },
        null,
        "auto"
      );
      expect(overrideCount(r)).toBe(0);
    });

    it("counts each flipped toggle vs preset", () => {
      const r = resolveUpstreamPassthroughPolicy(
        {
          type: "upstream",
          extra: {
            upstream_passthrough: {
              overrides: {
                skip_model_rewrite: false,
                forward_client_headers: false,
              },
            },
          },
        },
        null,
        "auto"
      );
      expect(overrideCount(r)).toBe(2);
    });
  });
});
