<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Single Row: Search, Filters, and Actions -->
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <!-- Left: Search + Active Filters -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- Search Box -->
            <div class="relative w-full md:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <!-- Role Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('role')" class="w-full sm:w-32">
              <Select
                v-model="filters.role"
                :options="[
                  { value: '', label: t('admin.users.allRoles') },
                  { value: 'admin', label: t('admin.users.admin') },
                  { value: 'user', label: t('admin.users.user') }
                ]"
                @change="applyFilter"
              />
            </div>

            <!-- Status Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('status')" class="w-full sm:w-32">
              <Select
                v-model="filters.status"
                :options="[
                  { value: '', label: t('admin.users.allStatus') },
                  { value: 'active', label: t('common.active') },
                  { value: 'disabled', label: t('admin.users.disabled') }
                ]"
                @change="applyFilter"
              />
            </div>

            <!-- Group Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('group')" class="w-full sm:w-44">
              <Select
                v-model="filters.group"
                :options="groupFilterOptions"
                searchable
                creatable
                :creatable-prefix="t('admin.users.fuzzySearch')"
                :search-placeholder="t('admin.users.searchAuthorizedGroups')"
                @change="applyFilter"
              />
            </div>

            <!-- API Key Group Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('apiKeyGroup')" class="w-full sm:w-44">
              <Select
                v-model="filters.apiKeyGroup"
                :options="apiKeyGroupFilterOptions"
                searchable
                :search-placeholder="t('admin.users.searchApiKeyGroups')"
                @change="applyFilter"
              />
            </div>

            <!-- Dynamic Attribute Filters -->
            <template v-for="(value, attrId) in activeAttributeFilters" :key="attrId">
              <div
                v-if="visibleFilters.has(`attr_${attrId}`)"
                class="relative w-full sm:w-36"
              >
                <!-- Text/Email/URL/Textarea/Date type: styled input -->
                <input
                  v-if="['text', 'textarea', 'email', 'url', 'date'].includes(getAttributeDefinition(Number(attrId))?.type || 'text')"
                  :value="value"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-full"
                />
                <!-- Number type: number input -->
                <input
                  v-else-if="getAttributeDefinition(Number(attrId))?.type === 'number'"
                  :value="value"
                  type="number"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-full"
                />
                <!-- Select/Multi-select type -->
                <template v-else-if="['select', 'multi_select'].includes(getAttributeDefinition(Number(attrId))?.type || '')">
                  <div class="w-full">
                    <Select
                      :model-value="value"
                      :options="[
                        { value: '', label: getAttributeDefinitionName(Number(attrId)) },
                        ...(getAttributeDefinition(Number(attrId))?.options || [])
                      ]"
                      @update:model-value="(val) => { updateAttributeFilter(Number(attrId), String(val ?? '')); applyFilter() }"
                    />
                  </div>
                </template>
                <!-- Fallback -->
                <input
                  v-else
                  :value="value"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-full"
                />
              </div>
            </template>
          </div>

          <!-- Right: Actions and Settings -->
          <div class="flex flex-wrap items-center justify-end gap-2">
            <!-- Mobile: Secondary buttons (icon only) -->
            <div class="flex items-center gap-2 md:contents">
              <!-- Refresh Button -->
              <button
                @click="loadUsers"
                :disabled="loading"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('common.refresh')"
              >
                <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
              </button>
              <!-- Filter Settings Dropdown -->
              <div class="relative" ref="filterDropdownRef">
                <button
                  @click="showFilterDropdown = !showFilterDropdown"
                  class="btn btn-secondary px-2 md:px-3"
                  :title="t('admin.users.filterSettings')"
                >
                  <Icon name="filter" size="sm" class="md:mr-1.5" />
                  <span class="hidden md:inline">{{ t('admin.users.filterSettings') }}</span>
                </button>
                <!-- Dropdown menu -->
                <div
                  v-if="showFilterDropdown"
                  class="absolute right-0 top-full z-50 mt-1 w-48 rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
                >
                  <!-- Built-in filters -->
                  <button
                    v-for="filter in builtInFilters"
                    :key="filter.key"
                    @click="toggleBuiltInFilter(filter.key)"
                    class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
                  >
                    <span>{{ filter.name }}</span>
                    <Icon
                      v-if="visibleFilters.has(filter.key)"
                      name="check"
                      size="sm"
                      class="text-primary-500"
                      :stroke-width="2"
                    />
                  </button>
                  <!-- Divider if custom attributes exist -->
                  <div
                    v-if="filterableAttributes.length > 0"
                    class="my-1 border-t border-gray-100 dark:border-dark-700"
                  ></div>
                  <!-- Custom attribute filters -->
                  <button
                    v-for="attr in filterableAttributes"
                    :key="attr.id"
                    @click="toggleAttributeFilter(attr)"
                    class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
                  >
                    <span>{{ attr.name }}</span>
                    <Icon
                      v-if="visibleFilters.has(`attr_${attr.id}`)"
                      name="check"
                      size="sm"
                      class="text-primary-500"
                      :stroke-width="2"
                    />
                  </button>
                </div>
              </div>
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
                  class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
                >
                  <button
                    v-for="col in toggleableColumns"
                    :key="col.key"
                    :disabled="isForcedVisibleColumn(col.key)"
                    @click="toggleColumn(col.key)"
                    :class="[
                      'flex w-full items-center justify-between px-4 py-2 text-left text-sm',
                      isForcedVisibleColumn(col.key)
                        ? 'cursor-not-allowed text-gray-400 dark:text-gray-500'
                        : 'text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700'
                    ]"
                    :title="isForcedVisibleColumn(col.key) ? t('admin.users.columnAlwaysVisible') : ''"
                  >
                    <span>{{ col.label }}</span>
                    <Icon
                      v-if="isColumnVisible(col.key)"
                      name="check"
                      size="sm"
                      :class="isForcedVisibleColumn(col.key) ? 'text-gray-400 dark:text-gray-500' : 'text-primary-500'"
                      :stroke-width="2"
                    />
                  </button>
                </div>
              </div>
              <!-- Attributes Config Button -->
              <button
                @click="showAttributesModal = true"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.users.attributes.configButton')"
              >
                <Icon name="cog" size="sm" class="md:mr-1.5" />
                <span class="hidden md:inline">{{ t('admin.users.attributes.configButton') }}</span>
              </button>
            </div>

            <!-- Create User Button (full width on mobile, auto width on desktop) -->
            <button @click="showCreateModal = true" class="btn btn-primary flex-1 md:flex-initial">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.users.createUser') }}
            </button>
          </div>
        </div>
      </template>

      <!-- Users Table -->
      <template #table>
        <DataTable
          :columns="columns"
          :data="sortedUsers"
          :loading="loading"
          :actions-count="7"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          :sort-storage-key="USER_SORT_STORAGE_KEY"
          @sort="handleSort"
        >
          <template #cell-email="{ value }">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ value.charAt(0).toUpperCase() }}
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
            </div>
          </template>

          <template #cell-username="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value || '-' }}</span>
          </template>

          <template #cell-notes="{ value }">
            <div class="max-w-xs">
              <span
                v-if="value"
                :title="value.length > 30 ? value : undefined"
                class="block truncate text-sm text-gray-600 dark:text-gray-400"
              >
                {{ value.length > 30 ? value.substring(0, 25) + '...' : value }}
              </span>
              <span v-else class="text-sm text-gray-400">-</span>
            </div>
          </template>

          <!-- Dynamic attribute columns -->
          <template
            v-for="def in attributeDefinitions.filter(d => d.enabled)"
            :key="def.id"
            #[`cell-attr_${def.id}`]="{ row }"
          >
            <div class="max-w-xs">
              <span
                class="block truncate text-sm text-gray-700 dark:text-gray-300"
                :title="getAttributeValue(row.id, def.id)"
              >
                {{ getAttributeValue(row.id, def.id) }}
              </span>
            </div>
          </template>

          <template #cell-role="{ value }">
            <span :class="['badge', value === 'admin' ? 'badge-purple' : 'badge-gray']">
              {{ t('admin.users.roles.' + value) }}
            </span>
          </template>

          <template #cell-groups="{ row }">
            <div v-if="allGroups.length > 0" class="flex flex-col gap-1">
              <!-- 专属分组行 -->
              <span
                v-if="getUserGroups(row).exclusive.length > 0"
                class="group/ex relative inline-flex cursor-pointer items-center gap-1 whitespace-nowrap text-xs"
                @click.stop="toggleExpandedGroup(row.id)"
              >
                <Icon name="shield" size="xs" class="h-3.5 w-3.5 text-purple-500 dark:text-purple-400" />
                <span class="font-medium text-purple-600 dark:text-purple-400">{{ getUserGroups(row).exclusive.length }}</span>
                <span class="text-gray-500 dark:text-dark-400">{{ t('admin.users.exclusiveLabel') }}</span>
                <!-- Hover tooltip（操作菜单未打开时显示） -->
                <div
                  v-if="expandedGroupUserId !== row.id"
                  class="pointer-events-none absolute left-0 top-full z-50 mt-1.5 rounded bg-gray-900 px-2.5 py-1.5 text-xs text-white opacity-0 shadow-lg transition-opacity duration-75 group-hover/ex:opacity-100 dark:bg-dark-600"
                >
                  <div class="absolute left-4 bottom-full border-4 border-transparent border-b-gray-900 dark:border-b-dark-600"></div>
                  <div class="flex flex-col gap-0.5 whitespace-nowrap">
                    <span v-for="g in getUserGroups(row).exclusive" :key="g.id">{{ g.name }}</span>
                  </div>
                </div>
                <!-- 点击展开分组操作菜单 -->
                <div
                  v-if="expandedGroupUserId === row.id"
                  class="absolute left-0 top-full z-50 mt-1.5 min-w-[160px] overflow-hidden rounded-lg border border-gray-200 bg-white py-1 text-xs shadow-xl dark:border-dark-600 dark:bg-dark-700"
                >
                  <div class="border-b border-gray-100 px-3 py-1.5 text-[10px] font-medium uppercase tracking-wider text-gray-400 dark:border-dark-600 dark:text-dark-400">
                    {{ t('admin.users.clickToReplace') }}
                  </div>
                  <div
                    v-for="g in getUserGroups(row).exclusive"
                    :key="g.id"
                    class="flex cursor-pointer items-center gap-2 px-3 py-2 text-gray-700 transition-colors hover:bg-primary-50 hover:text-primary-600 dark:text-dark-200 dark:hover:bg-primary-900/30 dark:hover:text-primary-400"
                    @click.stop="openGroupReplace(row, g)"
                  >
                    <Icon name="swap" size="xs" class="h-3.5 w-3.5 flex-shrink-0 opacity-50" />
                    <span class="flex-1">{{ g.name }}</span>
                  </div>
                </div>
              </span>
              <!-- 公开分组行 -->
              <span
                v-if="getUserGroups(row).publicGroups.length > 0"
                class="group/pub relative inline-flex cursor-default items-center gap-1 whitespace-nowrap text-xs"
              >
                <Icon name="globe" size="xs" class="h-3.5 w-3.5 text-gray-400 dark:text-dark-500" />
                <span class="font-medium text-gray-600 dark:text-dark-300">{{ getUserGroups(row).publicGroups.length }}</span>
                <span class="text-gray-400 dark:text-dark-500">{{ t('admin.users.publicLabel') }}</span>
                <!-- Tooltip: 向下弹出 -->
                <div class="pointer-events-none absolute left-0 top-full z-50 mt-1.5 rounded bg-gray-900 px-2.5 py-1.5 text-xs text-white opacity-0 shadow-lg transition-opacity duration-75 group-hover/pub:opacity-100 dark:bg-dark-600">
                  <div class="absolute left-4 bottom-full border-4 border-transparent border-b-gray-900 dark:border-b-dark-600"></div>
                  <div class="flex flex-col gap-0.5 whitespace-nowrap">
                    <span v-for="g in getUserGroups(row).publicGroups" :key="g.id">{{ g.name }}</span>
                  </div>
                </div>
              </span>
              <!-- 都没有 -->
              <span
                v-if="getUserGroups(row).exclusive.length === 0 && getUserGroups(row).publicGroups.length === 0"
                class="text-xs text-gray-400 dark:text-dark-500"
              >-</span>
            </div>
            <span v-else class="text-xs text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-subscriptions="{ row }">
            <div
              v-if="row.subscriptions && row.subscriptions.length > 0"
              class="flex flex-wrap gap-1.5"
            >
              <GroupBadge
                v-for="sub in row.subscriptions"
                :key="sub.id"
                :name="sub.group?.name || ''"
                :platform="sub.group?.platform"
                :subscription-type="sub.group?.subscription_type"
                :rate-multiplier="sub.group?.rate_multiplier"
                :days-remaining="sub.expires_at ? getDaysRemaining(sub.expires_at) : null"
                :title="sub.expires_at ? formatDateTime(sub.expires_at) : ''"
              />
            </div>
            <span
              v-else
              class="inline-flex items-center gap-1.5 rounded-md bg-gray-50 px-2 py-1 text-xs text-gray-400 dark:bg-dark-700/50 dark:text-dark-500"
            >
              <Icon name="ban" size="xs" class="h-3.5 w-3.5" />
              <span>{{ t('admin.users.noSubscription') }}</span>
            </span>
          </template>

          <template #cell-balance="{ value, row }">
            <div class="flex items-center gap-2">
              <div class="group relative">
                <button
                  class="font-medium text-gray-900 underline decoration-dashed decoration-gray-300 underline-offset-4 transition-colors hover:text-primary-600 dark:text-white dark:decoration-dark-500 dark:hover:text-primary-400"
                  @click="handleBalanceHistory(row)"
                >
                  ${{ value.toFixed(2) }}
                </button>
                <!-- Instant tooltip -->
                <div class="pointer-events-none absolute bottom-full left-1/2 z-50 mb-1.5 -translate-x-1/2 whitespace-nowrap rounded bg-gray-900 px-2 py-1 text-xs text-white opacity-0 shadow-lg transition-opacity duration-75 group-hover:opacity-100 dark:bg-dark-600">
                  {{ t('admin.users.balanceHistoryTip') }}
                  <div class="absolute left-1/2 top-full -translate-x-1/2 border-4 border-transparent border-t-gray-900 dark:border-t-dark-600"></div>
                </div>
              </div>
              <button
                @click.stop="handleDeposit(row)"
                class="rounded px-2 py-0.5 text-xs font-medium text-emerald-600 transition-colors hover:bg-emerald-50 dark:text-emerald-400 dark:hover:bg-emerald-900/20"
                :title="t('admin.users.deposit')"
              >
                {{ t('admin.users.deposit') }}
              </button>
            </div>
          </template>

          <template #cell-balance_platform_quota="{ row }">
            <button
              type="button"
              class="block text-left underline decoration-dashed decoration-gray-300 underline-offset-4 transition-colors hover:decoration-primary-400 dark:decoration-dark-500"
              :title="t('admin.users.platformQuota.cellColumnTooltip')"
              @click="handlePlatformQuota(row)"
            >
              <UserPlatformQuotaCell :quotas="platformQuotaStats[row.id]" />
            </button>
          </template>

          <!-- 用量列自定义表头：列名 + 单个排序图标按钮，点击展开"今日/近30天"菜单。
               column.sortable=false，DataTable 内置点击逻辑不会触发；
               菜单项三态循环：desc → asc → off。 -->
          <template
            v-for="usageKey in USAGE_COLUMN_KEYS"
            :key="usageKey"
            #[`header-${usageKey}`]="{ column }"
          >
            <div class="flex items-center gap-1.5">
              <span>{{ column.label }}</span>
              <div class="usage-sort-trigger relative">
                <button
                  type="button"
                  class="flex items-center gap-1 rounded px-1 py-0.5 transition-colors hover:bg-gray-200 dark:hover:bg-dark-700"
                  :class="usageSort && usageSort.key === usageKey
                    ? 'text-primary-600 dark:text-primary-400'
                    : 'text-gray-400 dark:text-dark-500'"
                  :title="t('admin.users.sortBy')"
                  @click.stop="toggleUsageSortMenu(usageKey)"
                >
                  <span
                    v-if="usageSort && usageSort.key === usageKey"
                    class="text-[10px] normal-case font-medium tracking-normal"
                  >{{ usageSort.metric === 'today' ? t('admin.users.today') : t('admin.users.total') }}</span>
                  <svg
                    v-if="usageSort && usageSort.key === usageKey"
                    class="h-3.5 w-3.5"
                    :class="{ 'rotate-180': usageSort.order === 'desc' }"
                    fill="currentColor"
                    viewBox="0 0 20 20"
                  >
                    <path
                      fill-rule="evenodd"
                      d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"
                      clip-rule="evenodd"
                    />
                  </svg>
                  <svg v-else class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 20 20">
                    <path d="M10 3l-4 5h8l-4-5zM10 17l4-5H6l4 5z" />
                  </svg>
                </button>
                <!-- 弹出菜单：今日 / 近30天，点击进行三态循环切换。 -->
                <div
                  v-if="openUsageSortMenu === usageKey"
                  class="absolute right-0 top-full z-50 mt-1 min-w-[120px] rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
                >
                  <button
                    v-for="metric in (['today', 'total'] as const)"
                    :key="metric"
                    type="button"
                    class="flex w-full items-center justify-between gap-3 px-3 py-1.5 text-left text-xs normal-case tracking-normal hover:bg-gray-100 dark:hover:bg-dark-700"
                    :class="isUsageSortActive(usageKey, metric)
                      ? 'font-medium text-primary-600 dark:text-primary-400'
                      : 'text-gray-700 dark:text-gray-300'"
                    @click.stop="toggleUsageSort(usageKey, metric)"
                  >
                    <span>{{ metric === 'today' ? t('admin.users.today') : t('admin.users.total') }}</span>
                    <svg
                      v-if="getUsageSortOrder(usageKey, metric)"
                      class="h-3 w-3"
                      :class="{ 'rotate-180': getUsageSortOrder(usageKey, metric) === 'desc' }"
                      fill="currentColor"
                      viewBox="0 0 20 20"
                    >
                      <path
                        fill-rule="evenodd"
                        d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z"
                        clip-rule="evenodd"
                      />
                    </svg>
                  </button>
                  <div class="mt-1 border-t border-gray-100 px-3 py-1 text-[10px] normal-case tracking-normal text-gray-400 dark:border-dark-700 dark:text-dark-500">
                    {{ t('admin.users.sortCurrentPageOnly') }}
                  </div>
                </div>
              </div>
            </div>
          </template>

          <template #cell-usage="{ row }">
            <PlatformUsageBreakdown
              :today="usageStats[row.id]?.today_actual_cost ?? 0"
              :total="usageStats[row.id]?.total_actual_cost ?? 0"
              :by-platform="usageStats[row.id]?.by_platform"
            />
          </template>

          <template #cell-usage_anthropic="{ row }">
            <PlatformCostCell :usage="getPlatformUsage(row.id, 'anthropic')" />
          </template>

          <template #cell-usage_openai="{ row }">
            <PlatformCostCell :usage="getPlatformUsage(row.id, 'openai')" />
          </template>

          <template #cell-usage_gemini="{ row }">
            <PlatformCostCell :usage="getPlatformUsage(row.id, 'gemini')" />
          </template>

          <template #cell-usage_antigravity="{ row }">
            <PlatformCostCell :usage="getPlatformUsage(row.id, 'antigravity')" />
          </template>

          <template #cell-concurrency="{ row }">
            <UserConcurrencyCell
              :current="row.current_concurrency ?? 0"
              :max="row.concurrency"
            />
          </template>

          <template #cell-status="{ value }">
            <div class="flex items-center gap-1.5">
              <span
                :class="[
                  'inline-block h-2 w-2 rounded-full',
                  value === 'active' ? 'bg-green-500' : 'bg-red-500'
                ]"
              ></span>
              <span class="text-sm text-gray-700 dark:text-gray-300">
                {{ value === 'active' ? t('common.active') : t('admin.users.disabled') }}
              </span>
            </div>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-last_used_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">
              {{ value ? formatDateTime(value) : '-' }}
            </span>
          </template>

          <template #cell-last_active_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-dark-400">
              {{ value ? formatDateTime(value) : '-' }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <!-- Edit Button -->
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>

              <!-- Toggle Status Button (not for admin) -->
              <button
                v-if="row.role !== 'admin'"
                @click="handleToggleStatus(row)"
                :class="[
                  'flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors',
                  row.status === 'active'
                    ? 'hover:bg-orange-50 hover:text-orange-600 dark:hover:bg-orange-900/20 dark:hover:text-orange-400'
                    : 'hover:bg-green-50 hover:text-green-600 dark:hover:bg-green-900/20 dark:hover:text-green-400'
                ]"
              >
                <Icon v-if="row.status === 'active'" name="ban" size="sm" />
                <Icon v-else name="checkCircle" size="sm" />
                <span class="text-xs">{{ row.status === 'active' ? t('admin.users.disable') : t('admin.users.enable') }}</span>
              </button>

              <!-- More Actions Menu Trigger -->
              <button
                @click="openActionMenu(row, $event)"
                class="action-menu-trigger flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
                :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': activeMenuId === row.id }"
              >
                <Icon name="more" size="sm" />
                <span class="text-xs">{{ t('common.more') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.users.noUsersYet')"
              :description="t('admin.users.createFirstUser')"
              :action-text="t('admin.users.createUser')"
              @action="showCreateModal = true"
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

    <!-- Action Menu (Teleported) -->
    <Teleport to="body">
      <div
        v-if="activeMenuId !== null && menuPosition"
        class="action-menu-content fixed z-[9999] w-48 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
        :style="{ top: menuPosition.top + 'px', left: menuPosition.left + 'px' }"
      >
        <div class="py-1">
          <template v-for="user in users" :key="user.id">
            <template v-if="user.id === activeMenuId">
              <!-- View API Keys -->
              <button
                @click="handleViewApiKeys(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="key" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('admin.users.apiKeys') }}
              </button>

              <!-- Allowed Groups -->
              <button
                @click="handleAllowedGroups(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="users" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('admin.users.groups') }}
              </button>

              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>

              <!-- Deposit -->
              <button
                @click="handleDeposit(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="plus" size="sm" class="text-emerald-500" :stroke-width="2" />
                {{ t('admin.users.deposit') }}
              </button>

              <!-- Withdraw -->
              <button
                @click="handleWithdraw(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
                </svg>
                {{ t('admin.users.withdraw') }}
              </button>

              <!-- Platform Quotas -->
              <button
                @click="handlePlatformQuota(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="chartBar" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('admin.users.platformQuota.menuItem') }}
              </button>

              <!-- Balance History -->
              <button
                @click="handleBalanceHistory(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <Icon name="dollar" size="sm" class="text-gray-400" :stroke-width="2" />
                {{ t('admin.users.balanceHistory') }}
              </button>

              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>

              <!-- Delete (not for admin) -->
              <button
                v-if="user.role !== 'admin'"
                @click="handleDelete(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
              >
                <Icon name="trash" size="sm" :stroke-width="2" />
                {{ t('common.delete') }}
              </button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

    <ConfirmDialog :show="showDeleteDialog" :title="t('admin.users.deleteUser')" :message="t('admin.users.deleteConfirm', { email: deletingUser?.email })" :danger="true" @confirm="confirmDelete" @cancel="showDeleteDialog = false" />
    <UserCreateModal :show="showCreateModal" @close="showCreateModal = false" @success="loadUsers" />
    <UserEditModal :show="showEditModal" :user="editingUser" @close="closeEditModal" @success="loadUsers" />
    <UserPlatformQuotaModal
      :show="showPlatformQuotaModal"
      :user="platformQuotaUser"
      @close="closePlatformQuotaModal"
      @success="loadUsers"
    />
    <UserApiKeysModal :show="showApiKeysModal" :user="viewingUser" @close="closeApiKeysModal" />
    <UserAllowedGroupsModal :show="showAllowedGroupsModal" :user="allowedGroupsUser" @close="closeAllowedGroupsModal" @success="loadUsers" />
    <UserBalanceModal :show="showBalanceModal" :user="balanceUser" :operation="balanceOperation" @close="closeBalanceModal" @success="loadUsers" />
    <UserBalanceHistoryModal :show="showBalanceHistoryModal" :user="balanceHistoryUser" @close="closeBalanceHistoryModal" @deposit="handleDepositFromHistory" @withdraw="handleWithdrawFromHistory" />
    <GroupReplaceModal :show="showGroupReplaceModal" :user="groupReplaceUser" :old-group="groupReplaceOldGroup" :all-groups="allGroups" @close="closeGroupReplaceModal" @success="loadUsers" />
    <UserAttributesConfigModal :show="showAttributesModal" @close="handleAttributesModalClose" />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'
import { formatDateTime } from '@/utils/format'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
import { adminAPI } from '@/api/admin'
import type { AdminUser, AdminGroup, UserAttributeDefinition } from '@/types'
import type { BatchUserUsageStats } from '@/api/admin/dashboard'
import type { PlatformQuotaItem } from '@/api/admin/users'
import type { Column } from '@/components/common/types'
import type { SelectOption } from '@/components/common/Select.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Select from '@/components/common/Select.vue'
import { buildApiKeyGroupFilterOptions } from './apiKeyGroupFilterOptions'
import UserAttributesConfigModal from '@/components/user/UserAttributesConfigModal.vue'
import UserConcurrencyCell from '@/components/user/UserConcurrencyCell.vue'
import PlatformUsageBreakdown from '@/components/user/PlatformUsageBreakdown.vue'
import PlatformCostCell from '@/components/user/PlatformCostCell.vue'
import UserPlatformQuotaCell from '@/components/user/UserPlatformQuotaCell.vue'
import UserCreateModal from '@/components/admin/user/UserCreateModal.vue'
import UserEditModal from '@/components/admin/user/UserEditModal.vue'
import UserPlatformQuotaModal from '@/components/admin/user/UserPlatformQuotaModal.vue'
import UserApiKeysModal from '@/components/admin/user/UserApiKeysModal.vue'
import UserAllowedGroupsModal from '@/components/admin/user/UserAllowedGroupsModal.vue'
import UserBalanceModal from '@/components/admin/user/UserBalanceModal.vue'
import UserBalanceHistoryModal from '@/components/admin/user/UserBalanceHistoryModal.vue'
import GroupReplaceModal from '@/components/admin/user/GroupReplaceModal.vue'

const appStore = useAppStore()

// Generate dynamic attribute columns from enabled definitions
const attributeColumns = computed<Column[]>(() =>
  attributeDefinitions.value
    .filter(def => def.enabled)
    .map(def => ({
      key: `attr_${def.id}`,
      label: def.name,
      sortable: false
    }))
)

// Get formatted attribute value for display in table
const getAttributeValue = (userId: number, attrId: number): string => {
  const userAttrs = userAttributeValues.value[userId]
  if (!userAttrs) return '-'
  const value = userAttrs[attrId]
  if (!value) return '-'

  // Find definition for this attribute
  const def = attributeDefinitions.value.find(d => d.id === attrId)
  if (!def) return value

  // Format based on type
  if (def.type === 'multi_select' && value) {
    try {
      const arr = JSON.parse(value)
      if (Array.isArray(arr)) {
        // Map values to labels
        return arr.map(v => {
          const opt = def.options?.find(o => o.value === v)
          return opt?.label || v
        }).join(', ')
      }
    } catch {
      return value
    }
  }

  if (def.type === 'select' && value && def.options) {
    const opt = def.options.find(o => o.value === value)
    return opt?.label || value
  }

  return value
}

// All possible columns (for column settings)
const allColumns = computed<Column[]>(() => [
  { key: 'email', label: t('admin.users.columns.user'), sortable: true },
  { key: 'id', label: t('admin.users.columns.id'), sortable: true },
  { key: 'username', label: t('admin.users.columns.username'), sortable: true },
  { key: 'notes', label: t('admin.users.columns.notes'), sortable: false },
  // Dynamic attribute columns
  ...attributeColumns.value,
  { key: 'role', label: t('admin.users.columns.role'), sortable: true },
  { key: 'groups', label: t('admin.users.columns.groups'), sortable: false },
  { key: 'subscriptions', label: t('admin.users.columns.subscriptions'), sortable: false },
  { key: 'balance', label: t('admin.users.columns.balance'), sortable: true },
  { key: 'balance_platform_quota', label: t('admin.users.columns.balancePlatformQuota'), sortable: false },
  { key: 'usage', label: t('admin.users.columns.usage'), sortable: false },
  { key: 'usage_anthropic', label: t('admin.users.columns.usageAnthropic'), sortable: false },
  { key: 'usage_openai', label: t('admin.users.columns.usageOpenAI'), sortable: false },
  { key: 'usage_gemini', label: t('admin.users.columns.usageGemini'), sortable: false },
  { key: 'usage_antigravity', label: t('admin.users.columns.usageAntigravity'), sortable: false },
  { key: 'concurrency', label: t('admin.users.columns.concurrency'), sortable: true },
  { key: 'status', label: t('admin.users.columns.status'), sortable: true },
  { key: 'last_active_at', label: t('admin.users.columns.lastActive'), sortable: true },
  { key: 'last_used_at', label: t('admin.users.columns.lastUsed'), sortable: true },
  { key: 'created_at', label: t('admin.users.columns.created'), sortable: true },
  { key: 'actions', label: t('admin.users.columns.actions'), sortable: false }
])

// Columns that can be toggled (exclude email and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'email' && col.key !== 'actions')
)

// Hidden columns (stored in Set - columns NOT in this set are visible)
// This way, new columns are visible by default
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns (columns hidden by default on first load)
const DEFAULT_HIDDEN_COLUMNS = [
  'notes', 'groups', 'subscriptions', 'usage', 'concurrency',
  'usage_anthropic', 'usage_openai', 'usage_gemini', 'usage_antigravity',
  'balance_platform_quota'
]
const REMOVED_COLUMNS = new Set(['last_login_at'])
// 强制可见列：加载时会被强制移出 hiddenColumns，并在列设置 UI 上 disabled。
// 当前没有列需要强制可见 —— last_active_at 已改为可被用户隐藏。
const FORCED_VISIBLE_COLUMNS = new Set<string>()

// localStorage keys for column settings
const HIDDEN_COLUMNS_KEY = 'user-hidden-columns'
// 列设置 schema 版本号。每次给 DEFAULT_HIDDEN_COLUMNS 新增列时 bump 一次，
// 并在 VERSION_NEW_HIDDEN_COLUMNS 中登记该版本新增的 key。
// 这样老用户升级后这些新列会被自动隐藏一次，而不会影响他们对其它老列的偏好。
const COLUMN_SETTINGS_VERSION_KEY = 'user-column-settings-version'
const COLUMN_SETTINGS_VERSION = 3
const VERSION_NEW_HIDDEN_COLUMNS: Record<number, string[]> = {
  2: ['usage_anthropic', 'usage_openai', 'usage_gemini', 'usage_antigravity'],
  3: ['balance_platform_quota']
}

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed
        .filter(key => !REMOVED_COLUMNS.has(key) && !FORCED_VISIBLE_COLUMNS.has(key))
        .forEach(key => hiddenColumns.add(key))

      // 老用户升级：把每个未应用过的版本里新增的默认隐藏列自动追加到 hiddenColumns。
      const storedVersion = Number(localStorage.getItem(COLUMN_SETTINGS_VERSION_KEY) ?? '1')
      if (storedVersion < COLUMN_SETTINGS_VERSION) {
        let mutated = false
        for (let v = storedVersion + 1; v <= COLUMN_SETTINGS_VERSION; v++) {
          for (const key of VERSION_NEW_HIDDEN_COLUMNS[v] ?? []) {
            if (REMOVED_COLUMNS.has(key) || FORCED_VISIBLE_COLUMNS.has(key)) continue
            if (!hiddenColumns.has(key)) {
              hiddenColumns.add(key)
              mutated = true
            }
          }
        }
        if (mutated) saveColumnsToStorage()
        else localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
      }
    } else {
      // Use default hidden columns on first load
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
      localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
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
    localStorage.setItem(COLUMN_SETTINGS_VERSION_KEY, String(COLUMN_SETTINGS_VERSION))
  } catch (e) {
    console.error('Failed to save columns:', e)
  }
}

