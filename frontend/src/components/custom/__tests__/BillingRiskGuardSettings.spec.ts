import { describe, expect, it, vi } from "vitest";
import { flushPromises, mount } from "@vue/test-utils";

import {
  validateBillingRiskGuardSettings,
  type BillingRiskGuardSettings,
} from "@/api/admin/settings";
import BillingRiskGuardSettingsComponent from "../BillingRiskGuardSettings.vue";

vi.mock("vue-i18n", async (importOriginal) => {
  const actual = await importOriginal<typeof import("vue-i18n")>();
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

const defaults: BillingRiskGuardSettings = {
  billing_risk_enabled: true,
  billing_risk_low_balance_threshold: 10,
  billing_risk_safety_factor: 1.25,
  billing_risk_minimum_request_risk: 0.001,
  billing_risk_overdraft_allowance: 0.2,
  billing_risk_high_cost_trigger: 1,
  billing_risk_lease_ttl_seconds: 60,
  billing_risk_refresh_interval_seconds: 15,
  billing_risk_uncertain_cooldown_seconds: 300,
  billing_risk_video_lease_ttl_seconds: 86400,
  billing_risk_idle_balance_ttl_seconds: 120,
};

describe("BillingRiskGuardSettings", () => {
  it("只保留总开关，并回显和更新低余额阈值", async () => {
    const wrapper = mount(BillingRiskGuardSettingsComponent, {
      props: {
        modelValue: { ...defaults },
        "onUpdate:modelValue": async (value: BillingRiskGuardSettings) => {
          await wrapper.setProps({ modelValue: value });
        },
      },
      global: {
        stubs: {
          Toggle: {
            props: ["modelValue"],
            emits: ["update:modelValue"],
            template: '<button data-testid="billing-risk-enabled-toggle" @click="$emit(\'update:modelValue\', false)" />',
          },
        },
      },
    });

    expect(wrapper.get('[data-testid="billing-risk-low-balance-threshold"]').attributes("value")).toBe("10");
    expect(wrapper.find('[data-testid^="billing-risk-mode-"]').exists()).toBe(false);

    await wrapper.get('[data-testid="billing-risk-enabled-toggle"]').trigger("click");
    await wrapper.get('[data-testid="billing-risk-low-balance-threshold"]').setValue("8");
    await flushPromises();

    expect(wrapper.props("modelValue")).toMatchObject({
      billing_risk_enabled: false,
      billing_risk_low_balance_threshold: 8,
    });
  });

  it("为每个数值参数显示易懂说明", () => {
    const wrapper = mount(BillingRiskGuardSettingsComponent, {
      props: { modelValue: { ...defaults } },
      global: { stubs: { Toggle: true } },
    });

    const hintKeys = [
      "lowBalanceThreshold",
      "safetyFactor",
      "minimumRequestRisk",
      "overdraftAllowance",
      "highCostTrigger",
      "leaseTTL",
      "refreshInterval",
      "uncertainCooldown",
      "videoLeaseTTL",
      "idleBalanceTTL",
    ];
    for (const key of hintKeys) {
      expect(wrapper.get(`[data-testid="billing-risk-${key}-hint"]`).text()).toBe(
        `admin.settings.billingRisk.fields.${key}Hint`,
      );
    }
  });

  it("使用与后端一致的 TTL 和金额边界", () => {
    expect(validateBillingRiskGuardSettings(defaults)).toBeNull();
    expect(validateBillingRiskGuardSettings({ ...defaults, billing_risk_safety_factor: 0.9 })).toBe("safetyFactor");
    expect(validateBillingRiskGuardSettings({ ...defaults, billing_risk_low_balance_threshold: -1 })).toBe("amount");
    expect(validateBillingRiskGuardSettings({ ...defaults, billing_risk_lease_ttl_seconds: 30 })).toBe("leaseTTL");
    expect(validateBillingRiskGuardSettings({ ...defaults, billing_risk_video_lease_ttl_seconds: 30 })).toBe("videoTTL");
  });
});
