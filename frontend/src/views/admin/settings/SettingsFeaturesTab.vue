<template>
  <div v-show="activeTab === 'features'" class="space-y-6">

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.channelMonitor.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.channelMonitor.description') }}
      </p>
      <p class="mt-1.5 text-xs">
        <router-link
          to="/admin/channels/monitor"
          class="inline-flex items-center gap-1 text-accent hover:underline"
        >
          {{ t('admin.settings.features.channelMonitor.configureLink') }}
          <span aria-hidden="true">→</span>
        </router-link>
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.channelMonitor.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.channelMonitor.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.channel_monitor_enabled" />
      </div>

      <div v-if="form.channel_monitor_enabled" class="space-y-5">
        <div>
          <label class="input-label">
            {{ t('admin.settings.features.channelMonitor.mode') }}
          </label>
          <div class="mt-1.5 inline-flex w-full max-w-md divide-x divide-border-strong border border-border-strong">
            <button
              type="button"
              class="inline-flex min-h-[40px] flex-1 items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
              :class="
                form.channel_monitor_mode === 'v2'
                  ? 'bg-accent text-white dark:text-surface-sunken'
                  : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
              "
              @click="form.channel_monitor_mode = 'v2'"
            >
              {{ t('admin.settings.features.channelMonitor.modeV2') }}
            </button>
            <button
              type="button"
              class="inline-flex min-h-[40px] flex-1 items-center justify-center gap-2 px-3 py-2 text-sm font-semibold transition-colors"
              :class="
                form.channel_monitor_mode === 'v1'
                  ? 'bg-accent text-white dark:text-surface-sunken'
                  : 'bg-surface text-fg-muted hover:bg-accent-weak hover:text-accent-strong'
              "
              @click="form.channel_monitor_mode = 'v1'"
            >
              {{ t('admin.settings.features.channelMonitor.modeV1') }}
            </button>
          </div>
          <p class="input-hint">
            {{
              form.channel_monitor_mode === 'v1'
                ? t('admin.settings.features.channelMonitor.modeV1Hint')
                : t('admin.settings.features.channelMonitor.modeV2Hint')
            }}
          </p>
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.channelMonitor.modeHint') }}
          </p>
        </div>

        <div v-if="form.channel_monitor_mode === 'v1'">
          <label class="input-label">
            {{ t('admin.settings.features.channelMonitor.defaultInterval') }}
            <span class="text-danger">*</span>
          </label>
          <input
            v-model.number="form.channel_monitor_default_interval_seconds"
            type="number"
            min="15"
            max="3600"
            class="input"
          />
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.channelMonitor.defaultIntervalHint') }}
          </p>
        </div>

        <div v-if="form.channel_monitor_mode === 'v2'" class="space-y-4">
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <p class="text-sm font-medium text-fg">
                {{ t('admin.settings.features.channelMonitor.hideThroughput') }}
              </p>
              <p class="mt-1 text-xs text-fg-muted">
                {{ t('admin.settings.features.channelMonitor.hideThroughputHint') }}
              </p>
            </div>
            <Toggle v-model="form.channel_monitor_hide_throughput" />
          </div>
          <div class="flex items-start justify-between gap-4">
            <div class="min-w-0">
              <p class="text-sm font-medium text-fg">
                {{ t('admin.settings.features.channelMonitor.hideUserRanking') }}
              </p>
              <p class="mt-1 text-xs text-fg-muted">
                {{ t('admin.settings.features.channelMonitor.hideUserRankingHint') }}
              </p>
            </div>
            <Toggle v-model="form.channel_monitor_hide_user_ranking" />
          </div>
        </div>

        <div v-if="form.channel_monitor_mode === 'v1'" class="flex items-start justify-between gap-4">
          <div class="min-w-0">
            <p class="text-sm font-medium text-fg">
              {{ t('admin.settings.features.channelMonitor.showQuota') }}
            </p>
            <p class="mt-1 text-xs text-fg-muted">
              {{ t('admin.settings.features.channelMonitor.showQuotaHint') }}
            </p>
          </div>
          <Toggle v-model="form.channel_monitor_show_quota" />
        </div>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.availableChannels.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.availableChannels.description') }}
      </p>
      <p class="mt-1.5 text-xs">
        <router-link
          to="/admin/channels/pricing"
          class="inline-flex items-center gap-1 text-accent hover:underline"
        >
          {{ t('admin.settings.features.availableChannels.configureLink') }}
          <span aria-hidden="true">→</span>
        </router-link>
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.availableChannels.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.availableChannels.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.available_channels_enabled" />
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.siteBillingMode.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.siteBillingMode.description') }}
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div class="min-w-0">
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.siteBillingMode.label') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ siteBillingModeHint }}
          </p>
        </div>
        <div class="w-full shrink-0 sm:w-56">
          <Select
            :modelValue="siteBillingMode"
            :options="siteBillingModeOptions"
            @update:modelValue="siteBillingMode = $event as SiteBillingMode"
          />
        </div>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.modelPlaza.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.modelPlaza.description') }}
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.modelPlaza.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.modelPlaza.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.model_plaza_enabled" />
      </div>

      <div v-if="form.model_plaza_enabled" class="flex items-center justify-between border-t border-border pt-4">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.modelPlaza.requireAuth') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.modelPlaza.requireAuthHint') }}
          </p>
        </div>
        <Toggle v-model="form.model_plaza_require_auth" />
      </div>

      <div v-if="form.model_plaza_enabled">
        <label class="text-sm font-medium text-fg">
          {{ t('admin.settings.features.modelPlaza.priceDescription') }}
        </label>
        <p class="mb-2 mt-0.5 text-xs text-fg-muted">
          {{ t('admin.settings.features.modelPlaza.priceDescriptionHint') }}
        </p>
        <textarea
          v-model="form.model_plaza_description"
          rows="6"
          class="input font-mono text-sm"
        ></textarea>
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.pluginManagement.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.pluginManagement.description') }}
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between gap-4">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.pluginManagement.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.pluginManagement.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.plugin_management_enabled" />
      </div>
    </div>
  </div>

  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.riskControl.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.riskControl.description') }}
      </p>
      <p class="mt-1.5 text-xs">
        <router-link
          to="/admin/risk-control"
          class="inline-flex items-center gap-1 text-accent hover:underline"
        >
          {{ t('admin.settings.features.riskControl.configureLink') }}
          <span aria-hidden="true">→</span>
        </router-link>
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.riskControl.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.riskControl.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.risk_control_enabled" />
      </div>

      <div>
        <label class="input-label">
          {{ t('admin.settings.features.riskControl.riskControlUserAllowlist') }}
        </label>
        <OpenAIFastPolicyUserSelector v-model="riskControlAllowlistedUserIds" />
        <p class="mt-1 text-xs text-fg-muted">
          {{ t('admin.settings.features.riskControl.riskControlUserAllowlistHint') }}
        </p>
      </div>

      <div class="flex items-center justify-between border-t border-border pt-4">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.riskControl.cyberSessionBlock') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.riskControl.cyberSessionBlockHint') }}
          </p>
        </div>
        <Toggle v-model="form.cyber_session_block_enabled" />
      </div>

      <div v-if="form.cyber_session_block_enabled">
        <label class="input-label">
          {{ t('admin.settings.features.riskControl.cyberSessionBlockTTL') }}
          <span class="text-danger">*</span>
        </label>
        <input
          v-model.number="form.cyber_session_block_ttl_seconds"
          type="number"
          min="1"
          class="input"
        />
      </div>
    </div>
  </div>

  <!-- Affiliate (邀请返利) feature card -->
  <div class="card">
    <div class="card-header">
      <h2 class="card-title">
        {{ t('admin.settings.features.affiliate.title') }}
      </h2>
      <p class="mt-1 text-sm text-fg-muted">
        {{ t('admin.settings.features.affiliate.description') }}
      </p>
    </div>
    <div class="card-body space-y-5">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-fg">
            {{ t('admin.settings.features.affiliate.enabled') }}
          </label>
          <p class="mt-0.5 text-xs text-fg-muted">
            {{ t('admin.settings.features.affiliate.enabledHint') }}
          </p>
        </div>
        <Toggle v-model="form.affiliate_enabled" />
      </div>

      <div v-if="form.affiliate_enabled" class="space-y-6">
        <div class="flex items-center justify-between">
          <div>
            <label class="text-sm font-medium text-fg">
              {{ t('admin.settings.features.affiliate.adminRechargeRebate') }}
            </label>
            <p class="mt-0.5 text-xs text-fg-muted">
              {{ t('admin.settings.features.affiliate.adminRechargeRebateHint') }}
            </p>
          </div>
          <Toggle v-model="form.affiliate_admin_recharge_enabled" />
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.settings.features.affiliate.rebateRate') }}
          </label>
          <div class="relative">
            <input
              v-model.number="form.affiliate_rebate_rate"
              type="number"
              step="0.01"
              min="0"
              max="100"
              class="input pr-8"
              placeholder="20"
            />
            <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-fg-subtle">%</span>
          </div>
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.rebateRateHint') }}
          </p>
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.settings.features.affiliate.freezeHours') }}
          </label>
          <input
            v-model.number="form.affiliate_rebate_freeze_hours"
            type="number"
            step="1"
            min="0"
            max="720"
            class="input"
          />
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.freezeHoursDesc') }}
          </p>
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.settings.features.affiliate.durationDays') }}
          </label>
          <input
            v-model.number="form.affiliate_rebate_duration_days"
            type="number"
            step="1"
            min="0"
            max="3650"
            class="input"
          />
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.durationDaysDesc') }}
          </p>
        </div>

        <div>
          <label class="input-label">
            {{ t('admin.settings.features.affiliate.perInviteeCap') }}
          </label>
          <input
            v-model.number="form.affiliate_rebate_per_invitee_cap"
            type="number"
            step="0.01"
            min="0"
            class="input"
          />
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.perInviteeCapDesc') }}
          </p>
        </div>

        <!-- 专属用户管理 -->
        <div class="border-t border-border pt-6">
          <div class="mb-3 flex items-center justify-between">
            <div>
              <h3 class="text-label font-bold text-fg">
                {{ t('admin.settings.features.affiliate.customUsers.title') }}
              </h3>
              <p class="mt-0.5 text-xs text-fg-muted">
                {{ t('admin.settings.features.affiliate.customUsers.description') }}
              </p>
            </div>
            <button
              type="button"
              class="btn btn-primary btn-sm"
              @click="openAffiliateModal(null)"
            >
              + {{ t('admin.settings.features.affiliate.customUsers.addButton') }}
            </button>
          </div>

          <div class="mb-3 flex items-center gap-2">
            <input
              v-model="affiliateState.search"
              type="text"
              class="input flex-1"
              :placeholder="t('admin.settings.features.affiliate.customUsers.searchPlaceholder')"
              @input="onAffiliateSearchInput"
            />
            <button
              v-if="affiliateState.selected.length > 0"
              type="button"
              class="btn btn-secondary btn-sm"
              @click="openAffiliateBatchModal"
            >
              {{ t('admin.settings.features.affiliate.customUsers.batchButton', { count: affiliateState.selected.length }) }}
            </button>
          </div>

          <div class="table-container">
            <table class="table min-w-full">
              <thead>
                <tr>
                  <th class="px-3 py-2 text-left">
                    <input
                      type="checkbox"
                      :checked="affiliateState.entries.length > 0 && affiliateState.selected.length === affiliateState.entries.length"
                      @change="toggleAffiliateSelectAll"
                    />
                  </th>
                  <th>{{ t('admin.settings.features.affiliate.customUsers.col.email') }}</th>
                  <th>{{ t('admin.settings.features.affiliate.customUsers.col.username') }}</th>
                  <th>{{ t('admin.settings.features.affiliate.customUsers.col.code') }}</th>
                  <th>{{ t('admin.settings.features.affiliate.customUsers.col.rate') }}</th>
                  <th>{{ t('admin.settings.features.affiliate.customUsers.col.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-if="affiliateState.loading">
                  <td colspan="6" class="px-3 py-6 text-center text-sm text-fg-muted">
                    {{ t('common.loading') }}
                  </td>
                </tr>
                <tr v-else-if="affiliateState.entries.length === 0">
                  <td colspan="6" class="px-3 py-6 text-center text-sm text-fg-muted">
                    {{ t('admin.settings.features.affiliate.customUsers.empty') }}
                  </td>
                </tr>
                <tr v-for="entry in affiliateState.entries" :key="entry.user_id">
                  <td class="px-3 py-2">
                    <input
                      type="checkbox"
                      :checked="affiliateState.selected.includes(entry.user_id)"
                      @change="toggleAffiliateSelect(entry.user_id)"
                    />
                  </td>
                  <td class="px-3 py-2 text-sm text-fg">{{ entry.email }}</td>
                  <td class="px-3 py-2 text-sm text-fg-muted">{{ entry.username }}</td>
                  <td class="px-3 py-2 text-sm font-mono">
                    {{ entry.aff_code }}
                    <span
                      v-if="entry.aff_code_custom"
                      class="badge badge-primary ml-1"
                    >{{ t('admin.settings.features.affiliate.customUsers.customBadge') }}</span>
                  </td>
                  <td class="text-right tabular-nums">
                    <span v-if="entry.aff_rebate_rate_percent != null">{{ entry.aff_rebate_rate_percent }}%</span>
                    <span v-else class="text-fg-subtle">{{ t('admin.settings.features.affiliate.customUsers.useGlobal') }}</span>
                  </td>
                  <td class="px-3 py-2 text-sm">
                    <div class="flex items-center gap-2">
                      <button type="button" class="text-accent hover:underline" @click="openAffiliateModal(entry)">
                        {{ t('common.edit') }}
                      </button>
                      <button
                        type="button"
                        class="text-danger hover:underline"
                        @click="askResetAffiliateUser(entry)"
                      >
                        {{ t('common.delete') }}
                      </button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="affiliateState.total > affiliateState.pageSize" class="mt-3 flex items-center justify-between text-sm">
            <span class="text-fg-muted">
              {{ t('admin.settings.features.affiliate.customUsers.totalLabel', { total: affiliateState.total }) }}
            </span>
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="affiliateState.page <= 1"
                @click="changeAffiliatePage(affiliateState.page - 1)"
              >
                {{ t('pagination.previous') }}
              </button>
              <span class="text-fg-muted">{{ affiliateState.page }} / {{ Math.max(1, Math.ceil(affiliateState.total / affiliateState.pageSize)) }}</span>
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="affiliateState.page >= Math.ceil(affiliateState.total / affiliateState.pageSize)"
                @click="changeAffiliatePage(affiliateState.page + 1)"
              >
                {{ t('pagination.next') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <!-- Affiliate add/edit modal -->
  <div
    v-if="affiliateModal.open"
    class="fixed inset-0 z-50 flex items-center justify-center bg-[rgb(12_20_33/0.55)] p-4"
    @click.self="closeAffiliateModal"
  >
    <div class="dialog-container w-full max-w-md p-6">
      <h3 class="mb-4 text-h2 font-bold text-fg">
        {{ affiliateModal.mode === 'add' ? t('admin.settings.features.affiliate.modal.addTitle') : t('admin.settings.features.affiliate.modal.editTitle') }}
      </h3>
      <div class="space-y-4">
        <div v-if="affiliateModal.mode === 'add'">
          <label class="input-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
          <!-- Chip showing the picked user; clicking it re-opens the search -->
          <div
            v-if="affiliateModal.selectedUser"
            class="flex items-center justify-between border border-accent/40 bg-accent-weak px-3 py-2"
          >
            <div class="text-sm">
              <span class="font-medium text-fg">{{ affiliateModal.selectedUser.email }}</span>
              <span class="ml-1 text-xs text-fg-muted">({{ affiliateModal.selectedUser.username }})</span>
            </div>
            <button
              type="button"
              class="text-lg leading-none text-fg-subtle hover:text-danger-strong"
              :title="t('admin.settings.features.affiliate.modal.changeUser')"
              @click="clearSelectedAffiliateUser"
            >
              ×
            </button>
          </div>
          <!-- Search input + result dropdown — hidden once a selection is made -->
          <template v-else>
            <input
              v-model="affiliateModal.userQuery"
              type="text"
              class="input"
              :placeholder="t('admin.settings.features.affiliate.modal.userPlaceholder')"
              @input="onAffiliateUserSearchInput"
            />
            <div
              v-if="affiliateModal.userResults.length > 0"
              class="mt-1 max-h-40 overflow-y-auto rounded-sm border border-border"
            >
              <button
                v-for="u in affiliateModal.userResults"
                :key="u.id"
                type="button"
                class="w-full px-3 py-1.5 text-left text-sm hover:bg-accent-weak"
                @click="selectAffiliateUser(u)"
              >
                {{ u.email }} <span class="text-xs text-fg-muted">({{ u.username }})</span>
              </button>
            </div>
          </template>
        </div>
        <div v-else>
          <label class="input-label">{{ t('admin.settings.features.affiliate.modal.userLabel') }}</label>
          <input
            type="text"
            class="input"
            :value="affiliateModal.editingEntry ? affiliateModal.editingEntry.email : ''"
            disabled
          />
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.features.affiliate.modal.codeLabel') }}</label>
          <input
            v-model="affiliateModal.code"
            type="text"
            class="input font-mono"
            :placeholder="t('admin.settings.features.affiliate.modal.codePlaceholder')"
            maxlength="32"
          />
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.modal.codeHint') }}
          </p>
        </div>

        <div>
          <label class="input-label">{{ t('admin.settings.features.affiliate.modal.rateLabel') }}</label>
          <div class="relative">
            <input
              v-model="affiliateModal.rate"
              type="number"
              step="0.01"
              min="0"
              max="100"
              class="input pr-8"
              :placeholder="t('admin.settings.features.affiliate.modal.ratePlaceholder')"
            />
            <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-fg-subtle">%</span>
          </div>
          <p class="mt-1 text-xs text-fg-subtle">
            {{ t('admin.settings.features.affiliate.modal.rateHint') }}
          </p>
        </div>
      </div>

      <div class="mt-6 flex items-center justify-between gap-3">
        <p
          v-if="!affiliateModalCanSubmit"
          class="text-xs text-fg-muted"
        >
          {{ t('admin.settings.features.affiliate.modal.errorEmpty') }}
        </p>
        <span v-else></span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" @click="closeAffiliateModal">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="affiliateModal.saving || !affiliateModalCanSubmit"
            @click="submitAffiliateModal"
          >
            {{ affiliateModal.saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </div>
    </div>
  </div>

  <!-- Affiliate batch rate modal -->
  <div
    v-if="affiliateBatchModal.open"
    class="fixed inset-0 z-50 flex items-center justify-center bg-[rgb(12_20_33/0.55)] p-4"
    @click.self="affiliateBatchModal.open = false"
  >
    <div class="dialog-container w-full max-w-md p-6">
      <h3 class="mb-4 text-h2 font-bold text-fg">
        {{ t('admin.settings.features.affiliate.batchModal.title', { count: affiliateState.selected.length }) }}
      </h3>
      <p class="mb-4 text-sm text-fg-muted">
        {{ t('admin.settings.features.affiliate.batchModal.hint') }}
      </p>
      <div class="relative">
        <input
          v-model="affiliateBatchModal.rate"
          type="number"
          step="0.01"
          min="0"
          max="100"
          class="input pr-8"
          :placeholder="t('admin.settings.features.affiliate.batchModal.placeholder')"
        />
        <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-fg-subtle">%</span>
      </div>
      <p class="mt-2 text-xs text-fg-subtle">
        {{ t('admin.settings.features.affiliate.batchModal.clearHint') }}
      </p>
      <div class="mt-6 flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" @click="affiliateBatchModal.open = false">
          {{ t('common.cancel') }}
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="affiliateBatchModal.saving"
          @click="submitAffiliateBatchModal"
        >
          {{ affiliateBatchModal.saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </div>
  </div>

  </div><!-- /Tab: Features -->
</template>

<script setup lang="ts">
import { useSettingsViewContext } from "./context";
import Toggle from "@/components/common/Toggle.vue";
import Select from "@/components/common/Select.vue";
import OpenAIFastPolicyUserSelector from "@/views/admin/settings/OpenAIFastPolicyUserSelector.vue";
import type { SiteBillingMode } from "@/utils/siteBillingMode";

// 纯移动拆分：所有状态与方法来自 SettingsView 提供的上下文（openspec: rebuild-frontend-design-system Phase 3）
const ctx = useSettingsViewContext();
const {
  activeTab,
  affiliateBatchModal,
  affiliateModal,
  affiliateModalCanSubmit,
  affiliateState,
  askResetAffiliateUser,
  changeAffiliatePage,
  clearSelectedAffiliateUser,
  closeAffiliateModal,
  form,
  onAffiliateSearchInput,
  onAffiliateUserSearchInput,
  openAffiliateBatchModal,
  openAffiliateModal,
  riskControlAllowlistedUserIds,
  selectAffiliateUser,
  siteBillingMode,
  siteBillingModeHint,
  siteBillingModeOptions,
  submitAffiliateBatchModal,
  submitAffiliateModal,
  t,
  toggleAffiliateSelect,
  toggleAffiliateSelectAll,
} = ctx;
</script>
