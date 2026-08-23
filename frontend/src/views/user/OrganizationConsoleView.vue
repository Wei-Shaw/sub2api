<template>
  <AppLayout>
    <div class="space-y-5">
    <div class="card p-5" data-testid="organization-header">
      <div class="flex flex-wrap items-center gap-2">
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ organization?.company_name }}</h2>
        <button v-if="isOwner" class="btn btn-ghost btn-sm" @click="showRename = true">
          {{ t('organization.nameChange.action') }}
        </button>
      </div>
      <p class="mt-1 font-mono text-xs text-gray-500">{{ organization?.company_id }}</p>
    </div>

    <!--
      企业管理各子 tab 已迁移到侧边栏折叠子菜单；页内 tab bar 不再显示，
      但保留 DOM（隐藏 + sr-only）以便无障碍导航与既有单测按 id 触发 tab 切换。
      加上 hidden 属性可避免 tailwind space-y-* 把它当作可见子项计入间距，
      从而消除页面顶部标题下方多出的空白间隔。
      activeTab 仍以派生形式控制下方内容区域。
    -->
    <div v-if="visibleTabs.length" hidden class="settings-tabs-shell sr-only" aria-hidden="true">
      <nav class="settings-tabs-scroll" role="tablist" :aria-label="t('organization.console')">
        <div class="settings-tabs">
          <button
            v-for="tab in visibleTabs"
            :id="`organization-tab-${tab}`"
            :key="tab"
            type="button"
            role="tab"
            :aria-selected="activeTab === tab"
            :tabindex="activeTab === tab ? 0 : -1"
            :class="['settings-tab', activeTab === tab && 'settings-tab-active']"
            @click="selectTab(tab)"
            @keydown="handleTabKeydown($event, tab)"
          >
            <span class="settings-tab-icon"><Icon :name="tabIcons[tab]" size="sm" /></span>
            <span class="settings-tab-label">{{ t(`organization.tabs.${tab}`) }}</span>
          </button>
        </div>
      </nav>
    </div>

    <p v-if="error" class="card border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </p>
    <div v-if="loading" class="card py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

    <div v-else-if="activeTab === 'finance'" class="space-y-6">
      <section class="card space-y-4 p-5" data-testid="company-finance-summary">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.finance.companyBalance') }}</h3>
        <div class="grid gap-4 sm:grid-cols-3">
          <div v-for="field in (['company_available', 'company_frozen', 'company_total'] as const)" :key="field" class="rounded-md border border-gray-200 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-900/50">
            <div class="text-xs text-gray-500">{{ t(`organization.finance.${field}`) }}</div>
            <div class="mt-2 break-all font-mono text-xl">{{ canViewCompanyFinance ? companyAmount(finance?.[field]) : t('organization.finance.noPermission') }}</div>
          </div>
        </div>
        <div v-if="isOwner" class="space-y-2">
          <div class="flex flex-wrap items-end gap-3">
            <div class="min-w-[200px] flex-1">
              <label class="input-label" for="company-balance-amount">{{ t('organization.finance.transferAmount') }}</label>
              <input id="company-balance-amount" v-model.trim="companyBalanceAmount" class="input w-full" type="number" min="0.00000001" step="0.00000001">
            </div>
            <button class="btn btn-primary" :disabled="!canCompanyDeposit || operationKey !== ''" @click="transferCompanyBalance('deposit')">{{ t('organization.finance.deposit') }}</button>
            <button class="btn btn-secondary" :disabled="!canCompanyWithdraw || operationKey !== ''" @click="transferCompanyBalance('withdraw')">{{ t('organization.finance.withdraw') }}</button>
          </div>
          <p class="text-xs text-gray-500">
            {{ t('organization.finance.depositAvailable') }}: {{ companyAmount(finance?.available) }}
            <span class="mx-1">·</span>
            {{ t('organization.finance.withdrawAvailable') }}: {{ companyAmount(finance?.company_available) }}
          </p>
        </div>
        <p v-if="isOwner" class="text-xs text-gray-500">{{ t('organization.finance.companyBalanceHint') }}</p>
      </section>

      <section class="card space-y-4 p-5" data-testid="organization-members">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.tabs.members') }}</h3>
          <div v-if="canManageIAM" class="flex flex-wrap items-center gap-3">
            <span class="text-sm text-gray-500">{{ t('organization.members.slots', { used: usedSlots, limit: memberLimit }) }}</span>
            <button data-testid="create-iam-member" class="btn btn-primary" :disabled="usedSlots >= memberLimit || operationKey !== ''" @click="showCreate = true">
              {{ t('organization.members.create') }}
            </button>
          </div>
        </div>
        <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[960px] text-sm">
            <thead class="bg-gray-50 text-left dark:bg-dark-800">
              <tr>
                <th class="p-3">{{ t('organization.usage.username') }}</th>
                <th class="p-3">{{ t('organization.login.loginName') }}</th>
                <th class="p-3">{{ t('organization.accountId') }}</th>
                <th class="p-3">{{ t('common.status') }}</th>
                <th class="p-3">{{ t('organization.finance.available') }}</th>
                <th class="p-3">{{ t('organization.policies') }}</th>
                <th v-if="canManageIAM || canAllocateBalance" class="p-3 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="member in visibleMembers" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="p-3 font-medium" :data-testid="`member-username-${member.user_id}`">{{ member.username || member.login_name }}</td>
                <td class="p-3">
                  <div class="font-medium">{{ member.login_name }}</div>
                  <div class="flex items-center gap-1">
                    <span class="max-w-xs break-all font-mono text-xs text-gray-500">{{ member.principal }}</span>
                    <button class="icon-btn shrink-0" :title="t('keys.copyToClipboard')" :aria-label="t('keys.copyToClipboard')" @click="copyToClipboard(member.principal, t('organization.members.copied'))"><Icon name="copy" size="sm" /></button>
                  </div>
                </td>
                <td class="p-3 font-mono text-xs">{{ member.account_id }}</td>
                <td class="p-3">{{ t(`organization.status.${member.status}`) }}</td>
                <td class="p-3 font-mono">{{ companyAmount(member.balance) }}</td>
                <td class="max-w-xs p-3">
                  <span v-if="member.policy_names.length === 0">-</span>
                  <div v-else class="flex flex-wrap gap-1.5">
                    <HelpTooltip
                      v-for="policyKey in member.policy_names"
                      :key="policyKey"
                      :content="policyDescriptionForKey(policyKey)"
                      width-class="w-72 max-w-[calc(100vw-2rem)]"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          class="inline-flex max-w-full rounded border border-gray-200 bg-gray-50 px-2 py-1 text-left text-xs text-gray-700 hover:border-primary-300 hover:text-primary-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200"
                          :data-testid="`member-policy-${policyKey}`"
                          :aria-label="`${policyNameForKey(policyKey)}: ${policyDescriptionForKey(policyKey)}`"
                        >
                          <span class="break-words">{{ policyNameForKey(policyKey) }}</span>
                        </button>
                      </template>
                    </HelpTooltip>
                  </div>
                </td>
                <td v-if="canManageIAM || canAllocateBalance" class="p-3 text-right">
                  <div class="flex flex-wrap justify-end gap-1">
                    <button v-if="isOwner && member.status !== 'archived'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="openAuthorization(member)">{{ t('organization.members.authorize') }}</button>
                    <button v-if="canAllocateBalance && member.status === 'active'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="openAllocation(member)">{{ t('organization.members.allocateFunds') }}</button>
                    <button v-if="canManageIAM && member.status !== 'archived'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="resetPassword(member)">{{ t('organization.members.resetPassword') }}</button>
                    <button v-if="isOwner && member.status === 'active'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'disabled')">{{ t('organization.members.disable') }}</button>
                    <button v-else-if="isOwner && member.status === 'disabled'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'active')">{{ t('organization.members.enable') }}</button>
                    <button v-if="isOwner && member.status !== 'archived'" class="btn btn-ghost btn-sm text-red-600" :disabled="isBusy(member)" @click="archiveMember(member)">{{ t('organization.members.archive') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <div v-else-if="activeTab === 'limits'" class="space-y-6">
      <section v-if="canManageSpendLimits" class="card space-y-4 p-5" data-testid="spend-limit-form">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.spendLimits.configure') }}</h3>
          <p class="mt-1 text-sm text-gray-500">{{ t('organization.spendLimits.description') }}</p>
        </div>

        <div class="space-y-3 border-y border-gray-200 py-5 dark:border-dark-700" data-testid="spend-limit-granularity">
          <label class="input-label">{{ t('organization.spendLimits.target') }}</label>
          <div class="inline-flex rounded-md border border-gray-200 p-1 dark:border-dark-600">
            <button type="button" :class="['btn btn-sm', spendLimitForm.target === 'all' ? 'btn-primary' : 'btn-ghost']" @click="spendLimitForm.target = 'all'">
              {{ t('organization.spendLimits.allMembers') }}
            </button>
            <button type="button" :class="['btn btn-sm', spendLimitForm.target === 'members' ? 'btn-primary' : 'btn-ghost']" @click="spendLimitForm.target = 'members'">
              {{ t('organization.spendLimits.selectedMembers') }}
            </button>
          </div>
          <div v-if="spendLimitForm.target === 'members'" class="max-h-48 space-y-1 overflow-y-auto rounded-md border border-gray-200 p-2 dark:border-dark-600">
            <label v-for="member in configurableMembers" :key="member.user_id" class="flex cursor-pointer items-center gap-2 rounded px-2 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-700">
              <input v-model="spendLimitMemberIDs" type="checkbox" :value="member.user_id" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500">
              <span class="min-w-0 flex-1">
                <span class="block truncate">{{ member.username || member.login_name }}</span>
                <span class="block truncate text-xs text-gray-500">{{ member.login_name }}</span>
              </span>
              <span class="font-mono text-xs text-gray-400">{{ member.account_id }}</span>
            </label>
            <p v-if="configurableMembers.length === 0" class="p-2 text-sm text-gray-500">{{ t('organization.spendLimits.noMembers') }}</p>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="input-label" for="spend-limit-daily">{{ t('organization.spendLimits.dailyLimit') }}</label>
            <input id="spend-limit-daily" v-model.trim="spendLimitForm.daily" class="input w-full" type="number" min="0.0000000001" step="0.01" placeholder="USD">
          </div>
          <div>
            <label class="input-label" for="spend-limit-monthly">{{ t('organization.spendLimits.monthlyLimit') }}</label>
            <input id="spend-limit-monthly" v-model.trim="spendLimitForm.monthly" class="input w-full" type="number" min="0.0000000001" step="0.01" placeholder="USD">
          </div>
        </div>

        <div class="space-y-4 border-y border-gray-200 py-5 dark:border-dark-700" data-testid="spend-limit-alert-settings">
          <label class="flex items-center gap-2 text-sm font-medium text-gray-700 dark:text-gray-300">
            <input v-model="spendLimitForm.alertEnabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500">
            {{ t('organization.spendLimits.enableAlert') }}
          </label>
          <div v-if="spendLimitForm.alertEnabled" class="grid gap-4 lg:grid-cols-2">
            <div class="max-w-xs">
              <label class="input-label" for="spend-limit-threshold">{{ t('organization.spendLimits.alertThreshold') }}</label>
              <div class="flex items-center gap-2">
                <input id="spend-limit-threshold" v-model.number="spendLimitForm.threshold" class="input w-full" type="number" min="1" max="100" step="1">
                <span class="text-sm text-gray-500">%</span>
              </div>
            </div>
            <div>
              <label class="input-label" for="spend-limit-recipient-input">{{ t('organization.spendLimits.additionalRecipients') }}</label>
              <div class="flex gap-2">
                <input
                  id="spend-limit-recipient-input"
                  v-model.trim="spendLimitRecipientInput"
                  class="input min-w-0 flex-1"
                  type="email"
                  list="spend-limit-member-emails"
                  autocomplete="off"
                  :placeholder="t('organization.spendLimits.recipientsPlaceholder')"
                  @keydown.enter.prevent="addSpendLimitRecipient()"
                >
                <datalist id="spend-limit-member-emails">
                  <option v-for="option in spendLimitRecipientOptions" :key="option.email" :value="option.email" :label="option.label" />
                </datalist>
                <button type="button" class="btn btn-secondary shrink-0" @click="addSpendLimitRecipient()">{{ t('common.add') }}</button>
              </div>
              <p v-if="spendLimitRecipientError" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ spendLimitRecipientError }}</p>
              <p class="mt-1 text-xs text-gray-500">{{ t('organization.spendLimits.recipientsHint') }}</p>
              <div v-if="parsedSpendLimitRecipients.length" class="mt-2 flex flex-wrap gap-2">
                <span v-for="email in parsedSpendLimitRecipients" :key="email" class="inline-flex items-center gap-1 rounded bg-blue-50 px-2 py-1 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300">
                  {{ email }}
                  <button type="button" class="rounded p-0.5 hover:bg-blue-100 dark:hover:bg-blue-800/50" :title="t('organization.spendLimits.removeRecipient')" :aria-label="t('organization.spendLimits.removeRecipient')" @click="removeSpendLimitRecipient(email)">
                    <Icon name="x" size="xs" />
                  </button>
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="flex justify-end">
          <button class="btn btn-primary" :disabled="!canSaveSpendLimit || operationKey !== ''" @click="saveSpendLimit">
            {{ t('common.save') }}
          </button>
        </div>
      </section>

      <section v-if="canManageSpendLimits && spendLimitRules.length" class="card space-y-3 p-5" data-testid="spend-limit-rules">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.spendLimits.rules') }}</h3>
        <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[760px] text-sm">
            <thead class="bg-gray-50 text-left dark:bg-dark-800"><tr>
              <th class="p-3">{{ t('organization.spendLimits.target') }}</th>
              <th class="p-3">{{ t('organization.spendLimits.dailyLimit') }}</th>
              <th class="p-3">{{ t('organization.spendLimits.monthlyLimit') }}</th>
              <th class="p-3">{{ t('organization.spendLimits.alert') }}</th>
              <th class="p-3 text-right">{{ t('common.actions') }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="rule in spendLimitRules" :key="rule.id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="p-3">
                  <template v-if="rule.member_user_id">
                    <div class="font-medium" :data-testid="`spend-limit-rule-username-${rule.member_user_id}`">{{ rule.member_username || rule.member_login || '-' }}</div>
                    <div class="text-xs text-gray-500">{{ rule.member_login }}</div>
                  </template>
                  <span v-else class="font-medium">{{ t('organization.spendLimits.allMembers') }}</span>
                </td>
                <td class="p-3 font-mono">{{ rule.daily_limit_usd ? formatMoney(rule.daily_limit_usd) : '-' }}</td>
                <td class="p-3 font-mono">{{ rule.monthly_limit_usd ? formatMoney(rule.monthly_limit_usd) : '-' }}</td>
                <td class="p-3">{{ rule.alert_enabled ? `${rule.alert_threshold_pct}%` : t('common.disabled') }}</td>
                <td class="p-3 text-right"><button class="btn btn-ghost btn-sm text-red-600" :disabled="operationKey !== ''" @click="deleteSpendLimit(rule)">{{ t('common.delete') }}</button></td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="card space-y-3 p-5" data-testid="spend-limit-usage">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.spendLimits.currentUsage') }}</h3>
        <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[640px] text-sm">
            <thead class="bg-gray-50 text-left dark:bg-dark-800"><tr>
              <th class="p-3">{{ t('organization.usage.member') }}</th>
              <th class="p-3">{{ t('organization.spendLimits.dailyUsage') }}</th>
              <th class="p-3">{{ t('organization.spendLimits.monthlyUsage') }}</th>
            </tr></thead>
            <tbody>
              <tr v-for="item in spendLimitUsage" :key="item.member_user_id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="p-3">
                  <div class="font-medium" :data-testid="`spend-limit-usage-username-${item.member_user_id}`">{{ item.member_username || item.member_login }}</div>
                  <div class="text-xs text-gray-500">{{ item.member_login }}</div>
                </td>
                <td
                  :class="['p-3 font-mono', spendUsageExceeded(item.daily_used_usd, item.daily_limit_usd) ? 'font-semibold text-red-600 dark:text-red-400' : '']"
                  :data-testid="`spend-limit-daily-${item.member_user_id}`"
                >
                  {{ formatSpendUsage(item.daily_used_usd, item.daily_limit_usd) }}
                </td>
                <td
                  :class="['p-3 font-mono', spendUsageExceeded(item.monthly_used_usd, item.monthly_limit_usd) ? 'font-semibold text-red-600 dark:text-red-400' : '']"
                  :data-testid="`spend-limit-monthly-${item.member_user_id}`"
                >
                  {{ formatSpendUsage(item.monthly_used_usd, item.monthly_limit_usd) }}
                </td>
              </tr>
              <tr v-if="spendLimitUsage.length === 0"><td colspan="3" class="p-8 text-center text-gray-500">{{ t('organization.spendLimits.noUsage') }}</td></tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <div v-else-if="activeTab === 'dashboard'" class="space-y-6">
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <DashboardStatCard icon="key" color="blue" :label="t('admin.dashboard.apiKeys')" :value="dashboardStats?.total_api_keys ?? 0" :detail="`${dashboardStats?.active_api_keys ?? 0} ${t('common.active')}`" />
        <DashboardStatCard icon="server" color="purple" :label="t('admin.dashboard.accounts')" :value="dashboardStats?.total_accounts ?? 0" :detail="`${dashboardStats?.normal_accounts ?? 0} ${t('common.active')}`" />
        <DashboardStatCard icon="chart" color="green" :label="t('admin.dashboard.todayRequests')" :value="dashboardStats?.today_requests ?? 0" :detail="`${t('common.total')}: ${formatDashboardTokens(dashboardStats?.total_requests ?? 0)}`" />
        <DashboardStatCard
          data-testid="organization-today-cost"
          icon="dollar"
          color="purple"
          :label="t('dashboard.todayCost')"
          :value="`$${formatDashboardCost(dashboardStats?.today_actual_cost ?? 0)} / $${formatDashboardCost(dashboardStats?.today_cost ?? 0)}`"
          :detail="`${t('common.total')}: $${formatDashboardCost(dashboardStats?.total_actual_cost ?? 0)} / $${formatDashboardCost(dashboardStats?.total_cost ?? 0)}`"
        >
          <template #value>
            <span class="text-purple-600 dark:text-purple-400" :title="t('dashboard.actual')">${{ formatDashboardCost(dashboardStats?.today_actual_cost ?? 0) }}</span>
            <span class="text-sm font-normal text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')"> / ${{ formatDashboardCost(dashboardStats?.today_cost ?? 0) }}</span>
          </template>
          <template #detail>
            <span class="text-gray-500 dark:text-gray-400">{{ t('common.total') }}: </span>
            <span class="text-purple-600 dark:text-purple-400" :title="t('dashboard.actual')">${{ formatDashboardCost(dashboardStats?.total_actual_cost ?? 0) }}</span>
            <span class="text-gray-400 dark:text-gray-500" :title="t('dashboard.standard')"> / ${{ formatDashboardCost(dashboardStats?.total_cost ?? 0) }}</span>
          </template>
        </DashboardStatCard>
      </div>
      <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <DashboardStatCard icon="cube" color="amber" :label="t('admin.dashboard.todayTokens')" :value="formatDashboardTokens(dashboardStats?.today_tokens ?? 0)" :detail="dashboardCostDetail('today')">
          <template #detail><DashboardCostDetail :actual="dashboardStats?.today_actual_cost ?? 0" :account="dashboardStats?.today_account_cost ?? 0" :standard="dashboardStats?.today_cost ?? 0" /></template>
        </DashboardStatCard>
        <DashboardStatCard icon="database" color="indigo" :label="t('admin.dashboard.totalTokens')" :value="formatDashboardTokens(dashboardStats?.total_tokens ?? 0)" :detail="dashboardCostDetail('total')">
          <template #detail><DashboardCostDetail :actual="dashboardStats?.total_actual_cost ?? 0" :account="dashboardStats?.total_account_cost ?? 0" :standard="dashboardStats?.total_cost ?? 0" /></template>
        </DashboardStatCard>
        <DashboardStatCard icon="bolt" color="violet" :label="t('admin.dashboard.performance')" :value="`${formatDashboardTokens(dashboardStats?.rpm ?? 0)} RPM`" :detail="`${formatDashboardTokens(dashboardStats?.tpm ?? 0)} TPM`" />
        <DashboardStatCard icon="clock" color="rose" :label="t('admin.dashboard.avgResponse')" :value="formatDashboardDuration(dashboardStats?.average_duration_ms ?? 0)" :detail="`${dashboardStats?.active_users ?? 0} ${t('admin.dashboard.activeUsers')}`" />
      </div>
      <div class="card p-4">
        <div class="flex flex-wrap items-center gap-4">
          <div class="flex items-center gap-2"><span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') }}:</span><DateRangePicker v-model:start-date="dashboardStartDate" v-model:end-date="dashboardEndDate" @change="onDashboardDateRangeChange" /></div>
          <button type="button" class="btn btn-secondary" :disabled="dashboardChartsLoading" @click="loadDashboardAggregates">{{ t('common.refresh') }}</button>
          <div class="ml-auto flex items-center gap-2"><span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') }}:</span><div class="w-28"><Select v-model="dashboardGranularity" :options="usageGranularityOptions" @change="loadDashboardAggregates" /></div></div>
        </div>
      </div>
      <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <ModelDistributionChart :model-stats="dashboardModelStats" :loading="dashboardChartsLoading" :show-metric-toggle="false" :show-source-toggle="false" :enable-breakdown="true" :enable-ranking-view="true" :show-account-cost="true" :ranking-items="dashboardRankingItems" :ranking-total-actual-cost="dashboardRankingTotalActualCost" :ranking-total-requests="dashboardRankingTotalRequests" :ranking-total-tokens="dashboardRankingTotalTokens" :ranking-loading="dashboardChartsLoading" :start-date="dashboardStartDate" :end-date="dashboardEndDate" :breakdown-loader="loadOrganizationBreakdown" :ranking-breakdown-loader="loadOrganizationRankingModels" :enable-export="true" export-filename-prefix="organization_dashboard" />
        <TokenUsageTrend :trend-data="dashboardTrend" :loading="dashboardChartsLoading" />
      </div>
      <UserUsageTrendChart :items="userUsageTrend" :loading="dashboardChartsLoading" />
    </div>

    <div v-else-if="activeTab === 'subscriptions'" class="space-y-6">
      <p class="card p-4 text-sm text-gray-500">{{ t('organization.subscriptions.description') }}</p>

      <section v-if="canManageSubscriptions" class="card space-y-3 p-5">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.subscriptions.createTitle') }}</h3>
        <p class="text-xs text-gray-500">{{ t('organization.subscriptions.createHint') }}</p>
        <PlanPlazaCards :cards="planCards" :loading="plansLoading" emit-select @select="openPurchase" />
      </section>

      <!-- Enterprise subscription purchase modal: reuses the embedded payment flow. -->
      <div v-if="showPurchase" class="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-black/40 p-4">
        <div class="my-8 w-full max-w-2xl rounded-lg bg-white shadow-xl dark:bg-dark-800">
          <div class="flex items-center justify-between border-b border-gray-200 px-5 py-3 dark:border-dark-700">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.subscriptions.createTitle') }}</h3>
            <button type="button" class="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:hover:bg-dark-700" :aria-label="t('common.close')" @click="closePurchase"><Icon name="x" size="sm" /></button>
          </div>
          <div class="max-h-[75vh] overflow-y-auto px-5 py-3">
            <PaymentView embedded :initial-plan-id="selectedPlanId" @refresh-subscriptions="onPurchaseFulfilled" @cancel="closePurchase" />
          </div>
        </div>
      </div>

      <div v-if="subscriptions.length === 0" class="card border-dashed p-8 text-center text-sm text-gray-500">
        {{ t('organization.subscriptions.empty') }}
      </div>
      <div v-else class="card overflow-x-auto">
        <table class="w-full min-w-[820px] text-sm">
          <thead class="bg-gray-50 text-left text-xs font-medium uppercase text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <tr>
              <th class="p-3">{{ t('organization.subscriptions.group') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.rate') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.status') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.usage') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.expiresAt') }}</th>
              <th v-if="canManageSubscriptions" class="p-3">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white text-sm text-gray-900 dark:divide-dark-700 dark:bg-dark-900 dark:text-white">
            <tr v-for="item in subscriptions" :key="item.id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">
                <div class="font-medium">{{ item.group_name }}</div>
                <div class="text-xs text-gray-500">{{ item.platform }} · {{ item.subscription_type }}</div>
              </td>
              <td class="p-3 whitespace-nowrap">{{ item.rate_multiplier }}x</td>
              <td class="p-3"><span :class="subscriptionStatusClass(item.status)">{{ t(`organization.subscriptions.statuses.${item.status}`) }}</span></td>
              <td class="p-3 text-xs">
                <div>{{ t('organization.subscriptions.daily') }}: {{ formatMoney(item.daily_usage_usd) }}<template v-if="item.daily_limit_usd"> / {{ formatMoney(item.daily_limit_usd) }}</template></div>
                <div>{{ t('organization.subscriptions.monthly') }}: {{ formatMoney(item.monthly_usage_usd) }}<template v-if="item.monthly_limit_usd"> / {{ formatMoney(item.monthly_limit_usd) }}</template></div>
              </td>
              <td class="p-3 whitespace-nowrap">{{ formatSubscriptionDate(item.expires_at) }}</td>
              <td v-if="canManageSubscriptions" class="p-3">
                <button class="btn btn-ghost btn-sm text-red-600" :disabled="item.status !== 'active' || operationKey !== ''" @click="cancelSubscription(item)">{{ t('organization.subscriptions.cancel') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <section v-else-if="activeTab === 'usage'" class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        <div class="card p-4">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statRequests') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ (usageStats?.requests ?? 0).toLocaleString() }}</div>
        </div>
        <UsageTokenSummaryCard
          :input-tokens="usageStats?.input_tokens ?? 0"
          :output-tokens="usageStats?.output_tokens ?? 0"
          :cache-creation-tokens="usageStats?.cache_creation_tokens ?? 0"
          :cache-read-tokens="usageStats?.cache_read_tokens ?? 0"
          :total-tokens="usageStats?.total_tokens ?? 0"
        />
        <div class="card p-4">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statCost') }}</div>
          <div class="mt-1 break-all font-mono text-xl font-semibold">{{ companyAmount(usageStats?.actual_cost) }}</div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card p-4">
          <div class="flex flex-wrap items-center gap-4">
            <div class="flex items-center gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.timeRange') }}:</span>
              <DateRangePicker v-model:start-date="usageStartDate" v-model:end-date="usageEndDate" @change="onUsageDateRangeChange" />
            </div>
            <div class="ml-auto flex items-center gap-2">
              <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.dashboard.granularity') }}:</span>
              <div class="w-28"><Select v-model="usageGranularity" :options="usageGranularityOptions" @change="loadUsageAggregates" /></div>
            </div>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <ModelDistributionChart
            v-model:metric="distributionMetric"
            :model-stats="modelStats"
            :loading="usageChartsLoading"
            :show-metric-toggle="true"
            :show-source-toggle="false"
            :enable-breakdown="false"
            :enable-ranking-view="false"
            :show-account-cost="false"
          />
          <GroupDistributionChart
            v-model:metric="distributionMetric"
            :group-stats="groupStats"
            :loading="usageChartsLoading"
            :show-metric-toggle="true"
            :enable-breakdown="false"
            :show-account-cost="false"
          />
        </div>
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <EndpointDistributionChart
            v-model:metric="distributionMetric"
            :endpoint-stats="endpointStats"
            :loading="usageChartsLoading"
            :show-metric-toggle="true"
            :show-source-toggle="false"
            :enable-breakdown="false"
          />
          <TokenUsageTrend :trend-data="usageTrend" :loading="usageChartsLoading" />
        </div>
      </div>

      <div class="card" data-testid="organization-usage-details">
      <div class="flex border-b border-gray-200 px-2 dark:border-dark-700 sm:px-4">
        <button type="button" class="border-b-2 px-4 py-2 text-sm font-medium" :class="usageDetailTab === 'usage' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" @click="usageDetailTab = 'usage'">{{ t('usage.tabs.usage') }}</button>
        <button type="button" class="border-b-2 px-4 py-2 text-sm font-medium" :class="usageDetailTab === 'errors' ? 'border-primary-500 text-primary-600' : 'border-transparent text-gray-500'" @click="switchUsageDetailTab('errors')">{{ t('usage.tabs.errors') }}</button>
      </div>

      <div class="space-y-4 p-4 sm:p-6">
        <div class="flex flex-wrap items-end justify-between gap-4">
          <div data-testid="organization-usage-filters" class="flex flex-1 flex-wrap items-end gap-4">
          <div class="w-full sm:w-auto sm:min-w-[240px]"><label class="input-label">{{ t('organization.usage.member') }}</label><Select v-model="usageFilters.memberId" :options="usageMemberOptions" searchable @change="onUsageMemberChange" /></div>
          <div ref="usageAPIKeySearchRef" class="relative w-full sm:w-auto sm:min-w-[240px]">
            <label class="input-label">{{ t('organization.usage.apiKey') }}</label>
            <input v-model="usageAPIKeyKeyword" class="input w-full pr-8" :placeholder="t('organization.usage.searchApiKeyPlaceholder')" @input="debounceUsageAPIKeySearch" @focus="onUsageAPIKeyFocus">
            <button v-if="usageFilters.apiKeyId" type="button" class="absolute right-2 top-9 text-gray-400" aria-label="Clear API key filter" @click="clearUsageAPIKey">✕</button>
            <div v-if="showUsageAPIKeyDropdown && usageAPIKeyResults.length" class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-md border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800">
              <button v-for="key in usageAPIKeyResults" :key="key.id" type="button" class="flex w-full items-center justify-between px-4 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-700" @click="selectUsageAPIKey(key)"><span class="truncate">{{ key.name || `#${key.id}` }}</span><span class="ml-2 text-xs text-gray-400">#{{ key.id }}</span></button>
            </div>
          </div>
          <div data-testid="organization-usage-model-filter" class="w-full sm:w-auto sm:min-w-[220px]"><label class="input-label">{{ t('organization.usage.model') }}</label><Select v-model="usageFilters.model" :options="usageModelOptions" searchable @change="applyUsageFilters" /></div>
          <div v-if="usageDetailTab === 'usage'" class="w-full sm:w-auto sm:min-w-[200px]"><label class="input-label">{{ t('admin.usage.group') }}</label><Select v-model="usageFilters.groupId" :options="usageGroupOptions" searchable @change="applyUsageFilters" /></div>
          <div v-if="usageDetailTab === 'usage'" class="w-full sm:w-auto sm:min-w-[200px]"><label class="input-label">{{ t('admin.usage.billingType') }}</label><Select v-model="usageFilters.billingType" :options="usageBillingTypeOptions" @change="applyUsageFilters" /></div>
          <div v-if="usageDetailTab === 'usage'" class="w-full sm:w-auto sm:min-w-[200px]"><label class="input-label">{{ t('admin.usage.billingMode') }}</label><Select v-model="usageFilters.billingMode" :options="usageBillingModeOptions" @change="applyUsageFilters" /></div>
          <div v-else class="w-full sm:w-auto sm:min-w-[180px]"><label class="input-label">{{ t('usage.errors.category') }}</label><Select v-model="usageErrorFilters.category" :options="usageErrorCategoryOptions" @change="loadUsageErrors(1)" /></div>
          <div v-if="usageDetailTab === 'errors'" class="w-full sm:w-auto sm:min-w-[180px]"><label class="input-label">{{ t('usage.errors.status') }}</label><input v-model.trim="usageErrorFilters.statusCode" class="input w-full" type="number" min="0" :placeholder="t('usage.errors.status')" @input="debounceUsageFilterChange"></div>
          </div>
        <div class="flex w-full items-center justify-end gap-2 sm:w-auto">
        <button type="button" class="btn btn-secondary" :disabled="usageLoading || usageErrorsLoading" @click="refreshUsageDetails">
          <Icon name="refresh" size="sm" />
          {{ t('common.refresh') }}
        </button>
        <div ref="usageColumnDropdownRef" class="relative">
          <button type="button" class="btn btn-secondary w-full" @click="showUsageColumnDropdown = !showUsageColumnDropdown">
            <Icon name="grid" size="sm" />
            {{ t('organization.usage.columnSettings') }}
          </button>
          <div v-if="showUsageColumnDropdown" class="absolute right-0 top-full z-50 mt-1 max-h-80 w-52 overflow-y-auto rounded-md border border-gray-200 bg-white py-1 shadow-lg dark:border-dark-600 dark:bg-dark-800">
            <button v-for="column in currentUsageToggleableColumns" :key="column.key" type="button" class="flex w-full items-center justify-between px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700" @click="toggleCurrentUsageColumn(column.key)">
              <span>{{ column.label }}</span>
              <Icon v-if="isCurrentUsageColumnVisible(column.key)" name="check" size="sm" class="text-primary-500" />
            </button>
          </div>
        </div>
        </div>
        </div>
      </div>
      <template v-if="usageDetailTab === 'usage'">
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[1900px] divide-y divide-gray-200 text-sm dark:divide-dark-700">
          <thead class="bg-gray-50 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:bg-dark-800 dark:text-dark-400">
            <tr><th v-if="isUsageColumnVisible('member_login')" class="p-3">{{ t('organization.usage.member') }}</th><th v-if="isUsageColumnVisible('username')" class="p-3">{{ t('organization.usage.username') }}</th><th v-if="isUsageColumnVisible('api_key')" class="p-3">{{ t('organization.usage.apiKey') }}</th><th v-if="isUsageColumnVisible('model')" class="p-3">{{ t('organization.usage.model') }}</th><th v-if="isUsageColumnVisible('endpoint')" class="p-3">{{ t('organization.usage.endpoint') }}</th><th v-if="isUsageColumnVisible('group')" class="p-3">{{ t('admin.usage.group') }}</th><th v-if="isUsageColumnVisible('type')" class="p-3">{{ t('usage.type') }}</th><th v-if="isUsageColumnVisible('billing_type')" class="p-3">{{ t('admin.usage.billingType') }}</th><th v-if="isUsageColumnVisible('billing_mode')" class="p-3">{{ t('admin.usage.billingMode') }}</th><th v-if="isUsageColumnVisible('tokens')" class="p-3">{{ t('usage.tokens') }}</th><th v-if="isUsageColumnVisible('result')" class="p-3">{{ t('usage.result') }}</th><th v-if="isUsageColumnVisible('cost')" class="p-3">{{ t('usage.cost') }}</th><th v-if="isUsageColumnVisible('balance_source')" class="p-3">{{ t('organization.balanceSource.label') }}</th><th v-if="isUsageColumnVisible('latency')" class="p-3">{{ t('usage.latency') }}</th><th v-if="isUsageColumnVisible('ip_address')" class="p-3">{{ t('admin.usage.ipAddress') }}</th><th v-if="isUsageColumnVisible('user_agent')" class="p-3">{{ t('usage.userAgent') }}</th><th v-if="isUsageColumnVisible('created_at')" class="p-3">{{ t('organization.usage.time') }}</th></tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white text-sm text-gray-900 dark:divide-dark-700 dark:bg-dark-900 dark:text-gray-100">
            <tr v-for="row in usagePage.items" :key="row.id" class="hover:bg-gray-50 dark:hover:bg-dark-800">
              <td v-if="isUsageColumnVisible('member_login')" class="p-3">{{ row.member_login }}</td>
              <td v-if="isUsageColumnVisible('username')" class="p-3">{{ row.member_username || '-' }}</td>
              <td v-if="isUsageColumnVisible('api_key')" class="p-3">{{ row.api_key_name || '-' }}</td>
              <td v-if="isUsageColumnVisible('model')" class="p-3">{{ row.model }}</td>
              <td v-if="isUsageColumnVisible('endpoint')" class="max-w-xs break-all p-3">{{ row.endpoint || '-' }}</td>
              <td v-if="isUsageColumnVisible('group')" class="p-3"><span v-if="row.group_name" class="inline-flex rounded bg-indigo-100 px-2 py-0.5 text-xs font-medium text-indigo-800 dark:bg-indigo-900 dark:text-indigo-200">{{ row.group_name }}</span><span v-else>-</span></td>
              <td v-if="isUsageColumnVisible('type')" class="p-3 whitespace-nowrap"><span class="inline-flex rounded px-2 py-0.5 text-xs font-medium" :class="usageRequestTypeClass(row.request_type)">{{ usageRequestTypeLabel(row.request_type) }}</span></td>
              <td v-if="isUsageColumnVisible('billing_type')" class="p-3 whitespace-nowrap">{{ enterpriseBillingTypeLabel(row) }}</td>
              <td v-if="isUsageColumnVisible('billing_mode')" class="p-3 whitespace-nowrap"><span class="inline-flex rounded px-2 py-0.5 text-xs font-medium" :class="getBillingModeBadgeClass(row.billing_mode)">{{ getBillingModeLabel(row.billing_mode, t) }}</span></td>
              <td v-if="isUsageColumnVisible('tokens')" class="p-3"><UsageTokenBreakdown :input-tokens="row.input_tokens" :output-tokens="row.output_tokens" :cache-creation-tokens="row.cache_creation_tokens || 0" :cache-read-tokens="row.cache_read_tokens || 0" :cache-creation-5m-tokens="row.cache_creation_5m_tokens || 0" :cache-creation-1h-tokens="row.cache_creation_1h_tokens || 0" /></td>
              <td v-if="isUsageColumnVisible('result')" class="p-3">
                <div class="flex flex-col items-start gap-1">
                  <!-- 图片行：展示缩略图网格。视频行不走这里，避免将视频 URL 当图片渲染导致裂图。 -->
                  <div v-if="!isVideoUsage(row) && usageResultURLs(row).length" class="flex max-w-[180px] flex-wrap gap-1.5">
                    <a v-for="(url, index) in usageResultURLs(row)" :key="index" :href="url" target="_blank" rel="noopener noreferrer" class="block h-12 w-12 overflow-hidden rounded border border-gray-200 hover:ring-2 hover:ring-blue-400 dark:border-dark-700">
                      <img :src="url" loading="lazy" alt="result" class="h-full w-full object-cover">
                    </a>
                  </div>
                  <!-- 视频行专属详情入口：点击弹窗展示完整 task_id / 参数 / 视频预览。 -->
                  <button
                    v-if="isVideoUsage(row) && row.task_id"
                    type="button"
					:data-testid="`organization-video-detail-${row.id}`"
                    class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium text-primary-600 hover:bg-primary-50 dark:text-primary-400 dark:hover:bg-primary-500/10"
                    @click="openVideoDetail(row)"
                  >{{ t('common.detail', '详情') }}</button>
                  <span v-else-if="!usageResultURLs(row).length && !(isVideoUsage(row) && row.task_id)">-</span>
                </div>
              </td>
              <td v-if="isUsageColumnVisible('cost')" class="p-3 font-mono"><div class="font-medium text-green-600 dark:text-green-400">${{ formatUsageCost(row.actual_cost) }}</div><div class="text-[11px] text-gray-400" :title="`${t('usage.rate')}: ${row.rate_multiplier || 1}x`">${{ formatUsageCost(row.total_cost) }}</div></td>
              <td v-if="isUsageColumnVisible('balance_source')" class="p-3 whitespace-nowrap">{{ t(`organization.balanceSource.${row.balance_source || 'self'}`) }}</td>
              <td v-if="isUsageColumnVisible('latency')" class="p-3"><UsageLatencyCell :first-token-ms="row.first_token_ms" :duration-ms="row.duration_ms" /></td>
              <td v-if="isUsageColumnVisible('ip_address')" class="p-3"><span v-if="row.ip_address" class="font-mono text-sm text-gray-600 dark:text-gray-400">{{ row.ip_address }}</span><span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span></td>
              <td v-if="isUsageColumnVisible('user_agent')" class="p-3"><span v-if="row.user_agent" class="block max-w-[320px] truncate text-sm text-gray-600 dark:text-gray-400" :title="row.user_agent">{{ row.user_agent }}</span><span v-else class="text-sm text-gray-400 dark:text-gray-500">-</span></td>
              <td v-if="isUsageColumnVisible('created_at')" class="p-3 whitespace-nowrap"><span class="text-sm text-gray-600 dark:text-gray-400">{{ formatDateTime(row.created_at) }}</span></td>
            </tr>
          </tbody>
        </table>
      </div>
      <Pagination
		v-if="usagePage.total > 0"
		:page="usagePage.page"
		:total="usagePage.total"
		:page-size="usagePage.page_size"
		@update:page="loadUsage"
		@update:pageSize="onUsagePageSize"
	  />
      </template>
      <UserErrorRequestsTable
        v-else
        flat
        :rows="usageErrors.items"
        :total="usageErrors.total"
        :loading="usageErrorsLoading"
        :page="usageErrors.page"
        :page-size="usageErrors.page_size"
        :visible-column-keys="usageErrorVisibleColumnKeys"
        :detail-loader="organizationAPI.getUsageErrorDetail"
        @sort="onUsageErrorSort"
        @update:page="loadUsageErrors"
        @update:pageSize="onUsageErrorPageSize"
      />
      </div>
    </section>

    <section v-else-if="activeTab === 'audit'" class="space-y-4" data-testid="organization-audit-tab">
      <div class="card p-4 space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.audit.title') }}</h3>
            <p class="mt-1 text-xs text-gray-500">{{ t('organization.audit.description') }}</p>
          </div>
          <div class="flex flex-wrap items-end gap-3">
            <div class="min-w-[160px]">
              <label class="input-label">{{ t('organization.audit.categoryLabel') }}</label>
              <Select v-model="auditCategory" :options="auditCategoryOptions" @change="loadAuditEvents(1)" />
            </div>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="min-w-full text-sm">
            <thead class="bg-gray-50 text-left text-xs font-medium uppercase text-gray-500 dark:bg-dark-800/60">
              <tr>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.time') }}</th>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.categoryLabel') }}</th>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.actor') }}</th>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.subject') }}</th>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.action') }}</th>
                <th class="whitespace-nowrap p-3">{{ t('organization.audit.result') }}</th>
                <th class="p-3">{{ t('organization.audit.detail') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="event in auditPage.items" :key="event.id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="whitespace-nowrap p-3 font-mono text-xs">{{ formatAuditTime(event.created_at) }}</td>
                <td class="whitespace-nowrap p-3">{{ t('organization.audit.categories.' + event.category) }}</td>
                <td class="p-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ auditActorEmail(event) }}</div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ auditActorUsername(event) }}</div>
                </td>
                <td class="p-3">
                  <div class="font-medium text-gray-900 dark:text-white">{{ auditSubjectEmail(event) }}</div>
                  <div class="text-xs text-gray-500 dark:text-gray-400">{{ auditSubjectUsername(event) }}</div>
                </td>
                <td class="p-3">{{ auditActionLabel(event.action) }}</td>
                <td class="whitespace-nowrap p-3">{{ auditResultLabel(event.result) }}</td>
                <td class="p-3 text-xs text-gray-600 dark:text-gray-300">{{ auditDetailText(event) }}</td>
              </tr>
              <tr v-if="!auditPage.items.length && !auditLoading">
                <td colspan="7" class="p-6 text-center text-sm text-gray-500">{{ t('organization.audit.empty') }}</td>
              </tr>
              <tr v-if="auditLoading">
                <td colspan="7" class="p-6 text-center text-sm text-gray-500">{{ t('common.loading') }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <Pagination
          v-if="auditPage.total > 0"
          :page="auditPage.page"
          :total="auditPage.total"
          :page-size="auditPage.page_size"
          @update:page="loadAuditEvents"
          @update:pageSize="onAuditPageSize"
        />
      </div>
    </section>

    <section v-else-if="activeTab === 'settings'" class="space-y-4" data-testid="organization-settings-tab">
      <div class="card p-5 space-y-4">
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.settings.title') }}</h3>
          <p class="mt-1 text-xs text-gray-500">{{ t('organization.settings.description') }}</p>
        </div>

        <div v-if="settingsLoading" class="text-sm text-gray-500">{{ t('common.loading') }}</div>
        <div v-else class="space-y-4">
          <div class="rounded-md border border-gray-200 p-4 dark:border-dark-700">
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0 flex-1">
                <div class="text-sm font-medium text-gray-900 dark:text-white">{{ t('organization.settings.autoSwitchSubscription.label') }}</div>
                <p class="mt-1 text-xs text-gray-500 leading-relaxed">{{ t('organization.settings.autoSwitchSubscription.description') }}</p>
              </div>
              <label class="relative inline-flex shrink-0 cursor-pointer items-center">
                <input
                  v-model="settingsState.auto_switch_subscription"
                  type="checkbox"
                  class="peer sr-only"
                  :disabled="!canManageSubscriptions"
                >
                <div class="peer h-6 w-11 rounded-full bg-gray-300 after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:bg-white after:transition-all after:content-[''] peer-checked:bg-primary-600 peer-checked:after:translate-x-full peer-disabled:cursor-not-allowed peer-disabled:opacity-60 dark:bg-dark-600" />
                <span class="ml-3 text-xs text-gray-600 dark:text-dark-300">
                  {{ settingsState.auto_switch_subscription ? t('organization.settings.autoSwitchSubscription.on') : t('organization.settings.autoSwitchSubscription.off') }}
                </span>
              </label>
            </div>
          </div>

          <div v-if="settingsError" class="text-sm text-red-600">{{ settingsError }}</div>
          <div class="flex items-center gap-3">
            <button
              type="button"
              class="btn btn-primary"
              :disabled="settingsSaving || !canManageSubscriptions"
              @click="saveSettings"
            >
              {{ settingsSaving ? t('organization.settings.saving') : t('organization.settings.save') }}
            </button>
            <span v-if="settingsSaved" class="text-sm text-emerald-600">{{ t('organization.settings.saved') }}</span>
            <span v-if="!canManageSubscriptions" class="text-xs text-gray-500">{{ t('organization.settings.noPermission') }}</span>
          </div>
        </div>
      </div>
    </section>

    <div v-if="showCreate" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <form class="w-full max-w-lg space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800" @submit.prevent="createMember">
        <h3 class="font-semibold">{{ t('organization.members.create') }}</h3>
        <div>
          <label class="input-label" for="iam-member-username">{{ t('organization.members.username') }}</label>
          <input id="iam-member-username" v-model.trim="createForm.username" class="input" maxlength="100" autocomplete="off" :placeholder="t('organization.members.usernamePlaceholder')">
        </div>
        <div>
          <label class="input-label" for="iam-member-login-name">{{ t('organization.login.loginName') }}</label>
          <div class="flex min-w-0 flex-col sm:flex-row">
            <input id="iam-member-login-name" v-model.trim="createForm.loginName" class="input min-w-0 flex-1 sm:rounded-r-none" required pattern="[A-Za-z][A-Za-z0-9._-]{0,63}" autocomplete="off">
            <span data-testid="iam-principal-suffix" class="flex min-h-10 max-w-full items-center break-all rounded-md border border-gray-300 bg-gray-50 px-3 font-mono text-xs text-gray-600 sm:-ml-px sm:rounded-l-none sm:whitespace-nowrap dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300">
              @{{ organization?.company_id }}.opentk.ai
            </span>
          </div>
        </div>
        <div>
          <label class="input-label" for="iam-member-password">{{ t('organization.members.password') }}</label>
          <div class="flex min-w-0 gap-2">
            <div class="relative min-w-0 flex-1">
              <input
                id="iam-member-password"
                v-model="createForm.password"
                class="input w-full pr-10 font-mono"
                :type="passwordVisible ? 'text' : 'password'"
                required
                minlength="8"
                maxlength="72"
                autocomplete="new-password"
              >
              <button
                type="button"
                class="absolute inset-y-0 right-0 grid w-10 place-items-center text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200"
                :title="t(passwordVisible ? 'organization.members.hidePassword' : 'organization.members.showPassword')"
                :aria-label="t(passwordVisible ? 'organization.members.hidePassword' : 'organization.members.showPassword')"
                @click="passwordVisible = !passwordVisible"
              >
                <Icon :name="passwordVisible ? 'eyeOff' : 'eye'" size="sm" />
              </button>
            </div>
            <button
              type="button"
              class="icon-btn shrink-0"
              data-testid="generate-iam-password"
              :title="t('organization.members.generatePassword')"
              :aria-label="t('organization.members.generatePassword')"
              @click="generatePassword"
            >
              <Icon name="refresh" size="sm" />
            </button>
          </div>
        </div>
        <label class="flex cursor-pointer items-start gap-2 text-sm text-gray-700 dark:text-dark-200">
          <input v-model="createForm.mustChangePassword" data-testid="must-change-password" class="mt-0.5 h-4 w-4" type="checkbox">
          <span>{{ t('organization.members.mustChangePassword') }}</span>
        </label>
        <input v-model.trim="createForm.recoveryEmail" class="input" type="email" :placeholder="t('organization.members.recoveryEmail')">
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="operationKey !== ''" @click="closeCreate">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="operationKey !== ''">{{ t('common.create') }}</button></div>
      </form>
    </div>

    <div v-if="credential" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <div class="w-full max-w-lg rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
        <h3 class="font-semibold">{{ t('organization.members.oneTimeCredential') }}</h3>
        <div class="mt-4 space-y-2">
          <div class="flex items-center gap-2 rounded bg-gray-100 p-3 dark:bg-dark-900">
            <div class="min-w-0 flex-1">
              <div class="text-xs text-gray-500">{{ t('organization.login.principal') }}</div>
              <div class="break-all font-mono text-sm">{{ credential.principal }}</div>
            </div>
            <button class="icon-btn shrink-0" :title="t('keys.copyToClipboard')" :aria-label="t('keys.copyToClipboard')" @click="copyToClipboard(credential.principal, t('organization.members.copied'))"><Icon name="copy" size="sm" /></button>
          </div>
          <div class="flex items-center gap-2 rounded bg-gray-100 p-3 dark:bg-dark-900">
            <div class="min-w-0 flex-1">
              <div class="text-xs text-gray-500">{{ t('organization.members.password') }}</div>
              <div class="break-all font-mono text-sm">{{ credential.password }}</div>
            </div>
            <button class="icon-btn shrink-0" :title="t('keys.copyToClipboard')" :aria-label="t('keys.copyToClipboard')" @click="copyToClipboard(credential.password, t('organization.members.copied'))"><Icon name="copy" size="sm" /></button>
          </div>
        </div>
        <p class="mt-3 text-xs text-amber-600">{{ t('organization.members.oneTimeWarning') }}</p>
        <button class="btn btn-primary mt-4" @click="credential = null">{{ t('common.confirm') }}</button>
      </div>
    </div>

    <div v-if="allocationTarget" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <div class="w-full max-w-md space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
        <h3 class="font-semibold">{{ t('organization.members.allocateFunds') }}</h3>
        <p class="text-sm text-gray-500">{{ allocationTarget.login_name }}</p>
        <p class="text-sm text-gray-500">{{ t('organization.allocation.rootAvailable', { amount: companyAmount(finance?.company_available) }) }}</p>
        <p class="text-sm text-gray-500">{{ t('organization.allocation.targetAvailable') }}: <span class="font-mono">{{ companyAmount(allocationTarget.balance) }}</span></p>
        <div>
          <label class="input-label" for="iam-allocate-amount">{{ t('organization.allocation.amount') }}</label>
          <input id="iam-allocate-amount" v-model.trim="amounts[allocationTarget.user_id]" class="input w-full" type="number" min="0.00000001" step="0.00000001">
        </div>
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex flex-wrap justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="isBusy(allocationTarget)" @click="closeAllocation">{{ t('common.cancel') }}</button>
          <button class="btn btn-ghost" :disabled="!canReclaim(allocationTarget) || isBusy(allocationTarget)" @click="transferFromModal('reclaim')">{{ t('organization.allocation.reclaim') }}</button>
          <button class="btn btn-primary" :disabled="!canAllocate(allocationTarget) || isBusy(allocationTarget)" @click="transferFromModal('allocate')">{{ t('organization.allocation.allocate') }}</button>
        </div>
      </div>
    </div>

    <div v-if="authorizationTarget" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <div class="w-full max-w-lg space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800">
        <div>
          <h3 class="font-semibold">{{ t('organization.authorization.title', { name: authorizationTarget.login_name }) }}</h3>
          <p class="mt-1 text-sm text-gray-500">{{ t('organization.authorization.subtitle') }}</p>
        </div>
        <p v-if="!policies.length" class="py-6 text-center text-sm text-gray-500">{{ t('organization.authorization.empty') }}</p>
        <ul v-else class="max-h-96 space-y-2 overflow-y-auto">
          <li v-for="policy in policies" :key="policy.key">
            <label class="flex cursor-pointer items-start gap-3 rounded-md border border-gray-200 p-3 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-900">
              <input
                type="checkbox"
                class="mt-0.5 h-4 w-4 shrink-0"
                :checked="authorizationTarget.policy_names.includes(policy.key)"
                :disabled="isBusy(authorizationTarget)"
                @change="togglePolicy(authorizationTarget, policy.key, ($event.target as HTMLInputElement).checked)"
              >
              <span class="min-w-0 flex-1">
                <span class="block font-medium">{{ policyName(policy) }}</span>
                <span v-if="policyDescription(policy)" class="block text-xs text-gray-500">{{ policyDescription(policy) }}</span>
                <span class="mt-1 block text-xs text-gray-400">{{ policy.type }} v{{ policy.version }}</span>
              </span>
            </label>
          </li>
        </ul>
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex justify-end">
          <button type="button" class="btn btn-secondary" :disabled="isBusy(authorizationTarget)" @click="closeAuthorization">{{ t('common.close') }}</button>
        </div>
      </div>
    </div>

    <div v-if="showRename" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <form class="w-full max-w-md space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800" @submit.prevent="requestNameChange">
        <h3 class="font-semibold">{{ t('organization.nameChange.title') }}</h3>
        <input v-model.trim="requestedName" class="input" required maxlength="255" :placeholder="t('organization.companyName')">
        <p v-if="renameMessage" class="text-sm text-green-600">{{ renameMessage }}</p>
        <p v-if="modalError" class="text-sm text-red-600">{{ modalError }}</p>
        <div class="flex justify-end gap-2"><button type="button" class="btn btn-secondary" :disabled="operationKey !== ''" @click="showRename = false">{{ t('common.cancel') }}</button><button class="btn btn-primary" :disabled="operationKey !== ''">{{ t('organization.nameChange.submit') }}</button></div>
      </form>
    </div>
    </div>
    <!-- 视频任务详情弹窗：使用记录视频行"详情"按钮触发。 -->
    <VideoTaskDetailModal
      v-model:show="videoDetailVisible"
      :task-id="videoDetailTaskId"
	  :organization-usage-id="videoDetailUsageId"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { organizationAPI } from '@/api'
import type { OrganizationAuditEntry } from '@/api/organization'
import { plazaAPI } from '@/api/plaza'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import PaymentView from '@/views/user/PaymentView.vue'
import PlanPlazaCards from '@/components/plaza/PlanPlazaCards.vue'
import ModelDistributionChart from '@/components/charts/ModelDistributionChart.vue'
import GroupDistributionChart from '@/components/charts/GroupDistributionChart.vue'
import EndpointDistributionChart from '@/components/charts/EndpointDistributionChart.vue'
import TokenUsageTrend from '@/components/charts/TokenUsageTrend.vue'
import DashboardStatCard from '@/components/usage/DashboardStatCard.vue'
import DashboardCostDetail from '@/components/usage/DashboardCostDetail.vue'
import UsageTokenBreakdown from '@/components/usage/UsageTokenBreakdown.vue'
import UsageLatencyCell from '@/components/usage/UsageLatencyCell.vue'
import UserUsageTrendChart from '@/components/usage/UserUsageTrendChart.vue'
import UserErrorRequestsTable from '@/components/user/UserErrorRequestsTable.vue'
import DateRangePicker from '@/components/common/DateRangePicker.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import UsageTokenSummaryCard from '@/components/usage/UsageTokenSummaryCard.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getLocale } from '@/i18n'
import { getBillingModeBadgeClass, getBillingModeLabel, isVideoUsage } from '@/utils/billingMode'
import { formatDateTime } from '@/utils/format'
import VideoTaskDetailModal from '@/components/user/VideoTaskDetailModal.vue'
import type { DashboardStats, EndpointStat, FinanceSummary, GroupStat, IAMMember, ManagedPolicy, ModelStat, OrganizationContext, OrganizationSpendLimitRule, OrganizationSpendUsage, OrganizationSubscription, OrganizationUsageParams, OrganizationUsageRow, OrganizationUsageStats, OrganizationUsageTrendPoint, PaginatedOrganizationUsage, UserBreakdownItem, UserErrorRequest, UserSpendingRankingItem, UserUsageTrendPoint } from '@/types'
import type { PlazaPlanCard } from '@/api/plaza'
import { useAuthStore } from '@/stores'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { copyToClipboard } = useClipboard()
type Tab = 'finance' | 'limits' | 'dashboard' | 'subscriptions' | 'usage' | 'audit' | 'settings'

// Tab 与新增独立子路由的映射。旧的 /organization?tab=xxx 仍兼容，
// 但新链接一律指向独立子路由。
const tabToPath: Record<Tab, string> = {
  finance: '/organization/finance',
  limits: '/organization/limits',
  dashboard: '/organization/dashboard',
  subscriptions: '/organization/subscriptions',
  usage: '/organization/usage',
  audit: '/organization/audit',
  settings: '/organization/settings',
}
function tabFromPath(path: string): Tab | null {
  const map: Record<string, Tab> = {
    '/organization/finance': 'finance',
    '/organization/limits': 'limits',
    '/organization/dashboard': 'dashboard',
    '/organization/subscriptions': 'subscriptions',
    '/organization/usage': 'usage',
    '/organization/audit': 'audit',
    '/organization/settings': 'settings',
  }
  return map[path] ?? null
}

const activeTab = ref<Tab>('finance')
const organization = ref<OrganizationContext>()
const members = ref<IAMMember[]>([])
const spendLimitRules = ref<OrganizationSpendLimitRule[]>([])
const spendLimitUsage = ref<OrganizationSpendUsage[]>([])
const spendLimitMemberIDs = ref<number[]>([])
const spendLimitForm = reactive({ target: 'all' as 'all' | 'members', daily: '', monthly: '', alertEnabled: false, threshold: 80, recipients: '' })
const spendLimitRecipientInput = ref('')
const spendLimitRecipientError = ref('')
const policies = ref<ManagedPolicy[]>([])
const finance = ref<FinanceSummary>()
const usagePage = ref<PaginatedOrganizationUsage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const usageStats = ref<OrganizationUsageStats | null>(null)
const dashboardStats = ref<DashboardStats | null>(null)
const dashboardChartsLoading = ref(false)
const dashboardModelStats = ref<ModelStat[]>([])
const dashboardTrend = ref<OrganizationUsageTrendPoint[]>([])
const dashboardRankingItems = ref<UserSpendingRankingItem[]>([])
const dashboardRankingTotalActualCost = ref(0)
const dashboardRankingTotalRequests = ref(0)
const dashboardRankingTotalTokens = ref(0)
const userUsageTrend = ref<UserUsageTrendPoint[]>([])
const rankingItems = ref<UserSpendingRankingItem[]>([])
const rankingTotalActualCost = ref(0)
const rankingTotalRequests = ref(0)
const rankingTotalTokens = ref(0)
const usageTrend = ref<OrganizationUsageTrendPoint[]>([])
const modelStats = ref<ModelStat[]>([])
const groupStats = ref<GroupStat[]>([])
const endpointStats = ref<EndpointStat[]>([])
const usageChartsLoading = ref(false)
const distributionMetric = ref<'tokens' | 'actual_cost'>('tokens')
function formatDashboardTokens(value: number): string {
  if (value >= 1_000_000_000) return `${(value / 1_000_000_000).toFixed(2)}B`
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(2)}M`
  if (value >= 1_000) return `${(value / 1_000).toFixed(2)}K`
  return value.toLocaleString()
}
function formatDashboardDuration(ms: number): string {
  if (ms < 1000) return `${ms.toFixed(0)}ms`
  return `${(ms / 1000).toFixed(2)}s`
}
function formatDashboardCost(value: number): string {
  return value >= 1 ? value.toFixed(2) : value >= 0.01 ? value.toFixed(3) : value.toFixed(4)
}
function dashboardCostDetail(scope: 'today' | 'total'): string {
  const stats = dashboardStats.value
  if (!stats) return '$0.0000 / $0.0000 / $0.0000'
  return `$${stats[`${scope}_actual_cost`].toFixed(4)} / $${stats[`${scope}_account_cost`].toFixed(4)} / $${stats[`${scope}_cost`].toFixed(4)}`
}
const formatLocalDate = (date: Date) => `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`
const usageRangeEnd = new Date()
const usageRangeStart = new Date(usageRangeEnd.getTime() - 24 * 60 * 60 * 1000)
const usageStartDate = ref(formatLocalDate(usageRangeStart))
const usageEndDate = ref(formatLocalDate(usageRangeEnd))
const usageGranularity = ref<'hour' | 'day'>('hour')
const dashboardStartDate = ref(formatLocalDate(usageRangeStart))
const dashboardEndDate = ref(formatLocalDate(usageRangeEnd))
const dashboardGranularity = ref<'hour' | 'day'>('hour')
const usageGranularityOptions = computed(() => [{ value: 'day', label: t('admin.dashboard.day') }, { value: 'hour', label: t('admin.dashboard.hour') }])
const memberLimit = ref(20)
const usedSlots = ref(0)
const showCreate = ref(false)
const showRename = ref(false)
const requestedName = ref('')
const renameMessage = ref('')
const credential = ref<{ principal: string; password: string } | null>(null)
const allocationTarget = ref<IAMMember | null>(null)
const authorizationTarget = ref<IAMMember | null>(null)
const createForm = reactive({ username: '', loginName: '', password: '', mustChangePassword: true, recoveryEmail: '' })
const passwordVisible = ref(false)
const amounts = reactive<Record<number, string>>({})
const usageFilters = reactive({ memberId: '', apiKeyId: '', model: '', groupId: '', billingType: '', billingMode: '' })
const usageAPIKeyKeyword = ref('')
const usageAPIKeyResults = ref<Array<{ id: number; name: string }>>([])
const showUsageAPIKeyDropdown = ref(false)
const usageAPIKeySearchRef = ref<HTMLElement | null>(null)
let usageAPIKeySearchTimer: ReturnType<typeof setTimeout> | undefined
let usageFilterTimer: ReturnType<typeof setTimeout> | undefined
const usageDetailTab = ref<'usage' | 'errors'>('usage')
const usageErrors = ref<{ items: UserErrorRequest[]; total: number; page: number; page_size: number; pages: number }>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const usageErrorsLoading = ref(false)
const usageErrorFilters = reactive({ category: '', statusCode: '' })
const usageErrorCategories = ['auth', 'rate_limit', 'quota', 'invalid_request', 'service_unavailable', 'upstream', 'internal', 'cyber']
const usageErrorSort = reactive({ by: 'created_at', order: 'desc' as 'asc' | 'desc' })
const usageMemberOptions = computed(() => [
  { value: '', label: t('organization.usage.allMembers') },
  ...members.value.map(member => ({
    value: String(member.user_id),
    label: member.username ? `${member.login_name} (${member.username})` : member.login_name,
  })),
])
const usageErrorCategoryOptions = computed(() => [
  { value: '', label: t('usage.errors.allCategories') },
  ...usageErrorCategories.map(category => ({ value: category, label: t(`usage.errors.categories.${category}`) })),
])
const usageGroupOptions = computed(() => [
  { value: '', label: t('admin.usage.allGroups') },
  ...groupStats.value.filter(group => group.group_id > 0).map(group => ({ value: String(group.group_id), label: group.group_name || `#${group.group_id}` })),
])
const usageBillingTypeOptions = computed(() => [
  { value: '', label: t('admin.usage.allBillingTypes') },
  { value: '0', label: t('admin.usage.billingTypeBalance') },
  { value: '1', label: t('admin.usage.billingTypeSubscription') },
])
const usageBillingModeOptions = computed(() => [
  { value: '', label: t('admin.usage.allBillingModes') },
  { value: 'token', label: t('admin.usage.billingModeToken') },
  { value: 'per_request', label: t('admin.usage.billingModePerRequest') },
  { value: 'image', label: t('admin.usage.billingModeImage') },
  { value: 'video', label: t('admin.usage.billingModeVideo') },
])
const usageModelOptions = computed(() => {
  const names = new Set(modelStats.value.map(item => item.model).filter(Boolean))
  if (usageFilters.model) names.add(usageFilters.model)
  return [
    { value: '', label: t('admin.usage.allModels') },
    ...Array.from(names).sort().map(model => ({ value: model, label: model })),
  ]
})

