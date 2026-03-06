<template>
  <Teleport to="body">
    <div v-if="show && position">
      <!-- Backdrop: click anywhere outside to close -->
      <div class="fixed inset-0 z-[9998]" @click="emit('close')"></div>
      <div
        class="action-menu-content fixed z-[9999] w-52 overflow-hidden rounded-xl bg-white shadow-lg ring-1 ring-black/5 dark:bg-dark-800"
        :style="{ top: position.top + 'px', left: position.left + 'px' REDACTED"
        @click.stop
      >
        <div class="py-1">
          <template v-if="account">
            <button @click="$emit('test', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="play" size="sm" class="text-green-500" :stroke-width="2" />
              {{ t('admin.accounts.testConnection') REDACTEDREDACTED
            </button>
            <button @click="$emit('stats', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="chart" size="sm" class="text-indigo-500" />
              {{ t('admin.accounts.viewStats') REDACTEDREDACTED
            </button>
            <button @click="$emit('schedule', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="clock" size="sm" class="text-orange-500" />
              {{ t('admin.scheduledTests.schedule') REDACTEDREDACTED
            </button>
            <template v-if="account.type === 'oauth' || account.type === 'setup-token'">
              <button @click="$emit('reauth', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-blue-600 hover:bg-gray-100 dark:hover:bg-dark-700">
                <Icon name="link" size="sm" />
                {{ t('admin.accounts.reAuthorize') REDACTEDREDACTED
              </button>
              <button @click="$emit('refresh-token', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-purple-600 hover:bg-gray-100 dark:hover:bg-dark-700">
                <Icon name="refresh" size="sm" />
                {{ t('admin.accounts.refreshToken') REDACTEDREDACTED
              </button>
            </template>
            <div v-if="account.status === 'error' || isRateLimited || isOverloaded" class="my-1 border-t border-gray-100 dark:border-dark-700"></div>
            <button v-if="account.status === 'error'" @click="$emit('reset-status', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-yellow-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="sync" size="sm" />
              {{ t('admin.accounts.resetStatus') REDACTEDREDACTED
            </button>
            <button v-if="isRateLimited || isOverloaded" @click="$emit('clear-rate-limit', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-amber-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="clock" size="sm" />
              {{ t('admin.accounts.clearRateLimit') REDACTEDREDACTED
            </button>
            <button v-if="hasQuotaLimit" @click="$emit('reset-quota', account); $emit('close')" class="flex w-full items-center gap-2 px-4 py-2 text-sm text-teal-600 hover:bg-gray-100 dark:hover:bg-dark-700">
              <Icon name="refresh" size="sm" />
              {{ t('admin.accounts.resetQuota') REDACTEDREDACTED
            </button>
          </template>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onUnmounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { Icon REDACTED from '@/components/icons'
import type { Account REDACTED from '@/types'

const props = defineProps<{ show: boolean; account: Account | null; position: { top: number; left: number REDACTED | null REDACTED>()
const emit = defineEmits(['close', 'test', 'stats', 'schedule', 'reauth', 'refresh-token', 'reset-status', 'clear-rate-limit', 'reset-quota'])
const { t REDACTED = useI18n()
const isRateLimited = computed(() => {
  if (props.account?.rate_limit_reset_at && new Date(props.account.rate_limit_reset_at) > new Date()) {
    return true
  REDACTED
  const modelLimits = (props.account?.extra as Record<string, unknown> | undefined)?.model_rate_limits as
    | Record<string, { rate_limit_reset_at: string REDACTED>
    | undefined
  if (modelLimits) {
    const now = new Date()
    return Object.values(modelLimits).some(info => new Date(info.rate_limit_reset_at) > now)
  REDACTED
  return false
REDACTED)
const isOverloaded = computed(() => props.account?.overload_until && new Date(props.account.overload_until) > new Date())
const hasQuotaLimit = computed(() => {
  return props.account?.type === 'apikey' &&
    props.account?.quota_limit !== undefined &&
    props.account?.quota_limit !== null &&
    props.account?.quota_limit > 0
REDACTED)

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
REDACTED

watch(
  () => props.show,
  (visible) => {
    if (visible) {
      window.addEventListener('keydown', handleKeydown)
    REDACTED else {
      window.removeEventListener('keydown', handleKeydown)
    REDACTED
  REDACTED,
  { immediate: true REDACTED
)

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
REDACTED)
</script>
