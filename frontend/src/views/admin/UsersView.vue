<template>
  <AppLayout>
    <TablePageLayout>
      <!-- Single Row: Search, Filters, and Actions -->
      <template #filters>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <!-- Left: Search + Active Filters -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <!-- Search Box -->
            <div class="relative w-64">
              <svg
                class="absolute left-3 top-1/2 h-5 w-5 -translate-y-1/2 text-gray-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z"
                />
              </svg>
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.users.searchUsers')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <!-- Role Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('role')" class="relative">
              <select
                v-model="filters.role"
                @change="applyFilter"
                class="input w-32 cursor-pointer appearance-none pr-8"
              >
                <option value="">{{ t('admin.users.allRoles') REDACTEDREDACTED</option>
                <option value="admin">{{ t('admin.users.admin') REDACTEDREDACTED</option>
                <option value="user">{{ t('admin.users.user') REDACTEDREDACTED</option>
              </select>
              <svg
                class="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
              </svg>
            </div>

            <!-- Status Filter (visible when enabled) -->
            <div v-if="visibleFilters.has('status')" class="relative">
              <select
                v-model="filters.status"
                @change="applyFilter"
                class="input w-32 cursor-pointer appearance-none pr-8"
              >
                <option value="">{{ t('admin.users.allStatus') REDACTEDREDACTED</option>
                <option value="active">{{ t('common.active') REDACTEDREDACTED</option>
                <option value="disabled">{{ t('admin.users.disabled') REDACTEDREDACTED</option>
              </select>
              <svg
                class="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
              </svg>
            </div>

            <!-- Dynamic Attribute Filters -->
            <template v-for="(value, attrId) in activeAttributeFilters" :key="attrId">
              <div v-if="visibleFilters.has(`attr_${attrIdREDACTED`)" class="relative">
                <!-- Text/Email/URL/Textarea/Date type: styled input -->
                <input
                  v-if="['text', 'textarea', 'email', 'url', 'date'].includes(getAttributeDefinition(Number(attrId))?.type || 'text')"
                  :value="value"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-36"
                />
                <!-- Number type: number input -->
                <input
                  v-else-if="getAttributeDefinition(Number(attrId))?.type === 'number'"
                  :value="value"
                  type="number"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-32"
                />
                <!-- Select/Multi-select type -->
                <template v-else-if="['select', 'multi_select'].includes(getAttributeDefinition(Number(attrId))?.type || '')">
                  <select
                    :value="value"
                    @change="(e) => { updateAttributeFilter(Number(attrId), (e.target as HTMLSelectElement).value); applyFilter() REDACTED"
                    class="input w-36 cursor-pointer appearance-none pr-8"
                  >
                    <option value="">{{ getAttributeDefinitionName(Number(attrId)) REDACTEDREDACTED</option>
                    <option
                      v-for="opt in getAttributeDefinition(Number(attrId))?.options || []"
                      :key="opt.value"
                      :value="opt.value"
                    >
                      {{ opt.label REDACTEDREDACTED
                    </option>
                  </select>
                  <svg
                    class="pointer-events-none absolute right-2.5 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-400"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    stroke-width="1.5"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
                  </svg>
                </template>
                <!-- Fallback -->
                <input
                  v-else
                  :value="value"
                  @input="(e) => updateAttributeFilter(Number(attrId), (e.target as HTMLInputElement).value)"
                  @keyup.enter="applyFilter"
                  :placeholder="getAttributeDefinitionName(Number(attrId))"
                  class="input w-36"
                />
              </div>
            </template>
          </div>

          <!-- Right: Actions and Settings -->
          <div class="flex items-center gap-3">
            <!-- Refresh Button -->
            <button
              @click="loadUsers"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <svg
                :class="['h-5 w-5', loading ? 'animate-spin' : '']"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99"
                />
              </svg>
            </button>
            <!-- Filter Settings Dropdown -->
            <div class="relative" ref="filterDropdownRef">
              <button
                @click="showFilterDropdown = !showFilterDropdown"
                class="btn btn-secondary"
              >
                <svg class="mr-1.5 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M12 3c2.755 0 5.455.232 8.083.678.533.09.917.556.917 1.096v1.044a2.25 2.25 0 01-.659 1.591l-5.432 5.432a2.25 2.25 0 00-.659 1.591v2.927a2.25 2.25 0 01-1.244 2.013L9.75 21v-6.568a2.25 2.25 0 00-.659-1.591L3.659 7.409A2.25 2.25 0 013 5.818V4.774c0-.54.384-1.006.917-1.096A48.32 48.32 0 0112 3z" />
                </svg>
                {{ t('admin.users.filterSettings') REDACTEDREDACTED
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
                  <span>{{ filter.name REDACTEDREDACTED</span>
                  <svg
                    v-if="visibleFilters.has(filter.key)"
                    class="h-4 w-4 text-primary-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
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
                  <span>{{ attr.name REDACTEDREDACTED</span>
                  <svg
                    v-if="visibleFilters.has(`attr_${attr.idREDACTED`)"
                    class="h-4 w-4 text-primary-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                </button>
              </div>
            </div>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary"
              >
                <svg class="mr-1.5 h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                {{ t('admin.users.columnSettings') REDACTEDREDACTED
              </button>
              <!-- Dropdown menu -->
              <div
                v-if="showColumnDropdown"
                class="absolute right-0 top-full z-50 mt-1 max-h-80 w-48 overflow-y-auto rounded-lg border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800"
              >
                <button
                  v-for="col in toggleableColumns"
                  :key="col.key"
                  @click="toggleColumn(col.key)"
                  class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
                >
                  <span>{{ col.label REDACTEDREDACTED</span>
                  <svg
                    v-if="isColumnVisible(col.key)"
                    class="h-4 w-4 text-primary-500"
                    fill="none"
                    stroke="currentColor"
                    viewBox="0 0 24 24"
                    stroke-width="2"
                  >
                    <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                  </svg>
                </button>
              </div>
            </div>
            <!-- Attributes Config Button -->
            <button @click="showAttributesModal = true" class="btn btn-secondary">
              <svg
                class="mr-1.5 h-4 w-4"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.594 3.94c.09-.542.56-.94 1.11-.94h2.593c.55 0 1.02.398 1.11.94l.213 1.281c.063.374.313.686.645.87.074.04.147.083.22.127.324.196.72.257 1.075.124l1.217-.456a1.125 1.125 0 011.37.49l1.296 2.247a1.125 1.125 0 01-.26 1.431l-1.003.827c-.293.24-.438.613-.431.992a6.759 6.759 0 010 .255c-.007.378.138.75.43.99l1.005.828c.424.35.534.954.26 1.43l-1.298 2.247a1.125 1.125 0 01-1.369.491l-1.217-.456c-.355-.133-.75-.072-1.076.124a6.57 6.57 0 01-.22.128c-.331.183-.581.495-.644.869l-.213 1.28c-.09.543-.56.941-1.11.941h-2.594c-.55 0-1.02-.398-1.11-.94l-.213-1.281c-.062-.374-.312-.686-.644-.87a6.52 6.52 0 01-.22-.127c-.325-.196-.72-.257-1.076-.124l-1.217.456a1.125 1.125 0 01-1.369-.49l-1.297-2.247a1.125 1.125 0 01.26-1.431l1.004-.827c.292-.24.437-.613.43-.992a6.932 6.932 0 010-.255c.007-.378-.138-.75-.43-.99l-1.004-.828a1.125 1.125 0 01-.26-1.43l1.297-2.247a1.125 1.125 0 011.37-.491l1.216.456c.356.133.751.072 1.076-.124.072-.044.146-.087.22-.128.332-.183.582-.495.644-.869l.214-1.281z" />
                <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
              </svg>
              {{ t('admin.users.attributes.configButton') REDACTEDREDACTED
            </button>
            <!-- Create User Button -->
            <button @click="showCreateModal = true" class="btn btn-primary">
              <svg
                class="mr-2 h-5 w-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
              </svg>
              {{ t('admin.users.createUser') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <!-- Users Table -->
      <template #table>
        <DataTable :columns="columns" :data="users" :loading="loading" :actions-count="7">
          <template #cell-email="{ value REDACTED">
            <div class="flex items-center gap-2">
              <div
                class="flex h-8 w-8 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
              >
                <span class="text-sm font-medium text-primary-700 dark:text-primary-300">
                  {{ value.charAt(0).toUpperCase() REDACTEDREDACTED
                </span>
              </div>
              <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
            </div>
          </template>

          <template #cell-username="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value || '-' REDACTEDREDACTED</span>
          </template>

          <template #cell-notes="{ value REDACTED">
            <div class="max-w-xs">
              <span
                v-if="value"
                :title="value.length > 30 ? value : undefined"
                class="block truncate text-sm text-gray-600 dark:text-gray-400"
              >
                {{ value.length > 30 ? value.substring(0, 25) + '...' : value REDACTEDREDACTED
              </span>
              <span v-else class="text-sm text-gray-400">-</span>
            </div>
          </template>

          <!-- Dynamic attribute columns -->
          <template
            v-for="def in attributeDefinitions.filter(d => d.enabled)"
            :key="def.id"
            #[`cell-attr_${def.idREDACTED`]="{ row REDACTED"
          >
            <div class="max-w-xs">
              <span
                class="block truncate text-sm text-gray-700 dark:text-gray-300"
                :title="getAttributeValue(row.id, def.id)"
              >
                {{ getAttributeValue(row.id, def.id) REDACTEDREDACTED
              </span>
            </div>
          </template>

          <template #cell-role="{ value REDACTED">
            <span :class="['badge', value === 'admin' ? 'badge-purple' : 'badge-gray']">
              {{ value REDACTEDREDACTED
            </span>
          </template>

          <template #cell-subscriptions="{ row REDACTED">
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
              <svg
                class="h-3.5 w-3.5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"
                />
              </svg>
              <span>{{ t('admin.users.noSubscription') REDACTEDREDACTED</span>
            </span>
          </template>

          <template #cell-balance="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">${{ value.toFixed(2) REDACTEDREDACTED</span>
          </template>

          <template #cell-usage="{ row REDACTED">
            <div class="text-sm">
              <div class="flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.today') REDACTEDREDACTED:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.today_actual_cost ?? 0).toFixed(4) REDACTEDREDACTED
                </span>
              </div>
              <div class="mt-0.5 flex items-center gap-1.5">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.users.total') REDACTEDREDACTED:</span>
                <span class="font-medium text-gray-900 dark:text-white">
                  ${{ (usageStats[row.id]?.total_actual_cost ?? 0).toFixed(4) REDACTEDREDACTED
                </span>
              </div>
            </div>
          </template>

          <template #cell-concurrency="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-status="{ value REDACTED">
            <div class="flex items-center gap-1.5">
              <span
                :class="[
                  'inline-block h-2 w-2 rounded-full',
                  value === 'active' ? 'bg-green-500' : 'bg-red-500'
                ]"
              ></span>
              <span class="text-sm text-gray-700 dark:text-gray-300">
                {{ value === 'active' ? t('common.active') : t('admin.users.disabled') REDACTEDREDACTED
              </span>
            </div>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-500 dark:text-dark-400">{{ formatDateTime(value) REDACTEDREDACTED</span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <!-- Edit Button -->
              <button
                @click="handleEdit(row)"
                class="flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.edit')"
              >
                <svg
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10"
                  />
                </svg>
              </button>

              <!-- More Actions Menu Trigger -->
              <button
                :ref="(el) => setActionButtonRef(row.id, el)"
                @click="openActionMenu(row)"
                class="action-menu-trigger flex h-8 w-8 items-center justify-center rounded-lg text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
                :class="{ 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white': activeMenuId === row.id REDACTED"
              >
                <svg
                  class="h-5 w-5"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M6.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM12.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0zM18.75 12a.75.75 0 11-1.5 0 .75.75 0 011.5 0z"
                  />
                </svg>
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
        :style="{ top: menuPosition.top + 'px', left: menuPosition.left + 'px' REDACTED"
      >
        <div class="py-1">
          <template v-for="user in users" :key="user.id">
            <template v-if="user.id === activeMenuId">
              <!-- View API Keys -->
              <button
                @click="handleViewApiKeys(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11.536 16.207l-1.414 1.414a2 2 0 01-2.828 0l-1.414-1.414a2 2 0 010-2.828l-1.414-1.414a2 2 0 010-2.828l1.414-1.414L10.257 6.257A6 6 0 1121 11.257V11.257" />
                </svg>
                {{ t('admin.users.apiKeys') REDACTEDREDACTED
              </button>

              <!-- Allowed Groups -->
              <button
                @click="handleAllowedGroups(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a3 3 0 00-5.356-1.857M17 20H7m10 0v-2c0-.656-.126-1.283-.356-1.857M7 20H2v-2a3 3 0 015.356-1.857M7 20v-2c0-.656.126-1.283.356-1.857m0 0a5.002 5.002 0 019.288 0M15 7a3 3 0 11-6 0 3 3 0 016 0zm6 3a2 2 0 11-4 0 2 2 0 014 0zM7 10a2 2 0 11-4 0 2 2 0 014 0z" />
                </svg>
                {{ t('admin.users.groups') REDACTEDREDACTED
              </button>

              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>

              <!-- Deposit -->
              <button
                @click="handleDeposit(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-emerald-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 6v6m0 0v6m0-6h6m-6 0H6" />
                </svg>
                {{ t('admin.users.deposit') REDACTEDREDACTED
              </button>

              <!-- Withdraw -->
              <button
                @click="handleWithdraw(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg class="h-4 w-4 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12H4" />
                </svg>
                {{ t('admin.users.withdraw') REDACTEDREDACTED
              </button>

              <div class="my-1 border-t border-gray-100 dark:border-dark-700"></div>

              <!-- Toggle Status (not for admin) -->
              <button
                v-if="user.role !== 'admin'"
                @click="handleToggleStatus(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
              >
                <svg
                  v-if="user.status === 'active'"
                  class="h-4 w-4 text-orange-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636" />
                </svg>
                <svg
                  v-else
                  class="h-4 w-4 text-green-500"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                {{ user.status === 'active' ? t('admin.users.disable') : t('admin.users.enable') REDACTEDREDACTED
              </button>

              <!-- Delete (not for admin) -->
              <button
                v-if="user.role !== 'admin'"
                @click="handleDelete(user); closeActionMenu()"
                class="flex w-full items-center gap-2 px-4 py-2 text-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
              >
                <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
                </svg>
                {{ t('common.delete') REDACTEDREDACTED
              </button>
            </template>
          </template>
        </div>
      </div>
    </Teleport>

    <!-- Create User Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.users.createUser')"
      width="normal"
      @close="closeCreateModal"
    >
      <form id="create-user-form" @submit.prevent="handleCreateUser" class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.users.email') REDACTEDREDACTED</label>
          <input
            v-model="createForm.email"
            type="email"
            required
            class="input"
            :placeholder="t('admin.users.enterEmail')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.password') REDACTEDREDACTED</label>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <input
                v-model="createForm.password"
                type="text"
                required
                class="input pr-10"
                :placeholder="t('admin.users.enterPassword')"
              />
              <!-- Copy Password Button -->
              <button
                v-if="createForm.password"
                type="button"
                @click="copyPassword"
                class="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  passwordCopied
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="passwordCopied ? t('keys.copied') : t('admin.users.copyPassword')"
              >
                <svg
                  v-if="passwordCopied"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg
                  v-else
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184"
                  />
                </svg>
              </button>
            </div>
            <!-- Generate Random Password Button -->
            <button
              type="button"
              @click="generateRandomPassword"
              class="btn btn-secondary px-3"
              :title="t('admin.users.generatePassword')"
            >
              <svg
                class="h-5 w-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99"
                />
              </svg>
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.username') REDACTEDREDACTED</label>
          <input
            v-model="createForm.username"
            type="text"
            class="input"
            :placeholder="t('admin.users.enterUsername')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.notes') REDACTEDREDACTED</label>
          <textarea
            v-model="createForm.notes"
            rows="3"
            class="input"
            :placeholder="t('admin.users.enterNotes')"
          ></textarea>
          <p class="input-hint">{{ t('admin.users.notesHint') REDACTEDREDACTED</p>
        </div>
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.users.columns.balance') REDACTEDREDACTED</label>
            <input v-model.number="createForm.balance" type="number" step="any" class="input" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.users.columns.concurrency') REDACTEDREDACTED</label>
            <input v-model.number="createForm.concurrency" type="number" class="input" />
          </div>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeCreateModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="create-user-form"
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
            {{ submitting ? t('admin.users.creating') : t('common.create') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit User Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.users.editUser')"
      width="normal"
      @close="closeEditModal"
    >
      <form
        v-if="editingUser"
        id="edit-user-form"
        @submit.prevent="handleUpdateUser"
        class="space-y-5"
      >
        <div>
          <label class="input-label">{{ t('admin.users.email') REDACTEDREDACTED</label>
          <input v-model="editForm.email" type="email" class="input" />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.password') REDACTEDREDACTED</label>
          <p class="mb-1 text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.users.leaveEmptyToKeep') REDACTEDREDACTED
          </p>
          <div class="flex gap-2">
            <div class="relative flex-1">
              <input
                v-model="editForm.password"
                type="text"
                class="input pr-10"
                :placeholder="t('admin.users.enterNewPassword')"
              />
              <!-- Copy Password Button -->
              <button
                v-if="editForm.password"
                type="button"
                @click="copyEditPassword"
                class="absolute right-2 top-1/2 -translate-y-1/2 rounded-lg p-1 transition-colors hover:bg-gray-100 dark:hover:bg-dark-700"
                :class="
                  editPasswordCopied
                    ? 'text-green-500'
                    : 'text-gray-400 hover:text-gray-600 dark:hover:text-gray-300'
                "
                :title="editPasswordCopied ? t('keys.copied') : t('admin.users.copyPassword')"
              >
                <svg
                  v-if="editPasswordCopied"
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                </svg>
                <svg
                  v-else
                  class="h-4 w-4"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184"
                  />
                </svg>
              </button>
            </div>
            <!-- Generate Random Password Button -->
            <button
              type="button"
              @click="generateEditPassword"
              class="btn btn-secondary px-3"
              :title="t('admin.users.generatePassword')"
            >
              <svg
                class="h-5 w-5"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99"
                />
              </svg>
            </button>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.username') REDACTEDREDACTED</label>
          <input
            v-model="editForm.username"
            type="text"
            class="input"
            :placeholder="t('admin.users.enterUsername')"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.notes') REDACTEDREDACTED</label>
          <textarea
            v-model="editForm.notes"
            rows="3"
            class="input"
            :placeholder="t('admin.users.enterNotes')"
          ></textarea>
          <p class="input-hint">{{ t('admin.users.notesHint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.users.columns.concurrency') REDACTEDREDACTED</label>
          <input v-model.number="editForm.concurrency" type="number" class="input" />
        </div>

        <!-- Custom Attributes -->
        <UserAttributeForm
          v-model="editForm.customAttributes"
          :user-id="editingUser?.id"
        />

      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeEditModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="edit-user-form"
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
            {{ submitting ? t('admin.users.updating') : t('common.update') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- View API Keys Modal -->
    <BaseDialog
      :show="showApiKeysModal"
      :title="t('admin.users.userApiKeys')"
      width="wide"
      @close="closeApiKeysModal"
    >
      <div v-if="viewingUser" class="space-y-4">
        <!-- User Info Header -->
        <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
          >
            <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
              {{ viewingUser.email.charAt(0).toUpperCase() REDACTEDREDACTED
            </span>
          </div>
          <div>
            <p class="font-medium text-gray-900 dark:text-white">{{ viewingUser.email REDACTEDREDACTED</p>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ viewingUser.username REDACTEDREDACTED</p>
          </div>
        </div>

        <!-- API Keys List -->
        <div v-if="loadingApiKeys" class="flex justify-center py-8">
          <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
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
        </div>
        <div v-else-if="userApiKeys.length === 0" class="py-8 text-center">
          <svg
            class="mx-auto h-12 w-12 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            stroke-width="1"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z"
            />
          </svg>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            {{ t('admin.users.noApiKeys') REDACTEDREDACTED
          </p>
        </div>
        <div v-else class="max-h-96 space-y-3 overflow-y-auto">
          <div
            v-for="key in userApiKeys"
            :key="key.id"
            class="rounded-xl border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800"
          >
            <div class="flex items-start justify-between">
              <div class="min-w-0 flex-1">
                <div class="mb-1 flex items-center gap-2">
                  <span class="font-medium text-gray-900 dark:text-white">{{ key.name REDACTEDREDACTED</span>
                  <span
                    :class="[
                      'badge text-xs',
                      key.status === 'active' ? 'badge-success' : 'badge-danger'
                    ]"
                  >
                    {{ key.status REDACTEDREDACTED
                  </span>
                </div>
                <p class="truncate font-mono text-sm text-gray-500 dark:text-dark-400">
                  {{ key.key.substring(0, 20) REDACTEDREDACTED...{{ key.key.substring(key.key.length - 8) REDACTEDREDACTED
                </p>
              </div>
            </div>
            <div class="mt-3 flex flex-wrap gap-4 text-xs text-gray-500 dark:text-dark-400">
              <div class="flex items-center gap-1">
                <svg
                  class="h-3.5 w-3.5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                  />
                </svg>
                <span
                  >{{ t('admin.users.group') REDACTEDREDACTED:
                  {{ key.group?.name || t('admin.users.none') REDACTEDREDACTED</span
                >
              </div>
              <div class="flex items-center gap-1">
                <svg
                  class="h-3.5 w-3.5"
                  fill="none"
                  stroke="currentColor"
                  viewBox="0 0 24 24"
                  stroke-width="1.5"
                >
                  <path
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    d="M6.75 3v2.25M17.25 3v2.25M3 18.75V7.5a2.25 2.25 0 012.25-2.25h13.5A2.25 2.25 0 0121 7.5v11.25m-18 0A2.25 2.25 0 005.25 21h13.5A2.25 2.25 0 0021 18.75m-18 0v-7.5A2.25 2.25 0 015.25 9h13.5A2.25 2.25 0 0121 11.25v7.5"
                  />
                </svg>
                <span
                  >{{ t('admin.users.columns.created') REDACTEDREDACTED: {{ formatDateTime(key.created_at) REDACTEDREDACTED</span
                >
              </div>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button @click="closeApiKeysModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Allowed Groups Modal -->
    <BaseDialog
      :show="showAllowedGroupsModal"
      :title="t('admin.users.setAllowedGroups')"
      width="normal"
      @close="closeAllowedGroupsModal"
    >
      <div v-if="allowedGroupsUser" class="space-y-4">
        <!-- User Info Header -->
        <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
          >
            <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
              {{ allowedGroupsUser.email.charAt(0).toUpperCase() REDACTEDREDACTED
            </span>
          </div>
          <div>
            <p class="font-medium text-gray-900 dark:text-white">{{ allowedGroupsUser.email REDACTEDREDACTED</p>
          </div>
        </div>

        <!-- Loading State -->
        <div v-if="loadingGroups" class="flex justify-center py-8">
          <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
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
        </div>

        <!-- Groups Selection -->
        <div v-else>
          <p class="mb-3 text-sm text-gray-600 dark:text-dark-400">
            {{ t('admin.users.allowedGroupsHint') REDACTEDREDACTED
          </p>

          <!-- Empty State -->
          <div v-if="standardGroups.length === 0" class="py-6 text-center">
            <svg
              class="mx-auto h-12 w-12 text-gray-400"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              stroke-width="1"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
              />
            </svg>
            <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.users.noStandardGroups') REDACTEDREDACTED
            </p>
          </div>

          <!-- Groups List -->
          <div v-else class="max-h-64 space-y-2 overflow-y-auto">
            <label
              v-for="group in standardGroups"
              :key="group.id"
              class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 p-3 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
              :class="{
                'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20':
                  selectedGroupIds.includes(group.id)
              REDACTED"
            >
              <input
                type="checkbox"
                :value="group.id"
                v-model="selectedGroupIds"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <div class="min-w-0 flex-1">
                <p class="font-medium text-gray-900 dark:text-white">{{ group.name REDACTEDREDACTED</p>
                <p
                  v-if="group.description"
                  class="truncate text-sm text-gray-500 dark:text-dark-400"
                >
                  {{ group.description REDACTEDREDACTED
                </p>
              </div>
              <div class="flex items-center gap-2">
                <span class="badge badge-gray text-xs">{{ group.platform REDACTEDREDACTED</span>
                <span v-if="group.is_exclusive" class="badge badge-purple text-xs">{{
                  t('admin.groups.exclusive')
                REDACTEDREDACTED</span>
              </div>
            </label>
          </div>

          <!-- Clear Selection -->
          <div class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-600">
            <label
              class="flex cursor-pointer items-center gap-3 rounded-lg border border-gray-200 p-3 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
              :class="{
                'border-green-300 bg-green-50 dark:border-green-700 dark:bg-green-900/20':
                  selectedGroupIds.length === 0
              REDACTED"
            >
              <input
                type="radio"
                :checked="selectedGroupIds.length === 0"
                @change="selectedGroupIds = []"
                class="h-4 w-4 border-gray-300 text-green-600 focus:ring-green-500"
              />
              <div class="flex-1">
                <p class="font-medium text-gray-900 dark:text-white">
                  {{ t('admin.users.allowAllGroups') REDACTEDREDACTED
                </p>
                <p class="text-sm text-gray-500 dark:text-dark-400">
                  {{ t('admin.users.allowAllGroupsHint') REDACTEDREDACTED
                </p>
              </div>
            </label>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeAllowedGroupsModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            @click="handleSaveAllowedGroups"
            :disabled="savingAllowedGroups"
            class="btn btn-primary"
          >
            <svg
              v-if="savingAllowedGroups"
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
            {{ savingAllowedGroups ? t('common.saving') : t('common.save') REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Deposit/Withdraw Modal -->
    <BaseDialog
      :show="showBalanceModal"
      :title="balanceOperation === 'add' ? t('admin.users.deposit') : t('admin.users.withdraw')"
      width="narrow"
      @close="closeBalanceModal"
    >
      <form
        v-if="balanceUser"
        id="balance-form"
        @submit.prevent="handleBalanceSubmit"
        class="space-y-5"
      >
        <div class="flex items-center gap-3 rounded-xl bg-gray-50 p-4 dark:bg-dark-700">
          <div
            class="flex h-10 w-10 items-center justify-center rounded-full bg-primary-100 dark:bg-primary-900/30"
          >
            <span class="text-lg font-medium text-primary-700 dark:text-primary-300">
              {{ balanceUser.email.charAt(0).toUpperCase() REDACTEDREDACTED
            </span>
          </div>
          <div class="flex-1">
            <p class="font-medium text-gray-900 dark:text-white">{{ balanceUser.email REDACTEDREDACTED</p>
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('admin.users.currentBalance') REDACTEDREDACTED: ${{ balanceUser.balance.toFixed(2) REDACTEDREDACTED
            </p>
          </div>
        </div>

        <div>
          <label class="input-label">
            {{
              balanceOperation === 'add'
                ? t('admin.users.depositAmount')
                : t('admin.users.withdrawAmount')
            REDACTEDREDACTED
          </label>
          <div class="relative">
            <div
              class="absolute left-3 top-1/2 -translate-y-1/2 font-medium text-gray-500 dark:text-dark-400"
            >
              $
            </div>
            <input
              v-model.number="balanceForm.amount"
              type="number"
              step="0.01"
              min="0.01"
              required
              class="input pl-8"
              :placeholder="balanceOperation === 'add' ? '10.00' : '5.00'"
            />
          </div>
          <p class="input-hint">
            {{ t('admin.users.amountHint') REDACTEDREDACTED
          </p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.users.notes') REDACTEDREDACTED</label>
          <textarea
            v-model="balanceForm.notes"
            rows="3"
            class="input"
            :placeholder="
              balanceOperation === 'add'
                ? t('admin.users.depositNotesPlaceholder')
                : t('admin.users.withdrawNotesPlaceholder')
            "
          ></textarea>
          <p class="input-hint">{{ t('admin.users.notesOptional') REDACTEDREDACTED</p>
        </div>

        <div
          v-if="balanceForm.amount > 0"
          class="rounded-xl border border-blue-200 bg-blue-50 p-4 dark:border-blue-800/50 dark:bg-blue-900/20"
        >
          <div class="flex items-center justify-between text-sm">
            <span class="text-blue-700 dark:text-blue-300">{{ t('admin.users.newBalance') REDACTEDREDACTED:</span>
            <span class="font-bold text-blue-900 dark:text-blue-100">
              ${{ calculateNewBalance().toFixed(2) REDACTEDREDACTED
            </span>
          </div>
        </div>

        <div
          v-if="balanceOperation === 'subtract' && calculateNewBalance() < 0"
          class="rounded-xl border border-red-200 bg-red-50 p-4 dark:border-red-800/50 dark:bg-red-900/20"
        >
          <div class="flex items-center gap-2 text-sm text-red-700 dark:text-red-300">
            <svg
              class="h-5 w-5 flex-shrink-0"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
              stroke-width="1.5"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M12 9v3.75m-9.303 3.376c-.866 1.5.217 3.374 1.948 3.374h14.71c1.73 0 2.813-1.874 1.948-3.374L13.949 3.378c-.866-1.5-3.032-1.5-3.898 0L2.697 16.126zM12 15.75h.007v.008H12v-.008z"
              />
            </svg>
            <span>{{ t('admin.users.insufficientBalance') REDACTEDREDACTED</span>
          </div>
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeBalanceModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="balance-form"
            :disabled="
              balanceSubmitting ||
              !balanceForm.amount ||
              balanceForm.amount <= 0 ||
              (balanceOperation === 'subtract' && calculateNewBalance() < 0)
            "
            class="btn"
            :class="
              balanceOperation === 'add'
                ? 'bg-emerald-600 text-white hover:bg-emerald-700'
                : 'btn-danger'
            "
          >
            <svg
              v-if="balanceSubmitting"
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
            {{
              balanceSubmitting
                ? balanceOperation === 'add'
                  ? t('admin.users.depositing')
                  : t('admin.users.withdrawing')
                : balanceOperation === 'add'
                  ? t('admin.users.confirmDeposit')
                  : t('admin.users.confirmWithdraw')
            REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation Dialog -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.users.deleteUser')"
      :message="t('admin.users.deleteConfirm', { email: deletingUser?.email REDACTED)"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- User Attributes Config Modal -->
    <UserAttributesConfigModal
      :show="showAttributesModal"
      @close="handleAttributesModalClose"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, type ComponentPublicInstance REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useClipboard REDACTED from '@/composables/useClipboard'
import { formatDateTime REDACTED from '@/utils/format'

const { t REDACTED = useI18n()
import { adminAPI REDACTED from '@/api/admin'
import type { User, ApiKey, Group, UserAttributeValuesMap, UserAttributeDefinition REDACTED from '@/types'
import type { BatchUserUsageStats REDACTED from '@/api/admin/dashboard'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import UserAttributesConfigModal from '@/components/user/UserAttributesConfigModal.vue'
import UserAttributeForm from '@/components/user/UserAttributeForm.vue'

const appStore = useAppStore()
const { copyToClipboard: clipboardCopy REDACTED = useClipboard()

// Generate dynamic attribute columns from enabled definitions
const attributeColumns = computed<Column[]>(() =>
  attributeDefinitions.value
    .filter(def => def.enabled)
    .map(def => ({
      key: `attr_${def.idREDACTED`,
      label: def.name,
      sortable: false
    REDACTED))
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
        REDACTED).join(', ')
      REDACTED
    REDACTED catch {
      return value
    REDACTED
  REDACTED

  if (def.type === 'select' && value && def.options) {
    const opt = def.options.find(o => o.value === value)
    return opt?.label || value
  REDACTED

  return value
REDACTED

// All possible columns (for column settings)
const allColumns = computed<Column[]>(() => [
  { key: 'email', label: t('admin.users.columns.user'), sortable: true REDACTED,
  { key: 'id', label: 'ID', sortable: true REDACTED,
  { key: 'username', label: t('admin.users.columns.username'), sortable: true REDACTED,
  { key: 'notes', label: t('admin.users.columns.notes'), sortable: false REDACTED,
  // Dynamic attribute columns
  ...attributeColumns.value,
  { key: 'role', label: t('admin.users.columns.role'), sortable: true REDACTED,
  { key: 'subscriptions', label: t('admin.users.columns.subscriptions'), sortable: false REDACTED,
  { key: 'balance', label: t('admin.users.columns.balance'), sortable: true REDACTED,
  { key: 'usage', label: t('admin.users.columns.usage'), sortable: false REDACTED,
  { key: 'concurrency', label: t('admin.users.columns.concurrency'), sortable: true REDACTED,
  { key: 'status', label: t('admin.users.columns.status'), sortable: true REDACTED,
  { key: 'created_at', label: t('admin.users.columns.created'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.users.columns.actions'), sortable: false REDACTED
])

// Columns that can be toggled (exclude email and actions which are always visible)
const toggleableColumns = computed(() =>
  allColumns.value.filter(col => col.key !== 'email' && col.key !== 'actions')
)

// Hidden columns (stored in Set - columns NOT in this set are visible)
// This way, new columns are visible by default
const hiddenColumns = reactive<Set<string>>(new Set())

// Default hidden columns (columns hidden by default on first load)
const DEFAULT_HIDDEN_COLUMNS = ['notes', 'subscriptions', 'usage', 'concurrency']

// localStorage key for column settings
const HIDDEN_COLUMNS_KEY = 'user-hidden-columns'

// Load saved column settings
const loadSavedColumns = () => {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed.forEach(key => hiddenColumns.add(key))
    REDACTED else {
      // Use default hidden columns on first load
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

// Check if column is visible (not in hidden set)
const isColumnVisible = (key: string) => !hiddenColumns.has(key)

// Filtered columns based on visibility
const columns = computed<Column[]>(() =>
  allColumns.value.filter(col =>
    col.key === 'email' || col.key === 'actions' || !hiddenColumns.has(col.key)
  )
)

const users = ref<User[]>([])
const loading = ref(false)
const searchQuery = ref('')

// Filter values (role, status, and custom attributes)
const filters = reactive({
  role: '',
  status: ''
REDACTED)
const activeAttributeFilters = reactive<Record<number, string>>({REDACTED)

// Visible filters tracking (which filters are shown in the UI)
// Keys: 'role', 'status', 'attr_${idREDACTED'
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
  { key: 'role', name: t('admin.users.columns.role'), type: 'select' as const REDACTED,
  { key: 'status', name: t('admin.users.columns.status'), type: 'select' as const REDACTED
])

// Load saved filters from localStorage
const loadSavedFilters = () => {
  try {
    // Load visible filters
    const savedVisible = localStorage.getItem(VISIBLE_FILTERS_KEY)
    if (savedVisible) {
      const parsed = JSON.parse(savedVisible) as string[]
      parsed.forEach(key => visibleFilters.add(key))
    REDACTED
    // Load filter values
    const savedValues = localStorage.getItem(FILTER_VALUES_KEY)
    if (savedValues) {
      const parsed = JSON.parse(savedValues)
      if (parsed.role) filters.role = parsed.role
      if (parsed.status) filters.status = parsed.status
      if (parsed.attributes) {
        Object.assign(activeAttributeFilters, parsed.attributes)
      REDACTED
    REDACTED
  REDACTED catch (e) {
    console.error('Failed to load saved filters:', e)
  REDACTED
REDACTED

// Save filters to localStorage
const saveFiltersToStorage = () => {
  try {
    // Save visible filters
    localStorage.setItem(VISIBLE_FILTERS_KEY, JSON.stringify([...visibleFilters]))
    // Save filter values
    const values = {
      role: filters.role,
      status: filters.status,
      attributes: activeAttributeFilters
    REDACTED
    localStorage.setItem(FILTER_VALUES_KEY, JSON.stringify(values))
  REDACTED catch (e) {
    console.error('Failed to save filters:', e)
  REDACTED
REDACTED

// Get attribute definition by ID
const getAttributeDefinition = (attrId: number): UserAttributeDefinition | undefined => {
  return attributeDefinitions.value.find(d => d.id === attrId)
REDACTED
const usageStats = ref<Record<string, BatchUserUsageStats>>({REDACTED)
// User attribute definitions and values
const attributeDefinitions = ref<UserAttributeDefinition[]>([])
const userAttributeValues = ref<Record<number, Record<number, string>>>({REDACTED)
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const showApiKeysModal = ref(false)
const showAttributesModal = ref(false)
const submitting = ref(false)
const editingUser = ref<User | null>(null)
const deletingUser = ref<User | null>(null)
const viewingUser = ref<User | null>(null)
const userApiKeys = ref<ApiKey[]>([])
const loadingApiKeys = ref(false)
const passwordCopied = ref(false)
let abortController: AbortController | null = null

// Action Menu State
const activeMenuId = ref<number | null>(null)
const menuPosition = ref<{ top: number; left: number REDACTED | null>(null)
const actionButtonRefs = ref<Map<number, HTMLElement>>(new Map())

const setActionButtonRef = (userId: number, el: Element | ComponentPublicInstance | null) => {
  if (el instanceof HTMLElement) {
    actionButtonRefs.value.set(userId, el)
  REDACTED else {
    actionButtonRefs.value.delete(userId)
  REDACTED
REDACTED

const openActionMenu = (user: User) => {
  if (activeMenuId.value === user.id) {
    closeActionMenu()
  REDACTED else {
    const buttonEl = actionButtonRefs.value.get(user.id)
    if (buttonEl) {
      const rect = buttonEl.getBoundingClientRect()
      const menuWidth = 192
      const menuHeight = 240
      const padding = 8
      const viewportWidth = window.innerWidth
      const viewportHeight = window.innerHeight
      const left = Math.min(
        Math.max(rect.right - menuWidth, padding),
        Math.max(viewportWidth - menuWidth - padding, padding)
      )
      let top = rect.bottom + 4
      if (top + menuHeight > viewportHeight - padding) {
        top = Math.max(rect.top - menuHeight - 4, padding)
      REDACTED
      // Position menu near the trigger, clamped to viewport
      menuPosition.value = {
        top,
        left
      REDACTED
    REDACTED
    activeMenuId.value = user.id
  REDACTED
REDACTED

const closeActionMenu = () => {
  activeMenuId.value = null
  menuPosition.value = null
REDACTED

// Close menu when clicking outside
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  if (!target.closest('.action-menu-trigger') && !target.closest('.action-menu-content')) {
    closeActionMenu()
  REDACTED
  // Close filter dropdown when clicking outside
  if (filterDropdownRef.value && !filterDropdownRef.value.contains(target)) {
    showFilterDropdown.value = false
  REDACTED
  // Close column dropdown when clicking outside
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  REDACTED
REDACTED

// Allowed groups modal state
const showAllowedGroupsModal = ref(false)
const allowedGroupsUser = ref<User | null>(null)
const standardGroups = ref<Group[]>([])
const selectedGroupIds = ref<number[]>([])
const loadingGroups = ref(false)
const savingAllowedGroups = ref(false)

// Balance (Deposit/Withdraw) modal state
const showBalanceModal = ref(false)
const balanceUser = ref<User | null>(null)
const balanceOperation = ref<'add' | 'subtract'>('add')
const balanceSubmitting = ref(false)
const balanceForm = reactive({
  amount: 0,
  notes: ''
REDACTED)

const createForm = reactive({
  email: '',
  password: '',
  username: '',
  notes: '',
  balance: 0,
  concurrency: 1
REDACTED)

const editForm = reactive({
  email: '',
  password: '',
  username: '',
  notes: '',
  concurrency: 1,
  customAttributes: {REDACTED as UserAttributeValuesMap
REDACTED)
const editPasswordCopied = ref(false)

// 计算剩余天数
const getDaysRemaining = (expiresAt: string): number => {
  const now = new Date()
  const expires = new Date(expiresAt)
  const diffMs = expires.getTime() - now.getTime()
  return Math.ceil(diffMs / (1000 * 60 * 60 * 24))
REDACTED

const generateRandomPasswordStr = () => {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghjkmnpqrstuvwxyz23456789!@#$%^&*'
  let password = ''
  for (let i = 0; i < 16; i++) {
    password += chars.charAt(Math.floor(Math.random() * chars.length))
  REDACTED
  return password
REDACTED

const generateRandomPassword = () => {
  createForm.password = generateRandomPasswordStr()
REDACTED

const generateEditPassword = () => {
  editForm.password = generateRandomPasswordStr()
REDACTED

const copyPassword = async () => {
  if (!createForm.password) return
  const success = await clipboardCopy(createForm.password, t('admin.users.passwordCopied'))
  if (success) {
    passwordCopied.value = true
    setTimeout(() => {
      passwordCopied.value = false
    REDACTED, 2000)
  REDACTED
REDACTED

const copyEditPassword = async () => {
  if (!editForm.password) return
  const success = await clipboardCopy(editForm.password, t('admin.users.passwordCopied'))
  if (success) {
    editPasswordCopied.value = true
    setTimeout(() => {
      editPasswordCopied.value = false
    REDACTED, 2000)
  REDACTED
REDACTED

const loadAttributeDefinitions = async () => {
  try {
    attributeDefinitions.value = await adminAPI.userAttributes.listEnabledDefinitions()
  REDACTED catch (e) {
    console.error('Failed to load attribute definitions:', e)
  REDACTED
REDACTED

// Handle attributes modal close - reload definitions and users
const handleAttributesModalClose = async () => {
  showAttributesModal.value = false
  await loadAttributeDefinitions()
  loadUsers()
REDACTED

const loadUsers = async () => {
  abortController?.abort()
  const currentAbortController = new AbortController()
  abortController = currentAbortController
  const { signal REDACTED = currentAbortController
  loading.value = true
  try {
    // Build attribute filters from active filters
    const attrFilters: Record<number, string> = {REDACTED
    for (const [attrId, value] of Object.entries(activeAttributeFilters)) {
      if (value) {
        attrFilters[Number(attrId)] = value
      REDACTED
    REDACTED

    const response = await adminAPI.users.list(
      pagination.page,
      pagination.page_size,
      {
        role: filters.role as any,
        status: filters.status as any,
        search: searchQuery.value || undefined,
        attributes: Object.keys(attrFilters).length > 0 ? attrFilters : undefined
      REDACTED,
      { signal REDACTED
    )
    if (signal.aborted) {
      return
    REDACTED
    users.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages

    // Load usage stats and attribute values for all users in the list
    if (response.items.length > 0) {
      const userIds = response.items.map((u) => u.id)
      // Load usage stats
      try {
        const usageResponse = await adminAPI.dashboard.getBatchUsersUsage(userIds)
        if (signal.aborted) {
          return
        REDACTED
        usageStats.value = usageResponse.stats
      REDACTED catch (e) {
        if (signal.aborted) {
          return
        REDACTED
        console.error('Failed to load usage stats:', e)
      REDACTED
      // Load attribute values
      if (attributeDefinitions.value.length > 0) {
        try {
          const attrResponse = await adminAPI.userAttributes.getBatchUserAttributes(userIds)
          if (signal.aborted) {
            return
          REDACTED
          userAttributeValues.value = attrResponse.attributes
        REDACTED catch (e) {
          if (signal.aborted) {
            return
          REDACTED
          console.error('Failed to load user attribute values:', e)
        REDACTED
      REDACTED
    REDACTED
  REDACTED catch (error) {
    const errorInfo = error as { name?: string; code?: string REDACTED
    if (errorInfo?.name === 'AbortError' || errorInfo?.name === 'CanceledError' || errorInfo?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('admin.users.failedToLoad'))
    console.error('Error loading users:', error)
  REDACTED finally {
    if (abortController === currentAbortController) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadUsers()
  REDACTED, 300)
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadUsers()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadUsers()
REDACTED

// Filter helpers
const getAttributeDefinitionName = (attrId: number): string => {
  const def = attributeDefinitions.value.find(d => d.id === attrId)
  return def?.name || String(attrId)
REDACTED

// Toggle a built-in filter (role/status)
const toggleBuiltInFilter = (key: string) => {
  if (visibleFilters.has(key)) {
    visibleFilters.delete(key)
    if (key === 'role') filters.role = ''
    if (key === 'status') filters.status = ''
  REDACTED else {
    visibleFilters.add(key)
  REDACTED
  saveFiltersToStorage()
  loadUsers()
REDACTED

// Toggle a custom attribute filter
const toggleAttributeFilter = (attr: UserAttributeDefinition) => {
  const key = `attr_${attr.idREDACTED`
  if (visibleFilters.has(key)) {
    visibleFilters.delete(key)
    delete activeAttributeFilters[attr.id]
  REDACTED else {
    visibleFilters.add(key)
    activeAttributeFilters[attr.id] = ''
  REDACTED
  saveFiltersToStorage()
  loadUsers()
REDACTED

const updateAttributeFilter = (attrId: number, value: string) => {
  activeAttributeFilters[attrId] = value
REDACTED

// Apply filter and save to localStorage
const applyFilter = () => {
  saveFiltersToStorage()
  loadUsers()
REDACTED

const closeCreateModal = () => {
  showCreateModal.value = false
  createForm.email = ''
  createForm.password = ''
  createForm.username = ''
  createForm.notes = ''
  createForm.balance = 0
  createForm.concurrency = 1
  passwordCopied.value = false
REDACTED

const handleCreateUser = async () => {
  submitting.value = true
  try {
    await adminAPI.users.create(createForm)
    appStore.showSuccess(t('admin.users.userCreated'))
    closeCreateModal()
    loadUsers()
  REDACTED catch (error: any) {
    appStore.showError(
      error.response?.data?.message ||
        error.response?.data?.detail ||
        t('admin.users.failedToCreate')
    )
    console.error('Error creating user:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleEdit = (user: User) => {
  editingUser.value = user
  editForm.email = user.email
  editForm.password = ''
  editForm.username = user.username || ''
  editForm.notes = user.notes || ''
  editForm.concurrency = user.concurrency
  editForm.customAttributes = {REDACTED
  editPasswordCopied.value = false
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingUser.value = null
  editForm.password = ''
  editForm.customAttributes = {REDACTED
  editPasswordCopied.value = false
REDACTED

const handleUpdateUser = async () => {
  if (!editingUser.value) return

  submitting.value = true
  try {
    const updateData: Record<string, any> = {
      email: editForm.email,
      username: editForm.username,
      notes: editForm.notes,
      concurrency: editForm.concurrency
    REDACTED
    if (editForm.password.trim()) {
      updateData.password = editForm.password.trim()
    REDACTED

    await adminAPI.users.update(editingUser.value.id, updateData)

    // Save custom attributes if any
    if (Object.keys(editForm.customAttributes).length > 0) {
      await adminAPI.userAttributes.updateUserAttributeValues(
        editingUser.value.id,
        editForm.customAttributes
      )
    REDACTED

    appStore.showSuccess(t('admin.users.userUpdated'))
    closeEditModal()
    loadUsers()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToUpdate'))
    console.error('Error updating user:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleToggleStatus = async (user: User) => {
  const newStatus = user.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.users.toggleStatus(user.id, newStatus)
    appStore.showSuccess(
      newStatus === 'active' ? t('admin.users.userEnabled') : t('admin.users.userDisabled')
    )
    loadUsers()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToToggle'))
    console.error('Error toggling user status:', error)
  REDACTED
REDACTED

const handleViewApiKeys = async (user: User) => {
  viewingUser.value = user
  showApiKeysModal.value = true
  loadingApiKeys.value = true
  userApiKeys.value = []

  try {
    const response = await adminAPI.users.getUserApiKeys(user.id)
    userApiKeys.value = response.items || []
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToLoadApiKeys'))
    console.error('Error loading user API keys:', error)
  REDACTED finally {
    loadingApiKeys.value = false
  REDACTED
REDACTED

const closeApiKeysModal = () => {
  showApiKeysModal.value = false
  viewingUser.value = null
  userApiKeys.value = []
REDACTED

// Allowed Groups functions
const handleAllowedGroups = async (user: User) => {
  allowedGroupsUser.value = user
  showAllowedGroupsModal.value = true
  loadingGroups.value = true
  standardGroups.value = []
  selectedGroupIds.value = user.allowed_groups ? [...user.allowed_groups] : []

  try {
    const allGroups = await adminAPI.groups.getAll()
    // Only show standard type groups (subscription type groups are managed in /admin/subscriptions)
    standardGroups.value = allGroups.filter(
      (g) => g.subscription_type === 'standard' && g.status === 'active'
    )
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToLoadGroups'))
    console.error('Error loading groups:', error)
  REDACTED finally {
    loadingGroups.value = false
  REDACTED
REDACTED

const closeAllowedGroupsModal = () => {
  showAllowedGroupsModal.value = false
  allowedGroupsUser.value = null
  standardGroups.value = []
  selectedGroupIds.value = []
REDACTED

const handleSaveAllowedGroups = async () => {
  if (!allowedGroupsUser.value) return

  savingAllowedGroups.value = true
  try {
    // null means allow all non-exclusive groups, empty array also means allow all
    const allowedGroups = selectedGroupIds.value.length > 0 ? selectedGroupIds.value : null
    await adminAPI.users.update(allowedGroupsUser.value.id, { allowed_groups: allowedGroups REDACTED)
    appStore.showSuccess(t('admin.users.allowedGroupsUpdated'))
    closeAllowedGroupsModal()
    loadUsers()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.failedToUpdateAllowedGroups'))
    console.error('Error updating allowed groups:', error)
  REDACTED finally {
    savingAllowedGroups.value = false
  REDACTED
REDACTED

const handleDelete = (user: User) => {
  deletingUser.value = user
  showDeleteDialog.value = true
REDACTED

const confirmDelete = async () => {
  if (!deletingUser.value) return

  try {
    await adminAPI.users.delete(deletingUser.value.id)
    appStore.showSuccess(t('admin.users.userDeleted'))
    showDeleteDialog.value = false
    deletingUser.value = null
    loadUsers()
  REDACTED catch (error: any) {
    appStore.showError(
      error.response?.data?.message ||
        error.response?.data?.detail ||
        t('admin.users.failedToDelete')
    )
    console.error('Error deleting user:', error)
  REDACTED
REDACTED

const handleDeposit = (user: User) => {
  balanceUser.value = user
  balanceOperation.value = 'add'
  balanceForm.amount = 0
  balanceForm.notes = ''
  showBalanceModal.value = true
REDACTED

const handleWithdraw = (user: User) => {
  balanceUser.value = user
  balanceOperation.value = 'subtract'
  balanceForm.amount = 0
  balanceForm.notes = ''
  showBalanceModal.value = true
REDACTED

const closeBalanceModal = () => {
  showBalanceModal.value = false
  balanceUser.value = null
  balanceForm.amount = 0
  balanceForm.notes = ''
REDACTED

const calculateNewBalance = () => {
  if (!balanceUser.value) return 0
  if (balanceOperation.value === 'add') {
    return balanceUser.value.balance + balanceForm.amount
  REDACTED else {
    return balanceUser.value.balance - balanceForm.amount
  REDACTED
REDACTED

const handleBalanceSubmit = async () => {
  if (!balanceUser.value || balanceForm.amount <= 0) return

  balanceSubmitting.value = true
  try {
    await adminAPI.users.updateBalance(
      balanceUser.value.id,
      balanceForm.amount,
      balanceOperation.value,
      balanceForm.notes
    )

    const successMsg =
      balanceOperation.value === 'add'
        ? t('admin.users.depositSuccess')
        : t('admin.users.withdrawSuccess')

    appStore.showSuccess(successMsg)
    closeBalanceModal()
    loadUsers()
  REDACTED catch (error: any) {
    const errorMsg =
      balanceOperation.value === 'add'
        ? t('admin.users.failedToDeposit')
        : t('admin.users.failedToWithdraw')

    appStore.showError(error.response?.data?.detail || errorMsg)
    console.error('Error updating balance:', error)
  REDACTED finally {
    balanceSubmitting.value = false
  REDACTED
REDACTED

onMounted(async () => {
  await loadAttributeDefinitions()
  loadSavedFilters()
  loadSavedColumns()
  loadUsers()
  document.addEventListener('click', handleClickOutside)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
REDACTED)
</script>
