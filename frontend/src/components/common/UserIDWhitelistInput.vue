<template>
  <div>
    <div
      class="rounded-lg border border-gray-300 bg-white p-2 dark:border-dark-500 dark:bg-dark-700"
    >
      <div class="flex flex-wrap items-center gap-2">
        <span
          v-for="userId in modelValue"
          :key="userId"
          class="inline-flex items-center gap-1 rounded bg-gray-100 px-2 py-1 text-xs font-mono text-gray-700 dark:bg-dark-600 dark:text-gray-200"
        >
          <span>{{ chipLabel(userId) }}</span>
          <button
            type="button"
            class="rounded-full text-gray-500 hover:bg-gray-200 hover:text-gray-700 dark:text-gray-300 dark:hover:bg-dark-500 dark:hover:text-white"
            :aria-label="t('common.delete')"
            @click="removeUser(userId)"
          >
            <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
          </button>
        </span>

        <div
          class="flex min-w-[220px] flex-1 items-center gap-1 rounded border border-transparent px-2 py-1 focus-within:border-primary-300 dark:focus-within:border-primary-700"
        >
          <input
            v-model="draft"
            type="text"
            inputmode="numeric"
            class="w-full bg-transparent text-sm font-mono text-gray-900 outline-none placeholder:text-gray-400 dark:text-white dark:placeholder:text-gray-500"
            :placeholder="placeholder"
            :aria-label="ariaLabel"
            @keydown="handleKeydown"
            @blur="commitDraft"
            @paste="handlePaste"
          />
        </div>
      </div>
    </div>
    <p v-if="limitError" class="mt-2 text-xs text-red-600 dark:text-red-400">{{ limitError }}</p>
    <p v-else-if="hint" class="mt-2 text-xs text-gray-500 dark:text-dark-400">{{ hint }}</p>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import Icon from '@/components/icons/Icon.vue'

const MAX_USER_WHITELIST_ITEMS = 1000
const MAX_SAFE_USER_ID = Number.MAX_SAFE_INTEGER
const EMAIL_LOOKUP_CONCURRENCY = 8

const props = defineProps<{
  modelValue: number[]
  placeholder?: string
  hint?: string
  ariaLabel?: string
}>()
const emit = defineEmits<{ (event: 'update:modelValue', value: number[]): void }>()
const { t } = useI18n()

const draft = ref('')
const emails = ref<Record<number, string | null>>({})
const inflight = new Map<number, Promise<void>>()
const separatorKeys = new Set([' ', ',', '，', 'Enter', 'Tab'])
const limitError = ref('')

function chipLabel(userId: number): string {
  if (!(userId in emails.value)) return `${userId}(...)`
  const email = emails.value[userId]
  return email ? `${userId}(${email})` : `${userId}(${t('common.userNotFound')})`
}

function parseUserID(raw: string): number | null {
  const value = raw.trim()
  if (!/^\d+$/.test(value)) return null
  if (value.length > 16) return null
  const id = Number(value)
  if (!Number.isSafeInteger(id) || id <= 0 || id > MAX_SAFE_USER_ID) return null
  if (String(id) !== value.replace(/^0+(?=\d)/, '')) return null
  return id
}

function parseUserIDs(raw: string): number[] {
  return raw.split(/[\s,，]+/).flatMap((token) => {
    const id = parseUserID(token)
    return id === null ? [] : [id]
  })
}

function addUsers(ids: number[]) {
  const next = [...props.modelValue]
  const seen = new Set(next)
  let rejected = false
  for (const id of ids) {
    if (seen.has(id)) continue
    if (next.length >= MAX_USER_WHITELIST_ITEMS) {
      rejected = true
      break
    }
    seen.add(id)
    next.push(id)
  }
  limitError.value = rejected ? t('common.userWhitelistLimit', { count: MAX_USER_WHITELIST_ITEMS }) : ''
  if (next.length !== props.modelValue.length) emit('update:modelValue', next)
}

function removeUser(userId: number) {
  limitError.value = ''
  emit('update:modelValue', props.modelValue.filter((id) => id !== userId))
}

function commitDraft() {
  const id = parseUserID(draft.value)
  if (id !== null) addUsers([id])
  draft.value = ''
}

function handleKeydown(event: KeyboardEvent) {
  if (event.isComposing) return
  if (separatorKeys.has(event.key)) {
    event.preventDefault()
    commitDraft()
    return
  }
  if (event.key === 'Backspace' && !draft.value && props.modelValue.length > 0) {
    limitError.value = ''
    emit('update:modelValue', props.modelValue.slice(0, -1))
  }
}

function handlePaste(event: ClipboardEvent) {
  const text = event.clipboardData?.getData('text') || ''
  if (!text.trim()) return
  event.preventDefault()
  addUsers(parseUserIDs(text))
  draft.value = ''
}

function lookupEmail(id: number): Promise<void> {
  const pending = inflight.get(id)
  if (pending) return pending
  const request = (async () => {
    try {
      const user = await adminAPI.users.getById(id, true)
      emails.value = { ...emails.value, [id]: user.email || null }
    } catch {
      emails.value = { ...emails.value, [id]: null }
    } finally {
      inflight.delete(id)
    }
  })()
  inflight.set(id, request)
  return request
}

async function resolveEmails(ids: number[]) {
  const pending = ids.filter((id) => !(id in emails.value) && !inflight.has(id))
  for (let index = 0; index < pending.length; index += EMAIL_LOOKUP_CONCURRENCY) {
    await Promise.all(pending.slice(index, index + EMAIL_LOOKUP_CONCURRENCY).map(lookupEmail))
  }
}

watch(() => props.modelValue, (ids) => { void resolveEmails(ids) }, { immediate: true })
</script>
