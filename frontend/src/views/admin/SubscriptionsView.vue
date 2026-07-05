<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <!-- Top Toolbar: Left (search + filters) / Right (actions) -->
        <div class="flex flex-wrap items-start justify-between gap-4">
          <!-- Left: Fuzzy user search + filters (wrap to multiple lines) -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- User Search -->
            <div
              class="relative w-full sm:w-64"
              data-filter-user-search
            >
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="filterUserKeyword"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10 pr-8"
                @input="debounceSearchFilterUsers"
                @focus="showFilterUserDropdown = true"
              />
              <button
                v-if="selectedFilterUser"
                @click="clearFilterUser"
                type="button"
                class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                :title="t('common.clear')"
              >
                <Icon name="x" size="sm" :stroke-width="2" />
              </button>

              <!-- User Dropdown -->
              <div
                v-if="showFilterUserDropdown && (filterUserResults.length > 0 || filterUserKeyword)"
                class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div
                  v-if="filterUserLoading"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="filterUserResults.length === 0 && filterUserKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') }}
                </div>
                <button
                  v-for="user in filterUserResults"
                  :key="user.id"
                  type="button"
                  @click="selectFilterUser(user)"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                  <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                </button>
              </div>
            </div>

            <!-- Filters -->
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.status"
                :options="statusOptions"
                :placeholder="t('admin.subscriptions.allStatus')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-48">
              <Select
                v-model="filters.group_id"
                :options="groupOptions"
                :placeholder="t('admin.subscriptions.allGroups')"
                @change="applyFilters"
              />
            </div>
            <div class="w-full sm:w-40">
              <Select
                v-model="filters.platform"
                :options="platformFilterOptions"
                :placeholder="t('admin.subscriptions.allPlatforms')"
                @change="applyFilters"
              />
            </div>
          </div>

          <!-- Right: Actions -->
          <div class="ml-auto flex flex-wrap items-center justify-end gap-3">
            <button
              @click="loadSubscriptions"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.users.columnSettings')"
              >
                <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                <span class="hidden md:inline">{{ t('admin.users.columnSettings') }}</span>
              </button>
              <!-- Dropdown menu -->
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 z-50 mt-2 w-48 origin-top-right rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div class="p-2">
                  <!-- User column mode selection -->
                  <div class="mb-2 border-b border-gray-200 pb-2 dark:border-gray-700">
                    <div class="px-3 py-1 text-xs font-medium text-gray-500 dark:text-gray-400">
                      {{ t('admin.subscriptions.columns.user') }}
                    </div>
                    <button
                      @click="setUserColumnMode('email')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.email') }}</span>
                      <Icon v-if="userColumnMode === 'email'" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <button
                      @click="setUserColumnMode('username')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.username') }}</span>
                      <Icon v-if="userColumnMode === 'username'" name="check" size="sm" class="text-primary-500" />
                    </button>
                  </div>
                  <!-- Other columns toggle -->
                  <button
                    v-for="col in toggleableColumns"
                    :key="col.key"
                    @click="toggleColumn(col.key)"
                    class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                  >
                    <span>{{ col.label }}</span>
                    <Icon v-if="isColumnVisible(col.key)" name="check" size="sm" class="text-primary-500" />
                  </button>
                </div>
              </div>
            </div>
            <button
              @click="showGuideModal = true"
              class="btn btn-secondary"
              :title="t('admin.subscriptions.guide.showGuide')"
            >
              <Icon name="questionCircle" size="md" />
            </button>
            <button @click="showAssignModal = true" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.subscriptions.assignSubscription') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Subscriptions Table -->
      <template #table>
        <DataTable
          :columns="columns"
          :data="subscriptions"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-user="{ row }">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ userColumnMode === 'email'
                    ? (row.user?.email?.charAt(0).toUpperCase() || '?')
                    : (row.user?.username?.charAt(0).toUpperCase() || '?')
                  }}
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ userColumnMode === 'email'
                  ? (row.user?.email || t('admin.redeem.userPrefix', { id: row.user_id }))
                  : (row.user?.username || '-')
                }}
              </span>
            </div>
          </template>

          <template #cell-group="{ row }">
            <GroupBadge
              v-if="row.group"
              :name="row.group.name"
              :platform="row.group.platform"
              :subscription-type="row.group.subscription_type"
              :rate-multiplier="row.group.rate_multiplier"
              :show-rate="false"
            />
            <span v-else class="text-sm text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-usage="{ row }">
            <div class="min-w-[280px] space-y-2">
              <!-- Daily Usage -->
              <div v-if="row.group?.daily_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.daily') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.daily_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.daily_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.daily_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatDailyUsageWindow(row) }}</span>
                </div>
              </div>

              <!-- Weekly Usage -->
              <div v-if="row.group?.weekly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.weekly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.weekly_usage_usd, row.group?.weekly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.weekly_usage_usd, row.group?.weekly_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.weekly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.weekly_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.weekly_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.weekly_window_start, 'weekly') }}</span>
                </div>
              </div>

              <!-- Monthly Usage -->
              <div v-if="row.group?.monthly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.monthly') }}</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.monthly_usage_usd, row.group?.monthly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.monthly_usage_usd, row.group?.monthly_limit_usd)
                      }"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.monthly_usage_usd?.toFixed(2) || '0.00' }}
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.monthly_limit_usd?.toFixed(2) }}
                  </span>
                </div>
                <div class="reset-info" v-if="row.monthly_window_start">
                  <svg
                    class="h-3 w-3"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    stroke-width="2"
                  >
                    <path
                      stroke-linecap="round"
                      stroke-linejoin="round"
                      d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z"
                    />
                  </svg>
                  <span>{{ formatResetTime(row.monthly_window_start, 'monthly') }}</span>
                </div>
              </div>

              <!-- No Limits - Unlimited badge -->
              <div
                v-if="
                  !row.group?.daily_limit_usd &&
                  !row.group?.weekly_limit_usd &&
                  !row.group?.monthly_limit_usd
                "
                class="flex items-center gap-2 rounded-lg bg-gradient-to-r from-emerald-50 to-teal-50 px-3 py-2 dark:from-emerald-900/20 dark:to-teal-900/20"
              >
                <span class="text-lg text-emerald-600 dark:text-emerald-400">∞</span>
                <span class="text-xs font-medium text-emerald-700 dark:text-emerald-300">
                  {{ t('admin.subscriptions.unlimited') }}
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value }">
            <div v-if="value">
              <span
                class="text-sm"
                :class="
                  isExpiringSoon(value)
                    ? 'text-orange-600 dark:text-orange-400'
                    : 'text-gray-700 dark:text-gray-300'
                "
              >
                {{ formatDateOnly(value) }}
              </span>
              <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
                {{ getDaysRemaining(value) }} {{ t('admin.subscriptions.daysRemaining') }}
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">{{
              t('admin.subscriptions.noExpiration')
            }}</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active'
                  ? 'badge-success'
                  : value === 'expired'
                    ? 'badge-warning'
                    : 'badge-danger'
              ]"
            >
              {{ t(`admin.subscriptions.status.${value}`) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                v-if="row.status === 'active' || row.status === 'expired'"
                @click="handleExtend(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.adjust') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleResetQuota(row)"
                :disabled="resettingQuota && resettingSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetQuota') }}</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.revoke') }}</span>
              </button>
              <button
                v-if="row.status === 'revoked'"
                @click="handleRestore(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.restore') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.subscriptions.noSubscriptionsYet')"
              :description="t('admin.subscriptions.assignFirstSubscription')"
              :action-text="t('admin.subscriptions.assignSubscription')"
              @action="showAssignModal = true"
            />
          </template>
        </DataTable>
      </template>

      <!-- Pagination -->
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

    <!-- Benefit Packages / Plans -->
    <div class="mt-6 space-y-6">
      <div class="grid gap-6 xl:grid-cols-2">
        <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-gray-700 dark:bg-dark-800">
          <div class="mb-4 flex items-center justify-between gap-3">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.subscriptions.benefitPackages.title') }}
            </h3>
            <button class="btn btn-primary btn-sm" @click="openCreateBenefitPackageModal">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.subscriptions.benefitPackages.create') }}
            </button>
          </div>

          <div class="overflow-x-auto">
            <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-gray-700">
              <thead class="bg-gray-50 dark:bg-dark-700">
                <tr>
                  <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                    {{ t('admin.subscriptions.benefitPackages.columns.name') }}
                  </th>
                  <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                    {{ t('admin.subscriptions.benefitPackages.columns.group') }}
                  </th>
                  <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                    {{ t('admin.subscriptions.benefitPackages.columns.days') }}
                  </th>
                  <th class="px-3 py-2 text-left font-medium text-gray-600 dark:text-gray-300">
                    {{ t('admin.subscriptions.benefitPackages.columns.description') }}
                  </th>
                  <th class="px-3 py-2 text-right font-medium text-gray-600 dark:text-gray-300">
                    {{ t('admin.subscriptions.columns.actions') }}
                  </th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-100 dark:divide-gray-700">
                <tr v-if="benefitPackagesLoading">
                  <td colspan="5" class="px-3 py-6 text-center text-gray-500 dark:text-gray-400">
                    {{ t('common.loading') }}
                  </td>
                </tr>
                <tr v-else-if="benefitPackages.length === 0">
                  <td colspan="5" class="px-3 py-6 text-center text-gray-500 dark:text-gray-400">
                    {{ t('admin.subscriptions.benefitPackages.empty') }}
                  </td>
                </tr>
                <tr v-for="item in benefitPackages" :key="item.id">
                  <td class="px-3 py-2 font-medium text-gray-900 dark:text-white">{{ item.name }}</td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ item.group_name }}</td>
                  <td class="px-3 py-2 text-gray-600 dark:text-gray-300">{{ item.lease_days }}</td>
                  <td class="max-w-xs truncate px-3 py-2 text-gray-500 dark:text-gray-400">
                    {{ item.description || '-' }}
                  </td>
                  <td class="px-3 py-2 text-right">
                    <div class="flex items-center justify-end gap-2">
                      <button
                        class="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400"
                        @click="openEditBenefitPackageModal(item)"
                      >
                        {{ t('common.edit') }}
                      </button>
                      <button
                        class="text-xs text-red-600 hover:text-red-700 dark:text-red-400"
                        @click="handleDeleteBenefitPackage(item)"
                      >
                        {{ t('common.delete') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </section>

        <section class="rounded-xl border border-gray-200 bg-white p-5 shadow-sm dark:border-gray-700 dark:bg-dark-800">
          <div class="mb-4 flex items-center justify-between gap-3">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.subscriptions.benefitPlans.title') }}
            </h3>
            <button class="btn btn-primary btn-sm" @click="openCreateBenefitPlanModal">
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.subscriptions.benefitPlans.create') }}
            </button>
          </div>

          <div v-if="benefitPlansLoading" class="rounded-lg border border-gray-200 px-4 py-6 text-center text-gray-500 dark:border-gray-700 dark:text-gray-400">
            {{ t('common.loading') }}
          </div>
          <div v-else-if="benefitPlans.length === 0" class="rounded-lg border border-gray-200 px-4 py-6 text-center text-gray-500 dark:border-gray-700 dark:text-gray-400">
            {{ t('admin.subscriptions.benefitPlans.empty') }}
          </div>
          <div v-else class="space-y-3">
            <article
              v-for="plan in benefitPlans"
              :key="plan.id"
              class="rounded-lg border border-gray-200 p-3 dark:border-gray-700"
            >
              <div class="mb-2 flex items-start justify-between gap-3">
                <div>
                  <h4 class="font-medium text-gray-900 dark:text-white">{{ plan.name }}</h4>
                  <p class="text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.subscriptions.benefitPlans.assignedUsers', { count: plan.assigned_user_count }) }}
                  </p>
                </div>
                <div class="flex items-center gap-2">
                  <button
                    class="text-xs text-emerald-600 hover:text-emerald-700 dark:text-emerald-400"
                    @click="openBenefitPlanMembersModal(plan)"
                  >
                    {{ t('admin.subscriptions.benefitPlans.manageMembers') }}
                  </button>
                  <button
                    class="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400"
                    @click="openEditBenefitPlanModal(plan)"
                  >
                    {{ t('common.edit') }}
                  </button>
                  <button
                    class="text-xs text-red-600 hover:text-red-700 dark:text-red-400"
                    @click="handleDeleteBenefitPlan(plan)"
                  >
                    {{ t('common.delete') }}
                  </button>
                </div>
              </div>
              <p v-if="plan.description" class="mb-2 text-xs text-gray-600 dark:text-gray-300">
                {{ plan.description }}
              </p>
              <div class="flex flex-wrap gap-1.5">
                <span
                  v-for="pkg in plan.packages"
                  :key="`${plan.id}-${pkg.package_id}`"
                  class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
                >
                  {{ pkg.name }} · {{ pkg.lease_days }}d
                </span>
              </div>
            </article>
          </div>
        </section>
      </div>
    </div>

    <!-- Benefit Package Modal -->
    <BaseDialog
      :show="showBenefitPackageModal"
      :title="editingBenefitPackage ? t('admin.subscriptions.benefitPackages.edit') : t('admin.subscriptions.benefitPackages.create')"
      width="normal"
      @close="closeBenefitPackageModal"
    >
      <form id="benefit-package-form" class="space-y-4" @submit.prevent="handleSaveBenefitPackage">
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPackages.columns.name') }}</label>
          <input v-model.trim="benefitPackageForm.name" class="input" maxlength="100" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPackages.columns.group') }}</label>
          <Select
            v-model="benefitPackageForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPackages.columns.days') }}</label>
          <input v-model.number="benefitPackageForm.lease_days" class="input" type="number" min="1" max="36500" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPackages.columns.description') }}</label>
          <textarea
            v-model.trim="benefitPackageForm.description"
            class="input min-h-[84px] resize-y"
            maxlength="2000"
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeBenefitPackageModal">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="benefit-package-form" class="btn btn-primary" :disabled="benefitSaving">
            {{ benefitSaving ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Benefit Plan Modal -->
    <BaseDialog
      :show="showBenefitPlanModal"
      :title="editingBenefitPlan ? t('admin.subscriptions.benefitPlans.edit') : t('admin.subscriptions.benefitPlans.create')"
      width="normal"
      @close="closeBenefitPlanModal"
    >
      <form id="benefit-plan-form" class="space-y-4" @submit.prevent="handleSaveBenefitPlan">
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPlans.columns.name') }}</label>
          <input v-model.trim="benefitPlanForm.name" class="input" maxlength="100" required />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPlans.columns.description') }}</label>
          <textarea
            v-model.trim="benefitPlanForm.description"
            class="input min-h-[84px] resize-y"
            maxlength="2000"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.benefitPlans.columns.packages') }}</label>
          <div class="max-h-56 space-y-2 overflow-auto rounded-lg border border-gray-200 p-3 dark:border-gray-700">
            <label
              v-for="item in benefitPackages"
              :key="item.id"
              class="flex cursor-pointer items-center justify-between rounded-md px-2 py-1.5 hover:bg-gray-50 dark:hover:bg-dark-700"
            >
              <div>
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ item.name }}</div>
                <div class="text-xs text-gray-500 dark:text-gray-400">
                  {{ item.group_name }} · {{ item.lease_days }}d
                </div>
              </div>
              <input
                v-model="benefitPlanForm.package_ids"
                :value="item.id"
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
            </label>
            <p v-if="benefitPackages.length === 0" class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.subscriptions.benefitPlans.noPackagesToSelect') }}
            </p>
          </div>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeBenefitPlanModal">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" form="benefit-plan-form" class="btn btn-primary" :disabled="benefitSaving">
            {{ benefitSaving ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Benefit Plan Members Modal -->
    <BaseDialog
      :show="showBenefitPlanMembersModal"
      :title="benefitPlanMembersModalTitle"
      width="wide"
      @close="closeBenefitPlanMembersModal"
    >
      <div class="space-y-5">
        <div class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 dark:border-emerald-900/70 dark:bg-emerald-950/30">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p class="text-sm font-medium text-emerald-900 dark:text-emerald-200">
                {{ t('admin.subscriptions.benefitPlanMembers.subtitle') }}
              </p>
              <p class="mt-1 text-xs text-emerald-700 dark:text-emerald-300">
                {{ t('admin.subscriptions.benefitPlanMembers.moveNotice') }}
              </p>
            </div>
            <span class="inline-flex items-center rounded-full bg-white px-3 py-1 text-xs font-medium text-emerald-700 shadow-sm dark:bg-emerald-900/50 dark:text-emerald-200">
              {{ t('admin.subscriptions.benefitPlanMembers.membersCount', { count: benefitPlanMembers.length }) }}
            </span>
          </div>
        </div>

        <div class="grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
          <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-gray-700">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.subscriptions.benefitPlanMembers.currentMembers') }}
                </h4>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.subscriptions.benefitPlanMembers.selectedToRemove', { count: selectedBenefitPlanMemberIds.length }) }}
                </p>
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="benefitPlanMembersSubmitting || selectedBenefitPlanMemberIds.length === 0"
                @click="handleBulkRemoveBenefitPlanUsers"
              >
                {{ t('admin.subscriptions.benefitPlanMembers.removeSelected') }}
              </button>
            </div>

            <div class="max-h-[420px] space-y-2 overflow-auto pr-1">
              <div
                v-if="benefitPlanMembersLoading"
                class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="benefitPlanMembers.length === 0"
                class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-gray-700 dark:text-gray-400"
              >
                {{ t('admin.subscriptions.benefitPlanMembers.noMembers') }}
              </div>
              <label
                v-for="member in benefitPlanMembers"
                :key="member.user_id"
                class="flex cursor-pointer items-start gap-3 rounded-lg border border-gray-200 px-3 py-3 transition-colors hover:border-gray-300 hover:bg-gray-50 dark:border-gray-700 dark:hover:border-gray-600 dark:hover:bg-dark-700"
              >
                <input
                  :checked="selectedBenefitPlanMemberIds.includes(member.user_id)"
                  type="checkbox"
                  class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                  @change="toggleBenefitPlanMemberSelection(member.user_id)"
                />
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ member.email }}</span>
                    <span class="text-xs text-gray-500 dark:text-gray-400">#{{ member.user_id }}</span>
                    <span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-600 dark:text-gray-300">
                      {{ member.role }}
                    </span>
                    <span class="inline-flex items-center rounded-full bg-gray-100 px-2 py-0.5 text-[11px] text-gray-600 dark:bg-dark-600 dark:text-gray-300">
                      {{ member.status }}
                    </span>
                  </div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.subscriptions.benefitPlanMembers.assignedAt') }}:
                    {{ formatDateOnly(member.assigned_at) }}
                  </div>
                </div>
              </label>
            </div>
          </section>

          <section class="space-y-3 rounded-xl border border-gray-200 p-4 dark:border-gray-700">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                  {{ t('admin.subscriptions.benefitPlanMembers.addUsers') }}
                </h4>
                <p class="text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.subscriptions.benefitPlanMembers.selectedToAdd', { count: selectedBenefitPlanAddUsers.length }) }}
                </p>
              </div>
              <button
                type="button"
                class="btn btn-primary btn-sm"
                :disabled="benefitPlanMembersSubmitting || selectedBenefitPlanAddUsers.length === 0"
                @click="handleBulkAssignBenefitPlanUsers"
              >
                {{ t('admin.subscriptions.benefitPlanMembers.addSelected') }}
              </button>
            </div>

            <div class="relative" data-benefit-plan-member-search>
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="benefitPlanMemberSearchKeyword"
                type="text"
                class="input pl-10"
                :placeholder="t('admin.users.searchUsers')"
                @input="debounceSearchBenefitPlanUsers"
                @focus="showBenefitPlanMemberSearchDropdown = true"
              />

              <div
                v-if="showBenefitPlanMemberSearchDropdown && (benefitPlanMemberSearchResults.length > 0 || benefitPlanMemberSearchKeyword)"
                class="absolute z-50 mt-1 max-h-72 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
              >
                <div
                  v-if="benefitPlanMemberSearchLoading"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.loading') }}
                </div>
                <div
                  v-else-if="benefitPlanMemberSearchResults.length === 0 && benefitPlanMemberSearchKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') }}
                </div>
                <label
                  v-for="user in benefitPlanMemberSearchResults"
                  :key="user.id"
                  class="flex cursor-pointer items-start gap-3 px-4 py-3 text-left hover:bg-gray-100 dark:hover:bg-gray-700"
                  :class="{ 'cursor-not-allowed opacity-60': benefitPlanMemberIdSet.has(user.id) }"
                >
                  <input
                    :checked="selectedBenefitPlanAddUserIdSet.has(user.id)"
                    :disabled="benefitPlanMemberIdSet.has(user.id)"
                    type="checkbox"
                    class="mt-0.5 h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    @change="toggleBenefitPlanAddSelection(user)"
                  />
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="truncate text-sm font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                      <span class="text-xs text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
                      <span
                        v-if="benefitPlanMemberIdSet.has(user.id)"
                        class="inline-flex items-center rounded-full bg-emerald-100 px-2 py-0.5 text-[11px] text-emerald-700 dark:bg-emerald-900/50 dark:text-emerald-200"
                      >
                        {{ t('admin.subscriptions.benefitPlanMembers.alreadyMember') }}
                      </span>
                    </div>
                  </div>
                </label>
              </div>
            </div>

            <div class="rounded-lg bg-gray-50 px-3 py-3 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-400">
              {{ t('admin.subscriptions.benefitPlanMembers.moveNotice') }}
            </div>

            <div
              v-if="selectedBenefitPlanAddUsers.length > 0"
              class="flex max-h-[240px] flex-wrap gap-2 overflow-auto rounded-lg border border-dashed border-gray-200 p-3 dark:border-gray-700"
            >
              <button
                v-for="user in selectedBenefitPlanAddUsers"
                :key="`selected-add-${user.id}`"
                type="button"
                class="inline-flex items-center gap-1 rounded-full bg-primary-50 px-3 py-1 text-xs text-primary-700 dark:bg-primary-900/30 dark:text-primary-200"
                @click="toggleBenefitPlanAddSelection(user)"
              >
                <span class="max-w-[180px] truncate">{{ user.email }}</span>
                <Icon name="x" size="xs" :stroke-width="2" />
              </button>
            </div>
          </section>
        </div>
      </div>
    </BaseDialog>

    <!-- Assign Subscription Modal -->
    <BaseDialog
      :show="showAssignModal"
      :title="t('admin.subscriptions.assignSubscription')"
      width="normal"
      @close="closeAssignModal"
    >
      <form
        id="assign-subscription-form"
        @submit.prevent="handleAssignSubscription"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.user') }}</label>
          <div class="relative" data-assign-user-search>
            <input
              v-model="userSearchKeyword"
              type="text"
              class="input pr-8"
              :placeholder="t('admin.usage.searchUserPlaceholder')"
              @input="debounceSearchUsers"
              @focus="showUserDropdown = true"
            />
            <button
              v-if="selectedUser"
              @click="clearUserSelection"
              type="button"
              class="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
            >
              <Icon name="x" size="sm" :stroke-width="2" />
            </button>
            <!-- User Dropdown -->
            <div
              v-if="showUserDropdown && (userSearchResults.length > 0 || userSearchKeyword)"
              class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-gray-700 dark:bg-gray-800"
            >
              <div
                v-if="userSearchLoading"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.loading') }}
              </div>
              <div
                v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') }}
              </div>
              <button
                v-for="user in userSearchResults"
                :key="user.id"
                type="button"
                @click="selectUser(user)"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email }}</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id }}</span>
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') }}</label>
          <Select
            v-model="assignForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          >
            <template #selected="{ option }">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectGroup') }}</span>
            </template>
            <template #option="{ option, selected }">
              <GroupOptionItem
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
                :description="(option as unknown as GroupOption).description"
                :selected="selected"
              />
            </template>
          </Select>
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') }}</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') }}</label>
          <input v-model.number="assignForm.validity_days" type="number" min="1" class="input" />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAssignModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="assign-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            <svg
              v-if="submitting"
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
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Adjust Subscription Modal -->
    <BaseDialog
      :show="showExtendModal"
      :title="t('admin.subscriptions.adjustSubscription')"
      width="narrow"
      @close="closeExtendModal"
    >
      <form
        v-if="extendingSubscription"
        id="extend-subscription-form"
        @submit.prevent="handleExtendSubscription"
        class="space-y-5"
      >
        <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-700">
          <p class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.adjustingFor') }}
            <span class="font-medium text-gray-900 dark:text-white">{{
              extendingSubscription.user?.email
            }}</span>
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentExpiration') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{
                extendingSubscription.expires_at
                  ? formatDateOnly(extendingSubscription.expires_at)
                  : t('admin.subscriptions.noExpiration')
              }}
            </span>
          </p>
          <p v-if="extendingSubscription.expires_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.remainingDays') }}:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ getDaysRemaining(extendingSubscription.expires_at) ?? 0 }}
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.adjustDays') }}</label>
          <div class="flex items-center gap-2">
            <input
              v-model.number="extendForm.days"
              type="number"
              required
              class="input text-center"
              :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.adjustHint') }}</p>
        </div>
      </form>
      <template #footer>
        <div v-if="extendingSubscription" class="flex justify-end gap-3">
          <button @click="closeExtendModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button
            type="submit"
            form="extend-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.adjusting') : t('admin.subscriptions.adjust') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.revoke')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmRevoke"
      @cancel="showRevokeDialog = false"
    />

    <!-- Restore Confirmation Dialog -->
    <ConfirmDialog
      :show="showRestoreDialog"
      :title="t('admin.subscriptions.restoreSubscription')"
      :message="t('admin.subscriptions.restoreConfirm', { user: restoringSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.restore')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmRestore"
      @cancel="showRestoreDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaConfirm"
      :title="t('admin.subscriptions.resetQuotaTitle')"
      :message="t('admin.subscriptions.resetQuotaConfirm', { user: resettingSubscription?.user?.email })"
      :confirm-text="t('admin.subscriptions.resetQuota')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmResetQuota"
      @cancel="showResetQuotaConfirm = false"
    />
    <!-- Subscription Guide Modal -->
    <teleport to="body">
      <transition name="modal">
        <div v-if="showGuideModal" class="fixed inset-0 z-50 flex items-center justify-center p-4" @mousedown.self="showGuideModal = false">
          <div class="fixed inset-0 bg-black/50" @click="showGuideModal = false"></div>
          <div class="relative max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button type="button" class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="showGuideModal = false">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.subscriptions.guide.title') }}</h2>
            <p class="mb-5 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.guide.subtitle') }}</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.subscriptions.guide.step1.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step1.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step1.line3') }}</li>
              </ol>
              <div class="ml-8 mt-2">
                <router-link
                  to="/admin/groups"
                  @click="showGuideModal = false"
                  class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  {{ t('admin.subscriptions.guide.step1.link') }}
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.subscriptions.guide.step2.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step2.line1') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line2') }}</li>
                <li>{{ t('admin.subscriptions.guide.step2.line3') }}</li>
              </ol>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.subscriptions.guide.step3.title') }}
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in guideActionRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.action }}</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ row.desc }}</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Tip -->
            <div class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
              {{ t('admin.subscriptions.guide.tip') }}
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showGuideModal = false">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  UserSubscription,
  Group,
  GroupPlatform,
  SubscriptionType,
  BenefitPackage,
  BenefitPlan,
  BenefitPlanMember
} from '@/types'
import type { SimpleUser } from '@/api/admin/usage'
import type { Column } from '@/components/common/types'
import { formatDateOnly } from '@/utils/format'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Icon from '@/components/icons/Icon.vue'
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts } from '@/utils/subscriptionQuota'

