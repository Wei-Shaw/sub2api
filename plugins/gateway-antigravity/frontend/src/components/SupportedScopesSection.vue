<template>
  <div class="border-t pt-4">
    <div class="mb-1.5 flex items-center gap-1">
      <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
        {{ t("admin.groups.supportedScopes.title") }}
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
              {{ t("admin.groups.supportedScopes.tooltip") }}
            </p>
            <div
              class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
            ></div>
          </div>
        </div>
      </div>
    </div>
    <div class="space-y-2">
      <label
        v-for="scope in SCOPES"
        :key="scope.value"
        class="flex items-center gap-2 cursor-pointer"
      >
        <input
          type="checkbox"
          :checked="scopes.includes(scope.value)"
          @change="toggleScope(scope.value)"
          class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
        />
        <span class="text-sm text-gray-700 dark:text-gray-300">{{
          t(scope.labelKey)
        }}</span>
      </label>
    </div>
    <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
      {{ t("admin.groups.supportedScopes.hint") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { Icon } from "@sub2api/plugin-sdk";

const SCOPES = [
  { value: "claude", labelKey: "admin.groups.supportedScopes.claude" },
  { value: "gemini_text", labelKey: "admin.groups.supportedScopes.geminiText" },
  { value: "gemini_image", labelKey: "admin.groups.supportedScopes.geminiImage" },
] as const;

const props = defineProps<{
  formData: Record<string, unknown>;
}>();

const { t } = useI18n();

const scopes = computed(
  () => props.formData.supported_model_scopes as string[],
);

const toggleScope = (scope: string) => {
  const arr = scopes.value;
  const idx = arr.indexOf(scope);
  if (idx === -1) {
    arr.push(scope);
  } else {
    arr.splice(idx, 1);
  }
};
</script>
