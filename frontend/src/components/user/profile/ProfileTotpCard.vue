<template>
  <div class="rounded border border-line bg-surface">
    <div class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.totp.title') }}
      </h2>
      <p class="mt-0.5 text-xs text-ink-tertiary">
        {{ t('profile.totp.description') }}
      </p>
    </div>
    <div class="px-4 py-4">
      <!-- Loading state -->
      <div v-if="loading" class="space-y-2 py-2">
        <div class="skeleton h-3 w-32"></div>
        <div class="skeleton h-3 w-56"></div>
      </div>

      <!--
        Three states, one shape. Each used to lead with a 48px pastel circle
        holding a 24px glyph — the loudest element in the row spent on
        decoration. State is a 6px dot beside the word that says it.
      -->
      <div
        v-else-if="status && !status.feature_enabled"
        class="flex flex-wrap items-start justify-between gap-3"
      >
        <div class="min-w-0">
          <StatusDot tone="neutral" :label="t('profile.totp.featureDisabled')" />
          <p class="mt-1 text-xs text-ink-tertiary">
            {{ t('profile.totp.featureDisabledHint') }}
          </p>
        </div>
      </div>

      <div v-else-if="status?.enabled" class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <StatusDot tone="success" :label="t('profile.totp.enabled')" />
          <p v-if="status.enabled_at" class="mt-1 text-xs text-ink-tertiary">
            {{ t('profile.totp.enabledAt') }}:
            <span class="font-mono tabular-nums">{{ formatDate(status.enabled_at) }}</span>
          </p>
        </div>
        <!--
          `btn-outline-danger` was never defined in style.css, so this control
          rendered as a bare transparent button with no danger channel at all.
        -->
        <Button tone="danger" variant="outline" size="md" @click="showDisableDialog = true">
          {{ t('profile.totp.disable') }}
        </Button>
      </div>

      <div v-else class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <StatusDot tone="neutral" :label="t('profile.totp.notEnabled')" />
          <p class="mt-1 text-xs text-ink-tertiary">
            {{ t('profile.totp.notEnabledHint') }}
          </p>
        </div>
        <Button tone="accent" variant="solid" size="md" @click="showSetupModal = true">
          {{ t('profile.totp.enable') }}
        </Button>
      </div>
    </div>

    <!-- Setup Modal -->
    <TotpSetupModal
      v-if="showSetupModal"
      @close="showSetupModal = false"
      @success="handleSetupSuccess"
    />

    <!-- Disable Dialog -->
    <TotpDisableDialog
      v-if="showDisableDialog"
      @close="showDisableDialog = false"
      @success="handleDisableSuccess"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { totpAPI } from '@/api'
import Button from '@/components/common/Button.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import type { TotpStatus } from '@/types'
import TotpSetupModal from './TotpSetupModal.vue'
import TotpDisableDialog from './TotpDisableDialog.vue'

const { t } = useI18n()

const loading = ref(true)
const status = ref<TotpStatus | null>(null)
const showSetupModal = ref(false)
const showDisableDialog = ref(false)

const loadStatus = async () => {
  loading.value = true
  try {
    status.value = await totpAPI.getStatus()
  } catch (error) {
    console.error('Failed to load TOTP status:', error)
  } finally {
    loading.value = false
  }
}

const handleSetupSuccess = () => {
  showSetupModal.value = false
  loadStatus()
}

const handleDisableSuccess = () => {
  showDisableDialog.value = false
  loadStatus()
}

const formatDate = (timestamp: number) => {
  // Backend returns Unix timestamp in seconds, convert to milliseconds
  const date = new Date(timestamp * 1000)
  // Numeric parts, not "August 13, 2026": a timestamp is a quantity and has to
  // sit in the mono column with every other number on the page.
  return date.toLocaleDateString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

onMounted(() => {
  loadStatus()
})
</script>
