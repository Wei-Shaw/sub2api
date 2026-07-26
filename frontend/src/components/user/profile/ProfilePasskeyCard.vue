<template>
  <div class="card">
    <div class="flex items-start justify-between border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div>
        <h2 class="text-lg font-medium text-gray-900 dark:text-white">
          {{ t('profile.passkey.title') REDACTEDREDACTED
        </h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.passkey.description') REDACTEDREDACTED
        </p>
      </div>
      <button
        v-if="enabled && supported && !showAddForm"
        type="button"
        class="btn btn-primary"
        :disabled="busy"
        @click="showAddForm = true"
      >
        {{ t('profile.passkey.add') REDACTEDREDACTED
      </button>
    </div>

    <div class="px-6 py-6">
      <div v-if="!enabled" class="mb-5 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.passkey.featureDisabled') REDACTEDREDACTED
      </div>
      <div v-if="enabled && !supported" class="mb-5 text-sm text-amber-600 dark:text-amber-400">
        {{ t('profile.passkey.unsupported') REDACTEDREDACTED
      </div>
      <div>
        <form
          v-if="enabled && supported && showAddForm"
          class="mb-5 flex flex-col gap-3 rounded-lg border border-gray-200 p-4 dark:border-dark-700 sm:flex-row sm:items-end"
          @submit.prevent="addPasskey"
        >
          <div class="flex-1">
            <label for="passkey-name" class="input-label">{{ t('profile.passkey.name') REDACTEDREDACTED</label>
            <input
              id="passkey-name"
              v-model="newName"
              class="input"
              maxlength="100"
              :placeholder="t('profile.passkey.namePlaceholder')"
              autofocus
            />
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn btn-secondary" :disabled="busy" @click="cancelAdd">
              {{ t('common.cancel') REDACTEDREDACTED
            </button>
            <button type="submit" class="btn btn-primary" :disabled="busy">
              {{ busy ? t('common.processing') : t('profile.passkey.continue') REDACTEDREDACTED
            </button>
          </div>
        </form>

        <div v-if="loading" class="flex justify-center py-6">
          <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-500"></div>
        </div>

        <div
          v-else-if="credentials.length === 0"
          class="rounded-lg border border-dashed border-gray-200 px-4 py-8 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400"
        >
          {{ t('profile.passkey.empty') REDACTEDREDACTED
        </div>

        <div v-else class="divide-y divide-gray-100 dark:divide-dark-700">
          <div
            v-for="credential in credentials"
            :key="credential.id"
            class="flex items-center justify-between gap-4 py-4 first:pt-0 last:pb-0"
          >
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Icon name="key" size="md" class="shrink-0 text-primary-500" />
                <p class="truncate font-medium text-gray-900 dark:text-white">
                  {{ credential.name REDACTEDREDACTED
                </p>
                <span
                  v-if="credential.backup"
                  class="rounded-full bg-green-50 px-2 py-0.5 text-xs text-green-700 dark:bg-green-900/30 dark:text-green-300"
                >
                  {{ t('profile.passkey.synced') REDACTEDREDACTED
                </span>
              </div>
              <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                {{ t('profile.passkey.createdAt', { date: formatDate(credential.created_at) REDACTED) REDACTEDREDACTED
                <template v-if="credential.last_used_at">
                  · {{ t('profile.passkey.lastUsed', { date: formatDate(credential.last_used_at) REDACTED) REDACTEDREDACTED
                </template>
              </p>
            </div>
            <div class="flex shrink-0 gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="busy"
                @click="renamePasskey(credential)"
              >
                {{ t('common.edit') REDACTEDREDACTED
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-sm text-red-600 hover:bg-red-50 dark:text-red-300 dark:hover:bg-red-950/30"
                :disabled="busy"
                @click="deletePasskey(credential)"
              >
                {{ t('common.delete') REDACTEDREDACTED
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { passkeyAPI, type PasskeyCredentialSummary REDACTED from '@/api'
import { Icon REDACTED from '@/components/icons'
import { useAppStore REDACTED from '@/stores/app'

const props = defineProps<{ enabled: boolean REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()
const supported = passkeyAPI.isSupported()
const loading = ref(false)
const busy = ref(false)
const showAddForm = ref(false)
const newName = ref('')
const credentials = ref<PasskeyCredentialSummary[]>([])

async function loadCredentials(): Promise<void> {
  loading.value = true
  try {
    credentials.value = await passkeyAPI.list()
  REDACTED catch (error) {
    const code = (error as { code?: string REDACTED).code
    if (code !== 'PASSKEY_DISABLED') {
      appStore.showError(t('profile.passkey.loadFailed'))
    REDACTED
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

async function addPasskey(): Promise<void> {
  busy.value = true
  try {
    await passkeyAPI.register(newName.value.trim())
    appStore.showSuccess(t('profile.passkey.added'))
    cancelAdd()
    await loadCredentials()
  REDACTED catch (error) {
    if (!(error instanceof DOMException && error.name === 'NotAllowedError')) {
      appStore.showError(t('profile.passkey.addFailed'))
    REDACTED
  REDACTED finally {
    busy.value = false
  REDACTED
REDACTED

function cancelAdd(): void {
  showAddForm.value = false
  newName.value = ''
REDACTED

async function renamePasskey(credential: PasskeyCredentialSummary): Promise<void> {
  const name = window.prompt(t('profile.passkey.renamePrompt'), credential.name)?.trim()
  if (!name || name === credential.name) return
  busy.value = true
  try {
    await passkeyAPI.rename(credential.id, name)
    credential.name = name
    appStore.showSuccess(t('profile.passkey.renamed'))
  REDACTED catch {
    appStore.showError(t('profile.passkey.renameFailed'))
  REDACTED finally {
    busy.value = false
  REDACTED
REDACTED

async function deletePasskey(credential: PasskeyCredentialSummary): Promise<void> {
  if (!window.confirm(t('profile.passkey.deleteConfirm', { name: credential.name REDACTED))) return
  busy.value = true
  try {
    await passkeyAPI.remove(credential.id)
    credentials.value = credentials.value.filter((item) => item.id !== credential.id)
    appStore.showSuccess(t('profile.passkey.deleted'))
  REDACTED catch {
    appStore.showError(t('profile.passkey.deleteFailed'))
  REDACTED finally {
    busy.value = false
  REDACTED
REDACTED

function formatDate(value: string): string {
  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric'
  REDACTED).format(new Date(value))
REDACTED

watch(
  () => props.enabled,
  () => {
    void loadCredentials()
  REDACTED,
  { immediate: true REDACTED
)
</script>
