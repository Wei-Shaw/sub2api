<template>
  <AppLayout>
    <GroupsTable />

    <!-- Create Group Modal -->
    <GroupCreateDialog />

    <!-- Edit Group Modal -->
    <GroupEditDialog />

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.groups.deleteGroup')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <ConfirmDialog
      :show="showUnsupportedLiveConfirm"
      :title="t('admin.groups.openaiLive.unsupportedTitle')"
      :message="t('admin.groups.openaiLive.unsupportedMessage')"
      :confirm-text="t('admin.groups.openaiLive.enableAnyway')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmUnsupportedLive"
      @cancel="cancelUnsupportedLive"
    />

    <!-- Sort Order Modal -->
    <GroupSortDialog />

    <!-- Composite Routes Modal -->
    <CompositeRoutesDialog />

    <!-- Group Rate Multipliers Modal -->
    <GroupRateMultipliersModal
      :show="showRateMultipliersModal"
      :group="rateMultipliersGroup"
      @close="showRateMultipliersModal = false"
      @success="loadGroups"
    />

    <!-- Group RPM Overrides Modal -->
    <GroupRPMOverridesModal
      :show="showRPMOverridesModal"
      :group="rpmOverridesGroup"
      @close="showRPMOverridesModal = false"
      @success="loadGroups"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import GroupsTable from "./groups/GroupsTable.vue";
import GroupCreateDialog from "./groups/GroupCreateDialog.vue";
import GroupEditDialog from "./groups/GroupEditDialog.vue";
import GroupSortDialog from "./groups/GroupSortDialog.vue";
import CompositeRoutesDialog from "./groups/CompositeRoutesDialog.vue";
import { provide } from "vue";
import AppLayout from "@/components/layout/AppLayout.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import GroupRateMultipliersModal from "@/components/admin/group/GroupRateMultipliersModal.vue";
import GroupRPMOverridesModal from "@/components/admin/group/GroupRPMOverridesModal.vue";
import { GROUPS_VIEW_CONTEXT } from "./groups/context";
import { useGroupsView } from "./groups/useGroupsView";

const ctx = useGroupsView();
provide(GROUPS_VIEW_CONTEXT, ctx);

const {
  t,
  showDeleteDialog,
  showUnsupportedLiveConfirm,
  showRateMultipliersModal,
  rateMultipliersGroup,
  showRPMOverridesModal,
  rpmOverridesGroup,
  deleteConfirmMessage,
  confirmUnsupportedLive,
  cancelUnsupportedLive,
  loadGroups,
  confirmDelete,
} = ctx;
</script>
