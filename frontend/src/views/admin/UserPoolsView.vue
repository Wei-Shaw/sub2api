<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: search + filters -->
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
                :placeholder="t('admin.userPools.searchPools')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>
            <Select
              v-model="filterStatus"
              :options="statusFilterOptions"
              :placeholder="t('admin.userPools.allStatus')"
              class="w-40"
              @change="loadPools"
            />
          </div>

          <!-- Right: actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button @click="loadPools" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <!-- Column Settings Dropdown -->
            <div class="relative" ref="columnDropdownRef">
              <button
                @click="showColumnDropdown = !showColumnDropdown"
                class="btn btn-secondary px-2 md:px-3"
                :title="t('admin.userPools.columnSettings')"
              >
                <svg class="h-4 w-4 md:mr-1.5" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M9 4.5v15m6-15v15m-10.875 0h15.75c.621 0 1.125-.504 1.125-1.125V5.625c0-.621-.504-1.125-1.125-1.125H4.125C3.504 4.5 3 5.004 3 5.625v12.75c0 .621.504 1.125 1.125 1.125z" />
                </svg>
                <span class="hidden md:inline">{{ t('admin.userPools.columnSettings') }}</span>
              </button>
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
                  <span>{{ col.label }}</span>
                  <Icon
                    v-if="isColumnVisible(col.key)"
                    name="check"
                    size="sm"
                    class="text-primary-500"
                    :stroke-width="2"
                  />
                </button>
              </div>
            </div>
            <button @click="openCreateModal" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.userPools.createPool') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="pools"
          :loading="loading"
          row-key="id"
        >
          <template #cell-name="{ row }">
            <button
              class="font-medium text-primary-600 hover:underline dark:text-primary-400 text-left"
              @click="openDetail(row)"
            >
              {{ row.name }}
            </button>
          </template>

          <template #cell-description="{ value }">
            <span class="text-gray-500 dark:text-gray-400 text-sm">{{ value || '—' }}</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'active'
                  ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                  : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
              ]"
            >
              {{ t(`admin.userPools.status.${value}`) }}
            </span>
          </template>

          <template #cell-groups="{ row }">
            <div v-if="getPoolGrants(row).length > 0" class="flex flex-wrap gap-1">
              <GroupBadge
                v-for="grant in getPoolGrants(row)"
                :key="grant.group_id"
                :name="grant.group_name"
                :platform="grant.platform"
                :subscription-type="grant.subscription_type"
                :rate-multiplier="grant.rate_multiplier ?? undefined"
                :show-rate="false"
              />
            </div>
            <span v-else class="text-xs text-gray-400 dark:text-dark-500">-</span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-sm text-gray-500 dark:text-gray-400">{{ formatDate(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn btn-ghost btn-sm" @click="openDetail(row)">
                <Icon name="eye" size="sm" class="mr-1" />
                {{ t('common.view') }}
              </button>
              <button class="btn btn-ghost btn-sm" @click="openEditModal(row)">
                <Icon name="edit" size="sm" class="mr-1" />
                {{ t('common.edit') }}
              </button>
              <button class="btn btn-ghost btn-sm text-red-600 hover:text-red-700 dark:text-red-400" @click="confirmDelete(row)">
                <Icon name="trash" size="sm" class="mr-1" />
                {{ t('common.delete') }}
              </button>
            </div>
          </template>

          <template #empty>
            <div class="flex flex-col items-center py-12">
              <Icon name="inbox" size="xl" class="mb-4 h-12 w-12 text-gray-400 dark:text-dark-500" />
              <p class="text-lg font-medium text-gray-900 dark:text-gray-100">
                {{ t('admin.userPools.noPoolsYet') }}
              </p>
              <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.userPools.createFirstPool') }}
              </p>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :page="currentPage"
          :total="total"
          :page-size="pageSize"
          @update:page="handlePageChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create Modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('admin.userPools.createPool')"
      width="narrow"
      @close="closeCreateModal"
    >
      <form @submit.prevent="handleCreate" class="space-y-4">
        <div>
          <label class="label">{{ t('admin.userPools.form.name') }}</label>
          <input
            v-model="createForm.name"
            type="text"
            :placeholder="t('admin.userPools.form.namePlaceholder')"
            class="input"
            required
          />
        </div>
        <div>
          <label class="label">{{ t('admin.userPools.form.description') }}</label>
          <textarea
            v-model="createForm.description"
            :placeholder="t('admin.userPools.form.descriptionPlaceholder')"
            class="input"
            rows="2"
          />
        </div>
        <div>
          <label class="label">{{ t('admin.userPools.form.status') }}</label>
          <Select
            v-model="createForm.status"
            :options="statusOptions"
            class="w-full"
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeCreateModal">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleCreate">
            {{ submitting ? t('admin.userPools.creating') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Edit Modal -->
    <BaseDialog
      :show="showEditModal"
      :title="t('admin.userPools.editPool')"
      width="narrow"
      @close="closeEditModal"
    >
      <form @submit.prevent="handleUpdate" class="space-y-4">
        <div>
          <label class="label">{{ t('admin.userPools.form.name') }}</label>
          <input
            v-model="editForm.name"
            type="text"
            :placeholder="t('admin.userPools.form.namePlaceholder')"
            class="input"
            required
          />
        </div>
        <div>
          <label class="label">{{ t('admin.userPools.form.description') }}</label>
          <textarea
            v-model="editForm.description"
            :placeholder="t('admin.userPools.form.descriptionPlaceholder')"
            class="input"
            rows="2"
          />
        </div>
        <div>
          <label class="label">{{ t('admin.userPools.form.status') }}</label>
          <Select
            v-model="editForm.status"
            :options="statusOptions"
            class="w-full"
          />
        </div>
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeEditModal">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleUpdate">
            {{ submitting ? t('admin.userPools.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirm -->
    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.userPools.deletePool')"
      :message="t('admin.userPools.deleteConfirm', { name: deletingPool?.name ?? '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleDelete"
      @cancel="showDeleteConfirm = false"
    />

    <!-- Detail Drawer -->
    <Teleport to="body">
      <Transition name="drawer">
        <div v-if="showDetail" class="fixed inset-0 z-40 flex justify-end">
          <div class="absolute inset-0 bg-black/40" @click="closeDetail" />
          <div class="relative z-10 flex h-full w-full max-w-2xl flex-col bg-white shadow-xl dark:bg-dark-800 overflow-hidden">
            <!-- Drawer header -->
            <div class="flex items-center justify-between border-b border-gray-200 px-6 py-4 dark:border-dark-700 flex-shrink-0">
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('admin.userPools.detailPanel') }}
              </h2>
              <button @click="closeDetail" class="rounded-xl p-2 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:text-dark-500 dark:hover:bg-dark-700 dark:hover:text-dark-300">
                <Icon name="x" size="md" />
              </button>
            </div>

            <!-- Drawer body -->
            <div class="flex-1 overflow-y-auto">
              <div v-if="detailPool" class="space-y-6 p-6">
                <!-- Basic Info -->
                <section>
                  <h3 class="mb-3 text-sm font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                    {{ t('admin.userPools.basicInfo') }}
                  </h3>
                  <div class="rounded-xl border border-gray-200 dark:border-dark-700 p-4 space-y-3">
                    <div class="flex items-center justify-between">
                      <span class="text-sm text-gray-500 dark:text-gray-400">ID</span>
                      <span class="text-sm font-mono text-gray-900 dark:text-white">{{ detailPool.id }}</span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.userPools.form.name') }}</span>
                      <span class="text-sm font-medium text-gray-900 dark:text-white">{{ detailPool.name }}</span>
                    </div>
                    <div class="flex items-center justify-between">
                      <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.userPools.form.status') }}</span>
                      <span
                        :class="[
                          'inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium',
                          detailPool.status === 'active'
                            ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                            : 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
                        ]"
                      >
                        {{ t(`admin.userPools.status.${detailPool.status}`) }}
                      </span>
                    </div>
                    <div v-if="detailPool.description" class="flex items-start justify-between gap-4">
                      <span class="text-sm text-gray-500 dark:text-gray-400 flex-shrink-0">{{ t('admin.userPools.form.description') }}</span>
                      <span class="text-sm text-gray-900 dark:text-white text-right">{{ detailPool.description }}</span>
                    </div>
                  </div>
                </section>

                <!-- Members -->
                <section>
                  <div class="mb-3 flex items-center justify-between">
                    <h3 class="text-sm font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      {{ t('admin.userPools.membersTitle') }}
                      <span v-if="membersTotal > 0" class="ml-1 text-xs">({{ membersTotal }})</span>
                    </h3>
                    <button class="btn btn-secondary btn-sm" @click="openAddMembersModal">
                      <Icon name="plus" size="sm" class="mr-1" />
                      {{ t('admin.userPools.addMembers') }}
                    </button>
                  </div>

                  <div class="rounded-xl border border-gray-200 dark:border-dark-700 overflow-hidden">
                    <div v-if="membersLoading" class="p-8 text-center">
                      <Icon name="refresh" size="lg" class="animate-spin text-gray-400 mx-auto" />
                    </div>
                    <div v-else-if="members.length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-gray-400">
                      {{ t('admin.userPools.noMembers') }}
                    </div>
                    <template v-else>
                      <table class="w-full">
                        <thead class="bg-gray-50 dark:bg-dark-700/50">
                          <tr>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.memberUserId') }}
                            </th>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.memberEmail') }}
                            </th>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.memberUsername') }}
                            </th>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.memberAddedAt') }}
                            </th>
                            <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400"></th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                          <tr v-for="member in members" :key="member.user_id">
                            <td class="px-4 py-3 text-sm font-mono text-gray-900 dark:text-white">
                              {{ member.user_id }}
                            </td>
                            <td class="px-4 py-3 text-sm text-gray-900 dark:text-white truncate max-w-xs" :title="member.email">
                              {{ member.email || '-' }}
                            </td>
                            <td class="px-4 py-3 text-sm text-gray-700 dark:text-gray-300 truncate max-w-xs" :title="member.username">
                              {{ member.username || '-' }}
                            </td>
                            <td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                              {{ formatDate(member.created_at) }}
                            </td>
                            <td class="px-4 py-3 text-right">
                              <button
                                class="text-xs text-red-500 hover:text-red-700 dark:text-red-400"
                                @click="confirmRemoveMember(member.user_id)"
                              >
                                {{ t('common.delete') }}
                              </button>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                      <Pagination
                        v-if="membersTotal > membersPageSize"
                        :page="membersPage"
                        :total="membersTotal"
                        :page-size="membersPageSize"
                        @update:page="handleMembersPageChange"
                      />
                    </template>
                  </div>
                </section>

                <!-- Group Grants -->
                <section>
                  <div class="mb-3 flex items-center justify-between">
                    <h3 class="text-sm font-medium text-gray-500 dark:text-gray-400 uppercase tracking-wider">
                      {{ t('admin.userPools.groupGrantsTitle') }}
                    </h3>
                    <button class="btn btn-secondary btn-sm" @click="openGrantsEditModal">
                      <Icon name="edit" size="sm" class="mr-1" />
                      {{ t('common.edit') }}
                    </button>
                  </div>

                  <div class="rounded-xl border border-gray-200 dark:border-dark-700 overflow-hidden">
                    <div v-if="grantsLoading" class="p-8 text-center">
                      <Icon name="refresh" size="lg" class="animate-spin text-gray-400 mx-auto" />
                    </div>
                    <div v-else-if="grants.length === 0" class="p-6 text-center text-sm text-gray-500 dark:text-gray-400">
                      {{ t('admin.userPools.noGrants') }}
                    </div>
                    <template v-else>
                      <table class="w-full">
                        <thead class="bg-gray-50 dark:bg-dark-700/50">
                          <tr>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.grantGroupId') }}
                            </th>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.grantRateMultiplier') }}
                            </th>
                            <th class="px-4 py-3 text-left text-xs font-medium text-gray-500 dark:text-gray-400">
                              {{ t('admin.userPools.grantRpmOverride') }}
                            </th>
                            <th class="px-4 py-3 text-right text-xs font-medium text-gray-500 dark:text-gray-400"></th>
                          </tr>
                        </thead>
                        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
                          <tr v-for="grant in grants" :key="grant.group_id">
                            <td class="px-4 py-3 text-sm font-medium text-gray-900 dark:text-white">
                              {{ getGroupName(grant.group_id) }}
                            </td>
                            <td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                              <span v-if="grant.rate_multiplier != null">{{ grant.rate_multiplier }}x</span>
                              <span v-else class="text-xs italic">{{ t('admin.userPools.grantRateDefault') }}</span>
                            </td>
                            <td class="px-4 py-3 text-sm text-gray-500 dark:text-gray-400">
                              <span v-if="grant.rpm_override != null">{{ grant.rpm_override }}</span>
                              <span v-else class="text-xs italic">{{ t('admin.userPools.grantRpmDefault') }}</span>
                            </td>
                            <td class="px-4 py-3 text-right">
                              <button
                                class="text-xs text-red-500 hover:text-red-700 dark:text-red-400"
                                @click="handleDeleteGrant(grant.group_id)"
                              >
                                {{ t('common.delete') }}
                              </button>
                            </td>
                          </tr>
                        </tbody>
                      </table>
                    </template>
                  </div>
                </section>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- Add Members Modal (rich user-picker) -->
    <BaseDialog
      :show="showAddMembersModal"
      :title="t('admin.userPools.addMembersModal.title')"
      width="wide"
      @close="closeAddMembersModal"
    >
      <div class="space-y-4">
        <!-- ── Filter row ── -->
        <div class="flex flex-wrap gap-2">
          <!-- Search -->
          <div class="relative flex-1 min-w-[180px]">
            <Icon name="search" size="md" class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500" />
            <input
              v-model="pickerSearch"
              type="text"
              :placeholder="t('admin.userPools.addMembersModal.searchPlaceholder')"
              class="input pl-10 w-full"
              @input="handlePickerSearch"
              @keyup.enter="loadPickerUsers(true)"
            />
          </div>
          <!-- Status filter -->
          <Select
            v-model="pickerStatus"
            :options="pickerStatusOptions"
            class="w-36"
            @change="loadPickerUsers(true)"
          />
          <!-- Group filter -->
          <Select
            v-model="pickerGroup"
            :options="pickerGroupOptions"
            class="w-44"
            @change="loadPickerUsers(true)"
          />
        </div>

        <!-- ── Attribute filters ── -->
        <div v-if="pickerAttrDefs.length > 0" class="flex flex-wrap gap-2">
          <template v-for="def in pickerAttrDefs" :key="def.id">
            <!-- select / multi_select -->
            <div v-if="['select', 'multi_select'].includes(def.type)" class="w-36">
              <Select
                :model-value="pickerAttrFilters[def.id] ?? ''"
                :options="[{ value: '', label: def.name }, ...def.options]"
                @update:model-value="(v) => { pickerAttrFilters[def.id] = String(v ?? ''); loadPickerUsers(true) }"
              />
            </div>
            <!-- number -->
            <input
              v-else-if="def.type === 'number'"
              :value="pickerAttrFilters[def.id] ?? ''"
              type="number"
              :placeholder="def.name"
              class="input w-28"
              @input="(e) => { pickerAttrFilters[def.id] = (e.target as HTMLInputElement).value; handlePickerSearch() }"
              @keyup.enter="loadPickerUsers(true)"
            />
            <!-- date -->
            <input
              v-else-if="def.type === 'date'"
              :value="pickerAttrFilters[def.id] ?? ''"
              type="date"
              :placeholder="def.name"
              class="input w-36"
              @input="(e) => { pickerAttrFilters[def.id] = (e.target as HTMLInputElement).value; loadPickerUsers(true) }"
            />
            <!-- text / textarea / email / url / fallback -->
            <input
              v-else
              :value="pickerAttrFilters[def.id] ?? ''"
              type="text"
              :placeholder="def.name"
              class="input w-32"
              @input="(e) => { pickerAttrFilters[def.id] = (e.target as HTMLInputElement).value; handlePickerSearch() }"
              @keyup.enter="loadPickerUsers(true)"
            />
          </template>
        </div>

        <!-- ── Bulk add by filter ── -->
        <div v-if="pickerTotal > 0" class="flex items-center justify-between rounded-lg bg-gray-50 dark:bg-dark-700/50 px-4 py-2.5 border border-gray-200 dark:border-dark-700">
          <span class="text-sm text-gray-600 dark:text-gray-400">
            {{ t('admin.userPools.addMembersModal.matchedCount', { n: pickerTotal }) }}
          </span>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="!hasAnyPickerFilter"
            :title="hasAnyPickerFilter ? '' : t('admin.userPools.addMembersModal.addAllMatchedNoFilter')"
            @click="openAddAllConfirm"
          >
            {{ t('admin.userPools.addMembersModal.addAllMatched', { n: pickerTotal }) }}
          </button>
        </div>

        <!-- ── User list ── -->
        <div class="rounded-xl border border-gray-200 dark:border-dark-700 overflow-hidden">
          <!-- Table header with select-all -->
          <div class="flex items-center gap-3 bg-gray-50 dark:bg-dark-700/50 px-4 py-2 border-b border-gray-200 dark:border-dark-700">
            <input
              type="checkbox"
              :checked="pickerPageAllSelected"
              :indeterminate="pickerPagePartialSelected"
              class="h-4 w-4 rounded border-gray-300 text-primary-600 cursor-pointer"
              @change="togglePickerPageAll"
            />
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.userPools.addMembersModal.selectAll') }}</span>
          </div>

          <!-- Loading -->
          <div v-if="pickerLoading" class="flex items-center justify-center py-10 text-gray-400 gap-2">
            <Icon name="refresh" size="md" class="animate-spin" />
            <span class="text-sm">{{ t('admin.userPools.addMembersModal.loading') }}</span>
          </div>

          <!-- Empty state -->
          <div v-else-if="pickerUsers.length === 0" class="py-10 text-center">
            <Icon name="inbox" size="xl" class="mx-auto mb-2 text-gray-300 dark:text-dark-600" />
            <p class="text-sm font-medium text-gray-500 dark:text-gray-400">{{ t('admin.userPools.addMembersModal.noResults') }}</p>
            <p class="text-xs text-gray-400 dark:text-gray-500 mt-1">{{ t('admin.userPools.addMembersModal.noResultsHint') }}</p>
          </div>

          <!-- User rows -->
          <template v-else>
            <div
              v-for="user in pickerUsers"
              :key="user.id"
              class="flex items-center gap-3 px-4 py-2.5 hover:bg-gray-50 dark:hover:bg-dark-700/50 cursor-pointer border-b border-gray-100 dark:border-dark-700 last:border-0"
              @click="togglePickerUser(user.id)"
            >
              <input
                type="checkbox"
                :checked="selectedUserIds.has(user.id)"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 cursor-pointer"
                @click.stop
                @change="togglePickerUser(user.id)"
              />
              <div class="flex-1 min-w-0">
                <div class="flex items-baseline gap-2">
                  <span class="text-sm font-medium text-gray-900 dark:text-white truncate">{{ user.email }}</span>
                  <span class="text-xs text-gray-400 font-mono flex-shrink-0">#{{ user.id }}</span>
                </div>
                <div v-if="userGroupNames(user).length > 0" class="flex flex-wrap gap-1 mt-0.5">
                  <span
                    v-for="gname in userGroupNames(user).slice(0, 3)"
                    :key="gname"
                    class="inline-flex items-center rounded px-1.5 py-0.5 text-xs bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400"
                  >{{ gname }}</span>
                  <span
                    v-if="userGroupNames(user).length > 3"
                    class="inline-flex items-center rounded px-1.5 py-0.5 text-xs bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-500"
                  >{{ t('admin.userPools.addMembersModal.moreGroups', { n: userGroupNames(user).length - 3 }) }}</span>
                </div>
              </div>
            </div>
          </template>
        </div>

        <!-- Pagination -->
        <Pagination
          v-if="pickerTotal > pickerPageSize"
          :page="pickerPage"
          :total="pickerTotal"
          :page-size="pickerPageSize"
          @update:page="handlePickerPageChange"
        />

        <!-- ── Selected chips ── -->
        <div v-if="selectedUserIds.size > 0" class="rounded-xl border border-primary-200 bg-primary-50 dark:border-primary-900/50 dark:bg-primary-900/10 px-4 py-3">
          <div class="flex items-center justify-between mb-2">
            <span class="text-xs font-medium text-primary-700 dark:text-primary-400">
              {{ t('admin.userPools.addMembersModal.selectedCount', { count: selectedUserIds.size }) }}
            </span>
            <button
              type="button"
              class="text-xs text-primary-600 hover:text-primary-800 dark:text-primary-400 dark:hover:text-primary-200 underline"
              @click="selectedUserIds.clear()"
            >
              {{ t('admin.userPools.addMembersModal.clearSelected') }}
            </button>
          </div>
          <div class="flex flex-wrap gap-1.5 max-h-24 overflow-y-auto">
            <span
              v-for="uid in Array.from(selectedUserIds)"
              :key="uid"
              class="inline-flex items-center gap-1 rounded-full bg-white dark:bg-dark-800 border border-primary-200 dark:border-primary-800 px-2 py-0.5 text-xs text-gray-700 dark:text-gray-300"
            >
              {{ pickerUserLabel(uid) }}
              <button
                type="button"
                class="text-gray-400 hover:text-red-500 dark:hover:text-red-400 ml-0.5"
                @click="selectedUserIds.delete(uid)"
              >×</button>
            </span>
          </div>
        </div>

        <!-- ── Advanced: manual ID input ── -->
        <div>
          <button
            type="button"
            class="flex items-center gap-1 text-xs text-gray-500 dark:text-gray-400 hover:text-gray-700 dark:hover:text-gray-200"
            @click="showAdvancedInput = !showAdvancedInput"
          >
            <Icon :name="showAdvancedInput ? 'chevronDown' : 'chevronRight'" size="sm" />
            {{ t('admin.userPools.addMembersModal.advancedInput') }}
          </button>
          <div v-if="showAdvancedInput" class="mt-2 space-y-2">
            <textarea
              v-model="addMembersInput"
              :placeholder="t('admin.userPools.addMembersModal.advancedInputPlaceholder')"
              class="input font-mono text-sm"
              rows="3"
            />
            <div class="flex items-center gap-2">
              <p class="flex-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.userPools.addMembersModal.advancedInputHint') }}</p>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                @click="mergeManualIds"
              >{{ t('admin.userPools.addMembersModal.mergeIds') }}</button>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeAddMembersModal">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="submitting || selectedUserIds.size === 0"
            @click="handleAddMembers"
          >
            <span v-if="submitting">{{ t('admin.userPools.saving') }}</span>
            <span v-else>{{ t('admin.userPools.addMembersModal.addNMembers', { count: selectedUserIds.size }) }}</span>
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Add All Matched Confirm -->
    <ConfirmDialog
      :show="showAddAllConfirm"
      :title="t('admin.userPools.addMembersModal.addAllMatchedConfirmTitle')"
      :message="t('admin.userPools.addMembersModal.addAllMatchedConfirmBody', { n: pickerTotal, name: detailPool?.name ?? '' })"
      :confirm-text="t('admin.userPools.addMembersModal.addAllMatched', { n: pickerTotal })"
      :cancel-text="t('common.cancel')"
      :danger="false"
      @confirm="handleAddAllByFilter"
      @cancel="showAddAllConfirm = false"
    />

    <!-- Grants Edit Modal -->
    <BaseDialog
      :show="showGrantsModal"
      :title="t('admin.userPools.groupGrantsTitle')"
      width="normal"
      @close="closeGrantsModal"
    >
      <div class="space-y-4">
        <!-- Existing grants editor -->
        <div
          v-for="(grant, index) in editGrants"
          :key="index"
          class="flex items-start gap-3 rounded-xl border border-gray-200 dark:border-dark-700 p-3"
        >
          <div class="flex-1 space-y-2">
            <div>
              <label class="label text-xs">{{ t('admin.userPools.grantGroupId') }}</label>
              <Select
                v-model="grant.group_id"
                :options="groupOptions"
                :placeholder="t('admin.userPools.selectGroup')"
                class="w-full"
              >
                <template #selected="{ option }">
                  <GroupBadge
                    v-if="option"
                    :name="(option as any).label"
                    :platform="(option as any).platform"
                    :subscription-type="(option as any).subscriptionType"
                    :show-rate="false"
                  />
                  <span v-else class="text-gray-400">{{ t('admin.userPools.selectGroup') }}</span>
                </template>
                <template #option="{ option, selected }">
                  <GroupOptionItem
                    :name="(option as any).label"
                    :platform="(option as any).platform"
                    :subscription-type="(option as any).subscriptionType"
                    :description="(option as any).description"
                    :selected="selected"
                  />
                </template>
              </Select>
            </div>
            <div class="grid grid-cols-2 gap-2">
              <div>
                <label class="label text-xs">{{ t('admin.userPools.grantRateMultiplier') }}</label>
                <input
                  v-model="grant.rate_multiplier_str"
                  type="number"
                  step="0.01"
                  min="0"
                  :placeholder="t('admin.userPools.grantRateDefault')"
                  class="input text-sm"
                />
              </div>
              <div>
                <label class="label text-xs">{{ t('admin.userPools.grantRpmOverride') }}</label>
                <input
                  v-model="grant.rpm_override_str"
                  type="number"
                  step="1"
                  min="0"
                  :placeholder="t('admin.userPools.grantRpmDefault')"
                  class="input text-sm"
                />
              </div>
            </div>
          </div>
          <button
            type="button"
            class="mt-6 text-red-500 hover:text-red-700 dark:text-red-400"
            @click="removeGrantRow(index)"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>

        <button type="button" class="btn btn-secondary w-full" @click="addGrantRow">
          <Icon name="plus" size="sm" class="mr-2" />
          {{ t('admin.userPools.addGrant') }}
        </button>
      </div>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeGrantsModal">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="submitting" @click="handleSaveGrants">
            {{ submitting ? t('admin.userPools.saving') : t('admin.userPools.saveGrants') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Remove Member Confirm -->
    <ConfirmDialog
      :show="showRemoveMemberConfirm"
      :title="t('admin.userPools.removeMembers')"
      :message="t('admin.userPools.confirmRemove', { count: 1 })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="handleRemoveMember"
      @cancel="showRemoveMemberConfirm = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted, onUnmounted, shallowRef } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { UserPool, UserPoolMember, UserPoolGroupGrant, PoolGroupGrantInput, AddMembersByFilterRequest } from '@/api/admin/userPools'
import type { AdminUser, UserAttributeDefinition, GroupPlatform, SubscriptionType } from '@/types'
import type { Column } from '@/components/common/types'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

// ── list state ────────────────────────────────────────────────────────────────

const pools = ref<UserPool[]>([])
const loading = ref(false)
const currentPage = ref(1)
const pageSize = ref(20)
const total = ref(0)
const searchQuery = ref('')
const filterStatus = ref('')

let searchDebounce: ReturnType<typeof setTimeout> | null = null

const allColumns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.userPools.columns.name') },
  { key: 'description', label: t('admin.userPools.columns.description') },
  { key: 'status', label: t('admin.userPools.columns.status') },
  { key: 'groups', label: t('admin.userPools.columns.groups') },
  { key: 'created_at', label: t('admin.userPools.columns.createdAt') },
  { key: 'actions', label: t('admin.userPools.columns.actions') }
])

const FORCED_VISIBLE_COLUMNS = new Set(['name', 'actions'])
const DEFAULT_HIDDEN_COLUMNS = ['description', 'groups']
const HIDDEN_COLUMNS_KEY = 'user-pool-hidden-columns'

const hiddenColumns = reactive<Set<string>>(new Set())
const showColumnDropdown = ref(false)
const columnDropdownRef = ref<HTMLElement | null>(null)

const toggleableColumns = computed(() =>
  allColumns.value.filter(col => !FORCED_VISIBLE_COLUMNS.has(col.key))
)

const columns = computed<Column[]>(() =>
  allColumns.value.filter(col => FORCED_VISIBLE_COLUMNS.has(col.key) || !hiddenColumns.has(col.key))
)

function loadSavedColumns() {
  try {
    const saved = localStorage.getItem(HIDDEN_COLUMNS_KEY)
    if (saved) {
      const parsed = JSON.parse(saved) as string[]
      parsed
        .filter(key => !FORCED_VISIBLE_COLUMNS.has(key))
        .forEach(key => hiddenColumns.add(key))
    } else {
      DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
    }
  } catch {
    DEFAULT_HIDDEN_COLUMNS.forEach(key => hiddenColumns.add(key))
  }
}

function saveColumnsToStorage() {
  try {
    localStorage.setItem(HIDDEN_COLUMNS_KEY, JSON.stringify([...hiddenColumns]))
  } catch {
    // ignore
  }
}

function toggleColumn(key: string) {
  if (hiddenColumns.has(key)) {
    hiddenColumns.delete(key)
  } else {
    hiddenColumns.add(key)
  }
  saveColumnsToStorage()
}

function isColumnVisible(key: string) {
  return !hiddenColumns.has(key)
}

function handleColumnDropdownOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (columnDropdownRef.value && !columnDropdownRef.value.contains(target)) {
    showColumnDropdown.value = false
  }
}

loadSavedColumns()

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.userPools.allStatus') },
  { value: 'active', label: t('admin.userPools.status.active') },
  { value: 'disabled', label: t('admin.userPools.status.disabled') }
])

