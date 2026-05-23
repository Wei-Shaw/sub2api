<template>
  <div class="space-y-4">
    <!-- OpenAI Messages Dispatch Config -->
    <div class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4">
      <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
        {{ t("admin.groups.openaiMessages.title") }}
      </h4>

      <!-- Allow Messages Dispatch toggle -->
      <div class="flex items-center justify-between">
        <label class="text-sm text-gray-600 dark:text-gray-400">{{
          t("admin.groups.openaiMessages.allowDispatch")
        }}</label>
        <button
          type="button"
          @click="
            formData.allow_messages_dispatch =
              !formData.allow_messages_dispatch
          "
          class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
          :class="
            formData.allow_messages_dispatch
              ? 'bg-primary-500'
              : 'bg-gray-300 dark:bg-dark-600'
          "
        >
          <span
            class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
            :class="
              formData.allow_messages_dispatch
                ? 'translate-x-6'
                : 'translate-x-1'
            "
          />
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
        {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
      </p>

      <div v-if="formData.allow_messages_dispatch" class="mt-3">
        <FamilyMappingSection :form-data="formData" />
        <ExactMappingSection
          :form-data="formData"
          @add="addMapping"
          @remove="removeMapping"
        />
      </div>
    </div>

    <SharedImagePricing :form-data="formData" />
    <SharedAccountFilters :form-data="formData" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import {
  SharedImagePricing,
  SharedAccountFilters,
  type GroupConfigGroup,
} from "@sub2api/plugin-sdk";
import FamilyMappingSection from "./FamilyMappingSection.vue";
import ExactMappingSection from "./ExactMappingSection.vue";
import type { MessagesDispatchMappingRow } from "./messagesDispatchTypes";

defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, unknown>;
  groups: GroupConfigGroup[];
  editingGroupId?: number | null;
}>();

defineEmits<{ "update:formData": [value: Record<string, unknown>] }>();

const { t } = useI18n();

const addMapping = (mappings: MessagesDispatchMappingRow[]) => {
  mappings.push({ claude_model: "", target_model: "" });
};

const removeMapping = (payload: {
  mappings: MessagesDispatchMappingRow[];
  row: MessagesDispatchMappingRow;
}) => {
  const index = payload.mappings.indexOf(payload.row);
  if (index !== -1) {
    payload.mappings.splice(index, 1);
  }
};
</script>
