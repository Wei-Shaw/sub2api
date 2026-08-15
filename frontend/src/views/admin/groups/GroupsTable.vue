<template>
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
              class="absolute left-3 top-1/2 -translate-y-1/2 text-ink-tertiary"
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
            :title="t('common.refresh')"
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
              class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-sm border border-line bg-surface-raised py-1 shadow-popover"
            >
              <button
                v-for="col in toggleableColumns"
                :key="col.key"
                @click="toggleColumn(col.key)"
                class="flex w-full items-center justify-between px-3 py-1.5 text-left text-sm text-ink-secondary hover:bg-surface-hover"
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
          <span class="font-medium text-ink">{{
            value
          }}</span>
        </template>

        <template #cell-id="{ value }">
          <span class="font-mono text-xs tabular-nums text-ink-secondary"
            >#{{ value }}</span
          >
        </template>

        <!--
          Platform is an identity, not a status, so it gets no hue. The mark
          carries the identity and the label spells it out; a five-way pastel
          ladder was spending the table's whole colour budget on a column that
          signals nothing.
        -->
        <template #cell-platform="{ value }">
          <span class="badge badge-gray">
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
                  ? 'badge-purple'
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
              class="space-y-0.5 text-xs text-ink-secondary"
            >
              <div
                v-if="
                  row.daily_limit_usd ||
                  row.weekly_limit_usd ||
                  row.monthly_limit_usd
                "
                class="flex flex-wrap items-center gap-x-1 gap-y-0.5"
              >
                <span
                  v-if="row.daily_limit_usd"
                  class="whitespace-nowrap font-mono tabular-nums"
                >
                  <span
                    v-if="usageLoading"
                    class="font-medium text-ink-tertiary"
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
                  <span class="text-ink-tertiary">
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
                  class="mx-1 text-ink-disabled"
                  >·</span
                >
                <span
                  v-if="row.weekly_limit_usd"
                  class="whitespace-nowrap font-mono tabular-nums"
                  >{{ formatUsd(row.weekly_limit_usd) }}/{{
                    t("admin.groups.limitWeek")
                  }}</span
                >
                <span
                  v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                  class="mx-1 text-ink-disabled"
                  >·</span
                >
                <span
                  v-if="row.monthly_limit_usd"
                  class="whitespace-nowrap font-mono tabular-nums"
                  >{{ formatUsd(row.monthly_limit_usd) }}/{{
                    t("admin.groups.limitMonth")
                  }}</span
                >
              </div>
              <span v-else class="text-ink-tertiary">{{
                t("admin.groups.subscription.noLimit")
              }}</span>
              <div class="text-ink-tertiary">
                {{ t("admin.groups.usageTotal") }}
                <span class="ml-1 font-mono font-medium tabular-nums text-ink-secondary"
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
          <span class="font-mono text-sm tabular-nums text-ink-secondary"
            >{{ value }}x</span
          >
        </template>

        <!--
          Only the exception is marked. Public is the norm here, and a badge on
          every row makes the column a texture instead of a signal. Accent is
          not used: it means interactive or selected, never a property.
        -->
        <template #cell-is_exclusive="{ value }">
          <span v-if="value" class="badge badge-gray">
            {{ t("admin.groups.exclusive") }}
          </span>
          <span v-else class="text-xs text-ink-tertiary">
            {{ t("admin.groups.public") }}
          </span>
        </template>

        <!--
          The unit used to be a filled grey chip on every one of these three
          lines, so each row carried up to three grey blobs that said nothing.
          The unit is now a step down in size and ink, which is where a unit
          belongs: readable, and never competing with the figure.
        -->
        <template #cell-account_count="{ row }">
          <div class="space-y-0.5 text-xs">
            <div>
              <span class="text-ink-secondary">{{
                t("admin.groups.accountsAvailable")
              }}</span>
              <span class="ml-1 font-mono font-medium tabular-nums text-success"
                >{{ row.active_account_count || 0 }}</span
              >
              <span class="ml-1 text-2xs text-ink-tertiary">{{
                t("admin.groups.accountsUnit")
              }}</span>
            </div>
            <div v-if="row.rate_limited_account_count">
              <span class="text-ink-secondary">{{
                t("admin.groups.accountsRateLimited")
              }}</span>
              <span class="ml-1 font-mono font-medium tabular-nums text-warn"
                >{{ row.rate_limited_account_count }}</span
              >
              <span class="ml-1 text-2xs text-ink-tertiary">{{
                t("admin.groups.accountsUnit")
              }}</span>
            </div>
            <div>
              <span class="text-ink-secondary">{{
                t("admin.groups.accountsTotal")
              }}</span>
              <span
                class="ml-1 font-mono font-medium tabular-nums text-ink-secondary"
                >{{ row.account_count || 0 }}</span
              >
              <span class="ml-1 text-2xs text-ink-tertiary">{{
                t("admin.groups.accountsUnit")
              }}</span>
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
          <span v-else class="text-xs text-ink-tertiary">—</span>
        </template>

        <template #cell-usage="{ row }">
          <div v-if="usageLoading" class="text-xs text-ink-tertiary">—</div>
          <div v-else class="space-y-0.5 text-xs">
            <div class="text-ink-secondary">
              <span class="text-ink-tertiary">{{
                t("admin.groups.usageToday")
              }}</span>
              <span class="ml-1 font-mono font-medium tabular-nums text-ink-secondary"
                >${{
                  formatCost(usageMap.get(row.id)?.today_cost ?? 0)
                }}</span
              >
            </div>
            <div class="text-ink-secondary">
              <span class="text-ink-tertiary">{{
                t("admin.groups.usageYesterday")
              }}</span>
              <span class="ml-1 font-mono font-medium tabular-nums text-ink-secondary"
                >${{
                  formatCost(usageMap.get(row.id)?.yesterday_cost ?? 0)
                }}</span
              >
            </div>
            <div class="text-ink-secondary">
              <span class="text-ink-tertiary">{{
                t("admin.groups.usageTotal")
              }}</span>
              <span class="ml-1 font-mono font-medium tabular-nums text-ink-secondary"
                >${{
                  formatCost(usageMap.get(row.id)?.total_cost ?? 0)
                }}</span
              >
            </div>
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
              class="row-action"
            >
              <Icon name="edit" size="sm" />
              <span class="text-xs">{{ t("common.edit") }}</span>
            </button>
            <button
              data-testid="group-duplicate"
              :title="
                duplicatingGroupIds.has(row.id)
                  ? t('admin.groups.duplicating')
                  : t('admin.groups.duplicate')
              "
              :disabled="duplicatingGroupIds.has(row.id)"
              @click="handleDuplicate(row)"
              class="row-action disabled:cursor-not-allowed disabled:opacity-50"
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
              v-if="row.platform === 'composite'"
              @click="handleCompositeRoutes(row)"
              class="row-action"
            >
              <Icon name="swap" size="sm" />
              <span class="text-xs">{{
                t("admin.groups.compositeRoutes.action")
              }}</span>
            </button>
            <button
              @click="handleRateMultipliers(row)"
              class="row-action"
            >
              <Icon name="dollar" size="sm" />
              <span class="text-xs">{{
                t("admin.groups.rateMultipliers")
              }}</span>
            </button>
            <button
              @click="handleRPMOverrides(row)"
              class="row-action"
            >
              <Icon name="bolt" size="sm" />
              <span class="text-xs">{{
                t("admin.groups.rpmOverrides")
              }}</span>
            </button>
            <button
              @click="handleDelete(row)"
              class="row-action row-action-danger"
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
</template>

