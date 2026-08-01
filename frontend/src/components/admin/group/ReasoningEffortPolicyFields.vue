<template>
  <div class="space-y-4">
    <div>
      <label :for="`${idPrefix}-max-effort`" class="input-label">
        {{ t("admin.groups.form.maxReasoningEffort") }}
      </label>
      <Select
        :id="`${idPrefix}-max-effort`"
        :model-value="maxEffort"
        :options="reasoningEffortOptions"
        :placeholder="t('admin.groups.form.maxReasoningEffortUnlimited')"
        :aria-label="t('admin.groups.form.maxReasoningEffort')"
        :searchable="false"
        clearable
        @update:model-value="updateMaxEffort"
      />
      <p class="input-hint">{{ t("admin.groups.form.maxReasoningEffortHint") }}</p>
    </div>

    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="mb-3 flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between">
        <label class="input-label mb-0">
          {{ t("admin.groups.form.reasoningEffortMappings") }}
        </label>
        <button
          type="button"
          class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:text-primary-400 dark:hover:bg-primary-900/20 dark:hover:text-primary-300"
          @click="addMapping"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.form.addReasoningEffortMapping") }}
        </button>
      </div>

      <div v-if="mappings.length > 0" class="space-y-2">
        <div
          v-for="row in mappings"
          :key="row.id"
          class="rounded-lg border border-gray-200 bg-gray-50/40 p-3 dark:border-dark-600 dark:bg-dark-800/40"
        >
          <div class="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] md:items-start">
            <div>
              <label :for="`${idPrefix}-${row.id}-from`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortFrom") }}
              </label>
              <Select
                :id="`${idPrefix}-${row.id}-from`"
                :model-value="row.from"
                :options="reasoningEffortOptions"
                :placeholder="t('admin.groups.form.reasoningEffortFromPlaceholder')"
                :error="showValidation && !!validationErrors[row.id]?.from"
                :aria-label="t('admin.groups.form.reasoningEffortFrom')"
                :aria-describedby="showValidation && validationErrors[row.id]?.from ? `${idPrefix}-${row.id}-from-error` : undefined"
                :searchable="false"
                clearable
                @update:model-value="updateMapping(row.id, 'from', $event)"
              />
              <p
                v-if="showValidation && validationErrors[row.id]?.from"
                :id="`${idPrefix}-${row.id}-from-error`"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ mappingErrorText(validationErrors[row.id]?.from) }}
              </p>
            </div>

            <div class="hidden pt-8 text-gray-400 md:block dark:text-dark-400">
              <Icon name="arrowRight" size="sm" />
            </div>

            <div>
              <label :for="`${idPrefix}-${row.id}-to`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortTo") }}
              </label>
              <Select
                :id="`${idPrefix}-${row.id}-to`"
                :model-value="row.to"
                :options="reasoningEffortOptions"
                :placeholder="t('admin.groups.form.reasoningEffortToPlaceholder')"
                :error="showValidation && !!validationErrors[row.id]?.to"
                :aria-label="t('admin.groups.form.reasoningEffortTo')"
                :aria-describedby="showValidation && validationErrors[row.id]?.to ? `${idPrefix}-${row.id}-to-error` : undefined"
                :searchable="false"
                clearable
                @update:model-value="updateMapping(row.id, 'to', $event)"
              />
              <p
                v-if="showValidation && validationErrors[row.id]?.to"
                :id="`${idPrefix}-${row.id}-to-error`"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ mappingErrorText(validationErrors[row.id]?.to) }}
              </p>
            </div>

            <button
              type="button"
              class="flex h-11 w-11 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/30 md:mt-6 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('admin.groups.form.removeReasoningEffortMapping')"
              :aria-label="t('admin.groups.form.removeReasoningEffortMapping')"
              @click="removeMapping(row.id)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="mb-3 flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <label class="input-label mb-0">
            {{ t("admin.groups.form.modelReasoningEffortRules") }}
          </label>
          <p class="input-hint">
            {{ t("admin.groups.form.modelReasoningEffortRulesHint") }}
          </p>
        </div>
        <button
          type="button"
          class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:text-primary-400 dark:hover:bg-primary-900/20 dark:hover:text-primary-300"
          @click="addModelRule"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.form.addModelReasoningEffortRule") }}
        </button>
      </div>

      <div v-if="modelRules.length > 0" class="space-y-3">
        <div
          v-for="rule in modelRules"
          :key="rule.id"
          class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
        >
          <div class="grid gap-3 md:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] md:items-start">
            <div>
              <label :for="`${idPrefix}-${rule.id}-model`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortModel") }}
              </label>
              <Select
                :id="`${idPrefix}-${rule.id}-model`"
                :model-value="rule.model"
                :options="modelOptionsForRule(rule.id)"
                :placeholder="t('admin.groups.form.reasoningEffortModelPlaceholder')"
                :error="showValidation && !!modelValidationErrors[rule.id]?.model"
                :disabled="modelOptionsLoading"
                :aria-label="t('admin.groups.form.reasoningEffortModel')"
                :aria-describedby="
                  showValidation && modelValidationErrors[rule.id]?.model
                    ? `${idPrefix}-${rule.id}-model-error`
                    : undefined
                "
                searchable
                clearable
                @update:model-value="updateModelRule(rule.id, 'model', asString($event))"
              />
              <p
                v-if="showValidation && modelValidationErrors[rule.id]?.model"
                :id="`${idPrefix}-${rule.id}-model-error`"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ modelRuleErrorText(modelValidationErrors[rule.id]?.model) }}
              </p>
            </div>

            <div>
              <label :for="`${idPrefix}-${rule.id}-max-effort`" class="input-label">
                {{ t("admin.groups.form.maxReasoningEffort") }}
              </label>
              <Select
                :id="`${idPrefix}-${rule.id}-max-effort`"
                :model-value="rule.max_reasoning_effort"
                :options="reasoningEffortOptions"
                :placeholder="t('admin.groups.form.modelReasoningEffortUnlimited')"
                :aria-label="t('admin.groups.form.maxReasoningEffort')"
                :disabled="!rule.model"
                :searchable="false"
                clearable
                @update:model-value="
                  updateModelRule(rule.id, 'max_reasoning_effort', asString($event))
                "
              />
            </div>

            <button
              type="button"
              class="flex h-11 w-11 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/30 md:mt-6 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('admin.groups.form.removeModelReasoningEffortRule')"
              :aria-label="t('admin.groups.form.removeModelReasoningEffortRule')"
              @click="removeModelRule(rule.id)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <div class="mt-3 border-t border-gray-200 pt-3 dark:border-dark-600">
            <div class="mb-2 flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between">
              <label class="input-label mb-0">
                {{ t("admin.groups.form.reasoningEffortMappings") }}
              </label>
              <button
                type="button"
                class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg px-2 text-sm font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/30 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-400 dark:hover:bg-primary-900/20 dark:hover:text-primary-300"
                :disabled="!rule.model"
                @click="addModelRuleMapping(rule.id)"
              >
                <Icon name="plus" size="sm" />
                {{ t("admin.groups.form.addReasoningEffortMapping") }}
              </button>
            </div>

            <div
              v-for="mapping in rule.reasoning_effort_mappings"
              :key="mapping.id"
              class="grid gap-3 border-t border-gray-100 py-3 first:border-t-0 first:pt-1 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)_auto] md:items-start dark:border-dark-700"
            >
              <div>
                <label :for="`${idPrefix}-${rule.id}-${mapping.id}-from`" class="input-label">
                  {{ t("admin.groups.form.reasoningEffortFrom") }}
                </label>
                <Select
                  :id="`${idPrefix}-${rule.id}-${mapping.id}-from`"
                  :model-value="mapping.from"
                  :options="reasoningEffortOptions"
                  :placeholder="t('admin.groups.form.reasoningEffortFromPlaceholder')"
                  :error="
                    showValidation &&
                    !!modelValidationErrors[rule.id]?.mappings[mapping.id]?.from
                  "
                  :disabled="!rule.model"
                  :searchable="false"
                  clearable
                  @update:model-value="
                    updateModelRuleMapping(rule.id, mapping.id, 'from', $event)
                  "
                />
                <p
                  v-if="
                    showValidation &&
                    modelValidationErrors[rule.id]?.mappings[mapping.id]?.from
                  "
                  class="mt-1 text-xs text-red-600 dark:text-red-400"
                  role="alert"
                >
                  {{
                    mappingErrorText(
                      modelValidationErrors[rule.id]?.mappings[mapping.id]?.from,
                    )
                  }}
                </p>
              </div>

              <div class="hidden pt-8 text-gray-400 md:block dark:text-dark-400">
                <Icon name="arrowRight" size="sm" />
              </div>

              <div>
                <label :for="`${idPrefix}-${rule.id}-${mapping.id}-to`" class="input-label">
                  {{ t("admin.groups.form.reasoningEffortTo") }}
                </label>
                <Select
                  :id="`${idPrefix}-${rule.id}-${mapping.id}-to`"
                  :model-value="mapping.to"
                  :options="reasoningEffortOptions"
                  :placeholder="t('admin.groups.form.reasoningEffortToPlaceholder')"
                  :error="
                    showValidation &&
                    !!modelValidationErrors[rule.id]?.mappings[mapping.id]?.to
                  "
                  :disabled="!rule.model"
                  :searchable="false"
                  clearable
                  @update:model-value="
                    updateModelRuleMapping(rule.id, mapping.id, 'to', $event)
                  "
                />
                <p
                  v-if="
                    showValidation &&
                    modelValidationErrors[rule.id]?.mappings[mapping.id]?.to
                  "
                  class="mt-1 text-xs text-red-600 dark:text-red-400"
                  role="alert"
                >
                  {{
                    mappingErrorText(
                      modelValidationErrors[rule.id]?.mappings[mapping.id]?.to,
                    )
                  }}
                </p>
              </div>

              <button
                type="button"
                class="flex h-11 w-11 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/30 md:mt-6 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('admin.groups.form.removeReasoningEffortMapping')"
                :aria-label="t('admin.groups.form.removeReasoningEffortMapping')"
                @click="removeModelRuleMapping(rule.id, mapping.id)"
              >
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useI18n } from "vue-i18n";
import type { GroupPlatform } from "@/types";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import {
  createReasoningEffortMappingRow,
  createModelReasoningEffortRuleRow,
  reasoningEffortOptionsForPlatform,
  validateModelReasoningEffortRules,
  validateReasoningEffortMappings,
  type ModelReasoningEffortRuleErrorCode,
  type ModelReasoningEffortRuleRow,
  type ReasoningEffortMappingErrorCode,
  type ReasoningEffortMappingRow,
} from "@/views/admin/groupsReasoningEffort";

