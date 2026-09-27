<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div
          class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start"
        >
          <!-- Left: fuzzy search + filters (can wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-fg-subtle"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.groups.searchGroups')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
            <Select
              v-model="filters.platform"
              :options="platformFilterOptions"
              :placeholder="t('admin.groups.allPlatforms')"
              class="w-44"
              @change="loadGroups"
            />
            <Select
              v-model="filters.status"
              :options="statusOptions"
              :placeholder="t('admin.groups.allStatus')"
              class="w-40"
              @change="loadGroups"
            />
            <Select
              v-if="!authStore.isSimpleMode"
              v-model="filters.is_exclusive"
              :options="exclusiveOptions"
              :placeholder="t('admin.groups.allGroups')"
              class="w-44"
              @change="loadGroups"
            />
          </div>

          <!-- Right: actions -->
          <div
            class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto"
          >
            <button
              @click="loadGroups"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')" :aria-label="t('common.refresh')"
            >
              <Icon
                name="refresh"
                size="md"
                :class="loading ? 'animate-spin' : ''"
              />
            </button>
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary"
                :title="t('admin.groups.columnSettings')"
              >
                <Icon name="grid" size="md" class="mr-2" />
                <span class="hidden md:inline">{{
                  t("admin.groups.columnSettings")
                }}</span>
              </button>
              <div
                v-if="showColumnDropdown"
                class="dropdown right-0 top-full mt-1 max-h-80 w-48 overflow-y-auto"
              >
                <button
                  v-for="col in toggleableColumns"
                  :key="col.key"
                  @click="toggleColumn(col.key)"
                  class="dropdown-item w-full justify-between text-left"
                >
                  <span>{{ col.label }}</span>
                  <Icon
                    v-if="isColumnVisible(col.key)"
                    name="check"
                    size="sm"
                    class="text-accent"
                    :stroke-width="2"
                  />
                </button>
              </div>
            </div>
            <button
              v-if="!authStore.isSimpleMode"
              @click="openSortModal"
              class="btn btn-secondary"
              :title="t('admin.groups.sortOrder')"
            >
              <Icon name="arrowsUpDown" size="md" class="mr-2" />
              {{ t("admin.groups.sortOrder") }}
            </button>
            <button
              @click="openCreateModal"
              class="btn btn-primary"
              data-tour="groups-create-btn"
            >
              <Icon name="plus" size="md" class="mr-2" />
              {{ t("admin.groups.createGroup") }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="groups"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="sort_order"
          default-sort-order="asc"
          @sort="handleSort"
        >
          <template #cell-name="{ value }">
            <span class="font-medium text-fg">{{
              value
            }}</span>
          </template>

          <template #cell-id="{ value }">
            <span class="font-mono text-xs text-fg-muted"
              >#{{ value }}</span
            >
          </template>

          <template #cell-platform="{ value }">
            <span class="badge badge-gray gap-1.5 text-fg">
              <PlatformIcon :platform="value" size="xs" />
              {{ t("admin.groups.platforms." + value) }}
            </span>
          </template>

          <template #cell-billing_type="{ row }">
            <div class="space-y-1">
              <!-- Type Badge -->
              <span
                :class="[
                  'badge',
                  row.subscription_type === 'subscription'
                    ? 'badge-primary'
                    : 'badge-gray',
                ]"
              >
                {{
                  row.subscription_type === "subscription"
                    ? t("admin.groups.subscription.subscription")
                    : t("admin.groups.subscription.standard")
                }}
              </span>
              <!-- Subscription Limits - compact single line -->
              <div
                v-if="row.subscription_type === 'subscription'"
                class="space-y-0.5 text-xs text-fg-muted"
              >
                <div
                  v-if="
                    row.daily_limit_usd ||
                    row.weekly_limit_usd ||
                    row.monthly_limit_usd
                  "
                  class="flex flex-wrap items-center gap-x-1 gap-y-0.5"
                >
                  <span v-if="row.daily_limit_usd" class="whitespace-nowrap">
                    <span
                      v-if="usageLoading"
                      class="font-medium text-fg-subtle"
                      >—</span
                    >
                    <span
                      v-else
                      :class="
                        getQuotaUsageClass(
                          usageMap.get(row.id)?.today_cost ?? 0,
                          row.daily_limit_usd
                        )
                      "
                      >{{
                        formatUsd(usageMap.get(row.id)?.today_cost ?? 0)
                      }}</span
                    >
                    <span class="text-fg-subtle">
                      / {{ formatUsd(row.daily_limit_usd) }}/{{
                        t("admin.groups.limitDay")
                      }}</span
                    >
                  </span>
                  <span
                    v-if="
                      row.daily_limit_usd &&
                      (row.weekly_limit_usd || row.monthly_limit_usd)
                    "
                    class="mx-1 text-fg-subtle"
                    >·</span
                  >
                  <span v-if="row.weekly_limit_usd" class="whitespace-nowrap"
                    >{{ formatUsd(row.weekly_limit_usd) }}/{{
                      t("admin.groups.limitWeek")
                    }}</span
                  >
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-fg-subtle"
                    >·</span
                  >
                  <span v-if="row.monthly_limit_usd" class="whitespace-nowrap"
                    >{{ formatUsd(row.monthly_limit_usd) }}/{{
                      t("admin.groups.limitMonth")
                    }}</span
                  >
                </div>
                <span v-else class="text-fg-subtle">{{
                  t("admin.groups.subscription.noLimit")
                }}</span>
                <div class="text-fg-subtle">
                  {{ t("admin.groups.usageTotal") }}
                  <span class="ml-1 font-medium text-fg-muted"
                    >{{
                      usageLoading
                        ? "—"
                        : formatUsd(usageMap.get(row.id)?.total_cost ?? 0)
                    }}</span
                  >
                </div>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value }">
            <span class="text-sm font-semibold tabular-nums text-fg"
              >×{{ value }}</span
            >
          </template>

          <template #cell-is_exclusive="{ value }">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
              {{
                value ? t("admin.groups.exclusive") : t("admin.groups.public")
              }}
            </span>
          </template>

          <template #cell-account_count="{ row }">
            <div class="space-y-0.5 text-xs tabular-nums">
              <RouterLink
                :to="{ path: '/admin/accounts', query: { group: String(row.id) } }"
                class="block hover:underline"
                data-testid="group-available-accounts-link"
              >
                <span class="text-fg-muted">{{
                  t("admin.groups.accountsAvailable")
                }}</span>
                <span
                  :class="['ml-1 font-medium', row.active_account_count ? 'text-success' : 'text-danger']"
                  >{{ row.active_account_count || 0 }}</span
                >
                <span
                  class="ml-0.5 text-fg-subtle"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </RouterLink>
              <div v-if="row.rate_limited_account_count">
                <span class="text-fg-muted">{{
                  t("admin.groups.accountsRateLimited")
                }}</span>
                <span
                  class="ml-1 font-medium text-warning"
                  >{{ row.rate_limited_account_count }}</span
                >
                <span
                  class="ml-0.5 text-fg-subtle"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
              <div>
                <span class="text-fg-muted">{{
                  t("admin.groups.accountsTotal")
                }}</span>
                <span
                  class="ml-1 font-medium text-fg"
                  >{{ row.account_count || 0 }}</span
                >
                <span
                  class="ml-0.5 text-fg-subtle"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
            </div>
          </template>

          <template #cell-capacity="{ row }">
            <GroupCapacityBadge
              v-if="capacityMap.get(row.id)"
              :concurrency-used="capacityMap.get(row.id)!.concurrencyUsed"
              :concurrency-max="capacityMap.get(row.id)!.concurrencyMax"
              :sessions-used="capacityMap.get(row.id)!.sessionsUsed"
              :sessions-max="capacityMap.get(row.id)!.sessionsMax"
              :rpm-used="capacityMap.get(row.id)!.rpmUsed"
              :rpm-max="capacityMap.get(row.id)!.rpmMax"
            />
            <span v-else class="text-xs text-fg-subtle">—</span>
          </template>

          <template #cell-usage="{ row }">
            <div v-if="usageLoading" class="text-xs text-fg-subtle">—</div>
            <div v-else class="grid grid-cols-[auto_auto] gap-x-2 text-xs tabular-nums">
              <span class="text-fg-subtle">{{ t("admin.groups.usageToday") }}</span>
              <span class="text-right font-medium text-fg">${{
                    formatCost(usageMap.get(row.id)?.today_cost ?? 0)
                  }}</span>
              <span class="text-fg-subtle">{{ t("admin.groups.usageYesterday") }}</span>
              <span class="text-right font-medium text-fg">${{
                    formatCost(usageMap.get(row.id)?.yesterday_cost ?? 0)
                  }}</span>
              <span class="text-fg-subtle">{{ t("admin.groups.usageTotal") }}</span>
              <span class="text-right font-medium text-fg">${{
                    formatCost(usageMap.get(row.id)?.total_cost ?? 0)
                  }}</span>
            </div>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active' ? 'badge-success' : 'badge-danger',
              ]"
            >
              {{ t("admin.accounts.status." + value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t("common.edit") }}</span>
              </button>
              <button
                v-if="!authStore.isSimpleMode"
                data-testid="group-duplicate"
                :title="
                  duplicatingGroupIds.has(row.id)
                    ? t('admin.groups.duplicating')
                    : t('admin.groups.duplicate')
                "
                :disabled="duplicatingGroupIds.has(row.id)"
                @click="handleDuplicate(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="copy" size="sm" />
                <span class="text-xs">
                  {{
                    duplicatingGroupIds.has(row.id)
                      ? t("admin.groups.duplicating")
                      : t("admin.groups.duplicate")
                  }}
                </span>
              </button>
              <button
                v-if="!authStore.isSimpleMode && row.platform === 'composite'"
                data-testid="group-composite-routes"
                @click="handleCompositeRoutes(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
              >
                <Icon name="swap" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.compositeRoutes.action")
                }}</span>
              </button>
              <button
                v-if="!authStore.isSimpleMode"
                data-testid="group-rate-multipliers"
                @click="handleRateMultipliers(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
              >
                <Icon name="dollar" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rateMultipliers")
                }}</span>
              </button>
              <button
                v-if="!authStore.isSimpleMode"
                data-testid="group-rpm-overrides"
                @click="handleRPMOverrides(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-accent-weak hover:text-accent-strong"
              >
                <Icon name="bolt" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rpmOverrides")
                }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-sm p-1.5 text-fg-muted transition-colors hover:bg-danger-weak hover:text-danger"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t("common.delete") }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.groups.noGroupsYet')"
              :description="t('admin.groups.createFirstGroup')"
              :action-text="t('admin.groups.createGroup')"
              @action="openCreateModal"
            />
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :total="pagination.total"
          :page-size="pagination.page_size"
          @update:page="handlePageChange"
          @update:pageSize="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <GroupCreateModal />

    <GroupEditModal />

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

    <GroupSortOrderModal />

    <GroupCompositeRoutesModal />

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
import { useGroupsView } from "./groups/useGroupsView";
import { provideGroupsViewContext } from "./groups/context";
import AppLayout from "@/components/layout/AppLayout.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import DataTable from "@/components/common/DataTable.vue";
import EmptyState from "@/components/common/EmptyState.vue";
import GroupCapacityBadge from "@/components/common/GroupCapacityBadge.vue";
import GroupCompositeRoutesModal from "./groups/GroupCompositeRoutesModal.vue";
import GroupCreateModal from "./groups/GroupCreateModal.vue";
import GroupEditModal from "./groups/GroupEditModal.vue";
import GroupRPMOverridesModal from "@/components/admin/group/GroupRPMOverridesModal.vue";
import GroupRateMultipliersModal from "@/components/admin/group/GroupRateMultipliersModal.vue";
import GroupSortOrderModal from "./groups/GroupSortOrderModal.vue";
import Icon from "@/components/icons/Icon.vue";
import Pagination from "@/components/common/Pagination.vue";
import PlatformIcon from "@/components/common/PlatformIcon.vue";
import Select from "@/components/common/Select.vue";
import TablePageLayout from "@/components/layout/TablePageLayout.vue";

