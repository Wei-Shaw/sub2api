<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else-if="detail">
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          <div class="card p-5">
            <p class="flex items-center gap-1.5 text-sm text-gray-500 dark:text-dark-400">
              <Icon name="dollar" size="sm" class="text-primary-500" />
              {{ t('affiliate.stats.rebateRate') }}
            </p>
            <p class="mt-2 text-2xl font-semibold text-primary-600 dark:text-primary-400">
              {{ formattedRebateRate }}<span class="ml-0.5 text-base font-medium">%</span>
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
              {{ t('affiliate.stats.rebateRateHint') }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.invitedUsers') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ formatCount(detail.aff_count) }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.availableQuota') }}</p>
            <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400">
              {{ formatCurrency(detail.aff_quota) }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.stats.totalQuota') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
              {{ formatCurrency(detail.aff_history_quota) }}
            </p>
            <p v-if="detail.aff_frozen_quota > 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
              {{ t('affiliate.stats.frozenQuota') }}: {{ formatCurrency(detail.aff_frozen_quota) }}
            </p>
          </div>
        </div>

        <div class="card p-6">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('affiliate.title') }}</h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.description') }}</p>

          <div class="mt-5 grid gap-4 md:grid-cols-2">
            <div class="space-y-2">
              <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('affiliate.yourCode') }}</p>
              <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900">
                <code class="flex-1 truncate text-sm font-semibold text-gray-900 dark:text-white">{{ detail.aff_code }}</code>
                <button class="btn btn-secondary btn-sm" @click="copyCode">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyCode') }}</span>
                </button>
              </div>
            </div>

            <div class="space-y-2">
              <p class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('affiliate.inviteLink') }}</p>
              <div class="flex items-center gap-2 rounded-xl border border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-700 dark:bg-dark-900">
                <code class="flex-1 truncate text-sm text-gray-700 dark:text-gray-300">{{ inviteLink }}</code>
                <button class="btn btn-secondary btn-sm" @click="copyInviteLink">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyLink') }}</span>
                </button>
              </div>
            </div>
          </div>

          <div class="mt-5 rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-900/40 dark:bg-primary-900/20">
            <p class="text-sm font-medium text-primary-800 dark:text-primary-200">{{ t('affiliate.tips.title') }}</p>
            <ul class="mt-2 space-y-1 text-sm text-primary-700 dark:text-primary-300">
              <li>1. {{ t('affiliate.tips.line1') }}</li>
              <li>2. {{ t('affiliate.tips.line2', { rate: `${formattedRebateRate}%` }) }}</li>
              <li>3. {{ t('affiliate.tips.line3') }}</li>
              <li v-if="detail.aff_frozen_quota > 0">4. {{ t('affiliate.tips.line4') }}</li>
            </ul>
          </div>
        </div>

        <div class="card p-6">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('affiliate.transfer.title') }}</h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('affiliate.transfer.description') }}</p>
            </div>
            <button
              class="btn btn-primary"
              :disabled="transferring || detail.aff_quota <= 0"
              @click="transferQuota"
            >
              <Icon v-if="transferring" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="dollar" size="sm" />
              <span>{{ transferring ? t('affiliate.transfer.transferring') : t('affiliate.transfer.button') }}</span>
            </button>
          </div>
          <p v-if="detail.aff_quota <= 0" class="mt-3 text-sm text-amber-600 dark:text-amber-400">
            {{ t('affiliate.transfer.empty') }}
          </p>
        </div>

        <div class="card p-0 overflow-hidden">
          <div class="p-6 pb-4">
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('affiliate.invitees.title') }}</h3>
          </div>
          <div
            v-if="inviteesLoading && invitees.length === 0"
            class="flex justify-center py-10"
          >
            <div
              class="h-6 w-6 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
            ></div>
          </div>
          <div
            v-else-if="inviteesTotal === 0"
            class="mx-6 mb-6 rounded-xl border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
          >
            {{ t('affiliate.invitees.empty') }}
          </div>
          <template v-else>
            <div class="overflow-x-auto px-6">
              <table class="w-full min-w-[720px] text-left text-sm">
                <thead>
                  <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-dark-400">
                    <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.email') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.username') }}</th>
                    <th class="px-3 py-2 font-medium text-right">{{ t('affiliate.invitees.columns.rebate') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.joinedAt') }}</th>
                    <th class="px-3 py-2 font-medium">{{ t('affiliate.invitees.columns.note') }}</th>
                    <th class="px-3 py-2 font-medium text-right">{{ t('affiliate.invitees.columns.actions') }}</th>
                  </tr>
                </thead>
                <tbody :class="{ 'opacity-60': inviteesLoading }">
                  <tr
                    v-for="item in displayInvitees"
                    :key="item.user_id"
                    class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
                    :class="{ 'bg-red-50/60 dark:bg-red-900/10': hasNewRecharge(item.user_id) }"
                  >
                    <td class="px-3 py-3 text-gray-900 dark:text-white">
                      <span class="inline-flex items-center gap-1">
                        <Icon
                          v-if="hasNewRecharge(item.user_id)"
                          name="arrowUp"
                          size="sm"
                          class="animate-bounce text-red-500 dark:text-red-400"
                          :title="t('affiliate.invitees.newRecharge')"
                        />
                        <span>{{ item.email || '-' }}</span>
                      </span>
                    </td>
                    <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ item.username || '-' }}</td>
                    <td class="px-3 py-3 text-right font-medium text-emerald-600 dark:text-emerald-400">{{ formatCurrency(item.total_rebate) }}</td>
                    <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ formatDateTime(item.created_at) || '-' }}</td>
                    <td class="px-3 py-3 max-w-[220px]">
                      <span
                        v-if="item.inviter_note"
                        :title="item.inviter_note"
                        class="block truncate text-gray-700 dark:text-gray-300"
                      >{{ item.inviter_note }}</span>
                      <span v-else class="text-gray-400 dark:text-dark-500">{{ t('affiliate.invitees.noteEmpty') }}</span>
                    </td>
                    <td class="px-3 py-3 text-right">
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm"
                        @click="openNoteEditor(item)"
                      >
                        <Icon name="edit" size="sm" />
                        <span>{{ item.inviter_note ? t('affiliate.invitees.noteEdit') : t('affiliate.invitees.noteAdd') }}</span>
                      </button>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
            <Pagination
              :total="inviteesTotal"
              :page="inviteesPage"
              :page-size="inviteesPageSize"
              @update:page="handleInviteesPageChange"
              @update:pageSize="handleInviteesPageSizeChange"
            />
          </template>
        </div>
      </template>
    </div>

    <BaseDialog
      :show="!!editingInvitee"
      :title="t('affiliate.invitees.noteModal.title')"
      width="narrow"
      @close="closeNoteEditor"
    >
      <div v-if="editingInvitee" class="space-y-3">
        <div class="rounded-xl bg-gray-50 p-3 text-sm dark:bg-dark-800">
          <div class="flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('affiliate.invitees.columns.email') }}</span>
            <span class="text-gray-900 dark:text-white">{{ editingInvitee.email || '-' }}</span>
          </div>
          <div class="mt-1 flex justify-between">
            <span class="text-gray-500 dark:text-gray-400">{{ t('affiliate.invitees.columns.username') }}</span>
            <span class="text-gray-700 dark:text-gray-300">{{ editingInvitee.username || '-' }}</span>
          </div>
        </div>
        <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('affiliate.invitees.noteModal.subtitle') }}</p>
        <textarea
          v-model="noteDraft"
          rows="4"
          maxlength="500"
          class="input w-full resize-none"
          :placeholder="t('affiliate.invitees.noteModal.placeholder')"
        ></textarea>
        <div class="flex items-center justify-between text-xs text-gray-400 dark:text-dark-500">
          <span>{{ t('affiliate.invitees.noteModal.counter', { count: noteDraftLength }) }}</span>
        </div>
      </div>
      <template #footer>
        <div class="flex items-center justify-between gap-3">
          <button
            type="button"
            class="btn btn-ghost text-red-500 hover:bg-red-50 dark:hover:bg-red-950/30"
            :disabled="noteSaving || !editingInvitee?.inviter_note"
            @click="clearNote"
          >
            {{ t('affiliate.invitees.noteModal.clear') }}
          </button>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary" :disabled="noteSaving" @click="closeNoteEditor">
              {{ t('affiliate.invitees.noteModal.cancel') }}
            </button>
            <button type="button" class="btn btn-primary" :disabled="noteSaving" @click="saveNote">
              <Icon v-if="noteSaving" name="refresh" size="sm" class="animate-spin" />
              <span>{{ t('affiliate.invitees.noteModal.save') }}</span>
            </button>
          </div>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import userAPI from '@/api/user'