const statusOptions = computed(() => [
  { value: 'active', label: t('admin.userPools.form.statusActive') },
  { value: 'disabled', label: t('admin.userPools.form.statusDisabled') }
])

async function loadPools() {
  loading.value = true
  try {
    const res = await adminAPI.userPools.list(
      currentPage.value,
      pageSize.value,
      filterStatus.value || undefined
    )
    pools.value = res.items
    total.value = res.total
    // 异步加载本页所有池的 grants，用于「分组」列展示
    // 每页最多 20 条，N+1 成本可接受；后端暂无批量 grants 接口
    loadPoolGrantsBatch(res.items)
  } catch {
    appStore.showError(t('admin.userPools.failedToLoad'))
  } finally {
    loading.value = false
  }
}

// ── pool grants map（列表页「分组」列用） ─────────────────────────────────────

// poolId -> UserPoolGroupGrant[]，用 shallowRef 提升大列表性能
const poolGrantsMap = shallowRef<Map<number, UserPoolGroupGrant[]>>(new Map())

async function loadPoolGrantsBatch(items: UserPool[]) {
  if (items.length === 0) return
  const results = await Promise.allSettled(
    items.map(p => adminAPI.userPools.listGroupGrants(p.id).then(r => ({ id: p.id, grants: r.grants })))
  )
  const next = new Map(poolGrantsMap.value)
  for (const r of results) {
    if (r.status === 'fulfilled') {
      next.set(r.value.id, r.value.grants)
    }
  }
  poolGrantsMap.value = next
}

