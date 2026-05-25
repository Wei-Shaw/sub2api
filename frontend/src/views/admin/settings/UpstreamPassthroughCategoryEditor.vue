<template>
  <div
    class="rounded-lg border border-gray-200 p-4 dark:border-dark-700"
    :class="categoryBorderClass"
  >
    <div class="mb-3 flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span
          class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold"
          :class="categoryBadgeClass"
        >
          {{ categoryLabel }}
        </span>
        <span class="text-xs text-gray-500 dark:text-gray-400">
          {{ categoryDesc }}
        </span>
      </div>
      <div class="flex items-center gap-2">
        <label class="text-xs text-gray-600 dark:text-gray-400">
          {{ t("admin.settings.upstreamPassthrough.profile") }}
        </label>
        <select
          :value="slotValue.profile"
          class="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs text-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
          @change="onProfileChange(($event.target as HTMLSelectElement).value)"
        >
          <option value="transparent">
            {{ t("admin.settings.upstreamPassthrough.profileTransparent") }}
          </option>
          <option value="protected">
            {{ t("admin.settings.upstreamPassthrough.profileProtected") }}
          </option>
          <option value="strict">
            {{ t("admin.settings.upstreamPassthrough.profileStrict") }}
          </option>
        </select>
      </div>
    </div>

    <!-- 7 toggle preview: read-only when overrides empty, click to flip -->
    <div class="grid grid-cols-1 gap-x-4 gap-y-2 md:grid-cols-2">
      <label
        v-for="key in toggleKeys"
        :key="key"
        class="flex cursor-pointer items-start gap-2 text-xs"
        :title="t(`admin.settings.upstreamPassthrough.toggles.${camelOf(key)}Desc`)"
      >
        <input
          type="checkbox"
          :checked="effectiveToggle(key)"
          class="mt-0.5 h-3.5 w-3.5 rounded text-blue-600 focus:ring-blue-500"
          @change="onToggleChange(key, ($event.target as HTMLInputElement).checked)"
        />
        <span class="text-gray-700 dark:text-gray-300">
          {{ t(`admin.settings.upstreamPassthrough.toggles.${camelOf(key)}`) }}
          <span
            v-if="hasOverride(key)"
            class="ml-1 inline-block rounded bg-amber-100 px-1 text-[10px] font-semibold text-amber-800 dark:bg-amber-900/40 dark:text-amber-200"
          >
            ovr
          </span>
        </span>
      </label>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type {
  UpstreamCategory,
  UpstreamPassthroughCategoryDefault,
  UpstreamPassthroughProfile,
  UpstreamPassthroughToggleKey,
} from "@/api/admin/settings";
import { UPSTREAM_PASSTHROUGH_TOGGLE_KEYS } from "@/api/admin/settings";
import { PROFILE_TOGGLE_PRESETS } from "@/utils/upstreamPassthrough";

const { t } = useI18n();

const props = defineProps<{
  category: UpstreamCategory;
  slotValue: UpstreamPassthroughCategoryDefault;
}>();

const emit = defineEmits<{
  (e: "update:slot-value", value: UpstreamPassthroughCategoryDefault): void;
}>();

const toggleKeys = UPSTREAM_PASSTHROUGH_TOGGLE_KEYS;

const categoryLabel = computed(() =>
  t(`admin.settings.upstreamPassthrough.category${capitalize(props.category)}`)
);

const categoryDesc = computed(() =>
  t(
    `admin.settings.upstreamPassthrough.category${capitalize(props.category)}Desc`
  )
);

const categoryBadgeClass = computed(() => {
  switch (props.category) {
    case "relay":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200";
    case "official":
      return "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200";
    case "reverse":
      return "bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-200";
  }
  return "";
});

const categoryBorderClass = computed(() => {
  switch (props.category) {
    case "reverse":
      return "border-orange-200 dark:border-orange-900/40";
    default:
      return "";
  }
});

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}

/** Convert snake_case toggle key to camelCase for i18n lookup. */
function camelOf(key: UpstreamPassthroughToggleKey): string {
  return key.replace(/_([a-z])/g, (_, c) => c.toUpperCase());
}

function hasOverride(key: UpstreamPassthroughToggleKey): boolean {
  return (
    !!props.slotValue.overrides &&
    Object.prototype.hasOwnProperty.call(props.slotValue.overrides, key)
  );
}

function effectiveToggle(key: UpstreamPassthroughToggleKey): boolean {
  if (hasOverride(key) && props.slotValue.overrides) {
    const v = props.slotValue.overrides[key];
    if (typeof v === "boolean") return v;
  }
  const preset =
    PROFILE_TOGGLE_PRESETS[props.slotValue.profile] ??
    PROFILE_TOGGLE_PRESETS.protected;
  return preset[key];
}

function onProfileChange(next: string) {
  if (next !== "transparent" && next !== "protected" && next !== "strict")
    return;
  // Switching profile clears the sparse overrides — they represent deltas from
  // the previous profile and would mean something different against the new one.
  emit("update:slot-value", {
    profile: next as UpstreamPassthroughProfile,
    overrides: undefined,
  });
}

function onToggleChange(key: UpstreamPassthroughToggleKey, value: boolean) {
  const preset =
    PROFILE_TOGGLE_PRESETS[props.slotValue.profile] ??
    PROFILE_TOGGLE_PRESETS.protected;
  const nextOverrides = { ...(props.slotValue.overrides ?? {}) };
  if (value === preset[key]) {
    // Setting it back to the preset default → remove from sparse overrides
    delete nextOverrides[key];
  } else {
    nextOverrides[key] = value;
  }
  emit("update:slot-value", {
    profile: props.slotValue.profile,
    overrides:
      Object.keys(nextOverrides).length > 0 ? nextOverrides : undefined,
  });
}
</script>
