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
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
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
            <button
              @click="openSortModal"
              class="btn btn-secondary"
              :title="t('admin.groups.sortOrder')"
            >
              <Icon name="arrowsUpDown" size="md" class="mr-2" />
              {{ t("admin.groups.sortOrder") }}
            </button>
            <button
              @click="showCreateModal = true"
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
            <span class="font-medium text-gray-900 dark:text-white">{{
              value
            }}</span>
          </template>

          <template #cell-platform="{ value }">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : value === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : value === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
              ]"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ t("admin.groups.platforms." + value) }}
            </span>
          </template>

          <template #cell-billing_type="{ row }">
            <div class="space-y-1">
              <!-- Type Badge -->
              <span
                :class="[
                  'inline-block rounded-full px-2 py-0.5 text-xs font-medium',
                  row.subscription_type === 'subscription'
                    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300',
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
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                <template
                  v-if="
                    row.daily_limit_usd ||
                    row.weekly_limit_usd ||
                    row.monthly_limit_usd
                  "
                >
                  <span v-if="row.daily_limit_usd"
                    >${{ row.daily_limit_usd }}/{{
                      t("admin.groups.limitDay")
                    }}</span
                  >
                  <span
                    v-if="
                      row.daily_limit_usd &&
                      (row.weekly_limit_usd || row.monthly_limit_usd)
                    "
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.weekly_limit_usd"
                    >${{ row.weekly_limit_usd }}/{{
                      t("admin.groups.limitWeek")
                    }}</span
                  >
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.monthly_limit_usd"
                    >${{ row.monthly_limit_usd }}/{{
                      t("admin.groups.limitMonth")
                    }}</span
                  >
                </template>
                <span v-else class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.subscription.noLimit")
                }}</span>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300"
              >{{ value }}x</span
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
            <div class="space-y-0.5 text-xs">
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsAvailable")
                }}</span>
                <span
                  class="ml-1 font-medium text-emerald-600 dark:text-emerald-400"
                  >{{
                    (row.active_account_count || 0) -
                    (row.rate_limited_account_count || 0)
                  }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
              <div v-if="row.rate_limited_account_count">
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsRateLimited")
                }}</span>
                <span
                  class="ml-1 font-medium text-amber-600 dark:text-amber-400"
                  >{{ row.rate_limited_account_count }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
                  >{{ t("admin.groups.accountsUnit") }}</span
                >
              </div>
              <div>
                <span class="text-gray-500 dark:text-gray-400">{{
                  t("admin.groups.accountsTotal")
                }}</span>
                <span
                  class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >{{ row.account_count || 0 }}</span
                >
                <span
                  class="ml-1 inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
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
            <span v-else class="text-xs text-gray-400">—</span>
          </template>

          <template #cell-usage="{ row }">
            <div v-if="usageLoading" class="text-xs text-gray-400">—</div>
            <div v-else class="space-y-0.5 text-xs">
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.usageToday")
                }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300"
                  >${{
                    formatCost(usageMap.get(row.id)?.today_cost ?? 0)
                  }}</span
                >
              </div>
              <div class="text-gray-500 dark:text-gray-400">
                <span class="text-gray-400 dark:text-gray-500">{{
                  t("admin.groups.usageTotal")
                }}</span>
                <span class="ml-1 font-medium text-gray-700 dark:text-gray-300"
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
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t("common.edit") }}</span>
              </button>
              <button
                @click="handleRateMultipliers(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-purple-600 dark:hover:bg-dark-700 dark:hover:text-purple-400"
              >
                <Icon name="dollar" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rateMultipliers")
                }}</span>
              </button>
              <button
                @click="handleRPMOverrides(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-orange-600 dark:hover:bg-dark-700 dark:hover:text-orange-400"
              >
                <Icon name="bolt" size="sm" />
                <span class="text-xs">{{
                  t("admin.groups.rpmOverrides")
                }}</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
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
              @action="showCreateModal = true"
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

    <!-- Create Group Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.groups.createGroup')"
      width="normal"
      @close="closeCreateModal"
    >
      <form
        id="create-group-form"
        @submit.prevent="handleCreateGroup"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t("admin.groups.form.name") }}</label>
          <input
            v-model="createForm.name"
            type="text"
            required
            class="input"
            :placeholder="t('admin.groups.enterGroupName')"
            data-tour="group-form-name"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.description")
          }}</label>
          <textarea
            v-model="createForm.description"
            rows="3"
            class="input"
            :placeholder="t('admin.groups.optionalDescription')"
          ></textarea>
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.platform")
          }}</label>
          <Select
            v-model="createForm.platform"
            :options="platformOptions"
            data-tour="group-form-platform"
            @change="createForm.copy_accounts_from_group_ids = []"
          />
          <p class="input-hint">{{ t("admin.groups.platformHint") }}</p>
        </div>
        <!-- 从分组复制账号 -->
        <div v-if="copyAccountsGroupOptions.length > 0">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.copyAccounts.title") }}
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.copyAccounts.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 已选分组标签 -->
          <div
            v-if="createForm.copy_accounts_from_group_ids.length > 0"
            class="flex flex-wrap gap-1.5 mb-2"
          >
            <span
              v-for="groupId in createForm.copy_accounts_from_group_ids"
              :key="groupId"
              class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{
                copyAccountsGroupOptions.find((o) => o.value === groupId)
                  ?.label || `#${groupId}`
              }}
              <button
                type="button"
                @click="
                  createForm.copy_accounts_from_group_ids =
                    createForm.copy_accounts_from_group_ids.filter(
                      (id) => id !== groupId,
                    )
                "
                class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <!-- 分组选择下拉 -->
          <select
            class="input"
            @change="
              (e) => {
                const val = Number((e.target as HTMLSelectElement).value);
                if (
                  val &&
                  !createForm.copy_accounts_from_group_ids.includes(val)
                ) {
                  createForm.copy_accounts_from_group_ids.push(val);
                }
                (e.target as HTMLSelectElement).value = '';
              }
            "
          >
            <option value="">
              {{ t("admin.groups.copyAccounts.selectPlaceholder") }}
            </option>
            <option
              v-for="opt in copyAccountsGroupOptions"
              :key="opt.value"
              :value="opt.value"
              :disabled="
                createForm.copy_accounts_from_group_ids.includes(opt.value)
              "
            >
              {{ opt.label }}
            </option>
          </select>
          <p class="input-hint">{{ t("admin.groups.copyAccounts.hint") }}</p>
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.rateMultiplier")
          }}</label>
          <input
            v-model.number="createForm.rate_multiplier"
            type="number"
            step="0.001"
            min="0.001"
            required
            class="input"
            data-tour="group-form-multiplier"
          />
          <p class="input-hint">{{ t("admin.groups.rateMultiplierHint") }}</p>
        </div>
        <div>
          <label class="input-label">{{ t("admin.groups.form.rpmLimit") }}</label>
          <input
            v-model.number="createForm.rpm_limit"
            type="number"
            min="0"
            step="1"
            class="input"
            :placeholder="t('admin.groups.form.rpmLimitPlaceholder')"
          />
          <p class="input-hint">{{ t("admin.groups.form.rpmLimitHint") }}</p>
        </div>
        <div
          v-if="createForm.subscription_type !== 'subscription'"
          data-tour="group-form-exclusive"
        >
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.form.exclusive") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <!-- Tooltip Popover -->
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="mb-2 text-xs font-medium">
                    {{ t("admin.groups.exclusiveTooltip.title") }}
                  </p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.exclusiveTooltip.description") }}
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span
                        class="inline-flex items-center gap-1 text-primary-400"
                        ><Icon name="lightbulb" size="xs" />
                        {{ t("admin.groups.exclusiveTooltip.example") }}</span
                      >
                      {{ t("admin.groups.exclusiveTooltip.exampleContent") }}
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="createForm.is_exclusive = !createForm.is_exclusive"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.is_exclusive
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.is_exclusive ? 'translate-x-6' : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                createForm.is_exclusive
                  ? t("admin.groups.exclusive")
                  : t("admin.groups.public")
              }}
            </span>
          </div>
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.type")
            }}</label>
            <Select
              v-model="createForm.subscription_type"
              :options="subscriptionTypeOptions"
            />
            <p class="input-hint">
              {{ t("admin.groups.subscription.typeHint") }}
            </p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="createForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.dailyLimit")
              }}</label>
              <input
                v-model.number="createForm.daily_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.weeklyLimit")
              }}</label>
              <input
                v-model.number="createForm.weekly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.monthlyLimit")
              }}</label>
              <input
                v-model.number="createForm.monthly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
          </div>
        </div>

        <!-- 图片生成计费配置 -->
        <div
          v-if="
            createForm.platform === 'antigravity' ||
            createForm.platform === 'gemini' ||
            createForm.platform === 'openai'
          "
          class="border-t pt-4"
        >
          <label
            class="block mb-2 font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.groups.imagePricing.title") }}
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t("admin.groups.imagePricing.description") }}
          </p>
          <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="createForm.allow_image_generation"
                type="checkbox"
                class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {{ t("admin.groups.imagePricing.allowImageGeneration") }}
            </label>
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="createForm.image_rate_independent"
                type="checkbox"
                class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {{ t("admin.groups.imagePricing.independentMultiplier") }}
            </label>
          </div>
          <div
            v-if="createForm.image_rate_independent"
            class="mb-4"
          >
            <label class="input-label">{{
              t("admin.groups.imagePricing.imageMultiplier")
            }}</label>
            <input
              v-model.number="createForm.image_rate_multiplier"
              type="number"
              step="0.0001"
              min="0"
              class="input"
              placeholder="1"
            />
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div>
              <label class="input-label">1K ($)</label>
              <input
                v-model.number="createForm.image_price_1k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.134"
              />
            </div>
            <div>
              <label class="input-label">2K ($)</label>
              <input
                v-model.number="createForm.image_price_2k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.201"
              />
            </div>
            <div>
              <label class="input-label">4K ($)</label>
              <input
                v-model.number="createForm.image_price_4k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.268"
              />
            </div>
          </div>
          <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.groups.imagePricing.modeHint") }}
          </p>
          <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">
            <div class="mb-1 font-medium">
              {{ t("admin.groups.imagePricing.finalPricePreview") }}
            </div>
            <div class="grid grid-cols-3 gap-2">
              <div
                v-for="item in createImageFinalPricePreview"
                :key="item.label"
              >
                {{ item.label }}: {{ item.value }}
              </div>
            </div>
          </div>
        </div>

        <!-- 支持的模型系列（仅 antigravity 平台） -->
        <div v-if="createForm.platform === 'antigravity'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.supportedScopes.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.supportedScopes.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="space-y-2">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="createForm.supported_model_scopes.includes('claude')"
                @change="toggleCreateScope('claude')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.claude")
              }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="
                  createForm.supported_model_scopes.includes('gemini_text')
                "
                @change="toggleCreateScope('gemini_text')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.geminiText")
              }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="
                  createForm.supported_model_scopes.includes('gemini_image')
                "
                @change="toggleCreateScope('gemini_image')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.geminiImage")
              }}</span>
            </label>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.groups.supportedScopes.hint") }}
          </p>
        </div>

        <!-- MCP XML 协议注入（仅 antigravity 平台） -->
        <div v-if="createForm.platform === 'antigravity'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.mcpXml.title") }}
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.mcpXml.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="createForm.mcp_xml_inject = !createForm.mcp_xml_inject"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.mcp_xml_inject
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.mcp_xml_inject ? 'translate-x-6' : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                createForm.mcp_xml_inject
                  ? t("admin.groups.mcpXml.enabled")
                  : t("admin.groups.mcpXml.disabled")
              }}
            </span>
          </div>
        </div>

        <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
        <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.claudeCode.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.claudeCode.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="
                createForm.claude_code_only = !createForm.claude_code_only
              "
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.claude_code_only
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.claude_code_only
                    ? 'translate-x-6'
                    : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                createForm.claude_code_only
                  ? t("admin.groups.claudeCode.enabled")
                  : t("admin.groups.claudeCode.disabled")
              }}
            </span>
          </div>
          <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
          <div v-if="createForm.claude_code_only" class="mt-3">
            <label class="input-label">{{
              t("admin.groups.claudeCode.fallbackGroup")
            }}</label>
            <Select
              v-model="createForm.fallback_group_id"
              :options="fallbackGroupOptions"
              :placeholder="t('admin.groups.claudeCode.noFallback')"
            />
            <p class="input-hint">
              {{ t("admin.groups.claudeCode.fallbackHint") }}
            </p>
          </div>
        </div>

        <!-- OpenAI Messages 调度配置（仅 openai 平台） -->
        <div
          v-if="createForm.platform === 'openai'"
          class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
        >
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            {{ t("admin.groups.openaiMessages.title") }}
          </h4>

          <!-- 允许 Messages 调度开关 -->
          <div class="flex items-center justify-between">
            <label class="text-sm text-gray-600 dark:text-gray-400">{{
              t("admin.groups.openaiMessages.allowDispatch")
            }}</label>
            <button
              type="button"
              @click="
                createForm.allow_messages_dispatch =
                  !createForm.allow_messages_dispatch
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                createForm.allow_messages_dispatch
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  createForm.allow_messages_dispatch
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
            {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
          </p>

          <div v-if="createForm.allow_messages_dispatch" class="mt-3">
            <div
              class="relative overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-800"
            >
              <div
                class="border-b border-gray-100 bg-gray-50/80 px-4 py-3 dark:border-dark-700 dark:bg-dark-700/50"
              >
                <div class="flex items-center gap-2">
                  <div class="h-2 w-2 rounded-full bg-blue-500"></div>
                  <label
                    class="text-sm font-medium text-gray-900 dark:text-white"
                    >{{
                      t("admin.groups.openaiMessages.familyMappingTitle")
                    }}</label
                  >
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.groups.openaiMessages.familyMappingHint") }}
                </p>
              </div>
              <div class="p-4">
                <div class="grid gap-4 md:grid-cols-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.opusModel")
                    }}</label>
                    <input
                      v-model="createForm.opus_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.opusModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.sonnetModel")
                    }}</label>
                    <input
                      v-model="createForm.sonnet_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.sonnetModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.haikuModel")
                    }}</label>
                    <input
                      v-model="createForm.haiku_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.haikuModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                </div>
              </div>
            </div>

            <div
              class="mt-5 relative overflow-hidden rounded-xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/50 dark:bg-dark-800"
            >
              <div
                class="border-b border-primary-100 bg-primary-50/80 px-4 py-3 dark:border-primary-900/40 dark:bg-primary-900/20"
              >
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="flex items-center gap-2">
                      <div class="h-2 w-2 rounded-full bg-primary-500"></div>
                      <label
                        class="text-sm font-medium text-primary-900 dark:text-primary-100"
                        >{{
                          t("admin.groups.openaiMessages.exactMappingTitle")
                        }}</label
                      >
                    </div>
                    <p
                      class="mt-1 text-xs text-primary-600/90 dark:text-primary-400/90"
                    >
                      {{ t("admin.groups.openaiMessages.exactMappingHint") }}
                    </p>
                  </div>
                </div>
              </div>

              <div class="p-4 bg-gray-50/30 dark:bg-dark-800/30">
                <div
                  v-if="createForm.exact_model_mappings.length === 0"
                  class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-white px-5 py-4 text-sm text-primary-700 transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:bg-dark-800 dark:text-primary-300 dark:hover:border-primary-800"
                >
                  <span>{{
                    t("admin.groups.openaiMessages.noExactMappings")
                  }}</span>
                  <button
                    type="button"
                    @click="addCreateMessagesDispatchMapping"
                    class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t("admin.groups.openaiMessages.addExactMapping") }}
                  </button>
                </div>

                <div v-else class="space-y-3">
                  <div
                    v-for="row in createForm.exact_model_mappings"
                    :key="getCreateMessagesDispatchRowKey(row)"
                    class="group relative rounded-xl border border-gray-200 bg-white p-4 shadow-sm transition-all hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-700 dark:hover:border-primary-700"
                  >
                    <div class="flex items-center gap-4">
                      <div
                        class="grid flex-1 gap-4 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:items-start"
                      >
                        <div>
                          <label class="input-label">{{
                            t("admin.groups.openaiMessages.claudeModel")
                          }}</label>
                          <input
                            v-model="row.claude_model"
                            type="text"
                            :placeholder="
                              t(
                                'admin.groups.openaiMessages.claudeModelPlaceholder',
                              )
                            "
                            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                          />
                        </div>
                        <div
                          class="hidden md:flex md:justify-center md:pt-7 text-primary-300 dark:text-primary-700"
                        >
                          <Icon
                            name="arrowRight"
                            size="sm"
                            class="transition-transform group-hover:translate-x-1"
                          />
                        </div>
                        <div>
                          <label class="input-label">{{
                            t("admin.groups.openaiMessages.targetModel")
                          }}</label>
                          <input
                            v-model="row.target_model"
                            type="text"
                            :placeholder="
                              t(
                                'admin.groups.openaiMessages.targetModelPlaceholder',
                              )
                            "
                            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                          />
                        </div>
                      </div>
                      <button
                        type="button"
                        @click="removeCreateMessagesDispatchMapping(row)"
                        class="mt-6 flex h-9 w-9 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        :title="
                          t('admin.groups.openaiMessages.removeExactMapping')
                        "
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                  </div>

                  <button
                    type="button"
                    @click="addCreateMessagesDispatchMapping"
                    class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-gray-300 bg-white py-3 text-sm font-medium text-gray-500 transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t("admin.groups.openaiMessages.addExactMapping") }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 账号过滤控制 (OpenAI/Antigravity/Anthropic/Gemini) -->
        <div
          v-if="
            ['openai', 'antigravity', 'anthropic', 'gemini'].includes(
              createForm.platform,
            )
          "
          class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4 space-y-4"
        >
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            账号过滤控制
          </h4>

          <!-- require_oauth_only toggle -->
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm text-gray-600 dark:text-gray-400"
                >仅允许 OAuth 账号</label
              >
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{
                  createForm.require_oauth_only
                    ? "已启用 — 排除 API Key 类型账号"
                    : "未启用"
                }}
              </p>
            </div>
            <button
              type="button"
              @click="
                createForm.require_oauth_only = !createForm.require_oauth_only
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                createForm.require_oauth_only
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  createForm.require_oauth_only
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>

          <!-- require_privacy_set toggle -->
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm text-gray-600 dark:text-gray-400"
                >仅允许隐私保护已设置的账号</label
              >
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{
                  createForm.require_privacy_set
                    ? "已启用 — Privacy 未设置的账号将被排除"
                    : "未启用"
                }}
              </p>
            </div>
            <button
              type="button"
              @click="
                createForm.require_privacy_set = !createForm.require_privacy_set
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                createForm.require_privacy_set
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  createForm.require_privacy_set
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>
        </div>

        <!-- 无效请求兜底（仅 anthropic/antigravity 平台，且非订阅分组） -->
        <div
          v-if="
            ['anthropic', 'antigravity'].includes(createForm.platform) &&
            createForm.subscription_type !== 'subscription'
          "
          class="border-t pt-4"
        >
          <label class="input-label">{{
            t("admin.groups.invalidRequestFallback.title")
          }}</label>
          <Select
            v-model="createForm.fallback_group_id_on_invalid_request"
            :options="invalidRequestFallbackOptions"
            :placeholder="t('admin.groups.invalidRequestFallback.noFallback')"
          />
          <p class="input-hint">
            {{ t("admin.groups.invalidRequestFallback.hint") }}
          </p>
        </div>

        <!-- 模型路由配置（仅 anthropic 平台） -->
        <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.modelRouting.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.modelRouting.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 启用开关 -->
          <div class="flex items-center gap-3 mb-3">
            <button
              type="button"
              @click="
                createForm.model_routing_enabled =
                  !createForm.model_routing_enabled
              "
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.model_routing_enabled
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.model_routing_enabled
                    ? 'translate-x-6'
                    : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                createForm.model_routing_enabled
                  ? t("admin.groups.modelRouting.enabled")
                  : t("admin.groups.modelRouting.disabled")
              }}
            </span>
          </div>
          <p
            v-if="!createForm.model_routing_enabled"
            class="text-xs text-gray-500 dark:text-gray-400 mb-3"
          >
            {{ t("admin.groups.modelRouting.disabledHint") }}
          </p>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t("admin.groups.modelRouting.noRulesHint") }}
          </p>
          <!-- 路由规则列表（仅在启用时显示） -->
          <div v-if="createForm.model_routing_enabled" class="space-y-3">
            <div
              v-for="rule in createModelRoutingRules"
              :key="getCreateRuleRenderKey(rule)"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 space-y-2">
                  <div>
                    <label class="input-label text-xs">{{
                      t("admin.groups.modelRouting.modelPattern")
                    }}</label>
                    <input
                      v-model="rule.pattern"
                      type="text"
                      class="input text-sm"
                      :placeholder="
                        t('admin.groups.modelRouting.modelPatternPlaceholder')
                      "
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">{{
                      t("admin.groups.modelRouting.accounts")
                    }}</label>
                    <!-- 已选账号标签 -->
                    <div
                      v-if="rule.accounts.length > 0"
                      class="flex flex-wrap gap-1.5 mb-2"
                    >
                      <span
                        v-for="account in rule.accounts"
                        :key="account.id"
                        class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                      >
                        {{ account.name }}
                        <button
                          type="button"
                          @click="removeSelectedAccount(rule, account.id)"
                          class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                        >
                          <Icon name="x" size="xs" />
                        </button>
                      </span>
                    </div>
                    <!-- 账号搜索输入框 -->
                    <div class="relative account-search-container">
                      <input
                        v-model="
                          accountSearchKeyword[getCreateRuleSearchKey(rule)]
                        "
                        type="text"
                        class="input text-sm"
                        :placeholder="
                          t(
                            'admin.groups.modelRouting.searchAccountPlaceholder',
                          )
                        "
                        @input="searchAccountsByRule(rule)"
                        @focus="onAccountSearchFocus(rule)"
                      />
                      <!-- 搜索结果下拉框 -->
                      <div
                        v-if="
                          showAccountDropdown[getCreateRuleSearchKey(rule)] &&
                          accountSearchResults[getCreateRuleSearchKey(rule)]
                            ?.length > 0
                        "
                        class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                      >
                        <button
                          v-for="account in accountSearchResults[
                            getCreateRuleSearchKey(rule)
                          ]"
                          :key="account.id"
                          type="button"
                          @click="selectAccount(rule, account)"
                          class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                          :class="{
                            'opacity-50': rule.accounts.some(
                              (a) => a.id === account.id,
                            ),
                          }"
                          :disabled="
                            rule.accounts.some((a) => a.id === account.id)
                          "
                        >
                          <span>{{ account.name }}</span>
                          <span class="ml-2 text-xs text-gray-400"
                            >#{{ account.id }}</span
                          >
                        </button>
                      </div>
                    </div>
                    <p class="text-xs text-gray-400 mt-1">
                      {{ t("admin.groups.modelRouting.accountsHint") }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  @click="removeCreateRoutingRule(rule)"
                  class="mt-5 p-1.5 text-gray-400 hover:text-red-500 transition-colors"
                  :title="t('admin.groups.modelRouting.removeRule')"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
          </div>
          <!-- 添加规则按钮（仅在启用时显示） -->
          <button
            v-if="createForm.model_routing_enabled"
            type="button"
            @click="addCreateRoutingRule"
            class="mt-3 flex items-center gap-1.5 text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <Icon name="plus" size="sm" />
            {{ t("admin.groups.modelRouting.addRule") }}
          </button>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button
            @click="closeCreateModal"
            type="button"
            class="btn btn-secondary"
          >
            {{ t("common.cancel") }}
          </button>
          <button
            type="submit"
            form="create-group-form"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="group-form-submit"
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
            {{ submitting ? t("admin.groups.creating") : t("common.create") }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit Group Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.groups.editGroup')"
      width="normal"
      @close="closeEditModal"
    >
      <form
        v-if="editingGroup"
        id="edit-group-form"
        @submit.prevent="handleUpdateGroup"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t("admin.groups.form.name") }}</label>
          <input
            v-model="editForm.name"
            type="text"
            required
            class="input"
            data-tour="edit-group-form-name"
          />
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.description")
          }}</label>
          <textarea
            v-model="editForm.description"
            rows="3"
            class="input"
          ></textarea>
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.platform")
          }}</label>
          <Select
            v-model="editForm.platform"
            :options="platformOptions"
            :disabled="true"
            data-tour="group-form-platform"
          />
          <p class="input-hint">{{ t("admin.groups.platformNotEditable") }}</p>
        </div>
        <!-- 从分组复制账号（编辑时） -->
        <div v-if="copyAccountsGroupOptionsForEdit.length > 0">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.copyAccounts.title") }}
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.copyAccounts.tooltipEdit") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 已选分组标签 -->
          <div
            v-if="editForm.copy_accounts_from_group_ids.length > 0"
            class="flex flex-wrap gap-1.5 mb-2"
          >
            <span
              v-for="groupId in editForm.copy_accounts_from_group_ids"
              :key="groupId"
              class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{
                copyAccountsGroupOptionsForEdit.find((o) => o.value === groupId)
                  ?.label || `#${groupId}`
              }}
              <button
                type="button"
                @click="
                  editForm.copy_accounts_from_group_ids =
                    editForm.copy_accounts_from_group_ids.filter(
                      (id) => id !== groupId,
                    )
                "
                class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <!-- 分组选择下拉 -->
          <select
            class="input"
            @change="
              (e) => {
                const val = Number((e.target as HTMLSelectElement).value);
                if (
                  val &&
                  !editForm.copy_accounts_from_group_ids.includes(val)
                ) {
                  editForm.copy_accounts_from_group_ids.push(val);
                }
                (e.target as HTMLSelectElement).value = '';
              }
            "
          >
            <option value="">
              {{ t("admin.groups.copyAccounts.selectPlaceholder") }}
            </option>
            <option
              v-for="opt in copyAccountsGroupOptionsForEdit"
              :key="opt.value"
              :value="opt.value"
              :disabled="
                editForm.copy_accounts_from_group_ids.includes(opt.value)
              "
            >
              {{ opt.label }}
            </option>
          </select>
          <p class="input-hint">
            {{ t("admin.groups.copyAccounts.hintEdit") }}
          </p>
        </div>
        <div>
          <label class="input-label">{{
            t("admin.groups.form.rateMultiplier")
          }}</label>
          <input
            v-model.number="editForm.rate_multiplier"
            type="number"
            step="0.001"
            min="0.001"
            required
            class="input"
            data-tour="group-form-multiplier"
          />
        </div>
        <div>
          <label class="input-label">{{ t("admin.groups.form.rpmLimit") }}</label>
          <input
            v-model.number="editForm.rpm_limit"
            type="number"
            min="0"
            step="1"
            class="input"
            :placeholder="t('admin.groups.form.rpmLimitPlaceholder')"
          />
          <p class="input-hint">{{ t("admin.groups.form.rpmLimitHint") }}</p>
        </div>
        <div v-if="editForm.subscription_type !== 'subscription'">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.form.exclusive") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <!-- Tooltip Popover -->
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="mb-2 text-xs font-medium">
                    {{ t("admin.groups.exclusiveTooltip.title") }}
                  </p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.exclusiveTooltip.description") }}
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span
                        class="inline-flex items-center gap-1 text-primary-400"
                        ><Icon name="lightbulb" size="xs" />
                        {{ t("admin.groups.exclusiveTooltip.example") }}</span
                      >
                      {{ t("admin.groups.exclusiveTooltip.exampleContent") }}
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="editForm.is_exclusive = !editForm.is_exclusive"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.is_exclusive
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.is_exclusive ? 'translate-x-6' : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                editForm.is_exclusive
                  ? t("admin.groups.exclusive")
                  : t("admin.groups.public")
              }}
            </span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t("admin.groups.form.status") }}</label>
          <Select v-model="editForm.status" :options="editStatusOptions" />
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{
              t("admin.groups.subscription.type")
            }}</label>
            <Select
              v-model="editForm.subscription_type"
              :options="subscriptionTypeOptions"
              :disabled="true"
            />
            <p class="input-hint">
              {{ t("admin.groups.subscription.typeNotEditable") }}
            </p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="editForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.dailyLimit")
              }}</label>
              <input
                v-model.number="editForm.daily_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.weeklyLimit")
              }}</label>
              <input
                v-model.number="editForm.weekly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
            <div>
              <label class="input-label">{{
                t("admin.groups.subscription.monthlyLimit")
              }}</label>
              <input
                v-model.number="editForm.monthly_limit_usd"
                type="number"
                step="0.01"
                min="0"
                class="input"
                :placeholder="t('admin.groups.subscription.noLimit')"
              />
            </div>
          </div>
        </div>

        <!-- 图片生成计费配置 -->
        <div
          v-if="
            editForm.platform === 'antigravity' ||
            editForm.platform === 'gemini' ||
            editForm.platform === 'openai'
          "
          class="border-t pt-4"
        >
          <label
            class="block mb-2 font-medium text-gray-700 dark:text-gray-300"
          >
            {{ t("admin.groups.imagePricing.title") }}
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t("admin.groups.imagePricing.description") }}
          </p>
          <div class="mb-4 grid grid-cols-1 gap-3 md:grid-cols-2">
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="editForm.allow_image_generation"
                type="checkbox"
                class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {{ t("admin.groups.imagePricing.allowImageGeneration") }}
            </label>
            <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
              <input
                v-model="editForm.image_rate_independent"
                type="checkbox"
                class="rounded border-gray-300 text-blue-600 focus:ring-blue-500"
              />
              {{ t("admin.groups.imagePricing.independentMultiplier") }}
            </label>
          </div>
          <div
            v-if="editForm.image_rate_independent"
            class="mb-4"
          >
            <label class="input-label">{{
              t("admin.groups.imagePricing.imageMultiplier")
            }}</label>
            <input
              v-model.number="editForm.image_rate_multiplier"
              type="number"
              step="0.0001"
              min="0"
              class="input"
              placeholder="1"
            />
          </div>
          <div class="grid grid-cols-3 gap-3">
            <div>
              <label class="input-label">1K ($)</label>
              <input
                v-model.number="editForm.image_price_1k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.134"
              />
            </div>
            <div>
              <label class="input-label">2K ($)</label>
              <input
                v-model.number="editForm.image_price_2k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.201"
              />
            </div>
            <div>
              <label class="input-label">4K ($)</label>
              <input
                v-model.number="editForm.image_price_4k"
                type="number"
                step="0.001"
                min="0"
                class="input"
                placeholder="0.268"
              />
            </div>
          </div>
          <p class="mt-3 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.groups.imagePricing.modeHint") }}
          </p>
          <div class="mt-2 rounded-lg bg-gray-50 p-3 text-xs text-gray-700 dark:bg-gray-800 dark:text-gray-300">
            <div class="mb-1 font-medium">
              {{ t("admin.groups.imagePricing.finalPricePreview") }}
            </div>
            <div class="grid grid-cols-3 gap-2">
              <div
                v-for="item in editImageFinalPricePreview"
                :key="item.label"
              >
                {{ item.label }}: {{ item.value }}
              </div>
            </div>
          </div>
        </div>

        <!-- 支持的模型系列（仅 antigravity 平台） -->
        <div v-if="editForm.platform === 'antigravity'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.supportedScopes.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.supportedScopes.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="space-y-2">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="editForm.supported_model_scopes.includes('claude')"
                @change="toggleEditScope('claude')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.claude")
              }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="
                  editForm.supported_model_scopes.includes('gemini_text')
                "
                @change="toggleEditScope('gemini_text')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.geminiText")
              }}</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                :checked="
                  editForm.supported_model_scopes.includes('gemini_image')
                "
                @change="toggleEditScope('gemini_image')"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
              />
              <span class="text-sm text-gray-700 dark:text-gray-300">{{
                t("admin.groups.supportedScopes.geminiImage")
              }}</span>
            </label>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.groups.supportedScopes.hint") }}
          </p>
        </div>

        <!-- MCP XML 协议注入（仅 antigravity 平台） -->
        <div v-if="editForm.platform === 'antigravity'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.mcpXml.title") }}
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.mcpXml.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="editForm.mcp_xml_inject = !editForm.mcp_xml_inject"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.mcp_xml_inject
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.mcp_xml_inject ? 'translate-x-6' : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                editForm.mcp_xml_inject
                  ? t("admin.groups.mcpXml.enabled")
                  : t("admin.groups.mcpXml.disabled")
              }}
            </span>
          </div>
        </div>

        <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
        <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.claudeCode.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.claudeCode.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="editForm.claude_code_only = !editForm.claude_code_only"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.claude_code_only
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.claude_code_only ? 'translate-x-6' : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                editForm.claude_code_only
                  ? t("admin.groups.claudeCode.enabled")
                  : t("admin.groups.claudeCode.disabled")
              }}
            </span>
          </div>
          <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
          <div v-if="editForm.claude_code_only" class="mt-3">
            <label class="input-label">{{
              t("admin.groups.claudeCode.fallbackGroup")
            }}</label>
            <Select
              v-model="editForm.fallback_group_id"
              :options="fallbackGroupOptionsForEdit"
              :placeholder="t('admin.groups.claudeCode.noFallback')"
            />
            <p class="input-hint">
              {{ t("admin.groups.claudeCode.fallbackHint") }}
            </p>
          </div>
        </div>

        <!-- OpenAI Messages 调度配置（仅 openai 平台） -->
        <div
          v-if="editForm.platform === 'openai'"
          class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4"
        >
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            {{ t("admin.groups.openaiMessages.title") }}
          </h4>

          <!-- 允许 Messages 调度开关 -->
          <div class="flex items-center justify-between">
            <label class="text-sm text-gray-600 dark:text-gray-400">{{
              t("admin.groups.openaiMessages.allowDispatch")
            }}</label>
            <button
              type="button"
              @click="
                editForm.allow_messages_dispatch =
                  !editForm.allow_messages_dispatch
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                editForm.allow_messages_dispatch
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  editForm.allow_messages_dispatch
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>
          <p class="text-xs text-gray-500 dark:text-gray-400 mt-1">
            {{ t("admin.groups.openaiMessages.allowDispatchHint") }}
          </p>

          <div v-if="editForm.allow_messages_dispatch" class="mt-3">
            <div
              class="relative overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-600 dark:bg-dark-800"
            >
              <div
                class="border-b border-gray-100 bg-gray-50/80 px-4 py-3 dark:border-dark-700 dark:bg-dark-700/50"
              >
                <div class="flex items-center gap-2">
                  <div class="h-2 w-2 rounded-full bg-blue-500"></div>
                  <label
                    class="text-sm font-medium text-gray-900 dark:text-white"
                    >{{
                      t("admin.groups.openaiMessages.familyMappingTitle")
                    }}</label
                  >
                </div>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  {{ t("admin.groups.openaiMessages.familyMappingHint") }}
                </p>
              </div>
              <div class="p-4">
                <div class="grid gap-4 md:grid-cols-3">
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.opusModel")
                    }}</label>
                    <input
                      v-model="editForm.opus_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.opusModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.sonnetModel")
                    }}</label>
                    <input
                      v-model="editForm.sonnet_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.sonnetModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                  <div>
                    <label class="input-label">{{
                      t("admin.groups.openaiMessages.haikuModel")
                    }}</label>
                    <input
                      v-model="editForm.haiku_mapped_model"
                      type="text"
                      :placeholder="
                        t('admin.groups.openaiMessages.haikuModelPlaceholder')
                      "
                      class="input"
                    />
                  </div>
                </div>
              </div>
            </div>

            <div
              class="mt-5 relative overflow-hidden rounded-xl border border-primary-200 bg-white shadow-sm dark:border-primary-900/50 dark:bg-dark-800"
            >
              <div
                class="border-b border-primary-100 bg-primary-50/80 px-4 py-3 dark:border-primary-900/40 dark:bg-primary-900/20"
              >
                <div class="flex items-start justify-between gap-3">
                  <div>
                    <div class="flex items-center gap-2">
                      <div class="h-2 w-2 rounded-full bg-primary-500"></div>
                      <label
                        class="text-sm font-medium text-primary-900 dark:text-primary-100"
                        >{{
                          t("admin.groups.openaiMessages.exactMappingTitle")
                        }}</label
                      >
                    </div>
                    <p
                      class="mt-1 text-xs text-primary-600/90 dark:text-primary-400/90"
                    >
                      {{ t("admin.groups.openaiMessages.exactMappingHint") }}
                    </p>
                  </div>
                </div>
              </div>

              <div class="p-4 bg-gray-50/30 dark:bg-dark-800/30">
                <div
                  v-if="editForm.exact_model_mappings.length === 0"
                  class="flex items-center justify-between gap-3 rounded-xl border-2 border-dashed border-primary-200 bg-white px-5 py-4 text-sm text-primary-700 transition-colors hover:border-primary-300 dark:border-primary-900/40 dark:bg-dark-800 dark:text-primary-300 dark:hover:border-primary-800"
                >
                  <span>{{
                    t("admin.groups.openaiMessages.noExactMappings")
                  }}</span>
                  <button
                    type="button"
                    @click="addEditMessagesDispatchMapping"
                    class="flex items-center gap-1.5 text-sm font-medium text-primary-600 transition-colors hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t("admin.groups.openaiMessages.addExactMapping") }}
                  </button>
                </div>

                <div v-else class="space-y-3">
                  <div
                    v-for="row in editForm.exact_model_mappings"
                    :key="getEditMessagesDispatchRowKey(row)"
                    class="group relative rounded-xl border border-gray-200 bg-white p-4 shadow-sm transition-all hover:border-primary-300 hover:shadow-md dark:border-dark-600 dark:bg-dark-700 dark:hover:border-primary-700"
                  >
                    <div class="flex items-center gap-4">
                      <div
                        class="grid flex-1 gap-4 md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] md:items-start"
                      >
                        <div>
                          <label class="input-label">{{
                            t("admin.groups.openaiMessages.claudeModel")
                          }}</label>
                          <input
                            v-model="row.claude_model"
                            type="text"
                            :placeholder="
                              t(
                                'admin.groups.openaiMessages.claudeModelPlaceholder',
                              )
                            "
                            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                          />
                        </div>
                        <div
                          class="hidden md:flex md:justify-center md:pt-7 text-primary-300 dark:text-primary-700"
                        >
                          <Icon
                            name="arrowRight"
                            size="sm"
                            class="transition-transform group-hover:translate-x-1"
                          />
                        </div>
                        <div>
                          <label class="input-label">{{
                            t("admin.groups.openaiMessages.targetModel")
                          }}</label>
                          <input
                            v-model="row.target_model"
                            type="text"
                            :placeholder="
                              t(
                                'admin.groups.openaiMessages.targetModelPlaceholder',
                              )
                            "
                            class="input bg-gray-50 focus:bg-white dark:bg-dark-800 dark:focus:bg-dark-900"
                          />
                        </div>
                      </div>
                      <button
                        type="button"
                        @click="removeEditMessagesDispatchMapping(row)"
                        class="mt-6 flex h-9 w-9 items-center justify-center rounded-lg text-gray-400 transition-colors hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                        :title="
                          t('admin.groups.openaiMessages.removeExactMapping')
                        "
                      >
                        <Icon name="trash" size="sm" />
                      </button>
                    </div>
                  </div>

                  <button
                    type="button"
                    @click="addEditMessagesDispatchMapping"
                    class="flex w-full items-center justify-center gap-2 rounded-xl border-2 border-dashed border-gray-300 bg-white py-3 text-sm font-medium text-gray-500 transition-all hover:border-primary-300 hover:bg-primary-50/50 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400 dark:hover:border-primary-800 dark:hover:bg-primary-900/20 dark:hover:text-primary-400"
                  >
                    <Icon name="plus" size="sm" />
                    {{ t("admin.groups.openaiMessages.addExactMapping") }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- 账号过滤控制 (OpenAI/Antigravity/Anthropic/Gemini) -->
        <div
          v-if="
            ['openai', 'antigravity', 'anthropic', 'gemini'].includes(
              editForm.platform,
            )
          "
          class="border-t border-gray-200 dark:border-dark-400 pt-4 mt-4 space-y-4"
        >
          <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300 mb-3">
            账号过滤控制
          </h4>

          <!-- require_oauth_only toggle -->
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm text-gray-600 dark:text-gray-400"
                >仅允许 OAuth 账号</label
              >
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{
                  editForm.require_oauth_only
                    ? "已启用 — 排除 API Key 类型账号"
                    : "未启用"
                }}
              </p>
            </div>
            <button
              type="button"
              @click="
                editForm.require_oauth_only = !editForm.require_oauth_only
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                editForm.require_oauth_only
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  editForm.require_oauth_only
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>

          <!-- require_privacy_set toggle -->
          <div class="flex items-center justify-between">
            <div>
              <label class="text-sm text-gray-600 dark:text-gray-400"
                >仅允许隐私保护已设置的账号</label
              >
              <p class="text-xs text-gray-500 dark:text-gray-400 mt-0.5">
                {{
                  editForm.require_privacy_set
                    ? "已启用 — Privacy 未设置的账号将被排除"
                    : "未启用"
                }}
              </p>
            </div>
            <button
              type="button"
              @click="
                editForm.require_privacy_set = !editForm.require_privacy_set
              "
              class="relative inline-flex h-6 w-12 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none"
              :class="
                editForm.require_privacy_set
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600'
              "
            >
              <span
                class="pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out"
                :class="
                  editForm.require_privacy_set
                    ? 'translate-x-6'
                    : 'translate-x-1'
                "
              />
            </button>
          </div>
        </div>

        <!-- 无效请求兜底（仅 anthropic/antigravity 平台，且非订阅分组） -->
        <div
          v-if="
            ['anthropic', 'antigravity'].includes(editForm.platform) &&
            editForm.subscription_type !== 'subscription'
          "
          class="border-t pt-4"
        >
          <label class="input-label">{{
            t("admin.groups.invalidRequestFallback.title")
          }}</label>
          <Select
            v-model="editForm.fallback_group_id_on_invalid_request"
            :options="invalidRequestFallbackOptionsForEdit"
            :placeholder="t('admin.groups.invalidRequestFallback.noFallback')"
          />
          <p class="input-hint">
            {{ t("admin.groups.invalidRequestFallback.hint") }}
          </p>
        </div>

        <!-- 模型路由配置（仅 anthropic 平台） -->
        <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t("admin.groups.modelRouting.title") }}
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div
                class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100"
              >
                <div
                  class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800"
                >
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t("admin.groups.modelRouting.tooltip") }}
                  </p>
                  <div
                    class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"
                  ></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 启用开关 -->
          <div class="flex items-center gap-3 mb-3">
            <button
              type="button"
              @click="
                editForm.model_routing_enabled = !editForm.model_routing_enabled
              "
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.model_routing_enabled
                  ? 'bg-primary-500'
                  : 'bg-gray-300 dark:bg-dark-600',
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.model_routing_enabled
                    ? 'translate-x-6'
                    : 'translate-x-1',
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{
                editForm.model_routing_enabled
                  ? t("admin.groups.modelRouting.enabled")
                  : t("admin.groups.modelRouting.disabled")
              }}
            </span>
          </div>
          <p
            v-if="!editForm.model_routing_enabled"
            class="text-xs text-gray-500 dark:text-gray-400 mb-3"
          >
            {{ t("admin.groups.modelRouting.disabledHint") }}
          </p>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t("admin.groups.modelRouting.noRulesHint") }}
          </p>
          <!-- 路由规则列表（仅在启用时显示） -->
          <div v-if="editForm.model_routing_enabled" class="space-y-3">
            <div
              v-for="rule in editModelRoutingRules"
              :key="getEditRuleRenderKey(rule)"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 space-y-2">
                  <div>
                    <label class="input-label text-xs">{{
                      t("admin.groups.modelRouting.modelPattern")
                    }}</label>
                    <input
                      v-model="rule.pattern"
                      type="text"
                      class="input text-sm"
                      :placeholder="
                        t('admin.groups.modelRouting.modelPatternPlaceholder')
                      "
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">{{
                      t("admin.groups.modelRouting.accounts")
                    }}</label>
                    <!-- 已选账号标签 -->
                    <div
                      v-if="rule.accounts.length > 0"
                      class="flex flex-wrap gap-1.5 mb-2"
                    >
                      <span
                        v-for="account in rule.accounts"
                        :key="account.id"
                        class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                      >
                        {{ account.name }}
                        <button
                          type="button"
                          @click="removeSelectedAccount(rule, account.id, true)"
                          class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                        >
                          <Icon name="x" size="xs" />
                        </button>
                      </span>
                    </div>
                    <!-- 账号搜索输入框 -->
                    <div class="relative account-search-container">
                      <input
                        v-model="
                          accountSearchKeyword[getEditRuleSearchKey(rule)]
                        "
                        type="text"
                        class="input text-sm"
                        :placeholder="
                          t(
                            'admin.groups.modelRouting.searchAccountPlaceholder',
                          )
                        "
                        @input="searchAccountsByRule(rule, true)"
                        @focus="onAccountSearchFocus(rule, true)"
                      />
                      <!-- 搜索结果下拉框 -->
                      <div
                        v-if="
                          showAccountDropdown[getEditRuleSearchKey(rule)] &&
                          accountSearchResults[getEditRuleSearchKey(rule)]
                            ?.length > 0
                        "
                        class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                      >
                        <button
                          v-for="account in accountSearchResults[
                            getEditRuleSearchKey(rule)
                          ]"
                          :key="account.id"
                          type="button"
                          @click="selectAccount(rule, account, true)"
                          class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                          :class="{
                            'opacity-50': rule.accounts.some(
                              (a) => a.id === account.id,
                            ),
                          }"
                          :disabled="
                            rule.accounts.some((a) => a.id === account.id)
                          "
                        >
                          <span>{{ account.name }}</span>
                          <span class="ml-2 text-xs text-gray-400"
                            >#{{ account.id }}</span
                          >
                        </button>
                      </div>
                    </div>
                    <p class="text-xs text-gray-400 mt-1">
                      {{ t("admin.groups.modelRouting.accountsHint") }}
                    </p>
                  </div>
                </div>
                <button
                  type="button"
                  @click="removeEditRoutingRule(rule)"
                  class="mt-5 p-1.5 text-gray-400 hover:text-red-500 transition-colors"
                  :title="t('admin.groups.modelRouting.removeRule')"
                >
                  <Icon name="trash" size="sm" />
                </button>
              </div>
            </div>
          </div>
          <!-- 添加规则按钮（仅在启用时显示） -->
          <button
            v-if="editForm.model_routing_enabled"
            type="button"
            @click="addEditRoutingRule"
            class="mt-3 flex items-center gap-1.5 text-sm text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300"
          >
            <Icon name="plus" size="sm" />
            {{ t("admin.groups.modelRouting.addRule") }}
          </button>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button
            @click="closeEditModal"
            type="button"
            class="btn btn-secondary"
          >
            {{ t("common.cancel") }}
          </button>
          <button
            type="submit"
            form="edit-group-form"
            :disabled="submitting"
            class="btn btn-primary"
            data-tour="group-form-submit"
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
            {{ submitting ? t("admin.groups.updating") : t("common.update") }}
          </button>
        </div>
      </template>
    </BaseDialog>

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

    <!-- Sort Order Modal -->
    <BaseDialog
      :show="showSortModal"
      :title="t('admin.groups.sortOrder')"
      width="normal"
      @close="closeSortModal"
    >
      <div class="space-y-4">
        <p class="text-sm text-gray-500 dark:text-gray-400">
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
            class="flex cursor-grab items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 transition-shadow hover:shadow-md active:cursor-grabbing dark:border-dark-600 dark:bg-dark-700"
          >
            <div class="text-gray-400">
              <Icon name="menu" size="md" />
            </div>
            <div class="flex-1">
              <div class="font-medium text-gray-900 dark:text-white">
                {{ group.name }}
              </div>
              <div class="text-xs text-gray-500 dark:text-gray-400">
                <span
                  :class="[
                    'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
                    group.platform === 'anthropic'
                      ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                      : group.platform === 'openai'
                        ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                        : group.platform === 'antigravity'
                          ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                          : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400',
                  ]"
                >
                  {{ t("admin.groups.platforms." + group.platform) }}
                </span>
              </div>
            </div>
            <div class="text-sm text-gray-400">#{{ group.id }}</div>
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
import { ref, reactive, computed, onMounted, onUnmounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { useAppStore } from "@/stores/app";
import { useOnboardingStore } from "@/stores/onboarding";
import { adminAPI } from "@/api/admin";
import type { AdminGroup, GroupPlatform, SubscriptionType } from "@/types";
import type { Column } from "@/components/common/types";
import AppLayout from "@/components/layout/AppLayout.vue";
import TablePageLayout from "@/components/layout/TablePageLayout.vue";
import DataTable from "@/components/common/DataTable.vue";
import Pagination from "@/components/common/Pagination.vue";
import BaseDialog from "@/components/common/BaseDialog.vue";
import ConfirmDialog from "@/components/common/ConfirmDialog.vue";
import EmptyState from "@/components/common/EmptyState.vue";
import Select from "@/components/common/Select.vue";
import PlatformIcon from "@/components/common/PlatformIcon.vue";
import Icon from "@/components/icons/Icon.vue";
import GroupRateMultipliersModal from "@/components/admin/group/GroupRateMultipliersModal.vue";
import GroupRPMOverridesModal from "@/components/admin/group/GroupRPMOverridesModal.vue";
import GroupCapacityBadge from "@/components/common/GroupCapacityBadge.vue";
import { VueDraggable } from "vue-draggable-plus";
import { createStableObjectKeyResolver } from "@/utils/stableObjectKey";
import { useKeyedDebouncedSearch } from "@/composables/useKeyedDebouncedSearch";
import { getPersistedPageSize } from "@/composables/usePersistedPageSize";
import {
  createDefaultMessagesDispatchFormState,
  messagesDispatchConfigToFormState,
  messagesDispatchFormStateToConfig,
  resetMessagesDispatchFormState,
  type MessagesDispatchMappingRow,
} from "./groupsMessagesDispatch";

const { t } = useI18n();
const appStore = useAppStore();
const onboardingStore = useOnboardingStore();

const columns = computed<Column[]>(() => [
  { key: "name", label: t("admin.groups.columns.name"), sortable: true },
  {
    key: "platform",
    label: t("admin.groups.columns.platform"),
    sortable: true,
  },
  {
    key: "billing_type",
    label: t("admin.groups.columns.billingType"),
    sortable: true,
  },
  {
    key: "rate_multiplier",
    label: t("admin.groups.columns.rateMultiplier"),
    sortable: true,
  },
  {
    key: "is_exclusive",
    label: t("admin.groups.columns.type"),
    sortable: true,
  },
  {
    key: "account_count",
    label: t("admin.groups.columns.accounts"),
    sortable: true,
  },
  {
    key: "capacity",
    label: t("admin.groups.columns.capacity"),
    sortable: false,
  },
  { key: "usage", label: t("admin.groups.columns.usage"), sortable: false },
  { key: "status", label: t("admin.groups.columns.status"), sortable: true },
  { key: "actions", label: t("admin.groups.columns.actions"), sortable: false },
]);

// Filter options
const statusOptions = computed(() => [
  { value: "", label: t("admin.groups.allStatus") },
  { value: "active", label: t("admin.accounts.status.active") },
  { value: "inactive", label: t("admin.accounts.status.inactive") },
]);

const exclusiveOptions = computed(() => [
  { value: "", label: t("admin.groups.allGroups") },
  { value: "true", label: t("admin.groups.exclusive") },
  { value: "false", label: t("admin.groups.nonExclusive") },
]);

const platformOptions = computed(() => [
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "antigravity", label: "Antigravity" },
]);