// Toggle column visibility
const isForcedVisibleColumn = (key: string) => FORCED_VISIBLE_COLUMNS.has(key)
const toggleColumn = (key: string) => {
  // 强制可见列(如 last_active_at)在加载时会被恢复成可见，
  // 这里阻止用户在当前会话隐藏它，避免"取消勾选 → 刷新又恢复"的反直觉行为。
  if (FORCED_VISIBLE_COLUMNS.has(key)) return
  const wasHidden = hiddenColumns.has(key)
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
  if (wasHidden && (key === 'usage' || key.startsWith('usage_') || key.startsWith('attr_') || key === 'balance_platform_quota')) {
    refreshCurrentPageSecondaryData()
  }
  if (key === 'subscriptions') {
    loadUsers()
  }
  if (wasHidden && key === 'groups') {
    loadAllGroups()
  }
}

// Check if column is visible (not in hidden set)
const isColumnVisible = (key: string) => !hiddenColumns.has(key)
// usage 主列或任意 usage_<platform> 子列可见时都需要批量拉取用量数据
// 列 key → 平台名（'usage' 主列汇总所有平台时为 null）
// 显式数组取代 Object.keys()：保证迭代顺序（决定列头排序按钮渲染顺序）
// 不会因 JS 引擎差异或 USAGE_COLUMN_PLATFORMS 属性顺序调整而静默变化。
const USAGE_COLUMN_KEYS: readonly string[] = ['usage', 'usage_anthropic', 'usage_openai', 'usage_gemini', 'usage_antigravity']
const USAGE_COLUMN_PLATFORMS: Record<string, string | null> = {
  usage: null,
  usage_anthropic: 'anthropic',
  usage_openai: 'openai',
  usage_gemini: 'gemini',
  usage_antigravity: 'antigravity'
}
const PLATFORM_USAGE_COLUMNS = USAGE_COLUMN_KEYS.filter((k) => k !== 'usage')
const hasVisibleUsageColumn = computed(
  () => !hiddenColumns.has('usage') || PLATFORM_USAGE_COLUMNS.some((k) => !hiddenColumns.has(k))
)
const hasVisibleGroupsColumn = computed(() => !hiddenColumns.has('groups'))
const hasVisiblePlatformQuotaColumn = computed(() => !hiddenColumns.has('balance_platform_quota'))
const hasVisibleAttributeColumns = computed(() =>
  attributeDefinitions.value.some((def) => def.enabled && !hiddenColumns.has(`attr_${def.id}`))
)