const usageColumns = computed(() => [
  { key: 'member_login', label: t('organization.usage.member') },
  { key: 'username', label: t('organization.usage.username') },
  { key: 'api_key', label: t('organization.usage.apiKey') },
  { key: 'model', label: t('organization.usage.model') },
  { key: 'endpoint', label: t('organization.usage.endpoint') },
  { key: 'group', label: t('admin.usage.group') },
  { key: 'type', label: t('usage.type') },
  { key: 'billing_type', label: t('admin.usage.billingType') },
  { key: 'billing_mode', label: t('admin.usage.billingMode') },
  { key: 'tokens', label: t('usage.tokens') },
  { key: 'result', label: t('usage.result') },
  { key: 'cost', label: t('usage.cost') },
  { key: 'balance_source', label: t('organization.balanceSource.label') },
  { key: 'latency', label: t('usage.latency') },
  { key: 'ip_address', label: t('admin.usage.ipAddress') },
  { key: 'user_agent', label: t('usage.userAgent') },
  { key: 'created_at', label: t('organization.usage.time') },
])
const usageErrorColumns = computed(() => [
  { key: 'key_name', label: t('usage.errors.keyName') },
  { key: 'model', label: t('usage.errors.model') },
  { key: 'endpoint', label: t('usage.errors.endpoint') },
  { key: 'client_ip', label: 'IP' },
  { key: 'group', label: t('admin.usage.group') },
  { key: 'type', label: t('usage.type') },
  { key: 'platform', label: t('usage.errors.platform') },
  { key: 'category', label: t('usage.errors.category') },
  { key: 'status', label: t('usage.errors.status') },
  { key: 'message', label: t('usage.errors.message') },
  { key: 'created_at', label: t('usage.errors.time') },
  { key: 'user_agent', label: t('usage.userAgent') },
])
const usageHiddenColumns = reactive(new Set<string>())
const usageErrorHiddenColumns = reactive(new Set<string>())
const showUsageColumnDropdown = ref(false)
const usageColumnDropdownRef = ref<HTMLElement | null>(null)
const currentUsageToggleableColumns = computed(() => usageDetailTab.value === 'errors' ? usageErrorColumns.value : usageColumns.value)
const usageErrorVisibleColumnKeys = computed(() => usageErrorColumns.value.filter(column => !usageErrorHiddenColumns.has(column.key)).map(column => column.key))
const isUsageColumnVisible = (key: string) => !usageHiddenColumns.has(key)
const isCurrentUsageColumnVisible = (key: string) => usageDetailTab.value === 'errors' ? !usageErrorHiddenColumns.has(key) : !usageHiddenColumns.has(key)
const toggleCurrentUsageColumn = (key: string) => {
  const hidden = usageDetailTab.value === 'errors' ? usageErrorHiddenColumns : usageHiddenColumns
  if (hidden.has(key)) hidden.delete(key)
  else hidden.add(key)
  localStorage.setItem(usageDetailTab.value === 'errors' ? 'organization-usage-error-hidden-columns' : 'organization-usage-hidden-columns', JSON.stringify([...hidden]))
}
const loadUsageColumnSettings = () => {
  for (const [key, target] of [
    ['organization-usage-hidden-columns', usageHiddenColumns],
    ['organization-usage-error-hidden-columns', usageErrorHiddenColumns],
  ] as const) {
    try {
      const saved = localStorage.getItem(key)
      if (saved) (JSON.parse(saved) as string[]).forEach(column => target.add(column))
    } catch {
      target.clear()
    }
  }
}
const handleUsageColumnClickOutside = (event: MouseEvent) => {
  if (usageColumnDropdownRef.value && !usageColumnDropdownRef.value.contains(event.target as Node)) showUsageColumnDropdown.value = false
  if (usageAPIKeySearchRef.value && !usageAPIKeySearchRef.value.contains(event.target as Node)) showUsageAPIKeyDropdown.value = false
}
const loading = ref(true)
const usageLoading = ref(false)
const operationKey = ref('')
const error = ref('')
const modalError = ref('')
const subscriptions = ref<OrganizationSubscription[]>([])
const planCards = ref<PlazaPlanCard[]>([])
const plansLoading = ref(false)
const showPurchase = ref(false)
// 当前要购买的套餐 id（由点击的套餐卡片决定），透传给弹窗里的 PaymentView，
// 使其直接进入该套餐的付款确认页，而非再展示一次套餐列表。
const selectedPlanId = ref<number | null>(null)