<script setup lang="ts">
import TablePageLayout from "@/components/layout/TablePageLayout.vue";
import Icon from "@/components/icons/Icon.vue";
import Select from "@/components/common/Select.vue";
import DataTable from "@/components/common/DataTable.vue";
import PlatformIcon from "@/components/common/PlatformIcon.vue";
import GroupCapacityBadge from "@/components/common/GroupCapacityBadge.vue";
import EmptyState from "@/components/common/EmptyState.vue";
import Pagination from "@/components/common/Pagination.vue";
import { useGroupsViewContext } from "./context";

const ctx = useGroupsViewContext();

const {
  t,
  toggleableColumns,
  showColumnDropdown,
  columnDropdownRef,
  isColumnVisible,
  toggleColumn,
  columns,
  statusOptions,
  exclusiveOptions,
  platformFilterOptions,
  groups,
  loading,
  usageMap,
  usageLoading,
  capacityMap,
  searchQuery,
  filters,
  pagination,
  duplicatingGroupIds,
  loadGroups,
  formatCost,
  formatUsd,
  getQuotaUsageClass,
  handleSearch,
  handlePageChange,
  handlePageSizeChange,
  handleSort,
  openCreateModal,
  handleEdit,
  handleRateMultipliers,
  handleRPMOverrides,
  handleDuplicate,
  handleCompositeRoutes,
  handleDelete,
  openSortModal,
} = ctx;
</script>

<style scoped>
/*
 * Row actions used to hand every button its own hover hue — accent, cyan,
 * purple, orange, red. Six colours for six equally ordinary verbs is
 * decoration, and it spent the signal budget the status column needs. One
 * interactive colour for all of them, danger reserved for the one action that
 * destroys something.
 */
.row-action {
  @apply flex flex-col items-center gap-0.5 rounded-sm p-1.5;
  @apply text-ink-secondary transition-colors;
  @apply hover:bg-surface-hover hover:text-accent;
}

.row-action-danger {
  @apply hover:bg-danger-tint hover:text-danger;
}
</style>