const props = defineProps<{
  idPrefix: string;
  platform: GroupPlatform;
  maxEffort: string;
  mappings: ReasoningEffortMappingRow[];
  modelRules: ModelReasoningEffortRuleRow[];
  modelOptions: string[];
  modelOptionsLoading?: boolean;
}>();

const emit = defineEmits<{
  (event: "update:maxEffort", value: string): void;
  (event: "update:mappings", value: ReasoningEffortMappingRow[]): void;
  (event: "update:modelRules", value: ModelReasoningEffortRuleRow[]): void;
}>();

const { t } = useI18n();
const showValidation = ref(false);
const reasoningEffortOptions = computed(() =>
  reasoningEffortOptionsForPlatform(props.platform),
);
const availableModelOptions = computed(() => {
  const seen = new Set<string>();
  return [...props.modelOptions, ...props.modelRules.map((rule) => rule.model)]
    .map((model) => model.trim())
    .filter((model) => {
      if (!model || seen.has(model)) return false;
      seen.add(model);
      return true;
    });
});
const validationErrors = computed(() =>
  validateReasoningEffortMappings(props.mappings, props.platform),
);
const modelValidationErrors = computed(() =>
  validateModelReasoningEffortRules(props.modelRules, props.platform),
);

const asString = (value: string | number | boolean | null): string =>
  value == null ? "" : String(value);