const platformFilterOptions = computed(() => [
  { value: "", label: t("admin.groups.allPlatforms") },
  { value: "anthropic", label: "Anthropic" },
  { value: "openai", label: "OpenAI" },
  { value: "gemini", label: "Gemini" },
  { value: "antigravity", label: "Antigravity" },
]);

const editStatusOptions = computed(() => [
  { value: "active", label: t("admin.accounts.status.active") },
  { value: "inactive", label: t("admin.accounts.status.inactive") },
]);

const subscriptionTypeOptions = computed(() => [
  { value: "standard", label: t("admin.groups.subscription.standard") },
  { value: "subscription", label: t("admin.groups.subscription.subscription") },
]);

// 降级分组选项（创建时）- 仅包含 anthropic 平台且未启用 claude_code_only 的分组
const fallbackGroupOptions = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.claudeCode.noFallback") },
  ];
  const eligibleGroups = groups.value.filter(
    (g) =>
      g.platform === "anthropic" &&
      !g.claude_code_only &&
      g.status === "active",
  );
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name });
  });
  return options;
});

// 降级分组选项（编辑时）- 排除自身
const fallbackGroupOptionsForEdit = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.claudeCode.noFallback") },
  ];
  const currentId = editingGroup.value?.id;
  const eligibleGroups = groups.value.filter(
    (g) =>
      g.platform === "anthropic" &&
      !g.claude_code_only &&
      g.status === "active" &&
      g.id !== currentId,
  );
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name });
  });
  return options;
});

