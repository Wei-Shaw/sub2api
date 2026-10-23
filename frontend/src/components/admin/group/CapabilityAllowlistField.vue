<template>
  <div class="border-t border-gray-200 pt-4 mt-4 dark:border-dark-400">
    <div class="mb-3 flex items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.groups.capabilityAllowlist.title") }}
        </label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.groups.capabilityAllowlist.hint") }}
        </p>
      </div>
      <Toggle
        data-testid="capability-allowlist-toggle"
        :aria-label="t('admin.groups.capabilityAllowlist.enable')"
        :model-value="config.enabled"
        @update:model-value="setEnabled($event)"
      />
    </div>

    <div v-if="config.enabled" class="space-y-4">
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.groups.capabilityAllowlist.enabledHint") }}
      </p>

      <div>
        <label class="input-label">
          {{ t("admin.groups.capabilityAllowlist.daybreak") }}
        </label>
        <div class="flex flex-wrap gap-4">
          <label
            v-for="program in cyberPrograms"
            :key="program"
            class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
          >
            <input
              type="checkbox"
              :data-testid="`capability-allowlist-cyber-${program}`"
              :checked="config.cyber_programs.includes(program)"
              @change="toggleValue('cyber_programs', program, ($event.target as HTMLInputElement).checked)"
            />
            {{ t(`admin.groups.capabilityAllowlist.programs.${program}`) }}
          </label>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.groups.capabilityAllowlist.daybreakHint") }}
        </p>
      </div>

      <div>
        <label class="input-label">
          {{ t("admin.groups.capabilityAllowlist.serviceTier") }}
        </label>
        <div class="flex flex-wrap gap-4">
          <label
            v-for="tier in serviceTiers"
            :key="tier"
            class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300"
          >
            <input
              type="checkbox"
              :data-testid="`capability-allowlist-tier-${tier}`"
              :checked="config.service_tiers.includes(tier)"
              @change="toggleValue('service_tiers', tier, ($event.target as HTMLInputElement).checked)"
            />
            {{ t(`admin.groups.capabilityAllowlist.tiers.${tier}`) }}
          </label>
        </div>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t("admin.groups.capabilityAllowlist.serviceTierHint") }}
        </p>
      </div>
    </div>
    <p v-else class="text-xs text-gray-500 dark:text-gray-400">
      {{ t("admin.groups.capabilityAllowlist.disabledHint") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Toggle from "@/components/common/Toggle.vue";
import type { CapabilityAllowlist } from "@/types";
import {
  CAPABILITY_CYBER_PROGRAMS,
  CAPABILITY_SERVICE_TIERS,
  cloneCapabilityAllowlist,
} from "@/views/admin/groupsCapabilityAllowlist";

const props = defineProps<{
  config: CapabilityAllowlist;
  platform: string;
}>();

const emit = defineEmits<{
  (event: "update:config", value: CapabilityAllowlist): void;
}>();

const { t } = useI18n();
const cyberPrograms = CAPABILITY_CYBER_PROGRAMS;
const serviceTiers = CAPABILITY_SERVICE_TIERS;

function setEnabled(enabled: boolean) {
  emit("update:config", { ...cloneCapabilityAllowlist(props.config), enabled });
}

function toggleValue(
  field: "cyber_programs" | "service_tiers",
  value: string,
  checked: boolean,
) {
  const next = cloneCapabilityAllowlist(props.config);
  const current = next[field].filter((item) => item !== value);
  next[field] = checked ? [...current, value] : current;
  emit("update:config", next);
}
</script>