// Filtered columns based on visibility
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'email' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

const users = ref<AdminUser[]>([])
const loading = ref(false)
const searchQuery = ref('')
const USER_SORT_STORAGE_KEY = 'admin-users-table-sort'
const loadInitialSortState = (): { sort_by: string; sort_order: 'asc' | 'desc' } => {
  const fallback = { sort_by: 'created_at', sort_order: 'desc' as 'asc' | 'desc' }
  const sortable = new Set(['email', 'id', 'username', 'role', 'balance', 'concurrency', 'status', 'last_used_at', 'last_active_at', 'created_at'])
  try {
    const raw = localStorage.getItem(USER_SORT_STORAGE_KEY)
    if (!raw) return fallback
    const parsed = JSON.parse(raw) as { key?: string; order?: string }
    const key = typeof parsed.key === 'string' ? parsed.key : ''
    if (!sortable.has(key)) return fallback
    return {
      sort_by: key,
      sort_order: parsed.order === 'asc' ? 'asc' : 'desc'
    }
  } catch {
    return fallback
  }
}
const sortState = reactive(loadInitialSortState())

// Groups data for the groups column and the existing "authorised group" filter (active only)
const allGroups = ref<AdminGroup[]>([])
const loadAllGroups = async () => {
  if (allGroups.value.length > 0) return
  try {
    allGroups.value = await adminAPI.groups.getAll()
  } catch (e) {
    console.error('Failed to load groups:', e)
  }
}