const { t } = useI18n()
const appStore = useAppStore()

interface GroupOption {
  value: number
  label: string
  description: string | null
  platform: GroupPlatform
  subscriptionType: SubscriptionType
  rate: number
}

// Guide modal state
const showGuideModal = ref(false)

const guideActionRows = computed(() => [
  { action: t('admin.subscriptions.guide.actions.adjust'), desc: t('admin.subscriptions.guide.actions.adjustDesc') },
  { action: t('admin.subscriptions.guide.actions.resetQuota'), desc: t('admin.subscriptions.guide.actions.resetQuotaDesc') },
  { action: t('admin.subscriptions.guide.actions.revoke'), desc: t('admin.subscriptions.guide.actions.revokeDesc') }
])

// User column display mode: 'email' or 'username'
const userColumnMode = ref<'email' | 'username'>('email')
const USER_COLUMN_MODE_KEY = 'subscription-user-column-mode'

const loadUserColumnMode = () => {
  try {
    const saved = localStorage.getItem(USER_COLUMN_MODE_KEY)
    if (saved === 'email' || saved === 'username') {
      userColumnMode.value = saved
    }
  } catch (e) {
    console.error('Failed to load user column mode:', e)
  }
}

const saveUserColumnMode = () => {
  try {
    localStorage.setItem(USER_COLUMN_MODE_KEY, userColumnMode.value)
  } catch (e) {
    console.error('Failed to save user column mode:', e)
  }
}

