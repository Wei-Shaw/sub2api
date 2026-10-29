<template>
  <BaseDialog
    :show="show"
    :title="t('admin.quota.title')"
    width="extra-wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <div class="flex flex-wrap items-center gap-3">
        <select v-model="selected" class="input w-auto">
          <option value="all">{{ t('admin.quota.allAccounts') }}</option>
          <option v-for="account in accounts" :key="account.id" :value="String(account.id)">
            {{ account.name }}
          </option>
        </select>
        <button class="btn btn-secondary" :disabled="loading" @click="load(true)">
          {{ loading ? t('admin.quota.loading') : t('admin.quota.refresh') }}
        </button>
        <span class="text-xs text-gray-400">{{ t('admin.quota.cacheHint') }}</span>
      </div>
      <p v-if="error" class="text-sm text-red-500">{{ error }}</p>
      <div v-if="selected === 'all'" class="space-y-3">
        <div v-for="section in totalSections" :key="section.key" class="card p-3 overflow-x-auto">
          <div class="mb-2 text-[11px] text-gray-500">
            {{ section.platform }} ·
            {{ section.tier === 'unknown' ? t('admin.quota.unknownPlan') : section.tier
            }}<span v-if="section.platform === 'gemini'">
              · {{ t('admin.quota.localEstimate') }}</span
            >
          </div>
          <div
            class="grid min-w-[280px] items-start gap-x-4 gap-y-3"
            :style="{
              gridTemplateColumns: `36px repeat(${section.groups.length}, minmax(100px, 1fr))`
            }"
          >
            <span class="text-[10px] text-gray-400" :title="t('admin.quota.legend')">{{
              t('admin.quota.period')
            }}</span>
            <span
              v-for="group in section.groups"
              :key="group.key"
              class="text-right text-sm font-semibold text-gray-900 dark:text-white"
              >{{ group.name }}</span
            >
            <template v-for="id in section.windowIds" :key="id">
              <span class="text-xs text-gray-500">{{ label(id) }}</span>
              <div v-for="group in section.groups" :key="group.key">
                <template v-for="window in [group.windows.find((w) => w.id === id)]" :key="window?.id ?? id">
                  <template v-if="window">
                    <div
                      class="flex flex-wrap items-center justify-end gap-x-2 text-xs tabular-nums"
                    >
                      <span class="whitespace-nowrap font-medium text-gray-800 dark:text-gray-100"
                        >{{ window.known ? window.total.toFixed(2) : '--'
                        }}<span v-if="window.known" class="font-normal text-gray-400">
                          / {{ window.known.toFixed(2) }}</span
                        ></span
                      >
                      <QuotaMeter wide :value="window.known ? window.total / window.known : null" />
                    </div>
                    <div
                      class="mt-1 flex flex-wrap justify-between gap-x-2 text-[10px] text-gray-400"
                    >
                      <span>{{
                        t('admin.quota.included', { known: window.known, total: window.accounts })
                      }}</span>
                      <span
                        :title="
                          window.nextReset
                            ? t('admin.quota.earliestReset', { time: date(window.nextReset) })
                            : t('admin.quota.noReset')
                        "
                        >↻ {{ quotaCountdown(window.nextReset, now.getTime()) }}</span
                      >
                    </div>
                  </template>
                  <span v-else class="block text-right text-xs text-gray-400">--</span>
                </template>
              </div>
            </template>
          </div>
          <p class="mt-2 text-[10px] text-gray-400">{{ t('admin.quota.capacityHint') }}</p>
        </div>
      </div>
      <div
        class="overflow-hidden rounded-xl border border-gray-200 divide-y divide-gray-100 dark:border-dark-700 dark:divide-dark-700"
      >
        <div
          v-for="group in visibleAccounts"
          :key="group.accountId"
          class="grid gap-3 px-4 py-3 md:grid-cols-[minmax(180px,0.7fr)_minmax(0,1.6fr)] md:items-center md:gap-6"
        >
          <div class="min-w-0 text-left">
            <div
              class="truncate text-sm font-medium text-gray-900 dark:text-white"
              :title="group.accountName"
            >
              {{ group.accountName }}
            </div>
            <div v-if="group.email" class="truncate text-xs text-gray-500" :title="group.email">
              {{ group.email }}
            </div>
            <div
              class="mt-0.5 text-[10px] text-gray-400"
              :title="
                [
                  sourceLabel(group.source),
                  group.updatedAt ? t('admin.quota.collected', { time: date(group.updatedAt) }) : ''
                ]
                  .filter(Boolean)
                  .join(' · ')
              "
            >
              {{
                group.updatedAt
                  ? t('admin.quota.collectedShort', { time: time(group.updatedAt) })
                  : t('admin.quota.noReport')
              }}{{ group.eligible ? '' : ' · ' + t('admin.quota.excluded')
              }}{{ group.stale ? ' · ' + t('admin.quota.pending') : '' }}
            </div>
            <p v-if="errors[group.accountId]" class="text-[10px] text-amber-600">
              {{ t('admin.quota.readFailed') }}
            </p>
          </div>
          <QuotaPoolTable
            expanded
            class="min-w-0 w-full justify-self-end"
            :pools="group.pools"
            :updated-at="group.updatedAt"
          />
        </div>
      </div>
      <p v-if="!loading && !accounts.length" class="text-sm text-gray-500">
        {{ t('admin.quota.empty') }}
      </p>
      <details class="text-xs text-gray-500 dark:text-gray-400">
        <summary class="cursor-pointer">{{ t('admin.quota.sources') }}</summary>
        <p class="mt-2 leading-relaxed">{{ t('admin.quota.sourceExplanation') }}</p>
      </details>
    </div>
  </BaseDialog>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useMediaQuery } from '@vueuse/core'
