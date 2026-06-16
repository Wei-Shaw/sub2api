<template>
  <div
    class="mt-5 relative overflow-hidden rounded-xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/50 dark:bg-dark-800"
  >
    <div
      class="border-b border-primary-100 bg-primary-50/80 px-4 py-3 dark:border-primary-900/40 dark:bg-primary-900/20"
    >
      <div class="flex items-start justify-between gap-3">
        <div>
          <div class="flex items-center gap-2">
            <div class="h-2 w-2 rounded-full bg-primary-500"></div>
            <label
              class="text-sm font-medium text-primary-900 dark:text-primary-100"
            >{{
              t("admin.groups.openaiMessages.exactMappingTitle")
            }}</label>
          </div>
          <p
            class="mt-1 text-xs text-primary-600/90 dark:text-primary-400/90"
          >
            {{ t("admin.groups.openaiMessages.exactMappingHint") }}
          </p>
        </div>
      </div>
    </div>

    <div class="p-4 bg-gray-50/30 dark:bg-dark-800/30">
      <!-- Empty state -->
      <div
        v-if="mappings.length === 0"
        class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-white px-5 py-4 text-sm text-primary-700 transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:bg-dark-800 dark:text-primary-300 dark:hover:border-primary-800"
      >
        <span>{{ t("admin.groups.openaiMessages.noExactMappings") }}</span>
        <button
          type="button"
          @click="$emit('add', mappings)"
          class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.openaiMessages.addExactMapping") }}
        </button>
      </div>

      <!-- Mapping rows -->
      <div v-else class="space-y-3">
        <MappingRow
          v-for="row in mappings"
          :key="getMappingRowKey(row)"
          :row="row"
          @remove="$emit('remove', { mappings, row })"
        />

        <button
          type="button"
          @click="$emit('add', mappings)"
          class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-gray-300 bg-white py-3 text-sm font-medium text-gray-500 transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.openaiMessages.addExactMapping") }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { Icon, createStableObjectKeyResolver } from "@sub2api/plugin-sdk";
import MappingRow from "./MappingRow.vue";
import type { MessagesDispatchMappingRow } from "./messagesDispatchTypes";

const props = defineProps<{
  formData: Record<string, unknown>;
}>();

defineEmits<{
  add: [mappings: MessagesDispatchMappingRow[]];
  remove: [payload: { mappings: MessagesDispatchMappingRow[]; row: MessagesDispatchMappingRow }];
}>();

const { t } = useI18n();

const resolveRowKey =
  createStableObjectKeyResolver<MessagesDispatchMappingRow>(
    "messages-dispatch-row",
  );

const getMappingRowKey = (row: MessagesDispatchMappingRow) =>
  resolveRowKey(row);

const mappings = computed(
  () => props.formData.exact_model_mappings as MessagesDispatchMappingRow[],
);
</script>