// Groups for the API Key group filter — includes disabled groups so admins can
// filter users whose keys are still bound to a now-disabled group.
const allGroupsForApiKeyFilter = ref<AdminGroup[]>([])
const loadAllGroupsForApiKeyFilter = async () => {
  if (allGroupsForApiKeyFilter.value.length > 0) return
  try {
    allGroupsForApiKeyFilter.value = await adminAPI.groups.getAllIncludingInactive()
  } catch (e) {
    console.error('Failed to load groups for API key filter:', e)
  }
}
// Resolve user's accessible groups: exclusive groups first, then public groups
const getUserGroups = (user: AdminUser) => {
  const exclusive: AdminGroup[] = []
  const publicGroups: AdminGroup[] = []
  for (const g of allGroups.value) {
    if (g.status !== 'active' || g.subscription_type !== 'standard') continue
    if (g.is_exclusive) {
      if (user.allowed_groups?.includes(g.id)) {
        exclusive.push(g)
      }
    } else {
      publicGroups.push(g)
    }
  }
  return { exclusive, publicGroups }
}

// Group filter options: "All Groups" + active exclusive groups (value = group name for fuzzy match)
const groupFilterOptions = computed(() => {
  const options: { value: string; label: string }[] = [
    { value: '', label: t('admin.users.allAuthorizedGroups') }
  ]
  for (const g of allGroups.value) {
    if (g.status !== 'active' || !g.is_exclusive || g.subscription_type !== 'standard') continue
    options.push({ value: g.name, label: g.name })
  }
  return options
})

