<template>
  <div class="border-t pt-4">
    <label class="input-label">{{
      t("admin.groups.invalidRequestFallback.title")
    }}</label>
    <Select
      v-model="formData.fallback_group_id_on_invalid_request"
      :options="fallbackOptions"
      :placeholder="t('admin.groups.invalidRequestFallback.noFallback')"
    />
    <p class="input-hint">
      {{ t("admin.groups.invalidRequestFallback.hint") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import type { AdminGroup } from "@/types";
import { Select } from "@sub2api/plugin-sdk";

const formData = defineModel<Record<string, any>>('formData', { required: true });

const props = defineProps<{
  groups: AdminGroup[];
  editingGroupId?: number | null;
  platform: string;
}>();

const { t } = useI18n();

const fallbackOptions = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.invalidRequestFallback.noFallback") },
  ];
  const currentId = props.editingGroupId;
  const eligibleGroups = props.groups.filter(
    (g) =>
      g.platform === props.platform &&
      g.status === "active" &&
      g.subscription_type !== "subscription" &&
      g.fallback_group_id_on_invalid_request === null &&
      g.id !== currentId,
  );
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name });
  });
  return options;
});
</script>