// 无效请求兜底分组选项（创建时）- 仅包含 anthropic 平台、非订阅且未配置兜底的分组
const invalidRequestFallbackOptions = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.invalidRequestFallback.noFallback") },
  ];
  const eligibleGroups = groups.value.filter(
    (g) =>
      g.platform === "anthropic" &&
      g.status === "active" &&
      g.subscription_type !== "subscription" &&
      g.fallback_group_id_on_invalid_request === null,
  );
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name });
  });
  return options;
});

// 无效请求兜底分组选项（编辑时）- 排除自身
const invalidRequestFallbackOptionsForEdit = computed(() => {
  const options: { value: number | null; label: string }[] = [
    { value: null, label: t("admin.groups.invalidRequestFallback.noFallback") },
  ];
  const currentId = editingGroup.value?.id;
  const eligibleGroups = groups.value.filter(
    (g) =>
      g.platform === "anthropic" &&
      g.status === "active" &&
      g.subscription_type !== "subscription" &&
      g.fallback_group_id_on_invalid_request === null &&
      g.id !== currentId,
  );
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name });
  });
  return options;
});

// 复制账号的源分组选项（创建时）- 仅包含相同平台且有账号的分组
const copyAccountsGroupOptions = computed(() => {
  const eligibleGroups = groups.value.filter(
    (g) => g.platform === createForm.platform && (g.account_count || 0) > 0,
  );
  return eligibleGroups.map((g) => ({
    value: g.id,
    label: `${g.name} (${g.account_count || 0} 个账号)`,
  }));
});