// API Key group filter options: "All" + groups partitioned by type (value = group id).
// Uses allGroupsForApiKeyFilter which includes disabled groups.
const apiKeyGroupFilterOptions = computed(() =>
  buildApiKeyGroupFilterOptions(allGroupsForApiKeyFilter.value, {
    all: t('admin.users.allApiKeyGroups'),
    exclusive: t('admin.users.apiKeyGroupExclusive'),
    public: t('admin.users.apiKeyGroupPublic'),
    subscription: t('admin.users.apiKeyGroupSubscription'),
    disabled: t('admin.users.apiKeyGroupDisabled'),
  }) as SelectOption[]
)

// Filter values (role, status, and custom attributes)
const filters = reactive({
  role: '',
  status: '',
  group: '',  // group name for fuzzy match, '' = all
  apiKeyGroup: null as number | null  // group id bound to the user's API keys, null = all
})
const activeAttributeFilters = reactive<Record<number, string>>({})

// Visible filters tracking (which filters are shown in the UI)
// Keys: 'role', 'status', 'attr_${id}'
const visibleFilters = reactive<Set<string>>(new Set())

// Dropdown states
const showFilterDropdown = ref(false)
const showColumnDropdown = ref(false)

// Dropdown refs for click outside detection
const filterDropdownRef = ref<HTMLElement | null>(null)
const columnDropdownRef = ref<HTMLElement | null>(null)

