import type { OpenAIServiceTierPolicy } from "@/types";

interface GroupServiceTierFlags {
  force_openai_fast?: boolean;
  force_openai_ultrafast?: boolean;
}

export function supportsGroupOpenAIFast(platform: string): boolean {
  return platform === "openai" || platform === "composite";
}

export function normalizeGroupOpenAIFast(
  platform: string,
  enabled: boolean,
): boolean {
  return supportsGroupOpenAIFast(platform) && enabled;
}
export function groupOpenAIServiceTier(flags: GroupServiceTierFlags): OpenAIServiceTierPolicy {
  if (flags.force_openai_ultrafast) return "ultrafast";
  return flags.force_openai_fast ? "priority" : "client";
}

export function setGroupOpenAIServiceTier(
  form: GroupServiceTierFlags & { platform: string },
  tier: OpenAIServiceTierPolicy,
): void {
  form.force_openai_fast = normalizeGroupOpenAIFast(form.platform, tier === "priority");
  form.force_openai_ultrafast = normalizeGroupOpenAIFast(form.platform, tier === "ultrafast");
}
