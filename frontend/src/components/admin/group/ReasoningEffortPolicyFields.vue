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

    <div>
      <label :for="`${idPrefix}-over-limit`" class="input-label">
        {{ t("admin.groups.form.maxReasoningEffortOverLimit") }}
      </label>
      <Select
        :id="`${idPrefix}-over-limit`"
        :model-value="overLimit"
        :options="overLimitOptions"
        :aria-label="t('admin.groups.form.maxReasoningEffortOverLimit')"
        :searchable="false"
        :disabled="!maxEffort"
        @update:model-value="updateOverLimit"
      />
      <p class="input-hint">{{ t("admin.groups.form.maxReasoningEffortOverLimitHint") }}</p>
    </div>

    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between gap-3">
        <div>
          <label class="input-label mb-0">
            {{ t("admin.groups.form.reasoningEffortMappings") }}
          </label>
          <p class="input-hint mb-0">
            {{ t("admin.groups.form.reasoningEffortMappingsHint") }}
          </p>
        </div>
        <button
          type="button"
          class="inline-flex min-h-11 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-primary-600 transition-colors hover:bg-primary-50 hover:text-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:text-primary-400 dark:hover:bg-primary-900/20 dark:hover:text-primary-300"
          @click="addGroup"
        >
          <Icon name="plus" size="sm" />
          {{ t("admin.groups.form.addReasoningEffortMapping") }}
        </button>
      </div>

      <div v-if="mappings.length > 0" class="space-y-2">
        <div
          v-for="group in mappings"
          :key="group.id"
          class="space-y-3 rounded-lg border border-gray-200 bg-gray-50/40 p-3 dark:border-dark-600 dark:bg-dark-800/40"
        >
          <div
            class="grid grid-cols-1 items-start gap-3 md:grid-cols-[minmax(0,1fr)_1.25rem_minmax(0,1fr)_2.75rem]"
          >
            <div>
              <label :for="`${idPrefix}-${group.id}-match-type`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortMatchType") }}
              </label>
              <Select
                :id="`${idPrefix}-${group.id}-match-type`"
                :model-value="group.match_type"
                :options="matchTypeOptions"
                :placeholder="t('admin.groups.form.reasoningEffortMatchTypePlaceholder')"
                :error="showValidation && !!groupErrors(group.id).match_type"
                :aria-label="t('admin.groups.form.reasoningEffortMatchType')"
                :searchable="false"
                clearable
                @update:model-value="updateGroup(group.id, 'match_type', $event)"
              />
              <p
                v-if="showValidation && groupErrors(group.id).match_type"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ mappingErrorText(groupErrors(group.id).match_type) }}
              </p>
            </div>

            <div class="hidden md:block" aria-hidden="true" />

            <div>
              <label :for="`${idPrefix}-${group.id}-model`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortModel") }}
              </label>
              <input
                :id="`${idPrefix}-${group.id}-model`"
                :value="group.model"
                type="text"
                maxlength="200"
                autocomplete="off"
                class="input"
                :placeholder="t('admin.groups.form.reasoningEffortModelPlaceholder')"
                :aria-label="t('admin.groups.form.reasoningEffortModel')"
                @input="onModelInput(group.id, $event)"
              />
            </div>

            <button
              type="button"
              class="flex h-11 w-11 items-center justify-center self-end rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/30 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('admin.groups.form.removeReasoningEffortMapping')"
              :aria-label="t('admin.groups.form.removeReasoningEffortMapping')"
              @click="removeGroup(group.id)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <p
            v-if="showValidation && groupErrors(group.id).duplicateScope"
            class="text-xs text-red-600 dark:text-red-400"
            role="alert"
          >
            {{ mappingErrorText(groupErrors(group.id).duplicateScope) }}
          </p>

          <div
            v-for="pair in group.pairs"
            :key="pair.id"
            class="grid grid-cols-1 items-start gap-3 md:grid-cols-[minmax(0,1fr)_1.25rem_minmax(0,1fr)_2.75rem]"
          >
            <div>
              <label :for="`${idPrefix}-${pair.id}-from`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortFrom") }}
              </label>
              <Select
                :id="`${idPrefix}-${pair.id}-from`"
                :model-value="pair.from"
                :options="reasoningEffortOptions"
                :placeholder="t('admin.groups.form.reasoningEffortFromPlaceholder')"
                :error="showValidation && !!pairErrors(pair.id).from"
                :aria-label="t('admin.groups.form.reasoningEffortFrom')"
                :searchable="false"
                clearable
                @update:model-value="updatePair(group.id, pair.id, 'from', $event)"
              />
              <p
                v-if="showValidation && pairErrors(pair.id).from"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ mappingErrorText(pairErrors(pair.id).from) }}
              </p>
            </div>

            <div class="hidden h-11 items-center justify-center self-end text-gray-400 md:flex dark:text-dark-400">
              <Icon name="arrowRight" size="sm" />
            </div>

            <div>
              <label :for="`${idPrefix}-${pair.id}-to`" class="input-label">
                {{ t("admin.groups.form.reasoningEffortTo") }}
              </label>
              <Select
                :id="`${idPrefix}-${pair.id}-to`"
                :model-value="pair.to"
                :options="reasoningEffortOptions"
                :placeholder="t('admin.groups.form.reasoningEffortToPlaceholder')"
                :error="showValidation && !!pairErrors(pair.id).to"
                :aria-label="t('admin.groups.form.reasoningEffortTo')"
                :searchable="false"
                clearable
                @update:model-value="updatePair(group.id, pair.id, 'to', $event)"
              />
              <p
                v-if="showValidation && pairErrors(pair.id).to"
                class="mt-1 text-xs text-red-600 dark:text-red-400"
                role="alert"
              >
                {{ mappingErrorText(pairErrors(pair.id).to) }}
              </p>
            </div>

            <button
              type="button"
              class="flex h-11 w-11 items-center justify-center self-end rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 focus:outline-none focus:ring-2 focus:ring-red-500/30 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('admin.groups.form.removeReasoningEffortPair')"
              :aria-label="t('admin.groups.form.removeReasoningEffortPair')"
              @click="removePair(group.id, pair.id)"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <button
            type="button"
            class="inline-flex min-h-9 items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/30 dark:text-primary-400 dark:hover:text-primary-300"
            @click="addPair(group.id)"
          >
            <Icon name="plus" size="sm" />
            {{ t("admin.groups.form.addReasoningEffortPair") }}
          </button>
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
  createReasoningEffortMappingPair,
  createReasoningEffortMappingRow,
  normalizeReasoningEffortMatchType,
  reasoningEffortOptionsForPlatform,
  reasoningEffortOverLimitDeny,
  reasoningEffortOverLimitDowngrade,
  validateReasoningEffortMappings,
  type ReasoningEffortMappingErrorCode,
  type ReasoningEffortMappingRow,
} from "@/views/admin/groupsReasoningEffort";