const setUserColumnMode = (mode: 'email' | 'username') => {
  userColumnMode.value = mode
  saveUserColumnMode()
}

// All available columns
const allColumns = computed<Column[]>(() => [
  {
    key: 'user',
    label: userColumnMode.value === 'email'
      ? t('admin.subscriptions.columns.user')
      : t('admin.users.columns.username'),
    sortable: false
  },
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: false },
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false },
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true },
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true },
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false }
])

// Columns that can be toggled (exclude user and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'user' && col.key !== 'actions')
)

// Hidden columns set
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns
const DEFAULT_HIDDEN_COLUMNS: string[] = []

// localStorage key
const HIDDEN_COLUMNS_KEY = 'subscription-hidden-columns'

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    }
  } catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  }
}

// Save column settings to localStorage
const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  } catch (e) {
    console.error('Failed to save columns:', e)
  }
}

// Toggle column visibility
const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

// Check if column is visible
const isColumnVisible = (key: string) => !hiddenColumns.has(key)

// Filtered columns for display
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'user' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

// Column dropdown state
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allStatus') },
  { value: 'active', label: t('admin.subscriptions.status.active') },
  { value: 'expired', label: t('admin.subscriptions.status.expired') },
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') }
])

const subscriptions = ref<UserSubscription[]>([])
const groups = ref<Group[]>([])
const loading = ref(false)
let abortController: AbortController | null = null

