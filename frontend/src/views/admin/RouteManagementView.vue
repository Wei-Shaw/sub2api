<template>
  <AppLayout>
    <template v-if="!editingScheme">
      <TablePageLayout>
        <template #filters>
          <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-center">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.routeManagement.searchSchemes')"
                class="input pl-10"
              />
            </div>
            <div class="flex flex-wrap items-center justify-end gap-2">
              <button
                type="button"
                class="btn btn-secondary"
                :disabled="loading"
                :title="t('common.refresh')"
                @click="() => loadSchemes()"
              >
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <button type="button" class="btn btn-primary" @click="openCreateDialog">
                <Icon name="plus" size="md" class="mr-1" />
                {{ t("admin.routeManagement.create") }}
              </button>
            </div>
          </div>
        </template>

        <template #table>
          <DataTable :columns="columns" :data="filteredSchemes" :loading="loading">
            <template #cell-name="{ row }">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
            </template>
            <template #cell-actions="{ row }">
              <div class="flex items-center gap-1">
                <button
                  type="button"
                  class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                  :title="t('common.edit')"
                  @click="openDetail(row)"
                >
                  <Icon name="edit" size="sm" />
                  <span class="text-xs">{{ t("common.edit") }}</span>
                </button>
                <button
                  type="button"
                  class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                  :title="t('admin.routeManagement.duplicate')"
                  @click="openDuplicateDialog(row)"
                >
                  <Icon name="copy" size="sm" />
                  <span class="text-xs">{{ t("admin.routeManagement.duplicate") }}</span>
                </button>
                <button
                  type="button"
                  class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                  :title="t('common.delete')"
                  @click="confirmDeleteScheme(row)"
                >
                  <Icon name="trash" size="sm" />
                  <span class="text-xs">{{ t("common.delete") }}</span>
                </button>
              </div>
            </template>
            <template #empty>
              <EmptyState
                :title="t('admin.routeManagement.empty')"
                :description="t('admin.routeManagement.emptyHint')"
                :action-text="t('admin.routeManagement.create')"
                @action="openCreateDialog"
              />
            </template>
          </DataTable>
        </template>
      </TablePageLayout>
    </template>

    <div v-else class="space-y-5">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
        <div class="min-w-0">
          <button
            type="button"
            class="mb-2 inline-flex items-center gap-1 text-sm text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200"
            @click="closeDetail"
          >
            <Icon name="arrowLeft" size="sm" />
            {{ t("admin.routeManagement.backToList") }}
          </button>
          <h1 class="truncate text-xl font-semibold text-gray-900 dark:text-white">
            {{ editingScheme.name }}
          </h1>
          <p
            v-if="editingScheme.description"
            class="mt-1 text-sm text-gray-500 dark:text-gray-400"
          >
            {{ editingScheme.description }}
          </p>
        </div>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <button type="button" class="btn btn-secondary" @click="openRenameDialog(editingScheme)">
            <Icon name="edit" size="sm" class="mr-2" />
            {{ t("admin.routeManagement.rename") }}
          </button>
        </div>
      </div>

      <div class="card p-4">
        <CompositeRouteEditor :scheme-id="editingScheme.id" @changed="refreshEditingScheme" />
      </div>
    </div>

    <BaseDialog
      :show="showSchemeDialog"
      :title="schemeDialogTitle"
      @close="closeSchemeDialog"
    >
      <form class="space-y-4" @submit.prevent="saveSchemeDialog">
        <div>
          <label class="input-label">{{ t("admin.routeManagement.schemeName") }}</label>
          <input
            v-model.trim="schemeForm.name"
            type="text"
            class="input"
            required
            :placeholder="t('admin.routeManagement.schemeNamePlaceholder')"
          />
        </div>
        <div v-if="schemeDialogMode !== 'duplicate'">
          <label class="input-label">{{ t("admin.routeManagement.schemeDescription") }}</label>
          <textarea
            v-model.trim="schemeForm.description"
            rows="3"
            class="input"
            :placeholder="t('admin.routeManagement.optionalDescription')"
          ></textarea>
        </div>
        <div v-if="schemeDialogMode === 'create'">
          <label class="input-label">{{ t("admin.routeManagement.copyFrom") }}</label>
          <Select
            v-model="schemeForm.copy_from_scheme_id"
            :options="copyFromOptions"
          />
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.routeManagement.copyFromHint") }}
          </p>
        </div>
        <div class="flex justify-end gap-2 pt-2">
          <button type="button" class="btn btn-secondary" @click="closeSchemeDialog">
            {{ t("common.cancel") }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="schemeSaving">
            {{ t("common.save") }}
          </button>
        </div>
      </form>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useAppStore } from "@/stores/app";
import { adminAPI } from "@/api/admin";
import type { CompositeRouteScheme } from "@/types";
import type { Column } from "@/components/common/types";
import AppLayout from "@/components/layout/AppLayout.vue";
import TablePageLayout from "@/components/layout/TablePageLayout.vue";
import DataTable from "@/components/common/DataTable.vue";
import EmptyState from "@/components/common/EmptyState.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import Select from "@/components/common/Select.vue";
import Icon from "@/components/icons/Icon.vue";
import CompositeRouteEditor from "@/components/admin/group/CompositeRouteEditor.vue";

const { t } = useI18n();
const appStore = useAppStore();