const props = defineProps<{
  idPrefix: string;
  platform: GroupPlatform;
  maxEffort: string;
  overLimit: string;
  mappings: ReasoningEffortMappingRow[];
}>();

const emit = defineEmits<{
  (event: "update:maxEffort", value: string): void;
  (event: "update:overLimit", value: string): void;
  (event: "update:mappings", value: ReasoningEffortMappingRow[]): void;
}>();

const { t } = useI18n();
const showValidation = ref(false);
const reasoningEffortOptions = computed(() =>
  reasoningEffortOptionsForPlatform(props.platform),
);
const matchTypeOptions = computed(() => [
  {
    value: "exact",
    label: t("admin.groups.form.reasoningEffortMatchExact"),
  },
  {
    value: "prefix",
    label: t("admin.groups.form.reasoningEffortMatchPrefix"),
  },
  {
    value: "suffix",
    label: t("admin.groups.form.reasoningEffortMatchSuffix"),
  },
]);
const overLimitOptions = computed(() => [
  {
    value: reasoningEffortOverLimitDowngrade,
    label: t("admin.groups.form.maxReasoningEffortOverLimitDowngrade"),
  },
  {
    value: reasoningEffortOverLimitDeny,
    label: t("admin.groups.form.maxReasoningEffortOverLimitDeny"),
  },
]);
const validationErrors = computed(() =>
  validateReasoningEffortMappings(props.mappings, props.platform),
);

const asString = (value: string | number | boolean | null): string =>
  value == null ? "" : String(value);

const groupErrors = (id: string) => validationErrors.value[id] ?? {};
const pairErrors = (id: string) => validationErrors.value[id] ?? {};

const updateMaxEffort = (value: string | number | boolean | null) => {
  emit("update:maxEffort", asString(value));
};

const updateOverLimit = (value: string | number | boolean | null) => {
  emit(
    "update:overLimit",
    asString(value) || reasoningEffortOverLimitDowngrade,
  );
};

const updateGroup = (
  id: string,
  field: "match_type" | "model",
  value: string | number | boolean | null,
) => {
  const nextValue =
    field === "match_type"
      ? normalizeReasoningEffortMatchType(asString(value))
      : asString(value);
  emit(
    "update:mappings",
    props.mappings.map((group) =>
      group.id === id ? { ...group, [field]: nextValue } : group,
    ),
  );
};

const onModelInput = (id: string, event: Event) => {
  const target = event.target as HTMLInputElement | null;
  updateGroup(id, "model", target?.value ?? "");
};

const updatePair = (
  groupId: string,
  pairId: string,
  field: "from" | "to",
  value: string | number | boolean | null,
) => {
  emit(
    "update:mappings",
    props.mappings.map((group) =>
      group.id === groupId
        ? {
            ...group,
            pairs: group.pairs.map((pair) =>
              pair.id === pairId ? { ...pair, [field]: asString(value) } : pair,
            ),
          }
        : group,
    ),
  );
};

const addGroup = () => {
  emit("update:mappings", [
    ...props.mappings,
    createReasoningEffortMappingRow(),
  ]);
};

const removeGroup = (id: string) => {
  emit(
    "update:mappings",
    props.mappings.filter((group) => group.id !== id),
  );
};

const addPair = (groupId: string) => {
  emit(
    "update:mappings",
    props.mappings.map((group) =>
      group.id === groupId
        ? { ...group, pairs: [...group.pairs, createReasoningEffortMappingPair()] }
        : group,
    ),
  );
};

const removePair = (groupId: string, pairId: string) => {
  emit(
    "update:mappings",
    props.mappings.flatMap((group) => {
      if (group.id !== groupId) return [group];
      const pairs = group.pairs.filter((pair) => pair.id !== pairId);
      return pairs.length > 0 ? [{ ...group, pairs }] : [];
    }),
  );
};

const mappingErrorText = (
  code: ReasoningEffortMappingErrorCode | undefined,
): string => (code ? t(`admin.groups.form.${code}`) : "");

const validate = (): boolean => {
  showValidation.value = true;
  return Object.keys(validationErrors.value).length === 0;
};

const resetValidation = () => {
  showValidation.value = false;
};

defineExpose({ validate, resetValidation });
</script>
