<template>
    <div class="space-y-6">
      <div class="card p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.soraS3.title') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.soraS3.description') }}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" @click="startCreateSoraProfile">
              {{ t('admin.settings.soraS3.newProfile') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingSoraProfiles" @click="loadSoraS3Profiles">
              {{ loadingSoraProfiles ? t('common.loading') : t('admin.settings.soraS3.reloadProfiles') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[1300px] text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.profileId') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.name') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.provider') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.active') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.storagePath') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.capacityUsage') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.videoCount') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.updatedAt') }}</th>
                <th class="py-2">{{ t('admin.settings.soraS3.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="profile in soraS3Profiles" :key="profile.profile_id" class="border-b border-gray-100 align-middle dark:border-dark-800">
                <td class="py-3 pr-4">
                  <div class="font-mono text-xs">{{ profile.profile_id }}</div>
                </td>
                <td class="py-3 pr-4 text-xs">{{ profile.name }}</td>
                <td class="py-3 pr-4">
                  <span
                    class="rounded px-2 py-0.5 text-xs"
                    :class="getProviderBadgeClass(profile.provider)"
                  >
                    {{ getProviderLabel(profile.provider) }}
                  </span>
                </td>
                <td class="py-3 pr-4">
                  <span
                    class="rounded px-2 py-0.5 text-xs"
                    :class="profile.is_active ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'"
                  >
                    {{ profile.is_active ? t('common.enabled') : t('common.disabled') }}
                  </span>
                </td>
                <td class="py-3 pr-4 text-xs">
                  <template v-if="(profile.provider || 's3') === 's3'">
                    <div>{{ profile.endpoint || '-' }}</div>
                    <div class="mt-1 text-gray-500 dark:text-gray-400">{{ [profile.bucket, profile.prefix].filter(Boolean).join('/') || '-' }}</div>
                  </template>
                  <template v-else>
                    <div>{{ profile.folder_id ? `folder: ${profile.folder_id}` : t('admin.settings.soraS3.columns.rootFolder') }}</div>
                  </template>
                </td>
                <td class="py-3 pr-4 text-xs">
                  <template v-if="(profile.provider || 's3') === 'gdrive'">
                    <template v-if="gdriveQuota">
                      <div>{{ formatBytes(gdriveQuota.used_bytes) }} / {{ formatBytes(gdriveQuota.limit_bytes) }}</div>
                    </template>
                    <template v-else-if="loadingGDriveQuota">
                      <div class="text-gray-400">{{ t('common.loading') }}</div>
                    </template>
                    <template v-else>
                      <div class="text-gray-400">-</div>
                    </template>
                  </template>
                  <template v-else>
                    <div class="text-gray-400">{{ t('admin.settings.soraS3.columns.capacityUnlimited') }}</div>
                  </template>
                </td>
                <td class="py-3 pr-4 text-xs">
                  <template v-if="videoStats && videoStats[profile.provider || 's3']">
                    <div>{{ videoStats[profile.provider || 's3'].completed }} {{ t('admin.settings.soraS3.columns.videoCompleted') }}</div>
                    <div v-if="videoStats[profile.provider || 's3'].in_progress > 0" class="mt-0.5 text-gray-500 dark:text-gray-400">
                      {{ videoStats[profile.provider || 's3'].in_progress }} {{ t('admin.settings.soraS3.columns.videoInProgress') }}
                    </div>
                  </template>
                  <template v-else-if="loadingVideoStats">
                    <div class="text-gray-400">{{ t('common.loading') }}</div>
                  </template>
                  <template v-else>
                    <div class="text-gray-400">-</div>
                  </template>
                </td>
                <td class="py-3 pr-4 text-xs">{{ formatDate(profile.updated_at) }}</td>
                <td class="py-3 text-xs">
                  <div class="flex items-center gap-1">
                    <button
                      type="button"
                      class="btn btn-secondary btn-xs"
                      :disabled="testingProfiles[profile.profile_id]
                        || ((profile.provider || 's3') === 'gdrive' && !profile.is_active)"
                      @click="testProfileInTable(profile)"
                    >
                      {{ testingProfiles[profile.profile_id] ? t('admin.settings.soraS3.columns.testingInTable') : t('admin.settings.soraS3.columns.testInTable') }}
                    </button>
                    <button type="button" class="btn btn-secondary btn-xs" @click="editSoraProfile(profile.profile_id)">
                      {{ t('common.edit') }}
                    </button>
                    <button
                      v-if="!profile.is_active"
                      type="button"
                      class="btn btn-secondary btn-xs"
                      :disabled="activatingSoraProfile"
                      @click="activateSoraProfile(profile.profile_id)"
                    >
                      {{ t('admin.settings.soraS3.activateProfile') }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-danger btn-xs"
                      :disabled="deletingSoraProfile"
                      @click="removeSoraProfile(profile.profile_id)"
                    >
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="soraS3Profiles.length === 0">
                <td colspan="9" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.soraS3.empty') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Profile drawer -->
    <Teleport to="body">
      <Transition name="dm-drawer-mask">
        <div
          v-if="soraProfileDrawerOpen"
          class="fixed inset-0 z-[54] bg-black/40 backdrop-blur-sm"
          @click="closeSoraProfileDrawer"
        ></div>
      </Transition>

      <Transition name="dm-drawer-panel">
        <div
          v-if="soraProfileDrawerOpen"
          class="fixed inset-y-0 right-0 z-[55] flex h-full w-full max-w-2xl flex-col border-l border-gray-200 bg-white shadow-2xl dark:border-dark-700 dark:bg-dark-900"
        >
          <div class="flex items-center justify-between border-b border-gray-200 px-4 py-3 dark:border-dark-700">
            <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
              {{ creatingSoraProfile ? t('admin.settings.soraS3.createTitle') : t('admin.settings.soraS3.editTitle') }}
              <span v-if="soraProfileForm.provider" class="ml-2 rounded bg-gray-100 px-2 py-0.5 text-xs font-normal text-gray-600 dark:bg-dark-800 dark:text-gray-400">
                {{ getProviderLabel(soraProfileForm.provider) }}
              </span>
            </h4>
            <button
              type="button"
              class="rounded p-1 text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-gray-400 dark:hover:bg-dark-800 dark:hover:text-gray-200"
              @click="closeSoraProfileDrawer"
            >
              ✕
            </button>
          </div>

          <div class="flex-1 overflow-y-auto p-4">
            <!-- Provider selection (only when creating and not yet chosen) -->
            <div v-if="creatingSoraProfile && !soraProfileForm.provider" class="flex flex-col items-center justify-center gap-4 py-12">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('admin.settings.soraS3.selectProvider') }}
              </h4>
              <div class="grid w-full max-w-sm grid-cols-2 gap-3">
                <button
                  type="button"
                  class="flex flex-col items-center gap-2 rounded-lg border-2 border-gray-200 p-4 transition hover:border-blue-400 hover:bg-blue-50 dark:border-dark-600 dark:hover:border-blue-500 dark:hover:bg-dark-800"
                  @click="selectProvider('s3')"
                >
                  <span class="text-2xl">S3</span>
                  <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.settings.soraS3.providerS3Desc') }}</span>
                </button>
                <button
                  type="button"
                  class="flex flex-col items-center gap-2 rounded-lg border-2 border-gray-200 p-4 transition hover:border-blue-400 hover:bg-blue-50 dark:border-dark-600 dark:hover:border-blue-500 dark:hover:bg-dark-800"
                  @click="selectProvider('gdrive')"
                >
                  <span class="text-2xl">GDrive</span>
                  <span class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.settings.soraS3.providerGDriveDesc') }}</span>
                </button>
              </div>
            </div>

            <!-- Profile form (shown after provider is selected) -->
            <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2">
              <!-- Common fields -->
              <input
                v-model="soraProfileForm.profile_id"
                class="input w-full"
                :placeholder="t('admin.settings.soraS3.profileID')"
                :disabled="!creatingSoraProfile"
              />
              <input
                v-model="soraProfileForm.name"
                class="input w-full"
                :placeholder="t('admin.settings.soraS3.profileName')"
              />
              <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
                <input v-model="soraProfileForm.enabled" type="checkbox" />
                <span>{{ t('admin.settings.soraS3.enabled') }}</span>
              </label>

              <!-- S3-specific fields -->
              <template v-if="soraProfileForm.provider === 's3'">
                <input v-model="soraProfileForm.endpoint" class="input w-full" :placeholder="t('admin.settings.soraS3.endpoint')" />
                <input v-model="soraProfileForm.region" class="input w-full" :placeholder="t('admin.settings.soraS3.region')" />
                <input v-model="soraProfileForm.bucket" class="input w-full" :placeholder="t('admin.settings.soraS3.bucket')" />
                <input v-model="soraProfileForm.prefix" class="input w-full" :placeholder="t('admin.settings.soraS3.prefix')" />
                <input v-model="soraProfileForm.access_key_id" class="input w-full" :placeholder="t('admin.settings.soraS3.accessKeyId')" />
                <input
                  v-model="soraProfileForm.secret_access_key"
                  type="password"
                  class="input w-full"
                  :placeholder="soraProfileForm.secret_access_key_configured ? t('admin.settings.soraS3.secretConfigured') : t('admin.settings.soraS3.secretAccessKey')"
                />
                <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input v-model="soraProfileForm.force_path_style" type="checkbox" />
                  <span>{{ t('admin.settings.soraS3.forcePathStyle') }}</span>
                </label>
              </template>

              <!-- GDrive-specific fields -->
              <template v-if="soraProfileForm.provider === 'gdrive'">
                <div class="md:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-700 dark:text-gray-300">
                    {{ t('admin.settings.soraS3.gdrive.authType') }}
                  </label>
                  <select v-model="soraProfileForm.auth_type" class="input w-full">
                    <option value="oauth2">OAuth2</option>
                    <option value="service_account">{{ t('admin.settings.soraS3.gdrive.serviceAccount') }}</option>
                  </select>
                </div>

                <template v-if="soraProfileForm.auth_type === 'oauth2'">
                  <input v-model="soraProfileForm.client_id" class="input w-full" :placeholder="t('admin.settings.soraS3.gdrive.clientId')" />
                  <input
                    v-model="soraProfileForm.client_secret"
                    type="password"
                    class="input w-full"
                    :placeholder="soraProfileForm.client_secret_configured ? t('admin.settings.soraS3.gdrive.clientSecretConfigured') : t('admin.settings.soraS3.gdrive.clientSecret')"
                  />
                  <div class="md:col-span-2">
                    <input
                      v-model="soraProfileForm.refresh_token"
                      type="password"
                      class="input w-full"
                      :placeholder="soraProfileForm.refresh_token_configured ? t('admin.settings.soraS3.gdrive.refreshTokenConfigured') : t('admin.settings.soraS3.gdrive.refreshToken')"
                    />
                    <div class="mt-2 flex items-center gap-2">
                      <button
                        type="button"
                        class="btn btn-secondary btn-xs"
                        :disabled="!soraProfileForm.client_id || !soraProfileForm.client_secret || startingOAuth"
                        @click="startGDriveOAuth"
                      >
                        {{ startingOAuth ? t('common.loading') : t('admin.settings.soraS3.gdrive.authorize') }}
                      </button>
                      <p class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.soraS3.gdrive.authorizeHint') }}</p>
                    </div>
                  </div>
                </template>

                <template v-if="soraProfileForm.auth_type === 'service_account'">
                  <div class="md:col-span-2">
                    <textarea
                      v-model="soraProfileForm.service_account_json"
                      class="input w-full"
                      rows="6"
                      :placeholder="soraProfileForm.service_account_configured ? t('admin.settings.soraS3.gdrive.serviceAccountConfigured') : t('admin.settings.soraS3.gdrive.serviceAccountJson')"
                    ></textarea>
                  </div>
                </template>

                <input v-model="soraProfileForm.folder_id" class="input w-full md:col-span-2" :placeholder="t('admin.settings.soraS3.gdrive.folderId')" />
              </template>

              <!-- Common bottom fields -->
              <input v-model="soraProfileForm.cdn_url" class="input w-full" :placeholder="t('admin.settings.soraS3.cdnUrl')" />
              <label v-if="creatingSoraProfile" class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
                <input v-model="soraProfileForm.set_active" type="checkbox" />
                <span>{{ t('admin.settings.soraS3.setActive') }}</span>
              </label>
            </div>
          </div>

          <div v-if="soraProfileForm.provider" class="flex flex-wrap justify-end gap-2 border-t border-gray-200 p-4 dark:border-dark-700">
            <button type="button" class="btn btn-secondary btn-sm" @click="closeSoraProfileDrawer">
              {{ t('common.cancel') }}
            </button>
            <button
              v-if="soraProfileForm.provider === 's3'"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="testingSoraProfile || !soraProfileForm.enabled"
              @click="testSoraProfileConnection"
            >
              {{ testingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.testConnection') }}
            </button>
            <button
              v-if="soraProfileForm.provider === 'gdrive' && !creatingSoraProfile"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="testingSoraProfile || !soraProfileForm.enabled"
              @click="testGDriveStorageConnection"
            >
              {{ testingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.gdrive.testStorage') }}
            </button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="savingSoraProfile" @click="saveSoraProfile">
              {{ savingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.saveProfile') }}
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import type { SoraS3Profile, GDriveQuotaInfo, StorageVideoStats } from '@/api/admin/settings'
import { adminAPI } from '@/api'
import { useAppStore } from '@/stores'

const { t } = useI18n()
const appStore = useAppStore()

const loadingSoraProfiles = ref(false)
const savingSoraProfile = ref(false)
const testingSoraProfile = ref(false)
const activatingSoraProfile = ref(false)
const deletingSoraProfile = ref(false)
const creatingSoraProfile = ref(false)
const soraProfileDrawerOpen = ref(false)
const startingOAuth = ref(false)
const loadingGDriveQuota = ref(false)
const loadingVideoStats = ref(false)
const testingProfiles: Record<string, boolean> = reactive({})
const gdriveQuota = ref<GDriveQuotaInfo | null>(null)
const videoStats = ref<StorageVideoStats | null>(null)

const soraS3Profiles = ref<SoraS3Profile[]>([])
const selectedSoraProfileID = ref('')

type SoraStorageProfileForm = {
  profile_id: string
  name: string
  set_active: boolean
  provider: string
  enabled: boolean
  // S3 fields
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key: string
  secret_access_key_configured: boolean
  prefix: string
  force_path_style: boolean
  // GDrive fields
  auth_type: string
  client_id: string
  client_secret: string
  client_secret_configured: boolean
  refresh_token: string
  refresh_token_configured: boolean
  service_account_json: string
  service_account_configured: boolean
  folder_id: string
  // Common
  cdn_url: string
}

const soraProfileForm = ref<SoraStorageProfileForm>(newDefaultProfileForm('s3'))

function getProviderLabel(provider?: string): string {
  return (provider || 's3') === 'gdrive' ? 'Google Drive' : 'S3'
}

function getProviderBadgeClass(provider?: string): string {
  return (provider || 's3') === 'gdrive'
    ? 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    : 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-300'
}

async function loadSoraS3Profiles() {
  loadingSoraProfiles.value = true
  try {
    const result = await adminAPI.settings.listSoraS3Profiles()
    soraS3Profiles.value = result.items || []
    if (!creatingSoraProfile.value) {
      const stillExists = selectedSoraProfileID.value
        ? soraS3Profiles.value.some((item) => item.profile_id === selectedSoraProfileID.value)
        : false
      if (!stillExists) {
        selectedSoraProfileID.value = pickPreferredSoraProfileID()
      }
      syncSoraProfileFormWithSelection()
    }
    // Async load quota and video stats (non-blocking)
    loadGDriveQuotaIfNeeded()
    loadVideoStats()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    loadingSoraProfiles.value = false
  }
}

const testProfileTimeoutMs = 15000

async function testProfileInTable(profile: SoraS3Profile) {
  const pid = profile.profile_id
  if (testingProfiles[pid]) return

  testingProfiles[pid] = true
  const provider = profile.provider || 's3'

  try {
    if (provider === 'gdrive') {
      const result = await Promise.race([
        adminAPI.settings.testGDriveStorage(),
        new Promise<never>((_, reject) => setTimeout(() => reject(new Error(t('admin.settings.soraS3.columns.testTimeout'))), testProfileTimeoutMs))
      ])
      const msg = result.status === 'ok'
        ? t('admin.settings.soraS3.gdrive.testSuccess')
        : t('admin.settings.soraS3.gdrive.testFailed')
      appStore.showSuccess(msg)
    } else {
      await Promise.race([
        adminAPI.settings.testSoraS3Connection({
          profile_id: pid,
          enabled: profile.enabled,
          endpoint: profile.endpoint || '',
          region: profile.region || '',
          bucket: profile.bucket || '',
          access_key_id: profile.access_key_id || '',
          prefix: profile.prefix || '',
          force_path_style: Boolean(profile.force_path_style),
          cdn_url: profile.cdn_url || '',
          default_storage_quota_bytes: profile.default_storage_quota_bytes || 0
        }),
        new Promise<never>((_, reject) => setTimeout(() => reject(new Error(t('admin.settings.soraS3.columns.testTimeout'))), testProfileTimeoutMs))
      ])
      appStore.showSuccess(t('admin.settings.soraS3.testSuccess'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingProfiles[pid] = false
  }
}

async function loadGDriveQuotaIfNeeded() {
  const hasGDrive = soraS3Profiles.value.some(
    (p) => (p.provider || 's3') === 'gdrive' && p.is_active
  )
  if (!hasGDrive) {
    gdriveQuota.value = null
    return
  }
  loadingGDriveQuota.value = true
  try {
    gdriveQuota.value = await adminAPI.settings.getGDriveQuota()
  } catch {
    gdriveQuota.value = null
  } finally {
    loadingGDriveQuota.value = false
  }
}

async function loadVideoStats() {
  loadingVideoStats.value = true
  try {
    videoStats.value = await adminAPI.settings.getStorageVideoStats()
  } catch {
    videoStats.value = null
  } finally {
    loadingVideoStats.value = false
  }
}

function startCreateSoraProfile() {
  creatingSoraProfile.value = true
  selectedSoraProfileID.value = ''
  soraProfileForm.value = newDefaultProfileForm('')
  soraProfileDrawerOpen.value = true
}

function selectProvider(provider: string) {
  soraProfileForm.value = newDefaultProfileForm(provider)
}

function editSoraProfile(profileID: string) {
  selectedSoraProfileID.value = profileID
  creatingSoraProfile.value = false
  syncSoraProfileFormWithSelection()
  soraProfileDrawerOpen.value = true
}

function closeSoraProfileDrawer() {
  soraProfileDrawerOpen.value = false
  if (creatingSoraProfile.value) {
    creatingSoraProfile.value = false
    selectedSoraProfileID.value = pickPreferredSoraProfileID()
    syncSoraProfileFormWithSelection()
  }
}

async function saveSoraProfile() {
  const form = soraProfileForm.value
  if (!form.name.trim()) {
    appStore.showError(t('admin.settings.soraS3.profileNameRequired'))
    return
  }
  if (creatingSoraProfile.value && !form.profile_id.trim()) {
    appStore.showError(t('admin.settings.soraS3.profileIDRequired'))
    return
  }
  if (!creatingSoraProfile.value && !selectedSoraProfileID.value) {
    appStore.showError(t('admin.settings.soraS3.profileSelectRequired'))
    return
  }
  // S3-specific validation
  if (form.provider === 's3' && form.enabled) {
    if (!form.endpoint.trim()) {
      appStore.showError(t('admin.settings.soraS3.endpointRequired'))
      return
    }
    if (!form.bucket.trim()) {
      appStore.showError(t('admin.settings.soraS3.bucketRequired'))
      return
    }
    if (!form.access_key_id.trim()) {
      appStore.showError(t('admin.settings.soraS3.accessKeyRequired'))
      return
    }
  }

  savingSoraProfile.value = true
  try {
    if (creatingSoraProfile.value) {
      const request: Record<string, unknown> = {
        profile_id: form.profile_id.trim(),
        name: form.name.trim(),
        set_active: form.set_active,
        provider: form.provider,
        enabled: form.enabled,
        cdn_url: form.cdn_url
      }
      if (form.provider === 's3') {
        Object.assign(request, {
          endpoint: form.endpoint,
          region: form.region,
          bucket: form.bucket,
          access_key_id: form.access_key_id,
          secret_access_key: form.secret_access_key || undefined,
          prefix: form.prefix,
          force_path_style: form.force_path_style
        })
      } else {
        Object.assign(request, {
          auth_type: form.auth_type,
          client_id: form.client_id,
          client_secret: form.client_secret || undefined,
          refresh_token: form.refresh_token || undefined,
          service_account_json: form.service_account_json || undefined,
          folder_id: form.folder_id
        })
      }
      const created = await adminAPI.settings.createSoraS3Profile(request as never)
      selectedSoraProfileID.value = created.profile_id
      creatingSoraProfile.value = false
      soraProfileDrawerOpen.value = false
      appStore.showSuccess(t('admin.settings.soraS3.profileCreated'))
    } else {
      const request: Record<string, unknown> = {
        name: form.name.trim(),
        enabled: form.enabled,
        cdn_url: form.cdn_url
      }
      if (form.provider === 's3') {
        Object.assign(request, {
          endpoint: form.endpoint,
          region: form.region,
          bucket: form.bucket,
          access_key_id: form.access_key_id,
          secret_access_key: form.secret_access_key || undefined,
          prefix: form.prefix,
          force_path_style: form.force_path_style
        })
      } else {
        Object.assign(request, {
          auth_type: form.auth_type,
          client_id: form.client_id,
          client_secret: form.client_secret || undefined,
          refresh_token: form.refresh_token || undefined,
          service_account_json: form.service_account_json || undefined,
          folder_id: form.folder_id
        })
      }
      await adminAPI.settings.updateSoraS3Profile(selectedSoraProfileID.value, request as never)
      soraProfileDrawerOpen.value = false
      appStore.showSuccess(t('admin.settings.soraS3.profileSaved'))
    }
    await loadSoraS3Profiles()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingSoraProfile.value = false
  }
}

async function testSoraProfileConnection() {
  testingSoraProfile.value = true
  try {
    const result = await adminAPI.settings.testSoraS3Connection({
      profile_id: creatingSoraProfile.value ? undefined : selectedSoraProfileID.value,
      enabled: soraProfileForm.value.enabled,
      endpoint: soraProfileForm.value.endpoint,
      region: soraProfileForm.value.region,
      bucket: soraProfileForm.value.bucket,
      access_key_id: soraProfileForm.value.access_key_id,
      secret_access_key: soraProfileForm.value.secret_access_key || undefined,
      prefix: soraProfileForm.value.prefix,
      force_path_style: soraProfileForm.value.force_path_style,
      cdn_url: soraProfileForm.value.cdn_url
    })
    appStore.showSuccess(result.message || t('admin.settings.soraS3.testSuccess'))
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingSoraProfile.value = false
  }
}

async function testGDriveStorageConnection() {
  testingSoraProfile.value = true
  try {
    const result = await adminAPI.settings.testGDriveStorage()
    const msg = result.status === 'ok'
      ? t('admin.settings.soraS3.gdrive.testSuccess')
      : t('admin.settings.soraS3.gdrive.testFailed')
    appStore.showSuccess(msg)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('admin.settings.soraS3.gdrive.testFailed'))
  } finally {
    testingSoraProfile.value = false
  }
}

async function startGDriveOAuth() {
  const form = soraProfileForm.value
  if (!form.client_id || !form.client_secret) {
    appStore.showError(t('admin.settings.soraS3.gdrive.oauthFieldsRequired'))
    return
  }

  startingOAuth.value = true
  try {
    const redirectUri = `${window.location.origin}/admin/gdrive-oauth-callback`
    const result = await adminAPI.settings.startGDriveOAuth({
      client_id: form.client_id,
      client_secret: form.client_secret,
      redirect_uri: redirectUri
    })

    // Store state for callback verification
    sessionStorage.setItem('gdrive_oauth_state', result.state)
    sessionStorage.setItem('gdrive_oauth_client_id', form.client_id)
    sessionStorage.setItem('gdrive_oauth_client_secret', form.client_secret)
    sessionStorage.setItem('gdrive_oauth_redirect_uri', redirectUri)
    sessionStorage.setItem('gdrive_oauth_profile_id', creatingSoraProfile.value ? '' : selectedSoraProfileID.value)

    // Open Google auth in new window
    const authWindow = window.open(result.auth_url, 'gdrive_oauth', 'width=600,height=700')

    // Listen for callback message
    const handleMessage = async (event: MessageEvent) => {
      if (event.data?.type !== 'gdrive_oauth_callback') return
      window.removeEventListener('message', handleMessage)
      if (authWindow) authWindow.close()

      const code = event.data.code
      if (!code) {
        appStore.showError(t('admin.settings.soraS3.gdrive.oauthFailed'))
        return
      }

      try {
        const exchangeResult = await adminAPI.settings.exchangeGDriveOAuthCode({
          client_id: form.client_id,
          client_secret: form.client_secret,
          redirect_uri: redirectUri,
          code,
          profile_id: creatingSoraProfile.value ? undefined : selectedSoraProfileID.value
        })
        form.refresh_token = exchangeResult.refresh_token
        form.refresh_token_configured = true
        appStore.showSuccess(exchangeResult.message || t('admin.settings.soraS3.gdrive.oauthSuccess'))
      } catch (err) {
        appStore.showError((err as { message?: string })?.message || t('admin.settings.soraS3.gdrive.oauthFailed'))
      }
    }
    window.addEventListener('message', handleMessage)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    startingOAuth.value = false
  }
}

async function activateSoraProfile(profileID: string) {
  activatingSoraProfile.value = true
  try {
    await adminAPI.settings.setActiveSoraS3Profile(profileID)
    appStore.showSuccess(t('admin.settings.soraS3.profileActivated'))
    await loadSoraS3Profiles()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    activatingSoraProfile.value = false
  }
}

async function removeSoraProfile(profileID: string) {
  if (!window.confirm(t('admin.settings.soraS3.deleteConfirm', { profileID }))) {
    return
  }
  deletingSoraProfile.value = true
  try {
    await adminAPI.settings.deleteSoraS3Profile(profileID)
    if (selectedSoraProfileID.value === profileID) {
      selectedSoraProfileID.value = ''
    }
    appStore.showSuccess(t('admin.settings.soraS3.profileDeleted'))
    await loadSoraS3Profiles()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    deletingSoraProfile.value = false
  }
}

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let idx = 0
  let val = bytes
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024
    idx++
  }
  return `${val.toFixed(idx === 0 ? 0 : 1)} ${units[idx]}`
}

function pickPreferredSoraProfileID(): string {
  const active = soraS3Profiles.value.find((item) => item.is_active)
  if (active) return active.profile_id
  return soraS3Profiles.value[0]?.profile_id || ''
}

function syncSoraProfileFormWithSelection() {
  const profile = soraS3Profiles.value.find((item) => item.profile_id === selectedSoraProfileID.value)
  soraProfileForm.value = newDefaultProfileForm(profile?.provider || 's3', profile)
}

function newDefaultProfileForm(provider: string, profile?: SoraS3Profile): SoraStorageProfileForm {
  if (!profile) {
    return {
      profile_id: '',
      name: '',
      set_active: false,
      provider,
      enabled: false,
      endpoint: '',
      region: '',
      bucket: '',
      access_key_id: '',
      secret_access_key: '',
      secret_access_key_configured: false,
      prefix: provider === 's3' ? 'sora/' : '',
      force_path_style: false,
      auth_type: 'oauth2',
      client_id: '',
      client_secret: '',
      client_secret_configured: false,
      refresh_token: '',
      refresh_token_configured: false,
      service_account_json: '',
      service_account_configured: false,
      folder_id: '',
      cdn_url: ''
    }
  }

  return {
    profile_id: profile.profile_id,
    name: profile.name,
    set_active: false,
    provider: profile.provider || 's3',
    enabled: profile.enabled,
    endpoint: profile.endpoint || '',
    region: profile.region || '',
    bucket: profile.bucket || '',
    access_key_id: profile.access_key_id || '',
    secret_access_key: '',
    secret_access_key_configured: Boolean(profile.secret_access_key_configured),
    prefix: profile.prefix || '',
    force_path_style: Boolean(profile.force_path_style),
    auth_type: profile.auth_type || 'oauth2',
    client_id: profile.client_id || '',
    client_secret: '',
    client_secret_configured: Boolean(profile.client_secret_configured),
    refresh_token: '',
    refresh_token_configured: Boolean(profile.refresh_token_configured),
    service_account_json: '',
    service_account_configured: Boolean(profile.service_account_configured),
    folder_id: profile.folder_id || '',
    cdn_url: profile.cdn_url || ''
  }
}

onMounted(async () => {
  await loadSoraS3Profiles()
})
</script>

<style scoped>
.dm-drawer-mask-enter-active,
.dm-drawer-mask-leave-active {
  transition: opacity 0.2s ease;
}

.dm-drawer-mask-enter-from,
.dm-drawer-mask-leave-to {
  opacity: 0;
}

.dm-drawer-panel-enter-active,
.dm-drawer-panel-leave-active {
  transition:
    transform 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.2s ease;
}

.dm-drawer-panel-enter-from,
.dm-drawer-panel-leave-to {
  opacity: 0.96;
  transform: translateX(100%);
}

@media (prefers-reduced-motion: reduce) {
  .dm-drawer-mask-enter-active,
  .dm-drawer-mask-leave-active,
  .dm-drawer-panel-enter-active,
  .dm-drawer-panel-leave-active {
    transition-duration: 0s;
  }
}
</style>
