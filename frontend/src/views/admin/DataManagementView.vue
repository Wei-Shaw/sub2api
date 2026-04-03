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
          <table class="w-full min-w-[1000px] text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.profileId') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.name') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.active') }}</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.storagePath') }}</th>
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
                    :class="profile.is_active ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'"
                  >
                    {{ profile.is_active ? t('common.enabled') : t('common.disabled') }}
                  </span>
                </td>
                <td class="py-3 pr-4 text-xs">
                  <div>{{ profile.endpoint || '-' }}</div>
                  <div class="mt-1 text-gray-500 dark:text-gray-400">{{ [profile.bucket, profile.prefix].filter(Boolean).join('/') || '-' }}</div>
                </td>
                <td class="py-3 pr-4 text-xs">{{ formatDate(profile.updated_at) }}</td>
                <td class="py-3 text-xs">
                  <div class="flex items-center gap-1">
                    <button
                      type="button"
                      class="btn btn-secondary btn-xs"
                      :disabled="testingProfiles[profile.profile_id]"
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
                <td colspan="6" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
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
            <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
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

              <!-- S3 fields -->
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

              <!-- Common bottom fields -->
              <input v-model="soraProfileForm.cdn_url" class="input w-full" :placeholder="t('admin.settings.soraS3.cdnUrl')" />
              <label v-if="creatingSoraProfile" class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
                <input v-model="soraProfileForm.set_active" type="checkbox" />
                <span>{{ t('admin.settings.soraS3.setActive') }}</span>
              </label>
            </div>
          </div>

          <div class="flex flex-wrap justify-end gap-2 border-t border-gray-200 p-4 dark:border-dark-700">
            <button type="button" class="btn btn-secondary btn-sm" @click="closeSoraProfileDrawer">
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="testingSoraProfile || !soraProfileForm.enabled"
              @click="testSoraProfileConnection"
            >
              {{ testingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.testConnection') }}
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
import type { SoraS3Profile } from '@/api/admin/settings'
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
const testingProfiles: Record<string, boolean> = reactive({})

const soraS3Profiles = ref<SoraS3Profile[]>([])
const selectedSoraProfileID = ref('')

type SoraStorageProfileForm = {
  profile_id: string
  name: string
  set_active: boolean
  enabled: boolean
  endpoint: string
  region: string
  bucket: string
  access_key_id: string
  secret_access_key: string
  secret_access_key_configured: boolean
  prefix: string
  force_path_style: boolean
  cdn_url: string
}

const soraProfileForm = ref<SoraStorageProfileForm>(newDefaultProfileForm())

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

  try {
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
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingProfiles[pid] = false
  }
}

function startCreateSoraProfile() {
  creatingSoraProfile.value = true
  selectedSoraProfileID.value = ''
  soraProfileForm.value = newDefaultProfileForm()
  soraProfileDrawerOpen.value = true
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
  // S3 validation
  if (form.enabled) {
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
      const created = await adminAPI.settings.createSoraS3Profile({
        profile_id: form.profile_id.trim(),
        name: form.name.trim(),
        set_active: form.set_active,
        enabled: form.enabled,
        endpoint: form.endpoint,
        region: form.region,
        bucket: form.bucket,
        access_key_id: form.access_key_id,
        secret_access_key: form.secret_access_key || undefined,
        prefix: form.prefix,
        force_path_style: form.force_path_style,
        cdn_url: form.cdn_url
      })
      selectedSoraProfileID.value = created.profile_id
      creatingSoraProfile.value = false
      soraProfileDrawerOpen.value = false
      appStore.showSuccess(t('admin.settings.soraS3.profileCreated'))
    } else {
      await adminAPI.settings.updateSoraS3Profile(selectedSoraProfileID.value, {
        name: form.name.trim(),
        enabled: form.enabled,
        endpoint: form.endpoint,
        region: form.region,
        bucket: form.bucket,
        access_key_id: form.access_key_id,
        secret_access_key: form.secret_access_key || undefined,
        prefix: form.prefix,
        force_path_style: form.force_path_style,
        cdn_url: form.cdn_url
      })
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

function pickPreferredSoraProfileID(): string {
  const active = soraS3Profiles.value.find((item) => item.is_active)
  if (active) return active.profile_id
  return soraS3Profiles.value[0]?.profile_id || ''
}

function syncSoraProfileFormWithSelection() {
  const profile = soraS3Profiles.value.find((item) => item.profile_id === selectedSoraProfileID.value)
  soraProfileForm.value = newDefaultProfileForm(profile)
}

function newDefaultProfileForm(profile?: SoraS3Profile): SoraStorageProfileForm {
  if (!profile) {
    return {
      profile_id: '',
      name: '',
      set_active: false,
      enabled: false,
      endpoint: '',
      region: '',
      bucket: '',
      access_key_id: '',
      secret_access_key: '',
      secret_access_key_configured: false,
      prefix: 'sora/',
      force_path_style: false,
      cdn_url: ''
    }
  }

  return {
    profile_id: profile.profile_id,
    name: profile.name,
    set_active: false,
    enabled: profile.enabled,
    endpoint: profile.endpoint || '',
    region: profile.region || '',
    bucket: profile.bucket || '',
    access_key_id: profile.access_key_id || '',
    secret_access_key: '',
    secret_access_key_configured: Boolean(profile.secret_access_key_configured),
    prefix: profile.prefix || '',
    force_path_style: Boolean(profile.force_path_style),
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
  transform: translateX(100%);
  opacity: 0;
}
</style>