interface EnrichedGrant {
  group_id: number
  group_name: string
  platform: GroupPlatform | undefined
  subscription_type: SubscriptionType | undefined
  is_exclusive: boolean | undefined
  rate_multiplier: number | null
}

function getPoolGrants(pool: UserPool): EnrichedGrant[] {
  const rawGrants = poolGrantsMap.value.get(pool.id)
  if (!rawGrants || rawGrants.length === 0) return []
  return rawGrants.map(gr => {
    const g = allGroups.value.find(x => x.id === gr.group_id)
    return {
      group_id: gr.group_id,
      group_name: g?.name ?? `#${gr.group_id}`,
      platform: g?.platform,
      subscription_type: g?.subscription_type,
      is_exclusive: g?.is_exclusive,
      rate_multiplier: gr.rate_multiplier
    }
  })
}

function handleSearch() {
  if (searchDebounce) clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => {
    currentPage.value = 1
    loadPools()
  }, 300)
}

function handlePageChange(page: number) {
  currentPage.value = page
  loadPools()
}

// ── create ────────────────────────────────────────────────────────────────────

const showCreateModal = ref(false)
const submitting = ref(false)
const createForm = ref({ name: '', description: '', status: 'active' })

function openCreateModal() {
  createForm.value = { name: '', description: '', status: 'active' }
  showCreateModal.value = true
}