// Toolbar user filter (fuzzy search -> select user_id)
const filterUserKeyword = ref('')
const filterUserResults = ref<SimpleUser[]>([])
const filterUserLoading = ref(false)
const showFilterUserDropdown = ref(false)
const selectedFilterUser = ref<SimpleUser | null>(null)
let filterUserSearchTimeout: ReturnType<typeof setTimeout> | null = null

// User search state
const userSearchKeyword = ref('')
const userSearchResults = ref<SimpleUser[]>([])
const userSearchLoading = ref(false)
const showUserDropdown = ref(false)
const selectedUser = ref<SimpleUser | null>(null)
let userSearchTimeout: ReturnType<typeof setTimeout> | null = null

// Benefit package / plan state
const benefitPackages = ref<BenefitPackage[]>([])
const benefitPlans = ref<BenefitPlan[]>([])
const benefitPackagesLoading = ref(false)
const benefitPlansLoading = ref(false)
const benefitSaving = ref(false)

const showBenefitPackageModal = ref(false)
const editingBenefitPackage = ref<BenefitPackage | null>(null)
const benefitPackageForm = reactive({
  name: '',
  description: '',
  group_id: null as number | null,
  lease_days: 30
})

const showBenefitPlanModal = ref(false)
const editingBenefitPlan = ref<BenefitPlan | null>(null)
const benefitPlanForm = reactive({
  name: '',
  description: '',
  package_ids: [] as number[]
})

