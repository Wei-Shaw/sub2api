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
                  {{ t('common.loading') REDACTEDREDACTED
                </div>
                <div
                  v-else-if="filterUserResults.length === 0 && filterUserKeyword"
                  class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
                >
                  {{ t('common.noOptionsFound') REDACTEDREDACTED
                </div>
                <button
                  v-for="user in filterUserResults"
                  :key="user.id"
                  type="button"
                  @click="selectFilterUser(user)"
                  class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
                >
                  <span class="font-medium text-gray-900 dark:text-white">{{ user.email REDACTEDREDACTED</span>
                  <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id REDACTEDREDACTED</span>
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
                <span class="hidden md:inline">{{ t('admin.users.columnSettings') REDACTEDREDACTED</span>
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
                      {{ t('admin.subscriptions.columns.user') REDACTEDREDACTED
                    </div>
                    <button
                      @click="setUserColumnMode('email')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.email') REDACTEDREDACTED</span>
                      <Icon v-if="userColumnMode === 'email'" name="check" size="sm" class="text-primary-500" />
                    </button>
                    <button
                      @click="setUserColumnMode('username')"
                      class="flex w-full items-center justify-between rounded-md px-3 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-200 dark:hover:bg-gray-700"
                    >
                      <span>{{ t('admin.users.columns.username') REDACTEDREDACTED</span>
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
                    <span>{{ col.label REDACTEDREDACTED</span>
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
              {{ t('admin.subscriptions.assignSubscription') REDACTEDREDACTED
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
          <template #cell-user="{ row REDACTED">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ userColumnMode === 'email'
                    ? (row.user?.email?.charAt(0).toUpperCase() || '?')
                    : (row.user?.username?.charAt(0).toUpperCase() || '?')
                  REDACTEDREDACTED
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">
                {{ userColumnMode === 'email'
                  ? (row.user?.email || t('admin.redeem.userPrefix', { id: row.user_id REDACTED))
                  : (row.user?.username || '-')
                REDACTEDREDACTED
              </span>
            </div>
          </template>

          <template #cell-group="{ row REDACTED">
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

          <template #cell-usage="{ row REDACTED">
            <div class="min-w-[280px] space-y-2">
              <!-- Daily Usage -->
              <div v-if="row.group?.daily_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.daily') REDACTEDREDACTED</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.daily_usage_usd, row.group?.daily_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.daily_usage_usd, row.group?.daily_limit_usd)
                      REDACTED"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.daily_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.daily_limit_usd?.toFixed(2) REDACTEDREDACTED
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
                  <span>{{ formatDailyUsageWindow(row) REDACTEDREDACTED</span>
                </div>
              </div>

              <!-- Weekly Usage -->
              <div v-if="row.group?.weekly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.weekly') REDACTEDREDACTED</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.weekly_usage_usd, row.group?.weekly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.weekly_usage_usd, row.group?.weekly_limit_usd)
                      REDACTED"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.weekly_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.weekly_limit_usd?.toFixed(2) REDACTEDREDACTED
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
                  <span>{{ formatResetTime(row.weekly_window_start, 'weekly') REDACTEDREDACTED</span>
                </div>
              </div>

              <!-- Monthly Usage -->
              <div v-if="row.group?.monthly_limit_usd" class="usage-row">
                <div class="flex items-center gap-2">
                  <span class="usage-label">{{ t('admin.subscriptions.monthly') REDACTEDREDACTED</span>
                  <div class="h-1.5 flex-1 rounded-full bg-gray-200 dark:bg-dark-600">
                    <div
                      class="h-1.5 rounded-full transition-all"
                      :class="getProgressClass(row.monthly_usage_usd, row.group?.monthly_limit_usd)"
                      :style="{
                        width: getProgressWidth(row.monthly_usage_usd, row.group?.monthly_limit_usd)
                      REDACTED"
                    ></div>
                  </div>
                  <span class="usage-amount">
                    ${{ row.monthly_usage_usd?.toFixed(2) || '0.00' REDACTEDREDACTED
                    <span class="text-gray-400">/</span>
                    ${{ row.group?.monthly_limit_usd?.toFixed(2) REDACTEDREDACTED
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
                  <span>{{ formatResetTime(row.monthly_window_start, 'monthly') REDACTEDREDACTED</span>
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
                  {{ t('admin.subscriptions.unlimited') REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>

          <template #cell-expires_at="{ value REDACTED">
            <div v-if="value">
              <span
                class="text-sm"
                :class="
                  isExpiringSoon(value)
                    ? 'text-orange-600 dark:text-orange-400'
                    : 'text-gray-700 dark:text-gray-300'
                "
              >
                {{ formatDateOnly(value) REDACTEDREDACTED
              </span>
              <div v-if="getDaysRemaining(value) !== null" class="text-xs text-gray-500">
                {{ getDaysRemaining(value) REDACTEDREDACTED {{ t('admin.subscriptions.daysRemaining') REDACTEDREDACTED
              </div>
            </div>
            <span v-else class="text-sm text-gray-500">{{
              t('admin.subscriptions.noExpiration')
            REDACTEDREDACTED</span>
          </template>

          <template #cell-status="{ value REDACTED">
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
              {{ t(`admin.subscriptions.status.${valueREDACTED`) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                v-if="row.status === 'active' || row.status === 'expired'"
                @click="handleExtend(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20 dark:hover:text-blue-400"
              >
                <Icon name="calendar" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.adjust') REDACTEDREDACTED</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleResetQuota(row)"
                :disabled="resettingQuota && resettingSubscription?.id === row.id"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.resetQuota') REDACTEDREDACTED</span>
              </button>
              <button
                v-if="row.status === 'active'"
                @click="handleRevoke(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="ban" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.revoke') REDACTEDREDACTED</span>
              </button>
              <button
                v-if="row.status === 'revoked'"
                @click="handleRestore(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400"
              >
                <Icon name="refresh" size="sm" />
                <span class="text-xs">{{ t('admin.subscriptions.restore') REDACTEDREDACTED</span>
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
          <label class="input-label">{{ t('admin.subscriptions.form.user') REDACTEDREDACTED</label>
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
                {{ t('common.loading') REDACTEDREDACTED
              </div>
              <div
                v-else-if="userSearchResults.length === 0 && userSearchKeyword"
                class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400"
              >
                {{ t('common.noOptionsFound') REDACTEDREDACTED
              </div>
              <button
                v-for="user in userSearchResults"
                :key="user.id"
                type="button"
                @click="selectUser(user)"
                class="w-full px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-gray-700"
              >
                <span class="font-medium text-gray-900 dark:text-white">{{ user.email REDACTEDREDACTED</span>
                <span class="ml-2 text-gray-500 dark:text-gray-400">#{{ user.id REDACTEDREDACTED</span>
              </button>
            </div>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.group') REDACTEDREDACTED</label>
          <Select
            v-model="assignForm.group_id"
            :options="subscriptionGroupOptions"
            :placeholder="t('admin.subscriptions.selectGroup')"
          >
            <template #selected="{ option REDACTED">
              <GroupBadge
                v-if="option"
                :name="(option as unknown as GroupOption).label"
                :platform="(option as unknown as GroupOption).platform"
                :subscription-type="(option as unknown as GroupOption).subscriptionType"
                :rate-multiplier="(option as unknown as GroupOption).rate"
              />
              <span v-else class="text-gray-400">{{ t('admin.subscriptions.selectGroup') REDACTEDREDACTED</span>
            </template>
            <template #option="{ option, selected REDACTED">
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
          <p class="input-hint">{{ t('admin.subscriptions.groupHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.validityDays') REDACTEDREDACTED</label>
          <input v-model.number="assignForm.validity_days" type="number" min="1" class="input" />
          <p class="input-hint">{{ t('admin.subscriptions.validityHint') REDACTEDREDACTED</p>
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAssignModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
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
            {{ submitting ? t('admin.subscriptions.assigning') : t('admin.subscriptions.assign') REDACTEDREDACTED
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
            {{ t('admin.subscriptions.adjustingFor') REDACTEDREDACTED
            <span class="font-medium text-gray-900 dark:text-white">{{
              extendingSubscription.user?.email
            REDACTEDREDACTED</span>
          </p>
          <p class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.currentExpiration') REDACTEDREDACTED:
            <span class="font-medium text-gray-900 dark:text-white">
              {{
                extendingSubscription.expires_at
                  ? formatDateOnly(extendingSubscription.expires_at)
                  : t('admin.subscriptions.noExpiration')
              REDACTEDREDACTED
            </span>
          </p>
          <p v-if="extendingSubscription.expires_at" class="mt-1 text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.subscriptions.remainingDays') REDACTEDREDACTED:
            <span class="font-medium text-gray-900 dark:text-white">
              {{ getDaysRemaining(extendingSubscription.expires_at) ?? 0 REDACTEDREDACTED
            </span>
          </p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.subscriptions.form.adjustDays') REDACTEDREDACTED</label>
          <div class="flex items-center gap-2">
            <input
              v-model.number="extendForm.days"
              type="number"
              required
              class="input text-center"
              :placeholder="t('admin.subscriptions.adjustDaysPlaceholder')"
            />
          </div>
          <p class="input-hint">{{ t('admin.subscriptions.adjustHint') REDACTEDREDACTED</p>
        </div>
      </form>
      <template #footer>
        <div v-if="extendingSubscription" class="flex justify-end gap-3">
          <button @click="closeExtendModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="extend-subscription-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting ? t('admin.subscriptions.adjusting') : t('admin.subscriptions.adjust') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Revoke Confirmation Dialog -->
    <ConfirmDialog
      :show="showRevokeDialog"
      :title="t('admin.subscriptions.revokeSubscription')"
      :message="t('admin.subscriptions.revokeConfirm', { user: revokingSubscription?.user?.email REDACTED)"
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
      :message="t('admin.subscriptions.restoreConfirm', { user: restoringSubscription?.user?.email REDACTED)"
      :confirm-text="t('admin.subscriptions.restore')"
      :cancel-text="t('common.cancel')"
      @confirm="confirmRestore"
      @cancel="showRestoreDialog = false"
    />

    <!-- Reset Quota Confirmation Dialog -->
    <ConfirmDialog
      :show="showResetQuotaConfirm"
      :title="t('admin.subscriptions.resetQuotaTitle')"
      :message="t('admin.subscriptions.resetQuotaConfirm', { user: resettingSubscription?.user?.email REDACTED)"
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

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.subscriptions.guide.title') REDACTEDREDACTED</h2>
            <p class="mb-5 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.subscriptions.guide.subtitle') REDACTEDREDACTED</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.subscriptions.guide.step1.title') REDACTEDREDACTED
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step1.line1') REDACTEDREDACTED</li>
                <li>{{ t('admin.subscriptions.guide.step1.line2') REDACTEDREDACTED</li>
                <li>{{ t('admin.subscriptions.guide.step1.line3') REDACTEDREDACTED</li>
              </ol>
              <div class="ml-8 mt-2">
                <router-link
                  to="/admin/groups"
                  @click="showGuideModal = false"
                  class="inline-flex items-center gap-1 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                >
                  {{ t('admin.subscriptions.guide.step1.link') REDACTEDREDACTED
                  <Icon name="arrowRight" size="xs" />
                </router-link>
              </div>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.subscriptions.guide.step2.title') REDACTEDREDACTED
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.subscriptions.guide.step2.line1') REDACTEDREDACTED</li>
                <li>{{ t('admin.subscriptions.guide.step2.line2') REDACTEDREDACTED</li>
                <li>{{ t('admin.subscriptions.guide.step2.line3') REDACTEDREDACTED</li>
              </ol>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.subscriptions.guide.step3.title') REDACTEDREDACTED
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in guideActionRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.action REDACTEDREDACTED</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400">{{ row.desc REDACTEDREDACTED</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Tip -->
            <div class="rounded-lg bg-blue-50 p-3 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
              {{ t('admin.subscriptions.guide.tip') REDACTEDREDACTED
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showGuideModal = false">{{ t('common.close') REDACTEDREDACTED</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { UserSubscription, Group, GroupPlatform, SubscriptionType REDACTED from '@/types'
import type { SimpleUser REDACTED from '@/api/admin/usage'
import type { Column REDACTED from '@/components/common/types'
import { formatDateOnly REDACTED from '@/utils/format'
import { getPersistedPageSize REDACTED from '@/composables/usePersistedPageSize'
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
import { getRemainingDurationParts, isOneTimeDailyQuota, type RemainingDurationParts REDACTED from '@/utils/subscriptionQuota'

const { t REDACTED = useI18n()
const appStore = useAppStore()

interface GroupOption {
  value: number
  label: string
  description: string | null
  platform: GroupPlatform
  subscriptionType: SubscriptionType
  rate: number
REDACTED

// Guide modal state
const showGuideModal = ref(false)

const guideActionRows = computed(() => [
  { action: t('admin.subscriptions.guide.actions.adjust'), desc: t('admin.subscriptions.guide.actions.adjustDesc') REDACTED,
  { action: t('admin.subscriptions.guide.actions.resetQuota'), desc: t('admin.subscriptions.guide.actions.resetQuotaDesc') REDACTED,
  { action: t('admin.subscriptions.guide.actions.revoke'), desc: t('admin.subscriptions.guide.actions.revokeDesc') REDACTED
])

// User column display mode: 'email' or 'username'
const userColumnMode = ref<'email' | 'username'>('email')
const USER_COLUMN_MODE_KEY = 'subscription-user-column-mode'

const loadUserColumnMode = () => {
  try {
    const saved = localStorage.getItem(USER_COLUMN_MODE_KEY)
    if (saved === 'email' || saved === 'username') {
      userColumnMode.value = saved
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load user column mode:', e)
  REDACTED
REDACTED

const saveUserColumnMode = () => {
  try {
    localStorage.setItem(USER_COLUMN_MODE_KEY, userColumnMode.value)
  REDACTED catch (e) {
    console.error('Failed to save user column mode:', e)
  REDACTED
REDACTED

const setUserColumnMode = (mode: 'email' | 'username') => {
  userColumnMode.value = mode
  saveUserColumnMode()
REDACTED

// All available columns
const allColumns = computed<Column[]>(() => [
  {
    key: 'user',
    label: userColumnMode.value === 'email'
      ? t('admin.subscriptions.columns.user')
      : t('admin.users.columns.username'),
    sortable: false
  REDACTED,
  { key: 'group', label: t('admin.subscriptions.columns.group'), sortable: false REDACTED,
  { key: 'usage', label: t('admin.subscriptions.columns.usage'), sortable: false REDACTED,
  { key: 'expires_at', label: t('admin.subscriptions.columns.expires'), sortable: true REDACTED,
  { key: 'status', label: t('admin.subscriptions.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.subscriptions.columns.actions'), sortable: false REDACTED
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
    REDACTED else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved columns:', e)
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  REDACTED
REDACTED

// Save column settings to localStorage
const saveColumnsToStorage = () => {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  REDACTED catch (e) {
    console.error('Failed to save columns:', e)
  REDACTED
REDACTED

// Toggle column visibility
const toggleColumn = (key: string) => {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  REDACTED else {
    hiddenColumns.add(key)
  REDACTED
  saveColumnsToStorage()
REDACTED

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
  { value: '', label: t('admin.subscriptions.allStatus') REDACTED,
  { value: 'active', label: t('admin.subscriptions.status.active') REDACTED,
  { value: 'expired', label: t('admin.subscriptions.status.expired') REDACTED,
  { value: 'revoked', label: t('admin.subscriptions.status.revoked') REDACTED
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

const filters = reactive({
  status: 'active',
  group_id: '',
  platform: '',
  user_id: null as number | null
REDACTED)

// Sorting state
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
REDACTED)

const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
REDACTED)

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
REDACTED)

const extendForm = reactive({
  days: 30
REDACTED)

// Group options for filter (all groups)
const groupOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allGroups') REDACTED,
  ...groups.value.map((g) => ({ value: g.id.toString(), label: g.name REDACTED))
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.subscriptions.allPlatforms') REDACTED,
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
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
    REDACTED))
)

const applyFilters = () => {
  pagination.page = 1
  loadSubscriptions()
REDACTED

const loadSubscriptions = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const requestController = new AbortController()
  abortController = requestController
  const { signal REDACTED = requestController

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
      REDACTED,
      {
        signal
      REDACTED
    )
    if (signal.aborted || abortController !== requestController) return
    subscriptions.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('admin.subscriptions.failedToLoad'))
    console.error('Error loading subscriptions:', error)
  REDACTED finally {
    if (abortController === requestController) {
      loading.value = false
      abortController = null
    REDACTED
  REDACTED
REDACTED

const loadGroups = async () => {
  try {
    groups.value = await adminAPI.groups.getAll()
  REDACTED catch (error) {
    console.error('Error loading groups:', error)
  REDACTED
REDACTED

// Toolbar user filter search with debounce
const debounceSearchFilterUsers = () => {
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  REDACTED
  filterUserSearchTimeout = setTimeout(searchFilterUsers, 300)
REDACTED

const searchFilterUsers = async () => {
  const keyword = filterUserKeyword.value.trim()

  // Clear active user filter if user modified the search keyword
  if (selectedFilterUser.value && keyword !== selectedFilterUser.value.email) {
    selectedFilterUser.value = null
    filters.user_id = null
    applyFilters()
  REDACTED

  if (!keyword) {
    filterUserResults.value = []
    return
  REDACTED

  filterUserLoading.value = true
  try {
    filterUserResults.value = await adminAPI.usage.searchUsers(keyword)
  REDACTED catch (error) {
    console.error('Failed to search users:', error)
    filterUserResults.value = []
  REDACTED finally {
    filterUserLoading.value = false
  REDACTED
REDACTED

const selectFilterUser = (user: SimpleUser) => {
  selectedFilterUser.value = user
  filterUserKeyword.value = user.email
  showFilterUserDropdown.value = false
  filters.user_id = user.id
  applyFilters()
REDACTED

const clearFilterUser = () => {
  selectedFilterUser.value = null
  filterUserKeyword.value = ''
  filterUserResults.value = []
  showFilterUserDropdown.value = false
  filters.user_id = null
  applyFilters()
REDACTED

// User search with debounce
const debounceSearchUsers = () => {
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  REDACTED
  userSearchTimeout = setTimeout(searchUsers, 300)
REDACTED

const searchUsers = async () => {
  const keyword = userSearchKeyword.value.trim()

  // Clear selection if user modified the search keyword
  if (selectedUser.value && keyword !== selectedUser.value.email) {
    selectedUser.value = null
    assignForm.user_id = null
  REDACTED

  if (!keyword) {
    userSearchResults.value = []
    return
  REDACTED

  userSearchLoading.value = true
  try {
    userSearchResults.value = await adminAPI.usage.searchUsers(keyword)
  REDACTED catch (error) {
    console.error('Failed to search users:', error)
    userSearchResults.value = []
  REDACTED finally {
    userSearchLoading.value = false
  REDACTED
REDACTED

const selectUser = (user: SimpleUser) => {
  selectedUser.value = user
  userSearchKeyword.value = user.email
  showUserDropdown.value = false
  assignForm.user_id = user.id
REDACTED

const clearUserSelection = () => {
  selectedUser.value = null
  userSearchKeyword.value = ''
  userSearchResults.value = []
  assignForm.user_id = null
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadSubscriptions()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadSubscriptions()
REDACTED

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadSubscriptions()
REDACTED

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
REDACTED

const handleAssignSubscription = async () => {
  if (!assignForm.user_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectUser'))
    return
  REDACTED
  if (!assignForm.group_id) {
    appStore.showError(t('admin.subscriptions.pleaseSelectGroup'))
    return
  REDACTED
  if (!assignForm.validity_days || assignForm.validity_days < 1) {
    appStore.showError(t('admin.subscriptions.validityDaysRequired'))
    return
  REDACTED

  submitting.value = true
  try {
    await adminAPI.subscriptions.assign({
      user_id: assignForm.user_id,
      group_id: assignForm.group_id,
      validity_days: assignForm.validity_days
    REDACTED)
    appStore.showSuccess(t('admin.subscriptions.subscriptionAssigned'))
    closeAssignModal()
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAssign'))
    console.error('Error assigning subscription:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleExtend = (subscription: UserSubscription) => {
  extendingSubscription.value = subscription
  extendForm.days = 30
  showExtendModal.value = true
REDACTED

const closeExtendModal = () => {
  showExtendModal.value = false
  extendingSubscription.value = null
REDACTED

const handleExtendSubscription = async () => {
  if (!extendingSubscription.value) return

  // 前端验证：调整后的过期时间必须在未来
  if (extendingSubscription.value.expires_at) {
    const expiresAt = new Date(extendingSubscription.value.expires_at)
    const newExpiresAt = new Date(expiresAt.getTime() + extendForm.days * 24 * 60 * 60 * 1000)
    if (newExpiresAt <= new Date()) {
      appStore.showError(t('admin.subscriptions.adjustWouldExpire'))
      return
    REDACTED
  REDACTED

  submitting.value = true
  try {
    await adminAPI.subscriptions.extend(extendingSubscription.value.id, {
      days: extendForm.days
    REDACTED)
    appStore.showSuccess(t('admin.subscriptions.subscriptionAdjusted'))
    closeExtendModal()
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToAdjust'))
    console.error('Error adjusting subscription:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleRevoke = (subscription: UserSubscription) => {
  revokingSubscription.value = subscription
  showRevokeDialog.value = true
REDACTED

const confirmRevoke = async () => {
  if (!revokingSubscription.value) return

  try {
    await adminAPI.subscriptions.revoke(revokingSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRevoked'))
    showRevokeDialog.value = false
    revokingSubscription.value = null
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRevoke'))
    console.error('Error revoking subscription:', error)
  REDACTED
REDACTED

const handleRestore = (subscription: UserSubscription) => {
  restoringSubscription.value = subscription
  showRestoreDialog.value = true
REDACTED

const confirmRestore = async () => {
  if (!restoringSubscription.value) return

  try {
    await adminAPI.subscriptions.restore(restoringSubscription.value.id)
    appStore.showSuccess(t('admin.subscriptions.subscriptionRestored'))
    showRestoreDialog.value = false
    restoringSubscription.value = null
    loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToRestore'))
    console.error('Error restoring subscription:', error)
  REDACTED
REDACTED

const handleResetQuota = (subscription: UserSubscription) => {
  resettingSubscription.value = subscription
  showResetQuotaConfirm.value = true
REDACTED

const confirmResetQuota = async () => {
  if (!resettingSubscription.value) return
  if (resettingQuota.value) return
  resettingQuota.value = true
  try {
    await adminAPI.subscriptions.resetQuota(resettingSubscription.value.id, { daily: true, weekly: true, monthly: true REDACTED)
    appStore.showSuccess(t('admin.subscriptions.quotaResetSuccess'))
    showResetQuotaConfirm.value = false
    resettingSubscription.value = null
    await loadSubscriptions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.subscriptions.failedToResetQuota'))
    console.error('Error resetting quota:', error)
  REDACTED finally {
    resettingQuota.value = false
  REDACTED
REDACTED

// Helper functions
const getDaysRemaining = (expiresAt: string): number | null => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diff = expires.getTime() - now.getTime()
  if (diff < 0) return null
  return Math.ceil(diff / (1000 * 60 * 60 * 24))
REDACTED

const isExpiringSoon = (expiresAt: string): boolean => {
  const days = getDaysRemaining(expiresAt)
  return days !== null && days <= 7
REDACTED

const getProgressWidth = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return '0%'
  const usedValue = used ?? 0
  const percentage = Math.min((usedValue / limit) * 100, 100)
  return `${percentageREDACTED%`
REDACTED

const getProgressClass = (used: number | null | undefined, limit: number | null): string => {
  if (!limit || limit === 0) return 'bg-gray-400'
  const usedValue = used ?? 0
  const percentage = (usedValue / limit) * 100
  if (percentage >= 90) return 'bg-red-500'
  if (percentage >= 70) return 'bg-orange-500'
  return 'bg-green-500'
REDACTED

const formatResetDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.resetInDaysHours', { days: parts.days, hours: parts.hours REDACTED)
  REDACTED

  if (parts.hours > 0) {
    return t('admin.subscriptions.resetInHoursMinutes', { hours: parts.hours, minutes: parts.minutes REDACTED)
  REDACTED

  return t('admin.subscriptions.resetInMinutes', { minutes: parts.minutes REDACTED)
REDACTED

const formatQuotaEndDuration = (parts: RemainingDurationParts): string => {
  if (parts.days > 0) {
    return t('admin.subscriptions.quotaEndsInDaysHours', { days: parts.days, hours: parts.hours REDACTED)
  REDACTED

  if (parts.hours > 0) {
    return t('admin.subscriptions.quotaEndsInHoursMinutes', { hours: parts.hours, minutes: parts.minutes REDACTED)
  REDACTED

  return t('admin.subscriptions.quotaEndsInMinutes', { minutes: parts.minutes REDACTED)
REDACTED

const formatDailyUsageWindow = (subscription: UserSubscription): string => {
  if (isOneTimeDailyQuota(subscription) && subscription.expires_at) {
    const parts = getRemainingDurationParts(subscription.expires_at)
    return parts ? formatQuotaEndDuration(parts) : t('admin.subscriptions.windowNotActive')
  REDACTED

  return formatResetTime(subscription.daily_window_start, 'daily')
REDACTED

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
  REDACTED

  const parts = getRemainingDurationParts(resetTime, now)

  return parts ? formatResetDuration(parts) : t('admin.subscriptions.windowNotActive')
REDACTED

// Handle click outside to close dropdowns
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('[data-assign-user-search]')) showUserDropdown.value = false
  if (!target.closest('[data-filter-user-search]')) showFilterUserDropdown.value = false
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  REDACTED
REDACTED

onMounted(() => {
  loadUserColumnMode()
  loadSavedColumns()
  loadSubscriptions()
  loadGroups()
  document.addEventListener('click', handleClickOutside)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  if (filterUserSearchTimeout) {
    clearTimeout(filterUserSearchTimeout)
  REDACTED
  if (userSearchTimeout) {
    clearTimeout(userSearchTimeout)
  REDACTED
REDACTED)
</script>

<style scoped>
.usage-row {
  @apply space-y-1;
REDACTED

.usage-label {
  @apply w-10 flex-shrink-0 text-xs font-medium text-gray-500 dark:text-gray-400;
REDACTED

.usage-amount {
  @apply whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-gray-300;
REDACTED

.reset-info {
  @apply flex items-center gap-1 pl-12 text-[10px] text-blue-600 dark:text-blue-400;
REDACTED
</style>
