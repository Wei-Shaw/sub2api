<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">
          {{ t('admin.users.reviewDetails') }}
        </h2>
        <RouterLink to="/admin/users" class="btn btn-secondary">
          {{ t('admin.users.backToUsers') }}
        </RouterLink>
      </div>

      <div v-if="loading" class="flex justify-center py-12" role="status">
        <LoadingSpinner />
        <span class="sr-only">{{ t('common.loading') }}</span>
      </div>
      <div v-else-if="loadFailed" class="card space-y-4 p-6" role="alert">
        <p class="text-red-600 dark:text-red-400">{{ t('admin.users.reviewDetailsLoadFailed') }}</p>
        <button class="btn btn-secondary" data-test="retry" @click="retry++">
          {{ t('common.tryAgain') }}
        </button>
      </div>
      <template v-else-if="user">
        <section class="card p-6">
          <dl class="grid gap-6 text-sm sm:grid-cols-2">
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.users.columns.email') }}</dt>
              <dd class="mt-1 break-all text-gray-900 dark:text-white">{{ user.email }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.users.columns.username') }}</dt>
              <dd class="mt-1 break-all text-gray-900 dark:text-white">{{ user.username || '-' }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.users.columns.created') }}</dt>
              <dd class="mt-1 text-gray-900 dark:text-white">{{ formatDateTime(user.created_at) }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('admin.users.columns.status') }}</dt>
              <dd class="mt-1 font-medium" :class="user.status === 'pending' ? 'text-yellow-600 dark:text-yellow-400' : 'text-gray-900 dark:text-white'">
                {{ user.status === 'pending' ? t('admin.users.pendingReview') : user.status === 'active' ? t('common.active') : t('admin.users.disabled') }}
              </dd>
            </div>
          </dl>
        </section>
        <section class="card p-6">
          <h3 class="font-semibold text-gray-900 dark:text-white">{{ t('admin.users.applicationContent') }}</h3>
          <p data-test="reason" class="mt-4 whitespace-pre-wrap break-words text-sm leading-7 text-gray-700 dark:text-gray-300 [overflow-wrap:anywhere]">{{ user.registration_reason || t('admin.users.noApplicationContent') }}</p>
        </section>
        <div v-if="user.status === 'pending' && user.role !== 'admin'" class="flex flex-wrap justify-end gap-3">
          <button data-test="disabled" class="btn btn-danger" :disabled="saving" @click="review('disabled')">
            {{ t('admin.users.reviewReject') }}
          </button>
          <button data-test="active" class="btn btn-primary" :disabled="saving" @click="review('active')">
            {{ t('admin.users.reviewApprove') }}
          </button>
        </div>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminUser } from '@/types'
import { formatDateTime } from '@/utils/format'
import AppLayout from '@/components/layout/AppLayout.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'

const props = defineProps<{ id: string }>()
const { t } = useI18n()
const appStore = useAppStore()
const user = ref<AdminUser | null>(null)
const loading = ref(true)
const loadFailed = ref(false)
const saving = ref(false)
const retry = ref(0)

watch([() => props.id, retry], async ([id], _previous, onCleanup) => {
  let cancelled = false
  onCleanup(() => { cancelled = true })
  user.value = null
  loading.value = true
  loadFailed.value = false
  try {
    const userId = Number(id)
    if (!Number.isSafeInteger(userId) || userId <= 0) throw new Error('Invalid user ID')
    const applicant = await adminAPI.users.getById(userId)
    if (!cancelled) user.value = applicant
  } catch {
    if (!cancelled) loadFailed.value = true
  } finally {
    if (!cancelled) loading.value = false
  }
}, { immediate: true })

async function review(status: 'active' | 'disabled') {
  if (!user.value || user.value.status !== 'pending' || user.value.role === 'admin' || saving.value) return
  const applicant = user.value
  saving.value = true
  try {
    const updated = await adminAPI.users.toggleStatus(applicant.id, status)
    if (user.value === applicant) {
      user.value = updated
      appStore.showSuccess(t(status === 'active' ? 'admin.users.reviewApproved' : 'admin.users.reviewRejected'))
    }
  } catch {
    appStore.showError(t('admin.users.failedToToggle'))
  } finally {
    saving.value = false
  }
}
</script>
