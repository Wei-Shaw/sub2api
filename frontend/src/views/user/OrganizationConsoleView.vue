<template>
  <AppLayout>
    <div class="space-y-5">
    <div>
      <div class="flex flex-wrap items-center gap-2">
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ organization?.company_name }}</h2>
        <button v-if="isOwner" class="btn btn-ghost btn-sm" @click="showRename = true">
          {{ t('organization.nameChange.action') }}
        </button>
      </div>
      <p class="mt-1 font-mono text-xs text-gray-500">{{ organization?.company_id }}</p>
    </div>

    <div v-if="visibleTabs.length" class="settings-tabs-shell">
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

    <p v-if="error" class="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700 dark:bg-red-950/30 dark:text-red-300">
      {{ error }}
    </p>
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">{{ t('common.loading') }}</div>

    <section v-else-if="activeTab === 'allocation'" class="space-y-3">
      <p class="text-sm text-gray-500">
        {{ t('organization.allocation.rootAvailable', { amount: companyAmount(finance?.available) }) }}
      </p>
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[720px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr><th class="p-3">{{ t('organization.login.loginName') }}</th><th class="p-3">{{ t('organization.finance.available') }}</th><th class="p-3">{{ t('organization.finance.frozen') }}</th><th class="p-3">{{ t('organization.allocation.amount') }}</th><th class="p-3">{{ t('common.actions') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="member in activeMembers" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">{{ member.login_name }}</td>
              <td class="p-3 font-mono">{{ companyAmount(member.balance) }}</td>
              <td class="p-3 font-mono">{{ companyAmount(member.frozen_balance) }}</td>
              <td class="p-3"><input v-model.trim="amounts[member.user_id]" class="input w-36 py-1.5" type="number" min="0.00000001" step="0.00000001"></td>
              <td class="p-3">
                <div class="flex gap-1">
                  <button class="btn btn-secondary btn-sm" :disabled="!canAllocate(member) || isBusy(member)" @click="transfer(member, 'allocate')">{{ t('organization.allocation.allocate') }}</button>
                  <button class="btn btn-ghost btn-sm" :disabled="!canReclaim(member) || isBusy(member)" @click="transfer(member, 'reclaim')">{{ t('organization.allocation.reclaim') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-else-if="activeTab === 'finance'" class="space-y-6">
      <section v-if="finance?.company_available !== undefined" class="space-y-4">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.finance.companyBalance') }}</h3>
        <div class="grid gap-4 sm:grid-cols-3">
          <div v-for="field in (['company_available', 'company_frozen', 'company_total'] as const)" :key="field" class="rounded-md border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
            <div class="text-xs text-gray-500">{{ t(`organization.finance.${field}`) }}</div>
            <div class="mt-2 break-all font-mono text-xl">{{ companyAmount(finance?.[field]) }}</div>
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

      <section v-if="isOwner" class="space-y-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.tabs.members') }}</h3>
          <div class="flex flex-wrap items-center gap-3">
            <span class="text-sm text-gray-500">{{ t('organization.members.slots', { used: usedSlots, limit: memberLimit }) }}</span>
            <button class="btn btn-primary" :disabled="usedSlots >= memberLimit || operationKey !== ''" @click="showCreate = true">
              {{ t('organization.members.create') }}
            </button>
          </div>
        </div>
        <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
          <table class="w-full min-w-[860px] text-sm">
            <thead class="bg-gray-50 text-left dark:bg-dark-800">
              <tr>
                <th class="p-3">{{ t('organization.login.loginName') }}</th>
                <th class="p-3">{{ t('organization.iamUserId') }}</th>
                <th class="p-3">{{ t('common.status') }}</th>
                <th class="p-3">{{ t('organization.policies') }}</th>
                <th class="p-3 text-right">{{ t('common.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="member in members" :key="member.user_id" class="border-t border-gray-100 dark:border-dark-700">
                <td class="p-3">
                  <div class="font-medium">{{ member.login_name }}</div>
                  <div class="flex items-center gap-1">
                    <span class="max-w-xs break-all font-mono text-xs text-gray-500">{{ member.principal }}</span>
                    <button class="icon-btn shrink-0" :title="t('keys.copyToClipboard')" :aria-label="t('keys.copyToClipboard')" @click="copyToClipboard(member.principal, t('organization.members.copied'))"><Icon name="copy" size="sm" /></button>
                  </div>
                </td>
                <td class="p-3 font-mono text-xs">{{ member.external_user_id }}</td>
                <td class="p-3">{{ t(`organization.status.${member.status}`) }}</td>
                <td class="max-w-xs break-words p-3">{{ member.policy_names.join(', ') || '-' }}</td>
                <td class="p-3 text-right">
                  <div class="flex flex-wrap justify-end gap-1">
                    <button v-if="member.status !== 'archived'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="openAuthorization(member)">{{ t('organization.members.authorize') }}</button>
                    <button v-if="member.status === 'active'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="openAllocation(member)">{{ t('organization.members.allocateFunds') }}</button>
                    <button v-if="member.status !== 'archived'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="resetPassword(member)">{{ t('organization.members.resetPassword') }}</button>
                    <button v-if="member.status === 'active'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'disabled')">{{ t('organization.members.disable') }}</button>
                    <button v-else-if="member.status === 'disabled'" class="btn btn-ghost btn-sm" :disabled="isBusy(member)" @click="setStatus(member, 'active')">{{ t('organization.members.enable') }}</button>
                    <button v-if="member.status !== 'archived'" class="btn btn-ghost btn-sm text-red-600" :disabled="isBusy(member)" @click="archiveMember(member)">{{ t('organization.members.archive') }}</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </div>

    <div v-else-if="activeTab === 'subscriptions'" class="space-y-6">
      <p class="text-sm text-gray-500">{{ t('organization.subscriptions.description') }}</p>

      <section v-if="isOwner" class="space-y-3 rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('organization.subscriptions.createTitle') }}</h3>
        <div class="flex flex-wrap items-end gap-3">
          <div class="min-w-[220px] flex-1">
            <label class="input-label" for="subscription-group">{{ t('organization.subscriptions.group') }}</label>
            <Select
              v-model="subscriptionForm.groupID"
              :options="subscriptionGroupOptions"
              :placeholder="t('organization.subscriptions.selectGroup')"
              :searchable="true"
            >
              <template #selected="{ option }">
                <GroupBadge
                  v-if="option"
                  :name="(option as unknown as SubscriptionGroupOption).label"
                  :platform="(option as unknown as SubscriptionGroupOption).platform"
                  :subscription-type="(option as unknown as SubscriptionGroupOption).subscriptionType"
                  :rate-multiplier="(option as unknown as SubscriptionGroupOption).rate"
                  :peak-rate-enabled="(option as unknown as SubscriptionGroupOption).peakRateEnabled"
                  :peak-start="(option as unknown as SubscriptionGroupOption).peakStart"
                  :peak-end="(option as unknown as SubscriptionGroupOption).peakEnd"
                  :peak-rate-multiplier="(option as unknown as SubscriptionGroupOption).peakRateMultiplier"
                />
                <span v-else class="text-gray-400">{{ t('organization.subscriptions.selectGroup') }}</span>
              </template>
              <template #option="{ option, selected }">
                <GroupOptionItem
                  :name="(option as unknown as SubscriptionGroupOption).label"
                  :platform="(option as unknown as SubscriptionGroupOption).platform"
                  :subscription-type="(option as unknown as SubscriptionGroupOption).subscriptionType"
                  :rate-multiplier="(option as unknown as SubscriptionGroupOption).rate"
                  :peak-rate-enabled="(option as unknown as SubscriptionGroupOption).peakRateEnabled"
                  :peak-start="(option as unknown as SubscriptionGroupOption).peakStart"
                  :peak-end="(option as unknown as SubscriptionGroupOption).peakEnd"
                  :peak-rate-multiplier="(option as unknown as SubscriptionGroupOption).peakRateMultiplier"
                  :description="(option as unknown as SubscriptionGroupOption).description"
                  :selected="selected"
                />
              </template>
            </Select>
          </div>
          <button class="btn btn-primary" :disabled="!subscriptionForm.groupID || operationKey !== ''" @click="createSubscription">{{ t('organization.subscriptions.create') }}</button>
        </div>
        <p class="text-xs text-gray-500">{{ t('organization.subscriptions.createHint') }}</p>
      </section>

      <div v-if="subscriptions.length === 0" class="rounded-md border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-dark-700">
        {{ t('organization.subscriptions.empty') }}
      </div>
      <div v-else class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[820px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr>
              <th class="p-3">{{ t('organization.subscriptions.group') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.status') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.usage') }}</th>
              <th class="p-3">{{ t('organization.subscriptions.expiresAt') }}</th>
              <th v-if="isOwner" class="p-3">{{ t('common.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in subscriptions" :key="item.id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">
                <div class="font-medium">{{ item.group_name }}</div>
                <div class="text-xs text-gray-500">{{ item.platform }} · {{ item.subscription_type }}</div>
              </td>
              <td class="p-3"><span :class="subscriptionStatusClass(item.status)">{{ t(`organization.subscriptions.statuses.${item.status}`) }}</span></td>
              <td class="p-3 text-xs">
                <div>{{ t('organization.subscriptions.daily') }}: {{ formatMoney(item.daily_usage_usd) }}<template v-if="item.daily_limit_usd"> / {{ formatMoney(item.daily_limit_usd) }}</template></div>
                <div>{{ t('organization.subscriptions.monthly') }}: {{ formatMoney(item.monthly_usage_usd) }}<template v-if="item.monthly_limit_usd"> / {{ formatMoney(item.monthly_limit_usd) }}</template></div>
              </td>
              <td class="p-3 whitespace-nowrap">{{ formatSubscriptionDate(item.expires_at) }}</td>
              <td v-if="isOwner" class="p-3">
                <button class="btn btn-ghost btn-sm text-red-600" :disabled="item.status !== 'active' || operationKey !== ''" @click="cancelSubscription(item)">{{ t('organization.subscriptions.cancel') }}</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <section v-else class="space-y-4">
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statRequests') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ (usageStats?.requests ?? 0).toLocaleString() }}</div>
        </div>
        <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statInputTokens') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ (usageStats?.input_tokens ?? 0).toLocaleString() }}</div>
        </div>
        <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statOutputTokens') }}</div>
          <div class="mt-1 text-xl font-semibold">{{ (usageStats?.output_tokens ?? 0).toLocaleString() }}</div>
        </div>
        <div class="rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="text-xs text-gray-500">{{ t('organization.usage.statCost') }}</div>
          <div class="mt-1 break-all font-mono text-xl font-semibold">{{ companyAmount(usageStats?.actual_cost) }}</div>
        </div>
      </div>

      <section class="rounded-md border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="mb-3 text-sm font-medium">{{ t('organization.usage.trendTitle') }}</div>
        <p v-if="!usageTrend.length" class="py-6 text-center text-sm text-gray-500">{{ t('organization.usage.trendEmpty') }}</p>
        <div v-else class="flex h-40 items-end gap-1">
          <div
            v-for="point in usageTrend"
            :key="point.bucket"
            class="flex min-w-0 flex-1 flex-col items-center justify-end"
            :title="`${new Date(point.bucket).toLocaleDateString()} · ${point.requests} ${t('organization.usage.statRequests')} · ${point.tokens} ${t('organization.usage.tokens')} · ${formatMoney(point.actual_cost)}`"
          >
            <div class="w-full rounded-t bg-blue-500/70 transition-all dark:bg-blue-400/70" :style="{ height: trendBarHeight(point) }"></div>
            <div class="mt-1 w-full truncate text-center text-[10px] text-gray-400">{{ new Date(point.bucket).toLocaleDateString(undefined, { month: 'numeric', day: 'numeric' }) }}</div>
          </div>
        </div>
      </section>

      <form class="grid gap-3 md:grid-cols-3 xl:grid-cols-4" @submit.prevent="searchUsage">
        <select v-model="usageFilters.memberId" class="input">
          <option value="">{{ t('organization.usage.allMembers') }}</option>
          <option v-for="member in members" :key="member.user_id" :value="String(member.user_id)">{{ member.login_name }}</option>
        </select>
        <input v-model.trim="usageFilters.apiKeyId" class="input" type="number" min="1" :placeholder="t('organization.usage.apiKeyId')">
        <input v-model.trim="usageFilters.model" class="input" :placeholder="t('organization.usage.model')">
        <input v-model.trim="usageFilters.endpoint" class="input" :placeholder="t('organization.usage.endpoint')">
        <select v-model="usageFilters.status" class="input">
          <option value="">{{ t('common.all') }}</option>
          <option value="charged">{{ t('organization.usage.charged') }}</option>
          <option value="refunded">{{ t('organization.usage.refunded') }}</option>
        </select>
        <input v-model="usageFilters.start" class="input" type="datetime-local" :aria-label="t('organization.usage.start')">
        <input v-model="usageFilters.end" class="input" type="datetime-local" :aria-label="t('organization.usage.end')">
        <button class="btn btn-secondary" type="submit" :disabled="usageLoading">{{ t('common.search') }}</button>
      </form>
      <div class="overflow-x-auto rounded-md border border-gray-200 dark:border-dark-700">
        <table class="w-full min-w-[1180px] text-sm">
          <thead class="bg-gray-50 text-left dark:bg-dark-800">
            <tr><th class="p-3">{{ t('organization.usage.member') }}</th><th class="p-3">{{ t('organization.usage.apiKey') }}</th><th class="p-3">{{ t('organization.usage.model') }}</th><th class="p-3">{{ t('organization.usage.endpoint') }}</th><th class="p-3">{{ t('common.status') }}</th><th class="p-3">{{ t('organization.usage.tokens') }}</th><th class="p-3">{{ t('organization.usage.charge') }}</th><th class="p-3">{{ t('organization.balanceSource.label') }}</th><th class="p-3">{{ t('organization.usage.duration') }}</th><th class="p-3">{{ t('organization.usage.time') }}</th></tr>
          </thead>
          <tbody>
            <tr v-for="row in usagePage.items" :key="row.id" class="border-t border-gray-100 dark:border-dark-700">
              <td class="p-3">{{ row.member_login }}</td><td class="p-3">{{ row.api_key_name || '-' }}</td><td class="p-3">{{ row.model }}</td><td class="max-w-xs break-all p-3">{{ row.endpoint || '-' }}</td><td class="p-3">{{ row.status }}</td><td class="p-3">{{ row.input_tokens + row.output_tokens }}</td><td class="p-3 font-mono">{{ formatMoney(row.actual_cost) }}</td><td class="p-3 whitespace-nowrap">{{ t(`organization.balanceSource.${row.balance_source || 'self'}`) }}</td><td class="p-3">{{ row.duration_ms ?? '-' }}</td><td class="p-3 whitespace-nowrap">{{ new Date(row.created_at).toLocaleString() }}</td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="flex items-center justify-between gap-3 text-sm">
        <span class="text-gray-500">{{ t('organization.usage.total', { total: usagePage.total }) }}</span>
        <div class="flex gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="usageLoading || usagePage.page <= 1" @click="loadUsage(usagePage.page - 1)">{{ t('organization.usage.previous') }}</button>
          <button class="btn btn-secondary btn-sm" :disabled="usageLoading || usagePage.page >= usagePage.pages" @click="loadUsage(usagePage.page + 1)">{{ t('organization.usage.next') }}</button>
        </div>
      </div>
    </section>

    <div v-if="showCreate" class="fixed inset-0 z-50 grid place-items-center bg-black/40 p-4">
      <form class="w-full max-w-lg space-y-4 rounded-md bg-white p-5 shadow-xl dark:bg-dark-800" @submit.prevent="createMember">
        <h3 class="font-semibold">{{ t('organization.members.create') }}</h3>
        <div>
          <label class="input-label" for="iam-member-login-name">{{ t('organization.login.loginName') }}</label>
          <div class="flex min-w-0 flex-col sm:flex-row">
            <input id="iam-member-login-name" v-model.trim="createForm.loginName" class="input min-w-0 flex-1 sm:rounded-r-none" required pattern="[A-Za-z0-9._-]{1,64}" autocomplete="off">
            <span data-testid="iam-principal-suffix" class="flex min-h-10 max-w-full items-center break-all rounded-md border border-gray-300 bg-gray-50 px-3 font-mono text-xs text-gray-600 sm:-ml-px sm:rounded-l-none sm:whitespace-nowrap dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300">
              @{{ organization?.account_id }}.opentk.ai
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
        <p class="text-sm text-gray-500">{{ t('organization.allocation.rootAvailable', { amount: companyAmount(finance?.available) }) }}</p>
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
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { organizationAPI, userGroupsAPI } from '@/api'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import GroupOptionItem from '@/components/common/GroupOptionItem.vue'
import { useClipboard } from '@/composables/useClipboard'
import { getLocale } from '@/i18n'
import type { FinanceSummary, Group, GroupPlatform, IAMMember, ManagedPolicy, OrganizationContext, OrganizationSubscription, OrganizationUsageParams, OrganizationUsageStats, OrganizationUsageTrendPoint, PaginatedOrganizationUsage, SubscriptionType } from '@/types'
import { useAuthStore } from '@/stores'

const { t, te } = useI18n()
const auth = useAuthStore()
const { copyToClipboard } = useClipboard()
type Tab = 'allocation' | 'finance' | 'subscriptions' | 'usage'

const activeTab = ref<Tab>('finance')
const organization = ref<OrganizationContext>()
const members = ref<IAMMember[]>([])
const policies = ref<ManagedPolicy[]>([])
const finance = ref<FinanceSummary>()
const usagePage = ref<PaginatedOrganizationUsage>({ items: [], total: 0, page: 1, page_size: 20, pages: 0 })
const usageStats = ref<OrganizationUsageStats | null>(null)
const usageTrend = ref<OrganizationUsageTrendPoint[]>([])
const memberLimit = ref(20)
const usedSlots = ref(0)
const showCreate = ref(false)
const showRename = ref(false)
const requestedName = ref('')
const renameMessage = ref('')
const credential = ref<{ principal: string; password: string } | null>(null)
const allocationTarget = ref<IAMMember | null>(null)
const authorizationTarget = ref<IAMMember | null>(null)
const createForm = reactive({ loginName: '', password: '', mustChangePassword: true, recoveryEmail: '' })
const passwordVisible = ref(false)
const amounts = reactive<Record<number, string>>({})
const usageFilters = reactive({ memberId: '', apiKeyId: '', model: '', endpoint: '', status: '', start: '', end: '' })
const loading = ref(true)
const usageLoading = ref(false)
const operationKey = ref('')
const error = ref('')
const modalError = ref('')
const subscriptions = ref<OrganizationSubscription[]>([])
const availableGroups = ref<Group[]>([])
const subscriptionForm = reactive({ groupID: 0 })

type SubscriptionGroupOption = {
  value: number
  label: string
  description: string | null
  rate: number
  peakRateEnabled: boolean
  peakStart: string
  peakEnd: string
  peakRateMultiplier: number
  subscriptionType: SubscriptionType
  platform: GroupPlatform
}

// 仅保留订阅类型分组（subscription_type === 'subscription'），非订阅分组不应出现在套餐里。
const subscriptionGroupOptions = computed<SubscriptionGroupOption[]>(() =>
  availableGroups.value
    .filter((group) => group.subscription_type === 'subscription')
    .map((group) => ({
      value: group.id,
      label: group.name,
      description: group.description,
      rate: group.rate_multiplier,
      peakRateEnabled: group.peak_rate_enabled,
      peakStart: group.peak_start,
      peakEnd: group.peak_end,
      peakRateMultiplier: group.peak_rate_multiplier,
      subscriptionType: group.subscription_type,
      platform: group.platform,
    })),
)

const isOwner = computed(() => organization.value?.role === 'owner')
const actions = computed(() => organization.value?.actions || [])
const visibleTabs = computed<Tab[]>(() => isOwner.value
  ? ['finance', 'subscriptions', 'allocation', 'usage']
  : (actions.value.includes('organization.finance.balance.read') ? ['finance', 'subscriptions'] : []))
const activeMembers = computed(() => members.value.filter(item => item.status === 'active'))

const tabIcons: Record<Tab, 'creditCard' | 'users' | 'chart' | 'sparkles'> = { finance: 'creditCard', subscriptions: 'sparkles', allocation: 'users', usage: 'chart' }
const tabKeyboardActions: Record<string, number | 'first' | 'last'> = { ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1, Home: 'first', End: 'last' }

function selectTab(tab: Tab) {
  activeTab.value = tab
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
  activeTab.value = nextTab
  focusTab(nextTab)
}

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
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  }).format(amount)
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

async function loadAvailableGroups() {
  try {
    availableGroups.value = await userGroupsAPI.getAvailable()
  } catch {
    availableGroups.value = []
  }
}

async function createSubscription() {
  if (!subscriptionForm.groupID) return
  operationKey.value = 'subscription:create'
  error.value = ''
  try {
    await organizationAPI.createSubscription(subscriptionForm.groupID, 0, '')
    subscriptionForm.groupID = 0
    await loadSubscriptions()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
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
  return positiveAmount(member) > 0 && positiveAmount(member) <= Number(finance.value?.available || 0)
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

function toISO(value: string): string | undefined {
  if (!value) return undefined
  const parsed = new Date(value)
  return Number.isNaN(parsed.getTime()) ? undefined : parsed.toISOString()
}

function organizationUsageParams(page: number): OrganizationUsageParams {
  return {
    page,
    page_size: usagePage.value.page_size || 20,
    member_id: usageFilters.memberId ? Number(usageFilters.memberId) : undefined,
    api_key_id: usageFilters.apiKeyId ? Number(usageFilters.apiKeyId) : undefined,
    model: usageFilters.model || undefined,
    endpoint: usageFilters.endpoint || undefined,
    status: usageFilters.status || undefined,
    start: toISO(usageFilters.start),
    end: toISO(usageFilters.end),
  }
}

const maxTrendTokens = computed(() => Math.max(1, ...usageTrend.value.map(point => point.tokens)))

function trendBarHeight(point: OrganizationUsageTrendPoint): string {
  return `${Math.max(2, Math.round((point.tokens / maxTrendTokens.value) * 100))}%`
}

async function loadUsage(page = 1) {
  if (!isOwner.value) return
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

async function loadUsageAggregates() {
  if (!isOwner.value) return
  try {
    const params = organizationUsageParams(1)
    const [stats, trend] = await Promise.all([
      organizationAPI.getUsageStats(params),
      organizationAPI.getUsageTrend(params),
    ])
    usageStats.value = stats
    usageTrend.value = trend
  } catch (cause) {
    error.value = errorMessage(cause)
  }
}

async function searchUsage() {
  await Promise.all([loadUsage(1), loadUsageAggregates()])
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const context = await organizationAPI.getContext()
    organization.value = context.organization
    finance.value = context.finance
    if (visibleTabs.value.includes('subscriptions')) await loadSubscriptions()
    if (isOwner.value) {
      const [memberData, policyData] = await Promise.all([organizationAPI.listMembers(), organizationAPI.listPolicies()])
      members.value = memberData.items
      memberLimit.value = memberData.member_limit
      usedSlots.value = memberData.used_slots
      policies.value = policyData
      if (!visibleTabs.value.includes(activeTab.value)) activeTab.value = 'finance'
      await Promise.all([loadUsage(usagePage.value.page || 1), loadUsageAggregates(), loadAvailableGroups()])
    } else {
      activeTab.value = 'finance'
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
  const key = `organization.policyMeta.${policy.key}.name`
  return te(key) ? t(key) : policy.display_name
}

function policyDescription(policy: ManagedPolicy): string {
  const key = `organization.policyMeta.${policy.key}.description`
  return te(key) ? t(key) : policy.description
}

function openAuthorization(member: IAMMember) {
  authorizationTarget.value = member
  modalError.value = ''
}

function closeAuthorization() {
  authorizationTarget.value = null
  modalError.value = ''
}

async function transfer(member: IAMMember, operation: 'allocate' | 'reclaim') {
  const amount = amounts[member.user_id]
  if (!amount || (operation === 'allocate' ? !canAllocate(member) : !canReclaim(member))) return
  operationKey.value = `${member.user_id}:balance`
  error.value = ''
  try {
    await organizationAPI.transferBalance(member.user_id, amount, operation)
    amounts[member.user_id] = ''
    await load()
  } catch (cause) {
    error.value = errorMessage(cause)
  } finally {
    operationKey.value = ''
  }
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

onMounted(load)
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
