<template>
  <section class="card" data-testid="billing-risk-settings">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.billingRisk.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.billingRisk.description") }}
      </p>
    </div>

    <div class="space-y-6 p-6">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-gray-900 dark:text-white">
            {{ t("admin.settings.billingRisk.enabled") }}
          </label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.billingRisk.enabledHint") }}
          </p>
        </div>
        <Toggle
          :model-value="modelValue.billing_risk_enabled"
          @update:model-value="update('billing_risk_enabled', $event)"
        />
      </div>

      <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.billingRisk.budget") }}
        </h3>
        <div class="mt-4 grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="field in amountFields" :key="field.key" class="space-y-2">
            <label class="input-label" :for="field.key">
              {{ t(`admin.settings.billingRisk.fields.${field.label}`) }}
            </label>
            <input
              :id="field.key"
              :value="modelValue[field.key]"
              type="number"
              :min="field.min"
              :step="field.step"
              class="input"
              :data-testid="field.testId"
              @input="updateNumber(field.key, $event)"
            />
            <p
              class="text-xs leading-5 text-gray-500 dark:text-gray-400"
              :data-testid="`billing-risk-${field.label}-hint`"
            >
              {{ t(`admin.settings.billingRisk.fields.${field.label}Hint`) }}
            </p>
          </div>
        </div>
      </div>

      <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t("admin.settings.billingRisk.lifecycle") }}
        </h3>
        <div class="mt-4 grid grid-cols-1 gap-x-4 gap-y-5 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="field in durationFields" :key="field.key" class="space-y-2">
            <label class="input-label" :for="field.key">
              {{ t(`admin.settings.billingRisk.fields.${field.label}`) }}
            </label>
            <input
              :id="field.key"
              :value="modelValue[field.key]"
              type="number"
              min="1"
              step="1"
              class="input"
              :data-testid="field.testId"
              @input="updateNumber(field.key, $event)"
            />
            <p
              class="text-xs leading-5 text-gray-500 dark:text-gray-400"
              :data-testid="`billing-risk-${field.label}-hint`"
            >
              {{ t(`admin.settings.billingRisk.fields.${field.label}Hint`) }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Toggle from "@/components/common/Toggle.vue";
import type { BillingRiskGuardSettings } from "@/api/admin/settings";

const { t } = useI18n();

const props = defineProps<{
  modelValue: BillingRiskGuardSettings;
}>();

const emit = defineEmits<{
  (event: "update:modelValue", value: BillingRiskGuardSettings): void;
}>();

type NumericKey = Exclude<keyof BillingRiskGuardSettings, "billing_risk_enabled">;

interface NumericField {
  key: NumericKey;
  label: string;
  testId: string;
  min?: string;
  step?: string;
}

const amountFields: NumericField[] = [
  { key: "billing_risk_low_balance_threshold", label: "lowBalanceThreshold", testId: "billing-risk-low-balance-threshold", min: "0", step: "0.01" },
  { key: "billing_risk_safety_factor", label: "safetyFactor", testId: "billing-risk-safety-factor", min: "1", step: "0.01" },
  { key: "billing_risk_minimum_request_risk", label: "minimumRequestRisk", testId: "billing-risk-minimum-request-risk", min: "0", step: "0.001" },
  { key: "billing_risk_overdraft_allowance", label: "overdraftAllowance", testId: "billing-risk-overdraft-allowance", min: "0", step: "0.01" },
  { key: "billing_risk_high_cost_trigger", label: "highCostTrigger", testId: "billing-risk-high-cost-trigger", min: "0", step: "0.01" },
];

const durationFields: NumericField[] = [
  { key: "billing_risk_lease_ttl_seconds", label: "leaseTTL", testId: "billing-risk-lease-ttl" },
  { key: "billing_risk_refresh_interval_seconds", label: "refreshInterval", testId: "billing-risk-refresh-interval" },
  { key: "billing_risk_uncertain_cooldown_seconds", label: "uncertainCooldown", testId: "billing-risk-uncertain-cooldown" },
  { key: "billing_risk_video_lease_ttl_seconds", label: "videoLeaseTTL", testId: "billing-risk-video-lease-ttl" },
  { key: "billing_risk_idle_balance_ttl_seconds", label: "idleBalanceTTL", testId: "billing-risk-idle-balance-ttl" },
];

function update<K extends keyof BillingRiskGuardSettings>(key: K, value: BillingRiskGuardSettings[K]): void {
  emit("update:modelValue", { ...props.modelValue, [key]: value });
}

function updateNumber(key: NumericKey, event: Event): void {
  update(key, (event.target as HTMLInputElement).valueAsNumber);
}
</script>
