<template>
  <div>
    <!-- Section header -->
    <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <h3 class="text-sm font-medium text-gray-900 dark:text-gray-100">
        {{ t('admin.accounts.quotaControl.title') }}
      </h3>
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaControl.hint') }}
      </p>
    </div>

    <!-- Window Cost Limit -->
    <ToggleCard :label="t('admin.accounts.quotaControl.windowCost.label')"
      :hint="t('admin.accounts.quotaControl.windowCost.hint')"
      :enabled="windowCostEnabled"
      @update:enabled="$emit('update:windowCostEnabled', $event)">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.windowCost.limit') }}</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
            <input :value="windowCostLimit" type="number" min="0" step="1" class="input pl-7"
              :placeholder="t('admin.accounts.quotaControl.windowCost.limitPlaceholder')"
              @input="$emit('update:windowCostLimit', numOrNull($event))" />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.limitHint') }}</p>
        </div>
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.windowCost.stickyReserve') }}</label>
          <div class="relative">
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">$</span>
            <input :value="windowCostStickyReserve" type="number" min="0" step="1" class="input pl-7"
              :placeholder="t('admin.accounts.quotaControl.windowCost.stickyReservePlaceholder')"
              @input="$emit('update:windowCostStickyReserve', numOrNull($event))" />
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.windowCost.stickyReserveHint') }}</p>
        </div>
      </div>
    </ToggleCard>

    <!-- Session Limit -->
    <ToggleCard :label="t('admin.accounts.quotaControl.sessionLimit.label')"
      :hint="t('admin.accounts.quotaControl.sessionLimit.hint')"
      :enabled="sessionLimitEnabled"
      @update:enabled="$emit('update:sessionLimitEnabled', $event)">
      <div class="grid grid-cols-2 gap-4">
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessions') }}</label>
          <input :value="maxSessions" type="number" min="1" class="input"
            :placeholder="t('admin.accounts.quotaControl.sessionLimit.maxSessionsPlaceholder')"
            @input="$emit('update:maxSessions', numOrNull($event))" />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.maxSessionsHint') }}</p>
        </div>
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeout') }}</label>
          <div class="relative">
            <input :value="sessionIdleTimeout" type="number" min="1" class="input pr-8"
              :placeholder="t('admin.accounts.quotaControl.sessionLimit.idleTimeoutPlaceholder')"
              @input="$emit('update:sessionIdleTimeout', numOrNull($event))" />
            <span class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-500">min</span>
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.sessionLimit.idleTimeoutHint') }}</p>
        </div>
      </div>
    </ToggleCard>

    <!-- RPM Limit -->
    <ToggleCard :label="t('admin.accounts.quotaControl.rpmLimit.label')"
      :hint="t('admin.accounts.quotaControl.rpmLimit.hint')"
      :enabled="rpmLimitEnabled"
      @update:enabled="$emit('update:rpmLimitEnabled', $event)">
      <div class="space-y-4">
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpm') }}</label>
          <input :value="baseRpm" type="number" min="1" max="1000" class="input"
            :placeholder="t('admin.accounts.quotaControl.rpmLimit.baseRpmPlaceholder')"
            @input="$emit('update:baseRpm', numOrNull($event))" />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.baseRpmHint') }}</p>
        </div>
        <div>
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.strategy') }}</label>
          <div class="flex gap-2">
            <button type="button" @click="$emit('update:rpmStrategy', 'tiered')"
              :class="strategyBtnClass(rpmStrategy === 'tiered')">
              {{ t('admin.accounts.quotaControl.rpmLimit.strategyTiered') }}
            </button>
            <button type="button" @click="$emit('update:rpmStrategy', 'sticky_exempt')"
              :class="strategyBtnClass(rpmStrategy === 'sticky_exempt')">
              {{ t('admin.accounts.quotaControl.rpmLimit.strategyStickyExempt') }}
            </button>
          </div>
          <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.strategyHint') }}</p>
        </div>
        <div v-if="rpmStrategy === 'tiered'">
          <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBuffer') }}</label>
          <input :value="rpmStickyBuffer" type="number" min="1" step="1" class="input"
            :placeholder="t('admin.accounts.quotaControl.rpmLimit.stickyBufferPlaceholder')"
            @input="$emit('update:rpmStickyBuffer', numOrNull($event))" />
          <p class="input-hint">{{ t('admin.accounts.quotaControl.rpmLimit.stickyBufferHint') }}</p>
        </div>
      </div>
    </ToggleCard>

    <!-- User Message Queue Mode -->
    <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
      <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueue') }}</label>
      <p class="mt-1 mb-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.accounts.quotaControl.rpmLimit.userMsgQueueHint') }}
      </p>
      <div class="flex space-x-2">
        <button type="button" v-for="opt in umqModeOptions" :key="opt.value"
          @click="$emit('update:userMsgQueueMode', userMsgQueueMode === opt.value ? '' : opt.value)"
          :class="['px-3 py-1.5 text-sm rounded-md border transition-colors',
            userMsgQueueMode === opt.value
              ? 'bg-primary-600 text-white border-primary-600'
              : 'bg-white dark:bg-dark-700 text-gray-700 dark:text-gray-300 border-gray-300 dark:border-dark-500 hover:bg-gray-50 dark:hover:bg-dark-600']">
          {{ opt.label }}
        </button>
      </div>
    </div>

    <!-- TLS Fingerprint -->
    <ToggleCard :label="t('admin.accounts.quotaControl.tlsFingerprint.label')"
      :hint="t('admin.accounts.quotaControl.tlsFingerprint.hint')"
      :enabled="tlsFingerprintEnabled"
      @update:enabled="$emit('update:tlsFingerprintEnabled', $event)">
      <select :value="tlsFingerprintProfileId" class="input"
        @change="$emit('update:tlsFingerprintProfileId', selectNum($event))">
        <option :value="null">{{ t('admin.accounts.quotaControl.tlsFingerprint.defaultProfile') }}</option>
        <option v-if="tlsFingerprintProfiles.length > 0" :value="-1">
          {{ t('admin.accounts.quotaControl.tlsFingerprint.randomProfile') }}
        </option>
        <option v-for="p in tlsFingerprintProfiles" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
    </ToggleCard>

    <!-- Session ID Masking -->
    <ToggleCard :label="t('admin.accounts.quotaControl.sessionIdMasking.label')"
      :hint="t('admin.accounts.quotaControl.sessionIdMasking.hint')"
      :enabled="sessionIdMaskingEnabled"
      @update:enabled="$emit('update:sessionIdMaskingEnabled', $event)" />

    <!-- Cache TTL Override -->
    <ToggleCard :label="t('admin.accounts.quotaControl.cacheTTLOverride.label')"
      :hint="t('admin.accounts.quotaControl.cacheTTLOverride.hint')"
      :enabled="cacheTTLOverrideEnabled"
      @update:enabled="$emit('update:cacheTTLOverrideEnabled', $event)">
      <div>
        <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.cacheTTLOverride.target') }}</label>
        <select :value="cacheTTLOverrideTarget" class="input"
          @change="$emit('update:cacheTTLOverrideTarget', ($event.target as HTMLSelectElement).value)">
          <option value="5m">5m</option>
          <option value="1h">1h</option>
        </select>
        <p class="input-hint">{{ t('admin.accounts.quotaControl.cacheTTLOverride.targetHint') }}</p>
      </div>
    </ToggleCard>

    <!-- Custom Base URL -->
    <ToggleCard :label="t('admin.accounts.quotaControl.customBaseUrl.label')"
      :hint="t('admin.accounts.quotaControl.customBaseUrl.hint')"
      :enabled="customBaseUrlEnabled"
      @update:enabled="$emit('update:customBaseUrlEnabled', $event)">
      <div>
        <input :value="customBaseUrl" type="text" class="input" placeholder="https://relay.example.com"
          @input="$emit('update:customBaseUrl', ($event.target as HTMLInputElement).value)" />
        <p class="input-hint">{{ t('admin.accounts.quotaControl.customBaseUrl.urlHint') }}</p>
      </div>
    </ToggleCard>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ToggleCard } from '@sub2api/plugin-sdk'

