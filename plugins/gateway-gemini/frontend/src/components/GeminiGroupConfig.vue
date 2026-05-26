<template>
  <div>
    <SharedImagePricing :form-data="formData" />
    <SharedAccountFilters :form-data="formData" />
  </div>
</template>

<script setup lang="ts">
import {
  SharedImagePricing,
  SharedAccountFilters,
  type GroupConfigGroup,
} from "@sub2api/plugin-sdk";

defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, unknown>;
  groups: GroupConfigGroup[];
  editingGroupId?: number | null;
}>();
</script>