import type { AffiliateInvitee, UserAffiliateDetail } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useInboxStore } from '@/stores/inbox'
import {
  unreadRechargeInviteeIDs,
  latestRechargeSeq,
  sortInviteesByRecharge,
} from '@/components/common/affiliateRechargeInbox'
import { useClipboard } from '@/composables/useClipboard'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'
import { getPersistedPageSize } from '@/composables/usePersistedPageSize'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const inboxStore = useInboxStore()
const { copyToClipboard } = useClipboard()

/** 通用信箱灰度是否开启（决定是否消费充值通知做置顶 + 红箭头）。 */
const inboxEnabled = computed(() => appStore.cachedPublicSettings?.inbox_v1_enabled === true)

/**
 * rechargeInviteeIDs：本次进入页面时"有新充值(未读)"的被邀请人 id 集合。
 * 进入页面时快照一次（见 onMounted），使置顶/红箭头在本次浏览期间稳定展示；
 * 离开页面时统一 ack 清除，下次进入不再高亮。
 */
const rechargeInviteeIDs = ref<Set<number>>(new Set())

/** 某被邀请人是否有新充值（用于红色向上箭头）。 */
function hasNewRecharge(userID: number): boolean {
  return rechargeInviteeIDs.value.has(userID)
}

const loading = ref(true)
const transferring = ref(false)
const detail = ref<UserAffiliateDetail | null>(null)

