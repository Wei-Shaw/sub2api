/**
 * Upstream passthrough policy TS Resolver — frontend mirror of the Go pure
 * function at backend/internal/service/upstream_passthrough_resolver.go.
 *
 * Used by the account modal to render a live preview of the effective policy
 * as the admin tweaks per-account category override / profile / sparse
 * toggle overrides. Output MUST match the Go side toggle-by-toggle.
 *
 * Resolution order (high → low precedence):
 *   1. Global kill switch (force_* short-circuits everything)
 *   2. Account sparse per-toggle overrides
 *   3. Account profile override
 *   4. Account category_override → picks which slot of system defaults to use
 *   5. System defaults for derived/overridden category
 *   6. Legacy fields (anthropic_passthrough / openai_passthrough)
 *   7. Code constants (profile presets)
 */

import type {
  AccountUpstreamPassthroughOverride,
  UpstreamCategory,
  UpstreamPassthroughDefaults,
  UpstreamPassthroughGlobalOverride,
  UpstreamPassthroughOverrides,
  UpstreamPassthroughProfile,
  UpstreamPassthroughToggleKey,
} from "@/api/admin/settings";
import { UPSTREAM_PASSTHROUGH_TOGGLE_KEYS } from "@/api/admin/settings";

export type PassthroughToggles = Record<UpstreamPassthroughToggleKey, boolean>;

/** Profile preset table — must match backend's profileTogglesByName. */
export const PROFILE_TOGGLE_PRESETS: Record<
  UpstreamPassthroughProfile,
  PassthroughToggles
> = {
  transparent: {
    forward_client_headers: true,
    forward_user_network_info: true,
    skip_body_scrub: true,
    skip_system_prompt_inject: true,
    forward_client_ua: true,
    forward_beta_flags: true,
    skip_model_rewrite: true,
  },
  protected: {
    forward_client_headers: false,
    forward_user_network_info: false,
    skip_body_scrub: false,
    // Protected only differs from Strict by skipping the CC system inject —
    // official-direct users own their system prompt.
    skip_system_prompt_inject: true,
    forward_client_ua: false,
    forward_beta_flags: false,
    skip_model_rewrite: false,
  },
  strict: {
    forward_client_headers: false,
    forward_user_network_info: false,
    skip_body_scrub: false,
    skip_system_prompt_inject: false,
    forward_client_ua: false,
    forward_beta_flags: false,
    skip_model_rewrite: false,
  },
};

/** Recommended default profile per category. Mirrors CategoryDefaultProfile. */
export function categoryDefaultProfile(
  category: UpstreamCategory
): UpstreamPassthroughProfile {
  switch (category) {
    case "relay":
      return "transparent";
    case "official":
      return "protected";
    case "reverse":
      return "strict";
  }
}

/**
 * Lightweight account shape used for category derivation. Mirror of the Go
 * Account fields read by DeriveUpstreamCategory + GetUpstreamPassthroughCategoryOverride.
 */
export interface AccountShapeForResolver {
  type?: string;
  platform?: string;
  extra?: {
    upstream_passthrough?: AccountUpstreamPassthroughOverride;
    anthropic_passthrough?: boolean;
    openai_passthrough?: boolean;
  } & Record<string, unknown>;
}

// Backend account constants (mirrored). Kept inline to avoid pulling in larger
// account type packages; values must stay in sync with backend/internal/domain/constants.go.
const ACCOUNT_TYPE_UPSTREAM = "upstream";
const ACCOUNT_TYPE_OAUTH = "oauth";
const ACCOUNT_TYPE_SETUP_TOKEN = "setup_token";
const PLATFORM_ANTHROPIC = "anthropic";
const PLATFORM_OPENAI = "openai";
const PLATFORM_ANTIGRAVITY = "antigravity";
const PLATFORM_KIRO = "kiro";

const VALID_CATEGORIES: readonly UpstreamCategory[] = [
  "relay",
  "official",
  "reverse",
];

/**
 * Pure function counterpart of Go's DeriveUpstreamCategory. Order matters:
 * an account.extra.upstream_passthrough.category_override wins; otherwise
 * the 5 rules in order; default → "official".
 */
export function deriveUpstreamCategory(
  account: AccountShapeForResolver | null | undefined
): UpstreamCategory {
  if (!account) {
    return "official";
  }

  const override = account.extra?.upstream_passthrough?.category_override;
  if (override && VALID_CATEGORIES.includes(override as UpstreamCategory)) {
    return override as UpstreamCategory;
  }

  if (account.type === ACCOUNT_TYPE_UPSTREAM) {
    return "relay";
  }
  if (
    account.platform === PLATFORM_ANTIGRAVITY ||
    account.platform === PLATFORM_KIRO
  ) {
    return "reverse";
  }
  if (account.type === ACCOUNT_TYPE_SETUP_TOKEN) {
    return "reverse";
  }
  if (account.type === ACCOUNT_TYPE_OAUTH) {
    if (
      account.platform === PLATFORM_ANTHROPIC ||
      account.platform === PLATFORM_OPENAI
    ) {
      return "reverse";
    }
  }
  return "official";
}

/**
 * Pick a category's slot out of UpstreamPassthroughDefaults. Empty profile in
 * the slot is filled with the category's recommended default profile.
 */
export function categorySlot(
  defaults: UpstreamPassthroughDefaults | null | undefined,
  category: UpstreamCategory
): {
  profile: UpstreamPassthroughProfile;
  overrides: UpstreamPassthroughOverrides;
} {
  const raw =
    defaults && (category === "relay"
      ? defaults.relay
      : category === "official"
      ? defaults.official
      : defaults.reverse);
  const profile = raw?.profile || categoryDefaultProfile(category);
  return {
    profile,
    overrides: raw?.overrides ?? {},
  };
}