const { t } = useI18n()

defineProps<{
  windowCostEnabled: boolean
  windowCostLimit: number | null
  windowCostStickyReserve: number | null
  sessionLimitEnabled: boolean
  maxSessions: number | null
  sessionIdleTimeout: number | null
  rpmLimitEnabled: boolean
  baseRpm: number | null
  rpmStrategy: 'tiered' | 'sticky_exempt'
  rpmStickyBuffer: number | null
  userMsgQueueMode: string
  umqModeOptions: { value: string; label: string }[]
  tlsFingerprintEnabled: boolean
  tlsFingerprintProfileId: number | null
  tlsFingerprintProfiles: { id: number; name: string }[]
  sessionIdMaskingEnabled: boolean
  cacheTTLOverrideEnabled: boolean
  cacheTTLOverrideTarget: string
  customBaseUrlEnabled: boolean
  customBaseUrl: string
}>()

defineEmits<{
  'update:windowCostEnabled': [value: boolean]
  'update:windowCostLimit': [value: number | null]
  'update:windowCostStickyReserve': [value: number | null]
  'update:sessionLimitEnabled': [value: boolean]
  'update:maxSessions': [value: number | null]
  'update:sessionIdleTimeout': [value: number | null]
  'update:rpmLimitEnabled': [value: boolean]
  'update:baseRpm': [value: number | null]
  'update:rpmStrategy': [value: 'tiered' | 'sticky_exempt']
  'update:rpmStickyBuffer': [value: number | null]
  'update:userMsgQueueMode': [value: string]
  'update:tlsFingerprintEnabled': [value: boolean]
  'update:tlsFingerprintProfileId': [value: number | null]
  'update:sessionIdMaskingEnabled': [value: boolean]
  'update:cacheTTLOverrideEnabled': [value: boolean]
  'update:cacheTTLOverrideTarget': [value: string]
  'update:customBaseUrlEnabled': [value: boolean]
  'update:customBaseUrl': [value: string]
}>()

function strategyBtnClass(active: boolean): string {
  return [
    'flex-1 rounded-lg px-3 py-2 text-sm font-medium transition-all',
    active
      ? 'bg-primary-100 text-primary-700 dark:bg-primary-900/30 dark:text-primary-400'
      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500',
  ].join(' ')
}

function numOrNull(event: Event): number | null {
  const val = (event.target as HTMLInputElement).valueAsNumber
  return Number.isFinite(val) ? val : null
}

function selectNum(event: Event): number | null {
  const val = (event.target as HTMLSelectElement).value
  const num = Number(val)
  return Number.isFinite(num) ? num : null
}
</script>
