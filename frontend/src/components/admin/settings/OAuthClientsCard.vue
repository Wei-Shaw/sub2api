<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <div class="flex items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.oauthClients.title') }}
          </h2>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.oauthClients.description') }}
          </p>
        </div>
        <button type="button" class="btn btn-primary shrink-0" @click="openCreate">
          {{ t('admin.settings.oauthClients.add') }}
        </button>
      </div>
    </div>

    <div class="px-6 py-6">
      <!-- Loading -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
      </div>

      <!-- Load error -->
      <div v-else-if="error" class="text-sm text-red-600 dark:text-red-400">
        {{ error }}
      </div>

      <!-- Empty -->
      <p v-else-if="clients.length === 0" class="py-2 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.oauthClients.empty') }}
      </p>

      <!-- Client list -->
      <ul v-else class="space-y-4">
        <li
          v-for="client in clients"
          :key="client.client_id"
          class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700"
        >
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <p class="text-sm font-medium text-gray-900 dark:text-white">{{ client.name }}</p>
              <span
                class="rounded-md px-1.5 py-0.5 text-xs font-medium"
                :class="client.status === 'active'
                  ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400'
                  : 'bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-dark-400'"
              >
                {{ client.status === 'active' ? t('admin.settings.oauthClients.active') : t('admin.settings.oauthClients.disabled') }}
              </span>
              <span class="rounded-md bg-dark-50 px-1.5 py-0.5 font-mono text-xs text-gray-500 dark:bg-dark-800 dark:text-dark-400">
                {{ client.client_id }}
              </span>
            </div>
            <p v-if="client.description" class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ client.description }}
            </p>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <span
                v-for="scope in client.allowed_scopes ?? []"
                :key="scope"
                class="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300"
              >{{ scope }}</span>
            </div>
            <p class="mt-2 truncate font-mono text-xs text-gray-400 dark:text-dark-500">
              {{ (client.redirect_uris ?? []).join('  ·  ') }}
            </p>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">
              {{ t('admin.settings.oauthClients.updatedAt', { date: formatDate(client.updated_at) }) }}
            </p>
          </div>
          <div class="flex shrink-0 gap-2">
            <button type="button" class="rounded-lg border border-gray-200 px-3 py-1.5 text-xs font-medium text-gray-600 transition hover:bg-gray-50 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800" @click="openEdit(client)">
              {{ t('admin.settings.oauthClients.edit') }}
            </button>
            <button
              type="button"
              class="rounded-lg border border-red-200 px-3 py-1.5 text-xs font-medium text-red-600 transition hover:bg-red-50 dark:border-red-900/50 dark:text-red-400 dark:hover:bg-red-900/20"
              @click="confirmDelete(client)"
            >
              {{ t('admin.settings.oauthClients.delete') }}
            </button>
          </div>
        </li>
      </ul>

      <p class="mt-4 text-xs text-gray-400 dark:text-dark-500">
        {{ t('admin.settings.oauthClients.pkceNote') }}
      </p>
    </div>

    <!-- Create / edit dialog -->
    <div v-if="dialogOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4" @click.self="dialogOpen = false">
      <div class="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-xl bg-white p-6 shadow-xl dark:bg-dark-900">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ editing ? t('admin.settings.oauthClients.edit') : t('admin.settings.oauthClients.add') }}
        </h3>

        <div class="mt-4 space-y-4">
          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.clientId') }}</label>
            <input
              v-model="form.client_id"
              type="text"
              :disabled="editing"
              class="mt-1 w-full rounded-lg border border-gray-200 px-3 py-2 font-mono text-sm disabled:opacity-60 dark:border-dark-700 dark:bg-dark-800 dark:text-white"
              :placeholder="editing ? '' : 'my-desktop-app'"
            />
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('admin.settings.oauthClients.clientIdHint') }}</p>
          </div>

          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.name') }}</label>
            <input
              v-model="form.name"
              type="text"
              class="mt-1 w-full rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-700 dark:bg-dark-800 dark:text-white"
            />
          </div>

          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.clientDescription') }}</label>
            <input
              v-model="form.description"
              type="text"
              class="mt-1 w-full rounded-lg border border-gray-200 px-3 py-2 text-sm dark:border-dark-700 dark:bg-dark-800 dark:text-white"
            />
          </div>

          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.logoUrl') }}</label>
            <input
              v-model="form.logo_url"
              type="text"
              class="mt-1 w-full rounded-lg border border-gray-200 px-3 py-2 font-mono text-sm dark:border-dark-700 dark:bg-dark-800 dark:text-white"
            />
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('admin.settings.oauthClients.logoUrlHint') }}</p>
          </div>

          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.redirectUris') }}</label>
            <textarea
              v-model="redirectUrisText"
              rows="4"
              wrap="off"
              class="mt-1 w-full overflow-x-auto rounded-lg border border-gray-200 px-3 py-2 font-mono text-sm whitespace-pre dark:border-dark-700 dark:bg-dark-800 dark:text-white"
            ></textarea>
            <p class="mt-1 text-xs text-gray-400 dark:text-dark-500">{{ t('admin.settings.oauthClients.redirectUrisHint') }}</p>
          </div>

          <div>
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.scopes') }}</label>
            <div class="mt-2 grid grid-cols-2 gap-2">
              <label
                v-for="scope in knownScopes"
                :key="scope"
                class="flex cursor-pointer items-center gap-2 text-sm text-gray-700 dark:text-dark-300"
              >
                <input v-model="form.allowed_scopes" type="checkbox" :value="scope" class="rounded" />
                <span class="font-mono text-xs">{{ scope }}</span>
              </label>
            </div>
          </div>

          <div class="flex items-center justify-between">
            <label class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.settings.oauthClients.status') }}</label>
            <Toggle v-model="active" />
          </div>
        </div>

        <p v-if="dialogError" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ dialogError }}</p>

        <div class="mt-6 flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="saving" @click="dialogOpen = false">
            {{ t('admin.settings.oauthClients.cancel') }}
          </button>
          <button type="button" class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? t('admin.settings.oauthClients.saving') : t('admin.settings.oauthClients.save') }}
          </button>
        </div>
      </div>
    </div>

    <!-- Delete confirmation -->
    <ConfirmDialog
      :show="deleteTarget !== null"
      :title="t('admin.settings.oauthClients.deleteTitle')"
      :message="t('admin.settings.oauthClients.deleteMessage', { clientId: deleteTarget?.client_id ?? '' })"
      :danger="true"
      @confirm="doDelete"
      @cancel="deleteTarget = null"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Toggle from '@/components/common/Toggle.vue'