// Benefit plan member management state
const showBenefitPlanMembersModal = ref(false)
const activeBenefitPlanForMembers = ref<BenefitPlan | null>(null)
const benefitPlanMembers = ref<BenefitPlanMember[]>([])
const benefitPlanMembersLoading = ref(false)
const benefitPlanMembersSubmitting = ref(false)
const selectedBenefitPlanMemberIds = ref<number[]>([])
const benefitPlanMemberSearchKeyword = ref('')
const benefitPlanMemberSearchResults = ref<SimpleUser[]>([])
const benefitPlanMemberSearchLoading = ref(false)
const showBenefitPlanMemberSearchDropdown = ref(false)
const selectedBenefitPlanAddUsers = ref<SimpleUser[]>([])
let benefitPlanMemberSearchTimeout: ReturnType<typeof setTimeout> | null = null

const filters = reactive({
  status: 'active',
  group_id: '',
  platform: '',
  user_id: null as number | null
})

// Sorting state
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
})

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const showAssignModal = ref(false)
const showExtendModal = ref(false)
const showRevokeDialog = ref(false)
const showRestoreDialog = ref(false)
const showResetQuotaConfirm = ref(false)
const submitting = ref(false)
const resettingSubscription = ref<UserSubscription | null>(null)
const resettingQuota = ref(false)
const extendingSubscription = ref<UserSubscription | null>(null)
const revokingSubscription = ref<UserSubscription | null>(null)
const restoringSubscription = ref<UserSubscription | null>(null)

const assignForm = reactive({
  user_id: null as number | null,
  group_id: null as number | null,
  validity_days: 30
})

const extendForm = reactive({
  days: 30
})

// Group options for filter (all groups)
const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') },
  ...groups.value.map((g) => ({ value: g.id.toString(), label: g.name }))
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'openai', label: 'OpenAI' },
  { value: 'gemini', label: 'Gemini' },
  { value: 'antigravity', label: 'Antigravity' }
])

