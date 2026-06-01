<template>
  <div
    class="relative overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-800"
  >
    <div
      class="border-b border-gray-100 bg-gray-50/80 px-4 py-3 dark:border-dark-700 dark:bg-dark-700/50"
    >
      <div class="flex items-center gap-2">
        <div class="dot-info h-2 w-2 rounded-full"></div>
        <label class="text-sm font-medium text-gray-900 dark:text-white">{{
          t("admin.groups.openaiMessages.familyMappingTitle")
        }}</label>
      </div>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t("admin.groups.openaiMessages.familyMappingHint") }}
      </p>
    </div>
    <div class="p-4">
      <div class="grid gap-4 md:grid-cols-3">
        <div>
          <label class="input-label">{{
            t("admin.groups.openaiMessages.opusModel")
          }}</label>
          <input
            v-model="formData.opus_mapped_model"
            type="text"
            :placeholder="
              t('admin.groups.openaiMessages.opusModelPlaceholder')
            "
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.openaiMessages.sonnetModel")
          }}</label>
          <input
            v-model="formData.sonnet_mapped_model"
            type="text"
            :placeholder="
              t('admin.groups.openaiMessages.sonnetModelPlaceholder')
            "
            class="input"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.openaiMessages.haikuModel")
          }}</label>
          <input
            v-model="formData.haiku_mapped_model"
            type="text"
            :placeholder="
              t('admin.groups.openaiMessages.haikuModelPlaceholder')
            "
            class="input"
          />
        </div>
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