function closeCreateModal() {
  showCreateModal.value = false
}

async function handleCreate() {
  if (!createForm.value.name.trim()) {
    appStore.showError(t('admin.userPools.form.nameRequired'))
    return
  }
  submitting.value = true
  try {
    await adminAPI.userPools.create({
      name: createForm.value.name.trim(),
      description: createForm.value.description,
      status: createForm.value.status
    })
    appStore.showSuccess(t('admin.userPools.poolCreated'))
    closeCreateModal()
    loadPools()
  } catch {
    appStore.showError(t('admin.userPools.failedToCreate'))
  } finally {
    submitting.value = false
  }
}

// ── edit ──────────────────────────────────────────────────────────────────────

const showEditModal = ref(false)
const editingPool = ref<UserPool | null>(null)
const editForm = ref({ name: '', description: '', status: 'active' })

function openEditModal(pool: UserPool) {
  editingPool.value = pool
  editForm.value = { name: pool.name, description: pool.description, status: pool.status }
  showEditModal.value = true
}

function closeEditModal() {
  showEditModal.value = false
  editingPool.value = null
}

async function handleUpdate() {
  if (!editingPool.value) return
  if (!editForm.value.name.trim()) {
    appStore.showError(t('admin.userPools.form.nameRequired'))
    return
  }
  submitting.value = true
  try {
    await adminAPI.userPools.update(editingPool.value.id, {
      name: editForm.value.name.trim(),
      description: editForm.value.description,
      status: editForm.value.status
    })
    appStore.showSuccess(t('admin.userPools.poolUpdated'))
    closeEditModal()
    loadPools()
    if (detailPool.value?.id === editingPool.value.id) {
      detailPool.value = { ...detailPool.value, ...editForm.value, name: editForm.value.name.trim() }
    }
  } catch {
    appStore.showError(t('admin.userPools.failedToUpdate'))
  } finally {
    submitting.value = false
  }
}