// 复制账号的源分组选项（编辑时）- 仅包含相同平台且有账号的分组，排除自身
const copyAccountsGroupOptionsForEdit = computed(() => {
  const currentId = editingGroup.value?.id;
  const eligibleGroups = groups.value.filter(
    (g) =>
      g.platform === editForm.platform &&
      (g.account_count || 0) > 0 &&
      g.id !== currentId,
  );
  return eligibleGroups.map((g) => ({
    value: g.id,
    label: `${g.name} (${g.account_count || 0} 个账号)`,
  }));
});

const groups = ref<AdminGroup[]>([]);
const loading = ref(false);
const usageMap = ref<Map<number, { today_cost: number; total_cost: number }>>(
  new Map(),
);
const usageLoading = ref(false);
const capacityMap = ref<
  Map<
    number,
    {
      concurrencyUsed: number;
      concurrencyMax: number;
      sessionsUsed: number;
      sessionsMax: number;
      rpmUsed: number;
      rpmMax: number;
    }
  >
>(new Map());
const searchQuery = ref("");
const filters = reactive({
  platform: "",
  status: "",
  is_exclusive: "",
});
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0,
});
const sortState = reactive({
  sort_by: "sort_order",
  sort_order: "asc" as "asc" | "desc",
});

let abortController: AbortController | null = null;

