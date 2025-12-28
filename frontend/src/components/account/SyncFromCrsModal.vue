<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.syncFromCrsTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="sync-from-crs-form" class="space-y-4" @submit.prevent="handleSync">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.accounts.syncFromCrsDesc') REDACTEDREDACTED
      </div>
      <div
        class="rounded-lg bg-gray-50 p-3 text-xs text-gray-500 dark:bg-dark-700/60 dark:text-dark-400"
      >
        已有账号仅同步 CRS
        返回的字段，缺失字段保持原值；凭据按键合并，不会清空未下发的键；未勾选"同步代理"时保留原有代理。
      </div>
      <div
        class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.accounts.crsVersionRequirement') REDACTEDREDACTED
      </div>

      <div class="grid grid-cols-1 gap-4">
        <div>
          <label class="input-label">{{ t('admin.accounts.crsBaseUrl') REDACTEDREDACTED</label>
          <input
            v-model="form.base_url"
            type="text"
            class="input"
            :placeholder="t('admin.accounts.crsBaseUrlPlaceholder')"
          />
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.accounts.crsUsername') REDACTEDREDACTED</label>
            <input v-model="form.username" type="text" class="input" autocomplete="username" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.accounts.crsPassword') REDACTEDREDACTED</label>
            <input
              v-model="form.password"
              type="password"
              class="input"
              autocomplete="current-password"
            />
          </div>
        </div>

        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-dark-300">
          <input
            v-model="form.sync_proxies"
            type="checkbox"
            class="rounded border-gray-300 dark:border-dark-600"
          />
          {{ t('admin.accounts.syncProxies') REDACTEDREDACTED
        </label>
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-xl border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.accounts.syncResult') REDACTEDREDACTED
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.accounts.syncResultSummary', result) REDACTEDREDACTED
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.accounts.syncErrors') REDACTEDREDACTED
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-lg bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind REDACTEDREDACTED {{ item.crs_account_id REDACTEDREDACTED — {{ item.action
              REDACTEDREDACTED{{ item.error ? `: ${item.errorREDACTED` : '' REDACTEDREDACTED
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="syncing" @click="handleClose">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="sync-from-crs-form"
          :disabled="syncing"
        >
          {{ syncing ? t('admin.accounts.syncing') : t('admin.accounts.syncNow') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'

interface Props {
  show: boolean
REDACTED

interface Emits {
  (e: 'close'): void
  (e: 'synced'): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const syncing = ref(false)
const result = ref<Awaited<ReturnType<typeof adminAPI.accounts.syncFromCrs>> | null>(null)

const form = reactive({
  base_url: '',
  username: '',
  password: '',
  sync_proxies: true
REDACTED)

const errorItems = computed(() => {
  if (!result.value?.items) return []
  return result.value.items.filter((i) => i.action === 'failed' || i.action === 'skipped')
REDACTED)

watch(
  () => props.show,
  (open) => {
    if (open) {
      result.value = null
    REDACTED
  REDACTED
)

const handleClose = () => {
  emit('close')
REDACTED

const handleSync = async () => {
  if (!form.base_url.trim() || !form.username.trim() || !form.password.trim()) {
    appStore.showError(t('admin.accounts.syncMissingFields'))
    return
  REDACTED

  syncing.value = true
  try {
    const res = await adminAPI.accounts.syncFromCrs({
      base_url: form.base_url.trim(),
      username: form.username.trim(),
      password: form.password,
      sync_proxies: form.sync_proxies
    REDACTED)
    result.value = res

    if (res.failed > 0) {
      appStore.showError(t('admin.accounts.syncCompletedWithErrors', res))
    REDACTED else {
      appStore.showSuccess(t('admin.accounts.syncCompleted', res))
      emit('synced')
    REDACTED
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.syncFailed'))
  REDACTED finally {
    syncing.value = false
  REDACTED
REDACTED
</script>
