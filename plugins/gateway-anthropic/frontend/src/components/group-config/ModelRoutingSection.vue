<template>
  <div class="border-t pt-4">
    <div class="mb-1.5 flex items-center gap-1">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.groups.modelRouting.title") }}
      </label>
      <div class="group relative inline-flex">
        <Icon
          name="questionCircle"
          size="sm"
          :stroke-width="2"
          class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
        />
        <div
          class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
        >
          <div
            class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
          >
            <p class="text-xs leading-relaxed text-gray-300">
              {{ t("admin.groups.modelRouting.tooltip") }}
            </p>
            <div
              class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
            ></div>
          </div>
        </div>
      </div>
    </div>
    <!-- Enable toggle -->
    <div class="flex items-center gap-3 mb-3">
      <button
        type="button"
        @click="formData.model_routing_enabled = !formData.model_routing_enabled"
        :class="[
          'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
          formData.model_routing_enabled
            ? 'bg-primary-500'
            : 'bg-gray-300 dark:bg-dark-600',
        ]"
      >
        <span
          :class="[
            'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
            formData.model_routing_enabled ? 'translate-x-6' : 'translate-x-1',
          ]"
        />
      </button>
      <span class="text-sm text-gray-500 dark:text-gray-400">
        {{
          formData.model_routing_enabled
            ? t("admin.groups.modelRouting.enabled")
            : t("admin.groups.modelRouting.disabled")
        }}
      </span>
    </div>
    <p
      v-if="!formData.model_routing_enabled"
      class="text-xs text-gray-500 dark:text-gray-400 mb-3"
    >
      {{ t("admin.groups.modelRouting.disabledHint") }}
    </p>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t("admin.groups.modelRouting.noRulesHint") }}
    </p>
    <!-- Routing rules list -->
    <div v-if="formData.model_routing_enabled" class="space-y-3">
      <RoutingRuleRow
        v-for="rule in routing.routingRules.value"
        :key="routing.getRuleRenderKey(rule)"
        :rule="rule"
        :search-key="routing.getRuleSearchKey(rule)"
        :search-keyword="routing.accountSearchKeyword.value[routing.getRuleSearchKey(rule)]"
        :search-results="routing.accountSearchResults.value[routing.getRuleSearchKey(rule)]"
        :show-dropdown="routing.showAccountDropdown.value[routing.getRuleSearchKey(rule)]"
        @search="routing.searchAccountsByRule(rule)"
        @focus="routing.onAccountSearchFocus(rule)"
        @update:keyword="routing.accountSearchKeyword.value[routing.getRuleSearchKey(rule)] = $event"
        @select="routing.selectAccount(rule, $event)"
        @remove-account="routing.removeSelectedAccount(rule, $event)"
        @remove-rule="routing.removeRoutingRule(rule)"
      />
    </div>
    <!-- Add rule button -->
    <button
      v-if="formData.model_routing_enabled"
      type="button"
      @click="routing.addRoutingRule()"
      class="mt-3 flex items-center gap-1.5 text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
    >
      <Icon name="plus" size="sm" />
      {{ t("admin.groups.modelRouting.addRule") }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import { Icon } from "@sub2api/plugin-sdk";
import RoutingRuleRow from "./RoutingRuleRow.vue";
import { useModelRouting } from "./useModelRouting";

const props = defineProps<{
  mode: "create" | "edit";
  formData: Record<string, unknown>;
}>();

const { t } = useI18n();
const routing = useModelRouting(props.mode);

// Click outside handler to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  if (!target.closest(".account-search-container")) {
    Object.keys(routing.showAccountDropdown.value).forEach((key) => {
      routing.showAccountDropdown.value[key] = false;
    });
  }
};

onMounted(() => {
  document.addEventListener("click", handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener("click", handleClickOutside);
  routing.clearAll();
});

defineExpose({
  routingRules: routing.routingRules,
  getRoutingRulesApiFormat: routing.getRoutingRulesApiFormat,
  loadRoutingRules: routing.loadRoutingRules,
  resetRoutingRules: routing.resetRoutingRules,
});
</script>