const showCreateModal = ref(false);
const showEditModal = ref(false);
const showDeleteDialog = ref(false);
const showSortModal = ref(false);
const submitting = ref(false);
const sortSubmitting = ref(false);
const editingGroup = ref<AdminGroup | null>(null);
const deletingGroup = ref<AdminGroup | null>(null);
const showRateMultipliersModal = ref(false);
const rateMultipliersGroup = ref<AdminGroup | null>(null);
const showRPMOverridesModal = ref(false);
const rpmOverridesGroup = ref<AdminGroup | null>(null);
const sortableGroups = ref<AdminGroup[]>([]);
const createMessagesDispatchDefaults = createDefaultMessagesDispatchFormState();
const editMessagesDispatchDefaults = createDefaultMessagesDispatchFormState();

const createForm = reactive({
  name: "",
  description: "",
  platform: "anthropic" as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  subscription_type: "standard" as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null,
  // 图片生成计费配置
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null as number | null,
  image_price_2k: null as number | null,
  image_price_4k: null as number | null,
  // Claude Code 客户端限制（仅 anthropic 平台使用）
  claude_code_only: false,
  fallback_group_id: null as number | null,
  fallback_group_id_on_invalid_request: null as number | null,
  // OpenAI Messages 调度配置（仅 openai 平台使用）
  allow_messages_dispatch: false,
  opus_mapped_model: createMessagesDispatchDefaults.opus_mapped_model,
  sonnet_mapped_model: createMessagesDispatchDefaults.sonnet_mapped_model,
  haiku_mapped_model: createMessagesDispatchDefaults.haiku_mapped_model,
  exact_model_mappings: [] as MessagesDispatchMappingRow[],
  // 账号过滤控制（OpenAI/Antigravity 平台）
  require_oauth_only: false,
  require_privacy_set: false,
  // 模型路由开关
  model_routing_enabled: false,
  // 支持的模型系列（仅 antigravity 平台）
  supported_model_scopes: ["claude", "gemini_text", "gemini_image"] as string[],
  // MCP XML 协议注入开关（仅 antigravity 平台）
  mcp_xml_inject: true,
  // 从分组复制账号
  copy_accounts_from_group_ids: [] as number[],
  // 分组级 RPM 限制（每用户每分钟最大请求数；0 = 不限制）
  rpm_limit: 0 as number,
});