/**
 * Apply a sparse overrides map onto a toggle preset. Unknown keys are silently
 * ignored (matches Go's Apply()).
 */
function applyOverrides(
  base: PassthroughToggles,
  overrides: UpstreamPassthroughOverrides | undefined | null
): PassthroughToggles {
  if (!overrides) {
    return base;
  }
  const out: PassthroughToggles = { ...base };
  for (const key of UPSTREAM_PASSTHROUGH_TOGGLE_KEYS) {
    if (Object.prototype.hasOwnProperty.call(overrides, key)) {
      const v = overrides[key];
      if (typeof v === "boolean") {
        out[key] = v;
      }
    }
  }
  return out;
}

/**
 * Apply the legacy *_passthrough fields as a last-resort fallback. Mirrors Go's
 * applyLegacyFallback: only fires when there is no profile/category/system override.
 *
 * Legacy "passthrough=true" historically meant: forward client headers, forward
 * beta flags, skip body scrub. It does NOT forward user network info or skip
 * system inject — keep those conservative defaults from the profile.
 */
function applyLegacyFallback(
  toggles: PassthroughToggles,
  account: AccountShapeForResolver | null | undefined,
  accountProfile: UpstreamPassthroughProfile | "",
  accountOverrides: UpstreamPassthroughOverrides | undefined,
  systemDefaultOverrides: UpstreamPassthroughOverrides | undefined
): PassthroughToggles {
  if (!account) {
    return toggles;
  }
  if (accountProfile !== "") {
    return toggles;
  }
  if (accountOverrides && Object.keys(accountOverrides).length > 0) {
    return toggles;
  }
  if (
    systemDefaultOverrides &&
    Object.keys(systemDefaultOverrides).length > 0
  ) {
    return toggles;
  }
  const legacyAnthropic = account.extra?.anthropic_passthrough === true;
  const legacyOpenAI = account.extra?.openai_passthrough === true;
  if (!legacyAnthropic && !legacyOpenAI) {
    return toggles;
  }
  return {
    ...toggles,
    forward_client_headers: true,
    forward_beta_flags: true,
    skip_body_scrub: true,
  };
}

export interface ResolvedUpstreamPolicy {
  toggles: PassthroughToggles;
  category: UpstreamCategory;
  profileApplied: UpstreamPassthroughProfile;
  globalOverrideActive: boolean;
}

/**
 * Compute the effective 7-toggle policy for a given account + system defaults +
 * kill-switch mode. Pure function; safe to call from Vue reactivity.
 *
 * MUST match backend/internal/service/upstream_passthrough_resolver.go's
 * ResolveUpstreamPassthroughPolicy semantically. Differences here vs there are
 * bugs; please update both sides together.
 */
export function resolveUpstreamPassthroughPolicy(
  account: AccountShapeForResolver | null | undefined,
  defaults: UpstreamPassthroughDefaults | null | undefined,
  globalOverride: UpstreamPassthroughGlobalOverride | string | null | undefined
): ResolvedUpstreamPolicy {
  // Step 1: kill switch short-circuit
  const normalizedOverride = (globalOverride ?? "auto") as
    | UpstreamPassthroughGlobalOverride
    | string;
  let forcedProfile: UpstreamPassthroughProfile | "" = "";
  if (normalizedOverride === "force_transparent") forcedProfile = "transparent";
  else if (normalizedOverride === "force_protected") forcedProfile = "protected";
  else if (normalizedOverride === "force_strict") forcedProfile = "strict";

  const category = deriveUpstreamCategory(account);

  if (forcedProfile !== "") {
    return {
      toggles: { ...PROFILE_TOGGLE_PRESETS[forcedProfile] },
      category,
      profileApplied: forcedProfile,
      globalOverrideActive: true,
    };
  }

  // Step 2: pick category default
  const slot = categorySlot(defaults, category);

  // Step 3: account profile override replaces category slot's profile
  // (but does NOT inherit system-default overrides — matches Go behavior)
  const accountProfile = account?.extra?.upstream_passthrough?.profile ?? "";
  let appliedProfile: UpstreamPassthroughProfile;
  let systemDefaultOverrides: UpstreamPassthroughOverrides | undefined;
  if (
    accountProfile === "transparent" ||
    accountProfile === "protected" ||
    accountProfile === "strict"
  ) {
    appliedProfile = accountProfile;
    systemDefaultOverrides = undefined;
  } else {
    appliedProfile = slot.profile;
    systemDefaultOverrides = slot.overrides;
  }

  // Step 4: start from profile toggles
  let toggles: PassthroughToggles = { ...PROFILE_TOGGLE_PRESETS[appliedProfile] };

  // Step 5: layer system-default per-toggle overrides
  toggles = applyOverrides(toggles, systemDefaultOverrides);

  // Step 6: layer account sparse per-toggle overrides
  const accountOverrides = account?.extra?.upstream_passthrough?.overrides;
  toggles = applyOverrides(toggles, accountOverrides);

  // Step 7: legacy fallback ONLY when nothing above contributed
  toggles = applyLegacyFallback(
    toggles,
    account,
    accountProfile,
    accountOverrides,
    systemDefaultOverrides
  );

  return {
    toggles,
    category,
    profileApplied: appliedProfile,
    globalOverrideActive: false,
  };
}

/** Count how many toggles differ from the resolved profile's preset. */
export function overrideCount(resolved: ResolvedUpstreamPolicy): number {
  const preset = PROFILE_TOGGLE_PRESETS[resolved.profileApplied];
  let n = 0;
  for (const key of UPSTREAM_PASSTHROUGH_TOGGLE_KEYS) {
    if (resolved.toggles[key] !== preset[key]) {
      n++;
    }
  }
  return n;
}