// localStorage keys
const FILTER_VALUES_KEY = 'user-filter-values'
const VISIBLE_FILTERS_KEY = 'user-visible-filters'

// All filterable attribute definitions (enabled attributes)
const filterableAttributes = computed(() =>
  attributeDefinitions.value.filter(def => def.enabled)
)

// Built-in filter definitions
const builtInFilters = computed(() => [
  { key: 'role', name: t('admin.users.columns.role'), type: 'select' as const },
  { key: 'status', name: t('admin.users.columns.status'), type: 'select' as const },
  { key: 'group', name: t('admin.users.authorizedGroupFilter'), type: 'select' as const },
  { key: 'apiKeyGroup', name: t('admin.users.apiKeyGroupFilter'), type: 'select' as const }
])

// Load saved filters from localStorage
const loadSavedFilters = () => {
  try {
    // Load visible filters
    const savedVisible = localStorage.getItem(VISIBLE_FILTERS_KEY)
    if (savedVisible) {
      const parsed = JSON.parse(savedVisible) as string[]
      parsed.forEach(key => visibleFilters.add(key))
    }
    // Load filter values
    const savedValues = localStorage.getItem(FILTER_VALUES_KEY)
    if (savedValues) {
      const parsed = JSON.parse(savedValues)
      if (parsed.role) filters.role = parsed.role
      if (parsed.status) filters.status = parsed.status
      if (parsed.group) filters.group = parsed.group
      if (typeof parsed.apiKeyGroup === 'number') filters.apiKeyGroup = parsed.apiKeyGroup
      if (parsed.attributes) {
        Object.assign(activeAttributeFilters, parsed.attributes)
      }
    }
  } catch (e) {
    console.error('Failed to load saved filters:', e)
  }
}

// Save filters to localStorage
const saveFiltersToStorage = () => {
  try {
    // Save visible filters
    localStorage.setItem(VISIBLE_FILTERS_KEY, JSON.stringify([...visibleFilters]))
    // Save filter values
    const values = {
      role: filters.role,
      status: filters.status,
      group: filters.group,
      apiKeyGroup: filters.apiKeyGroup,
      attributes: activeAttributeFilters
    }
    localStorage.setItem(FILTER_VALUES_KEY, JSON.stringify(values))
  } catch (e) {
    console.error('Failed to save filters:', e)
  }
}

// Get attribute definition by ID
const getAttributeDefinition = (attrId: number): UserAttributeDefinition | undefined => {
  return attributeDefinitions.value.find(d => d.id === attrId)
}
const usageStats = ref<Record<string, BatchUserUsageStats>>({})
const platformQuotaStats = ref<Record<number, PlatformQuotaItem[]>>({})

const getPlatformUsage = (userId: number, platform: string) =>
  usageStats.value[userId]?.by_platform?.find((p) => p.platform === platform)

// 用量列前端排序：DataTable 工作在 server-side-sort 模式，所有 sortable
// 字段都会触发后端查询，而用量列数据是异步批量拉取后再合并到当前页，
// 因此采用独立的前端排序状态对当前页 users 做本地排序。
// 排序状态独立于后端 sortState 持久化；缺失数据按 0 处理（desc 沉底、asc 置顶）。
type UsageMetric = 'today' | 'total'
type UsageSortState = { key: string; metric: UsageMetric; order: 'asc' | 'desc' } | null
const USAGE_SORT_STORAGE_KEY = 'admin-users-usage-sort'

const loadInitialUsageSort = (): UsageSortState => {
  try {
    const raw = localStorage.getItem(USAGE_SORT_STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<{ key: string; metric: string; order: string }>
    if (!parsed.key || !USAGE_COLUMN_KEYS.includes(parsed.key)) return null
    const metric: UsageMetric = parsed.metric === 'total' ? 'total' : 'today'
    const order: 'asc' | 'desc' = parsed.order === 'asc' ? 'asc' : 'desc'
    return { key: parsed.key, metric, order }
  } catch {
    return null
  }
}
const usageSort = ref<UsageSortState>(loadInitialUsageSort())
const persistUsageSort = () => {
  try {
    if (usageSort.value) {
      localStorage.setItem(USAGE_SORT_STORAGE_KEY, JSON.stringify(usageSort.value))
    } else {
      localStorage.removeItem(USAGE_SORT_STORAGE_KEY)
    }
  } catch (e) {
    console.error('Failed to persist usage sort:', e)
  }
}

const isUsageSortActive = (key: string, metric: UsageMetric) =>
  !!usageSort.value && usageSort.value.key === key && usageSort.value.metric === metric
const getUsageSortOrder = (key: string, metric: UsageMetric): 'asc' | 'desc' | null =>
  isUsageSortActive(key, metric) ? usageSort.value!.order : null

// 三态循环：desc → asc → off。选完即关闭菜单（用户大多希望"选中即应用"，
// 想再切换 order 时重新打开菜单点同一项即可）。
const toggleUsageSort = (key: string, metric: UsageMetric) => {
  const cur = usageSort.value
  if (cur && cur.key === key && cur.metric === metric) {
    usageSort.value = cur.order === 'desc' ? { key, metric, order: 'asc' } : null
  } else {
    usageSort.value = { key, metric, order: 'desc' }
  }
  persistUsageSort()
  openUsageSortMenu.value = null
}

// 列头排序按钮点击后弹出的"今日/近30天"选择菜单，同时只允许一个列展开。
// 点击图标本身不触发排序，仅开关菜单；首次排序由用户在菜单内选择 metric 触发（默认 desc，详见 toggleUsageSort）。
const openUsageSortMenu = ref<string | null>(null)
const toggleUsageSortMenu = (key: string) => {
  openUsageSortMenu.value = openUsageSortMenu.value === key ? null : key
}

const getUsageValue = (userId: number, key: string, metric: UsageMetric): number => {
  const stats = usageStats.value[userId]
  if (!stats) return 0
  const platform = USAGE_COLUMN_PLATFORMS[key]
  if (platform === null) {
    return metric === 'today' ? stats.today_actual_cost ?? 0 : stats.total_actual_cost ?? 0
  }
  const p = stats.by_platform?.find((x) => x.platform === platform)
  if (!p) return 0
  return metric === 'today' ? p.today_actual_cost ?? 0 : p.total_actual_cost ?? 0
}

// 在 server-side 排序结果之上叠加用量列的本地排序；无 usageSort 时直接透传原数组。
// 稳定排序：等值按原 index 保序，避免拉取新用量数据时表行抖动。
const sortedUsers = computed(() => {
  const s = usageSort.value
  if (!s) return users.value
  return [...users.value]
    .map((row, index) => ({ row, index }))
    .sort((a, b) => {
      const av = getUsageValue(a.row.id, s.key, s.metric)
      const bv = getUsageValue(b.row.id, s.key, s.metric)
      if (av !== bv) return s.order === 'asc' ? av - bv : bv - av
      return a.index - b.index
    })
    .map((x) => x.row)
})

// User attribute definitions and values
const attributeDefinitions = ref<UserAttributeDefinition[]>([])
const userAttributeValues = ref<Record<number, Record<number, string>>>({})
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0,
  pages: 0
})

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showApiKeysModal = ref(false)
const showAttributesModal = ref(false)
const showPlatformQuotaModal = ref(false)
const editingUser = ref<AdminUser | null>(null)
const deletingUser = ref<AdminUser | null>(null)
const viewingUser = ref<AdminUser | null>(null)
const platformQuotaUser = ref<AdminUser | null>(null)