// 简单账号类型（用于模型路由选择）
interface SimpleAccount {
  id: number;
  name: string;
}

// 模型路由规则类型
interface ModelRoutingRule {
  pattern: string;
  accounts: SimpleAccount[]; // 选中的账号对象数组
}

// 创建表单的模型路由规则
const createModelRoutingRules = ref<ModelRoutingRule[]>([]);

// 编辑表单的模型路由规则
const editModelRoutingRules = ref<ModelRoutingRule[]>([]);

// 规则对象稳定 key（避免使用 index 导致状态错位）
const resolveCreateRuleKey =
  createStableObjectKeyResolver<ModelRoutingRule>("create-rule");
const resolveEditRuleKey =
  createStableObjectKeyResolver<ModelRoutingRule>("edit-rule");
const resolveCreateMessagesDispatchRowKey =
  createStableObjectKeyResolver<MessagesDispatchMappingRow>(
    "create-messages-dispatch-row",
  );
const resolveEditMessagesDispatchRowKey =
  createStableObjectKeyResolver<MessagesDispatchMappingRow>(
    "edit-messages-dispatch-row",
  );

const getCreateRuleRenderKey = (rule: ModelRoutingRule) =>
  resolveCreateRuleKey(rule);
const getEditRuleRenderKey = (rule: ModelRoutingRule) =>
  resolveEditRuleKey(rule);
const getCreateMessagesDispatchRowKey = (row: MessagesDispatchMappingRow) =>
  resolveCreateMessagesDispatchRowKey(row);
const getEditMessagesDispatchRowKey = (row: MessagesDispatchMappingRow) =>
  resolveEditMessagesDispatchRowKey(row);

const getCreateRuleSearchKey = (rule: ModelRoutingRule) =>
  `create-${resolveCreateRuleKey(rule)}`;
const getEditRuleSearchKey = (rule: ModelRoutingRule) =>
  `edit-${resolveEditRuleKey(rule)}`;

const getRuleSearchKey = (rule: ModelRoutingRule, isEdit: boolean = false) => {
  return isEdit ? getEditRuleSearchKey(rule) : getCreateRuleSearchKey(rule);
};

// 账号搜索相关状态
const accountSearchKeyword = ref<Record<string, string>>({});
const accountSearchResults = ref<Record<string, SimpleAccount[]>>({});
const showAccountDropdown = ref<Record<string, boolean>>({});

const clearAccountSearchStateByKey = (key: string) => {
  delete accountSearchKeyword.value[key];
  delete accountSearchResults.value[key];
  delete showAccountDropdown.value[key];
};

const clearAllAccountSearchState = () => {
  accountSearchKeyword.value = {};
  accountSearchResults.value = {};
  showAccountDropdown.value = {};
};

const accountSearchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
  delay: 300,
  search: async (keyword, { signal }) => {
    const res = await adminAPI.accounts.list(
      1,
      20,
      {
        search: keyword,
        platform: "anthropic",
      },
      { signal },
    );
    return res.items.map((account) => ({ id: account.id, name: account.name }));
  },
  onSuccess: (key, result) => {
    accountSearchResults.value[key] = result;
  },
  onError: (key) => {
    accountSearchResults.value[key] = [];
  },
});

// 搜索账号（仅限 anthropic 平台）
const searchAccounts = (key: string) => {
  accountSearchRunner.trigger(key, accountSearchKeyword.value[key] || "");
};

const searchAccountsByRule = (
  rule: ModelRoutingRule,
  isEdit: boolean = false,
) => {
  searchAccounts(getRuleSearchKey(rule, isEdit));
};

// 选择账号
const selectAccount = (
  rule: ModelRoutingRule,
  account: SimpleAccount,
  isEdit: boolean = false,
) => {
  if (!rule) return;

  // 检查是否已选择
  if (!rule.accounts.some((a) => a.id === account.id)) {
    rule.accounts.push(account);
  }

  // 清空搜索
  const key = getRuleSearchKey(rule, isEdit);
  accountSearchKeyword.value[key] = "";
  showAccountDropdown.value[key] = false;
};

// 移除已选账号
const removeSelectedAccount = (
  rule: ModelRoutingRule,
  accountId: number,
  _isEdit: boolean = false,
) => {
  if (!rule) return;

  rule.accounts = rule.accounts.filter((a) => a.id !== accountId);
};

// 切换创建表单的模型系列选择
const toggleCreateScope = (scope: string) => {
  const idx = createForm.supported_model_scopes.indexOf(scope);
  if (idx === -1) {
    createForm.supported_model_scopes.push(scope);
  } else {
    createForm.supported_model_scopes.splice(idx, 1);
  }
};

// 切换编辑表单的模型系列选择
const toggleEditScope = (scope: string) => {
  const idx = editForm.supported_model_scopes.indexOf(scope);
  if (idx === -1) {
    editForm.supported_model_scopes.push(scope);
  } else {
    editForm.supported_model_scopes.splice(idx, 1);
  }
};

// 处理账号搜索输入框聚焦
const onAccountSearchFocus = (
  rule: ModelRoutingRule,
  isEdit: boolean = false,
) => {
  const key = getRuleSearchKey(rule, isEdit);
  showAccountDropdown.value[key] = true;
  // 如果没有搜索结果，触发一次搜索
  if (!accountSearchResults.value[key]?.length) {
    searchAccounts(key);
  }
};

// 添加创建表单的路由规则
const addCreateRoutingRule = () => {
  createModelRoutingRules.value.push({ pattern: "", accounts: [] });
};

// 删除创建表单的路由规则
const removeCreateRoutingRule = (rule: ModelRoutingRule) => {
  const index = createModelRoutingRules.value.indexOf(rule);
  if (index === -1) return;

  const key = getCreateRuleSearchKey(rule);
  accountSearchRunner.clearKey(key);
  clearAccountSearchStateByKey(key);
  createModelRoutingRules.value.splice(index, 1);
};

// 添加编辑表单的路由规则
const addEditRoutingRule = () => {
  editModelRoutingRules.value.push({ pattern: "", accounts: [] });
};

// 删除编辑表单的路由规则
const removeEditRoutingRule = (rule: ModelRoutingRule) => {
  const index = editModelRoutingRules.value.indexOf(rule);
  if (index === -1) return;

  const key = getEditRuleSearchKey(rule);
  accountSearchRunner.clearKey(key);
  clearAccountSearchStateByKey(key);
  editModelRoutingRules.value.splice(index, 1);
};

// 将 UI 格式的路由规则转换为 API 格式
const convertRoutingRulesToApiFormat = (
  rules: ModelRoutingRule[],
): Record<string, number[]> | null => {
  const result: Record<string, number[]> = {};
  let hasValidRules = false;

  for (const rule of rules) {
    const pattern = rule.pattern.trim();
    if (!pattern) continue;

    const accountIds = rule.accounts.map((a) => a.id).filter((id) => id > 0);

    if (accountIds.length > 0) {
      result[pattern] = accountIds;
      hasValidRules = true;
    }
  }

  return hasValidRules ? result : null;
};