const isOwner = computed(() => organization.value?.role === 'owner')
const actions = computed(() => organization.value?.actions || [])
const canViewCompanyFinance = computed(() => isOwner.value || actions.value.includes('organization.finance.balance.read') || actions.value.includes('organization.balance.allocate') || actions.value.includes('organization.spend_limit.manage') || actions.value.includes('organization.subscription.manage'))
const canAllocateBalance = computed(() => isOwner.value || actions.value.includes('organization.balance.allocate'))
const canManageSpendLimits = computed(() => isOwner.value || actions.value.includes('organization.spend_limit.manage'))
const canManageSubscriptions = computed(() => isOwner.value || actions.value.includes('organization.subscription.manage'))
const canManageIAM = computed(() => isOwner.value || actions.value.includes('organization.iam.member.manage'))
// 拥有仪表盘与使用记录访问能力：owner 或持有企业财务只读/管理相关的能力标志。
// CompanyFinanceReadOnly / CompanyFinanceManage 均会附带 organization.finance.balance.read 动作，
// 而 balance allocate / spend_limit manage / subscription manage 同样属于财务管理范畴，因此一并放行。
const hasFinanceReadOnly = computed(() => canViewCompanyFinance.value)
const visibleMembers = computed(() => (isOwner.value || canManageIAM.value)
  ? members.value
  : members.value.filter(member => member.user_id === auth.user?.id))
