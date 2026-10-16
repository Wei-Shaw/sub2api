import type { CapabilityAllowlist } from "@/types";
import { supportsGroupOpenAIFast } from "./groupsOpenAIFast";

export const CAPABILITY_CYBER_PROGRAMS = ["daybreak_blue", "daybreak_red"] as const;
export const CAPABILITY_SERVICE_TIERS = ["priority", "ultrafast"] as const;

export function createCapabilityAllowlistDefaults(): CapabilityAllowlist {
  return { enabled: false, cyber_programs: [], service_tiers: [] };
}

export function cloneCapabilityAllowlist(
  source: CapabilityAllowlist | null | undefined,
): CapabilityAllowlist {
  if (!source) return createCapabilityAllowlistDefaults();
  return {
    enabled: source.enabled ?? false,
    cyber_programs: [...(source.cyber_programs ?? [])],
    service_tiers: [...(source.service_tiers ?? [])],
  };
}

/** 能力白名单只对 openai / composite 分组生效（与 OpenAI Fast 同一平台谓词）。 */
export function supportsCapabilityAllowlist(platform: string): boolean {
  return supportsGroupOpenAIFast(platform);
}

/** 非 openai / composite 平台提交关闭且清空的配置，与后端归一化一致。 */
export function normalizeCapabilityAllowlistForPlatform(
  platform: string,
  config: CapabilityAllowlist,
): CapabilityAllowlist {
  if (!supportsCapabilityAllowlist(platform)) {
    return createCapabilityAllowlistDefaults();
  }
  return cloneCapabilityAllowlist(config);
}
