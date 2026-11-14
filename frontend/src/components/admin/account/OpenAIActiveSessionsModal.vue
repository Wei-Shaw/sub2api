<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.activeSessions.title')"
    width="wide"
    :show-close-button="!busy"
    :close-on-escape="!busy"
    @close="emit('close')"
  >
    <div class="space-y-4">
      <div class="flex items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            <p class="break-all font-medium text-gray-900 dark:text-gray-100">{{ account?.name }}</p>
            <span v-if="snapshot" class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
              {{ t('admin.accounts.activeSessions.sessionCount', { count: snapshot.sessions.length }) }}
            </span>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.accounts.activeSessions.description') }}</p>
        </div>
        <button class="btn btn-secondary shrink-0" :disabled="loading || busy" @click="loadSessions">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          {{ t('common.refresh') }}
        </button>
      </div>

      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">{{ error }}</p>
      <p v-if="resultMessage" role="status" class="rounded-lg bg-primary-50 p-3 text-sm text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ resultMessage }}</p>
      <p v-if="snapshot && !snapshot.current_session_identified && snapshot.sessions.length" class="rounded-lg bg-amber-50 p-3 text-sm text-amber-800 dark:bg-amber-900/20 dark:text-amber-300">
        {{ t('admin.accounts.activeSessions.currentUnknown') }}
      </p>

      <div v-if="loading && !snapshot" class="py-10 text-center text-sm text-gray-500" role="status">{{ t('common.loading') }}</div>
      <div v-else-if="snapshot?.sessions.length" class="divide-y divide-gray-200 overflow-hidden rounded-xl border border-gray-200 dark:divide-dark-600 dark:border-dark-600">
        <div v-for="session in sortedSessions" :key="session.session_id" class="flex flex-wrap items-center justify-between gap-3 p-4">
          <div class="flex min-w-0 flex-1 items-start gap-3">
            <input
              v-if="isSelectable(session)"
              type="checkbox"
              class="mt-1 h-4 w-4 shrink-0 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              :checked="isSelected(session.session_id)"
              :disabled="busy || loading"
              :aria-label="t('admin.accounts.activeSessions.selectSession', { name: sessionName(session) })"
              @change="toggleSessionSelection(session.session_id)"
            />
            <span v-else class="mt-1 h-4 w-4 shrink-0" aria-hidden="true"></span>
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="break-words font-medium text-gray-900 dark:text-gray-100">{{ sessionName(session) }}</span>
                <span v-if="session.is_current" class="rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300">
                  {{ t('admin.accounts.activeSessions.current') }}
                </span>
                <span v-else-if="pendingIDs.has(session.session_id)" class="rounded-full bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">
                  {{ t('admin.accounts.activeSessions.pending') }}
                </span>
              </div>
              <p class="mt-1 break-words text-sm text-gray-500 dark:text-gray-400">{{ deviceDetails(session) }}</p>
              <p class="mt-1 text-xs text-gray-400">{{ t('admin.accounts.activeSessions.lastSignedIn', { time: signedInTime(session) }) }}</p>
            </div>
          </div>
          <button
            class="btn btn-secondary shrink-0 text-sm"
            :disabled="!listReady || !session.can_revoke || pendingIDs.has(session.session_id) || busy || loading"
            :title="session.is_current ? t('admin.accounts.activeSessions.currentProtected') : undefined"
            @click="confirmTarget = session"
          >
            {{ t('admin.accounts.activeSessions.logout') }}
          </button>
        </div>
      </div>
      <p v-else-if="snapshot" class="py-8 text-center text-sm text-gray-500">{{ t('admin.accounts.activeSessions.empty') }}</p>

      <div class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-gray-200 p-4 dark:border-dark-600">
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-x-3 gap-y-2">
            <p class="text-sm font-medium text-gray-800 dark:text-gray-100">{{ t('admin.accounts.activeSessions.logoutSelectedTitle') }}</p>
            <label v-if="selectableSessions.length" class="flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
              <input
                type="checkbox"
                class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
                :checked="allSelectableSelected"
                :disabled="busy || loading"
                :aria-label="t('admin.accounts.activeSessions.selectAll')"
                @change="toggleSelectAll"
              />
              {{ t('admin.accounts.activeSessions.selectAll') }}
            </label>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.activeSessions.logoutSelectedHint') }}</p>
        </div>
        <button class="btn btn-danger shrink-0" :disabled="!listReady || !selectedSessions.length || busy || loading" @click="confirmSelected = true">
          {{ t('admin.accounts.activeSessions.logoutSelected', { count: selectedSessions.length }) }}
        </button>
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.activeSessions.delayHint') }}</p>
      <p v-if="busy" role="status" class="text-sm text-primary-600 dark:text-primary-400">
        {{ t('admin.accounts.activeSessions.progress', { done: completedCount, total: totalCount }) }}
      </p>
    </div>
    <template #footer>
      <button v-if="busy && totalCount > 1" class="btn btn-secondary" :disabled="stopRequested" @click="stopRequested = true">
        {{ t('admin.accounts.activeSessions.stop') }}
      </button>
      <button v-else class="btn btn-secondary" :disabled="busy" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
  <ConfirmDialog
    :show="Boolean(confirmTarget) || confirmSelected"
    :title="t('admin.accounts.activeSessions.confirmTitle')"
    :message="confirmationMessage"
    :confirm-text="t('admin.accounts.activeSessions.logout')"
    :danger="true"
    @cancel="clearConfirmation"
    @confirm="confirmLogout"
  />
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { getOpenAIActiveSessions, revokeOpenAIActiveSession } from '@/api/admin/accounts'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatDateTime } from '@/utils/format'
import type { Account } from '@/types'
import type { OpenAIActiveSession, OpenAIActiveSessions } from '@/types/openaiSessions'