const modelOptionsForRule = (ruleID: string) => {
  const selectedByOtherRules = new Set(
    props.modelRules
      .filter((rule) => rule.id !== ruleID)
      .map((rule) => rule.model.trim())
      .filter(Boolean),
  );
  return availableModelOptions.value.map((model) => ({
    value: model,
    label: model,
    disabled: selectedByOtherRules.has(model),
  }));
};

const updateMaxEffort = (value: string | number | boolean | null) => {
  emit("update:maxEffort", asString(value));
};

const updateMapping = (
  id: string,
  field: "from" | "to",
  value: string | number | boolean | null,
) => {
  emit(
    "update:mappings",
    props.mappings.map((row) =>
      row.id === id ? { ...row, [field]: asString(value) } : row,
    ),
  );
};

const addMapping = () => {
  emit("update:mappings", [
    ...props.mappings,
    createReasoningEffortMappingRow(),
  ]);
};

const removeMapping = (id: string) => {
  emit(
    "update:mappings",
    props.mappings.filter((row) => row.id !== id),
  );
};

const updateModelRule = (
  id: string,
  field: "model" | "max_reasoning_effort",
  value: string,
) => {
  emit(
    "update:modelRules",
    props.modelRules.map((rule) =>
      rule.id === id ? { ...rule, [field]: value } : rule,
    ),
  );
};