const visibleTabs = computed<Tab[]>(() => {
  if (isOwner.value) return ['finance', 'limits', 'dashboard', 'subscriptions', 'usage', 'settings', 'audit']
  const tabs: Tab[] = ['finance']
  if (canManageSpendLimits.value) tabs.push('limits')
  else if (!canViewCompanyFinance.value) tabs.push('limits') // personal spend visibility keeps limits page
  if (hasFinanceReadOnly.value) tabs.push('dashboard')
  if (canManageSubscriptions.value || canViewCompanyFinance.value) tabs.push('subscriptions')
  if (hasFinanceReadOnly.value) tabs.push('usage')
  // 企业功能设置：owner 或 CompanyFinanceManage（挂载了 subscription.manage）。
  if (canManageSubscriptions.value) tabs.push('settings')
  return tabs
})

const configurableMembers = computed(() => members.value.filter(member => member.status !== 'archived'))
const parsedSpendLimitRecipients = computed(() => spendLimitForm.recipients
  .split(/[\s,;]+/)
  .map(value => value.trim().toLowerCase())
  .filter(Boolean))
const spendLimitRecipientOptions = computed(() => {
  const selected = new Set(parsedSpendLimitRecipients.value)
  const seen = new Set<string>()
  return configurableMembers.value.flatMap(member => {
    const email = member.recovery_email?.trim().toLowerCase() || ''
    if (!email || selected.has(email) || seen.has(email)) return []
    seen.add(email)
    return [{ email, label: `${member.login_name} · ${email}` }]
  })
})
const canSaveSpendLimit = computed(() => {
  if (!spendLimitForm.daily && !spendLimitForm.monthly) return false
  if (spendLimitForm.target === 'members' && spendLimitMemberIDs.value.length === 0) return false
  return !spendLimitForm.alertEnabled || (spendLimitForm.threshold >= 1 && spendLimitForm.threshold <= 100)
})

