<template>
  <div class="space-y-4 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800/40">
    <!-- Section header -->
    <div>
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
        {{ t("admin.accounts.upstreamPassthrough.title") }}
      </h3>
      <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.accounts.upstreamPassthrough.description") }}
      </p>
    </div>

    <!-- Category badge + manual-override picker -->
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs text-gray-600 dark:text-gray-400">
        {{ t("admin.accounts.upstreamPassthrough.category") }}:
      </span>
      <span
        class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-semibold"
        :class="categoryBadgeClass(derivedCategory)"
      >
        {{ categoryLabel(derivedCategory) }}
        <span
          v-if="!override.category_override"
          class="ml-1 text-[10px] opacity-70"
        >
          ({{ t("admin.accounts.upstreamPassthrough.categoryDerivedBadge") }})
        </span>
      </span>
      <select
        :value="override.category_override || ''"
        class="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs text-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
        @change="
          updateCategoryOverride(
            ($event.target as HTMLSelectElement).value as
              | ''
              | 'relay'
              | 'official'
              | 'reverse',
          )
        "
      >
        <option value="">
          {{
            t("admin.accounts.upstreamPassthrough.categoryAuto", {
              derived: categoryLabel(derivedCategory),
            })
          }}
        </option>
        <option value="relay">
          {{ t("admin.accounts.upstreamPassthrough.categoryRelay") }}
        </option>
        <option value="official">
          {{ t("admin.accounts.upstreamPassthrough.categoryOfficial") }}
        </option>
        <option value="reverse">
          {{ t("admin.accounts.upstreamPassthrough.categoryReverse") }}
        </option>
      </select>
      <span
        v-if="resolvedCategory === 'reverse'"
        class="text-xs text-orange-700 dark:text-orange-300"
      >
        {{ t("admin.accounts.upstreamPassthrough.categoryReverseWarn") }}
      </span>
    </div>

    <!-- Profile selector -->
    <div class="flex flex-wrap items-center gap-2">
      <label class="text-xs text-gray-600 dark:text-gray-400">
        {{ t("admin.accounts.upstreamPassthrough.profileLabel") }}:
      </label>
      <select
        :value="override.profile || ''"
        class="rounded-md border border-gray-300 bg-white px-2 py-1 text-xs text-gray-700 focus:border-blue-500 focus:ring-1 focus:ring-blue-500 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-200"
        @change="
          updateProfile(
            ($event.target as HTMLSelectElement).value as
              | ''
              | 'transparent'
              | 'protected'
              | 'strict',
          )
        "
      >
        <option value="">
          {{
            t("admin.accounts.upstreamPassthrough.profileInherit", {
              profile: profileShortLabel(categoryDefaultProfile(resolvedCategory)),
            })
          }}
        </option>
        <option value="transparent">
          {{ t("admin.accounts.upstreamPassthrough.profileTransparent") }}
        </option>
        <option value="protected">
          {{ t("admin.accounts.upstreamPassthrough.profileProtected") }}
        </option>
        <option value="strict">
          {{ t("admin.accounts.upstreamPassthrough.profileStrict") }}
        </option>
      </select>
    </div>

    <!-- Advanced: 7 tri-state toggles -->
    <details>
      <summary
        class="cursor-pointer text-xs font-medium text-gray-700 hover:text-gray-900 dark:text-gray-300 dark:hover:text-white"
      >
        {{ t("admin.accounts.upstreamPassthrough.advancedTitle") }}
      </summary>
      <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
        {{ t("admin.accounts.upstreamPassthrough.advancedHelp") }}
      </p>
      <div class="mt-3 space-y-2">
        <div
          v-for="key in toggleKeys"
          :key="key"
          class="flex items-start justify-between gap-3"
        >
          <div class="flex-1">
            <label
              class="text-xs font-medium text-gray-700 dark:text-gray-300"
              :title="
                t(`admin.settings.upstreamPassthrough.toggles.${camelOf(key)}Desc`)
              "
            >
              {{ t(`admin.settings.upstreamPassthrough.toggles.${camelOf(key)}`) }}
            </label>
            <p class="mt-0.5 text-[11px] text-gray-500 dark:text-gray-400">
              {{
                t(
                  `admin.settings.upstreamPassthrough.toggles.${camelOf(key)}Desc`,
                )
              }}
            </p>
          </div>
          <div class="flex shrink-0 gap-1">
            <button
              v-for="opt in tristateOptions"
              :key="opt.value"
              type="button"
              class="rounded px-2 py-1 text-[10px] font-semibold transition-colors"
              :class="tristateButtonClass(toggleStateOf(key), opt.value)"
              @click="setToggleState(key, opt.value)"
            >
              {{ t(opt.labelKey) }}
            </button>
          </div>
        </div>
      </div>
    </details>

    <!-- Live preview footer -->
    <div
      class="rounded-md border border-blue-200 bg-blue-50 p-3 text-xs dark:border-blue-900/40 dark:bg-blue-900/20"
    >
      <div class="flex items-center justify-between">
        <div>
          <span class="font-semibold text-blue-900 dark:text-blue-200">
            {{ t("admin.accounts.upstreamPassthrough.finalEffectiveTitle") }}:
          </span>
          <span class="ml-1 text-blue-700 dark:text-blue-300">
            {{ profileShortLabel(resolved.profileApplied) }}
          </span>
          <span class="ml-1 text-blue-600 dark:text-blue-400">
            ({{
              t("admin.accounts.upstreamPassthrough.finalEffectiveOverrideCount", {
                count: resolvedOverrideCount,
              })
            }})
          </span>
        </div>
        <button
          type="button"
          class="text-blue-700 underline-offset-2 hover:underline dark:text-blue-300"
          @click="previewExpanded = !previewExpanded"
        >
          {{
            previewExpanded
              ? t("admin.accounts.upstreamPassthrough.finalEffectiveCollapse")
              : t("admin.accounts.upstreamPassthrough.finalEffectiveExpand")
          }}
        </button>
      </div>
      <div
        v-if="resolved.globalOverrideActive"
        class="mt-1 text-amber-700 dark:text-amber-300"
      >
        {{ t("admin.accounts.upstreamPassthrough.killSwitchActive") }}
      </div>
      <div v-if="previewExpanded" class="mt-2 space-y-1">
        <div
          v-for="key in toggleKeys"
          :key="key"
          class="flex items-center justify-between text-[11px] text-blue-900 dark:text-blue-200"
        >
          <span>
            {{
              t(`admin.settings.upstreamPassthrough.toggles.${camelOf(key)}`)
            }}
          </span>
          <span class="font-mono">
            {{ resolved.toggles[key] ? "✓" : "✗" }}
          </span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import type {
  AccountUpstreamPassthroughOverride,
  UpstreamCategory,
  UpstreamPassthroughDefaults,
  UpstreamPassthroughGlobalOverride,
  UpstreamPassthroughOverrides,
  UpstreamPassthroughProfile,
  UpstreamPassthroughToggleKey,
} from "@/api/admin/settings";
import { UPSTREAM_PASSTHROUGH_TOGGLE_KEYS } from "@/api/admin/settings";
import {
  categoryDefaultProfile,
  deriveUpstreamCategory,
  overrideCount,
  resolveUpstreamPassthroughPolicy,
} from "@/utils/upstreamPassthrough";