const schemes = ref<CompositeRouteScheme[]>([]);
const searchQuery = ref("");
const editingScheme = ref<CompositeRouteScheme | null>(null);
const loading = ref(false);
const showSchemeDialog = ref(false);
const schemeDialogMode = ref<"create" | "rename" | "duplicate">("create");
const schemeSaving = ref(false);
const dialogTarget = ref<CompositeRouteScheme | null>(null);
const schemeForm = reactive({
  name: "",
  description: "",
  copy_from_scheme_id: null as number | null,
});

const columns = computed<Column[]>(() => [
  { key: "name", label: t("admin.routeManagement.columns.name") },
  { key: "actions", label: t("admin.routeManagement.columns.actions"), class: "w-48" },
]);

const filteredSchemes = computed(() => {
  const keyword = searchQuery.value.trim().toLowerCase();
  if (!keyword) return schemes.value;
  return schemes.value.filter((scheme) => scheme.name.toLowerCase().includes(keyword));
});

const copyFromOptions = computed(() => [
  { value: null, label: t("admin.routeManagement.copyFromNone") },
  ...schemes.value.map((scheme) => ({
    value: scheme.id,
    label: scheme.name,
  })),
]);

const schemeDialogTitle = computed(() => {
  if (schemeDialogMode.value === "rename") return t("admin.routeManagement.rename");
  if (schemeDialogMode.value === "duplicate") return t("admin.routeManagement.duplicate");
  return t("admin.routeManagement.create");
});

const loadSchemes = async () => {
  loading.value = true;
  try {
    schemes.value = await adminAPI.routeSchemes.list();
    if (editingScheme.value) {
      editingScheme.value =
        schemes.value.find((scheme) => scheme.id === editingScheme.value?.id) ||
        editingScheme.value;
    }
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.routeManagement.failedToLoad"),
    );
  } finally {
    loading.value = false;
  }
};

const refreshEditingScheme = async () => {
  await loadSchemes();
};

const openDetail = (scheme: CompositeRouteScheme) => {
  editingScheme.value = scheme;
};

const closeDetail = () => {
  editingScheme.value = null;
  void loadSchemes();
};

const openCreateDialog = () => {
  schemeDialogMode.value = "create";
  dialogTarget.value = null;
  schemeForm.name = "";
  schemeForm.description = "";
  schemeForm.copy_from_scheme_id = null;
  showSchemeDialog.value = true;
};

const openRenameDialog = (scheme: CompositeRouteScheme) => {
  schemeDialogMode.value = "rename";
  dialogTarget.value = scheme;
  schemeForm.name = scheme.name;
  schemeForm.description = scheme.description || "";
  schemeForm.copy_from_scheme_id = null;
  showSchemeDialog.value = true;
};

const openDuplicateDialog = (scheme: CompositeRouteScheme) => {
  schemeDialogMode.value = "duplicate";
  dialogTarget.value = scheme;
  schemeForm.name = `${scheme.name} (Copy)`;
  schemeForm.description = scheme.description || "";
  schemeForm.copy_from_scheme_id = scheme.id;
  showSchemeDialog.value = true;
};

const closeSchemeDialog = () => {
  showSchemeDialog.value = false;
  dialogTarget.value = null;
};

const saveSchemeDialog = async () => {
  if (!schemeForm.name.trim()) {
    appStore.showError(t("admin.routeManagement.nameRequired"));
    return;
  }
  schemeSaving.value = true;
  try {
    if (schemeDialogMode.value === "rename" && dialogTarget.value) {
      const updated = await adminAPI.routeSchemes.update(dialogTarget.value.id, {
        name: schemeForm.name.trim(),
        description: schemeForm.description.trim(),
      });
      appStore.showSuccess(t("admin.routeManagement.updated"));
      closeSchemeDialog();
      await loadSchemes();
      if (editingScheme.value?.id === updated.id) {
        editingScheme.value = updated;
      }
    } else if (schemeDialogMode.value === "duplicate" && dialogTarget.value) {
      const duplicated = await adminAPI.routeSchemes.duplicate(
        dialogTarget.value.id,
        schemeForm.name.trim(),
      );
      appStore.showSuccess(t("admin.routeManagement.duplicated"));
      closeSchemeDialog();
      await loadSchemes();
      openDetail(duplicated);
    } else {
      const created = await adminAPI.routeSchemes.create({
        name: schemeForm.name.trim(),
        description: schemeForm.description.trim(),
        copy_from_scheme_id: schemeForm.copy_from_scheme_id || undefined,
      });
      appStore.showSuccess(t("admin.routeManagement.created"));
      closeSchemeDialog();
      await loadSchemes();
      openDetail(created);
    }
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.routeManagement.failedToSave"),
    );
  } finally {
    schemeSaving.value = false;
  }
};

const confirmDeleteScheme = async (scheme: CompositeRouteScheme) => {
  if (scheme.group_count > 0) {
    appStore.showError(
      t("admin.routeManagement.deleteInUse", {
        count: scheme.group_count,
      }),
    );
    return;
  }
  if (!window.confirm(t("admin.routeManagement.deleteConfirm", { name: scheme.name }))) {
    return;
  }
  try {
    await adminAPI.routeSchemes.delete(scheme.id);
    appStore.showSuccess(t("admin.routeManagement.deleted"));
    if (editingScheme.value?.id === scheme.id) {
      editingScheme.value = null;
    }
    await loadSchemes();
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail ||
        error.response?.data?.message ||
        t("admin.routeManagement.failedToDelete"),
    );
  }
};

onMounted(() => {
  loadSchemes();
});
</script>