const tabIcons: Record<Tab, 'creditCard' | 'chart' | 'sparkles'> = { finance: 'creditCard', limits: 'chart', dashboard: 'chart', subscriptions: 'sparkles', usage: 'chart', audit: 'chart', settings: 'sparkles' }
const tabKeyboardActions: Record<string, number | 'first' | 'last'> = { ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1, Home: 'first', End: 'last' }

function selectTab(tab: Tab) {
  if (!visibleTabs.value.includes(tab)) return
  activeTab.value = tab
  // 迁移策略：只有当前 URL 已经是"独立子路由"（/organization/xxx）时才切换到目标子路由；
  // 若当前是旧式 /organization?tab=xxx，则保留旧式 query 行为，避免向后不兼容。
  // 侧边栏子菜单直接使用独立 path 打开，路径判断也走这条分支进入新式行为。
  const targetPath = tabToPath[tab]
  const inSubRoute = route.path.startsWith('/organization/') && route.path !== '/organization'
  if (inSubRoute && targetPath && route.path !== targetPath) {
    void router.push({ path: targetPath })
  } else if (route.query.tab !== tab) {
    void router.push({ query: { ...route.query, tab } })
  }
}

function focusTab(tab: Tab) {
  window.requestAnimationFrame(() => {
    document.getElementById(`organization-tab-${tab}`)?.focus()
  })
}

