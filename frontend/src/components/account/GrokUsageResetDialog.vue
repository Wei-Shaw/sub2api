<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.grokReset.title')"
    width="normal"
    :show-close-button="!loading"
    :close-on-escape="!loading"
    @close="close"
  >
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">
        {{ t('admin.accounts.grokReset.sessionHint', { account: account.name }) }}
      </p>
      <template v-if="!succeeded">
        <label class="block text-sm font-medium">
          {{ t('admin.accounts.grokReset.sessionLabel') }}
          <input
            v-model="sso"
            type="password"
            autocomplete="off"
            :disabled="loading || confirming"
            class="input mt-1 w-full"
            :aria-label="t('admin.accounts.grokReset.sessionLabel')"
          />
        </label>
        <p class="text-xs text-gray-500">{{ t('admin.accounts.grokReset.sessionPrivacy') }}</p>
        <button v-if="!confirming" type="button" class="btn btn-secondary" :disabled="loading || !sso.trim()" @click="query">
          {{ t('admin.accounts.grokReset.query') }}
        </button>
        <template v-if="queried">
          <p v-if="!cards.length" class="text-sm text-gray-500">{{ t('admin.accounts.grokReset.empty') }}</p>
          <label v-for="card in cards" :key="card.token_id" class="flex items-center gap-2 text-sm">
            <input v-model="selectedID" type="radio" :value="card.token_id" :disabled="loading || confirming" />
            {{ t('admin.accounts.grokReset.expires', { time: formatExpiry(card.expires_at) }) }}
          </label>
        </template>
        <p v-if="confirming" class="text-sm text-amber-700 dark:text-amber-300">
          {{ t('admin.accounts.grokReset.confirmMessage', { time: formatExpiry(selectedCard?.expires_at || '') }) }}
        </p>
      </template>
      <p v-if="succeeded" role="status" class="text-sm text-green-700 dark:text-green-300">{{ t('admin.accounts.grokReset.success') }}</p>
      <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      <a class="text-sm text-primary-600 hover:underline" href="https://grok.com/?_s=usage" target="_blank" rel="noopener noreferrer">
        {{ t('admin.accounts.grokReset.openOfficial') }}
      </a>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="confirming ? confirming = false : close()">
          {{ t(confirming ? 'common.cancel' : 'common.close') }}
        </button>
        <button v-if="!succeeded && selectedCard" type="button" class="btn btn-primary" :disabled="loading" @click="confirming ? redeem() : confirming = true">
          {{ t(confirming ? 'admin.accounts.grokReset.confirm' : 'admin.accounts.grokReset.redeem') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { queryUsageResetCards, redeemUsageResetCard, type GrokUsageResetCard } from '@/api/admin/grok'
import type { Account } from '@/types'

const props = defineProps<{ show: boolean; account: Account }>()
const emit = defineEmits<{ close: []; redeemed: [] }>()
const { t, locale } = useI18n()
const sso = ref('')
const cards = ref<GrokUsageResetCard[]>([])
const selectedID = ref('')
const loading = ref(false)
const queried = ref(false)
const confirming = ref(false)
const succeeded = ref(false)
const error = ref('')
let generation = 0

const selectedCard = computed(() => cards.value.find(card => card.token_id === selectedID.value))
const formatExpiry = (value: string) => value ? new Date(value).toLocaleString(locale.value) : ''

function clearCards() {
  cards.value = []
  selectedID.value = ''
  queried.value = false
  confirming.value = false
}

function clearSession() {
  generation++
  sso.value = ''
  clearCards()
  loading.value = false
  succeeded.value = false
  error.value = ''
}

function close() {
  if (loading.value) return
  clearSession()
  emit('close')
}

async function query() {
  if (loading.value || !sso.value.trim()) return
  const attempt = ++generation
  loading.value = true
  error.value = ''
  clearCards()
  try {
    const result = await queryUsageResetCards(props.account.id, sso.value)
    if (attempt !== generation) return
    cards.value = result.cards
    selectedID.value = result.cards[0]?.token_id || ''
    queried.value = true
  } catch {
    if (attempt === generation) error.value = t('admin.accounts.grokReset.queryFailed')
  } finally {
    if (attempt === generation) loading.value = false
  }
}

async function redeem() {
  if (loading.value || !confirming.value || !selectedCard.value) return
  if (new Date(selectedCard.value.expires_at).getTime() <= Date.now()) {
    clearCards()
    error.value = t('admin.accounts.grokReset.queryAgain')
    return
  }
  const attempt = ++generation
  loading.value = true
  error.value = ''
  try {
    await redeemUsageResetCard(props.account.id, sso.value, selectedID.value)
    if (attempt !== generation) return
    succeeded.value = true
    sso.value = ''
    clearCards()
    emit('redeemed')
  } catch {
    if (attempt !== generation) return
    // A timeout can occur after redemption. Require a fresh card query instead
    // of retaining an enabled retry button for a possibly consumed card.
    clearCards()
    error.value = t('admin.accounts.grokReset.redeemUnknown')
  } finally {
    if (attempt === generation) loading.value = false
  }
}

watch(sso, clearCards)
watch([() => props.show, () => props.account.id], clearSession)
onBeforeUnmount(clearSession)
</script>