// ── delete ────────────────────────────────────────────────────────────────────

const showDeleteConfirm = ref(false)
const deletingPool = ref<UserPool | null>(null)

function confirmDelete(pool: UserPool) {
  deletingPool.value = pool
  showDeleteConfirm.value = true
}

async function handleDelete() {
  if (!deletingPool.value) return
  try {
    await adminAPI.userPools.delete(deletingPool.value.id)
    appStore.showSuccess(t('admin.userPools.poolDeleted'))
    showDeleteConfirm.value = false
    if (detailPool.value?.id === deletingPool.value.id) {
      closeDetail()
    }
    loadPools()
  } catch {
    appStore.showError(t('admin.userPools.failedToDelete'))
  } finally {
    deletingPool.value = null
  }
}

// ── detail drawer ─────────────────────────────────────────────────────────────

const showDetail = ref(false)
const detailPool = ref<UserPool | null>(null)

async function openDetail(pool: UserPool) {
  detailPool.value = pool
  showDetail.value = true
  membersPage.value = 1
  await Promise.all([loadMembers(), loadGrants(), loadAllGroups()])
}

function closeDetail() {
  showDetail.value = false
  detailPool.value = null
  members.value = []
  grants.value = []
}

// ── members ───────────────────────────────────────────────────────────────────

