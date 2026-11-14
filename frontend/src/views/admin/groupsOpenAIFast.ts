export function supportsGroupOpenAIFast(platform: string): boolean {
  return platform === "openai" || platform === "composite";
}

export function normalizeGroupOpenAIFast(
  platform: string,
  enabled: boolean,
): boolean {
  return supportsGroupOpenAIFast(platform) && enabled;
}

// 分组强制的 OpenAI 服务档位。后端用两个互斥的布尔列表达，表单用单选避免
// 同时勾选两者（后端会以 400 拒绝）。
export type GroupOpenAIForcedTier = "client" | "priority" | "ultrafast";

interface GroupOpenAIForcedTierFlags {
  force_openai_fast?: boolean;
  force_openai_ultrafast?: boolean;
}

export function groupOpenAIForcedTier(
  flags: GroupOpenAIForcedTierFlags,
): GroupOpenAIForcedTier {
  if (flags.force_openai_ultrafast) return "ultrafast";
  return flags.force_openai_fast ? "priority" : "client";
}

export function setGroupOpenAIForcedTier(
  flags: GroupOpenAIForcedTierFlags,
  tier: unknown,
): void {
  flags.force_openai_fast = tier === "priority";
  flags.force_openai_ultrafast = tier === "ultrafast";
}
