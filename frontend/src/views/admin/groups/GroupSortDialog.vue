<template>
  <BaseDialog
    :show="showSortModal"
    :title="t('admin.groups.sortOrder')"
    width="normal"
    @close="closeSortModal"
  >
    <div class="space-y-4">
      <p class="text-sm text-ink-secondary">
        {{ t("admin.groups.sortOrderHint") }}
      </p>
      <VueDraggable
        v-model="sortableGroups"
        :animation="200"
        class="space-y-2"
      >
        <div
          v-for="group in sortableGroups"
          :key="group.id"
          class="flex cursor-grab items-center gap-3 rounded-lg border border-line bg-surface p-3 transition-shadow hover:shadow-md active:cursor-grabbing"
        >
          <div class="text-ink-tertiary">
            <Icon name="menu" size="md" />
          </div>
          <div class="flex-1">
            <div class="font-medium text-ink">
              {{ group.name }}
            </div>
            <div class="text-xs text-ink-secondary">
              <span
                :class="[
                  'inline-flex items-center gap-1 rounded-sm px-2 py-0.5 text-xs font-medium',
                  group.platform === 'anthropic'
                    ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                    : group.platform === 'openai'
                      ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                      : group.platform === 'antigravity'
                        ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                        : group.platform === 'grok'
                          ? 'bg-zinc-200 text-zinc-800 dark:bg-zinc-700 dark:text-zinc-100'
                          : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
                ]"
              >
                {{ t("admin.groups.platforms." + group.platform) }}
              </span>
            </div>
          </div>
          <div class="text-sm text-ink-tertiary">#{{ group.id }}</div>
        </div>
      </VueDraggable>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3 pt-4">
        <button
          @click="closeSortModal"
          type="button"
          class="btn btn-secondary"
        >
          {{ t("common.cancel") }}
        </button>
        <button
          @click="saveSortOrder"
          :disabled="sortSubmitting"
          class="btn btn-primary"
        >
          <svg
            v-if="sortSubmitting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{ sortSubmitting ? t("common.saving") : t("common.save") }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import BaseDialog from "@/components/common/BaseDialog.vue";
import { VueDraggable } from "vue-draggable-plus";
import Icon from "@/components/icons/Icon.vue";
import { useGroupsViewContext } from "./context";

const ctx = useGroupsViewContext();

const {
  t,
  showSortModal,
  sortSubmitting,
  sortableGroups,
  closeSortModal,
  saveSortOrder,
} = ctx;
</script>