const members = ref<UserPoolMember[]>([])
const membersLoading = ref(false)
const membersPage = ref(1)
const membersPageSize = 10
const membersTotal = ref(0)

async function loadMembers() {
  if (!detailPool.value) return
  membersLoading.value = true
  try {
    const res = await adminAPI.userPools.listMembers(detailPool.value.id, membersPage.value, membersPageSize)
    members.value = res.items
    membersTotal.value = res.total
  } catch {
    appStore.showError(t('admin.userPools.failedToLoad'))
  } finally {
    membersLoading.value = false
  }
}

function handleMembersPageChange(page: number) {
  membersPage.value = page
  loadMembers()
}

// ── Add Members Modal (rich picker) ──────────────────────────────────────────

const showAddMembersModal = ref(false)
const addMembersInput = ref('')
const showAdvancedInput = ref(false)

// Selection: reactive Set wrapped in a ref so Vue tracks .size changes
const selectedUserIds = ref<Set<number>>(new Set())

// Picker filter state
const pickerSearch = ref('')
const pickerStatus = ref('')
const pickerGroup = ref('')
const pickerAttrFilters = reactive<Record<number, string>>({})

// Picker data state
const pickerUsers = ref<AdminUser[]>([])
const pickerLoading = ref(false)
const pickerPage = ref(1)
const pickerPageSize = 20
const pickerTotal = ref(0)
const pickerAttrDefs = ref<UserAttributeDefinition[]>([])

// Label cache: id -> display string (built from loaded pages)
const pickerLabelCache = reactive<Record<number, string>>({})

let pickerSearchTimeout: ReturnType<typeof setTimeout> | null = null
let pickerAbortController: AbortController | null = null

const pickerStatusOptions = computed(() => [
  { value: '', label: t('admin.userPools.addMembersModal.statusFilter') },
  { value: 'active', label: t('admin.userPools.status.active') },
  { value: 'disabled', label: t('admin.userPools.status.disabled') }
])

