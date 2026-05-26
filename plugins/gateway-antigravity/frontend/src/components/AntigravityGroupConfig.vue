<template>
  <div class="space-y-4">
    <SharedImagePricing :form-data="formData" />
    <SupportedScopesSection :form-data="formData" />
    <McpXmlInjectSection :form-data="formData" />
    <SharedAccountFilters :form-data="formData" />

    <SharedInvalidRequestFallback
      v-if="formData.subscription_type !== SUBSCRIPTION_TYPE_SUBSCRIPTION"
      :form-data="formData"
      :groups="groups"
      :editing-group-id="editingGroupId"
      :platform="(formData.platform as string) || 'antigravity'"
    />
  </div>
</template>

<script setup lang="ts">
import {
  SharedImagePricing,
  SharedAccountFilters,
  SharedInvalidRequestFallback,
  SUBSCRIPTION_TYPE_SUBSCRIPTION,
  type GroupConfigGroup,
} from "@sub2api/plugin-sdk";
import SupportedScopesSection from "./SupportedScopesSection.vue";
import McpXmlInjectSection from "./McpXmlInjectSection.vue";

defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, unknown>;
  groups: GroupConfigGroup[];
  editingGroupId?: number | null;
}>();
</script>
