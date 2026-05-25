<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.upstreamPassthrough.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.upstreamPassthrough.description") }}
      </p>
    </div>

    <div class="space-y-6 p-6">
      <!-- Feature flag toggle -->
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.upstreamPassthrough.featureFlagEnabled") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.upstreamPassthrough.featureFlagEnabledHint") }}
          </p>
        </div>
        <Toggle
          :model-value="featureFlagEnabled"
          @update:model-value="$emit('update:featureFlagEnabled', $event)"
        />
      </div>

      <!-- Global kill switch -->
      <div>
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.settings.upstreamPassthrough.globalOverrideTitle") }}
        </label>
        <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.settings.upstreamPassthrough.globalOverrideHint") }}
        </p>
        <div class="mt-3 space-y-2">
          <label
            v-for="mode in globalOverrideOptions"
            :key="mode.value"
            class="flex cursor-pointer items-center gap-2"
          >
            <input
              type="radio"
              :value="mode.value"
              :checked="normalizedGlobalOverride === mode.value"
              class="h-4 w-4 text-blue-600 focus:ring-blue-500"
              @change="$emit('update:globalOverride', mode.value)"
            />
            <span class="text-sm text-gray-700 dark:text-gray-300">
              {{ t(mode.labelKey) }}
            </span>
          </label>
        </div>
      </div>

      <!-- Per-category defaults -->
      <div>
        <div class="flex items-center justify-between">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.upstreamPassthrough.categoryDefaultsTitle") }}
          </label>
          <button
            type="button"
            class="text-xs font-medium text-blue-600 hover:text-blue-700 dark:text-blue-400 dark:hover:text-blue-300"
            @click="resetDefaults"
          >
            {{ t("admin.settings.upstreamPassthrough.resetDefaults") }}
          </button>
        </div>

        <div class="mt-3 space-y-4">
          <UpstreamPassthroughCategoryEditor
            v-for="category in categoryOrder"
            :key="category"
            :category="category"
            :slot-value="slotFor(category)"
            @update:slot-value="updateSlot(category, $event)"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Toggle from "@/components/common/Toggle.vue";
import UpstreamPassthroughCategoryEditor from "@/views/admin/settings/UpstreamPassthroughCategoryEditor.vue";
import type {
  UpstreamCategory,
  UpstreamPassthroughCategoryDefault,
  UpstreamPassthroughDefaults,
  UpstreamPassthroughGlobalOverride,
} from "@/api/admin/settings";

const { t } = useI18n();

const props = defineProps<{
  featureFlagEnabled: boolean;
  globalOverride: UpstreamPassthroughGlobalOverride | string;
  defaults: UpstreamPassthroughDefaults;
}>();

const emit = defineEmits<{
  (e: "update:featureFlagEnabled", value: boolean): void;
  (e: "update:globalOverride", value: UpstreamPassthroughGlobalOverride): void;
  (e: "update:defaults", value: UpstreamPassthroughDefaults): void;
}>();

const categoryOrder: UpstreamCategory[] = ["relay", "official", "reverse"];

const globalOverrideOptions: Array<{
  value: UpstreamPassthroughGlobalOverride;
  labelKey: string;
}> = [
  {
    value: "auto",
    labelKey: "admin.settings.upstreamPassthrough.globalOverrideAuto",
  },
  {
    value: "force_transparent",
    labelKey:
      "admin.settings.upstreamPassthrough.globalOverrideForceTransparent",
  },
  {
    value: "force_protected",
    labelKey: "admin.settings.upstreamPassthrough.globalOverrideForceProtected",
  },
  {
    value: "force_strict",
    labelKey: "admin.settings.upstreamPassthrough.globalOverrideForceStrict",
  },
];

const normalizedGlobalOverride = computed<UpstreamPassthroughGlobalOverride>(
  () => {
    const v = (props.globalOverride || "auto") as string;
    if (
      v === "force_transparent" ||
      v === "force_protected" ||
      v === "force_strict"
    ) {
      return v;
    }
    return "auto";
  }
);

function slotFor(
  category: UpstreamCategory
): UpstreamPassthroughCategoryDefault {
  const raw =
    category === "relay"
      ? props.defaults.relay
      : category === "official"
        ? props.defaults.official
        : props.defaults.reverse;
  if (raw) return raw;
  // Fall back to the category's recommended preset shape so the editor
  // always has a valid profile string to bind on.
  const fallbackProfile =
    category === "relay"
      ? "transparent"
      : category === "official"
        ? "protected"
        : "strict";
  return { profile: fallbackProfile };
}

function updateSlot(
  category: UpstreamCategory,
  next: UpstreamPassthroughCategoryDefault
) {
  emit("update:defaults", {
    ...props.defaults,
    [category]: next,
  });
}

function resetDefaults() {
  emit("update:defaults", {
    relay: { profile: "transparent" },
    official: { profile: "protected" },
    reverse: { profile: "strict" },
  });
}
</script>