const pickerGroupOptions = computed(() => [
  { value: '', label: t('admin.userPools.addMembersModal.groupFilter') },
  ...allGroups.value.map(g => ({ value: g.name, label: g.name }))
])

const pickerPageAllSelected = computed(() =>
  pickerUsers.value.length > 0 && pickerUsers.value.every(u => selectedUserIds.value.has(u.id))
)

const pickerPagePartialSelected = computed(() =>
  pickerUsers.value.some(u => selectedUserIds.value.has(u.id)) && !pickerPageAllSelected.value
)

function userGroupNames(user: AdminUser): string[] {
  if (!user.allowed_groups || user.allowed_groups.length === 0) return []
  return user.allowed_groups
    .map(gid => {
      const g = allGroups.value.find(x => x.id === gid)
      return g ? g.name : `#${gid}`
    })
}

function pickerUserLabel(uid: number): string {
  return pickerLabelCache[uid] ?? `#${uid}`
}

async function loadPickerUsers(resetPage = false) {
  if (resetPage) pickerPage.value = 1

  if (pickerAbortController) pickerAbortController.abort()
  const ctrl = new AbortController()
  pickerAbortController = ctrl

  pickerLoading.value = true
  try {
    const attrFilters: Record<number, string> = {}
    for (const [k, v] of Object.entries(pickerAttrFilters)) {
      if (v) attrFilters[Number(k)] = v
    }
    const res = await adminAPI.users.list(
      pickerPage.value,
      pickerPageSize,
      {
        search: pickerSearch.value || undefined,
        status: (pickerStatus.value || undefined) as 'active' | 'disabled' | undefined,
        group_name: pickerGroup.value || undefined,
        attributes: Object.keys(attrFilters).length > 0 ? attrFilters : undefined
      },
      { signal: ctrl.signal }
    )
    if (ctrl.signal.aborted) return
    pickerUsers.value = res.items
    pickerTotal.value = res.total
    // Update label cache
    for (const u of res.items) {
      pickerLabelCache[u.id] = u.email || `#${u.id}`
    }
  } catch (e: unknown) {
    const err = e as { name?: string; code?: string }
    if (err?.name === 'AbortError' || err?.name === 'CanceledError' || err?.code === 'ERR_CANCELED') return
    appStore.showError(t('admin.userPools.failedToAddMembers'))
  } finally {
    if (pickerAbortController === ctrl) {
      pickerLoading.value = false
    }
  }
}

function handlePickerSearch() {
  if (pickerSearchTimeout) clearTimeout(pickerSearchTimeout)
  pickerSearchTimeout = setTimeout(() => loadPickerUsers(true), 300)
}

function handlePickerPageChange(page: number) {
  pickerPage.value = page
  loadPickerUsers(false)
}

function togglePickerUser(id: number) {
  const next = new Set(selectedUserIds.value)
  if (next.has(id)) {
    next.delete(id)
  } else {
    next.add(id)
  }
  selectedUserIds.value = next
}

function togglePickerPageAll() {
  const next = new Set(selectedUserIds.value)
  if (pickerPageAllSelected.value) {
    for (const u of pickerUsers.value) next.delete(u.id)
  } else {
    for (const u of pickerUsers.value) {
      next.add(u.id)
      pickerLabelCache[u.id] = u.email || `#${u.id}`
    }
  }
  selectedUserIds.value = next
}

function parseUserIds(input: string): number[] {
  return input
    .split(/[\s,\n]+/)
    .map(s => parseInt(s.trim(), 10))
    .filter(n => !isNaN(n) && n > 0)
}

function mergeManualIds() {
  const parsed = parseUserIds(addMembersInput.value)
  if (parsed.length === 0) return
  const next = new Set(selectedUserIds.value)
  for (const id of parsed) next.add(id)
  selectedUserIds.value = next
  addMembersInput.value = ''
}

async function openAddMembersModal() {
  pickerSearch.value = ''
  pickerStatus.value = ''
  pickerGroup.value = ''
  // clear attr filters
  for (const k of Object.keys(pickerAttrFilters)) {
    delete pickerAttrFilters[Number(k)]
  }
  selectedUserIds.value = new Set()
  addMembersInput.value = ''
  showAdvancedInput.value = false
  pickerPage.value = 1
  pickerUsers.value = []
  pickerTotal.value = 0
  showAddMembersModal.value = true

  // Load attribute definitions and initial users in parallel
  const [, attrDefs] = await Promise.all([
    loadPickerUsers(true),
    adminAPI.userAttributes.listEnabledDefinitions().catch(() => [] as UserAttributeDefinition[])
  ])
  pickerAttrDefs.value = attrDefs
}

function closeAddMembersModal() {
  showAddMembersModal.value = false
  if (pickerAbortController) pickerAbortController.abort()
}

async function handleAddMembers() {
  if (!detailPool.value) return
  const unique = [...selectedUserIds.value]
  if (unique.length === 0) return
  submitting.value = true
  try {
    const res = await adminAPI.userPools.addMembers(detailPool.value.id, unique)
    appStore.showSuccess(
      t('admin.userPools.addMembersModal.addMembersSuccess', { added: res.added, skipped: res.skipped })
    )
    closeAddMembersModal()
    loadMembers()
  } catch {
    appStore.showError(t('admin.userPools.failedToAddMembers'))
  } finally {
    submitting.value = false
  }
}

// ── Add all by filter ────────────────────────────────────────────────────────

const showAddAllConfirm = ref(false)
const addAllSubmitting = ref(false)

const hasAnyPickerFilter = computed(() => {
  if (pickerSearch.value || pickerStatus.value || pickerGroup.value) return true
  return Object.values(pickerAttrFilters).some(v => !!v)
})

function openAddAllConfirm() {
  if (!hasAnyPickerFilter.value) return
  showAddAllConfirm.value = true
}