function handleTabKeydown(event: KeyboardEvent, tab: Tab) {
  const action = tabKeyboardActions[event.key]
  if (action === undefined) return
  event.preventDefault()
  const tabs = visibleTabs.value
  if (!tabs.length) return
  const currentIndex = Math.max(0, tabs.indexOf(tab))
  let nextIndex = currentIndex
  if (action === 'first') nextIndex = 0
  else if (action === 'last') nextIndex = tabs.length - 1
  else nextIndex = (currentIndex + action + tabs.length) % tabs.length
  const nextTab = tabs[nextIndex]
  if (!nextTab) return
  selectTab(nextTab)
  focusTab(nextTab)
}

function routeTab(): Tab | null {
  // 优先从子路由推导（新方式），回退到 ?tab=xxx（旧链接兼容）。
  const fromPath = tabFromPath(route.path)
  if (fromPath) return fromPath
  const value = Array.isArray(route.query.tab) ? route.query.tab[0] : route.query.tab
  return value === 'finance' || value === 'limits' || value === 'dashboard' || value === 'subscriptions' || value === 'usage' || value === 'audit' ? value : null
}

function restoreTabFromRoute() {
  const requested = routeTab()
  const nextTab = requested && visibleTabs.value.includes(requested)
    ? requested
    : visibleTabs.value[0]
  if (!nextTab) return
  activeTab.value = nextTab
  // 只有当前处于 /organization 根路径时才回写旧式 query；
  // 在子路由上不再保留 ?tab= 参数，保持 URL 整洁。
  if (route.path === '/organization' && route.query.tab !== nextTab) {
    void router.replace({ query: { ...route.query, tab: nextTab } })
  }
}

watch(() => route.path, () => {
  if (organization.value) restoreTabFromRoute()
})

watch(() => route.query.tab, () => {
  if (organization.value) restoreTabFromRoute()
})

function errorMessage(cause: unknown): string {
  return (cause as { message?: string })?.message || t('common.error')
}

/** 将金额格式化为固定 2 位小数并带货币符号（如 $1,234.56）。 */
function formatMoney(value: string | number | null | undefined): string {
  const num = typeof value === 'number' ? value : Number(value ?? 0)
  const amount = Number.isFinite(num) ? num : 0
  return new Intl.NumberFormat(getLocale(), {
    style: 'currency',
    currency: 'USD',
    currencyDisplay: 'narrowSymbol',
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount)
}

function formatSpendUsage(used: string, limit?: string): string {
  return limit
    ? `${formatMoney(used)} / ${formatMoney(limit)}`
    : `${formatMoney(used)} · ${t('organization.spendLimits.unlimited')}`
}

function spendUsageExceeded(used: string, limit?: string): boolean {
  if (limit === undefined || limit === '') return false
  const usedAmount = Number(used)
  const limitAmount = Number(limit)
  return Number.isFinite(usedAmount) && Number.isFinite(limitAmount) && usedAmount >= limitAmount
}

function addSpendLimitRecipient(value = spendLimitRecipientInput.value) {
  const email = value.trim().toLowerCase()
  if (!email) return
  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    spendLimitRecipientError.value = t('common.invalidEmail')
    return
  }
  const recipients = [...parsedSpendLimitRecipients.value]
  if (!recipients.includes(email)) recipients.push(email)
  spendLimitForm.recipients = recipients.join(', ')
  spendLimitRecipientInput.value = ''
  spendLimitRecipientError.value = ''
}

function removeSpendLimitRecipient(email: string) {
  spendLimitForm.recipients = parsedSpendLimitRecipients.value.filter(recipient => recipient !== email).join(', ')
}