const props = defineProps<{ show: boolean; account: Pick<Account, 'id' | 'name'> | null }>()
const emit = defineEmits<{ (e: 'close'): void }>()
const { t } = useI18n()
const snapshot = ref<OpenAIActiveSessions | null>(null)
const loading = ref(false)
const listReady = ref(false)
const busy = ref(false)
const error = ref('')
const resultMessage = ref('')
const pendingIDs = ref(new Set<string>())
const confirmTarget = ref<OpenAIActiveSession | null>(null)
const confirmSelected = ref(false)
const selectedSessionIDs = ref(new Set<string>())
const stopRequested = ref(false)
const completedCount = ref(0)
const totalCount = ref(0)
let generation = 0
let loadController: AbortController | null = null

const sortedSessions = computed(() => [...(snapshot.value?.sessions ?? [])].sort((a, b) => Number(b.is_current) - Number(a.is_current)))
const selectableSessions = computed(() => snapshot.value?.current_session_identified
  ? sortedSessions.value.filter(session => session.can_revoke && !session.is_current && !pendingIDs.value.has(session.session_id))
  : [])
const selectedSessions = computed(() => selectableSessions.value.filter(session => selectedSessionIDs.value.has(session.session_id)))
const allSelectableSelected = computed(() => selectableSessions.value.length > 0 && selectableSessions.value.every(session => selectedSessionIDs.value.has(session.session_id)))
const confirmationMessage = computed(() => confirmSelected.value
  ? t('admin.accounts.activeSessions.confirmSelected', { count: selectedSessions.value.length })
  : t('admin.accounts.activeSessions.confirmOne', { name: confirmTarget.value ? sessionName(confirmTarget.value) : '' }))

function isSelectable(session: OpenAIActiveSession): boolean {
  return selectableSessions.value.some(item => item.session_id === session.session_id)
}

function isSelected(sessionID: string): boolean {
  return selectedSessionIDs.value.has(sessionID)
}

function toggleSessionSelection(sessionID: string) {
  if (!selectableSessions.value.some(session => session.session_id === sessionID)) return
  const next = new Set(selectedSessionIDs.value)
  if (next.has(sessionID)) next.delete(sessionID)
  else next.add(sessionID)
  selectedSessionIDs.value = next
}

function toggleSelectAll() {
  selectedSessionIDs.value = allSelectableSelected.value
    ? new Set()
    : new Set(selectableSessions.value.map(session => session.session_id))
}

