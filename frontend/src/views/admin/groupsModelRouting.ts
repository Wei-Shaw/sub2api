// 分组模型路由（model_routing）的平台适用范围与账号候选过滤。
//
// 路由规则本身与平台无关，但只有接线过的调度栈才会读取它：
//   - anthropic / gemini / antigravity 走通用网关 GatewayService
//   - openai / grok / kimi / zhipu / deepseek / minimax 走 OpenAIGatewayService
//   - composite 按解析出的目标平台落到上述两者之一
// 未接线的平台不展示该配置块，避免出现"规则能存能显示但不生效"的界面。

export const MODEL_ROUTING_PLATFORMS = [
  "anthropic",
  "openai",
  "gemini",
  "antigravity",
  "grok",
  "kimi",
  "zhipu",
  "deepseek",
  "minimax",
  "composite",
] as const;

export const supportsModelRoutingPlatform = (platform: string): boolean =>
  (MODEL_ROUTING_PLATFORMS as readonly string[]).includes(platform);

export interface ModelRoutingAccountSearchFilters {
  search: string;
  platform?: string;
  group?: string;
}

/**
 * 路由规则的账号候选过滤条件。
 *
 * - 平台：与分组平台一致。composite 分组的账号本就跨平台，不施加平台过滤；
 *   其余平台按分组平台收敛，避免选到调度根本不会考虑的账号。这里不放宽跨平台
 *   调度权限——候选账号最终仍由后端的分组归属与平台过滤决定。
 * - 分组：仅在编辑既有分组时可用。创建分组时还没有分组 ID，只能按平台过滤，
 *   此时选到未绑定该分组的账号，保存后那条规则不会命中任何候选。
 */
export const modelRoutingAccountSearchFilters = (
  keyword: string,
  platform: string,
  groupId: number | null,
): ModelRoutingAccountSearchFilters => {
  const filters: ModelRoutingAccountSearchFilters = { search: keyword };
  if (platform && platform !== "composite") {
    filters.platform = platform;
  }
  if (groupId != null && groupId > 0) {
    filters.group = String(groupId);
  }
  return filters;
};
