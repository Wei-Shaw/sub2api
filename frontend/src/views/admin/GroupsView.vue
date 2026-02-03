<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
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
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadGroups"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button
              @click="showCreateModal = true"
              class="btn btn-primary"
              data-tour="groups-create-btn"
            >
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.groups.createGroup') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="groups" :loading="loading">
          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-platform="{ value REDACTED">
            <span
              :class="[
                'inline-flex items-center gap-1.5 rounded-full px-2.5 py-0.5 text-xs font-medium',
                value === 'anthropic'
                  ? 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
                  : value === 'openai'
                    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
                    : value === 'antigravity'
                      ? 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
                      : 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
              ]"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ t('admin.groups.platforms.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-billing_type="{ row REDACTED">
            <div class="space-y-1">
              <!-- Type Badge -->
              <span
                :class="[
                  'inline-block rounded-full px-2 py-0.5 text-xs font-medium',
                  row.subscription_type === 'subscription'
                    ? 'bg-violet-100 text-violet-700 dark:bg-violet-900/30 dark:text-violet-400'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
                ]"
              >
                {{
                  row.subscription_type === 'subscription'
                    ? t('admin.groups.subscription.subscription')
                    : t('admin.groups.subscription.standard')
                REDACTEDREDACTED
              </span>
              <!-- Subscription Limits - compact single line -->
              <div
                v-if="row.subscription_type === 'subscription'"
                class="text-xs text-gray-500 dark:text-gray-400"
              >
                <template
                  v-if="row.daily_limit_usd || row.weekly_limit_usd || row.monthly_limit_usd"
                >
                  <span v-if="row.daily_limit_usd"
                    >${{ row.daily_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitDay') REDACTEDREDACTED</span
                  >
                  <span
                    v-if="row.daily_limit_usd && (row.weekly_limit_usd || row.monthly_limit_usd)"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.weekly_limit_usd"
                    >${{ row.weekly_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitWeek') REDACTEDREDACTED</span
                  >
                  <span
                    v-if="row.weekly_limit_usd && row.monthly_limit_usd"
                    class="mx-1 text-gray-300 dark:text-gray-600"
                    >·</span
                  >
                  <span v-if="row.monthly_limit_usd"
                    >${{ row.monthly_limit_usd REDACTEDREDACTED/{{ t('admin.groups.limitMonth') REDACTEDREDACTED</span
                  >
                </template>
                <span v-else class="text-gray-400 dark:text-gray-500">{{
                  t('admin.groups.subscription.noLimit')
                REDACTEDREDACTED</span>
              </div>
            </div>
          </template>

          <template #cell-rate_multiplier="{ value REDACTED">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ value REDACTEDREDACTEDx</span>
          </template>

          <template #cell-is_exclusive="{ value REDACTED">
            <span :class="['badge', value ? 'badge-primary' : 'badge-gray']">
              {{ value ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-account_count="{ value REDACTED">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ t('admin.groups.accountsCount', { count: value || 0 REDACTED) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-status="{ value REDACTED">
            <span :class="['badge', value === 'active' ? 'badge-success' : 'badge-danger']">
              {{ t('admin.accounts.status.' + value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                @click="handleEdit(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') REDACTEDREDACTED</span>
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
      <form id="create-group-form" @submit.prevent="handleCreateGroup" class="space-y-5">
        <div>
          <label class="input-label">{{ t('admin.groups.form.name') REDACTEDREDACTED</label>
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
          <label class="input-label">{{ t('admin.groups.form.description') REDACTEDREDACTED</label>
          <textarea
            v-model="createForm.description"
            rows="3"
            class="input"
            :placeholder="t('admin.groups.optionalDescription')"
          ></textarea>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.platform') REDACTEDREDACTED</label>
          <Select
            v-model="createForm.platform"
            :options="platformOptions"
            data-tour="group-form-platform"
            @change="createForm.copy_accounts_from_group_ids = []"
          />
          <p class="input-hint">{{ t('admin.groups.platformHint') REDACTEDREDACTED</p>
        </div>
        <!-- 从分组复制账号 -->
        <div v-if="copyAccountsGroupOptions.length > 0">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.copyAccounts.title') REDACTEDREDACTED
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.copyAccounts.tooltip') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 已选分组标签 -->
          <div v-if="createForm.copy_accounts_from_group_ids.length > 0" class="flex flex-wrap gap-1.5 mb-2">
            <span
              v-for="groupId in createForm.copy_accounts_from_group_ids"
              :key="groupId"
              class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{ copyAccountsGroupOptions.find(o => o.value === groupId)?.label || `#${groupIdREDACTED` REDACTEDREDACTED
              <button
                type="button"
                @click="createForm.copy_accounts_from_group_ids = createForm.copy_accounts_from_group_ids.filter(id => id !== groupId)"
                class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <!-- 分组选择下拉 -->
          <select
            class="input"
            @change="(e) => {
              const val = Number((e.target as HTMLSelectElement).value)
              if (val && !createForm.copy_accounts_from_group_ids.includes(val)) {
                createForm.copy_accounts_from_group_ids.push(val)
              REDACTED
              (e.target as HTMLSelectElement).value = ''
            REDACTED"
          >
            <option value="">{{ t('admin.groups.copyAccounts.selectPlaceholder') REDACTEDREDACTED</option>
            <option
              v-for="opt in copyAccountsGroupOptions"
              :key="opt.value"
              :value="opt.value"
              :disabled="createForm.copy_accounts_from_group_ids.includes(opt.value)"
            >
              {{ opt.label REDACTEDREDACTED
            </option>
          </select>
          <p class="input-hint">{{ t('admin.groups.copyAccounts.hint') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.rateMultiplier') REDACTEDREDACTED</label>
          <input
            v-model.number="createForm.rate_multiplier"
            type="number"
            step="0.001"
            min="0.001"
            required
            class="input"
            data-tour="group-form-multiplier"
          />
          <p class="input-hint">{{ t('admin.groups.rateMultiplierHint') REDACTEDREDACTED</p>
        </div>
        <div v-if="createForm.subscription_type !== 'subscription'" data-tour="group-form-exclusive">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.form.exclusive') REDACTEDREDACTED
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
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="mb-2 text-xs font-medium">{{ t('admin.groups.exclusiveTooltip.title') REDACTEDREDACTED</p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.exclusiveTooltip.description') REDACTEDREDACTED
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span class="inline-flex items-center gap-1 text-primary-400"><Icon name="lightbulb" size="xs" /> {{ t('admin.groups.exclusiveTooltip.example') REDACTEDREDACTED</span>
                      {{ t('admin.groups.exclusiveTooltip.exampleContent') REDACTEDREDACTED
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
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
                createForm.is_exclusive ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.is_exclusive ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ createForm.is_exclusive ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </div>
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{ t('admin.groups.subscription.type') REDACTEDREDACTED</label>
            <Select v-model="createForm.subscription_type" :options="subscriptionTypeOptions" />
            <p class="input-hint">{{ t('admin.groups.subscription.typeHint') REDACTEDREDACTED</p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="createForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.dailyLimit') REDACTEDREDACTED</label>
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
              <label class="input-label">{{ t('admin.groups.subscription.weeklyLimit') REDACTEDREDACTED</label>
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
              <label class="input-label">{{ t('admin.groups.subscription.monthlyLimit') REDACTEDREDACTED</label>
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

        <!-- 图片生成计费配置（antigravity 和 gemini 平台） -->
        <div v-if="createForm.platform === 'antigravity' || createForm.platform === 'gemini'" class="border-t pt-4">
          <label class="block mb-2 font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.groups.imagePricing.title') REDACTEDREDACTED
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.imagePricing.description') REDACTEDREDACTED
          </p>
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
                placeholder="0.134"
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
        </div>

        <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
        <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.claudeCode.title') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.claudeCode.tooltip') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              @click="createForm.claude_code_only = !createForm.claude_code_only"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.claude_code_only ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.claude_code_only ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ createForm.claude_code_only ? t('admin.groups.claudeCode.enabled') : t('admin.groups.claudeCode.disabled') REDACTEDREDACTED
            </span>
          </div>
          <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
          <div v-if="createForm.claude_code_only" class="mt-3">
            <label class="input-label">{{ t('admin.groups.claudeCode.fallbackGroup') REDACTEDREDACTED</label>
            <Select
              v-model="createForm.fallback_group_id"
              :options="fallbackGroupOptions"
              :placeholder="t('admin.groups.claudeCode.noFallback')"
            />
            <p class="input-hint">{{ t('admin.groups.claudeCode.fallbackHint') REDACTEDREDACTED</p>
          </div>
        </div>

        <!-- 模型路由配置（仅 anthropic 平台） -->
        <div v-if="createForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.modelRouting.title') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.modelRouting.tooltip') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 启用开关 -->
          <div class="flex items-center gap-3 mb-3">
            <button
              type="button"
              @click="createForm.model_routing_enabled = !createForm.model_routing_enabled"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                createForm.model_routing_enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  createForm.model_routing_enabled ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ createForm.model_routing_enabled ? t('admin.groups.modelRouting.enabled') : t('admin.groups.modelRouting.disabled') REDACTEDREDACTED
            </span>
          </div>
          <p v-if="!createForm.model_routing_enabled" class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.modelRouting.disabledHint') REDACTEDREDACTED
          </p>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.modelRouting.noRulesHint') REDACTEDREDACTED
          </p>
          <!-- 路由规则列表（仅在启用时显示） -->
          <div v-if="createForm.model_routing_enabled" class="space-y-3">
            <div
              v-for="(rule, index) in createModelRoutingRules"
              :key="index"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 space-y-2">
                  <div>
                    <label class="input-label text-xs">{{ t('admin.groups.modelRouting.modelPattern') REDACTEDREDACTED</label>
                    <input
                      v-model="rule.pattern"
                      type="text"
                      class="input text-sm"
                      :placeholder="t('admin.groups.modelRouting.modelPatternPlaceholder')"
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">{{ t('admin.groups.modelRouting.accounts') REDACTEDREDACTED</label>
                    <!-- 已选账号标签 -->
                    <div v-if="rule.accounts.length > 0" class="flex flex-wrap gap-1.5 mb-2">
                      <span
                        v-for="account in rule.accounts"
                        :key="account.id"
                        class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                      >
                        {{ account.name REDACTEDREDACTED
                        <button
                          type="button"
                          @click="removeSelectedAccount(index, account.id, false)"
                          class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                        >
                          <Icon name="x" size="xs" />
                        </button>
                      </span>
                    </div>
                    <!-- 账号搜索输入框 -->
                    <div class="relative account-search-container">
                      <input
                        v-model="accountSearchKeyword[`create-${indexREDACTED`]"
                        type="text"
                        class="input text-sm"
                        :placeholder="t('admin.groups.modelRouting.searchAccountPlaceholder')"
                        @input="searchAccounts(`create-${indexREDACTED`)"
                        @focus="onAccountSearchFocus(index, false)"
                      />
                      <!-- 搜索结果下拉框 -->
                      <div
                        v-if="showAccountDropdown[`create-${indexREDACTED`] && accountSearchResults[`create-${indexREDACTED`]?.length > 0"
                        class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                      >
                        <button
                          v-for="account in accountSearchResults[`create-${indexREDACTED`]"
                          :key="account.id"
                          type="button"
                          @click="selectAccount(index, account, false)"
                          class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                          :class="{ 'opacity-50': rule.accounts.some(a => a.id === account.id) REDACTED"
                          :disabled="rule.accounts.some(a => a.id === account.id)"
                        >
                          <span>{{ account.name REDACTEDREDACTED</span>
                          <span class="ml-2 text-xs text-gray-400">#{{ account.id REDACTEDREDACTED</span>
                        </button>
                      </div>
                    </div>
                    <p class="text-xs text-gray-400 mt-1">{{ t('admin.groups.modelRouting.accountsHint') REDACTEDREDACTED</p>
                  </div>
                </div>
                <button
                  type="button"
                  @click="removeCreateRoutingRule(index)"
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
            {{ t('admin.groups.modelRouting.addRule') REDACTEDREDACTED
          </button>
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button @click="closeCreateModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
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
            {{ submitting ? t('admin.groups.creating') : t('common.create') REDACTEDREDACTED
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
          <label class="input-label">{{ t('admin.groups.form.name') REDACTEDREDACTED</label>
          <input
            v-model="editForm.name"
            type="text"
            required
            class="input"
            data-tour="edit-group-form-name"
          />
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.description') REDACTEDREDACTED</label>
          <textarea v-model="editForm.description" rows="3" class="input"></textarea>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.platform') REDACTEDREDACTED</label>
          <Select
            v-model="editForm.platform"
            :options="platformOptions"
            :disabled="true"
            data-tour="group-form-platform"
          />
          <p class="input-hint">{{ t('admin.groups.platformNotEditable') REDACTEDREDACTED</p>
        </div>
        <!-- 从分组复制账号（编辑时） -->
        <div v-if="copyAccountsGroupOptionsForEdit.length > 0">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.copyAccounts.title') REDACTEDREDACTED
            </label>
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.copyAccounts.tooltipEdit') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 已选分组标签 -->
          <div v-if="editForm.copy_accounts_from_group_ids.length > 0" class="flex flex-wrap gap-1.5 mb-2">
            <span
              v-for="groupId in editForm.copy_accounts_from_group_ids"
              :key="groupId"
              class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
            >
              {{ copyAccountsGroupOptionsForEdit.find(o => o.value === groupId)?.label || `#${groupIdREDACTED` REDACTEDREDACTED
              <button
                type="button"
                @click="editForm.copy_accounts_from_group_ids = editForm.copy_accounts_from_group_ids.filter(id => id !== groupId)"
                class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
              >
                <Icon name="x" size="xs" />
              </button>
            </span>
          </div>
          <!-- 分组选择下拉 -->
          <select
            class="input"
            @change="(e) => {
              const val = Number((e.target as HTMLSelectElement).value)
              if (val && !editForm.copy_accounts_from_group_ids.includes(val)) {
                editForm.copy_accounts_from_group_ids.push(val)
              REDACTED
              (e.target as HTMLSelectElement).value = ''
            REDACTED"
          >
            <option value="">{{ t('admin.groups.copyAccounts.selectPlaceholder') REDACTEDREDACTED</option>
            <option
              v-for="opt in copyAccountsGroupOptionsForEdit"
              :key="opt.value"
              :value="opt.value"
              :disabled="editForm.copy_accounts_from_group_ids.includes(opt.value)"
            >
              {{ opt.label REDACTEDREDACTED
            </option>
          </select>
          <p class="input-hint">{{ t('admin.groups.copyAccounts.hintEdit') REDACTEDREDACTED</p>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.rateMultiplier') REDACTEDREDACTED</label>
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
        <div v-if="editForm.subscription_type !== 'subscription'">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.form.exclusive') REDACTEDREDACTED
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
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="mb-2 text-xs font-medium">{{ t('admin.groups.exclusiveTooltip.title') REDACTEDREDACTED</p>
                  <p class="mb-2 text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.exclusiveTooltip.description') REDACTEDREDACTED
                  </p>
                  <div class="rounded bg-gray-800 p-2 dark:bg-gray-700">
                    <p class="text-xs leading-relaxed text-gray-300">
                      <span class="inline-flex items-center gap-1 text-primary-400"><Icon name="lightbulb" size="xs" /> {{ t('admin.groups.exclusiveTooltip.example') REDACTEDREDACTED</span>
                      {{ t('admin.groups.exclusiveTooltip.exampleContent') REDACTEDREDACTED
                    </p>
                  </div>
                  <!-- Arrow -->
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
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
                editForm.is_exclusive ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.is_exclusive ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ editForm.is_exclusive ? t('admin.groups.exclusive') : t('admin.groups.public') REDACTEDREDACTED
            </span>
          </div>
        </div>
        <div>
          <label class="input-label">{{ t('admin.groups.form.status') REDACTEDREDACTED</label>
          <Select v-model="editForm.status" :options="editStatusOptions" />
        </div>

        <!-- Subscription Configuration -->
        <div class="mt-4 border-t pt-4">
          <div>
            <label class="input-label">{{ t('admin.groups.subscription.type') REDACTEDREDACTED</label>
            <Select
              v-model="editForm.subscription_type"
              :options="subscriptionTypeOptions"
              :disabled="true"
            />
            <p class="input-hint">{{ t('admin.groups.subscription.typeNotEditable') REDACTEDREDACTED</p>
          </div>

          <!-- Subscription limits (only show when subscription type is selected) -->
          <div
            v-if="editForm.subscription_type === 'subscription'"
            class="space-y-4 border-l-2 border-primary-200 pl-4 dark:border-primary-800"
          >
            <div>
              <label class="input-label">{{ t('admin.groups.subscription.dailyLimit') REDACTEDREDACTED</label>
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
              <label class="input-label">{{ t('admin.groups.subscription.weeklyLimit') REDACTEDREDACTED</label>
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
              <label class="input-label">{{ t('admin.groups.subscription.monthlyLimit') REDACTEDREDACTED</label>
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

        <!-- 图片生成计费配置（antigravity 和 gemini 平台） -->
        <div v-if="editForm.platform === 'antigravity' || editForm.platform === 'gemini'" class="border-t pt-4">
          <label class="block mb-2 font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.groups.imagePricing.title') REDACTEDREDACTED
          </label>
          <p class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.imagePricing.description') REDACTEDREDACTED
          </p>
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
                placeholder="0.134"
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
        </div>

        <!-- Claude Code 客户端限制（仅 anthropic 平台） -->
        <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.claudeCode.title') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-72 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.claudeCode.tooltip') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
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
                editForm.claude_code_only ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.claude_code_only ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ editForm.claude_code_only ? t('admin.groups.claudeCode.enabled') : t('admin.groups.claudeCode.disabled') REDACTEDREDACTED
            </span>
          </div>
          <!-- 降级分组选择（仅当启用 claude_code_only 时显示） -->
          <div v-if="editForm.claude_code_only" class="mt-3">
            <label class="input-label">{{ t('admin.groups.claudeCode.fallbackGroup') REDACTEDREDACTED</label>
            <Select
              v-model="editForm.fallback_group_id"
              :options="fallbackGroupOptionsForEdit"
              :placeholder="t('admin.groups.claudeCode.noFallback')"
            />
            <p class="input-hint">{{ t('admin.groups.claudeCode.fallbackHint') REDACTEDREDACTED</p>
          </div>
        </div>

        <!-- 模型路由配置（仅 anthropic 平台） -->
        <div v-if="editForm.platform === 'anthropic'" class="border-t pt-4">
          <div class="mb-1.5 flex items-center gap-1">
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.groups.modelRouting.title') REDACTEDREDACTED
            </label>
            <!-- Help Tooltip -->
            <div class="group relative inline-flex">
              <Icon
                name="questionCircle"
                size="sm"
                :stroke-width="2"
                class="cursor-help text-gray-400 transition-colors hover:text-primary-500 dark:text-gray-500 dark:hover:text-primary-400"
              />
              <div class="pointer-events-none absolute bottom-full left-0 z-50 mb-2 w-80 opacity-0 transition-all duration-200 group-hover:pointer-events-auto group-hover:opacity-100">
                <div class="rounded-lg bg-gray-900 p-3 text-white shadow-lg dark:bg-gray-800">
                  <p class="text-xs leading-relaxed text-gray-300">
                    {{ t('admin.groups.modelRouting.tooltip') REDACTEDREDACTED
                  </p>
                  <div class="absolute -bottom-1.5 left-3 h-3 w-3 rotate-45 bg-gray-900 dark:bg-gray-800"></div>
                </div>
              </div>
            </div>
          </div>
          <!-- 启用开关 -->
          <div class="flex items-center gap-3 mb-3">
            <button
              type="button"
              @click="editForm.model_routing_enabled = !editForm.model_routing_enabled"
              :class="[
                'relative inline-flex h-6 w-11 items-center rounded-full transition-colors',
                editForm.model_routing_enabled ? 'bg-primary-500' : 'bg-gray-300 dark:bg-dark-600'
              ]"
            >
              <span
                :class="[
                  'inline-block h-4 w-4 transform rounded-full bg-white shadow transition-transform',
                  editForm.model_routing_enabled ? 'translate-x-6' : 'translate-x-1'
                ]"
              />
            </button>
            <span class="text-sm text-gray-500 dark:text-gray-400">
              {{ editForm.model_routing_enabled ? t('admin.groups.modelRouting.enabled') : t('admin.groups.modelRouting.disabled') REDACTEDREDACTED
            </span>
          </div>
          <p v-if="!editForm.model_routing_enabled" class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.modelRouting.disabledHint') REDACTEDREDACTED
          </p>
          <p v-else class="text-xs text-gray-500 dark:text-gray-400 mb-3">
            {{ t('admin.groups.modelRouting.noRulesHint') REDACTEDREDACTED
          </p>
          <!-- 路由规则列表（仅在启用时显示） -->
          <div v-if="editForm.model_routing_enabled" class="space-y-3">
            <div
              v-for="(rule, index) in editModelRoutingRules"
              :key="index"
              class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
            >
              <div class="flex items-start gap-3">
                <div class="flex-1 space-y-2">
                  <div>
                    <label class="input-label text-xs">{{ t('admin.groups.modelRouting.modelPattern') REDACTEDREDACTED</label>
                    <input
                      v-model="rule.pattern"
                      type="text"
                      class="input text-sm"
                      :placeholder="t('admin.groups.modelRouting.modelPatternPlaceholder')"
                    />
                  </div>
                  <div>
                    <label class="input-label text-xs">{{ t('admin.groups.modelRouting.accounts') REDACTEDREDACTED</label>
                    <!-- 已选账号标签 -->
                    <div v-if="rule.accounts.length > 0" class="flex flex-wrap gap-1.5 mb-2">
                      <span
                        v-for="account in rule.accounts"
                        :key="account.id"
                        class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2.5 py-1 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
                      >
                        {{ account.name REDACTEDREDACTED
                        <button
                          type="button"
                          @click="removeSelectedAccount(index, account.id, true)"
                          class="ml-0.5 text-primary-500 hover:text-primary-700 dark:hover:text-primary-200"
                        >
                          <Icon name="x" size="xs" />
                        </button>
                      </span>
                    </div>
                    <!-- 账号搜索输入框 -->
                    <div class="relative account-search-container">
                      <input
                        v-model="accountSearchKeyword[`edit-${indexREDACTED`]"
                        type="text"
                        class="input text-sm"
                        :placeholder="t('admin.groups.modelRouting.searchAccountPlaceholder')"
                        @input="searchAccounts(`edit-${indexREDACTED`)"
                        @focus="onAccountSearchFocus(index, true)"
                      />
                      <!-- 搜索结果下拉框 -->
                      <div
                        v-if="showAccountDropdown[`edit-${indexREDACTED`] && accountSearchResults[`edit-${indexREDACTED`]?.length > 0"
                        class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                      >
                        <button
                          v-for="account in accountSearchResults[`edit-${indexREDACTED`]"
                          :key="account.id"
                          type="button"
                          @click="selectAccount(index, account, true)"
                          class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                          :class="{ 'opacity-50': rule.accounts.some(a => a.id === account.id) REDACTED"
                          :disabled="rule.accounts.some(a => a.id === account.id)"
                        >
                          <span>{{ account.name REDACTEDREDACTED</span>
                          <span class="ml-2 text-xs text-gray-400">#{{ account.id REDACTEDREDACTED</span>
                        </button>
                      </div>
                    </div>
                    <p class="text-xs text-gray-400 mt-1">{{ t('admin.groups.modelRouting.accountsHint') REDACTEDREDACTED</p>
                  </div>
                </div>
                <button
                  type="button"
                  @click="removeEditRoutingRule(index)"
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
            {{ t('admin.groups.modelRouting.addRule') REDACTEDREDACTED
          </button>
        </div>

      </form>

      <template #footer>
        <div class="flex justify-end gap-3 pt-4">
          <button @click="closeEditModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') REDACTEDREDACTED
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
            {{ submitting ? t('admin.groups.updating') : t('common.update') REDACTEDREDACTED
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
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { useOnboardingStore REDACTED from '@/stores/onboarding'
import { adminAPI REDACTED from '@/api/admin'
import type { AdminGroup, GroupPlatform, SubscriptionType REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()
const onboardingStore = useOnboardingStore()

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.groups.columns.name'), sortable: true REDACTED,
  { key: 'platform', label: t('admin.groups.columns.platform'), sortable: true REDACTED,
  { key: 'billing_type', label: t('admin.groups.columns.billingType'), sortable: true REDACTED,
  { key: 'rate_multiplier', label: t('admin.groups.columns.rateMultiplier'), sortable: true REDACTED,
  { key: 'is_exclusive', label: t('admin.groups.columns.type'), sortable: true REDACTED,
  { key: 'account_count', label: t('admin.groups.columns.accounts'), sortable: true REDACTED,
  { key: 'status', label: t('admin.groups.columns.status'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.groups.columns.actions'), sortable: false REDACTED
])

// Filter options
const statusOptions = computed(() => [
  { value: '', label: t('admin.groups.allStatus') REDACTED,
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const exclusiveOptions = computed(() => [
  { value: '', label: t('admin.groups.allGroups') REDACTED,
  { value: 'true', label: t('admin.groups.exclusive') REDACTED,
  { value: 'false', label: t('admin.groups.nonExclusive') REDACTED
])

const platformOptions = computed(() => [
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.groups.allPlatforms') REDACTED,
  { value: 'anthropic', label: 'Anthropic' REDACTED,
  { value: 'openai', label: 'OpenAI' REDACTED,
  { value: 'gemini', label: 'Gemini' REDACTED,
  { value: 'antigravity', label: 'Antigravity' REDACTED
])

const editStatusOptions = computed(() => [
  { value: 'active', label: t('admin.accounts.status.active') REDACTED,
  { value: 'inactive', label: t('admin.accounts.status.inactive') REDACTED
])

const subscriptionTypeOptions = computed(() => [
  { value: 'standard', label: t('admin.groups.subscription.standard') REDACTED,
  { value: 'subscription', label: t('admin.groups.subscription.subscription') REDACTED
])

// 降级分组选项（创建时）- 仅包含 anthropic 平台且未启用 claude_code_only 的分组
const fallbackGroupOptions = computed(() => {
  const options: { value: number | null; label: string REDACTED[] = [
    { value: null, label: t('admin.groups.claudeCode.noFallback') REDACTED
  ]
  const eligibleGroups = groups.value.filter(
    (g) => g.platform === 'anthropic' && !g.claude_code_only && g.status === 'active'
  )
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name REDACTED)
  REDACTED)
  return options
REDACTED)

// 降级分组选项（编辑时）- 排除自身
const fallbackGroupOptionsForEdit = computed(() => {
  const options: { value: number | null; label: string REDACTED[] = [
    { value: null, label: t('admin.groups.claudeCode.noFallback') REDACTED
  ]
  const currentId = editingGroup.value?.id
  const eligibleGroups = groups.value.filter(
    (g) => g.platform === 'anthropic' && !g.claude_code_only && g.status === 'active' && g.id !== currentId
  )
  eligibleGroups.forEach((g) => {
    options.push({ value: g.id, label: g.name REDACTED)
  REDACTED)
  return options
REDACTED)

// 复制账号的源分组选项（创建时）- 仅包含相同平台且有账号的分组
const copyAccountsGroupOptions = computed(() => {
  const eligibleGroups = groups.value.filter(
    (g) => g.platform === createForm.platform && (g.account_count || 0) > 0
  )
  return eligibleGroups.map((g) => ({
    value: g.id,
    label: `${g.nameREDACTED (${g.account_count || 0REDACTED 个账号)`
  REDACTED))
REDACTED)

// 复制账号的源分组选项（编辑时）- 仅包含相同平台且有账号的分组，排除自身
const copyAccountsGroupOptionsForEdit = computed(() => {
  const currentId = editingGroup.value?.id
  const eligibleGroups = groups.value.filter(
    (g) => g.platform === editForm.platform && (g.account_count || 0) > 0 && g.id !== currentId
  )
  return eligibleGroups.map((g) => ({
    value: g.id,
    label: `${g.nameREDACTED (${g.account_count || 0REDACTED 个账号)`
  REDACTED))
REDACTED)

const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({
  platform: '',
  status: '',
  is_exclusive: ''
REDACTED)
const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
REDACTED)

let abortController: AbortController | null = null

const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const submitting = ref(false)
const editingGroup = ref<AdminGroup | null>(null)
const deletingGroup = ref<AdminGroup | null>(null)

const createForm = reactive({
  name: '',
  description: '',
  platform: 'anthropic' as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  subscription_type: 'standard' as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null,
  // 图片生成计费配置（仅 antigravity 平台使用）
  image_price_1k: null as number | null,
  image_price_2k: null as number | null,
  image_price_4k: null as number | null,
  // Claude Code 客户端限制（仅 anthropic 平台使用）
  claude_code_only: false,
  fallback_group_id: null as number | null,
  // 模型路由开关
  model_routing_enabled: false,
  // 从分组复制账号
  copy_accounts_from_group_ids: [] as number[]
REDACTED)

// 简单账号类型（用于模型路由选择）
interface SimpleAccount {
  id: number
  name: string
REDACTED

// 模型路由规则类型
interface ModelRoutingRule {
  pattern: string
  accounts: SimpleAccount[] // 选中的账号对象数组
REDACTED

// 创建表单的模型路由规则
const createModelRoutingRules = ref<ModelRoutingRule[]>([])

// 编辑表单的模型路由规则
const editModelRoutingRules = ref<ModelRoutingRule[]>([])

// 账号搜索相关状态
const accountSearchKeyword = ref<Record<string, string>>({REDACTED) // 每个规则的搜索关键词 (key: "create-0" 或 "edit-0")
const accountSearchResults = ref<Record<string, SimpleAccount[]>>({REDACTED) // 每个规则的搜索结果
const showAccountDropdown = ref<Record<string, boolean>>({REDACTED) // 每个规则的下拉框显示状态
let accountSearchTimeout: ReturnType<typeof setTimeout> | null = null

// 搜索账号（仅限 anthropic 平台）
const searchAccounts = async (key: string) => {
  if (accountSearchTimeout) clearTimeout(accountSearchTimeout)
  accountSearchTimeout = setTimeout(async () => {
    const keyword = accountSearchKeyword.value[key] || ''
    try {
      const res = await adminAPI.accounts.list(1, 20, {
        search: keyword,
        platform: 'anthropic'
      REDACTED)
      accountSearchResults.value[key] = res.items.map((a) => ({ id: a.id, name: a.name REDACTED))
    REDACTED catch {
      accountSearchResults.value[key] = []
    REDACTED
  REDACTED, 300)
REDACTED

// 选择账号
const selectAccount = (ruleIndex: number, account: SimpleAccount, isEdit: boolean = false) => {
  const rules = isEdit ? editModelRoutingRules.value : createModelRoutingRules.value
  const rule = rules[ruleIndex]
  if (!rule) return

  // 检查是否已选择
  if (!rule.accounts.some(a => a.id === account.id)) {
    rule.accounts.push(account)
  REDACTED

  // 清空搜索
  const key = `${isEdit ? 'edit' : 'create'REDACTED-${ruleIndexREDACTED`
  accountSearchKeyword.value[key] = ''
  showAccountDropdown.value[key] = false
REDACTED

// 移除已选账号
const removeSelectedAccount = (ruleIndex: number, accountId: number, isEdit: boolean = false) => {
  const rules = isEdit ? editModelRoutingRules.value : createModelRoutingRules.value
  const rule = rules[ruleIndex]
  if (!rule) return

  rule.accounts = rule.accounts.filter(a => a.id !== accountId)
REDACTED

// 处理账号搜索输入框聚焦
const onAccountSearchFocus = (ruleIndex: number, isEdit: boolean = false) => {
  const key = `${isEdit ? 'edit' : 'create'REDACTED-${ruleIndexREDACTED`
  showAccountDropdown.value[key] = true
  // 如果没有搜索结果，触发一次搜索
  if (!accountSearchResults.value[key]?.length) {
    searchAccounts(key)
  REDACTED
REDACTED

// 添加创建表单的路由规则
const addCreateRoutingRule = () => {
  createModelRoutingRules.value.push({ pattern: '', accounts: [] REDACTED)
REDACTED

// 删除创建表单的路由规则
const removeCreateRoutingRule = (index: number) => {
  createModelRoutingRules.value.splice(index, 1)
  // 清理相关的搜索状态
  const key = `create-${indexREDACTED`
  delete accountSearchKeyword.value[key]
  delete accountSearchResults.value[key]
  delete showAccountDropdown.value[key]
REDACTED

// 添加编辑表单的路由规则
const addEditRoutingRule = () => {
  editModelRoutingRules.value.push({ pattern: '', accounts: [] REDACTED)
REDACTED

// 删除编辑表单的路由规则
const removeEditRoutingRule = (index: number) => {
  editModelRoutingRules.value.splice(index, 1)
  // 清理相关的搜索状态
  const key = `edit-${indexREDACTED`
  delete accountSearchKeyword.value[key]
  delete accountSearchResults.value[key]
  delete showAccountDropdown.value[key]
REDACTED

// 将 UI 格式的路由规则转换为 API 格式
const convertRoutingRulesToApiFormat = (rules: ModelRoutingRule[]): Record<string, number[]> | null => {
  const result: Record<string, number[]> = {REDACTED
  let hasValidRules = false

  for (const rule of rules) {
    const pattern = rule.pattern.trim()
    if (!pattern) continue

    const accountIds = rule.accounts.map(a => a.id).filter(id => id > 0)

    if (accountIds.length > 0) {
      result[pattern] = accountIds
      hasValidRules = true
    REDACTED
  REDACTED

  return hasValidRules ? result : null
REDACTED

// 将 API 格式的路由规则转换为 UI 格式（需要加载账号名称）
const convertApiFormatToRoutingRules = async (apiFormat: Record<string, number[]> | null): Promise<ModelRoutingRule[]> => {
  if (!apiFormat) return []

  const rules: ModelRoutingRule[] = []
  for (const [pattern, accountIds] of Object.entries(apiFormat)) {
    // 加载账号信息
    const accounts: SimpleAccount[] = []
    for (const id of accountIds) {
      try {
        const account = await adminAPI.accounts.getById(id)
        accounts.push({ id: account.id, name: account.name REDACTED)
      REDACTED catch {
        // 如果账号不存在，仍然显示 ID
        accounts.push({ id, name: `#${idREDACTED` REDACTED)
      REDACTED
    REDACTED
    rules.push({ pattern, accounts REDACTED)
  REDACTED
  return rules
REDACTED

const editForm = reactive({
  name: '',
  description: '',
  platform: 'anthropic' as GroupPlatform,
  rate_multiplier: 1.0,
  is_exclusive: false,
  status: 'active' as 'active' | 'inactive',
  subscription_type: 'standard' as SubscriptionType,
  daily_limit_usd: null as number | null,
  weekly_limit_usd: null as number | null,
  monthly_limit_usd: null as number | null,
  // 图片生成计费配置（仅 antigravity 平台使用）
  image_price_1k: null as number | null,
  image_price_2k: null as number | null,
  image_price_4k: null as number | null,
  // Claude Code 客户端限制（仅 anthropic 平台使用）
  claude_code_only: false,
  fallback_group_id: null as number | null,
  // 模型路由开关
  model_routing_enabled: false,
  // 从分组复制账号
  copy_accounts_from_group_ids: [] as number[]
REDACTED)

// 根据分组类型返回不同的删除确认消息
const deleteConfirmMessage = computed(() => {
  if (!deletingGroup.value) {
    return ''
  REDACTED
  if (deletingGroup.value.subscription_type === 'subscription') {
    return t('admin.groups.deleteConfirmSubscription', { name: deletingGroup.value.name REDACTED)
  REDACTED
  return t('admin.groups.deleteConfirm', { name: deletingGroup.value.name REDACTED)
REDACTED)

const loadGroups = async () => {
  if (abortController) {
    abortController.abort()
  REDACTED
  const currentController = new AbortController()
  abortController = currentController
  const { signal REDACTED = currentController
  loading.value = true
  try {
    const response = await adminAPI.groups.list(pagination.page, pagination.page_size, {
      platform: (filters.platform as GroupPlatform) || undefined,
      status: filters.status as any,
      is_exclusive: filters.is_exclusive ? filters.is_exclusive === 'true' : undefined,
      search: searchQuery.value.trim() || undefined
    REDACTED, { signal REDACTED)
    if (signal.aborted) return
    groups.value = response.items
    pagination.total = response.total
    pagination.pages = response.pages
  REDACTED catch (error: any) {
    if (signal.aborted || error?.name === 'AbortError' || error?.code === 'ERR_CANCELED') {
      return
    REDACTED
    appStore.showError(t('admin.groups.failedToLoad'))
    console.error('Error loading groups:', error)
  REDACTED finally {
    if (abortController === currentController && !signal.aborted) {
      loading.value = false
    REDACTED
  REDACTED
REDACTED

let searchTimeout: ReturnType<typeof setTimeout>
const handleSearch = () => {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadGroups()
  REDACTED, 300)
REDACTED

const handlePageChange = (page: number) => {
  pagination.page = page
  loadGroups()
REDACTED

const handlePageSizeChange = (pageSize: number) => {
  pagination.page_size = pageSize
  pagination.page = 1
  loadGroups()
REDACTED

const closeCreateModal = () => {
  showCreateModal.value = false
  createForm.name = ''
  createForm.description = ''
  createForm.platform = 'anthropic'
  createForm.rate_multiplier = 1.0
  createForm.is_exclusive = false
  createForm.subscription_type = 'standard'
  createForm.daily_limit_usd = null
  createForm.weekly_limit_usd = null
  createForm.monthly_limit_usd = null
  createForm.image_price_1k = null
  createForm.image_price_2k = null
  createForm.image_price_4k = null
  createForm.claude_code_only = false
  createForm.fallback_group_id = null
  createForm.copy_accounts_from_group_ids = []
  createModelRoutingRules.value = []
REDACTED

const handleCreateGroup = async () => {
  if (!createForm.name.trim()) {
    appStore.showError(t('admin.groups.nameRequired'))
    return
  REDACTED
  submitting.value = true
  try {
    // 构建请求数据，包含模型路由配置
    const requestData = {
      ...createForm,
      model_routing: convertRoutingRulesToApiFormat(createModelRoutingRules.value)
    REDACTED
    await adminAPI.groups.create(requestData)
    appStore.showSuccess(t('admin.groups.groupCreated'))
    closeCreateModal()
    loadGroups()
    // Only advance tour if active, on submit step, and creation succeeded
    if (onboardingStore.isCurrentStep('[data-tour="group-form-submit"]')) {
      onboardingStore.nextStep(500)
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToCreate'))
    console.error('Error creating group:', error)
    // Don't advance tour on error
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleEdit = async (group: AdminGroup) => {
  editingGroup.value = group
  editForm.name = group.name
  editForm.description = group.description || ''
  editForm.platform = group.platform
  editForm.rate_multiplier = group.rate_multiplier
  editForm.is_exclusive = group.is_exclusive
  editForm.status = group.status
  editForm.subscription_type = group.subscription_type || 'standard'
  editForm.daily_limit_usd = group.daily_limit_usd
  editForm.weekly_limit_usd = group.weekly_limit_usd
  editForm.monthly_limit_usd = group.monthly_limit_usd
  editForm.image_price_1k = group.image_price_1k
  editForm.image_price_2k = group.image_price_2k
  editForm.image_price_4k = group.image_price_4k
  editForm.claude_code_only = group.claude_code_only || false
  editForm.fallback_group_id = group.fallback_group_id
  editForm.model_routing_enabled = group.model_routing_enabled || false
  editForm.copy_accounts_from_group_ids = [] // 复制账号字段每次编辑时重置为空
  // 加载模型路由规则（异步加载账号名称）
  editModelRoutingRules.value = await convertApiFormatToRoutingRules(group.model_routing)
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingGroup.value = null
  editModelRoutingRules.value = []
  editForm.copy_accounts_from_group_ids = []
REDACTED

const handleUpdateGroup = async () => {
  if (!editingGroup.value) return
  if (!editForm.name.trim()) {
    appStore.showError(t('admin.groups.nameRequired'))
    return
  REDACTED

  submitting.value = true
  try {
    // 转换 fallback_group_id: null -> 0 (后端使用 0 表示清除)
    const payload = {
      ...editForm,
      fallback_group_id: editForm.fallback_group_id === null ? 0 : editForm.fallback_group_id,
      model_routing: convertRoutingRulesToApiFormat(editModelRoutingRules.value)
    REDACTED
    await adminAPI.groups.update(editingGroup.value.id, payload)
    appStore.showSuccess(t('admin.groups.groupUpdated'))
    closeEditModal()
    loadGroups()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToUpdate'))
    console.error('Error updating group:', error)
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

const handleDelete = (group: AdminGroup) => {
  deletingGroup.value = group
  showDeleteDialog.value = true
REDACTED

const confirmDelete = async () => {
  if (!deletingGroup.value) return

  try {
    await adminAPI.groups.delete(deletingGroup.value.id)
    appStore.showSuccess(t('admin.groups.groupDeleted'))
    showDeleteDialog.value = false
    deletingGroup.value = null
    loadGroups()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.groups.failedToDelete'))
    console.error('Error deleting group:', error)
  REDACTED
REDACTED

// 监听 subscription_type 变化，订阅模式时 is_exclusive 默认为 true
watch(
  () => createForm.subscription_type,
  (newVal) => {
    if (newVal === 'subscription') {
      createForm.is_exclusive = true
    REDACTED
  REDACTED
)

// 点击外部关闭账号搜索下拉框
const handleClickOutside = (event: MouseEvent) => {
  const target = event.target as HTMLElement
  // 检查是否点击在下拉框或输入框内
  if (!target.closest('.account-search-container')) {
    Object.keys(showAccountDropdown.value).forEach(key => {
      showAccountDropdown.value[key] = false
    REDACTED)
  REDACTED
REDACTED

onMounted(() => {
  loadGroups()
  document.addEventListener('click', handleClickOutside)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
REDACTED)
</script>
