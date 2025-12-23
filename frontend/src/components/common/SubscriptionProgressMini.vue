<template>
  <div v-if="hasActiveSubscriptions" class="relative" ref="containerRef">
    <!-- Mini Progress Display -->
    <button
      @click="toggleTooltip"
      class="flex items-center gap-2 px-3 py-1.5 rounded-xl bg-purple-50 dark:bg-purple-900/20 hover:bg-purple-100 dark:hover:bg-purple-900/30 transition-colors cursor-pointer"
      :title="t('subscriptionProgress.viewDetails')"
    >
      <svg class="w-4 h-4 text-purple-600 dark:text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
        <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z" />
      </svg>
      <div class="flex items-center gap-1.5">
        <!-- Combined progress indicator -->
        <div class="flex items-center gap-0.5">
          <div
            v-for="(sub, index) in displaySubscriptions.slice(0, 3)"
            :key="index"
            class="w-2 h-2 rounded-full"
            :class="getProgressDotClass(sub)"
          ></div>
        </div>
        <span class="text-xs font-medium text-purple-700 dark:text-purple-300">
          {{ activeSubscriptions.length REDACTEDREDACTED
        </span>
      </div>
    </button>

    <!-- Hover/Click Tooltip -->
    <transition name="dropdown">
      <div
        v-if="tooltipOpen"
        class="absolute right-0 mt-2 w-[340px] bg-white dark:bg-dark-800 rounded-xl shadow-xl border border-gray-200 dark:border-dark-700 z-50 overflow-hidden"
      >
        <div class="p-3 border-b border-gray-100 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('subscriptionProgress.title') REDACTEDREDACTED
          </h3>
          <p class="text-xs text-gray-500 dark:text-dark-400 mt-0.5">
            {{ t('subscriptionProgress.activeCount', { count: activeSubscriptions.length REDACTED) REDACTEDREDACTED
          </p>
        </div>

        <div class="max-h-64 overflow-y-auto">
          <div
            v-for="subscription in displaySubscriptions"
            :key="subscription.id"
            class="p-3 border-b border-gray-50 dark:border-dark-700/50 last:border-b-0"
          >
            <div class="flex items-center justify-between mb-2">
              <span class="text-sm font-medium text-gray-900 dark:text-white">
                {{ subscription.group?.name || `Group #${subscription.group_idREDACTED` REDACTEDREDACTED
              </span>
              <span
                v-if="subscription.expires_at"
                class="text-xs"
                :class="getDaysRemainingClass(subscription.expires_at)"
              >
                {{ formatDaysRemaining(subscription.expires_at) REDACTEDREDACTED
              </span>
            </div>

            <!-- Progress bars -->
            <div class="space-y-1.5">
              <div v-if="subscription.group?.daily_limit_usd" class="flex items-center gap-2">
                <span class="text-[10px] text-gray-500 w-8 flex-shrink-0">{{ t('subscriptionProgress.daily') REDACTEDREDACTED</span>
                <div class="flex-1 min-w-0 bg-gray-200 dark:bg-dark-600 rounded-full h-1.5">
                  <div
                    class="h-1.5 rounded-full transition-all"
                    :class="getProgressBarClass(subscription.daily_usage_usd, subscription.group?.daily_limit_usd)"
                    :style="{ width: getProgressWidth(subscription.daily_usage_usd, subscription.group?.daily_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-[10px] text-gray-500 w-24 text-right flex-shrink-0">
                  {{ formatUsage(subscription.daily_usage_usd, subscription.group?.daily_limit_usd) REDACTEDREDACTED
                </span>
              </div>

              <div v-if="subscription.group?.weekly_limit_usd" class="flex items-center gap-2">
                <span class="text-[10px] text-gray-500 w-8 flex-shrink-0">{{ t('subscriptionProgress.weekly') REDACTEDREDACTED</span>
                <div class="flex-1 min-w-0 bg-gray-200 dark:bg-dark-600 rounded-full h-1.5">
                  <div
                    class="h-1.5 rounded-full transition-all"
                    :class="getProgressBarClass(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd)"
                    :style="{ width: getProgressWidth(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-[10px] text-gray-500 w-24 text-right flex-shrink-0">
                  {{ formatUsage(subscription.weekly_usage_usd, subscription.group?.weekly_limit_usd) REDACTEDREDACTED
                </span>
              </div>

              <div v-if="subscription.group?.monthly_limit_usd" class="flex items-center gap-2">
                <span class="text-[10px] text-gray-500 w-8 flex-shrink-0">{{ t('subscriptionProgress.monthly') REDACTEDREDACTED</span>
                <div class="flex-1 min-w-0 bg-gray-200 dark:bg-dark-600 rounded-full h-1.5">
                  <div
                    class="h-1.5 rounded-full transition-all"
                    :class="getProgressBarClass(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd)"
                    :style="{ width: getProgressWidth(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd) REDACTED"
                  ></div>
                </div>
                <span class="text-[10px] text-gray-500 w-24 text-right flex-shrink-0">
                  {{ formatUsage(subscription.monthly_usage_usd, subscription.group?.monthly_limit_usd) REDACTEDREDACTED
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="p-2 border-t border-gray-100 dark:border-dark-700">
          <router-link
            to="/subscriptions"
            @click="closeTooltip"
            class="block w-full text-center text-xs text-primary-600 dark:text-primary-400 hover:underline py-1"
          >
            {{ t('subscriptionProgress.viewAll') REDACTEDREDACTED
          </router-link>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount REDACTED from 'vue';
import { useI18n REDACTED from 'vue-i18n';
import subscriptionsAPI from '@/api/subscriptions';
import type { UserSubscription REDACTED from '@/types';

const { t REDACTED = useI18n();

const containerRef = ref<HTMLElement | null>(null);
const tooltipOpen = ref(false);
const activeSubscriptions = ref<UserSubscription[]>([]);
const loading = ref(false);

const hasActiveSubscriptions = computed(() => activeSubscriptions.value.length > 0);

const displaySubscriptions = computed(() => {
  // Sort by most usage (highest percentage first)
  return [...activeSubscriptions.value].sort((a, b) => {
    const aMax = getMaxUsagePercentage(a);
    const bMax = getMaxUsagePercentage(b);
    return bMax - aMax;
  REDACTED);
REDACTED);

function getMaxUsagePercentage(sub: UserSubscription): number {
  const percentages: number[] = [];
  if (sub.group?.daily_limit_usd) {
    percentages.push((sub.daily_usage_usd || 0) / sub.group.daily_limit_usd * 100);
  REDACTED
  if (sub.group?.weekly_limit_usd) {
    percentages.push((sub.weekly_usage_usd || 0) / sub.group.weekly_limit_usd * 100);
  REDACTED
  if (sub.group?.monthly_limit_usd) {
    percentages.push((sub.monthly_usage_usd || 0) / sub.group.monthly_limit_usd * 100);
  REDACTED
  return percentages.length > 0 ? Math.max(...percentages) : 0;
REDACTED

function getProgressDotClass(sub: UserSubscription): string {
  const maxPercentage = getMaxUsagePercentage(sub);
  if (maxPercentage >= 90) return 'bg-red-500';
  if (maxPercentage >= 70) return 'bg-orange-500';
  return 'bg-green-500';
REDACTED

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400';
  const percentage = ((used || 0) / limit) * 100;
  if (percentage >= 90) return 'bg-red-500';
  if (percentage >= 70) return 'bg-orange-500';
  return 'bg-green-500';
REDACTED

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%';
  const percentage = Math.min(((used || 0) / limit) * 100, 100);
  return `${percentageREDACTED%`;
REDACTED

function formatUsage(used: number | undefined, limit: number | null | undefined): string {
  const usedValue = (used || 0).toFixed(2);
  const limitValue = limit?.toFixed(2) || '∞';
  return `$${usedValueREDACTED/$${limitValueREDACTED`;
REDACTED

function formatDaysRemaining(expiresAt: string): string {
  const now = new Date();
  const expires = new Date(expiresAt);
  const diff = expires.getTime() - now.getTime();
  if (diff < 0) return t('subscriptionProgress.expired');
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24));
  if (days === 0) return t('subscriptionProgress.expirestoday');
  if (days === 1) return t('subscriptionProgress.expiresTomorrow');
  return t('subscriptionProgress.daysRemaining', { days REDACTED);
REDACTED

function getDaysRemainingClass(expiresAt: string): string {
  const now = new Date();
  const expires = new Date(expiresAt);
  const diff = expires.getTime() - now.getTime();
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24));
  if (days <= 3) return 'text-red-600 dark:text-red-400';
  if (days <= 7) return 'text-orange-600 dark:text-orange-400';
  return 'text-gray-500 dark:text-dark-400';
REDACTED

function toggleTooltip() {
  tooltipOpen.value = !tooltipOpen.value;
REDACTED

function closeTooltip() {
  tooltipOpen.value = false;
REDACTED

function handleClickOutside(event: MouseEvent) {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    closeTooltip();
  REDACTED
REDACTED

async function loadSubscriptions() {
  try {
    loading.value = true;
    activeSubscriptions.value = await subscriptionsAPI.getActiveSubscriptions();
  REDACTED catch (error) {
    console.error('Failed to load subscriptions:', error);
    activeSubscriptions.value = [];
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

onMounted(() => {
  document.addEventListener('click', handleClickOutside);
  loadSubscriptions();
REDACTED);

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside);
REDACTED);

// Refresh subscriptions periodically (every 5 minutes)
let refreshInterval: ReturnType<typeof setInterval> | null = null;
onMounted(() => {
  refreshInterval = setInterval(loadSubscriptions, 5 * 60 * 1000);
REDACTED);
onBeforeUnmount(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval);
  REDACTED
REDACTED);
</script>

<style scoped>
.dropdown-enter-active,
.dropdown-leave-active {
  transition: all 0.2s ease;
REDACTED

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: scale(0.95) translateY(-4px);
REDACTED
</style>