const handlePlatformQuota = (user: AdminUser) => {
  platformQuotaUser.value = user
  showPlatformQuotaModal.value = true
}

const closePlatformQuotaModal = () => {
  showPlatformQuotaModal.value = false
  platformQuotaUser.value = null
}
let abortController: AbortController | null = null
let secondaryDataSeq = 0

const loadUsersSecondaryData = async (
  userIds: number[],
  signal?: AbortSignal,
  expectedSeq?: number
) => {
  if (userIds.length === 0) return

  const tasks: Promise<void>[] = []

  if (hasVisibleUsageColumn.value) {
    tasks.push(
      (async () => {
        try {
          const usageResponse = await adminAPI.dashboard.getBatchUsersUsage(userIds)
          if (signal?.aborted) return
          if (typeof expectedSeq === 'number' && expectedSeq !== secondaryDataSeq) return
          usageStats.value = usageResponse.stats
        } catch (e) {
          if (signal?.aborted) return
          console.error('Failed to load usage stats:', e)
        }
      })()
    )
  }

  if (attributeDefinitions.value.length > 0 && hasVisibleAttributeColumns.value) {
    tasks.push(
      (async () => {
        try {
          const attrResponse = await adminAPI.userAttributes.getBatchUserAttributes(userIds)
          if (signal?.aborted) return
          if (typeof expectedSeq === 'number' && expectedSeq !== secondaryDataSeq) return
          userAttributeValues.value = attrResponse.attributes
        } catch (e) {
          if (signal?.aborted) return
          console.error('Failed to load user attribute values:', e)
        }
      })()
    )
  }

  if (hasVisiblePlatformQuotaColumn.value) {
    tasks.push(
      (async () => {
        try {
          // 无批量端点：对当前页用户逐个拉取，分块并发（每批 6），批间检查中止条件，避免大 pageSize 时请求洪峰
          const CHUNK = 6
          for (let i = 0; i < userIds.length; i += CHUNK) {
            if (signal?.aborted) return
            if (typeof expectedSeq === 'number' && expectedSeq !== secondaryDataSeq) return
            const chunk = userIds.slice(i, i + CHUNK)
            const results = await Promise.allSettled(
              chunk.map((id) => adminAPI.users.getPlatformQuotas(id))
            )
            if (signal?.aborted) return
            if (typeof expectedSeq === 'number' && expectedSeq !== secondaryDataSeq) return
            const merged = { ...platformQuotaStats.value }
            results.forEach((r, idx) => {
              if (r.status === 'fulfilled') {
                merged[chunk[idx]] = r.value.platform_quotas || []
              }
            })
            platformQuotaStats.value = merged
          }
        } catch (e) {
          if (signal?.aborted) return
          console.error('Failed to load platform quotas:', e)
        }
      })()
    )
  }

  if (tasks.length > 0) {
    await Promise.allSettled(tasks)
  }
}

const refreshCurrentPageSecondaryData = () => {
  const userIds = users.value.map((u) => u.id)
  if (userIds.length === 0) return
  const seq = ++secondaryDataSeq
  void loadUsersSecondaryData(userIds, undefined, seq)
}

// Action Menu State
const activeMenuId = ref<number | null>(null)
const menuPosition = ref<{ top: number; left: number } | null>(null)

const openActionMenu = (user: AdminUser, e: MouseEvent) => {
  if (activeMenuId.value === user.id) {
    closeActionMenu()
  } else {
    const target = e.currentTarget as HTMLElement
    if (!target) {
      closeActionMenu()
      return
    }

    const rect = target.getBoundingClientRect()
    const menuWidth = 200
    const menuHeight = 240
    const padding = 8
    const viewportWidth = window.innerWidth
    const viewportHeight = window.innerHeight

    let left, top

    if (viewportWidth < 768) {
      // 居中显示,水平位置
      left = Math.max(padding, Math.min(
        rect.left + rect.width / 2 - menuWidth / 2,
        viewportWidth - menuWidth - padding
      ))

      // 优先显示在按钮下方
      top = rect.bottom + 4

      // 如果下方空间不够,显示在上方
      if (top + menuHeight > viewportHeight - padding) {
        top = rect.top - menuHeight - 4
        // 如果上方也不够,就贴在视口顶部
        if (top < padding) {
          top = padding
        }
      }
    } else {
      left = Math.max(padding, Math.min(
        e.clientX - menuWidth,
        viewportWidth - menuWidth - padding
      ))
      top = e.clientY
      if (top + menuHeight > viewportHeight - padding) {
        top = viewportHeight - menuHeight - padding
      }
    }

    menuPosition.value = { top, left }
    activeMenuId.value = user.id
  }
}

const closeActionMenu = () => {
  activeMenuId.value = null
  menuPosition.value = null
}

// Close menu when clicking outside
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.action-menu-trigger') && !target.closest('.action-menu-content')) {
    closeActionMenu()
  }
  // Close filter dropdown when clicking outside
  if (filterDropdownRef.value && !filterDropdownRef.value.contains(target)) {
    showFilterDropdown.value = false
  }
  // Close column dropdown when clicking outside
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
  // Close usage sort dropdown when clicking outside any usage-sort-trigger
  if (openUsageSortMenu.value !== null && !target.closest('.usage-sort-trigger')) {
    openUsageSortMenu.value = null
  }
  // Close expanded group dropdown when clicking outside
  if (expandedGroupUserId.value !== null) {
    expandedGroupUserId.value = null
  }
}

// Allowed groups modal state
const showAllowedGroupsModal = ref(false)
const allowedGroupsUser = ref<AdminUser | null>(null)

// Expanded group dropdown state (click to show exclusive groups list)
const expandedGroupUserId = ref<number | null>(null)
const toggleExpandedGroup = (userId: number) => {
  expandedGroupUserId.value = expandedGroupUserId.value === userId ? null : userId
}

// Group replace modal state
const showGroupReplaceModal = ref(false)
const groupReplaceUser = ref<AdminUser | null>(null)
const groupReplaceOldGroup = ref<{ id: number; name: string } | null>(null)

// Balance (Deposit/Withdraw) modal state
const showBalanceModal = ref(false)
const balanceUser = ref<AdminUser | null>(null)
const balanceOperation = ref<'add' | 'subtract'>('add')

// Balance History modal state
const showBalanceHistoryModal = ref(false)
const balanceHistoryUser = ref<AdminUser | null>(null)

// 计算剩余天数
const getDaysRemaining = (expiresAt: string): number => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diffMs = expires.getTime() - now.getTime()
  return Math.ceil(diffMs / (1000 * 60 * 60 * 24))
}

const loadAttributeDefinitions = async () => {
  try {
    attributeDefinitions.value = await adminAPI.userAttributes.listEnabledDefinitions()
  } catch (e) {
    console.error('Failed to load attribute definitions:', e)
  }
}