function pruneSelection(sessions: OpenAIActiveSession[]) {
  const validIDs = new Set(sessions
    .filter(session => session.can_revoke && !session.is_current && !pendingIDs.value.has(session.session_id))
    .map(session => session.session_id))
  selectedSessionIDs.value = new Set([...selectedSessionIDs.value].filter(sessionID => validIDs.has(sessionID)))
}

function sessionName(session: OpenAIActiveSession): string {
  return session.applications.join(' / ') || session.device_name || t('admin.accounts.activeSessions.unknownDevice')
}

function deviceDetails(session: OpenAIActiveSession): string {
  const device = session.description || [session.device_name, session.platform, session.os_version].filter(Boolean).join(' · ')
  const location = [session.city, session.region, session.country].filter(Boolean).join(', ')
  return [device, location].filter(Boolean).join(' · ')
}

function signedInTime(session: OpenAIActiveSession): string {
  if (!session.last_signed_in) return t('admin.accounts.activeSessions.unknownTime')
  const date = new Date(session.last_signed_in * 1000)
  return Number.isNaN(date.getTime()) ? t('admin.accounts.activeSessions.unknownTime') : formatDateTime(date)
}

function clearConfirmation() {
  confirmTarget.value = null
  confirmSelected.value = false
}

async function loadSessions() {
  if (!props.account || !props.show) return
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  const currentGeneration = generation
  loading.value = true
  listReady.value = false
  if (!busy.value) error.value = ''
  try {
    const result = await getOpenAIActiveSessions(props.account.id, controller.signal)
    if (currentGeneration !== generation || controller.signal.aborted) return
    snapshot.value = result
    listReady.value = true
    // If upstream now identifies a pending entry as current, show its protection.
    for (const session of result.sessions) {
      if (session.is_current) pendingIDs.value.delete(session.session_id)
    }
    pruneSelection(result.sessions)
  } catch (err) {
    if (currentGeneration !== generation || controller.signal.aborted) return
    error.value = extractApiErrorMessage(err, t('admin.accounts.activeSessions.loadFailed'))
  } finally {
    if (currentGeneration === generation && !controller.signal.aborted) loading.value = false
  }
}

async function confirmLogout() {
  if (busy.value || !listReady.value || !props.account) return
  const targets = confirmSelected.value
    ? [...selectedSessions.value]
    : selectableSessions.value.filter(s => s.session_id === confirmTarget.value?.session_id)
  clearConfirmation()
  if (!targets.length) return
  const accountID = props.account.id
  const currentGeneration = generation
  busy.value = true
  stopRequested.value = false
  error.value = ''
  resultMessage.value = ''
  totalCount.value = targets.length
  completedCount.value = 0
  try {
    // Each call rechecks the current session on the server. Stop at the first
    // failure so an auth/current-session change cannot trigger further logout.
    for (const session of targets) {
      if (stopRequested.value || currentGeneration !== generation) break
      const result = await revokeOpenAIActiveSession(accountID, session.session_id)
      if (currentGeneration !== generation) return
      if (!result.revoked) throw new Error(t('admin.accounts.activeSessions.logoutFailed'))
      pendingIDs.value.add(session.session_id)
      selectedSessionIDs.value = new Set([...selectedSessionIDs.value].filter(id => id !== session.session_id))
      completedCount.value += 1
    }
  } catch (err) {
    if (currentGeneration !== generation) return
    error.value = extractApiErrorMessage(err, t('admin.accounts.activeSessions.logoutFailed'))
  } finally {
    if (currentGeneration === generation) {
      resultMessage.value = t('admin.accounts.activeSessions.result', { done: completedCount.value, remaining: totalCount.value - completedCount.value })
      await loadSessions()
      if (currentGeneration === generation) busy.value = false
    }
  }
}

watch([() => props.show, () => props.account?.id], () => {
  generation += 1
  loadController?.abort()
  snapshot.value = null
  listReady.value = false
  loading.value = false
  busy.value = false
  error.value = ''
  resultMessage.value = ''
  pendingIDs.value = new Set()
  selectedSessionIDs.value = new Set()
  clearConfirmation()
  if (props.show && props.account) void loadSessions()
}, { immediate: true })

onUnmounted(() => {
  generation += 1
  loadController?.abort()
})
</script>
