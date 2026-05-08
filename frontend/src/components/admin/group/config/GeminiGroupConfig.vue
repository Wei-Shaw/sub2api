<template>
  <div>
    <!-- 图片生成计费配置 -->
    <SharedImagePricing :form-data="formData" />

    <!-- 账号过滤控制 -->
    <SharedAccountFilters :form-data="formData" />
  </div>
</template>

<script setup lang="ts">
import type { AdminGroup } from "@/types";
import SharedImagePricing from "./SharedImagePricing.vue";
import SharedAccountFilters from "./SharedAccountFilters.vue";

defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, any>;
  groups: AdminGroup[];
  editingGroupId?: number | null;
}>();
</script>