// Group options for assign (only subscription type groups)
const subscriptionGroupOptions = computed(() =>
  groups.value
    .filter((g) => g.subscription_type === 'subscription' && g.status === 'active')
    .map((g) => ({
      value: g.id,
      label: g.name,
      description: g.description,
      platform: g.platform,
      subscriptionType: g.subscription_type,
      rate: g.rate_multiplier
    }))
)

const benefitPlanMembersModalTitle = computed(() =>
  activeBenefitPlanForMembers.value
    ? `${activeBenefitPlanForMembers.value.name} · ${t('admin.subscriptions.benefitPlanMembers.title')}`
    : t('admin.subscriptions.benefitPlanMembers.title')
)

const benefitPlanMemberIdSet = computed(() =>
  new Set(benefitPlanMembers.value.map((member) => member.user_id))
)

const selectedBenefitPlanAddUserIdSet = computed(() =>
  new Set(selectedBenefitPlanAddUsers.value.map((user) => user.id))
)

const applyFilters = () => {
  pagination.page = 1
  loadSubscriptions()
}

const loadSubscriptions = async () => {
  if (abortController) {
    abortController.abort()
  }
  const requestController = new AbortController()
  abortController = requestController
  const { signal } = requestController

  loading.value = true
  try {
    const response = await adminAPI.subscriptions.list(
      pagination.page,
      pagination.page_size,
      {
        status: (filters.status as any) || undefined,
        group_id: filters.group_id ? parseInt(filters.group_id) : undefined,
        platform: filters.platform || undefined,
        user_id: filters.user_id || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      {
        signal
      }
    )
    if (signal.aborted || abortController !== requestController) return
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  } catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    }
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  } finally {
    if (abortController === requestController) {
      loading.value = false
      abortController = null
    }
  }
}

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  } catch (error) {
    console.error('Error loading groups:', error)
  }
}

const loadBenefitPackages = async () => {
  benefitPackagesLoading.value = true
  try {
    benefitPackages.value = await adminAPI.subscriptions.listBenefitPackages()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPackages.failedToLoad'))
    console.error('Error loading benefit packages:', error)
  } finally {
    benefitPackagesLoading.value = false
  }
}

const loadBenefitPlans = async () => {
  benefitPlansLoading.value = true
  try {
    benefitPlans.value = await adminAPI.subscriptions.listBenefitPlans()
    if (activeBenefitPlanForMembers.value) {
      const refreshedPlan = benefitPlans.value.find((plan) => plan.id === activeBenefitPlanForMembers.value?.id)
      if (refreshedPlan) {
        activeBenefitPlanForMembers.value = refreshedPlan
      } else {
        closeBenefitPlanMembersModal()
      }
    }
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlans.failedToLoad'))
    console.error('Error loading benefit plans:', error)
  } finally {
    benefitPlansLoading.value = false
  }
}

const loadBenefitData = async () => {
  await Promise.all([loadBenefitPackages(), loadBenefitPlans()])
}

const resetBenefitPackageForm = () => {
  benefitPackageForm.name = ''
  benefitPackageForm.description = ''
  benefitPackageForm.group_id = null
  benefitPackageForm.lease_days = 30
}

const openCreateBenefitPackageModal = () => {
  editingBenefitPackage.value = null
  resetBenefitPackageForm()
  showBenefitPackageModal.value = true
}

const openEditBenefitPackageModal = (item: BenefitPackage) => {
  editingBenefitPackage.value = item
  benefitPackageForm.name = item.name
  benefitPackageForm.description = item.description || ''
  benefitPackageForm.group_id = item.group_id
  benefitPackageForm.lease_days = item.lease_days
  showBenefitPackageModal.value = true
}

const closeBenefitPackageModal = () => {
  showBenefitPackageModal.value = false
  editingBenefitPackage.value = null
  resetBenefitPackageForm()
}

const handleSaveBenefitPackage = async () => {
  if (!benefitPackageForm.name.trim()) {
    appStore.showError(t('admin.subscriptions.benefitPackages.nameRequired'))
    return
  }
  if (!benefitPackageForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  }
  if (!benefitPackageForm.lease_days || benefitPackageForm.lease_days < 1) {
    appStore.showError(t('admin.subscriptions.benefitPackages.daysRequired'))
    return
  }

  benefitSaving.value = true
  try {
    const payload = {
      name: benefitPackageForm.name.trim(),
      description: benefitPackageForm.description.trim(),
      group_id: benefitPackageForm.group_id,
      lease_days: benefitPackageForm.lease_days
    }
    if (editingBenefitPackage.value) {
      await adminAPI.subscriptions.updateBenefitPackage(editingBenefitPackage.value.id, payload)
      appStore.showSuccess(t('admin.subscriptions.benefitPackages.updated'))
    } else {
      await adminAPI.subscriptions.createBenefitPackage(payload)
      appStore.showSuccess(t('admin.subscriptions.benefitPackages.created'))
    }
    closeBenefitPackageModal()
    await Promise.all([loadBenefitPackages(), loadBenefitPlans()])
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPackages.saveFailed'))
    console.error('Error saving benefit package:', error)
  } finally {
    benefitSaving.value = false
  }
}

const handleDeleteBenefitPackage = async (item: BenefitPackage) => {
  if (!window.confirm(t('admin.subscriptions.benefitPackages.deleteConfirm', { name: item.name }))) {
    return
  }
  try {
    await adminAPI.subscriptions.deleteBenefitPackage(item.id)
    appStore.showSuccess(t('admin.subscriptions.benefitPackages.deleted'))
    await Promise.all([loadBenefitPackages(), loadBenefitPlans()])
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPackages.deleteFailed'))
    console.error('Error deleting benefit package:', error)
  }
}

const resetBenefitPlanForm = () => {
  benefitPlanForm.name = ''
  benefitPlanForm.description = ''
  benefitPlanForm.package_ids = []
}

const openCreateBenefitPlanModal = () => {
  editingBenefitPlan.value = null
  resetBenefitPlanForm()
  showBenefitPlanModal.value = true
}

const openEditBenefitPlanModal = (item: BenefitPlan) => {
  editingBenefitPlan.value = item
  benefitPlanForm.name = item.name
  benefitPlanForm.description = item.description || ''
  benefitPlanForm.package_ids = item.packages.map((pkg) => pkg.package_id)
  showBenefitPlanModal.value = true
}

const closeBenefitPlanModal = () => {
  showBenefitPlanModal.value = false
  editingBenefitPlan.value = null
  resetBenefitPlanForm()
}

