<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="card p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.settings.soraS3.title') REDACTEDREDACTED
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.soraS3.description') REDACTEDREDACTED
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            <button type="button" class="btn btn-secondary btn-sm" @click="startCreateSoraProfile">
              {{ t('admin.settings.soraS3.newProfile') REDACTEDREDACTED
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingSoraProfiles" @click="loadSoraS3Profiles">
              {{ loadingSoraProfiles ? t('common.loading') : t('admin.settings.soraS3.reloadProfiles') REDACTEDREDACTED
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[1000px] text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.profile') REDACTEDREDACTED</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.active') REDACTEDREDACTED</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.endpoint') REDACTEDREDACTED</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.bucket') REDACTEDREDACTED</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.quota') REDACTEDREDACTED</th>
                <th class="py-2 pr-4">{{ t('admin.settings.soraS3.columns.updatedAt') REDACTEDREDACTED</th>
                <th class="py-2">{{ t('admin.settings.soraS3.columns.actions') REDACTEDREDACTED</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="profile in soraS3Profiles" :key="profile.profile_id" class="border-b border-gray-100 align-top dark:border-dark-800">
                <td class="py-3 pr-4">
                  <div class="font-mono text-xs">{{ profile.profile_id REDACTEDREDACTED</div>
                  <div class="mt-1 text-xs text-gray-600 dark:text-gray-400">{{ profile.name REDACTEDREDACTED</div>
                </td>
                <td class="py-3 pr-4">
                  <span
                    class="rounded px-2 py-0.5 text-xs"
                    :class="profile.is_active ? 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300' : 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'"
                  >
                    {{ profile.is_active ? t('common.enabled') : t('common.disabled') REDACTEDREDACTED
                  </span>
                </td>
                <td class="py-3 pr-4 text-xs">
                  <div>{{ profile.endpoint || '-' REDACTEDREDACTED</div>
                  <div class="mt-1 text-gray-500 dark:text-gray-400">{{ profile.region || '-' REDACTEDREDACTED</div>
                </td>
                <td class="py-3 pr-4 text-xs">{{ profile.bucket || '-' REDACTEDREDACTED</td>
                <td class="py-3 pr-4 text-xs">{{ formatStorageQuotaGB(profile.default_storage_quota_bytes) REDACTEDREDACTED</td>
                <td class="py-3 pr-4 text-xs">{{ formatDate(profile.updated_at) REDACTEDREDACTED</td>
                <td class="py-3 text-xs">
                  <div class="flex flex-wrap gap-2">
                    <button type="button" class="btn btn-secondary btn-xs" @click="editSoraProfile(profile.profile_id)">
                      {{ t('common.edit') REDACTEDREDACTED
                    </button>
                    <button
                      v-if="!profile.is_active"
                      type="button"
                      class="btn btn-secondary btn-xs"
                      :disabled="activatingSoraProfile"
                      @click="activateSoraProfile(profile.profile_id)"
                    >
                      {{ t('admin.settings.soraS3.activateProfile') REDACTEDREDACTED
                    </button>
                    <button
                      type="button"
                      class="btn btn-danger btn-xs"
                      :disabled="deletingSoraProfile"
                      @click="removeSoraProfile(profile.profile_id)"
                    >
                      {{ t('common.delete') REDACTEDREDACTED
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="soraS3Profiles.length === 0">
                <td colspan="7" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.settings.soraS3.empty') REDACTEDREDACTED
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

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
              {{ creatingSoraProfile ? t('admin.settings.soraS3.createTitle') : t('admin.settings.soraS3.editTitle') REDACTEDREDACTED
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
                <span>{{ t('admin.settings.soraS3.enabled') REDACTEDREDACTED</span>
              </label>
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
              <input v-model="soraProfileForm.cdn_url" class="input w-full" :placeholder="t('admin.settings.soraS3.cdnUrl')" />
              <div>
                <input
                  v-model.number="soraProfileForm.default_storage_quota_gb"
                  type="number"
                  min="0"
                  step="0.1"
                  class="input w-full"
                  :placeholder="t('admin.settings.soraS3.defaultQuota')"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.settings.soraS3.defaultQuotaHint') REDACTEDREDACTED</p>
              </div>
              <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                <input v-model="soraProfileForm.force_path_style" type="checkbox" />
                <span>{{ t('admin.settings.soraS3.forcePathStyle') REDACTEDREDACTED</span>
              </label>
              <label v-if="creatingSoraProfile" class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
                <input v-model="soraProfileForm.set_active" type="checkbox" />
                <span>{{ t('admin.settings.soraS3.setActive') REDACTEDREDACTED</span>
              </label>
            </div>
          </div>

          <div class="flex flex-wrap justify-end gap-2 border-t border-gray-200 p-4 dark:border-dark-700">
            <button type="button" class="btn btn-secondary btn-sm" @click="closeSoraProfileDrawer">
              {{ t('common.cancel') REDACTEDREDACTED
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="testingSoraProfile || !soraProfileForm.enabled" @click="testSoraProfileConnection">
              {{ testingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.testConnection') REDACTEDREDACTED
            </button>
            <button type="button" class="btn btn-primary btn-sm" :disabled="savingSoraProfile" @click="saveSoraProfile">
              {{ savingSoraProfile ? t('common.loading') : t('admin.settings.soraS3.saveProfile') REDACTEDREDACTED
            </button>
          </div>
        </div>
      </Transition>
    </Teleport>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { SoraS3Profile REDACTED from '@/api/admin/settings'
import { adminAPI REDACTED from '@/api'
import { useAppStore REDACTED from '@/stores'

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loadingSoraProfiles = ref(false)
const savingSoraProfile = ref(false)
const testingSoraProfile = ref(false)
const activatingSoraProfile = ref(false)
const deletingSoraProfile = ref(false)
const creatingSoraProfile = ref(false)
const soraProfileDrawerOpen = ref(false)

const soraS3Profiles = ref<SoraS3Profile[]>([])
const selectedSoraProfileID = ref('')

type SoraS3ProfileForm = {
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
  default_storage_quota_gb: number
REDACTED

const soraProfileForm = ref<SoraS3ProfileForm>(newDefaultSoraS3ProfileForm())

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
      REDACTED
      syncSoraProfileFormWithSelection()
    REDACTED
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED)?.message || t('errors.networkError'))
  REDACTED finally {
    loadingSoraProfiles.value = false
  REDACTED
REDACTED

function startCreateSoraProfile() {
  creatingSoraProfile.value = true
  selectedSoraProfileID.value = ''
  soraProfileForm.value = newDefaultSoraS3ProfileForm()
  soraProfileDrawerOpen.value = true
REDACTED

function editSoraProfile(profileID: string) {
  selectedSoraProfileID.value = profileID
  creatingSoraProfile.value = false
  syncSoraProfileFormWithSelection()
  soraProfileDrawerOpen.value = true
REDACTED

function closeSoraProfileDrawer() {
  soraProfileDrawerOpen.value = false
  if (creatingSoraProfile.value) {
    creatingSoraProfile.value = false
    selectedSoraProfileID.value = pickPreferredSoraProfileID()
    syncSoraProfileFormWithSelection()
  REDACTED
REDACTED

async function saveSoraProfile() {
  if (!soraProfileForm.value.name.trim()) {
    appStore.showError(t('admin.settings.soraS3.profileNameRequired'))
    return
  REDACTED
  if (creatingSoraProfile.value && !soraProfileForm.value.profile_id.trim()) {
    appStore.showError(t('admin.settings.soraS3.profileIDRequired'))
    return
  REDACTED
  if (!creatingSoraProfile.value && !selectedSoraProfileID.value) {
    appStore.showError(t('admin.settings.soraS3.profileSelectRequired'))
    return
  REDACTED
  if (soraProfileForm.value.enabled) {
    if (!soraProfileForm.value.endpoint.trim()) {
      appStore.showError(t('admin.settings.soraS3.endpointRequired'))
      return
    REDACTED
    if (!soraProfileForm.value.bucket.trim()) {
      appStore.showError(t('admin.settings.soraS3.bucketRequired'))
      return
    REDACTED
    if (!soraProfileForm.value.access_key_id.trim()) {
      appStore.showError(t('admin.settings.soraS3.accessKeyRequired'))
      return
    REDACTED
  REDACTED

  savingSoraProfile.value = true
  try {
    if (creatingSoraProfile.value) {
      const created = await adminAPI.settings.createSoraS3Profile({
        profile_id: soraProfileForm.value.profile_id.trim(),
        name: soraProfileForm.value.name.trim(),
        set_active: soraProfileForm.value.set_active,
        enabled: soraProfileForm.value.enabled,
        endpoint: soraProfileForm.value.endpoint,
        region: soraProfileForm.value.region,
        bucket: soraProfileForm.value.bucket,
        access_key_id: soraProfileForm.value.access_key_id,
        secret_access_key: soraProfileForm.value.secret_access_key || undefined,
        prefix: soraProfileForm.value.prefix,
        force_path_style: soraProfileForm.value.force_path_style,
        cdn_url: soraProfileForm.value.cdn_url,
        default_storage_quota_bytes: Math.round((soraProfileForm.value.default_storage_quota_gb || 0) * 1024 * 1024 * 1024)
      REDACTED)
      selectedSoraProfileID.value = created.profile_id
      creatingSoraProfile.value = false
      soraProfileDrawerOpen.value = false
      appStore.showSuccess(t('admin.settings.soraS3.profileCreated'))
    REDACTED else {
      await adminAPI.settings.updateSoraS3Profile(selectedSoraProfileID.value, {
        name: soraProfileForm.value.name.trim(),
        enabled: soraProfileForm.value.enabled,
        endpoint: soraProfileForm.value.endpoint,
        region: soraProfileForm.value.region,
        bucket: soraProfileForm.value.bucket,
        access_key_id: soraProfileForm.value.access_key_id,
        secret_access_key: soraProfileForm.value.secret_access_key || undefined,
        prefix: soraProfileForm.value.prefix,
        force_path_style: soraProfileForm.value.force_path_style,
        cdn_url: soraProfileForm.value.cdn_url,
        default_storage_quota_bytes: Math.round((soraProfileForm.value.default_storage_quota_gb || 0) * 1024 * 1024 * 1024)
      REDACTED)
      soraProfileDrawerOpen.value = false
      appStore.showSuccess(t('admin.settings.soraS3.profileSaved'))
    REDACTED
    await loadSoraS3Profiles()
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED)?.message || t('errors.networkError'))
  REDACTED finally {
    savingSoraProfile.value = false
  REDACTED
REDACTED

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
      cdn_url: soraProfileForm.value.cdn_url,
      default_storage_quota_bytes: Math.round((soraProfileForm.value.default_storage_quota_gb || 0) * 1024 * 1024 * 1024)
    REDACTED)
    appStore.showSuccess(result.message || t('admin.settings.soraS3.testSuccess'))
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED)?.message || t('errors.networkError'))
  REDACTED finally {
    testingSoraProfile.value = false
  REDACTED
REDACTED

async function activateSoraProfile(profileID: string) {
  activatingSoraProfile.value = true
  try {
    await adminAPI.settings.setActiveSoraS3Profile(profileID)
    appStore.showSuccess(t('admin.settings.soraS3.profileActivated'))
    await loadSoraS3Profiles()
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED)?.message || t('errors.networkError'))
  REDACTED finally {
    activatingSoraProfile.value = false
  REDACTED
REDACTED

async function removeSoraProfile(profileID: string) {
  if (!window.confirm(t('admin.settings.soraS3.deleteConfirm', { profileID REDACTED))) {
    return
  REDACTED
  deletingSoraProfile.value = true
  try {
    await adminAPI.settings.deleteSoraS3Profile(profileID)
    if (selectedSoraProfileID.value === profileID) {
      selectedSoraProfileID.value = ''
    REDACTED
    appStore.showSuccess(t('admin.settings.soraS3.profileDeleted'))
    await loadSoraS3Profiles()
  REDACTED catch (error) {
    appStore.showError((error as { message?: string REDACTED)?.message || t('errors.networkError'))
  REDACTED finally {
    deletingSoraProfile.value = false
  REDACTED
REDACTED

function formatDate(value?: string): string {
  if (!value) {
    return '-'
  REDACTED
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  REDACTED
  return date.toLocaleString()
REDACTED

function formatStorageQuotaGB(bytes: number): string {
  if (!bytes || bytes <= 0) {
    return '0 GB'
  REDACTED
  const gb = bytes / (1024 * 1024 * 1024)
  return `${gb.toFixed(gb >= 10 ? 0 : 1)REDACTED GB`
REDACTED

function pickPreferredSoraProfileID(): string {
  const active = soraS3Profiles.value.find((item) => item.is_active)
  if (active) {
    return active.profile_id
  REDACTED
  return soraS3Profiles.value[0]?.profile_id || ''
REDACTED

function syncSoraProfileFormWithSelection() {
  const profile = soraS3Profiles.value.find((item) => item.profile_id === selectedSoraProfileID.value)
  soraProfileForm.value = newDefaultSoraS3ProfileForm(profile)
REDACTED

function newDefaultSoraS3ProfileForm(profile?: SoraS3Profile): SoraS3ProfileForm {
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
      cdn_url: '',
      default_storage_quota_gb: 0
    REDACTED
  REDACTED

  const quotaBytes = profile.default_storage_quota_bytes || 0

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
    cdn_url: profile.cdn_url || '',
    default_storage_quota_gb: Number((quotaBytes / (1024 * 1024 * 1024)).toFixed(2))
  REDACTED
REDACTED

onMounted(async () => {
  await loadSoraS3Profiles()
REDACTED)
</script>

<style scoped>
.dm-drawer-mask-enter-active,
.dm-drawer-mask-leave-active {
  transition: opacity 0.2s ease;
REDACTED

.dm-drawer-mask-enter-from,
.dm-drawer-mask-leave-to {
  opacity: 0;
REDACTED

.dm-drawer-panel-enter-active,
.dm-drawer-panel-leave-active {
  transition:
    transform 0.24s cubic-bezier(0.22, 1, 0.36, 1),
    opacity 0.2s ease;
REDACTED

.dm-drawer-panel-enter-from,
.dm-drawer-panel-leave-to {
  opacity: 0.96;
  transform: translateX(100%);
REDACTED

@media (prefers-reduced-motion: reduce) {
  .dm-drawer-mask-enter-active,
  .dm-drawer-mask-leave-active,
  .dm-drawer-panel-enter-active,
  .dm-drawer-panel-leave-active {
    transition-duration: 0s;
  REDACTED
REDACTED
</style>
