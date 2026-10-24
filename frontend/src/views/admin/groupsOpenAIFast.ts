import type { ForcedServiceTier } from "@/types";

export function supportsGroupOpenAIFast(platform: string): boolean {
  return platform === "openai" || platform === "composite";
}

export function normalizeGroupOpenAIFast(
  platform: string,
  enabled: boolean,
): boolean {
  return supportsGroupOpenAIFast(platform) && enabled;
}

export function normalizeForcedServiceTier(
  platform: string,
  tier: ForcedServiceTier,
): ForcedServiceTier {
  return supportsGroupOpenAIFast(platform) ? tier : "";
}