const handleSaveBenefitPlan = async () => {
  if (!benefitPlanForm.name.trim()) {
    appStore.showError(t('admin.subscriptions.benefitPlans.nameRequired'))
    return
  }
  if (benefitPlanForm.package_ids.length === 0) {
    appStore.showError(t('admin.subscriptions.benefitPlans.packagesRequired'))
    return
  }

  benefitSaving.value = true
  try {
    const payload = {
      name: benefitPlanForm.name.trim(),
      description: benefitPlanForm.description.trim(),
      package_ids: [...benefitPlanForm.package_ids]
    }
    if (editingBenefitPlan.value) {
      await adminAPI.subscriptions.updateBenefitPlan(editingBenefitPlan.value.id, payload)
      appStore.showSuccess(t('admin.subscriptions.benefitPlans.updated'))
    } else {
      await adminAPI.subscriptions.createBenefitPlan(payload)
      appStore.showSuccess(t('admin.subscriptions.benefitPlans.created'))
    }
    closeBenefitPlanModal()
    await loadBenefitPlans()
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlans.saveFailed'))
    console.error('Error saving benefit plan:', error)
  } finally {
    benefitSaving.value = false
  }
}

const handleDeleteBenefitPlan = async (item: BenefitPlan) => {
  if (!window.confirm(t('admin.subscriptions.benefitPlans.deleteConfirm', { name: item.name }))) {
    return
  }
  try {
    await adminAPI.subscriptions.deleteBenefitPlan(item.id)
    appStore.showSuccess(t('admin.subscriptions.benefitPlans.deleted'))
    await loadBenefitPlans()
    if (activeBenefitPlanForMembers.value?.id === item.id) {
      closeBenefitPlanMembersModal()
    }
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlans.deleteFailed'))
    console.error('Error deleting benefit plan:', error)
  }
}

const resetBenefitPlanMemberSelection = () => {
  selectedBenefitPlanMemberIds.value = []
  selectedBenefitPlanAddUsers.value = []
  benefitPlanMemberSearchKeyword.value = ''
  benefitPlanMemberSearchResults.value = []
  showBenefitPlanMemberSearchDropdown.value = false
}

const closeBenefitPlanMembersModal = () => {
  showBenefitPlanMembersModal.value = false
  activeBenefitPlanForMembers.value = null
  benefitPlanMembers.value = []
  benefitPlanMembersLoading.value = false
  benefitPlanMembersSubmitting.value = false
  resetBenefitPlanMemberSelection()
}

const loadBenefitPlanMembers = async (planID: number) => {
  benefitPlanMembersLoading.value = true
  try {
    benefitPlanMembers.value = await adminAPI.subscriptions.listBenefitPlanMembers(planID)
    const memberIDs = new Set(benefitPlanMembers.value.map((member) => member.user_id))
    selectedBenefitPlanMemberIds.value = selectedBenefitPlanMemberIds.value.filter((userID) => memberIDs.has(userID))
    selectedBenefitPlanAddUsers.value = selectedBenefitPlanAddUsers.value.filter((user) => !memberIDs.has(user.id))
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlanMembers.loadFailed'))
    console.error('Failed to load benefit plan members:', error)
  } finally {
    benefitPlanMembersLoading.value = false
  }
}

const openBenefitPlanMembersModal = (plan: BenefitPlan) => {
  activeBenefitPlanForMembers.value = plan
  showBenefitPlanMembersModal.value = true
  benefitPlanMembers.value = []
  resetBenefitPlanMemberSelection()
  void loadBenefitPlanMembers(plan.id)
}

const debounceSearchBenefitPlanUsers = () => {
  if (benefitPlanMemberSearchTimeout) {
    clearTimeout(benefitPlanMemberSearchTimeout)
  }
  benefitPlanMemberSearchTimeout = setTimeout(searchBenefitPlanUsers, 300)
}

const searchBenefitPlanUsers = async () => {
  const keyword = benefitPlanMemberSearchKeyword.value.trim()
  if (!keyword) {
    benefitPlanMemberSearchResults.value = []
    return
  }
  benefitPlanMemberSearchLoading.value = true
  try {
    benefitPlanMemberSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users for benefit plan members:', error)
    benefitPlanMemberSearchResults.value = []
  } finally {
    benefitPlanMemberSearchLoading.value = false
  }
}

const toggleBenefitPlanMemberSelection = (userID: number) => {
  if (selectedBenefitPlanMemberIds.value.includes(userID)) {
    selectedBenefitPlanMemberIds.value = selectedBenefitPlanMemberIds.value.filter((id) => id !== userID)
    return
  }
  selectedBenefitPlanMemberIds.value = [...selectedBenefitPlanMemberIds.value, userID]
}

const toggleBenefitPlanAddSelection = (user: SimpleUser) => {
  if (benefitPlanMemberIdSet.value.has(user.id)) {
    return
  }
  if (selectedBenefitPlanAddUserIdSet.value.has(user.id)) {
    selectedBenefitPlanAddUsers.value = selectedBenefitPlanAddUsers.value.filter((item) => item.id !== user.id)
    return
  }
  selectedBenefitPlanAddUsers.value = [...selectedBenefitPlanAddUsers.value, user]
}

const handleBulkAssignBenefitPlanUsers = async () => {
  const plan = activeBenefitPlanForMembers.value
  const userIDs = selectedBenefitPlanAddUsers.value.map((user) => user.id)
  if (!plan) {
    return
  }
  if (userIDs.length === 0) {
    appStore.showError(t('admin.subscriptions.benefitPlanMembers.selectAtLeastOneUser'))
    return
  }
  benefitPlanMembersSubmitting.value = true
  try {
    const result = await adminAPI.subscriptions.bulkAssignBenefitPlanUsers(plan.id, userIDs)
    if (result.failed_count > 0 && result.success_count === 0) {
      appStore.showError(result.errors[0] || t('admin.subscriptions.benefitPlanMembers.addFailed'))
    } else {
      appStore.showSuccess(t('admin.subscriptions.benefitPlanMembers.added'))
      if (result.failed_count > 0 && result.errors.length > 0) {
        appStore.showError(result.errors[0])
      }
    }
    resetBenefitPlanMemberSelection()
    await Promise.all([loadBenefitPlanMembers(plan.id), loadBenefitPlans(), loadSubscriptions()])
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlanMembers.addFailed'))
    console.error('Failed to bulk assign benefit plan users:', error)
  } finally {
    benefitPlanMembersSubmitting.value = false
  }
}

const handleBulkRemoveBenefitPlanUsers = async () => {
  const plan = activeBenefitPlanForMembers.value
  if (!plan) {
    return
  }
  if (selectedBenefitPlanMemberIds.value.length === 0) {
    appStore.showError(t('admin.subscriptions.benefitPlanMembers.selectAtLeastOneUser'))
    return
  }
  benefitPlanMembersSubmitting.value = true
  try {
    const result = await adminAPI.subscriptions.bulkRemoveBenefitPlanUsers(
      plan.id,
      selectedBenefitPlanMemberIds.value
    )
    if (result.failed_count > 0 && result.success_count === 0) {
      appStore.showError(result.errors[0] || t('admin.subscriptions.benefitPlanMembers.removeFailed'))
    } else {
      appStore.showSuccess(t('admin.subscriptions.benefitPlanMembers.removed'))
      if (result.failed_count > 0 && result.errors.length > 0) {
        appStore.showError(result.errors[0])
      }
    }
    selectedBenefitPlanMemberIds.value = []
    await Promise.all([loadBenefitPlanMembers(plan.id), loadBenefitPlans(), loadSubscriptions()])
  } catch (error: any) {
    appStore.showError(error?.response?.data?.detail || t('admin.subscriptions.benefitPlanMembers.removeFailed'))
    console.error('Failed to bulk remove benefit plan users:', error)
  } finally {
    benefitPlanMembersSubmitting.value = false
  }
}

