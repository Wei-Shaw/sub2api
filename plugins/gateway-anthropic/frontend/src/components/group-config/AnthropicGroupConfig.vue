<template>
  <div class="space-y-4">
    <!-- Claude Code Only Toggle -->
    <div class="border-t pt-4">
      <div class="mb-1.5 flex items-center gap-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.claudeCode.title") }}
        </label>
        <div class="group relative inline-flex">
          <Icon
            name="questionCircle"
            size="sm"
            :stroke-width="2"
            class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
          />
          <div
            class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
          >
            <div
              class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
            >
              <p class="text-xs leading-relaxed text-gray-300">
                {{ t("admin.groups.claudeCode.tooltip") }}
              </p>
              <div
                class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
              ></div>
            </div>
          </div>
        </div>
      </div>
      <div class="flex items-center gap-3">
        <button
          type="button"
          @click="formData.claude_code_only = !formData.claude_code_only"
          :class="[
            'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
            formData.claude_code_only
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600',
          ]"
        >
          <span
            :class="[
              'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
              formData.claude_code_only ? 'translate-x-6' : 'translate-x-1',
            ]"
          />
        </button>
        <span class="text-sm text-gray-500 dark:text-gray-400">
          {{
            formData.claude_code_only
              ? t("admin.groups.claudeCode.enabled")
              : t("admin.groups.claudeCode.disabled")
          }}
        </span>
      </div>
      <!-- Fallback group selector (only when claude_code_only is enabled) -->
      <div v-if="formData.claude_code_only" class="mt-3">
        <label class="input-label">{{
          t("admin.groups.claudeCode.fallbackGroup")
        }}</label>
        <Select
          :model-value="formData.fallback_group_id as number | null"
          @update:model-value="formData.fallback_group_id = $event"
          :options="fallbackGroupOptions"
          :placeholder="t('admin.groups.claudeCode.noFallback')"
        />
        <p class="input-hint">
          {{ t("admin.groups.claudeCode.fallbackHint") }}
        </p>
      </div>
    </div>

    <!-- Invalid Request Fallback (non-subscription only) -->
    <SharedInvalidRequestFallback
      v-if="formData.subscription_type !== SUBSCRIPTION_TYPE_SUBSCRIPTION"
      :form-data="formData"
      :groups="groups"
      :editing-group-id="editingGroupId"
      :platform="(formData.platform as string) || 'anthropic'"
    />

    <!-- Account Filters -->
    <SharedAccountFilters :form-data="formData" />

    <!-- Model Routing -->
    <ModelRoutingSection
      ref="modelRoutingRef"
      :mode="mode"
      :form-data="formData"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from "vue";
import { useI18n } from "vue-i18n";
import {
  Icon,
  Select,
  SharedInvalidRequestFallback,
  SharedAccountFilters,
  SUBSCRIPTION_TYPE_SUBSCRIPTION,
  type GroupConfigGroup,
} from "@sub2api/plugin-sdk";
import ModelRoutingSection from "./ModelRoutingSection.vue";

const props = defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, unknown>;
  groups: GroupConfigGroup[];
  editingGroupId?: number | null;
}>();

defineEmits<{ "update:formData": [value: Record<string, unknown>] }>();

const { t } = useI18n();

const modelRoutingRef = ref<InstanceType<typeof ModelRoutingSection> | null>(
  null,
);

// --- Fallback group options ---

const fallbackGroupOptions = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.claudeCode.noFallback") },
  ];
  const currentId = props.mode === "edit" ? props.editingGroupId : undefined;
  const eligible = props.groups.filter(
    (g) =>
      g.platform === "anthropic" &&
      !g.claude_code_only &&
      g.status === "active" &&
      g.id !== currentId,
  );
  eligible.forEach((g) => options.push({ value: g.id, label: g.name }));
  return options;
});

// --- Public API (delegates to ModelRoutingSection) ---

const getRoutingRulesApiFormat = () =>
  modelRoutingRef.value?.getRoutingRulesApiFormat() ?? null;

const loadRoutingRules = async (
  apiFormat: Record<string, number[]> | null,
) => {
  await modelRoutingRef.value?.loadRoutingRules(apiFormat);
};

const resetRoutingRules = () => {
  modelRoutingRef.value?.resetRoutingRules();
};

defineExpose({
  getRoutingRulesApiFormat,
  loadRoutingRules,
  resetRoutingRules,
});
</script>