// 将 API 格式的路由规则转换为 UI 格式（需要加载账号名称）
const convertApiFormatToRoutingRules = async (
  apiFormat: Record<string, number[]> | null,
): Promise<ModelRoutingRule[]> => {
  if (!apiFormat) return [];

  const rules: ModelRoutingRule[] = [];
  for (const [pattern, accountIds] of Object.entries(apiFormat)) {
    // 加载账号信息
    const accounts: SimpleAccount[] = [];
    for (const id of accountIds) {
      try {
        const account = await adminAPI.accounts.getById(id);
        accounts.push({ id: account.id, name: account.name });
      } catch {
        // 如果账号不存在，仍然显示 ID
        accounts.push({ id, name: `#${id}` });
      }
    }
    rules.push({ pattern, accounts });
  }
  return rules;
};

const editForm = reactive({
  name: "",
  description: "",
  platform: "anthropic" as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  status: "active" as "active" | "inactive",
  subscription_type: "standard" as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null,
  // 图片生成计费配置
  allow_image_generation: false,
  image_rate_independent: false,
  image_rate_multiplier: 1,
  image_price_1k: null as number | null,
  image_price_2k: null as number | null,
  image_price_4k: null as number | null,
  // Claude Code 客户端限制（仅 anthropic 平台使用）
  claude_code_only: false,
  fallback_group_id: null as number | null,
  fallback_group_id_on_invalid_request: null as number | null,
  // OpenAI Messages 调度配置（仅 openai 平台使用）
  allow_messages_dispatch: false,
  default_mapped_model: '',
  opus_mapped_model: editMessagesDispatchDefaults.opus_mapped_model,
  sonnet_mapped_model: editMessagesDispatchDefaults.sonnet_mapped_model,
  haiku_mapped_model: editMessagesDispatchDefaults.haiku_mapped_model,
  exact_model_mappings: [] as MessagesDispatchMappingRow[],
  // 账号过滤控制（OpenAI/Antigravity 平台）
  require_oauth_only: false,
  require_privacy_set: false,
  // 模型路由开关
  model_routing_enabled: false,
  // 支持的模型系列（仅 antigravity 平台）
  supported_model_scopes: ["claude", "gemini_text", "gemini_image"] as string[],
  // MCP XML 协议注入开关（仅 antigravity 平台）
  mcp_xml_inject: true,
  // 从分组复制账号
  copy_accounts_from_group_ids: [] as number[],
  // 分组级 RPM 限制（每用户每分钟最大请求数；0 = 不限制）
  rpm_limit: 0 as number,
});

type ImagePricingFormState = {
  rate_multiplier: number;
  image_rate_independent: boolean;
  image_rate_multiplier: number;
  image_price_1k: number | string | null;
  image_price_2k: number | string | null;
  image_price_4k: number | string | null;
};

const imagePricingTiers = [
  { key: "image_price_1k", label: "1K" },
  { key: "image_price_2k", label: "2K" },
  { key: "image_price_4k", label: "4K" },
] as const;

const normalizePreviewNumber = (value: number | string | null | undefined, fallback = 0) => {
  if (value === null || value === undefined || value === "") {
    return fallback;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : fallback;
};

const formatImagePricePreview = (value: number | string | null | undefined) => {
  if (value === null || value === undefined || value === "") {
    return t("admin.groups.imagePricing.notConfigured");
  }
  const price = Number(value);
  if (!Number.isFinite(price) || price < 0) {
    return t("admin.groups.imagePricing.notConfigured");
  }
  return `$${price.toFixed(6).replace(/0+$/, "").replace(/\.$/, "")}`;
};

const buildImageFinalPricePreview = (form: ImagePricingFormState) => {
  const multiplier = form.image_rate_independent
    ? normalizePreviewNumber(form.image_rate_multiplier, 1)
    : normalizePreviewNumber(form.rate_multiplier, 1);
  return imagePricingTiers.map((tier) => {
    const basePrice = normalizePreviewNumber(form[tier.key]);
    return {
      label: tier.label,
      value: basePrice > 0
        ? formatImagePricePreview(basePrice * multiplier)
        : t("admin.groups.imagePricing.notConfigured"),
    };
  });
};

const createImageFinalPricePreview = computed(() =>
  buildImageFinalPricePreview(createForm),
);
const editImageFinalPricePreview = computed(() =>
  buildImageFinalPricePreview(editForm),
);

// 根据分组类型返回不同的删除确认消息
const deleteConfirmMessage = computed(() => {
  if (!deletingGroup.value) {
    return "";
  }
  if (deletingGroup.value.subscription_type === "subscription") {
    return t("admin.groups.deleteConfirmSubscription", {
      name: deletingGroup.value.name,
    });
  }
  return t("admin.groups.deleteConfirm", { name: deletingGroup.value.name });
});

const loadGroups = async () => {
  if (abortController) {
    abortController.abort();
  }
  const currentController = new AbortController();
  abortController = currentController;
  const { signal } = currentController;
  loading.value = true;
  try {
    const response = await adminAPI.groups.list(
      pagination.page,
      pagination.page_size,
      {
        platform: (filters.platform as GroupPlatform) || undefined,
        status: filters.status as any,
        is_exclusive: filters.is_exclusive
          ? filters.is_exclusive === "true"
          : undefined,
        search: searchQuery.value.trim() || undefined,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order,
      },
      { signal },
    );
    if (signal.aborted) return;
    groups.value = response.items;
    pagination.total = response.total;
    pagination.pages = response.pages;
    loadUsageSummary();
    loadCapacitySummary();
  } catch (error: any) {
    if (
      signal.aborted ||
      error?.name === "AbortError" ||
      error?.code === "ERR_CANCELED"
    ) {
      return;
    }
    appStore.showError(t("admin.groups.failedToLoad"));
    console.error("Error loading groups:", error);
  } finally {
    if (abortController === currentController && !signal.aborted) {
      loading.value = false;
    }
  }
};

const formatCost = (cost: number): string => {
  if (cost >= 1000) return cost.toFixed(0);
  if (cost >= 100) return cost.toFixed(1);
  return cost.toFixed(2);
};

const loadUsageSummary = async () => {
  usageLoading.value = true;
  try {
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const data = await adminAPI.groups.getUsageSummary(tz);
    const map = new Map<number, { today_cost: number; total_cost: number }>();
    for (const item of data) {
      map.set(item.group_id, {
        today_cost: item.today_cost,
        total_cost: item.total_cost,
      });
    }
    usageMap.value = map;
  } catch (error) {
    console.error("Error loading group usage summary:", error);
  } finally {
    usageLoading.value = false;
  }
};

const loadCapacitySummary = async () => {
  try {
    const data = await adminAPI.groups.getCapacitySummary();
    const map = new Map<
      number,
      {
        concurrencyUsed: number;
        concurrencyMax: number;
        sessionsUsed: number;
        sessionsMax: number;
        rpmUsed: number;
        rpmMax: number;
      }
    >();
    for (const item of data) {
      map.set(item.group_id, {
        concurrencyUsed: item.concurrency_used,
        concurrencyMax: item.concurrency_max,
        sessionsUsed: item.sessions_used,
        sessionsMax: item.sessions_max,
        rpmUsed: item.rpm_used,
        rpmMax: item.rpm_max,
      });
    }
    capacityMap.value = map;
  } catch (error) {
    console.error("Error loading group capacity summary:", error);
  }
};

let searchTimeout: ReturnType<typeof setTimeout>;
const handleSearch = () => {
  clearTimeout(searchTimeout);
  searchTimeout = setTimeout(() => {
    pagination.page = 1;
    loadGroups();
  }, 300);
};

const handlePageChange = (page: number) => {
  pagination.page = page;
  loadGroups();
};

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize;
  pagination.page = 1;
  loadGroups();
};

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key;
  sortState.sort_order = order;
  pagination.page = 1;
  loadGroups();
};

const closeCreateModal = () => {
  showCreateModal.value = false;
  createModelRoutingRules.value.forEach((rule) => {
    accountSearchRunner.clearKey(getCreateRuleSearchKey(rule));
  });
  clearAllAccountSearchState();
  createForm.name = "";
  createForm.description = "";
  createForm.platform = "anthropic";
  createForm.rate_multiplier = 1.0;
  createForm.is_exclusive = false;
  createForm.subscription_type = "standard";
  createForm.daily_limit_usd = null;
  createForm.weekly_limit_usd = null;
  createForm.monthly_limit_usd = null;
  createForm.allow_image_generation = false;
  createForm.image_rate_independent = false;
  createForm.image_rate_multiplier = 1;
  createForm.image_price_1k = null;
  createForm.image_price_2k = null;
  createForm.image_price_4k = null;
  createForm.claude_code_only = false;
  createForm.fallback_group_id = null;
  createForm.fallback_group_id_on_invalid_request = null;
  resetMessagesDispatchFormState(createForm);
  createForm.require_oauth_only = false;
  createForm.require_privacy_set = false;
  createForm.supported_model_scopes = ["claude", "gemini_text", "gemini_image"];
  createForm.mcp_xml_inject = true;
  createForm.copy_accounts_from_group_ids = [];
  createModelRoutingRules.value = [];
};

const normalizeOptionalLimit = (
  value: number | string | null | undefined,
): number | null => {
  if (value === null || value === undefined) {
    return null;
  }

  if (typeof value === "string") {
    const trimmed = value.trim();
    if (!trimmed) {
      return null;
    }
    const parsed = Number(trimmed);
    return Number.isFinite(parsed) && parsed > 0 ? parsed : null;
  }

  return Number.isFinite(value) && value > 0 ? value : null;
};

const normalizeImageRateMultiplier = (
  value: number | string | null | undefined,
): number => {
  if (value === null || value === undefined || value === "") {
    return 1;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : 1;
};

const handleCreateGroup = async () => {
  if (!createForm.name.trim()) {
    appStore.showError(t("admin.groups.nameRequired"));
    return;
  }
  submitting.value = true;
  try {
    // 构建请求数据，包含模型路由配置
    const requestData = {
      ...createForm,
      daily_limit_usd: normalizeOptionalLimit(
        createForm.daily_limit_usd as number | string | null,
      ),
      weekly_limit_usd: normalizeOptionalLimit(
        createForm.weekly_limit_usd as number | string | null,
      ),
      monthly_limit_usd: normalizeOptionalLimit(
        createForm.monthly_limit_usd as number | string | null,
      ),
      model_routing: convertRoutingRulesToApiFormat(
        createModelRoutingRules.value,
      ),
      messages_dispatch_model_config:
        createForm.platform === "openai"
          ? messagesDispatchFormStateToConfig({
              allow_messages_dispatch: createForm.allow_messages_dispatch,
              opus_mapped_model: createForm.opus_mapped_model,
              sonnet_mapped_model: createForm.sonnet_mapped_model,
              haiku_mapped_model: createForm.haiku_mapped_model,
              exact_model_mappings: createForm.exact_model_mappings,
            })
          : undefined,
    };
    // v-model.number 清空输入框时产生 ""，转为 null 让后端设为无限制
    const emptyToNull = (v: any) => (v === "" ? null : v);
    requestData.daily_limit_usd = emptyToNull(requestData.daily_limit_usd);
    requestData.weekly_limit_usd = emptyToNull(requestData.weekly_limit_usd);
    requestData.monthly_limit_usd = emptyToNull(requestData.monthly_limit_usd);
    requestData.image_rate_multiplier = normalizeImageRateMultiplier(
      requestData.image_rate_multiplier,
    );
    await adminAPI.groups.create(requestData);
    appStore.showSuccess(t("admin.groups.groupCreated"));
    closeCreateModal();
    loadGroups();
    // Only advance tour if active, on submit step, and creation succeeded
    if (onboardingStore.isCurrentStep('[data-tour="group-form-submit"]')) {
      onboardingStore.nextStep(500);
    }
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail || t("admin.groups.failedToCreate"),
    );
    console.error("Error creating group:", error);
    // Don't advance tour on error
  } finally {
    submitting.value = false;
  }
};

