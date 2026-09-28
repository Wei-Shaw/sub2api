import { describe, expect, it } from "vitest";

import {
  modelRoutingAccountSearchFilters,
  supportsModelRoutingPlatform,
} from "../groupsModelRouting";

describe("supportsModelRoutingPlatform", () => {
  it("放行已接线的通用网关平台", () => {
    for (const platform of ["anthropic", "gemini", "antigravity"]) {
      expect(supportsModelRoutingPlatform(platform)).toBe(true);
    }
  });

  it("放行走 OpenAI 调度栈的平台", () => {
    for (const platform of [
      "openai",
      "grok",
      "kimi",
      "zhipu",
      "deepseek",
      "minimax",
    ]) {
      expect(supportsModelRoutingPlatform(platform)).toBe(true);
    }
  });

  it("放行 composite（按解析出的目标平台落到两套调度之一）", () => {
    expect(supportsModelRoutingPlatform("composite")).toBe(true);
  });

  it("不放行未接线的平台，避免出现存了不生效的规则", () => {
    for (const platform of ["kiro", "", "unknown"]) {
      expect(supportsModelRoutingPlatform(platform)).toBe(false);
    }
  });
});

describe("modelRoutingAccountSearchFilters", () => {
  it("按分组平台收敛候选", () => {
    expect(modelRoutingAccountSearchFilters("acc", "openai", null)).toEqual({
      search: "acc",
      platform: "openai",
    });
  });

  it("composite 分组不施加平台过滤：其账号本就跨平台", () => {
    expect(modelRoutingAccountSearchFilters("acc", "composite", 12)).toEqual({
      search: "acc",
      group: "12",
    });
  });

  it("编辑既有分组时按分组过滤，避免选到不属于该分组的账号", () => {
    expect(modelRoutingAccountSearchFilters("acc", "anthropic", 42)).toEqual({
      search: "acc",
      platform: "anthropic",
      group: "42",
    });
  });

  it("创建分组时还没有分组 ID，只按平台过滤", () => {
    for (const groupId of [null, 0]) {
      expect(modelRoutingAccountSearchFilters("acc", "gemini", groupId)).toEqual(
        {
          search: "acc",
          platform: "gemini",
        },
      );
    }
  });
});
