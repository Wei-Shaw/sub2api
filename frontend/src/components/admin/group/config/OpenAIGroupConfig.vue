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
        <!-- Family Mapping -->
        <div
          class="relative overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-800"
        >
          <div
            class="border-b border-gray-100 bg-gray-50/80 px-4 py-3 dark:border-dark-700 dark:bg-dark-700/50"
          >
            <div class="flex items-center gap-2">
              <div class="h-2 w-2 rounded-full bg-blue-500"></div>
              <label
                class="text-sm font-medium text-gray-900 dark:text-white"
                >{{
                  t("admin.groups.openaiMessages.familyMappingTitle")
                }}</label
              >
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

        <!-- Exact Model Mappings -->
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
                    }}</label
                  >
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
            <div
              v-if="formData.exact_model_mappings.length === 0"
              class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-white px-5 py-4 text-sm text-primary-700 transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:bg-dark-800 dark:text-primary-300 dark:hover:border-primary-800"
            >
              <span>{{
                t("admin.groups.openaiMessages.noExactMappings")
              }}</span>
              <button
                type="button"
                @click="addMapping"
                class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
              >
                <Icon name="plus" size="sm" />
                {{ t("admin.groups.openaiMessages.addExactMapping") }}
              </button>
            </div>

            <div v-else class="space-y-3">
              <div
                v-for="row in formData.exact_model_mappings"
                :key="getMappingRowKey(row)"
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
                          t(
                            'admin.groups.openaiMessages.claudeModelPlaceholder',
                          )
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
                          t(
                            'admin.groups.openaiMessages.targetModelPlaceholder',
                          )
                        "
                        class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                      />
                    </div>
                  </div>
                  <button
                    type="button"
                    @click="removeMapping(row)"
                    class="mt-6 flex h-9 w-9 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                    :title="
                      t('admin.groups.openaiMessages.removeExactMapping')
                    "
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>

              <button
                type="button"
                @click="addMapping"
                class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-gray-300 bg-white py-3 text-sm font-medium text-gray-500 transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
              >
                <Icon name="plus" size="sm" />
                {{ t("admin.groups.openaiMessages.addExactMapping") }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Image Pricing -->
    <SharedImagePricing :form-data="formData" />

    <!-- Account Filters -->
    <SharedAccountFilters :form-data="formData" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import Icon from "@/components/icons/Icon.vue";
import SharedImagePricing from "./SharedImagePricing.vue";
import SharedAccountFilters from "./SharedAccountFilters.vue";
import { createStableObjectKeyResolver } from "@/utils/stableObjectKey";
import type { MessagesDispatchMappingRow } from "@/views/admin/groupsMessagesDispatch";

const props = defineProps<{
  mode: "create" | "edit";
  platform: string;
  formData: Record<string, any>;
  groups: unknown[];
  editingGroupId?: number | null;
}>();

defineEmits<{ "update:formData": [value: Record<string, any>] }>();

const { t } = useI18n();

const resolveRowKey =
  createStableObjectKeyResolver<MessagesDispatchMappingRow>(
    "messages-dispatch-row",
  );

const getMappingRowKey = (row: MessagesDispatchMappingRow) =>
  resolveRowKey(row);

const addMapping = () => {
  const mappings = props.formData
    .exact_model_mappings as MessagesDispatchMappingRow[];
  mappings.push({ claude_model: "", target_model: "" });
};

const removeMapping = (row: MessagesDispatchMappingRow) => {
  const mappings = props.formData
    .exact_model_mappings as MessagesDispatchMappingRow[];
  const index = mappings.indexOf(row);
  if (index !== -1) {
    mappings.splice(index, 1);
  }
};
</script>
