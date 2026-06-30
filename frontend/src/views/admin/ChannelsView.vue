<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search + Filters -->
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
                :placeholder="t('admin.channels.searchChannels', 'Search channels...')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.status"
              :options="statusFilterOptions"
              :placeholder="t('admin.channels.allStatus', 'All Status')"
              class="w-40"
              @change="loadChannels"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.channels.createChannel', 'Create Channel') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="channels"
          :loading="loading"
          :server-side-sort="true"
          default-sort-key="created_at"
          default-sort-order="desc"
          @sort="handleSort"
        >
          <template #cell-name="{ value REDACTED">
            <span class="font-medium text-gray-900 dark:text-white">{{ value REDACTEDREDACTED</span>
          </template>

          <template #cell-description="{ value REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' REDACTEDREDACTED</span>
          </template>

          <template #cell-status="{ row REDACTED">
            <Toggle
              :modelValue="row.status === 'active'"
              @update:modelValue="toggleChannelStatus(row)"
            />
          </template>

          <template #cell-group_count="{ row REDACTED">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.group_ids || []).length REDACTEDREDACTED
              {{ t('admin.channels.groupsUnit', 'groups') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-pricing_count="{ row REDACTED">
            <span
              class="inline-flex items-center rounded bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-800 dark:bg-dark-600 dark:text-gray-300"
            >
              {{ (row.model_pricing || []).length REDACTEDREDACTED
              {{ t('admin.channels.pricingUnit', 'pricing rules') REDACTEDREDACTED
            </span>
          </template>

          <template #cell-created_at="{ value REDACTED">
            <span class="text-sm text-gray-600 dark:text-gray-400">
              {{ formatDate(value) REDACTEDREDACTED
            </span>
          </template>

          <template #cell-actions="{ row REDACTED">
            <div class="flex items-center gap-1">
              <button
                @click="openEditDialog(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit', 'Edit') REDACTEDREDACTED</span>
              </button>
              <button
                @click="handleDelete(row)"
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete', 'Delete') REDACTEDREDACTED</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.channels.noChannelsYet', 'No Channels Yet')"
              :description="t('admin.channels.createFirstChannel', 'Create your first channel to manage model pricing')"
              :action-text="t('admin.channels.createChannel', 'Create Channel')"
              @action="openCreateDialog"
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

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editingChannel ? t('admin.channels.editChannel', 'Edit Channel') : t('admin.channels.createChannel', 'Create Channel')"
      width="extra-wide"
      @close="closeDialog"
    >
      <div class="channel-dialog-body">
        <!-- Tab Bar -->
        <div class="flex items-center border-b border-gray-200 dark:border-dark-700 flex-shrink-0 -mx-4 sm:-mx-6 px-4 sm:px-6 -mt-3 sm:-mt-4">
          <!-- Basic Settings Tab -->
          <button
            type="button"
            @click="activeTab = 'basic'"
            class="channel-tab"
            :class="activeTab === 'basic' ? 'channel-tab-active' : 'channel-tab-inactive'"
          >
            {{ t('admin.channels.form.basicSettings', 'Basic Settings') REDACTEDREDACTED
          </button>
          <!-- Platform Tabs (only enabled) -->
          <button
            v-for="section in form.platforms.filter(s => s.enabled)"
            :key="section.platform"
            type="button"
            @click="activeTab = section.platform"
            class="channel-tab group"
            :class="activeTab === section.platform ? 'channel-tab-active' : 'channel-tab-inactive'"
          >
            <PlatformIcon :platform="section.platform" size="xs" :class="platformTextClass(section.platform)" />
            <span :class="platformTextClass(section.platform)">{{ t('admin.groups.platforms.' + section.platform, section.platform) REDACTEDREDACTED</span>
          </button>
        </div>

        <!-- Tab Content -->
        <form id="channel-form" @submit.prevent="handleSubmit" class="flex-1 overflow-y-auto pt-4">
          <!-- Basic Settings Tab -->
          <div v-show="activeTab === 'basic'" class="space-y-5">
            <!-- Name -->
            <div>
              <label class="input-label">{{ t('admin.channels.form.name', 'Name') REDACTEDREDACTED <span class="text-red-500">*</span></label>
              <input
                v-model="form.name"
                type="text"
                required
                class="input"
                :placeholder="t('admin.channels.form.namePlaceholder', 'Enter channel name')"
              />
            </div>

            <!-- Description -->
            <div>
              <label class="input-label">{{ t('admin.channels.form.description', 'Description') REDACTEDREDACTED</label>
              <textarea
                v-model="form.description"
                rows="2"
                class="input"
                :placeholder="t('admin.channels.form.descriptionPlaceholder', 'Optional description')"
              ></textarea>
            </div>

            <!-- Status (edit only) -->
            <div v-if="editingChannel">
              <label class="input-label">{{ t('admin.channels.form.status', 'Status') REDACTEDREDACTED</label>
              <Select v-model="form.status" :options="statusEditOptions" />
            </div>

            <!-- Model Restriction -->
            <div>
              <label class="flex items-center gap-2 cursor-pointer">
                <input
                  type="checkbox"
                  v-model="form.restrict_models"
                  class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                />
                <span class="input-label mb-0">{{ t('admin.channels.form.restrictModels', 'Restrict Models') REDACTEDREDACTED</span>
              </label>
              <p class="mt-1 ml-6 text-xs text-gray-400">
                {{ t('admin.channels.form.restrictModelsHint', 'When enabled, only models in the pricing list are allowed. Others will be rejected.') REDACTEDREDACTED
              </p>
            </div>

            <!-- Billing Basis -->
            <div>
              <label class="input-label">{{ t('admin.channels.form.billingModelSource', 'Billing Basis') REDACTEDREDACTED</label>
              <Select v-model="form.billing_model_source" :options="billingModelSourceOptions" />
              <p class="mt-1 text-xs text-gray-400">
                {{ t('admin.channels.form.billingModelSourceHint', 'Controls which model name is used for pricing lookup') REDACTEDREDACTED
              </p>
            </div>

            <!-- Platform Management -->
            <div class="space-y-3">
              <label class="input-label mb-0">{{ t('admin.channels.form.platformConfig', 'Platform Configuration') REDACTEDREDACTED</label>
              <div class="flex flex-wrap gap-2">
                <label
                  v-for="p in platformOrder"
                  :key="p"
                  class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
                  :class="activePlatforms.includes(p)
                    ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
                    : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
                >
                  <input
                    type="checkbox"
                    :checked="activePlatforms.includes(p)"
                    class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                    @change="togglePlatform(p)"
                  />
                  <PlatformIcon :platform="p" size="xs" :class="platformTextClass(p)" />
                  <span :class="platformTextClass(p)">{{ t('admin.groups.platforms.' + p, p) REDACTEDREDACTED</span>
                </label>
              </div>
            </div>

            <!-- Apply Pricing to Account Stats (toggle only in basic settings) -->
            <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
              <div class="flex items-center justify-between">
                <div>
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.channels.form.applyPricingToAccountStats') REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
                    {{ t('admin.channels.form.applyPricingToAccountStatsDesc') REDACTEDREDACTED
                  </p>
                </div>
                <Toggle
                  :modelValue="form.apply_pricing_to_account_stats"
                  @update:modelValue="form.apply_pricing_to_account_stats = $event"
                />
              </div>
            </div>
          </div>

          <!-- Platform Tab Content -->
          <div
            v-for="(section, sIdx) in form.platforms"
            :key="'tab-' + section.platform"
            v-show="section.enabled && activeTab === section.platform"
            class="space-y-4"
          >
            <!-- Groups -->
            <div>
              <label class="input-label text-xs">
                {{ t('admin.channels.form.groups', 'Associated Groups') REDACTEDREDACTED <span class="text-red-500">*</span>
                <span v-if="section.group_ids.length > 0" class="ml-1 font-normal text-gray-400">
                  ({{ t('admin.channels.form.selectedCount', { count: section.group_ids.length REDACTED, `${section.group_ids.lengthREDACTED selected`) REDACTEDREDACTED)
                </span>
              </label>
              <div class="max-h-40 overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-900">
                <div v-if="groupsLoading" class="py-2 text-center text-xs text-gray-500">
                  {{ t('common.loading', 'Loading...') REDACTEDREDACTED
                </div>
                <div v-else-if="getGroupsForPlatform(section.platform).length === 0" class="py-2 text-center text-xs text-gray-500">
                  {{ t('admin.channels.form.noGroupsAvailable', 'No groups available') REDACTEDREDACTED
                </div>
                <div v-else class="flex flex-wrap gap-1">
                  <label
                    v-for="group in getGroupsForPlatform(section.platform)"
                    :key="group.id"
                    class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
                    :class="[
                      section.group_ids.includes(group.id) ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700' : '',
                      isGroupInOtherChannel(group.id, section.platform) ? 'opacity-40' : ''
                    ]"
                  >
                    <input
                      type="checkbox"
                      :checked="section.group_ids.includes(group.id)"
                      :disabled="isGroupInOtherChannel(group.id, section.platform)"
                      class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                      @change="toggleGroupInSection(sIdx, group.id)"
                    />
                    <span :class="['font-medium', platformTextClass(group.platform)]">{{ group.name REDACTEDREDACTED</span>
                    <span
                      :class="['rounded-full px-1 py-0 text-[10px]', platformBadgeLightClass(group.platform)]"
                    >{{ group.rate_multiplier REDACTEDREDACTEDx</span>
                    <span class="text-[10px] text-gray-400">{{ group.account_count || 0 REDACTEDREDACTED</span>
                    <span
                      v-if="isGroupInOtherChannel(group.id, section.platform)"
                      class="text-[10px] text-gray-400"
                    >{{ getGroupInOtherChannelLabel(group.id) REDACTEDREDACTED</span>
                  </label>
                </div>
              </div>
            </div>

            <!-- Web Search Emulation (Anthropic only, hidden when global disabled) -->
            <div v-if="section.platform === 'anthropic' && webSearchGlobalEnabled" class="border-t border-gray-200 pt-3 dark:border-dark-600">
              <div class="flex items-center justify-between">
                <div>
                  <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.channels.form.webSearchEmulation') REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-[11px] text-red-500 dark:text-red-400">
                    {{ t('admin.channels.form.webSearchEmulationHint') REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="section.web_search_emulation" />
              </div>
            </div>

            <!-- Codex Image Generation Bridge (OpenAI only) -->
            <div v-if="section.platform === 'openai'" class="border-t border-gray-200 pt-3 dark:border-dark-600">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.channels.form.codexImageGenerationBridge') REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t('admin.channels.form.codexImageGenerationBridgeHint') REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="section.codex_image_generation_bridge" />
              </div>
            </div>

            <!-- Bedrock CC Compatibility (Anthropic only) -->
            <div v-if="section.platform === 'anthropic'" class="border-t border-gray-200 pt-3 dark:border-dark-600">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <label class="text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.channels.form.bedrockCCCompat') REDACTEDREDACTED
                  </label>
                  <p class="mt-0.5 text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t('admin.channels.form.bedrockCCCompatHint') REDACTEDREDACTED
                  </p>
                </div>
                <Toggle v-model="section.bedrock_cc_compat" />
              </div>
            </div>

            <!-- Model Mapping -->
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelMapping', 'Model Mapping') REDACTEDREDACTED</label>
                <button type="button" @click="addMappingEntry(sIdx)" class="text-xs text-primary-600 hover:text-primary-700">
                  + {{ t('common.add', 'Add') REDACTEDREDACTED
                </button>
              </div>
              <div
                v-if="Object.keys(section.model_mapping).length === 0"
                class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
              >
                {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') REDACTEDREDACTED
              </div>
              <div v-else class="space-y-1">
                <div
                  v-for="(_, srcModel) in section.model_mapping"
                  :key="srcModel"
                  class="flex items-center gap-2"
                >
                  <input
                    :value="srcModel"
                    type="text"
                    class="input flex-1 text-xs"
                    :class="platformTextClass(section.platform)"
                    :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
                    @change="renameMappingKey(sIdx, srcModel, ($event.target as HTMLInputElement).value)"
                  />
                  <span class="text-gray-400 text-xs">→</span>
                  <input
                    :value="section.model_mapping[srcModel]"
                    type="text"
                    class="input flex-1 text-xs"
                    :class="platformTextClass(section.platform)"
                    :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
                    @input="section.model_mapping[srcModel] = ($event.target as HTMLInputElement).value"
                  />
                  <button
                    type="button"
                    @click="removeMappingEntry(sIdx, srcModel)"
                    class="rounded p-0.5 text-gray-400 hover:text-red-500"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </div>
            </div>

            <!-- Model Pricing -->
            <div>
              <div class="mb-1 flex items-center justify-between">
                <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelPricing', 'Model Pricing') REDACTEDREDACTED</label>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    @click="syncLatestModels(sIdx)"
                    :disabled="syncingPlatform === section.platform"
                    class="text-xs text-gray-500 hover:text-primary-600 disabled:opacity-50"
                  >
                    {{ syncingPlatform === section.platform ? t('admin.channels.form.syncingModels') : t('admin.channels.form.syncLatestModels') REDACTEDREDACTED
                  </button>
                  <button type="button" @click="addPricingEntry(sIdx)" class="text-xs text-primary-600 hover:text-primary-700">
                    + {{ t('common.add', 'Add') REDACTEDREDACTED
                  </button>
                </div>
              </div>
              <div
                v-if="section.model_pricing.length === 0"
                class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
              >
                {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') REDACTEDREDACTED
              </div>
              <div v-else class="space-y-2">
                <PricingEntryCard
                  v-for="(entry, idx) in section.model_pricing"
                  :key="idx"
                  :entry="entry"
                  :platform="section.platform"
                  @update="updatePricingEntry(sIdx, idx, $event)"
                  @remove="removePricingEntry(sIdx, idx)"
                />
              </div>
            </div>

            <!-- Account Stats Pricing Rules (per-platform, always visible) -->
            <div class="mt-4 border-t border-gray-200 pt-4 dark:border-dark-700 space-y-3">
              <div class="flex items-center justify-between">
                <h4 class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('admin.channels.form.accountStatsPricingRules') REDACTEDREDACTED
                </h4>
                <button
                  type="button"
                  @click="addAccountStatsRule(sIdx)"
                  class="rounded-lg border border-primary-300 px-3 py-1 text-xs font-medium text-primary-600 hover:bg-primary-50 dark:border-primary-600 dark:text-primary-400 dark:hover:bg-primary-900/20"
                >
                  + {{ t('admin.channels.form.addRule') REDACTEDREDACTED
                </button>
              </div>

              <!-- Filter rules for this platform's groups -->
              <p
                v-if="section.account_stats_pricing_rules.length === 0"
                class="text-xs italic text-gray-400 dark:text-gray-500"
              >
                {{ t('admin.channels.form.noRulesConfigured') REDACTEDREDACTED
              </p>

              <div
                v-for="(rule, ruleIndex) in section.account_stats_pricing_rules"
                :key="ruleIndex"
                class="space-y-3 rounded-lg border border-gray-200 p-4 dark:border-dark-600"
              >
                <div class="flex items-center justify-between">
                  <input
                    v-model="rule.name"
                    :placeholder="t('admin.channels.form.ruleName')"
                    class="bg-transparent text-sm font-medium text-gray-700 placeholder-gray-400 outline-none dark:text-gray-300"
                  />
                  <button type="button" @click="removeAccountStatsRule(sIdx, ruleIndex)" class="text-xs text-red-500 hover:text-red-700">
                    {{ t('common.delete') REDACTEDREDACTED
                  </button>
                </div>

                <div>
                  <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleGroups') REDACTEDREDACTED</label>
                  <div class="mt-1 flex flex-wrap gap-1">
                    <label
                      v-for="gid in section.group_ids"
                      :key="gid"
                      class="inline-flex cursor-pointer items-center gap-1 rounded-md border px-2 py-1 text-xs transition-colors"
                      :class="rule.group_ids.includes(gid)
                        ? 'border-primary-300 bg-primary-50 dark:border-primary-700 dark:bg-primary-900/20'
                        : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
                    >
                      <input type="checkbox" :checked="rule.group_ids.includes(gid)" class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500" @change="rule.group_ids.includes(gid) ? rule.group_ids.splice(rule.group_ids.indexOf(gid), 1) : rule.group_ids.push(gid)" />
                      <span :class="['font-medium', platformTextClass(section.platform)]">{{ getGroupNameById(gid) REDACTEDREDACTED</span>
                    </label>
                  </div>
                  <p v-if="section.group_ids.length === 0" class="mt-1 text-xs text-gray-400">
                    {{ t('admin.channels.form.noGroupsInChannel') REDACTEDREDACTED
                  </p>
                </div>

                <div>
                  <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleAccounts') REDACTEDREDACTED</label>
                  <!-- Selected account chips -->
                  <div class="mt-1 flex flex-wrap gap-1">
                    <span
                      v-for="accountId in rule.account_ids"
                      :key="accountId"
                      class="inline-flex items-center gap-1 rounded-md border border-primary-300 bg-primary-50 px-2 py-0.5 text-xs dark:border-primary-700 dark:bg-primary-900/20"
                    >
                      <span :class="['font-medium', platformTextClass(section.platform)]">{{ getRuleAccountLabel(accountId) REDACTEDREDACTED</span>
                      <button type="button" @click="removeRuleAccount(rule, accountId)" class="text-gray-400 hover:text-red-500">
                        <Icon name="x" size="xs" />
                      </button>
                    </span>
                  </div>
                  <!-- Account search input -->
                  <div class="relative mt-1 rule-account-search-container">
                    <input
                      v-model="ruleAccountSearchKeyword[`${section.platformREDACTED-${ruleIndexREDACTED`]"
                      type="text"
                      class="input text-sm"
                      :placeholder="t('admin.channels.form.searchAccountPlaceholder')"
                      @input="onRuleAccountSearchInput(section.platform, ruleIndex)"
                      @focus="onRuleAccountSearchFocus(section.platform, ruleIndex)"
                    />
                    <!-- Search results dropdown -->
                    <div
                      v-if="showRuleAccountDropdown[`${section.platformREDACTED-${ruleIndexREDACTED`] && (ruleAccountSearchResults[`${section.platformREDACTED-${ruleIndexREDACTED`]?.length ?? 0) > 0"
                      class="absolute z-50 mt-1 max-h-48 w-full overflow-auto rounded-lg border bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800"
                    >
                      <button
                        v-for="account in ruleAccountSearchResults[`${section.platformREDACTED-${ruleIndexREDACTED`]"
                        :key="account.id"
                        type="button"
                        @click="selectRuleAccount(rule, account, section.platform, ruleIndex)"
                        class="w-full px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700"
                        :class="{ 'opacity-50': rule.account_ids.includes(account.id) REDACTED"
                        :disabled="rule.account_ids.includes(account.id)"
                      >
                        <span :class="platformTextClass(account.platform)">{{ account.name REDACTEDREDACTED</span>
                        <span class="ml-2 text-xs text-gray-400">#{{ account.id REDACTEDREDACTED</span>
                      </button>
                    </div>
                  </div>
                  <p class="mt-1 text-xs text-gray-400">
                    {{ t('admin.channels.form.ruleAccountsHint') REDACTEDREDACTED
                  </p>
                </div>

                <div>
                  <div class="mb-1 flex items-center justify-between">
                    <label class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.channels.form.ruleModelPricing') REDACTEDREDACTED</label>
                    <button type="button" @click="addRulePricingEntry(sIdx, ruleIndex)" class="text-xs text-primary-600 hover:text-primary-700">
                      + {{ t('common.add') REDACTEDREDACTED
                    </button>
                  </div>
                  <div v-if="rule.pricing.length === 0" class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500">
                    {{ t('admin.channels.form.noPricingRules') REDACTEDREDACTED
                  </div>
                  <div v-else class="space-y-2">
                    <PricingEntryCard
                      v-for="(entry, pIdx) in rule.pricing"
                      :key="pIdx"
                      :entry="entry"
                      :platform="section.platform"
                      @update="rule.pricing.splice(pIdx, 1, $event)"
                      @remove="removeRulePricingEntry(sIdx, ruleIndex, pIdx)"
                    />
                  </div>
                </div>
              </div>
            </div>
          </div>
        </form>
      </div>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeDialog" type="button" class="btn btn-secondary">
            {{ t('common.cancel', 'Cancel') REDACTEDREDACTED
          </button>
          <button
            type="submit"
            form="channel-form"
            :disabled="submitting"
            class="btn btn-primary"
          >
            {{ submitting
              ? t('common.submitting', 'Submitting...')
              : editingChannel
                ? t('common.update', 'Update')
                : t('common.create', 'Create')
            REDACTEDREDACTED
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.channels.deleteChannel', 'Delete Channel')"
      :message="deleteConfirmMessage"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { extractApiErrorMessage REDACTED from '@/utils/apiError'
import { adminAPI REDACTED from '@/api/admin'
import type { Channel, ChannelModelPricing, CreateChannelRequest, UpdateChannelRequest, AccountStatsPricingRule REDACTED from '@/api/admin/channels'
import type { PricingFormEntry REDACTED from '@/components/admin/channel/types'
import { mTokToPerToken, perTokenToMTok, apiIntervalsToForm, formIntervalsToAPI, findModelConflict, validateIntervals REDACTED from '@/components/admin/channel/types'
import type { AdminGroup, GroupPlatform REDACTED from '@/types'
import type { Column REDACTED from '@/components/common/types'
import { platformTextClass, platformBadgeLightClass REDACTED from '@/utils/platformColors'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Toggle from '@/components/common/Toggle.vue'
import PricingEntryCard from '@/components/admin/channel/PricingEntryCard.vue'
import { getPersistedPageSize REDACTED from '@/composables/usePersistedPageSize'
import { useKeyedDebouncedSearch REDACTED from '@/composables/useKeyedDebouncedSearch'

const { t REDACTED = useI18n()
const appStore = useAppStore()

// Web Search global enabled state (loaded once on mount)
const webSearchGlobalEnabled = ref(false)
async function loadWebSearchGlobalState() {
  try {
    const cfg = await adminAPI.settings.getWebSearchEmulationConfig()
    webSearchGlobalEnabled.value = cfg?.enabled === true && (cfg?.providers?.length ?? 0) > 0
  REDACTED catch (err: unknown) {
    console.warn('Failed to load web search global state:', err)
    webSearchGlobalEnabled.value = false
  REDACTED
REDACTED

// ── Form-level pricing rule type (per-platform) ──
interface FormPricingRule {
  name: string
  group_ids: number[]
  account_ids: number[]
  pricing: PricingFormEntry[]
REDACTED

// ── Platform Section type ──
interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  web_search_emulation: boolean
  codex_image_generation_bridge: boolean
  bedrock_cc_compat: boolean
  account_stats_pricing_rules: FormPricingRule[]
REDACTED

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.channels.columns.name', 'Name'), sortable: true REDACTED,
  { key: 'description', label: t('admin.channels.columns.description', 'Description'), sortable: false REDACTED,
  { key: 'status', label: t('admin.channels.columns.status', 'Status'), sortable: true REDACTED,
  { key: 'group_count', label: t('admin.channels.columns.groups', 'Groups'), sortable: false REDACTED,
  { key: 'pricing_count', label: t('admin.channels.columns.pricing', 'Pricing'), sortable: false REDACTED,
  { key: 'created_at', label: t('admin.channels.columns.createdAt', 'Created'), sortable: true REDACTED,
  { key: 'actions', label: t('admin.channels.columns.actions', 'Actions'), sortable: false REDACTED
])

const statusFilterOptions = computed(() => [
  { value: '', label: t('admin.channels.allStatus', 'All Status') REDACTED,
  { value: 'active', label: t('admin.channels.statusActive', 'Active') REDACTED,
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') REDACTED
])

const statusEditOptions = computed(() => [
  { value: 'active', label: t('admin.channels.statusActive', 'Active') REDACTED,
  { value: 'disabled', label: t('admin.channels.statusDisabled', 'Disabled') REDACTED
])

const billingModelSourceOptions = computed(() => [
  { value: 'channel_mapped', label: t('admin.channels.form.billingModelSourceChannelMapped', 'Bill by channel-mapped model') REDACTED,
  { value: 'requested', label: t('admin.channels.form.billingModelSourceRequested', 'Bill by requested model') REDACTED,
  { value: 'upstream', label: t('admin.channels.form.billingModelSourceUpstream', 'Bill by final upstream model') REDACTED
])

// ── State ──
const channels = ref<Channel[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({ status: '' REDACTED)
const pagination = reactive({
  page: 1,
  page_size: getPersistedPageSize(),
  total: 0
REDACTED)
const sortState = reactive({
  sort_by: 'created_at',
  sort_order: 'desc' as 'asc' | 'desc'
REDACTED)

// Dialog state
const showDialog = ref(false)
const editingChannel = ref<Channel | null>(null)
const submitting = ref(false)
const showDeleteDialog = ref(false)
const deletingChannel = ref<Channel | null>(null)
const activeTab = ref<string>('basic')

// Groups
const allGroups = ref<AdminGroup[]>([])
const groupsLoading = ref(false)

// All channels for group-conflict detection (independent of current page)
const allChannelsForConflict = ref<Channel[]>([])

// Form data
const form = reactive({
  name: '',
  description: '',
  status: 'active',
  restrict_models: false,
  billing_model_source: 'channel_mapped' as string,
  platforms: [] as PlatformSection[],
  apply_pricing_to_account_stats: false,
REDACTED)

let abortController: AbortController | null = null

// ── Platform config ──
const platformOrder: GroupPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity', 'grok']

// ── Helpers ──
function formatDate(value: string): string {
  if (!value) return '-'
  return new Date(value).toLocaleDateString()
REDACTED

// ── Platform section helpers ──
const activePlatforms = computed(() => form.platforms.filter(s => s.enabled).map(s => s.platform))

function addPlatformSection(platform: GroupPlatform) {
  form.platforms.push({
    platform,
    enabled: true,
    collapsed: false,
    group_ids: [],
    model_mapping: {REDACTED,
    model_pricing: [],
    web_search_emulation: false,
    codex_image_generation_bridge: false,
    bedrock_cc_compat: false,
    account_stats_pricing_rules: [],
  REDACTED)
REDACTED

function togglePlatform(platform: GroupPlatform) {
  const section = form.platforms.find(s => s.platform === platform)
  if (section) {
    section.enabled = !section.enabled
    if (!section.enabled && activeTab.value === platform) {
      activeTab.value = 'basic'
    REDACTED
  REDACTED else {
    addPlatformSection(platform)
  REDACTED
REDACTED

function getGroupsForPlatform(platform: GroupPlatform): AdminGroup[] {
  return allGroups.value.filter(g => g.platform === platform)
REDACTED

// ── Group helpers ──
const groupToChannelMap = computed(() => {
  const map = new Map<number, Channel>()
  for (const ch of allChannelsForConflict.value) {
    if (editingChannel.value && ch.id === editingChannel.value.id) continue
    for (const gid of ch.group_ids || []) {
      map.set(gid, ch)
    REDACTED
  REDACTED
  return map
REDACTED)

function isGroupInOtherChannel(groupId: number, _platform: string): boolean {
  return groupToChannelMap.value.has(groupId)
REDACTED

function getGroupChannelName(groupId: number): string {
  return groupToChannelMap.value.get(groupId)?.name || ''
REDACTED

function getGroupInOtherChannelLabel(groupId: number): string {
  const name = getGroupChannelName(groupId)
  return t('admin.channels.form.inOtherChannel', { name REDACTED, `In "${nameREDACTED"`)
REDACTED

const deleteConfirmMessage = computed(() => {
  const name = deletingChannel.value?.name || ''
  return t(
    'admin.channels.deleteConfirm',
    { name REDACTED,
    `Are you sure you want to delete channel "${nameREDACTED"? This action cannot be undone.`
  )
REDACTED)

function toggleGroupInSection(sectionIdx: number, groupId: number) {
  const section = form.platforms[sectionIdx]
  const idx = section.group_ids.indexOf(groupId)
  if (idx >= 0) {
    section.group_ids.splice(idx, 1)
  REDACTED else {
    section.group_ids.push(groupId)
  REDACTED
REDACTED

// ── Pricing helpers ──
function addPricingEntry(sectionIdx: number) {
  form.platforms[sectionIdx].model_pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  REDACTED)
REDACTED

const syncingPlatform = ref<string | null>(null)

async function syncLatestModels(sectionIdx: number) {
  const platform = form.platforms[sectionIdx].platform
  if (syncingPlatform.value) return
  syncingPlatform.value = platform
  try {
    const result = await adminAPI.channels.syncPricingModels(platform)
    // Collect all model names already present in this platform's pricing entries
    const existingModels = new Set<string>()
    for (const entry of form.platforms[sectionIdx].model_pricing) {
      for (const m of entry.models) existingModels.add(m)
    REDACTED
    const newModels = result.models.filter(m => !existingModels.has(m))
    if (newModels.length === 0) {
      appStore.showSuccess(t('admin.channels.form.syncModelsAlreadyUpToDate'))
      return
    REDACTED
    // Add new models as a single new pricing entry (user fills in prices)
    form.platforms[sectionIdx].model_pricing.push({
      models: newModels,
      billing_mode: 'token',
      input_price: null,
      output_price: null,
      cache_write_price: null,
      cache_read_price: null,
      image_output_price: null,
      per_request_price: null,
      intervals: []
    REDACTED)
    appStore.showSuccess(t('admin.channels.form.syncModelsSuccess', { count: newModels.length REDACTED))
  REDACTED catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.form.syncModelsError')))
  REDACTED finally {
    syncingPlatform.value = null
  REDACTED
REDACTED

function updatePricingEntry(sectionIdx: number, idx: number, updated: PricingFormEntry) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1, updated)
REDACTED

function removePricingEntry(sectionIdx: number, idx: number) {
  form.platforms[sectionIdx].model_pricing.splice(idx, 1)
REDACTED

// ── Model Mapping helpers ──
function addMappingEntry(sectionIdx: number) {
  const mapping = form.platforms[sectionIdx].model_mapping
  let key = ''
  let i = 1
  while (key === '' || key in mapping) {
    key = `model-${iREDACTED`
    i++
  REDACTED
  mapping[key] = ''
REDACTED

function removeMappingEntry(sectionIdx: number, key: string) {
  delete form.platforms[sectionIdx].model_mapping[key]
REDACTED

function renameMappingKey(sectionIdx: number, oldKey: string, newKey: string) {
  newKey = newKey.trim()
  if (!newKey || newKey === oldKey) return
  const mapping = form.platforms[sectionIdx].model_mapping
  if (newKey in mapping) return
  const value = mapping[oldKey]
  delete mapping[oldKey]
  mapping[newKey] = value
REDACTED

// ── Account Stats Pricing helpers ──
function addAccountStatsRule(sectionIdx: number) {
  form.platforms[sectionIdx].account_stats_pricing_rules.push({
    name: '',
    group_ids: [],
    account_ids: [],
    pricing: []
  REDACTED)
REDACTED

function addRulePricingEntry(sectionIdx: number, ruleIndex: number) {
  form.platforms[sectionIdx].account_stats_pricing_rules[ruleIndex].pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  REDACTED)
REDACTED

function removeAccountStatsRule(sectionIdx: number, ruleIndex: number) {
  form.platforms[sectionIdx].account_stats_pricing_rules.splice(ruleIndex, 1)
  // Clear all search state since indices shift after removal
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
REDACTED

function removeRulePricingEntry(sectionIdx: number, ruleIndex: number, pricingIndex: number) {
  form.platforms[sectionIdx].account_stats_pricing_rules[ruleIndex].pricing.splice(pricingIndex, 1)
REDACTED

function getGroupNameById(groupId: number): string {
  const group = allGroups.value.find(g => g.id === groupId)
  return group ? group.name : `#${groupIdREDACTED`
REDACTED

// ── Account search for pricing rules ──
interface SimpleAccount { id: number; name: string; platform: string REDACTED

const ruleAccountSearchKeyword = ref<Record<string, string>>({REDACTED)
const ruleAccountSearchResults = ref<Record<string, SimpleAccount[]>>({REDACTED)
const showRuleAccountDropdown = ref<Record<string, boolean>>({REDACTED)
// Cache: account ID → name, populated when search results are selected
const ruleAccountNameCache = ref<Record<number, string>>({REDACTED)

const ruleAccountSearchRunner = useKeyedDebouncedSearch<SimpleAccount[]>({
  delay: 300,
  search: async (keyword, { key, signal REDACTED) => {
    const platform = key.split('-')[0]
    const res = await adminAPI.accounts.list(1, 20, { platform, search: keyword REDACTED, { signal REDACTED)
    return res.items.map(a => ({ id: a.id, name: a.name, platform: a.platform REDACTED))
  REDACTED,
  onSuccess: (key, result) => { ruleAccountSearchResults.value[key] = result REDACTED,
  onError: (key) => { ruleAccountSearchResults.value[key] = [] REDACTED,
REDACTED)

function onRuleAccountSearchInput(platform: string, ruleIndex: number) {
  const key = `${platformREDACTED-${ruleIndexREDACTED`
  showRuleAccountDropdown.value[key] = true
  ruleAccountSearchRunner.trigger(key, ruleAccountSearchKeyword.value[key] || '')
REDACTED

function onRuleAccountSearchFocus(platform: string, ruleIndex: number) {
  const key = `${platformREDACTED-${ruleIndexREDACTED`
  showRuleAccountDropdown.value[key] = true
  if (!ruleAccountSearchResults.value[key]?.length) {
    ruleAccountSearchRunner.trigger(key, ruleAccountSearchKeyword.value[key] || '')
  REDACTED
REDACTED

function selectRuleAccount(
  rule: { account_ids: number[] REDACTED,
  account: SimpleAccount,
  platform: string,
  ruleIndex: number,
) {
  if (!rule.account_ids.includes(account.id)) {
    rule.account_ids.push(account.id)
    ruleAccountNameCache.value[account.id] = account.name
  REDACTED
  const key = `${platformREDACTED-${ruleIndexREDACTED`
  ruleAccountSearchKeyword.value[key] = ''
  showRuleAccountDropdown.value[key] = false
REDACTED

function removeRuleAccount(rule: { account_ids: number[] REDACTED, accountId: number) {
  const idx = rule.account_ids.indexOf(accountId)
  if (idx !== -1) rule.account_ids.splice(idx, 1)
REDACTED

function getRuleAccountLabel(accountId: number): string {
  const name = ruleAccountNameCache.value[accountId]
  return name ? `${nameREDACTED #${accountIdREDACTED` : `#${accountIdREDACTED`
REDACTED

function handleRuleAccountClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest('.rule-account-search-container')) {
    Object.keys(showRuleAccountDropdown.value).forEach(key => {
      showRuleAccountDropdown.value[key] = false
    REDACTED)
  REDACTED
REDACTED

function clearAllRuleAccountSearchState() {
  ruleAccountSearchKeyword.value = {REDACTED
  ruleAccountSearchResults.value = {REDACTED
  showRuleAccountDropdown.value = {REDACTED
REDACTED

function accountStatsRulesToAPI(): AccountStatsPricingRule[] {
  const rules: AccountStatsPricingRule[] = []
  for (const section of form.platforms) {
    if (!section.enabled) continue
    for (const rule of section.account_stats_pricing_rules) {
      rules.push({
        name: rule.name,
        group_ids: rule.group_ids,
        account_ids: rule.account_ids,
        pricing: rule.pricing
          .filter(p => p.models.length > 0)
          .map(p => ({
            platform: section.platform,
            models: p.models,
            billing_mode: p.billing_mode,
            input_price: mTokToPerToken(p.input_price),
            output_price: mTokToPerToken(p.output_price),
            cache_write_price: mTokToPerToken(p.cache_write_price),
            cache_read_price: mTokToPerToken(p.cache_read_price),
            image_output_price: mTokToPerToken(p.image_output_price),
            per_request_price: p.per_request_price != null && p.per_request_price !== '' ? Number(p.per_request_price) : null,
            intervals: formIntervalsToAPI(p.intervals || [])
          REDACTED))
      REDACTED)
    REDACTED
  REDACTED
  return rules
REDACTED

// ── Form ↔ API conversion ──
function formToAPI(): { group_ids: number[], model_pricing: ChannelModelPricing[], model_mapping: Record<string, Record<string, string>>, features_config: Record<string, unknown> REDACTED {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  const model_mapping: Record<string, Record<string, string>> = {REDACTED
  // Preserve existing features_config fields not managed by the form
  const featuresConfig: Record<string, unknown> = editingChannel.value?.features_config
    ? { ...editingChannel.value.features_config REDACTED
    : {REDACTED

  for (const section of form.platforms) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)

    // Model mapping per platform
    if (Object.keys(section.model_mapping).length > 0) {
      model_mapping[section.platform] = { ...section.model_mapping REDACTED
    REDACTED

    // Model pricing with platform tag
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push({
        platform: section.platform,
        models: entry.models,
        billing_mode: entry.billing_mode,
        input_price: mTokToPerToken(entry.input_price),
        output_price: mTokToPerToken(entry.output_price),
        cache_write_price: mTokToPerToken(entry.cache_write_price),
        cache_read_price: mTokToPerToken(entry.cache_read_price),
        image_output_price: mTokToPerToken(entry.image_output_price),
        per_request_price: entry.per_request_price != null && entry.per_request_price !== '' ? Number(entry.per_request_price) : null,
        intervals: formIntervalsToAPI(entry.intervals || [])
      REDACTED)
    REDACTED
  REDACTED

  // Collect web_search_emulation (only anthropic platform supports it)
  // Always write the key so that disabling in the UI correctly sets platform to false,
  // rather than leaving a stale true value from the cloned features_config.
  const wsEmulation: Record<string, boolean> = {REDACTED
  for (const section of form.platforms) {
    if (!section.enabled) continue
    if (section.platform === 'anthropic') {
      wsEmulation[section.platform] = !!section.web_search_emulation
    REDACTED
  REDACTED
  if (Object.keys(wsEmulation).length > 0) {
    featuresConfig.web_search_emulation = wsEmulation
  REDACTED else {
    delete featuresConfig.web_search_emulation
  REDACTED

  const codexImageGenerationBridge: Record<string, boolean> = {REDACTED
  for (const section of form.platforms) {
    if (!section.enabled) continue
    if (section.platform === 'openai') {
      codexImageGenerationBridge[section.platform] = !!section.codex_image_generation_bridge
    REDACTED
  REDACTED
  if (Object.keys(codexImageGenerationBridge).length > 0) {
    featuresConfig.codex_image_generation_bridge = codexImageGenerationBridge
  REDACTED else {
    delete featuresConfig.codex_image_generation_bridge
  REDACTED

  const bedrockCCCompat: Record<string, boolean> = {REDACTED
  for (const section of form.platforms) {
    if (!section.enabled) continue
    if (section.platform === 'anthropic') {
      bedrockCCCompat[section.platform] = !!section.bedrock_cc_compat
    REDACTED
  REDACTED
  if (Object.keys(bedrockCCCompat).length > 0) {
    featuresConfig.bedrock_cc_compat = bedrockCCCompat
  REDACTED else {
    delete featuresConfig.bedrock_cc_compat
  REDACTED

  return { group_ids, model_pricing, model_mapping, features_config: featuresConfig REDACTED
REDACTED

function apiToForm(channel: Channel): PlatformSection[] {
  // Build a map: groupID → platform
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups.value) {
    groupPlatformMap.set(g.id, g.platform)
  REDACTED

  // Determine which platforms are active (from groups + pricing + mapping)
  const activePlatforms = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = groupPlatformMap.get(gid)
    if (p) activePlatforms.add(p)
  REDACTED
  for (const p of channel.model_pricing || []) {
    if (p.platform) activePlatforms.add(p.platform as GroupPlatform)
  REDACTED
  for (const p of Object.keys(channel.model_mapping || {REDACTED)) {
    if (platformOrder.includes(p as GroupPlatform)) activePlatforms.add(p as GroupPlatform)
  REDACTED

  // Build sections in platform order
  const sections: PlatformSection[] = []
  for (const platform of platformOrder) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(gid => groupPlatformMap.get(gid) === platform)
    const mapping = (channel.model_mapping || {REDACTED)[platform] || {REDACTED
    const pricing = (channel.model_pricing || [])
      .filter(p => (p.platform || 'anthropic') === platform)
      .map(p => ({
        models: p.models || [],
        billing_mode: p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      REDACTED as PricingFormEntry))

    // Read web_search_emulation from features_config
    const fc = channel.features_config
    const wsEmulation = fc?.web_search_emulation as Record<string, boolean> | undefined
    const webSearchEnabled = wsEmulation?.[platform] === true
    const codexImageGenerationBridge = fc?.codex_image_generation_bridge as Record<string, boolean> | undefined
    const codexImageGenerationBridgeEnabled = codexImageGenerationBridge?.[platform] === true
    const bedrockCCCompatEnabled = fc?.bedrock_cc_compat === true

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...mapping REDACTED,
      model_pricing: pricing,
      web_search_emulation: webSearchEnabled,
      codex_image_generation_bridge: codexImageGenerationBridgeEnabled,
      bedrock_cc_compat: bedrockCCCompatEnabled,
      account_stats_pricing_rules: [],
    REDACTED)
  REDACTED

  return sections
REDACTED

// ── Load data ──
async function loadChannels() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true

  try {
    const response = await adminAPI.channels.list(pagination.page, pagination.page_size, {
      status: filters.status || undefined,
      search: searchQuery.value || undefined,
      sort_by: sortState.sort_by,
      sort_order: sortState.sort_order
    REDACTED, { signal: ctrl.signal REDACTED)

    if (ctrl.signal.aborted || abortController !== ctrl) return
    channels.value = response.items || []
    pagination.total = response.total
  REDACTED catch (error: unknown) {
    const e = error as { name?: string; code?: string REDACTED
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.loadError', 'Failed to load channels')))
  REDACTED finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    REDACTED
  REDACTED
REDACTED

async function loadGroups() {
  groupsLoading.value = true
  try {
    allGroups.value = await adminAPI.groups.getAll()
  REDACTED catch (error) {
    console.error('Error loading groups:', error)
  REDACTED finally {
    groupsLoading.value = false
  REDACTED
REDACTED

async function loadAllChannelsForConflict() {
  try {
    const response = await adminAPI.channels.list(1, 1000)
    allChannelsForConflict.value = response.items || []
  REDACTED catch (error) {
    // Fallback to current page data
    allChannelsForConflict.value = channels.value
  REDACTED
REDACTED

let searchTimeout: ReturnType<typeof setTimeout>
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    pagination.page = 1
    loadChannels()
  REDACTED, 300)
REDACTED

function handlePageChange(page: number) {
  pagination.page = page
  loadChannels()
REDACTED

function handlePageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadChannels()
REDACTED

function handleSort(key: string, order: 'asc' | 'desc') {
  sortState.sort_by = key
  sortState.sort_order = order
  pagination.page = 1
  loadChannels()
REDACTED

// ── Dialog ──
function resetForm() {
  form.name = ''
  form.description = ''
  form.status = 'active'
  form.restrict_models = false
  form.billing_model_source = 'channel_mapped'
  form.platforms = []
  form.apply_pricing_to_account_stats = false
  activeTab.value = 'basic'
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
  ruleAccountNameCache.value = {REDACTED
REDACTED

async function openCreateDialog() {
  editingChannel.value = null
  resetForm()
  await Promise.all([loadGroups(), loadAllChannelsForConflict()])
  showDialog.value = true
REDACTED

async function openEditDialog(channel: Channel) {
  editingChannel.value = channel
  form.name = channel.name
  form.description = channel.description || ''
  form.status = channel.status
  form.restrict_models = channel.restrict_models || false
  form.billing_model_source = channel.billing_model_source || 'channel_mapped'
  form.apply_pricing_to_account_stats = channel.apply_pricing_to_account_stats || false
  // Must load groups first so apiToForm can map groupID → platform
  await Promise.all([loadGroups(), loadAllChannelsForConflict()])
  form.platforms = apiToForm(channel)

  // Distribute channel-level rules into per-platform sections
  distributeRulesToPlatforms(channel.account_stats_pricing_rules || [])

  // Populate ruleAccountNameCache for existing rule accounts
  await populateRuleAccountNameCache()

  showDialog.value = true
REDACTED

/** Distribute flat channel-level rules into the matching platform section based on group_ids */
function distributeRulesToPlatforms(apiRules: AccountStatsPricingRule[]) {
  // Build groupID → platform lookup
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups.value) {
    groupPlatformMap.set(g.id, g.platform)
  REDACTED

  for (const apiRule of apiRules) {
    // Infer platform from group_ids
    const platforms = new Set<GroupPlatform>()
    for (const gid of apiRule.group_ids || []) {
      const p = groupPlatformMap.get(gid)
      if (p) platforms.add(p)
    REDACTED
    // If pricing has a platform field, use that as fallback
    if (platforms.size === 0 && apiRule.pricing?.length > 0) {
      const p = apiRule.pricing[0].platform as GroupPlatform | undefined
      if (p) platforms.add(p)
    REDACTED
    const targetPlatform = platforms.size >= 1 ? [...platforms][0] : null
    if (!targetPlatform) continue

    const section = form.platforms.find(s => s.platform === targetPlatform)
    if (!section) continue

    const formRule: FormPricingRule = {
      name: apiRule.name || '',
      group_ids: [...(apiRule.group_ids || [])],
      account_ids: [...(apiRule.account_ids || [])],
      pricing: (apiRule.pricing || []).map(p => ({
        models: [...(p.models || [])],
        billing_mode: p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      REDACTED as PricingFormEntry))
    REDACTED
    section.account_stats_pricing_rules.push(formRule)
  REDACTED
REDACTED

/** Populate ruleAccountNameCache by fetching account details for all account_ids in rules */
async function populateRuleAccountNameCache() {
  const allAccountIds = new Set<number>()
  for (const section of form.platforms) {
    for (const rule of section.account_stats_pricing_rules) {
      for (const id of rule.account_ids) {
        allAccountIds.add(id)
      REDACTED
    REDACTED
  REDACTED
  if (allAccountIds.size === 0) return

  // Fetch account details in parallel (batch of individual getById calls)
  const ids = [...allAccountIds]
  const results = await Promise.allSettled(
    ids.map(id => adminAPI.accounts.getById(id))
  )
  for (let i = 0; i < ids.length; i++) {
    const result = results[i]
    if (result.status === 'fulfilled') {
      ruleAccountNameCache.value[ids[i]] = result.value.name
    REDACTED
    // If rejected, the cache won't have the name, so it'll show "#ID" which is acceptable
  REDACTED
REDACTED

function closeDialog() {
  showDialog.value = false
  editingChannel.value = null
  resetForm()
REDACTED

async function handleSubmit() {
  if (submitting.value) return
  if (!form.name.trim()) {
    appStore.showError(t('admin.channels.nameRequired', 'Please enter a channel name'))
    return
  REDACTED

  // Check for pricing entries with empty models (would be silently skipped)
  for (const section of form.platforms.filter(s => s.enabled)) {
    if (section.group_ids.length === 0) {
      const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
      appStore.showError(t('admin.channels.noGroupsSelected', { platform: platformLabel REDACTED, `${platformLabelREDACTED has no selected group. Select at least one group or disable this platform.`))
      activeTab.value = section.platform
      return
    REDACTED
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) {
        const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
        appStore.showError(t('admin.channels.emptyModelsInPricing', { platform: platformLabel REDACTED, `${platformLabelREDACTED has a pricing entry without models. Add models or remove the entry.`))
        activeTab.value = section.platform
        return
      REDACTED
    REDACTED
  REDACTED

  // Check model pattern conflicts per platform (duplicate / wildcard overlap)
  for (const section of form.platforms.filter(s => s.enabled)) {
    // Collect all pricing models for this platform
    const allModels: string[] = []
    for (const entry of section.model_pricing) {
      allModels.push(...entry.models)
    REDACTED
    const pricingConflict = findModelConflict(allModels)
    if (pricingConflict) {
      appStore.showError(
        t('admin.channels.modelConflict',
          { model1: pricingConflict[0], model2: pricingConflict[1] REDACTED,
          `Model patterns '${pricingConflict[0]REDACTED' and '${pricingConflict[1]REDACTED' conflict: overlapping match range.`)
      )
      activeTab.value = section.platform
      return
    REDACTED
    // Check model mapping source pattern conflicts
    const mappingKeys = Object.keys(section.model_mapping)
    if (mappingKeys.length > 0) {
      const mappingConflict = findModelConflict(mappingKeys)
      if (mappingConflict) {
        appStore.showError(
          t('admin.channels.mappingConflict',
            { model1: mappingConflict[0], model2: mappingConflict[1] REDACTED,
            `Mapping source patterns '${mappingConflict[0]REDACTED' and '${mappingConflict[1]REDACTED' conflict: overlapping match range.`)
        )
        activeTab.value = section.platform
        return
      REDACTED
    REDACTED
  REDACTED

  // 校验 per_request/image 模式必须有价格 (只校验启用的平台)
  for (const section of form.platforms.filter(s => s.enabled)) {
    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      if ((entry.billing_mode === 'per_request' || entry.billing_mode === 'image') &&
          (entry.per_request_price == null || entry.per_request_price === '') &&
          (!entry.intervals || entry.intervals.length === 0)) {
        appStore.showError(t('admin.channels.form.perRequestPriceRequired', 'Per-request price or billing tiers required for per-request/image billing mode'))
        return
      REDACTED
    REDACTED
  REDACTED

  // 校验区间合法性（范围、重叠等）
  for (const section of form.platforms.filter(s => s.enabled)) {
    for (const entry of section.model_pricing) {
      if (!entry.intervals || entry.intervals.length === 0) continue
      const intervalErr = validateIntervals(entry.intervals, entry.billing_mode)
      if (intervalErr) {
        const platformLabel = t('admin.groups.platforms.' + section.platform, section.platform)
        const modelLabel = entry.models.join(', ') || t('admin.channels.form.unnamed')
        appStore.showError(`${platformLabelREDACTED - ${modelLabelREDACTED: ${intervalErrREDACTED`)
        activeTab.value = section.platform
        return
      REDACTED
    REDACTED
  REDACTED

  const { group_ids, model_pricing, model_mapping, features_config REDACTED = formToAPI()

  submitting.value = true
  try {
    if (editingChannel.value) {
      const req: UpdateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        status: form.status,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {REDACTED,
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models,
        features_config,
        apply_pricing_to_account_stats: form.apply_pricing_to_account_stats,
        account_stats_pricing_rules: accountStatsRulesToAPI()
      REDACTED
      await adminAPI.channels.update(editingChannel.value.id, req)
      appStore.showSuccess(t('admin.channels.updateSuccess', 'Channel updated'))
    REDACTED else {
      const req: CreateChannelRequest = {
        name: form.name.trim(),
        description: form.description.trim() || undefined,
        group_ids,
        model_pricing,
        model_mapping: Object.keys(model_mapping).length > 0 ? model_mapping : {REDACTED,
        billing_model_source: form.billing_model_source,
        restrict_models: form.restrict_models,
        features_config,
        apply_pricing_to_account_stats: form.apply_pricing_to_account_stats,
        account_stats_pricing_rules: accountStatsRulesToAPI()
      REDACTED
      await adminAPI.channels.create(req)
      appStore.showSuccess(t('admin.channels.createSuccess', 'Channel created'))
    REDACTED
    closeDialog()
    loadChannels()
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, editingChannel.value
      ? t('admin.channels.updateError', 'Failed to update channel')
      : t('admin.channels.createError', 'Failed to create channel')))
  REDACTED finally {
    submitting.value = false
  REDACTED
REDACTED

// ── Toggle status ──
async function toggleChannelStatus(channel: Channel) {
  const newStatus = channel.status === 'active' ? 'disabled' : 'active'
  try {
    await adminAPI.channels.update(channel.id, { status: newStatus REDACTED)
    if (filters.status && filters.status !== newStatus) {
      // Item no longer matches the active filter — reload list
      await loadChannels()
    REDACTED else {
      channel.status = newStatus
    REDACTED
  REDACTED catch (error) {
    appStore.showError(t('admin.channels.updateError', 'Failed to update channel'))
    console.error('Error toggling channel status:', error)
  REDACTED
REDACTED

// ── Delete ──
function handleDelete(channel: Channel) {
  deletingChannel.value = channel
  showDeleteDialog.value = true
REDACTED

async function confirmDelete() {
  if (!deletingChannel.value) return

  try {
    await adminAPI.channels.remove(deletingChannel.value.id)
    appStore.showSuccess(t('admin.channels.deleteSuccess', 'Channel deleted'))
    showDeleteDialog.value = false
    deletingChannel.value = null
    loadChannels()
  REDACTED catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.channels.deleteError', 'Failed to delete channel')))
  REDACTED
REDACTED

// ── Lifecycle ──
onMounted(() => {
  loadChannels()
  loadGroups()
  loadWebSearchGlobalState()
  document.addEventListener('click', handleRuleAccountClickOutside)
REDACTED)

onUnmounted(() => {
  clearTimeout(searchTimeout)
  abortController?.abort()
  document.removeEventListener('click', handleRuleAccountClickOutside)
  ruleAccountSearchRunner.clearAll()
  clearAllRuleAccountSearchState()
REDACTED)
</script>

<style scoped>
.channel-dialog-body {
  display: flex;
  flex-direction: column;
  height: 70vh;
  min-height: 400px;
REDACTED

.channel-tab {
  @apply flex items-center gap-1.5 px-3 py-2.5 text-sm font-medium border-b-2 transition-colors whitespace-nowrap;
REDACTED

.channel-tab-active {
  @apply border-primary-600 text-primary-600 dark:border-primary-400 dark:text-primary-400;
REDACTED

.channel-tab-inactive {
  @apply border-transparent text-gray-500 hover:text-gray-700 hover:border-gray-300 dark:text-gray-400 dark:hover:text-gray-300;
REDACTED
</style>