const addModelRule = () => {
  emit("update:modelRules", [
    ...props.modelRules,
    createModelReasoningEffortRuleRow({}, props.platform),
  ]);
};

const removeModelRule = (id: string) => {
  emit(
    "update:modelRules",
    props.modelRules.filter((rule) => rule.id !== id),
  );
};

const updateModelRuleMapping = (
  ruleID: string,
  mappingID: string,
  field: "from" | "to",
  value: string | number | boolean | null,
) => {
  emit(
    "update:modelRules",
    props.modelRules.map((rule) =>
      rule.id === ruleID
        ? {
            ...rule,
            reasoning_effort_mappings: rule.reasoning_effort_mappings.map(
              (mapping) =>
                mapping.id === mappingID
                  ? { ...mapping, [field]: asString(value) }
                  : mapping,
            ),
          }
        : rule,
    ),
  );
};

const addModelRuleMapping = (ruleID: string) => {
  emit(
    "update:modelRules",
    props.modelRules.map((rule) =>
      rule.id === ruleID
        ? {
            ...rule,
            reasoning_effort_mappings: [
              ...rule.reasoning_effort_mappings,
              createReasoningEffortMappingRow(),
            ],
          }
        : rule,
    ),
  );
};

const removeModelRuleMapping = (ruleID: string, mappingID: string) => {
  emit(
    "update:modelRules",
    props.modelRules.map((rule) =>
      rule.id === ruleID
        ? {
            ...rule,
            reasoning_effort_mappings: rule.reasoning_effort_mappings.filter(
              (mapping) => mapping.id !== mappingID,
            ),
          }
        : rule,
    ),
  );
};

const mappingErrorText = (
  code: ReasoningEffortMappingErrorCode | undefined,
): string => (code ? t(`admin.groups.form.${code}`) : "");

const modelRuleErrorText = (
  code: ModelReasoningEffortRuleErrorCode | undefined,
): string => (code ? t(`admin.groups.form.${code}`) : "");

const validate = (): boolean => {
  showValidation.value = true;
  return (
    Object.keys(validationErrors.value).length === 0 &&
    Object.keys(modelValidationErrors.value).length === 0
  );
};

const resetValidation = () => {
  showValidation.value = false;
};

defineExpose({ validate, resetValidation });
</script>
