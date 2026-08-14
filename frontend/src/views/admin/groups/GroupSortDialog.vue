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
          class="flex cursor-grab items-center gap-3 rounded-sm border border-line bg-surface p-3 transition-colors hover:bg-surface-hover active:cursor-grabbing"
        >
          <div class="text-ink-tertiary">
            <Icon name="menu" size="md" />
          </div>
          <div class="flex-1">
            <div class="font-medium text-ink">
              {{ group.name }}
            </div>
            <div class="text-xs text-ink-secondary">
              <span class="badge badge-gray">
                <PlatformIcon :platform="group.platform" size="xs" />
                {{ t("admin.groups.platforms." + group.platform) }}
              </span>
            </div>
          </div>
          <div class="font-mono text-sm tabular-nums text-ink-tertiary">
            #{{ group.id }}
          </div>
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
          <span v-if="sortSubmitting" class="spinner -ml-1 mr-2" />
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
import PlatformIcon from "@/components/common/PlatformIcon.vue";
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