// Handle attributes modal close - reload definitions and users
const handleAttributesModalClose = async () => {
  showAttributesModal.value = false
  await loadAttributeDefinitions()
  loadUsers()
}

const loadUsers = async () => {
  abortController?.abort()
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  const { signal } = currentAbortController
  loading.value = true
  try {
    // Build attribute filters from active filters
    const attrFilters: Record<number, string> = {}
    for (const [attrId, value] of Object.entries(activeAttributeFilters)) {
      if (value) {
        attrFilters[Number(attrId)] = value
      }
    }

    const response = await adminAPI.users.list(
      pagination.page,
      pagination.page_size,
      {
        role: filters.role as any,
        status: filters.status as any,
        search: searchQuery.value || undefined,
        group_name: filters.group || undefined,
        api_key_group_id: filters.apiKeyGroup ?? undefined,
        attributes: Object.keys(attrFilters).length > 0 ? attrFilters : undefined,
        // 始终请求 subscriptions：列隐藏时仍需用于 UserPlatformQuotaModal 的 active-subscription 警示 banner
        include_subscriptions: true,
        sort_by: sortState.sort_by,
        sort_order: sortState.sort_order
      },
      { signal }
    )
    if (signal.aborted) {
      return
    }
    users.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
    usageStats.value = {}
    userAttributeValues.value = {}
    platformQuotaStats.value = {}

    // Defer heavy secondary data so table can render first.
    if (response.items.length > 0) {
      const userIds = response.items.map((u) => u.id)
      const seq = ++secondaryDataSeq
      window.setTimeout(() => {
        if (signal.aborted || seq !== secondaryDataSeq) return
        void loadUsersSecondaryData(userIds, signal, seq)
      }, 50)
    }
  } catch (error: any) {
    const errorInfo = error as { name?: string; code?: string }
    if (errorInfo?.name === 'AbortError' || errorInfo?.name === 'CanceledError' || errorInfo?.code === 'ERR_CANCELED') {
      return
    }
    const message = error.response?.data?.detail || error.message || t('admin.users.failedToLoad')
    appStore.showError(message)
    console.error('Error loading users:', error)
  } finally {
    if (abortController === currentAbortController) {
      loading.value = false
    }
  }
}

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadUsers()
  }, 300)
}

const handlePageChange = (page: number) => {
  // 确保页码在有效范围内
  const validPage = Math.max(1, Math.min(page, pagination.pages || 1))
  pagination.page = validPage
  loadUsers()
}

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadUsers()
}

const handleSort = (key: string, order: 'asc' | 'desc') => {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadUsers()
}

// Filter helpers
const getAttributeDefinitionName = (attrId: number): string => {
  const def = attributeDefinitions.value.find(d => d.id === attrId)
  return def?.name || String(attrId)
}

// Toggle a built-in filter (role/status)
const toggleBuiltInFilter = (key: string) => {
  if (visibleFilters.has(key)) {
    visibleFilters.delete(key)
    if (key === 'role') filters.role = ''
    if (key === 'status') filters.status = ''
    if (key === 'group') filters.group = ''
    if (key === 'apiKeyGroup') filters.apiKeyGroup = null
  } else {
    visibleFilters.add(key)
    if (key === 'group') loadAllGroups()
    if (key === 'apiKeyGroup') loadAllGroupsForApiKeyFilter()
  }
  saveFiltersToStorage()
  pagination.page = 1
  loadUsers()
}

// Toggle a custom attribute filter
const toggleAttributeFilter = (attr: UserAttributeDefinition) => {
  const key = `attr_${attr.id}`
  if (visibleFilters.has(key)) {
    visibleFilters.delete(key)
    delete activeAttributeFilters[attr.id]
  } else {
    visibleFilters.add(key)
    activeAttributeFilters[attr.id] = ''
  }
  saveFiltersToStorage()
  pagination.page = 1
  loadUsers()
}

const updateAttributeFilter = (attrId: number, value: string) => {
  activeAttributeFilters[attrId] = value
}

// Apply filter and save to localStorage
const applyFilter = () => {
  saveFiltersToStorage()
  loadUsers()
}

const handleEdit = (user: AdminUser) => {
  editingUser.value = user
  showEditModal.value = true
}

const closeEditModal = () => {
  showEditModal.value = false
  editingUser.value = null
}

const handleToggleStatus = async (user: AdminUser) => {
  const newStatus = user.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.users.toggleStatus(user.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('admin.users.userEnabled') : t('admin.users.userDisabled')
    )
    loadUsers()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToToggle'))
    console.error('Error toggling user status:', error)
  }
}

const handleViewApiKeys = (user: AdminUser) => {
  viewingUser.value = user
  showApiKeysModal.value = true
}

const closeApiKeysModal = () => {
  showApiKeysModal.value = false
  viewingUser.value = null
}

const handleAllowedGroups = (user: AdminUser) => {
  allowedGroupsUser.value = user
  showAllowedGroupsModal.value = true
}

const closeAllowedGroupsModal = () => {
  showAllowedGroupsModal.value = false
  allowedGroupsUser.value = null
}

const openGroupReplace = (user: AdminUser, group: { id: number; name: string }) => {
  expandedGroupUserId.value = null
  groupReplaceUser.value = user
  groupReplaceOldGroup.value = group
  showGroupReplaceModal.value = true
}

const closeGroupReplaceModal = () => {
  showGroupReplaceModal.value = false
  groupReplaceUser.value = null
  groupReplaceOldGroup.value = null
}

const handleDelete = (user: AdminUser) => {
  deletingUser.value = user
  showDeleteDialog.value = true
}

const confirmDelete = async () => {
  if (!deletingUser.value) return
  try {
    await adminAPI.users.delete(deletingUser.value.id)
    appStore.showSuccess(t('common.success'))
    showDeleteDialog.value = false
    deletingUser.value = null
    loadUsers()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToDelete'))
    console.error('Error deleting user:', error)
  }
}

const handleDeposit = (user: AdminUser) => {
  balanceUser.value = user
  balanceOperation.value = 'add'
  showBalanceModal.value = true
}

const handleWithdraw = (user: AdminUser) => {
  balanceUser.value = user
  balanceOperation.value = 'subtract'
  showBalanceModal.value = true
}

const closeBalanceModal = () => {
  showBalanceModal.value = false
  balanceUser.value = null
}

const handleBalanceHistory = (user: AdminUser) => {
  balanceHistoryUser.value = user
  showBalanceHistoryModal.value = true
}

const closeBalanceHistoryModal = () => {
  showBalanceHistoryModal.value = false
  balanceHistoryUser.value = null
}

// Handle deposit from balance history modal
const handleDepositFromHistory = () => {
  if (balanceHistoryUser.value) {
    handleDeposit(balanceHistoryUser.value)
  }
}

// Handle withdraw from balance history modal
const handleWithdrawFromHistory = () => {
  if (balanceHistoryUser.value) {
    handleWithdraw(balanceHistoryUser.value)
  }
}

// 滚动时关闭菜单
const handleScroll = () => {
  closeActionMenu()
}

onMounted(async () => {
  await loadAttributeDefinitions()
  loadSavedFilters()
  loadSavedColumns()
  loadUsers()
  if (hasVisibleGroupsColumn.value || visibleFilters.has('group')) {
    loadAllGroups()
  }
  if (visibleFilters.has('apiKeyGroup')) {
    loadAllGroupsForApiKeyFilter()
  }
  document.addEventListener('click', handleClickOutside)
  window.addEventListener('scroll', handleScroll, true)
})

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  window.removeEventListener('scroll', handleScroll, true)
  clearTimeout(searchTimeout)
  abortController?.abort()
})
</script>