const { t } = useI18n();

const props = defineProps<{
  modelValue: AccountUpstreamPassthroughOverride;
  accountType?: string;
  accountPlatform?: string;
  /** Other extra fields needed for category derivation (anthropic_passthrough etc.) */
  accountExtra?: Record<string, unknown>;
  /** System defaults — required for accurate preview when account inherits. */
  systemDefaults?: UpstreamPassthroughDefaults | null;
  /** Global kill-switch mode from system settings. */
  globalOverride?: UpstreamPassthroughGlobalOverride | string | null;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", value: AccountUpstreamPassthroughOverride): void;
}>();

const previewExpanded = ref(false);

const toggleKeys = UPSTREAM_PASSTHROUGH_TOGGLE_KEYS;

const tristateOptions: Array<{
  value: "inherit" | "on" | "off";
  labelKey: string;
}> = [
  { value: "inherit", labelKey: "admin.accounts.upstreamPassthrough.tristateInherit" },
  { value: "on", labelKey: "admin.accounts.upstreamPassthrough.tristateOn" },
  { value: "off", labelKey: "admin.accounts.upstreamPassthrough.tristateOff" },
];

const override = computed<AccountUpstreamPassthroughOverride>(
  () => props.modelValue ?? {}
);

/** Minimal Account shape fed to the resolver — only fields the resolver reads. */
const accountShape = computed(() => ({
  type: props.accountType,
  platform: props.accountPlatform,
  extra: {
    ...(props.accountExtra ?? {}),
    upstream_passthrough: override.value,
  },
}));