// 状态与逻辑已纯移动到 groups/useGroupsView.ts；本文件只保留列表与小型对话框（openspec Phase 3）
const ctx = useGroupsView();
provideGroupsViewContext(ctx);
const {
  authStore,
  cancelUnsupportedLive,
  capacityMap,
  columns,
  confirmDelete,
  confirmUnsupportedLive,
  deleteConfirmMessage,
  duplicatingGroupIds,
  exclusiveOptions,
  filters,
  formatCost,
  formatUsd,
  getQuotaUsageClass,
  groups,
  handleCompositeRoutes,
  handleDelete,
  handleDuplicate,
  handleEdit,
  handlePageChange,
  handlePageSizeChange,
  handleRPMOverrides,
  handleRateMultipliers,
  handleSearch,
  handleSort,
  isColumnVisible,
  loadGroups,
  loading,
  openCreateModal,
  openSortModal,
  pagination,
  platformFilterOptions,
  rateMultipliersGroup,
  rpmOverridesGroup,
  searchQuery,
  showColumnDropdown,
  showDeleteDialog,
  showRPMOverridesModal,
  showRateMultipliersModal,
  showUnsupportedLiveConfirm,
  statusOptions,
  t,
  toggleColumn,
  toggleableColumns,
  usageLoading,
  usageMap,
  columnDropdownRef,
} = ctx;
</script>