import { useQuotaNow } from '@/composables/useQuotaNow'
import { useAccountUsage } from '@/composables/useAccountUsage'
import { supportsBatchUsage } from '@/utils/accountUsageCache'
import { quotaCountdown } from '@/utils/quotaCountdown'
import BaseDialog from '@/components/common/BaseDialog.vue'
import QuotaPoolTable from '@/components/account/QuotaPoolTable.vue'
import QuotaMeter from '@/components/account/QuotaMeter.vue'
import * as api from '@/api/admin/accounts'
import type { AccountListItem } from '@/types'
import {
  accountPools,
  aggregatePools,
  sortQuotaWindows,
  quotaPoolGroups,
  quotaWindowLabel
} from '@/utils/quotaPools'
const { t } = useI18n()
const sourceLabel = (source: string) =>
  source === 'local-estimate'
    ? t('admin.quota.localEstimate')
    : source === 'quota-snapshot'
      ? t('admin.quota.snapshot')
      : source
const props = defineProps<{ show: boolean }>()
defineEmits<{ close: [] }>()
const accounts = ref<AccountListItem[]>([])
const { usage, errors, loading: usageLoading, load: loadSharedUsage } = useAccountUsage()
const selected = ref('all'),
  listLoading = ref(false),
  error = ref('')
const loading = computed(
  () => listLoading.value || accounts.value.some((account) => usageLoading.value[account.id])
)
const now = useQuotaNow()
const wideScreen = useMediaQuery('(min-width: 768px)')
const rows = computed(() =>
  accounts.value.flatMap((account) =>
    accountPools(account, usage.value[account.id] ?? undefined, now.value.getTime())
  )
)
const totals = computed(() => aggregatePools(rows.value, now.value.getTime()))
// Keep each platform/plan separate; transpose presentation without changing quota aggregation.
const totalSections = computed(() => {
  const sections = new Map<
    string,
    { key: string; platform: string; tier: string; groups: typeof totals.value }
  >()
  for (const group of totals.value) {
    const key = JSON.stringify([group.platform, group.tier])
    if (!sections.has(key))
      sections.set(key, { key, platform: group.platform, tier: group.tier, groups: [] })
    sections.get(key)!.groups.push(group)
  }
  return [...sections.values()]
    .flatMap((section) =>
      quotaPoolGroups(section.groups, wideScreen.value ? section.groups.length : 2).map(
        (groups, index) => ({ ...section, key: section.key + ':' + index, groups })
      )
    )
    .map((section) => ({
      ...section,
      windowIds: sortQuotaWindows(
        [...new Set(section.groups.flatMap((group) => group.windows.map((w) => w.id)))].map(
          (id) => ({ id })
        )
      ).map((w) => w.id)
    }))
})
const visibleAccounts = computed(() =>
  accounts.value
    .filter((account) => selected.value === 'all' || String(account.id) === selected.value)
    .map((account) => {
      const pools = rows.value.filter((row) => row.accountId === account.id)
      const extra = account.extra as Record<string, unknown> | undefined
      const email = String(
        extra?.email_address ||
          extra?.email ||
          account.credentials?.email ||
          account.parent_email ||
          ''
      )
      return {
        accountId: account.id,
        accountName: account.name,
        eligible: account.status === 'active' && account.schedulable !== false,
        updatedAt: pools[0]?.updatedAt ?? usage.value[account.id]?.updated_at,
        source: pools[0]?.source ?? '',
        stale: pools[0]?.stale ?? false,
        pools,
        email
      }
    })
)
const label = quotaWindowLabel
const date = (s: string) => new Date(s).toLocaleString()
const time = (s: string) =>
  new Date(s).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
async function load(force = false) {
  if (listLoading.value) return
  listLoading.value = true
  error.value = ''
  try {
    const all: AccountListItem[] = []
    for (let page = 1; ; page++) {
      const result = await api.list(page, 100)
      all.push(...result.items)
      if (all.length >= result.total || !result.items.length) break
    }
    accounts.value = all
    const queryAccounts = all.filter(supportsBatchUsage)
    await loadSharedUsage(
      queryAccounts.map((account) => account.id),
      force
    )
  } catch {
    error.value = t('admin.quota.loadFailed')
  } finally {
    listLoading.value = false
  }
}
watch(
  () => props.show,
  (v) => {
    if (v) void load()
  }
)
</script>