async function saveSpendLimit() {
  if (!canSaveSpendLimit.value) return
  operationKey.value = 'spend-limit-save'
  error.value = ''
  try {
    spendLimitRules.value = await organizationAPI.upsertSpendLimits({
      target: spendLimitForm.target,
      member_ids: spendLimitForm.target === 'members' ? spendLimitMemberIDs.value : undefined,
      daily_limit_usd: spendLimitForm.daily || undefined,
      monthly_limit_usd: spendLimitForm.monthly || undefined,
      alert_enabled: spendLimitForm.alertEnabled,
      alert_threshold_pct: spendLimitForm.threshold,
      additional_recipients: parsedSpendLimitRecipients.value,
    })
    spendLimitUsage.value = await organizationAPI.getSpendLimitUsage()
    spendLimitMemberIDs.value = []
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function deleteSpendLimit(rule: OrganizationSpendLimitRule) {
  if (!window.confirm(t('organization.spendLimits.deleteConfirm'))) return
  operationKey.value = `spend-limit-delete-${rule.id}`
  error.value = ''
  try {
    await organizationAPI.deleteSpendLimit(rule.member_user_id)
    const [rules, usage] = await Promise.all([organizationAPI.listSpendLimits(), organizationAPI.getSpendLimitUsage()])
    spendLimitRules.value = rules
    spendLimitUsage.value = usage
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

function formatUsageCost(value: string | number | null | undefined): string {
  const amount = Number(value ?? 0)
  return (Number.isFinite(amount) ? amount : 0).toFixed(6)
}

function usageRequestTypeLabel(type: OrganizationUsageRow['request_type']): string {
  if (type === 'ws_v2') return t('usage.ws')
  if (type === 'stream') return t('usage.stream')
  if (type === 'sync') return t('usage.sync')
  if (type === 'cyber') return t('usage.cyber')
  return t('usage.unknown')
}

function usageRequestTypeClass(type: OrganizationUsageRow['request_type']): string {
  if (type === 'cyber') return 'bg-red-100 text-red-800 dark:bg-red-900 dark:text-red-200'
  if (type === 'ws_v2') return 'bg-violet-100 text-violet-800 dark:bg-violet-900 dark:text-violet-200'
  if (type === 'stream') return 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200'
  if (type === 'sync') return 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200'
  return 'bg-amber-100 text-amber-800 dark:bg-amber-900 dark:text-amber-200'
}

function usageResultURLs(row: OrganizationUsageRow): string[] {
  return row.cos_urls?.length ? row.cos_urls : (row.image_urls || [])
}

/**
 * 企业/IAM 使用记录专用的“计费类型”短标签。
 *
 * IAM 用户没有钱包余额（self），仅有划拨/共享/套餐/企业余额，
 * 直接按 balance_source 展示对应中文短名；对 owner、subscription 也兼容。
 *   allocated    -> 划拨余额
 *   shared       -> 共享余额
 *   subscription -> 套餐
 *   company      -> 企业余额
 *   self / 其它  -> 个人钱包（owner本人才会出现）
 */
function enterpriseBillingTypeLabel(row: OrganizationUsageRow): string {
  const src = row.balance_source || ''
  if (src === 'subscription' || row.billing_type === 1) return '套餐'
  if (src === 'allocated') return '划拨余额'
  if (src === 'shared') return '共享余额'
  if (src === 'company') return '企业余额'
  return '钱包余额'
}

// 视频任务详情弹窗：企业使用记录视频行点"详情"时触发。
// 弹窗内部默认走 /user/video-models/tasks/by-id/:id，当前登录用户只能看自己发起的任务（后端强制归属校验）。
const videoDetailVisible = ref(false)
const videoDetailTaskId = ref<number | null>(null)
const videoDetailUsageId = ref<number | null>(null)
function openVideoDetail(row: OrganizationUsageRow) {
	videoDetailTaskId.value = row.task_id || null
	videoDetailUsageId.value = row.id
  videoDetailVisible.value = true
}

/** 企业余额格式化：不做千分位分组、货币符号仅保留 $（不含 US），空值返回破折号。 */
function companyAmount(value: string | number | null | undefined): string {
  if (value === null || value === undefined || value === '') return '-'
  const num = typeof value === 'number' ? value : Number(value)
  const amount = Number.isFinite(num) ? num : 0
  return new Intl.NumberFormat(getLocale(), {
    style: 'currency',
    currency: 'USD',
    currencyDisplay: 'narrowSymbol',
    useGrouping: false,
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount)
}

function subscriptionStatusClass(status: string): string {
  const base = 'inline-flex rounded-full px-2 py-0.5 text-xs font-medium '
  if (status === 'active') return `${base}bg-green-100 text-green-700 dark:bg-green-950/40 dark:text-green-300`
  if (status === 'expired') return `${base}bg-amber-100 text-amber-700 dark:bg-amber-950/40 dark:text-amber-300`
  return `${base}bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300`
}

function formatSubscriptionDate(value: string): string {
  return value ? new Date(value).toLocaleDateString() : '-'
}

async function loadSubscriptions() {
  try {
    subscriptions.value = await organizationAPI.listSubscriptions()
  } catch (cause) {
    error.value = errorMessage(cause)
  }
}

// 企业订阅套餐复用广场套餐，并用同一份订阅分组数据补齐分组属性。
// 卡片以 emit-select 模式运行：点击「立即购买」不跳转 /purchase，而是弹出内嵌
// PaymentView（企业嵌入模式）完成下单，下单主体为公司。
async function loadPlans() {
  plansLoading.value = true
  try {
    const [resp, groups] = await Promise.all([
      plazaAPI.listPlans(),
      organizationAPI.listSubscriptionGroups(),
    ])
    const groupsByID = new Map(groups.map(group => [group.id, group]))
    planCards.value = resp.cards.flatMap((card) => {
      const group = groupsByID.get(card.group_id)
      if (!group) return []
      return [{
        ...card,
        group_name: group.name,
        platform: group.platform,
        rate_multiplier: group.rate_multiplier,
      }]
    })
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    plansLoading.value = false
  }
}

function openPurchase(card: PlazaPlanCard) {
  selectedPlanId.value = card.id
  showPurchase.value = true
}

function closePurchase() {
  showPurchase.value = false
  selectedPlanId.value = null
}

function onPurchaseFulfilled() {
  showPurchase.value = false
  selectedPlanId.value = null
  void loadSubscriptions()
}

async function cancelSubscription(item: OrganizationSubscription) {
  operationKey.value = `subscription:${item.id}`
  error.value = ''
  try {
    await organizationAPI.cancelSubscription(item.id)
    await loadSubscriptions()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

function isBusy(member: IAMMember): boolean {
  return operationKey.value.startsWith(`${member.user_id}:`)
}

function positiveAmount(member: IAMMember): number {
  const value = Number(amounts[member.user_id])
  return Number.isFinite(value) && value > 0 ? value : 0
}

function canAllocate(member: IAMMember): boolean {
  return positiveAmount(member) > 0 && positiveAmount(member) <= Number(finance.value?.company_available || 0)
}

function canReclaim(member: IAMMember): boolean {
  return positiveAmount(member) > 0 && positiveAmount(member) <= Number(member.balance)
}

const companyBalanceAmount = ref('')

const companyTransferAmount = computed(() => {
  const value = Number(companyBalanceAmount.value)
  return Number.isFinite(value) && value > 0 ? value : 0
})

const canCompanyDeposit = computed(
  () => companyTransferAmount.value > 0 && companyTransferAmount.value <= Number(finance.value?.available || 0),
)

const canCompanyWithdraw = computed(
  () => companyTransferAmount.value > 0 && companyTransferAmount.value <= Number(finance.value?.company_available || 0),
)

function usageRangeISO(value: string, exclusiveEnd = false): string | undefined {
  const parsed = new Date(`${value}T00:00:00`)
  if (Number.isNaN(parsed.getTime())) return undefined
  if (exclusiveEnd) parsed.setDate(parsed.getDate() + 1)
  return parsed.toISOString()
}

function usageGranularityForRange(start: string, end: string): 'hour' | 'day' {
  return new Date(`${end}T00:00:00`).getTime() - new Date(`${start}T00:00:00`).getTime() <= 24 * 60 * 60 * 1000 ? 'hour' : 'day'
}

async function onUsageDateRangeChange(range: { startDate: string; endDate: string }) {
  usageStartDate.value = range.startDate
  usageEndDate.value = range.endDate
  usageGranularity.value = usageGranularityForRange(range.startDate, range.endDate)
  await searchUsage()
}

async function onDashboardDateRangeChange(range: { startDate: string; endDate: string }) {
  dashboardStartDate.value = range.startDate
  dashboardEndDate.value = range.endDate
  dashboardGranularity.value = usageGranularityForRange(range.startDate, range.endDate)
  await loadDashboardAggregates()
}

function organizationDashboardParams(): OrganizationUsageParams {
  return {
    start: usageRangeISO(dashboardStartDate.value),
    end: usageRangeISO(dashboardEndDate.value, true),
    granularity: dashboardGranularity.value,
  }
}

function organizationUsageParams(page: number): OrganizationUsageParams {
  return {
    page,
    page_size: usagePage.value.page_size || 20,
    member_id: usageFilters.memberId ? Number(usageFilters.memberId) : undefined,
    api_key_id: usageFilters.apiKeyId ? Number(usageFilters.apiKeyId) : undefined,
    group_id: usageFilters.groupId ? Number(usageFilters.groupId) : undefined,
    billing_type: usageFilters.billingType !== '' ? Number(usageFilters.billingType) : undefined,
    billing_mode: usageFilters.billingMode || undefined,
    model: usageFilters.model || undefined,
    start: usageRangeISO(usageStartDate.value),
    end: usageRangeISO(usageEndDate.value, true),
    granularity: usageGranularity.value,
  }
}

async function loadUsage(page = 1) {
  if (!hasFinanceReadOnly.value) return
  usageLoading.value = true
  error.value = ''
  try {
    usagePage.value = await organizationAPI.getUsage(organizationUsageParams(page))
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    usageLoading.value = false
  }
}

function onUsagePageSize(pageSize: number) {
	usagePage.value.page_size = pageSize
	void loadUsage(1)
}

// ============================================================================
// Audit log tab
// ----------------------------------------------------------------------------
// 操作记录页仅 owner 可见，聚合展示 organization_audit_events 中的充值 / 授权 /
// 划拨 / 限额配置 4 类操作。后端已通过 owner 校验 + 类别过滤实现；前端只负责
// 展示和翻页/过滤。
// ============================================================================
const auditCategory = ref<'' | 'recharge' | 'authorize' | 'allocate' | 'spend_limit'>('')
const auditPage = ref<{ items: OrganizationAuditEntry[]; total: number; page: number; page_size: number; pages: number }>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const auditLoading = ref(false)

const auditCategoryOptions = computed(() => ([
  { value: '', label: t('organization.audit.allCategories') },
  { value: 'recharge', label: t('organization.audit.categories.recharge') },
  { value: 'authorize', label: t('organization.audit.categories.authorize') },
  { value: 'allocate', label: t('organization.audit.categories.allocate') },
  { value: 'spend_limit', label: t('organization.audit.categories.spend_limit') },
]))

async function loadAuditEvents(page = 1) {
  if (!isOwner.value) return
  auditLoading.value = true
  error.value = ''
  try {
    auditPage.value = await organizationAPI.listAuditEvents({
      category: auditCategory.value || undefined,
      page,
      page_size: auditPage.value.page_size || 20,
    })
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    auditLoading.value = false
  }
}

function formatAuditTime(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return new Intl.DateTimeFormat(getLocale(), {
    year: 'numeric', month: '2-digit', day: '2-digit',
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }).format(date)
}

function auditResultLabel(result: string): string {
  const key = `organization.audit.resultValues.${result}`
  const translated = t(key)
  return translated === key ? result : translated
}

// 将后端 action 字符串翻译为用户可读的中文描述。因为 vue-i18n 会把 key
// 中的英文点号当作嵌套路径分隔符，所以把 action 里的 `.` 统一替换为 `_`，
// 与 i18n 里配置的 key 命名保持一致。未命中的 action 回退到原始字符串。
function auditActionLabel(action: string): string {
  if (!action) return '-'
  const normalized = action.replace(/\./g, '_')
  const key = `organization.audit.actions.${normalized}`
  const translated = t(key)
  return translated === key ? action : translated
}

// 操作人 / 操作对象列採用双行展示：上行邮箱（email），下行用户名
// （username）。部分 IAM 成员可能未维护 email，需要按可用字段
// 依次降级，确保至少能看到一行可识别信息。
function auditActorEmail(event: OrganizationAuditEntry): string {
  return event.actor_email || event.actor_login_name || (event.actor_user_id ? '#' + event.actor_user_id : '-')
}
function auditActorUsername(event: OrganizationAuditEntry): string {
  return event.actor_username || event.actor_login_name || '-'
}
function auditSubjectEmail(event: OrganizationAuditEntry): string {
  return event.subject_email || event.subject_login_name || (event.subject_user_id ? '#' + event.subject_user_id : '-')
}
function auditSubjectUsername(event: OrganizationAuditEntry): string {
  return event.subject_username || event.subject_login_name || '-'
}

function onAuditPageSize(pageSize: number) {
  auditPage.value.page_size = pageSize
  void loadAuditEvents(1)
}

function auditDetailText(event: OrganizationAuditEntry): string {
  if (!event.metadata) return ''
  const parts: string[] = []
  for (const [key, value] of Object.entries(event.metadata)) {
    if (value === null || value === undefined || value === '') continue
    parts.push(`${key}=${typeof value === 'object' ? JSON.stringify(value) : String(value)}`)
  }
  return parts.join(', ')
}

// 切到 audit 子路由时按需拉取。使用 watch(route.path) 已在别处，这里追加一个
// activeTab 侦听器只覆盖 audit 页；避免每次切换其他 tab 都触发一次审计请求。
watch(activeTab, tab => {
  if (tab === 'audit' && organization.value && isOwner.value) {
    void loadAuditEvents(1)
  }
  if (tab === 'settings' && organization.value && canManageSubscriptions.value) {
    void loadSettings()
  }
})

// ── Company feature settings ────────────────────────────────────────────
const settingsState = ref<{ auto_switch_subscription: boolean }>({ auto_switch_subscription: true })
const settingsLoading = ref(false)
const settingsSaving = ref(false)
const settingsSaved = ref(false)
const settingsError = ref('')

async function loadSettings() {
  settingsLoading.value = true
  settingsError.value = ''
  try {
    const data = await organizationAPI.getSettings()
    settingsState.value.auto_switch_subscription = !!data.auto_switch_subscription
  } catch (cause) {
    settingsError.value = errorMessage(cause)
  } finally {
    settingsLoading.value = false
  }
}

async function saveSettings() {
  if (!canManageSubscriptions.value) return
  settingsSaving.value = true
  settingsSaved.value = false
  settingsError.value = ''
  try {
    const data = await organizationAPI.updateSettings({
      auto_switch_subscription: settingsState.value.auto_switch_subscription,
    })
    settingsState.value.auto_switch_subscription = !!data.auto_switch_subscription
    settingsSaved.value = true
    setTimeout(() => { settingsSaved.value = false }, 2500)
  } catch (cause) {
    settingsError.value = errorMessage(cause)
  } finally {
    settingsSaving.value = false
  }
}

async function loadUsageAggregates() {
  if (!hasFinanceReadOnly.value) return
  usageChartsLoading.value = true
  try {
    const params = organizationUsageParams(1)
    const [stats, charts, ranking] = await Promise.all([
      organizationAPI.getUsageStats(params),
      organizationAPI.getUsageCharts(params),
      organizationAPI.getDashboardSpendingRanking({ ...params, limit: 12 }),
    ])
    usageStats.value = stats
    usageTrend.value = charts.trend
    modelStats.value = charts.models
    groupStats.value = charts.groups
    endpointStats.value = charts.endpoints
    rankingItems.value = ranking.ranking || []
    rankingTotalActualCost.value = ranking.total_actual_cost || 0
    rankingTotalRequests.value = ranking.total_requests || 0
    rankingTotalTokens.value = ranking.total_tokens || 0
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    usageChartsLoading.value = false
  }
}

async function loadDashboardAggregates() {
  if (!hasFinanceReadOnly.value) return
  dashboardChartsLoading.value = true
  try {
    const params = organizationDashboardParams()
    const [charts, ranking, usersTrend] = await Promise.all([
      organizationAPI.getUsageCharts(params),
      organizationAPI.getDashboardSpendingRanking({ ...params, limit: 12 }),
      organizationAPI.getDashboardUsersTrend({ ...params, limit: 12 }),
    ])
    dashboardTrend.value = charts.trend
    dashboardModelStats.value = charts.models
    dashboardRankingItems.value = ranking.ranking || []
    dashboardRankingTotalActualCost.value = ranking.total_actual_cost || 0
    dashboardRankingTotalRequests.value = ranking.total_requests || 0
    dashboardRankingTotalTokens.value = ranking.total_tokens || 0
    userUsageTrend.value = usersTrend
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    dashboardChartsLoading.value = false
  }
}

async function loadOrganizationBreakdown(params: Record<string, unknown>): Promise<{ users: UserBreakdownItem[] }> {
  return organizationAPI.getDashboardUserBreakdown({
    ...organizationDashboardParams(),
    model: typeof params.model === 'string' ? params.model : undefined,
    limit: 50,
  })
}

async function loadOrganizationRankingModels(item: UserSpendingRankingItem): Promise<ModelStat[]> {
  const charts = await organizationAPI.getUsageCharts({
    ...organizationDashboardParams(),
    member_id: item.user_id,
  })
  return charts.models
}

async function searchUsage() {
  await Promise.all([loadUsage(1), loadUsageAggregates()])
}

function applyUsageFilters() {
  if (usageDetailTab.value === 'errors') void loadUsageErrors(1)
  else void searchUsage()
}

function refreshUsageDetails() {
  applyUsageFilters()
}

function debounceUsageFilterChange() {
  if (usageFilterTimer) clearTimeout(usageFilterTimer)
  usageFilterTimer = setTimeout(applyUsageFilters, 350)
}

async function searchUsageAPIKeys() {
  try {
    usageAPIKeyResults.value = await organizationAPI.searchUsageAPIKeys(
      usageAPIKeyKeyword.value.trim(),
      usageFilters.memberId ? Number(usageFilters.memberId) : undefined,
    )
    showUsageAPIKeyDropdown.value = true
  } catch {
    usageAPIKeyResults.value = []
  }
}

function debounceUsageAPIKeySearch() {
  if (usageFilters.apiKeyId) {
    usageFilters.apiKeyId = ''
    applyUsageFilters()
  }
  if (usageAPIKeySearchTimer) clearTimeout(usageAPIKeySearchTimer)
  usageAPIKeySearchTimer = setTimeout(() => void searchUsageAPIKeys(), 300)
}

function onUsageAPIKeyFocus() {
  showUsageAPIKeyDropdown.value = true
  if (!usageAPIKeyResults.value.length) void searchUsageAPIKeys()
}

function selectUsageAPIKey(key: { id: number; name: string }) {
  usageFilters.apiKeyId = String(key.id)
  usageAPIKeyKeyword.value = key.name || String(key.id)
  showUsageAPIKeyDropdown.value = false
  applyUsageFilters()
}

function clearUsageAPIKey() {
  usageFilters.apiKeyId = ''
  usageAPIKeyKeyword.value = ''
  usageAPIKeyResults.value = []
  showUsageAPIKeyDropdown.value = false
  applyUsageFilters()
}

function onUsageMemberChange() {
  usageFilters.apiKeyId = ''
  usageAPIKeyKeyword.value = ''
  usageAPIKeyResults.value = []
  applyUsageFilters()
}

async function loadUsageErrors(page = 1) {
	if (!hasFinanceReadOnly.value) return
	usageErrorsLoading.value = true
	try {
		usageErrors.value = await organizationAPI.getUsageErrors({
			page,
			page_size: usageErrors.value.page_size,
			member_id: usageFilters.memberId ? Number(usageFilters.memberId) : undefined,
			api_key_id: usageFilters.apiKeyId ? Number(usageFilters.apiKeyId) : undefined,
			model: usageFilters.model || undefined,
			category: usageErrorFilters.category || undefined,
			status_code: usageErrorFilters.statusCode ? Number(usageErrorFilters.statusCode) : undefined,
			start: usageRangeISO(usageStartDate.value),
			end: usageRangeISO(usageEndDate.value, true),
			sort_by: usageErrorSort.by,
			sort_order: usageErrorSort.order,
		})
	} catch (cause) {
		error.value = errorMessage(cause)
	} finally {
		usageErrorsLoading.value = false
	}
}

function switchUsageDetailTab(tab: 'usage' | 'errors') {
	usageDetailTab.value = tab
	showUsageColumnDropdown.value = false
	if (tab === 'errors' && usageErrors.value.items.length === 0) void loadUsageErrors(1)
}

function onUsageErrorSort(sortBy: string, sortOrder: 'asc' | 'desc') {
	usageErrorSort.by = sortBy
	usageErrorSort.order = sortOrder
	void loadUsageErrors(1)
}

function onUsageErrorPageSize(pageSize: number) {
	usageErrors.value.page_size = pageSize
	void loadUsageErrors(1)
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const context = await organizationAPI.getContext()
    organization.value = context.organization
    finance.value = context.finance
    restoreTabFromRoute()
    if (visibleTabs.value.includes('subscriptions')) {
      await loadSubscriptions()
      if (canManageSubscriptions.value) void loadPlans()
    }
    if (isOwner.value) {
      const [memberData, policyData, rules, limitUsage] = await Promise.all([
        organizationAPI.listMembers(), organizationAPI.listPolicies(), organizationAPI.listSpendLimits(), organizationAPI.getSpendLimitUsage(),
      ])
      members.value = memberData.items
      memberLimit.value = memberData.member_limit
      usedSlots.value = memberData.used_slots
      policies.value = policyData
      spendLimitRules.value = rules
      spendLimitUsage.value = limitUsage
      const [, , dashboard] = await Promise.all([loadUsage(usagePage.value.page || 1), loadUsageAggregates(), organizationAPI.getDashboard(), loadDashboardAggregates()])
      dashboardStats.value = dashboard
    } else {
      const memberData = await organizationAPI.listMembers()
      members.value = memberData.items
      memberLimit.value = memberData.member_limit
      usedSlots.value = memberData.used_slots
      spendLimitUsage.value = await organizationAPI.getSpendLimitUsage()
      if (canManageSpendLimits.value) {
        try { spendLimitRules.value = await organizationAPI.listSpendLimits() } catch { /* ignored */ }
      }
      if (hasFinanceReadOnly.value) {
        // 财务只读 / 财务管理成员同样能看仪表盘与使用记录。
        try {
          const [, , dashboard] = await Promise.all([
            loadUsage(usagePage.value.page || 1),
            loadUsageAggregates(),
            organizationAPI.getDashboard(),
            loadDashboardAggregates(),
          ])
          dashboardStats.value = dashboard
        } catch { /* ignored */ }
      }
    }
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    loading.value = false
  }
}

async function createMember() {
  operationKey.value = 'create'
  modalError.value = ''
  try {
    const result = await organizationAPI.createMember(
      createForm.loginName,
      createForm.password,
      createForm.mustChangePassword,
      createForm.recoveryEmail || undefined,
      createForm.username || undefined,
    )
    credential.value = { principal: result.member.principal, password: result.initial_password }
    closeCreate()
    await load()
  } catch (cause) {
    modalError.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

function closeCreate() {
  showCreate.value = false
  createForm.loginName = ''
  createForm.username = ''
  createForm.password = ''
  createForm.mustChangePassword = true
  createForm.recoveryEmail = ''
  passwordVisible.value = false
  modalError.value = ''
}

function generatePassword() {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'
  const random = new Uint8Array(24)
  globalThis.crypto.getRandomValues(random)
  createForm.password = Array.from(random, value => alphabet[value & 63]).join('')
  passwordVisible.value = true
}

async function setStatus(member: IAMMember, status: IAMMember['status']) {
  operationKey.value = `${member.user_id}:status`
  error.value = ''
  try {
    await organizationAPI.setMemberStatus(member.user_id, status)
    await load()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function archiveMember(member: IAMMember) {
  if (!window.confirm(t('organization.members.archiveConfirm', { name: member.login_name }))) return
  await setStatus(member, 'archived')
}

async function resetPassword(member: IAMMember) {
  operationKey.value = `${member.user_id}:reset`
  error.value = ''
  try {
    const result = await organizationAPI.resetMemberPassword(member.user_id)
    credential.value = { principal: member.principal, password: result.initial_password }
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function togglePolicy(member: IAMMember, key: string, attached: boolean) {
  operationKey.value = `${member.user_id}:policy`
  modalError.value = ''
  error.value = ''
  try {
    await organizationAPI.setPolicy(member.user_id, key, attached)
    await load()
    await auth.refreshUser()
    if (authorizationTarget.value) {
      authorizationTarget.value = members.value.find(item => item.user_id === member.user_id) ?? null
    }
  } catch (cause) {
    const message = errorMessage(cause)
    if (authorizationTarget.value) modalError.value = message
    else error.value = message
  } finally {
    operationKey.value = ''
  }
}

function policyName(policy: ManagedPolicy): string {
  return isKnownPolicyKey(policy.key) ? policyNameForKey(policy.key) : policy.display_name
}

function policyDescription(policy: ManagedPolicy): string {
  return isKnownPolicyKey(policy.key) ? policyDescriptionForKey(policy.key) : policy.description
}

const knownPolicyKeys = new Set(['CompanyFinanceReadOnly', 'CompanySharedBalanceUse', 'CompanyFinanceManage', 'IAMUserManage'])

function isKnownPolicyKey(key: string): boolean {
  return knownPolicyKeys.has(key)
}

function policyNameForKey(key: string): string {
  return isKnownPolicyKey(key) ? t(`organization.policyMeta.${key}.name`) : key
}

function policyDescriptionForKey(key: string): string {
  return isKnownPolicyKey(key) ? t(`organization.policyMeta.${key}.description`) : ''
}

function openAuthorization(member: IAMMember) {
  authorizationTarget.value = member
  modalError.value = ''
}

function closeAuthorization() {
  authorizationTarget.value = null
  modalError.value = ''
}

async function transferCompanyBalance(operation: 'deposit' | 'withdraw') {
  if (operation === 'deposit' ? !canCompanyDeposit.value : !canCompanyWithdraw.value) return
  operationKey.value = 'company:balance'
  error.value = ''
  try {
    await organizationAPI.transferCompanyBalance(companyBalanceAmount.value, operation)
    companyBalanceAmount.value = ''
    await load()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

function openAllocation(member: IAMMember) {
  allocationTarget.value = member
  amounts[member.user_id] = ''
  modalError.value = ''
}

function closeAllocation() {
  allocationTarget.value = null
  modalError.value = ''
}

async function transferFromModal(operation: 'allocate' | 'reclaim') {
  const member = allocationTarget.value
  if (!member) return
  const amount = amounts[member.user_id]
  if (!amount || (operation === 'allocate' ? !canAllocate(member) : !canReclaim(member))) return
  operationKey.value = `${member.user_id}:balance`
  modalError.value = ''
  try {
    await organizationAPI.transferBalance(member.user_id, amount, operation)
    amounts[member.user_id] = ''
    closeAllocation()
    await load()
  } catch (cause) {
    modalError.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

async function requestNameChange() {
  if (!requestedName.value) return
  operationKey.value = 'rename'
  modalError.value = ''
  try {
    await organizationAPI.requestNameChange(requestedName.value)
    renameMessage.value = t('organization.nameChange.pending')
    requestedName.value = ''
  } catch (cause) {
    modalError.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
}

onMounted(() => {
  loadUsageColumnSettings()
  document.addEventListener('click', handleUsageColumnClickOutside)
  void load()
})

onUnmounted(() => {
  document.removeEventListener('click', handleUsageColumnClickOutside)
  if (usageAPIKeySearchTimer) clearTimeout(usageAPIKeySearchTimer)
  if (usageFilterTimer) clearTimeout(usageFilterTimer)
})
</script>

<style scoped>
/* ============ 企业控制台 Tab 导航（复用系统设置样式） ============ */
.settings-tabs-shell {
  @apply sticky z-20 -mx-1 rounded-2xl border border-white/80 bg-white/90 p-1.5 backdrop-blur-xl;
  top: 4.75rem;
  box-shadow:
    0 12px 28px rgb(15 23 42 / 0.07),
    0 1px 0 rgb(255 255 255 / 0.9) inset;
}

.settings-tabs-scroll {
  @apply overflow-x-auto;
  -ms-overflow-style: none;
  scrollbar-width: none;
}

.settings-tabs-scroll::-webkit-scrollbar {
  display: none;
}

.settings-tabs {
  @apply flex min-w-max items-center gap-1;
}

.settings-tab {
  @apply relative isolate flex h-10 min-w-[6.75rem] shrink-0 items-center justify-center gap-1.5 whitespace-nowrap rounded-xl border border-transparent px-3 text-sm font-medium text-gray-600 outline-none transition-colors duration-200 ease-out dark:text-gray-300;
}

@media (min-width: 768px) {
  .settings-tabs {
    @apply min-w-full;
  }

  .settings-tab {
    @apply min-w-0 flex-1 basis-0 overflow-hidden px-2 text-[13px];
  }

  .settings-tab-icon {
    @apply h-6 w-6;
  }
}

.settings-tab::before {
  @apply absolute inset-0 -z-10 rounded-xl opacity-0 transition-opacity duration-200;
  content: "";
  background: linear-gradient(135deg, rgb(248 250 252 / 0.95), rgb(241 245 249 / 0.8));
}

.settings-tab:hover::before,
.settings-tab:focus-visible::before {
  opacity: 1;
}

.settings-tab:focus-visible {
  @apply ring-2 ring-primary-500/40 ring-offset-2 ring-offset-white dark:ring-offset-dark-900;
}

.settings-tab-active {
  @apply border-primary-200/80 bg-white text-primary-700 shadow-sm dark:border-primary-400/30 dark:bg-dark-700/95 dark:text-primary-200;
  box-shadow:
    0 8px 18px rgb(15 23 42 / 0.08),
    0 1px 0 rgb(255 255 255 / 0.92) inset;
}

.settings-tab-active::before {
  opacity: 0;
}

.settings-tab-active::after {
  position: absolute;
  right: 0.75rem;
  bottom: 0.25rem;
  left: 0.75rem;
  height: 2px;
  border-radius: 9999px;
  content: "";
  background: linear-gradient(90deg, #14b8a6, #0ea5e9);
}

.settings-tab-icon {
  @apply flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-gray-500 transition-colors duration-200 dark:text-gray-400;
}

.settings-tab:hover .settings-tab-icon,
.settings-tab:focus-visible .settings-tab-icon {
  @apply text-gray-700 dark:text-gray-200;
}

.settings-tab-active .settings-tab-icon {
  @apply bg-primary-50 text-primary-600 dark:bg-primary-400/10 dark:text-primary-300;
}

.settings-tab-label {
  @apply min-w-0 overflow-hidden text-ellipsis whitespace-nowrap leading-none;
}
</style>

<style>
/* Dark-mode overrides for the console tabs shell. Kept in an UNSCOPED block
   because Vue's scoped-CSS compiler was dropping the `:global(.dark) ...`
   rules in the production build, leaving inactive tabs unreadable on dark. */
.dark .settings-tabs-shell {
  border-color: rgb(51 65 85 / 0.65);
  background: rgb(15 23 42 / 0.86);
  box-shadow:
    0 16px 36px rgb(0 0 0 / 0.28),
    0 1px 0 rgb(255 255 255 / 0.06) inset;
}

.dark .settings-tab::before {
  background: linear-gradient(135deg, rgb(30 41 59 / 0.9), rgb(51 65 85 / 0.62));
}

.dark .settings-tab-active {
  box-shadow:
    0 12px 26px rgb(0 0 0 / 0.22),
    0 1px 0 rgb(255 255 255 / 0.08) inset;
}
</style>
