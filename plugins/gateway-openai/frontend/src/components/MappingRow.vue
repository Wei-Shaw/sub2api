<template>
  <div
    class="group relative rounded-xl border border-gray-200 bg-white p-4 shadow-sm transition-all hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-700 dark:hover:border-primary-700"
  >
    <div class="flex items-center gap-4">
      <div
        class="grid flex-1 gap-4 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:items-start"
      >
        <div>
          <label class="input-label">{{
            t("admin.groups.openaiMessages.claudeModel")
          }}</label>
          <input
            v-model="row.claude_model"
            type="text"
            :placeholder="
              t('admin.groups.openaiMessages.claudeModelPlaceholder')
            "
            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
          />
        </div>
        <div
          class="hidden md:flex md:justify-center md:pt-7 text-primary-300 dark:text-primary-700"
        >
          <Icon
            name="arrowRight"
            size="sm"
            class="transition-transform group-hover:translate-x-1"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.openaiMessages.targetModel")
          }}</label>
          <input
            v-model="row.target_model"
            type="text"
            :placeholder="
              t('admin.groups.openaiMessages.targetModelPlaceholder')
            "
            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
          />
        </div>
      </div>
      <button
        type="button"
        @click="$emit('remove')"
        class="btn-icon-danger mt-6 flex h-9 w-9 items-center justify-center"
        :title="t('admin.groups.openaiMessages.removeExactMapping')"
      >
        <Icon name="trash" size="sm" />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { Icon } from "@sub2api/plugin-sdk";
import type { MessagesDispatchMappingRow } from "./messagesDispatchTypes";

defineProps<{
  row: MessagesDispatchMappingRow;
}>();

defineEmits<{ remove: [] }>();

const { t } = useI18n();
</script>
