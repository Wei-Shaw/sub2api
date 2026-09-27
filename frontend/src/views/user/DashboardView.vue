<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- 账单抬头：运营方品牌 + 客户名/编号 | 余额读数（数字方格） | 撕角充值存根 -->
      <section v-if="user" class="flex flex-wrap items-stretch border border-border border-t-2 border-t-accent bg-surface">
        <div class="min-w-0 flex-1 px-5 py-3">
          <p class="flex items-center gap-2 text-meta font-semibold text-accent-strong">
            <img v-if="siteLogo" :src="siteLogo" alt="" class="h-4 w-4 object-contain" />
            <span class="truncate">{{ appStore.siteName }}</span>
          </p>
          <p class="mt-1 truncate text-h2 font-bold text-fg">{{ user.username || user.email }}</p>
          <p class="mt-0.5 flex flex-wrap items-baseline gap-x-4 gap-y-1 text-meta text-fg-muted">
            <span>{{ t('payment.orders.userId') }} <span class="font-mono text-fg">{{ user.id }}</span></span>
            <span v-if="user.username && user.email" class="truncate">{{ user.email }}</span>
          </p>
        </div>
        <div v-if="!authStore.isSimpleMode" class="flex min-w-0 flex-col justify-center gap-1 border-l border-border bg-meter-weak px-5 py-3 max-sm:w-full max-sm:border-l-0 max-sm:border-t">
          <span class="text-meta font-medium text-meter-ink">{{ t('dashboard.balance') }} · {{ t('common.available') }}</span>
          <span class="text-h1 font-bold text-fg">$<MeterValue :value="formatBalance(user.balance || 0)" boxed /></span>
        </div>
        <router-link
          v-if="canTopUp"
          to="/purchase"
          class="stub-edge flex min-h-[44px] items-center gap-2 bg-accent px-6 py-3 text-label font-bold text-white transition-colors hover:bg-accent-strong dark:text-surface-sunken max-sm:w-full max-sm:justify-center"
        >
          {{ t('nav.recharge') }}
          <Icon name="arrowRight" size="sm" aria-hidden="true" />
        </router-link>
      </section>

      <div v-if="showZeroBalanceBanner" role="status" class="flex flex-wrap items-center gap-3 border border-warning/40 bg-warning-weak px-4 py-3 text-body text-warning-strong" data-test="zero-balance-banner">
        <div class="min-w-0 flex-1">
          <p class="font-semibold">{{ t('dashboard.zeroBalanceTitle') }}</p>
          <p>{{ t('dashboard.zeroBalanceDesc') }}</p>
        </div>
        <router-link v-if="canTopUp" to="/purchase" class="btn btn-primary btn-sm">{{ t('nav.recharge') }}</router-link>
        <router-link to="/redeem" class="btn btn-secondary btn-sm">{{ t('nav.redeem') }}</router-link>
      </div>

      <div v-if="loading" class="flex items-center justify-center py-12"><LoadingSpinner /></div>
      <template v-else-if="stats">
        <UserDashboardStats :stats="stats" :balance="user?.balance || 0" :is-simple="authStore.isSimpleMode" :platform-quotas="platformQuotas" />
        <UserDashboardCharts v-model:startDate="startDate" v-model:endDate="endDate" v-model:granularity="granularity" :loading="loadingCharts" :trend="trendData" :models="modelStats" @dateRangeChange="loadCharts" @granularityChange="loadCharts" @refresh="refreshAll" />
        <div class="grid grid-cols-1 gap-6 lg:grid-cols-3">
          <div class="lg:col-span-2"><UserDashboardRecentUsage :data="recentUsage" :loading="loadingUsage" /></div>
          <div class="lg:col-span-1"><UserDashboardQuickActions /></div>
        </div>
      </template>
      <template v-else-if="loadError">
        <EmptyState class="card" :title="t('dashboard.loadFailed')" :description="t('errors.tryAgain')" :action-text="t('common.refresh')" :action-icon="false" @action="refreshAll" />
        <UserDashboardQuickActions />
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'; import { useAuthStore } from '@/stores/auth'; import { usageAPI, type UserDashboardStats as UserStatsType } from '@/api/usage'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'; import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import { extractApiErrorMessage } from '@/utils/apiError'
import Icon from '@/components/icons/Icon.vue'
import MeterValue from '@/components/common/MeterValue.vue'
import { useAppStore } from '@/stores'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { sanitizeUrl } from '@/utils/url'
import UserDashboardStats from '@/components/user/dashboard/UserDashboardStats.vue'; import UserDashboardCharts from '@/components/user/dashboard/UserDashboardCharts.vue'
import UserDashboardRecentUsage from '@/components/user/dashboard/UserDashboardRecentUsage.vue'; import UserDashboardQuickActions from '@/components/user/dashboard/UserDashboardQuickActions.vue'
import type { UsageLog, TrendDataPoint, ModelStat, PlatformQuotaItem } from '@/types'
import { getMyPlatformQuotas } from '@/api/user'
import { formatDateLocalInput } from '@/utils/format'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()
const appStore = useAppStore()
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const formatBalance = (b: number) => new Intl.NumberFormat('en-US', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(b)
const authStore = useAuthStore(); const user = computed(() => authStore.user)
// 充值入口与侧边栏 /purchase 同条件：支付开关 + 非简单模式
const canTopUp = computed(() => !authStore.isSimpleMode && isFeatureFlagEnabled(FeatureFlags.payment))
// 网关在余额 <= 0 时拒绝请求（订阅分组除外）；订阅列表加载完之前不判断，避免订阅用户看到闪现。
const subscriptionStore = useSubscriptionStore()
const showZeroBalanceBanner = computed(() => !authStore.isSimpleMode && !!user.value && (user.value.balance ?? 0) <= 0 && !subscriptionStore.loading && !subscriptionStore.hasActiveSubscriptions)
const stats = ref<UserStatsType | null>(null); const loadError = ref(false); const loading = ref(false); const loadingUsage = ref(false); const loadingCharts = ref(false)
const trendData = ref<TrendDataPoint[]>([]); const modelStats = ref<ModelStat[]>([]); const recentUsage = ref<UsageLog[]>([])
const platformQuotas = ref<PlatformQuotaItem[] | null>(null)

const startDate = ref(formatDateLocalInput(new Date(Date.now() - 6 * 86400000))); const endDate = ref(formatDateLocalInput(new Date())); const granularity = ref('day')

const loadStats = async () => { loading.value = true; loadError.value = false; try { stats.value = await usageAPI.getDashboardStats() } catch (error) { console.error('Failed to load dashboard stats:', error); loadError.value = !stats.value; appStore.showError(extractApiErrorMessage(error, t('dashboard.loadFailed'))) } finally { loading.value = false } }
const loadCharts = async () => { loadingCharts.value = true; try { const res = await Promise.all([usageAPI.getDashboardTrend({ start_date: startDate.value, end_date: endDate.value, granularity: granularity.value as any }), usageAPI.getDashboardModels({ start_date: startDate.value, end_date: endDate.value })]); trendData.value = res[0].trend || []; modelStats.value = res[1].models || [] } catch (error) { console.error('Failed to load charts:', error) } finally { loadingCharts.value = false } }
const loadRecent = async () => { loadingUsage.value = true; try { const res = await usageAPI.query({ start_date: startDate.value, end_date: endDate.value, page: 1, page_size: 5 }); recentUsage.value = res.items } catch (error) { console.error('Failed to load recent usage:', error) } finally { loadingUsage.value = false } }
const loadPlatformQuotas = async () => { try { const data = await getMyPlatformQuotas(); platformQuotas.value = data.platform_quotas ?? [] } catch (error) { console.warn('Failed to load platform quotas:', error); platformQuotas.value = [] } }
// 用户资料刷新与统计请求并行，不再串行等待 /auth/me。
const refreshUser = async () => { try { await authStore.refreshUser() } catch (error) { console.error('Failed to refresh user:', error) } }
const refreshAll = () => { refreshUser(); loadStats(); loadCharts(); loadRecent(); loadPlatformQuotas() }

onMounted(() => { refreshAll() })
</script>
