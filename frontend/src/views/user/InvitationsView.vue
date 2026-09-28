<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div
          class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"
        ></div>
      </div>

      <template v-else-if="overview">
        <div class="grid gap-4 sm:grid-cols-3">
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('userInvitation.stats.used') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white" data-testid="invitation-used">
              {{ overview.used_count }}
              <span v-if="overview.max_codes_per_user > 0" class="text-base font-medium text-gray-400 dark:text-dark-500">
                / {{ overview.max_codes_per_user }}
              </span>
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('userInvitation.stats.remaining') }}</p>
            <p class="mt-2 text-2xl font-semibold text-emerald-600 dark:text-emerald-400" data-testid="invitation-remaining">
              {{ overview.remaining < 0 ? t('userInvitation.stats.unlimited') : overview.remaining }}
            </p>
          </div>
          <div class="card p-5">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('userInvitation.stats.validity') }}</p>
            <p class="mt-2 text-2xl font-semibold text-gray-900 dark:text-white">
              {{
                overview.code_validity_days > 0
                  ? t('userInvitation.stats.validityDays', { days: overview.code_validity_days })
                  : t('userInvitation.stats.neverExpires')
              }}
            </p>
          </div>
        </div>

        <div class="card p-6">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('userInvitation.title') }}</h3>
              <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('userInvitation.description') }}</p>
            </div>
            <button
              class="btn btn-primary"
              data-testid="invitation-generate"
              :disabled="!canGenerate"
              @click="generate"
            >
              <Icon v-if="creating" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="userPlus" size="sm" />
              <span>{{ creating ? t('userInvitation.generating') : t('userInvitation.generate') }}</span>
            </button>
          </div>
          <p v-if="!overview.enabled" class="mt-3 text-sm text-amber-600 dark:text-amber-400">
            {{ t('userInvitation.disabled') }}
          </p>
          <p v-else-if="overview.remaining === 0" class="mt-3 text-sm text-amber-600 dark:text-amber-400">
            {{ t('userInvitation.limitReached') }}
          </p>

          <div class="mt-5 rounded-xl border border-primary-200 bg-primary-50 p-4 dark:border-primary-900/40 dark:bg-primary-900/20">
            <p class="text-sm font-medium text-primary-800 dark:text-primary-200">{{ t('userInvitation.tips.title') }}</p>
            <ul class="mt-2 space-y-1 text-sm text-primary-700 dark:text-primary-300">
              <li>1. {{ t('userInvitation.tips.line1') }}</li>
              <li>2. {{ t('userInvitation.tips.line2') }}</li>
              <li v-if="overview.code_validity_days > 0">3. {{ t('userInvitation.tips.line3') }}</li>
            </ul>
          </div>
        </div>

        <div class="card p-6">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ t('userInvitation.list.title') }}</h3>
          <div
            v-if="overview.codes.length === 0"
            class="mt-4 rounded-xl border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-dark-400"
          >
            {{ t('userInvitation.list.empty') }}
          </div>
          <div v-else class="mt-4 overflow-x-auto">
            <table class="w-full min-w-[720px] text-left text-sm">
              <thead>
                <tr class="border-b border-gray-200 text-gray-500 dark:border-dark-700 dark:text-dark-400">
                  <th class="px-3 py-2 font-medium">{{ t('userInvitation.list.columns.code') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('userInvitation.list.columns.status') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('userInvitation.list.columns.invitee') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('userInvitation.list.columns.createdAt') }}</th>
                  <th class="px-3 py-2 font-medium">{{ t('userInvitation.list.columns.expiresAt') }}</th>
                  <th class="px-3 py-2 font-medium text-right">{{ t('userInvitation.list.columns.actions') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in overview.codes"
                  :key="item.code"
                  class="border-b border-gray-100 last:border-b-0 dark:border-dark-800"
                >
                  <td class="px-3 py-3">
                    <code class="font-semibold text-gray-900 dark:text-white">{{ item.code }}</code>
                  </td>
                  <td class="px-3 py-3">
                    <span :class="['badge', statusBadgeClass(item.status)]">
                      {{ t(`userInvitation.list.status.${item.status}`) }}
                    </span>
                  </td>
                  <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ item.used_by_email_mask || '-' }}</td>
                  <td class="px-3 py-3 text-gray-700 dark:text-gray-300">{{ formatDateTime(item.created_at) || '-' }}</td>
                  <td class="px-3 py-3 text-gray-700 dark:text-gray-300">
                    {{ item.expires_at ? formatDateTime(item.expires_at) : t('userInvitation.stats.neverExpires') }}
                  </td>
                  <td class="px-3 py-3">
                    <div v-if="item.status === 'unused'" class="flex justify-end gap-2">
                      <button class="btn btn-secondary btn-sm" :title="t('userInvitation.list.copyCode')" @click="copyCode(item.code)">
                        <Icon name="copy" size="sm" />
                        <span class="hidden sm:inline">{{ t('userInvitation.list.copyCode') }}</span>
                      </button>
                      <button class="btn btn-secondary btn-sm" :title="t('userInvitation.list.copyLink')" @click="copyLink(item.code)">
                        <Icon name="link" size="sm" />
                        <span class="hidden sm:inline">{{ t('userInvitation.list.copyLink') }}</span>
                      </button>
                    </div>
                    <div v-else class="text-right text-gray-400 dark:text-dark-500">-</div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import invitationsAPI, { type UserInvitationOverview, type UserInvitationStatus } from '@/api/invitations'
import { useAppStore } from '@/stores/app'
import { useClipboard } from '@/composables/useClipboard'
import { formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const creating = ref(false)
const overview = ref<UserInvitationOverview | null>(null)

const canGenerate = computed(
  () => !!overview.value && overview.value.enabled && overview.value.remaining !== 0 && !creating.value
)

function buildInviteLink(code: string): string {
  const path = `/register?invitation_code=${encodeURIComponent(code)}`
  if (typeof window === 'undefined') return path
  return `${window.location.origin}${path}`
}

function statusBadgeClass(status: UserInvitationStatus): string {
  switch (status) {
    case 'used':
      return 'badge-success'
    case 'expired':
      return 'badge-gray'
    default:
      return 'badge-primary'
  }
}

async function loadOverview(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
  }
  try {
    overview.value = await invitationsAPI.getOverview()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('userInvitation.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function generate(): Promise<void> {
  if (!canGenerate.value) return
  creating.value = true
  try {
    const created = await invitationsAPI.create()
    appStore.showSuccess(t('userInvitation.created'))
    await loadOverview(true)
    await copyToClipboard(buildInviteLink(created.code), t('userInvitation.list.linkCopied'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('userInvitation.createFailed')))
    await loadOverview(true)
  } finally {
    creating.value = false
  }
}

async function copyCode(code: string): Promise<void> {
  await copyToClipboard(code, t('userInvitation.list.codeCopied'))
}

async function copyLink(code: string): Promise<void> {
  await copyToClipboard(buildInviteLink(code), t('userInvitation.list.linkCopied'))
}

onMounted(() => {
  void loadOverview()
})
</script>