const derivedCategory = computed<UpstreamCategory>(() =>
  // Category WITHOUT honoring the account's own category_override — used
  // purely to show the "derived" badge value next to the override picker.
  deriveUpstreamCategory({
    type: props.accountType,
    platform: props.accountPlatform,
    extra: {
      ...(props.accountExtra ?? {}),
      // Strip override field so we see the raw derivation result.
      upstream_passthrough: undefined,
    },
  })
);

const resolved = computed(() =>
  resolveUpstreamPassthroughPolicy(
    accountShape.value,
    props.systemDefaults ?? null,
    props.globalOverride ?? null
  )
);

const resolvedCategory = computed<UpstreamCategory>(
  () => resolved.value.category
);

const resolvedOverrideCount = computed(() => overrideCount(resolved.value));

function categoryLabel(category: UpstreamCategory): string {
  switch (category) {
    case "relay":
      return t("admin.accounts.upstreamPassthrough.categoryRelay");
    case "official":
      return t("admin.accounts.upstreamPassthrough.categoryOfficial");
    case "reverse":
      return t("admin.accounts.upstreamPassthrough.categoryReverse");
  }
  return category;
}

function categoryBadgeClass(category: UpstreamCategory): string {
  switch (category) {
    case "relay":
      return "bg-blue-100 text-blue-800 dark:bg-blue-900/40 dark:text-blue-200";
    case "official":
      return "bg-green-100 text-green-800 dark:bg-green-900/40 dark:text-green-200";
    case "reverse":
      return "bg-orange-100 text-orange-800 dark:bg-orange-900/40 dark:text-orange-200";
  }
  return "bg-gray-100 text-gray-800";
}

function profileShortLabel(profile: UpstreamPassthroughProfile): string {
  switch (profile) {
    case "transparent":
      return t("admin.accounts.upstreamPassthrough.profileTransparent");
    case "protected":
      return t("admin.accounts.upstreamPassthrough.profileProtected");
    case "strict":
      return t("admin.accounts.upstreamPassthrough.profileStrict");
  }
  return profile;
}

function camelOf(key: UpstreamPassthroughToggleKey): string {
  return key.replace(/_([a-z])/g, (_, c: string) => c.toUpperCase());
}

function emitNext(next: AccountUpstreamPassthroughOverride) {
  // Keep the wire shape sparse: drop empty-string profile/category_override
  // and empty overrides so the backend doesn't see ambiguous "" markers.
  const out: AccountUpstreamPassthroughOverride = {};
  if (next.category_override) out.category_override = next.category_override;
  if (next.profile) out.profile = next.profile;
  if (next.overrides && Object.keys(next.overrides).length > 0) {
    out.overrides = { ...next.overrides };
  }
  emit("update:modelValue", out);
}

function updateCategoryOverride(value: "" | UpstreamCategory) {
  emitNext({
    ...override.value,
    category_override: value === "" ? "" : value,
  });
}

function updateProfile(value: "" | UpstreamPassthroughProfile) {
  emitNext({
    ...override.value,
    profile: value === "" ? "" : value,
  });
}

function toggleStateOf(
  key: UpstreamPassthroughToggleKey
): "inherit" | "on" | "off" {
  const o = override.value.overrides;
  if (!o || !Object.prototype.hasOwnProperty.call(o, key)) return "inherit";
  return o[key] ? "on" : "off";
}

function setToggleState(
  key: UpstreamPassthroughToggleKey,
  state: "inherit" | "on" | "off"
) {
  const nextOverrides: UpstreamPassthroughOverrides = {
    ...(override.value.overrides ?? {}),
  };
  if (state === "inherit") {
    delete nextOverrides[key];
  } else {
    nextOverrides[key] = state === "on";
  }
  emitNext({
    ...override.value,
    overrides: nextOverrides,
  });
}

function tristateButtonClass(
  current: "inherit" | "on" | "off",
  option: "inherit" | "on" | "off"
): string {
  if (current !== option) {
    return "bg-gray-200 text-gray-600 hover:bg-gray-300 dark:bg-dark-700 dark:text-gray-400 dark:hover:bg-dark-600";
  }
  switch (option) {
    case "on":
      return "bg-green-600 text-white";
    case "off":
      return "bg-red-600 text-white";
    case "inherit":
      return "bg-blue-600 text-white";
  }
  return "";
}
</script>