// Toolbar user filter search with debounce
const debounceSearchFilterUsers = () => {
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  filterUserSearchTimeout = setTimeout(searchFilterUsers, 300)
}

const searchFilterUsers = async () => {
  const keyword = filterUserKeyword.value.trim()

  // Clear active user filter if user modified the search keyword
  if (selectedFilterUser.value && keyword !== selectedFilterUser.value.email) {
    selectedFilterUser.value = null
    filters.user_id = null
    applyFilters()
  }

  if (!keyword) {
    filterUserResults.value = []
    return
  }

  filterUserLoading.value = true
  try {
    filterUserResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    filterUserResults.value = []
  } finally {
    filterUserLoading.value = false
  }
}

const selectFilterUser = (user: SimpleUser) => {
  selectedFilterUser.value = user
  filterUserKeyword.value = user.email
  showFilterUserDropdown.value = false
  filters.user_id = user.id
  applyFilters()
}

const clearFilterUser = () => {
  selectedFilterUser.value = null
  filterUserKeyword.value = ''
  filterUserResults.value = []
  showFilterUserDropdown.value = false
  filters.user_id = null
  applyFilters()
}

// User search with debounce
const debounceSearchUsers = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  userSearchTimeout = setTimeout(searchUsers, 300)
}

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()

  // Clear selection if user modified the search keyword
  if (selectedUser.value && keyword !== selectedUser.value.email) {
    selectedUser.value = null
    assignForm.user_id = null
  }

  if (!keyword) {
    userSearchResults.value = []
    return
  }

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  } catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  } finally {
    userSearchLoading.value = false
  }
}

const selectUser = (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  assignForm.user_id = user.id
}

const clearUserSelection = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  assignForm.user_id = null
}

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSubscriptions()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSubscriptions()
}

const closeAssignModal = () => {
  showAssignModal.value = false
  assignForm.user_id = null
  assignForm.group_id = null
  assignForm.validity_days = 30
  // Clear user search state
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  showUserDropdown.value = false
}

const handleAssignSubscription = async () => {
  if (!assignForm.user_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  }
  if (!assignForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  }
  if (!assignForm.validity_days || assignForm.validity_days < 1) {
    appStore.showError(t('admin.subscriptions.validityDaysRequired'))
    return
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign({
      user_id: assignForm.user_id,
      group_id: assignForm.group_id,
      validity_days: assignForm.validity_days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  showExtendModal.value = true
}

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
}

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  // 前端验证：调整后的过期时间必须在未来
  if (extendingSubscription.value.expires_at) {
    const expiresAt = new Date(extendingSubscription.value.expires_at)
    const newExpiresAt = new Date(expiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000)
    if (newExpiresAt <= new Date()) {
      appStore.showError(t('admin.subscriptions.adjustWouldExpire'))
      return
    }
  }

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, {
      days: extendForm.days
    })
    appStore.showSuccess(t('admin.subscriptions.subscriptionAdjusted'))
    closeExtendModal()
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAdjust'))
    console.error('Error adjusting subscription:', error)
  } finally {
    submitting.value = false
  }
}

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
}

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  }
}

const handleRestore = (subscription: UserSubscription) => {
  restoringSubscription.value = subscription
  showRestoreDialog.value = true
}

const confirmRestore = async () => {
  if (!restoringSubscription.value) return

  try {
    await adminAPI.subscriptions.restore(restoringSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRestored'))
    showRestoreDialog.value = false
    restoringSubscription.value = null
    loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRestore'))
    console.error('Error restoring subscription:', error)
  }
}

const handleResetQuota = (subscription: UserSubscription) => {
  resettingSubscription.value = subscription
  showResetQuotaConfirm.value = true
}

const confirmResetQuota = async () => {
  if (!resettingSubscription.value) return
  if (resettingQuota.value) return
  resettingQuota.value = true
  try {
    await adminAPI.subscriptions.resetQuota(resettingSubscription.value.id, { daily: true, weekly: true, monthly: true })
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    showResetQuotaConfirm.value = false
    resettingSubscription.value = null
    await loadSubscriptions()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToResetQuota'))
    console.error('Error resetting quota:', error)
  } finally {
    resettingQuota.value = false
  }
}

// Helper functions
const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
}

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
}

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentage}%`
}

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
}

const formatResetDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes })
}

const formatQuotaEndDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.quotaEndsInDaysHours', { days: parts.days, hours: parts.hours })
  }

  if (parts.hours > 0) {
    return t('admin.subscriptions.quotaEndsInHoursMinutes', { hours: parts.hours, minutes: parts.minutes })
  }

  return t('admin.subscriptions.quotaEndsInMinutes', { minutes: parts.minutes })
}

const formatDailyUsageWindow = (subscription: UserSubscription): string => {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    return parts ? formatQuotaEndDuration(parts) : t('admin.subscriptions.windowNotActive')
  }

  return formatResetTime(subscription.daily_window_start, 'daily')
}

// Format reset time based on window start and period type
const formatResetTime = (windowStart: string | null, period: 'daily' | 'weekly' | 'monthly'): string => {
  if (!windowStart) return t('admin.subscriptions.windowNotActive')

  const start = new Date(windowStart)
  const now = new Date()

  // Calculate reset time based on period
  let resetTime: Date
  switch (period) {
    case 'daily':
      resetTime = new Date(start.getTime() + 24 * 60 * 60 * 1000)
      break
    case 'weekly':
      resetTime = new Date(start.getTime() + 7 * 24 * 60 * 60 * 1000)
      break
    case 'monthly':
      resetTime = new Date(start.getTime() + 30 * 24 * 60 * 60 * 1000)
      break
  }

  const parts = getRemainingDurationParts(resetTime, now)

  return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
}

// Handle click outside to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('[data-assign-user-search]')) showUserDropdown.value = false
  if (!target.closest('[data-filter-user-search]')) showFilterUserDropdown.value = false
  if (!target.closest('[data-benefit-plan-member-search]')) showBenefitPlanMemberSearchDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

onMounted(() => {
  loadUserColumnMode()
  loadSavedColumns()
  loadSubscriptions()
  loadGroups()
  loadBenefitData()
  document.addEventListener('click', handleClickOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  }
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  }
  if (benefitPlanMemberSearchTimeout) {
    clearTimeout(benefitPlanMemberSearchTimeout)
  }
})
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
}

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
}

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
}

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
}
</style>