const handleEdit = async (group: AdminGroup) => {
  editingGroup.value = group;
  editForm.name = group.name;
  editForm.description = group.description || "";
  editForm.platform = group.platform;
  editForm.rate_multiplier = group.rate_multiplier;
  editForm.is_exclusive = group.is_exclusive;
  editForm.status = group.status;
  editForm.subscription_type = group.subscription_type || "standard";
  editForm.daily_limit_usd = group.daily_limit_usd;
  editForm.weekly_limit_usd = group.weekly_limit_usd;
  editForm.monthly_limit_usd = group.monthly_limit_usd;
  editForm.allow_image_generation = group.allow_image_generation ?? false;
  editForm.image_rate_independent = group.image_rate_independent ?? false;
  editForm.image_rate_multiplier = group.image_rate_multiplier ?? 1;
  editForm.image_price_1k = group.image_price_1k;
  editForm.image_price_2k = group.image_price_2k;
  editForm.image_price_4k = group.image_price_4k;
  editForm.claude_code_only = group.claude_code_only || false;
  editForm.fallback_group_id = group.fallback_group_id;
  editForm.fallback_group_id_on_invalid_request =
    group.fallback_group_id_on_invalid_request;
  const messagesDispatchFormState = messagesDispatchConfigToFormState(
    group.messages_dispatch_model_config,
  );
  editForm.allow_messages_dispatch =
    group.allow_messages_dispatch ||
    messagesDispatchFormState.allow_messages_dispatch;
  editForm.opus_mapped_model = messagesDispatchFormState.opus_mapped_model;
  editForm.sonnet_mapped_model = messagesDispatchFormState.sonnet_mapped_model;
  editForm.haiku_mapped_model = messagesDispatchFormState.haiku_mapped_model;
  editForm.exact_model_mappings =
    messagesDispatchFormState.exact_model_mappings;
  editForm.require_oauth_only = group.require_oauth_only ?? false;
  editForm.require_privacy_set = group.require_privacy_set ?? false;
  editForm.model_routing_enabled = group.model_routing_enabled || false;
  editForm.supported_model_scopes = group.supported_model_scopes || [
    "claude",
    "gemini_text",
    "gemini_image",
  ];
  editForm.mcp_xml_inject = group.mcp_xml_inject ?? true;
  editForm.copy_accounts_from_group_ids = []; // 复制账号字段每次编辑时重置为空
  editForm.rpm_limit = group.rpm_limit ?? 0;
  // 加载模型路由规则（异步加载账号名称）
  editModelRoutingRules.value = await convertApiFormatToRoutingRules(
    group.model_routing,
  );
  showEditModal.value = true;
};

const closeEditModal = () => {
  editModelRoutingRules.value.forEach((rule) => {
    accountSearchRunner.clearKey(getEditRuleSearchKey(rule));
  });
  clearAllAccountSearchState();
  showEditModal.value = false;
  editingGroup.value = null;
  editModelRoutingRules.value = [];
  editForm.copy_accounts_from_group_ids = [];
  resetMessagesDispatchFormState(editForm);
};

const handleUpdateGroup = async () => {
  if (!editingGroup.value) return;
  if (!editForm.name.trim()) {
    appStore.showError(t("admin.groups.nameRequired"));
    return;
  }

  submitting.value = true;
  try {
    // 转换 fallback_group_id: null -> 0 (后端使用 0 表示清除)
    const payload = {
      ...editForm,
      daily_limit_usd: normalizeOptionalLimit(
        editForm.daily_limit_usd as number | string | null,
      ),
      weekly_limit_usd: normalizeOptionalLimit(
        editForm.weekly_limit_usd as number | string | null,
      ),
      monthly_limit_usd: normalizeOptionalLimit(
        editForm.monthly_limit_usd as number | string | null,
      ),
      fallback_group_id:
        editForm.fallback_group_id === null ? 0 : editForm.fallback_group_id,
      fallback_group_id_on_invalid_request:
        editForm.fallback_group_id_on_invalid_request === null
          ? 0
          : editForm.fallback_group_id_on_invalid_request,
      model_routing: convertRoutingRulesToApiFormat(
        editModelRoutingRules.value,
      ),
      messages_dispatch_model_config:
        editForm.platform === "openai"
          ? messagesDispatchFormStateToConfig({
              allow_messages_dispatch: editForm.allow_messages_dispatch,
              opus_mapped_model: editForm.opus_mapped_model,
              sonnet_mapped_model: editForm.sonnet_mapped_model,
              haiku_mapped_model: editForm.haiku_mapped_model,
              exact_model_mappings: editForm.exact_model_mappings,
            })
          : undefined,
    };
    // v-model.number 清空输入框时产生 ""，转为 null 让后端设为无限制
    const emptyToNull = (v: any) => (v === "" ? null : v);
    payload.daily_limit_usd = emptyToNull(payload.daily_limit_usd);
    payload.weekly_limit_usd = emptyToNull(payload.weekly_limit_usd);
    payload.monthly_limit_usd = emptyToNull(payload.monthly_limit_usd);
    payload.image_rate_multiplier = normalizeImageRateMultiplier(
      payload.image_rate_multiplier,
    );
    await adminAPI.groups.update(editingGroup.value.id, payload);
    appStore.showSuccess(t("admin.groups.groupUpdated"));
    closeEditModal();
    loadGroups();
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail || t("admin.groups.failedToUpdate"),
    );
    console.error("Error updating group:", error);
  } finally {
    submitting.value = false;
  }
};

const addCreateMessagesDispatchMapping = () => {
  createForm.exact_model_mappings.push({ claude_model: "", target_model: "" });
};

const removeCreateMessagesDispatchMapping = (
  row: MessagesDispatchMappingRow,
) => {
  const index = createForm.exact_model_mappings.indexOf(row);
  if (index !== -1) {
    createForm.exact_model_mappings.splice(index, 1);
  }
};

const addEditMessagesDispatchMapping = () => {
  editForm.exact_model_mappings.push({ claude_model: "", target_model: "" });
};

const removeEditMessagesDispatchMapping = (row: MessagesDispatchMappingRow) => {
  const index = editForm.exact_model_mappings.indexOf(row);
  if (index !== -1) {
    editForm.exact_model_mappings.splice(index, 1);
  }
};

const handleRateMultipliers = (group: AdminGroup) => {
  rateMultipliersGroup.value = group;
  showRateMultipliersModal.value = true;
};

const handleRPMOverrides = (group: AdminGroup) => {
  rpmOverridesGroup.value = group;
  showRPMOverridesModal.value = true;
};

const handleDelete = (group: AdminGroup) => {
  deletingGroup.value = group;
  showDeleteDialog.value = true;
};

const confirmDelete = async () => {
  if (!deletingGroup.value) return;

  try {
    await adminAPI.groups.delete(deletingGroup.value.id);
    appStore.showSuccess(t("admin.groups.groupDeleted"));
    showDeleteDialog.value = false;
    deletingGroup.value = null;
    loadGroups();
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail || t("admin.groups.failedToDelete"),
    );
    console.error("Error deleting group:", error);
  }
};

// 监听 subscription_type 变化，订阅模式时 is_exclusive 默认为 true
watch(
  () => createForm.subscription_type,
  (newVal) => {
    if (newVal === "subscription") {
      createForm.is_exclusive = true;
      createForm.fallback_group_id_on_invalid_request = null;
    }
  },
);

watch(
  () => createForm.platform,
  (newVal) => {
    if (!["anthropic", "antigravity"].includes(newVal)) {
      createForm.fallback_group_id_on_invalid_request = null;
    }
    if (newVal !== "openai") {
      resetMessagesDispatchFormState(createForm);
    }
    if (!["openai", "antigravity", "anthropic", "gemini"].includes(newVal)) {
      createForm.require_oauth_only = false;
      createForm.require_privacy_set = false;
    }
  },
);

watch(
  () => editForm.platform,
  (newVal) => {
    if (!["anthropic", "antigravity"].includes(newVal)) {
      editForm.fallback_group_id_on_invalid_request = null;
    }
    if (newVal !== "openai") {
      resetMessagesDispatchFormState(editForm);
    }
    if (!["openai", "antigravity", "anthropic", "gemini"].includes(newVal)) {
      editForm.require_oauth_only = false;
      editForm.require_privacy_set = false;
    }
  },
);

watch(
  () => editForm.platform,
  (newVal) => {
    if (!['anthropic', 'antigravity'].includes(newVal)) {
      editForm.fallback_group_id_on_invalid_request = null
    }
    if (newVal !== 'openai') {
      editForm.allow_messages_dispatch = false
      editForm.default_mapped_model = ''
    }
  }
)

// 点击外部关闭账号搜索下拉框
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement;
  // 检查是否点击在下拉框或输入框内
  if (!target.closest(".account-search-container")) {
    Object.keys(showAccountDropdown.value).forEach((key) => {
      showAccountDropdown.value[key] = false;
    });
  }
};

// 打开排序弹窗
const openSortModal = async () => {
  try {
    // 获取所有分组（不分页）
    const allGroups = await adminAPI.groups.getAll();
    // 按 sort_order 排序
    sortableGroups.value = [...allGroups].sort(
      (a, b) => a.sort_order - b.sort_order,
    );
    showSortModal.value = true;
  } catch (error) {
    appStore.showError(t("admin.groups.failedToLoad"));
    console.error("Error loading groups for sorting:", error);
  }
};

// 关闭排序弹窗
const closeSortModal = () => {
  showSortModal.value = false;
  sortableGroups.value = [];
};

// 保存排序
const saveSortOrder = async () => {
  sortSubmitting.value = true;
  try {
    const updates = sortableGroups.value.map((g, index) => ({
      id: g.id,
      sort_order: index * 10,
    }));
    await adminAPI.groups.updateSortOrder(updates);
    appStore.showSuccess(t("admin.groups.sortOrderUpdated"));
    closeSortModal();
    loadGroups();
  } catch (error: any) {
    appStore.showError(
      error.response?.data?.detail || t("admin.groups.failedToUpdateSortOrder"),
    );
    console.error("Error updating sort order:", error);
  } finally {
    sortSubmitting.value = false;
  }
};

onMounted(() => {
  loadGroups();
  document.addEventListener("click", handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener("click", handleClickOutside);
  accountSearchRunner.clearAll();
  clearAllAccountSearchState();
});
</script>