// 邀请列表独立分页：不依赖 detail.invitees（那是老接口给的前 100 条快照）
const invitees = ref<AffiliateInvitee[]>([])
const inviteesTotal = ref(0)
const inviteesPage = ref(1)
const inviteesPageSize = ref(getPersistedPageSize())
const inviteesLoading = ref(false)

/** 展示用列表：inbox 启用时把有新充值的被邀请人稳定置顶，否则原样。 */
const displayInvitees = computed(() =>
  inboxEnabled.value
    ? sortInviteesByRecharge(invitees.value, rechargeInviteeIDs.value)
    : invitees.value,
)

// 备注编辑
const editingInvitee = ref<AffiliateInvitee | null>(null)
const noteDraft = ref('')
const noteSaving = ref(false)
// 用 Array.from 按 code point 计数，与后端 utf8.RuneCountInString 保持一致口径。
const noteDraftLength = computed(() => Array.from(noteDraft.value).length)

const inviteLink = computed(() => {
  if (!detail.value) return ''
  if (typeof window === 'undefined') return `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return `${window.location.origin}/register?aff=${encodeURIComponent(detail.value.aff_code)}`
})

// Rebate rate is a percentage in the range [0, 100]; backend already clamps it.
// We trim trailing zeros (e.g. 20.00 → "20", 12.50 → "12.5") for a cleaner UI.
const formattedRebateRate = computed(() => {
  const v = detail.value?.effective_rebate_rate_percent ?? 0
  const rounded = Math.round(v * 100) / 100
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
})

function formatCount(value: number): string {
  return value.toLocaleString()
}

async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
  }
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function loadInvitees(): Promise<void> {
  inviteesLoading.value = true
  try {
    const resp = await userAPI.listAffiliateInvitees(
      inviteesPage.value,
      inviteesPageSize.value,
    )
    invitees.value = resp.items
    inviteesTotal.value = resp.total
    // 如果当前页超出范围（例如从最后一页操作后回退），修正为最后一页并重新加载
    const pages = Math.max(1, Math.ceil(resp.total / inviteesPageSize.value))
    if (inviteesPage.value > pages) {
      inviteesPage.value = pages
      await loadInvitees()
    }
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    inviteesLoading.value = false
  }
}

function handleInviteesPageChange(page: number): void {
  inviteesPage.value = page
  void loadInvitees()
}

function handleInviteesPageSizeChange(pageSize: number): void {
  inviteesPageSize.value = pageSize
  inviteesPage.value = 1
  void loadInvitees()
}

function openNoteEditor(item: AffiliateInvitee): void {
  editingInvitee.value = item
  noteDraft.value = item.inviter_note ?? ''
}

function closeNoteEditor(): void {
  if (noteSaving.value) return
  editingInvitee.value = null
  noteDraft.value = ''
}

async function submitNote(rawNote: string, successKey: 'saveSuccess' | 'clearSuccess'): Promise<void> {
  const target = editingInvitee.value
  if (!target || noteSaving.value) return
  noteSaving.value = true
  try {
    await userAPI.updateAffiliateInviteeNote(target.user_id, rawNote)
    // 就地更新，避免整页 flicker
    const idx = invitees.value.findIndex((i) => i.user_id === target.user_id)
    if (idx !== -1) {
      invitees.value[idx] = { ...invitees.value[idx], inviter_note: rawNote }
    }
    appStore.showSuccess(t(`affiliate.invitees.noteModal.${successKey}`))
    editingInvitee.value = null
    noteDraft.value = ''
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.invitees.noteModal.saveFailed')))
  } finally {
    noteSaving.value = false
  }
}

async function saveNote(): Promise<void> {
  const note = noteDraft.value.trim()
  if (Array.from(note).length > 500) {
    appStore.showError(t('affiliate.invitees.noteModal.tooLong'))
    return
  }
  await submitNote(note, 'saveSuccess')
}

async function clearNote(): Promise<void> {
  await submitNote('', 'clearSuccess')
}

async function copyCode(): Promise<void> {
  if (!detail.value?.aff_code) return
  await copyToClipboard(detail.value.aff_code, t('affiliate.codeCopied'))
}

async function copyInviteLink(): Promise<void> {
  if (!inviteLink.value) return
  await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))
}

async function transferQuota(): Promise<void> {
  if (!detail.value || detail.value.aff_quota <= 0 || transferring.value) return
  transferring.value = true
  try {
    const resp = await userAPI.transferAffiliateQuota()
    appStore.showSuccess(t('affiliate.transfer.success', { amount: formatCurrency(resp.transferred_quota) }))
    await Promise.all([
      loadAffiliateDetail(true),
      loadInvitees(),
      authStore.refreshUser().catch(() => undefined),
    ])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.transferFailed')))
  } finally {
    transferring.value = false
  }
}

onMounted(() => {
  void loadAffiliateDetail()
  void loadInvitees()
  // 快照本次进入时"有新充值(未读)"的被邀请人集合，用于列表置顶 + 红色向上箭头。
  // 灰度关闭时集合为空，行为与旧版一致。
  if (inboxEnabled.value) {
    rechargeInviteeIDs.value = unreadRechargeInviteeIDs(
      inboxStore.messages,
      inboxStore.localAckSeq,
    )
  }
})

onBeforeUnmount(() => {
  // 离开页面视为已查看这些新充值：ack 到最新的充值通知 seq，下次进入不再高亮。
  // fail-safe：markReadUpTo 内部对已读/无消息是幂等的。
  if (!inboxEnabled.value) return
  const seq = latestRechargeSeq(inboxStore.messages)
  if (seq > 0) {
    void inboxStore.markReadUpTo(seq)
  }
})
</script>