async function handleAddAllByFilter() {
  if (!detailPool.value) return
  showAddAllConfirm.value = false
  addAllSubmitting.value = true
  const filters: AddMembersByFilterRequest = {
    search: pickerSearch.value || undefined,
    status: (pickerStatus.value || undefined) as 'active' | 'disabled' | undefined,
    group_name: pickerGroup.value || undefined,
  }
  const attrFilters: Record<number, string> = {}
  for (const [k, v] of Object.entries(pickerAttrFilters)) {
    if (v) attrFilters[Number(k)] = v
  }
  if (Object.keys(attrFilters).length > 0) {
    filters.attributes = attrFilters
  }
  try {
    const res = await adminAPI.userPools.addMembersByFilter(detailPool.value.id, filters)
    appStore.showSuccess(
      t('admin.userPools.addMembersModal.addAllMatchedSuccess', {
        added: res.added,
        skipped: res.skipped,
        matched: res.matched
      })
    )
    closeAddMembersModal()
    loadMembers()
  } catch (e: unknown) {
    const err = e as { response?: { data?: { message?: string } } }
    appStore.showError(err?.response?.data?.message ?? t('admin.userPools.failedToAddMembers'))
  } finally {
    addAllSubmitting.value = false
  }
}

// Remove member
const showRemoveMemberConfirm = ref(false)
const removingMemberId = ref<number | null>(null)

function confirmRemoveMember(userId: number) {
  removingMemberId.value = userId
  showRemoveMemberConfirm.value = true
}

async function handleRemoveMember() {
  if (!detailPool.value || removingMemberId.value === null) return
  try {
    await adminAPI.userPools.removeMembers(detailPool.value.id, [removingMemberId.value])
    appStore.showSuccess(t('admin.userPools.membersRemoved'))
    showRemoveMemberConfirm.value = false
    loadMembers()
  } catch {
    appStore.showError(t('admin.userPools.failedToRemoveMembers'))
  } finally {
    removingMemberId.value = null
  }
}

// ── group grants ──────────────────────────────────────────────────────────────

const grants = ref<UserPoolGroupGrant[]>([])
const grantsLoading = ref(false)

async function loadGrants() {
  if (!detailPool.value) return
  grantsLoading.value = true
  try {
    const res = await adminAPI.userPools.listGroupGrants(detailPool.value.id)
    grants.value = res.grants
  } catch {
    appStore.showError(t('admin.userPools.failedToLoad'))
  } finally {
    grantsLoading.value = false
  }
}

async function handleDeleteGrant(groupId: number) {
  if (!detailPool.value) return
  try {
    await adminAPI.userPools.deleteGroupGrant(detailPool.value.id, groupId)
    appStore.showSuccess(t('admin.userPools.grantDeleted'))
    loadGrants()
  } catch {
    appStore.showError(t('admin.userPools.failedToDeleteGrant'))
  }
}

// Groups data for dropdowns
const allGroups = ref<{
  id: number
  name: string
  description: string | null
  platform: GroupPlatform
  subscription_type: SubscriptionType
  is_exclusive: boolean
}[]>([])

async function loadAllGroups() {
  try {
    const res = await adminAPI.groups.getAll()
    allGroups.value = res.map(g => ({
      id: g.id,
      name: g.name,
      description: g.description,
      platform: g.platform,
      subscription_type: g.subscription_type,
      is_exclusive: g.is_exclusive
    }))
  } catch {
    // non-fatal
  }
}

function getGroupName(groupId: number): string {
  const g = allGroups.value.find(x => x.id === groupId)
  return g ? g.name : `#${groupId}`
}

// 用户池授权只允许「订阅分组」和「专属分组」，公开分组（standard + 非专属）不可分配
const groupOptions = computed(() =>
  allGroups.value
    .filter(g => g.subscription_type === 'subscription' || g.is_exclusive)
    .map(g => ({
      value: g.id,
      label: g.name,
      description: g.description,
      platform: g.platform,
      subscriptionType: g.subscription_type,
      groupType: g.is_exclusive ? 'exclusive' : 'standard' as 'exclusive' | 'standard'
    }))
)

// Grants edit modal
interface EditGrantRow {
  group_id: number | null
  rate_multiplier_str: string
  rpm_override_str: string
}

const showGrantsModal = ref(false)
const editGrants = ref<EditGrantRow[]>([])

function openGrantsEditModal() {
  editGrants.value = grants.value.map(g => ({
    group_id: g.group_id,
    rate_multiplier_str: g.rate_multiplier != null ? String(g.rate_multiplier) : '',
    rpm_override_str: g.rpm_override != null ? String(g.rpm_override) : ''
  }))
  if (editGrants.value.length === 0) {
    addGrantRow()
  }
  showGrantsModal.value = true
}

function closeGrantsModal() {
  showGrantsModal.value = false
}

function addGrantRow() {
  editGrants.value.push({ group_id: null, rate_multiplier_str: '', rpm_override_str: '' })
}

function removeGrantRow(index: number) {
  editGrants.value.splice(index, 1)
}

async function handleSaveGrants() {
  if (!detailPool.value) return
  const payload: PoolGroupGrantInput[] = editGrants.value
    .filter(g => g.group_id != null)
    .map(g => ({
      group_id: g.group_id as number,
      rate_multiplier: g.rate_multiplier_str !== '' ? parseFloat(g.rate_multiplier_str) : null,
      rpm_override: g.rpm_override_str !== '' ? parseInt(g.rpm_override_str, 10) : null
    }))
  submitting.value = true
  try {
    const res = await adminAPI.userPools.replaceGroupGrants(detailPool.value.id, payload)
    grants.value = res.grants
    appStore.showSuccess(t('admin.userPools.grantsSaved'))
    closeGrantsModal()
  } catch {
    appStore.showError(t('admin.userPools.failedToSaveGrants'))
  } finally {
    submitting.value = false
  }
}

// ── utils ─────────────────────────────────────────────────────────────────────

function formatDate(iso: string): string {
  if (!iso) return '—'
  return new Date(iso).toLocaleDateString()
}

// ── init ──────────────────────────────────────────────────────────────────────

onMounted(() => {
  // 同步加载分组元数据（用于列表「分组」列 badge 渲染）和池列表
  loadAllGroups()
  loadPools()
  document.addEventListener('click', handleColumnDropdownOutside)
})

onUnmounted(() => {
  document.removeEventListener('click', handleColumnDropdownOutside)
})
</script>

<style scoped>
.drawer-enter-active,
.drawer-leave-active {
  transition: opacity 0.2s ease;
}
.drawer-enter-active .relative.z-10,
.drawer-leave-active .relative.z-10 {
  transition: transform 0.25s ease;
}
.drawer-enter-from {
  opacity: 0;
}
.drawer-leave-to {
  opacity: 0;
}
.drawer-enter-from .relative.z-10 {
  transform: translateX(100%);
}
.drawer-leave-to .relative.z-10 {
  transform: translateX(100%);
}
</style>
