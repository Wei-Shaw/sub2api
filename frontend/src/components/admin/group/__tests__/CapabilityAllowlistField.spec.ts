import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";
import { defineComponent, h, ref } from "vue";

import CapabilityAllowlistField from "../CapabilityAllowlistField.vue";
import type { CapabilityAllowlist } from "@/types";
import {
  createCapabilityAllowlistDefaults,
  normalizeCapabilityAllowlistForPlatform,
  supportsCapabilityAllowlist,
} from "@/views/admin/groupsCapabilityAllowlist";
import en from "@/i18n/locales/en/admin/overview";
import zh from "@/i18n/locales/zh/admin/overview";

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

// 挂载一个 v-model 宿主，验证勾选 → 配置的完整流程。
const mountInteractive = (initial: CapabilityAllowlist) => {
  const config = ref(initial);
  const host = defineComponent({
    setup() {
      return () =>
        h(CapabilityAllowlistField, {
          config: config.value,
          platform: "openai",
          "onUpdate:config": (value: CapabilityAllowlist) => {
            config.value = value;
          },
        });
    },
  });
  return { wrapper: mount(host), config };
};

describe("CapabilityAllowlistField", () => {
  it("hides the selectors while disabled and emits enabled on toggle", async () => {
    const wrapper = mount(CapabilityAllowlistField, {
      props: { config: createCapabilityAllowlistDefaults(), platform: "openai" },
    });
    expect(wrapper.find('[data-testid="capability-allowlist-cyber-daybreak_blue"]').exists()).toBe(false);
    expect(wrapper.text()).toContain("admin.groups.capabilityAllowlist.disabledHint");

    await wrapper.find('[data-testid="capability-allowlist-toggle"]').trigger("click");
    const emitted = wrapper.emitted("update:config");
    expect(emitted).toHaveLength(1);
    expect(emitted?.[0]?.[0]).toEqual({ enabled: true, cyber_programs: [], service_tiers: [] });
  });

  it("produces {priority} and {daybreak_blue} after checking Fast and Blue, and clears on uncheck", async () => {
    const { wrapper, config } = mountInteractive({ ...createCapabilityAllowlistDefaults(), enabled: true });
    expect(wrapper.text()).toContain("admin.groups.capabilityAllowlist.daybreakHint");

    await wrapper.find('[data-testid="capability-allowlist-tier-priority"]').setValue(true);
    await wrapper.find('[data-testid="capability-allowlist-cyber-daybreak_blue"]').setValue(true);
    expect(config.value).toEqual({ enabled: true, cyber_programs: ["daybreak_blue"], service_tiers: ["priority"] });

    await wrapper.find('[data-testid="capability-allowlist-cyber-daybreak_red"]').setValue(true);
    expect(config.value.cyber_programs).toEqual(["daybreak_blue", "daybreak_red"]);

    await wrapper.find('[data-testid="capability-allowlist-cyber-daybreak_blue"]').setValue(false);
    await wrapper.find('[data-testid="capability-allowlist-tier-priority"]').setValue(false);
    expect(config.value).toEqual({ enabled: true, cyber_programs: ["daybreak_red"], service_tiers: [] });

    // 关闭总开关保留集合（再次开启不丢选择），提交时由后端按 enabled 判定。
    await wrapper.find('[data-testid="capability-allowlist-toggle"]').trigger("click");
    expect(config.value.enabled).toBe(false);
    expect(config.value.cyber_programs).toEqual(["daybreak_red"]);
  });

  it("only applies to openai and composite groups and clears on other platforms", () => {
    expect(supportsCapabilityAllowlist("openai")).toBe(true);
    expect(supportsCapabilityAllowlist("composite")).toBe(true);
    expect(supportsCapabilityAllowlist("anthropic")).toBe(false);
    const enabled: CapabilityAllowlist = { enabled: true, cyber_programs: ["daybreak_blue"], service_tiers: ["priority"] };
    expect(normalizeCapabilityAllowlistForPlatform("anthropic", enabled)).toEqual(createCapabilityAllowlistDefaults());
    expect(normalizeCapabilityAllowlistForPlatform("composite", enabled)).toEqual(enabled);
    expect(normalizeCapabilityAllowlistForPlatform("openai", enabled)).not.toBe(enabled);
  });

  it("provides localized labels in both locales", () => {
    for (const locale of [zh, en]) {
      expect(locale.groups.capabilityAllowlist).toMatchObject({
        title: expect.any(String),
        hint: expect.stringContaining("capability_not_allowed"),
        daybreakHint: expect.stringContaining("Blue"),
        programs: { daybreak_blue: expect.any(String), daybreak_red: expect.any(String) },
        tiers: { priority: expect.stringContaining("priority"), ultrafast: expect.any(String) },
      });
    }
  });
});
