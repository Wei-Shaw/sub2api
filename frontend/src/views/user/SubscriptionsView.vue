<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <div class="animate-spin rounded-full h-8 w-8 border-2 border-primary-500 border-t-transparent"></div>
      </div>

      <!-- Empty State -->
      <div v-else-if="subscriptions.length === 0" class="card p-12 text-center">
        <div class="w-16 h-16 mx-auto mb-4 rounded-full bg-gray-100 dark:bg-dark-700 flex items-center justify-center">
          <svg class="w-8 h-8 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z" />
          </svg>
        </div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white mb-2">
          {{ t('userSubscriptions.noActiveSubscriptions') REDACTEDREDACTED
        </h3>
        <p class="text-gray-500 dark:text-dark-400">
          {{ t('userSubscriptions.noActiveSubscriptionsDesc') REDACTEDREDACTED
        </p>
      </div>

      <!-- Subscriptions Grid -->
      <div v-else class="grid gap-6 lg:grid-cols-2">
        <div
          v-for="subscription in subscriptions"
          :key="subscription.id"
          class="card overflow-hidden"
        >
          <!-- Header -->
          <div class="flex items-center justify-between p-4 border-b border-gray-100 dark:border-dark-700">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 rounded-xl bg-purple-100 dark:bg-purple-900/30 flex items-center justify-center">
                <svg class="w-5 h-5 text-purple-600 dark:text-purple-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M2.25 8.25h19.5M2.25 9h19.5m-16.5 5.25h6m-6 2.25h3m-3.75 3h15a2.25 2.25 0 002.25-2.25V6.75A2.25 2.25 0 0019.5 4.5h-15a2.25 2.25 0 00-2.25 2.25v10.5A2.25 2.25 0 004.5 19.5z" />
                </svg>
              </div>
              <div>
                <h3 class="font-semibold text-gray-900 dark:text-white">
                  {{ subscription.group?.name || `Group #${subscription.group_idREDACTED` REDACTEDREDACTED
                </h3>
                <p class="text-xs text-gray-500 dark:text-dark-400">
                  {{ subscription.group?.description || '' REDACTEDREDACTED
                </p>
              </div>
            </div>
            <span
              :class="[
                'badge',
                subscription.status === 'active' ? 'badge-success' :
                subscription.status === 'expired' ? 'badge-warning' : 'badge-danger'
              ]"
            >
              {{ t(`userSubscriptions.status.${subscription.statusREDACTED`) REDACTEDREDACTED
            </span>
          </div>

          <!-- Usage Progress -->
          <div class="p-4 space-y-4">
            <!-- Expiration Info -->
            <div v-if="subscription.expires_at" class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.expires') REDACTEDREDACTED</span>
              <span :class="getExpirationClass(subscription.expires_at)">
                {{ formatExpirationDate(subscription.expires_at) REDACTEDREDACTED
              </span>
            </div>
            <div v-else class="flex items-center justify-between text-sm">
              <span class="text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.expires') REDACTEDREDACTED</span>
              <span class="text-gray-700 dark:text-gray-300">{{ t('userSubscriptions.noExpiration') REDACTEDREDACTED</span>
            </div>

            <!-- Daily Usage -->
            <div v-if="subscription.group?.daily_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.daily') REDACTEDREDACTED
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.daily_usage_usd || 0).toFixed(2) REDACTEDREDACTED / ${{ subscription.group.daily_limit_usd.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div class="relative h-2 bg-gray-200 dark:bg-dark-600 rounded-full overflow-hidden">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="getProgressBarClass(subscription.daily_usage_usd, subscription.group.daily_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.daily_usage_usd, subscription.group.daily_limit_usd) REDACTED"
                ></div>
              </div>
              <p v-if="subscription.daily_window_start" class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.resetIn', { time: formatResetTime(subscription.daily_window_start, 24) REDACTED) REDACTEDREDACTED
              </p>
            </div>

            <!-- Weekly Usage -->
            <div v-if="subscription.group?.weekly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.weekly') REDACTEDREDACTED
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.weekly_usage_usd || 0).toFixed(2) REDACTEDREDACTED / ${{ subscription.group.weekly_limit_usd.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div class="relative h-2 bg-gray-200 dark:bg-dark-600 rounded-full overflow-hidden">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="getProgressBarClass(subscription.weekly_usage_usd, subscription.group.weekly_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.weekly_usage_usd, subscription.group.weekly_limit_usd) REDACTED"
                ></div>
              </div>
              <p v-if="subscription.weekly_window_start" class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.resetIn', { time: formatResetTime(subscription.weekly_window_start, 168) REDACTED) REDACTEDREDACTED
              </p>
            </div>

            <!-- Monthly Usage -->
            <div v-if="subscription.group?.monthly_limit_usd" class="space-y-2">
              <div class="flex items-center justify-between">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
                  {{ t('userSubscriptions.monthly') REDACTEDREDACTED
                </span>
                <span class="text-sm text-gray-500 dark:text-dark-400">
                  ${{ (subscription.monthly_usage_usd || 0).toFixed(2) REDACTEDREDACTED / ${{ subscription.group.monthly_limit_usd.toFixed(2) REDACTEDREDACTED
                </span>
              </div>
              <div class="relative h-2 bg-gray-200 dark:bg-dark-600 rounded-full overflow-hidden">
                <div
                  class="absolute inset-y-0 left-0 rounded-full transition-all duration-300"
                  :class="getProgressBarClass(subscription.monthly_usage_usd, subscription.group.monthly_limit_usd)"
                  :style="{ width: getProgressWidth(subscription.monthly_usage_usd, subscription.group.monthly_limit_usd) REDACTED"
                ></div>
              </div>
              <p v-if="subscription.monthly_window_start" class="text-xs text-gray-500 dark:text-dark-400">
                {{ t('userSubscriptions.resetIn', { time: formatResetTime(subscription.monthly_window_start, 720) REDACTED) REDACTEDREDACTED
              </p>
            </div>

            <!-- No limits configured -->
            <div
              v-if="!subscription.group?.daily_limit_usd && !subscription.group?.weekly_limit_usd && !subscription.group?.monthly_limit_usd"
              class="text-center py-4"
            >
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ t('userSubscriptions.unlimited') REDACTEDREDACTED</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, onMounted REDACTED from 'vue';
import { useI18n REDACTED from 'vue-i18n';
import { useAppStore REDACTED from '@/stores/app';
import subscriptionsAPI from '@/api/subscriptions';
import type { UserSubscription REDACTED from '@/types';
import AppLayout from '@/components/layout/AppLayout.vue';

const { t REDACTED = useI18n();
const appStore = useAppStore();

const subscriptions = ref<UserSubscription[]>([]);
const loading = ref(true);

async function loadSubscriptions() {
  try {
    loading.value = true;
    subscriptions.value = await subscriptionsAPI.getMySubscriptions();
  REDACTED catch (error) {
    console.error('Failed to load subscriptions:', error);
    appStore.showError('Failed to load subscriptions');
  REDACTED finally {
    loading.value = false;
  REDACTED
REDACTED

function getProgressWidth(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return '0%';
  const percentage = Math.min(((used || 0) / limit) * 100, 100);
  return `${percentageREDACTED%`;
REDACTED

function getProgressBarClass(used: number | undefined, limit: number | null | undefined): string {
  if (!limit || limit === 0) return 'bg-gray-400';
  const percentage = ((used || 0) / limit) * 100;
  if (percentage >= 90) return 'bg-red-500';
  if (percentage >= 70) return 'bg-orange-500';
  return 'bg-green-500';
REDACTED

function formatExpirationDate(expiresAt: string): string {
  const now = new Date();
  const expires = new Date(expiresAt);
  const diff = expires.getTime() - now.getTime();
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24));

  if (days < 0) {
    return t('userSubscriptions.status.expired');
  REDACTED

  const dateStr = expires.toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  REDACTED);

  if (days === 0) {
    return `${dateStrREDACTED (Today)`;
  REDACTED
  if (days === 1) {
    return `${dateStrREDACTED (Tomorrow)`;
  REDACTED

  return t('userSubscriptions.daysRemaining', { days REDACTED) + ` (${dateStrREDACTED)`;
REDACTED

function getExpirationClass(expiresAt: string): string {
  const now = new Date();
  const expires = new Date(expiresAt);
  const diff = expires.getTime() - now.getTime();
  const days = Math.ceil(diff / (1000 * 60 * 60 * 24));

  if (days <= 0) return 'text-red-600 dark:text-red-400 font-medium';
  if (days <= 3) return 'text-red-600 dark:text-red-400';
  if (days <= 7) return 'text-orange-600 dark:text-orange-400';
  return 'text-gray-700 dark:text-gray-300';
REDACTED

function formatResetTime(windowStart: string | null, windowHours: number): string {
  if (!windowStart) return t('userSubscriptions.windowNotActive');

  const start = new Date(windowStart);
  const end = new Date(start.getTime() + windowHours * 60 * 60 * 1000);
  const now = new Date();
  const diff = end.getTime() - now.getTime();

  if (diff <= 0) return t('userSubscriptions.windowNotActive');

  const hours = Math.floor(diff / (1000 * 60 * 60));
  const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

  if (hours > 24) {
    const days = Math.floor(hours / 24);
    const remainingHours = hours % 24;
    return `${daysREDACTEDd ${remainingHoursREDACTEDh`;
  REDACTED

  if (hours > 0) {
    return `${hoursREDACTEDh ${minutesREDACTEDm`;
  REDACTED

  return `${minutesREDACTEDm`;
REDACTED

onMounted(() => {
  loadSubscriptions();
REDACTED);
</script>
