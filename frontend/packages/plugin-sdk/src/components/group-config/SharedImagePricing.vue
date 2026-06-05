<template>
  <div class="border-t pt-4">
    <label class="block mb-2 font-medium text-gray-700 dark:text-gray-300">
      {{ t("admin.groups.imagePricing.title") }}
    </label>
    <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
      {{ t("admin.groups.imagePricing.description") }}
    </p>
    <div class="grid grid-cols-3 gap-3">
      <div>
        <label class="input-label">1K ($)</label>
        <input
          v-model.number="formData.image_price_1k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          placeholder="0.134"
        />
      </div>
      <div>
        <label class="input-label">2K ($)</label>
        <input
          v-model.number="formData.image_price_2k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          placeholder="0.201"
        />
      </div>
      <div>
        <label class="input-label">4K ($)</label>
        <input
          v-model.number="formData.image_price_4k"
          type="number"
          step="0.001"
          min="0"
          class="input"
          placeholder="0.268"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";

defineProps<{
  formData: Record<string, unknown>;
}>();

const { t } = useI18n();
</script>