import {
  deleteOAuthClient,
  listOAuthClients,
  upsertOAuthClient,
  type OAuthClientAdminView
} from '@/api/oauthProvider'

const { t } = useI18n()

// Keep in sync with the provider's scope registry (oauth_provider_service.go).
const knownScopes = ['openid', 'profile', 'groups:read', 'keys:read', 'keys:create', 'keys:revoke']

const clients = ref<OAuthClientAdminView[]>([])
const loading = ref(true)
const error = ref('')

const dialogOpen = ref(false)
const editing = ref(false)
const saving = ref(false)
const dialogError = ref('')
const form = ref({
  client_id: '',
  name: '',
  description: '',
  logo_url: '',
  allowed_scopes: [] as string[]
})
const redirectUrisText = ref('')
const active = ref(true)
const deleteTarget = ref<OAuthClientAdminView | null>(null)

const activeComputed = computed(() => (active.value ? 'active' : 'disabled'))

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    clients.value = await listOAuthClients()
  } catch (err: any) {
    error.value = err?.message || t('admin.settings.oauthClients.loadFailed')
  } finally {
    loading.value = false
  }
}

function openCreate(): void {
  editing.value = false
  form.value = { client_id: '', name: '', description: '', logo_url: '', allowed_scopes: ['openid', 'profile'] }
  redirectUrisText.value = ''
  active.value = true
  dialogError.value = ''
  dialogOpen.value = true
}

function openEdit(client: OAuthClientAdminView): void {
  editing.value = true
  form.value = {
    client_id: client.client_id,
    name: client.name,
    description: client.description,
    logo_url: client.logo_url,
    allowed_scopes: [...client.allowed_scopes]
  }
  redirectUrisText.value = client.redirect_uris.join('\n')
  active.value = client.status === 'active'
  dialogError.value = ''
  dialogOpen.value = true
}

function parseRedirectUris(): string[] {
  return redirectUrisText.value
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line !== '')
}

async function save(): Promise<void> {
  saving.value = true
  dialogError.value = ''
  try {
    const saved = await upsertOAuthClient(form.value.client_id.trim(), {
      name: form.value.name,
      description: form.value.description,
      logo_url: form.value.logo_url,
      redirect_uris: parseRedirectUris(),
      allowed_scopes: [...form.value.allowed_scopes],
      status: activeComputed.value
    })
    const index = clients.value.findIndex((item) => item.client_id === saved.client_id)
    if (index >= 0) {
      clients.value.splice(index, 1, saved)
    } else {
      clients.value.push(saved)
    }
    dialogOpen.value = false
  } catch (err: any) {
    dialogError.value = err?.message || t('admin.settings.oauthClients.saveFailed')
  } finally {
    saving.value = false
  }
}

function confirmDelete(client: OAuthClientAdminView): void {
  deleteTarget.value = client
}

async function doDelete(): Promise<void> {
  const target = deleteTarget.value
  if (!target) return
  error.value = ''
  try {
    await deleteOAuthClient(target.client_id)
    clients.value = clients.value.filter((item) => item.client_id !== target.client_id)
    deleteTarget.value = null
  } catch (err: any) {
    deleteTarget.value = null
    error.value = err?.message || t('admin.settings.oauthClients.deleteFailed')
  }
}

onMounted(load)
</script>
