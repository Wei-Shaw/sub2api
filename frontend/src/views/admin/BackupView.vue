<template>
    <div class="space-y-6">
      <!-- S3 Storage Config -->
      <div class="card p-5">
        <div class="card-header -mx-5 -mt-5 mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="card-title">
              {{ t('admin.backup.s3.title') }}
            </h3>
            <p class="mt-0.5 text-meta text-fg-muted">
              {{ t('admin.backup.s3.descriptionPrefix') }}
              <button type="button" class="text-accent underline hover:text-accent-strong" @click="showR2Guide = true">Cloudflare R2</button>
              {{ t('admin.backup.s3.descriptionSuffix') }}
            </p>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.backup.s3.endpoint') }}</label>
            <input v-model="s3Form.endpoint" class="input w-full" placeholder="https://<account_id>.r2.cloudflarestorage.com" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.s3.region') }}</label>
            <input v-model="s3Form.region" class="input w-full" placeholder="auto" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.s3.bucket') }}</label>
            <input v-model="s3Form.bucket" class="input w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.s3.prefix') }}</label>
            <input v-model="s3Form.prefix" class="input w-full" placeholder="backups/" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.s3.accessKeyId') }}</label>
            <input v-model="s3Form.access_key_id" class="input w-full" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.s3.secretAccessKey') }}</label>
            <input v-model="s3Form.secret_access_key" type="password" class="input w-full" :placeholder="s3SecretConfigured ? t('admin.backup.s3.secretConfigured') : ''" />
          </div>
          <label class="inline-flex items-center gap-2 text-sm text-fg md:col-span-2">
            <input v-model="s3Form.force_path_style" type="checkbox" />
            <span>{{ t('admin.backup.s3.forcePathStyle') }}</span>
          </label>
        </div>
        <div class="card-footer -mx-5 -mb-5 mt-5 flex flex-wrap justify-end gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="testingS3" @click="testS3">
            {{ testingS3 ? t('common.loading') : t('admin.backup.s3.testConnection') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" :disabled="savingS3" @click="saveS3Config">
            {{ savingS3 ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Async image object storage -->
      <div class="card p-5">
        <div class="card-header -mx-5 -mt-5 mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="card-title">
              {{ t('admin.backup.imageStorage.title') }}
            </h3>
            <p class="mt-0.5 text-meta text-fg-muted">
              {{ t('admin.backup.imageStorage.description') }}
            </p>
          </div>
          <label class="inline-flex items-center gap-2 text-sm text-fg">
            <input v-model="imageStorageForm.enabled" type="checkbox" />
            <span>{{ t('admin.backup.imageStorage.enabled') }}</span>
          </label>
        </div>

        <label class="inline-flex items-center gap-2 text-sm text-fg">
          <input v-model="imageStorageForm.reuse_backup_s3" type="checkbox" />
          <span>{{ t('admin.backup.imageStorage.reuseBackupS3') }}</span>
        </label>

        <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.backup.imageStorage.bucket') }}</label>
            <input v-model="imageStorageForm.bucket" class="input w-full" :placeholder="imageStorageForm.reuse_backup_s3 ? t('admin.backup.imageStorage.bucketInherited') : ''" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.imageStorage.prefix') }}</label>
            <input v-model="imageStorageForm.prefix" class="input w-full" placeholder="images/" />
          </div>

          <template v-if="!imageStorageForm.reuse_backup_s3">
            <div>
              <label class="input-label">{{ t('admin.backup.s3.endpoint') }}</label>
              <input v-model="imageStorageForm.endpoint" class="input w-full" placeholder="https://<account_id>.r2.cloudflarestorage.com" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.backup.s3.region') }}</label>
              <input v-model="imageStorageForm.region" class="input w-full" placeholder="auto" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.backup.s3.accessKeyId') }}</label>
              <input v-model="imageStorageForm.access_key_id" class="input w-full" />
            </div>
            <div>
              <label class="input-label">{{ t('admin.backup.s3.secretAccessKey') }}</label>
              <input v-model="imageStorageForm.secret_access_key" type="password" class="input w-full" :placeholder="imageStorageSecretConfigured ? t('admin.backup.s3.secretConfigured') : ''" />
            </div>
            <label class="inline-flex items-center gap-2 text-sm text-fg md:col-span-2">
              <input v-model="imageStorageForm.force_path_style" type="checkbox" />
              <span>{{ t('admin.backup.s3.forcePathStyle') }}</span>
            </label>
          </template>

          <div>
            <label class="input-label">{{ t('admin.backup.imageStorage.publicBaseUrl') }}</label>
            <input v-model="imageStorageForm.public_base_url" class="input w-full" :placeholder="t('admin.backup.imageStorage.publicBaseUrlPlaceholder')" />
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.imageStorage.presignExpiryHours') }}</label>
            <input v-model.number="imageStorageForm.presign_expiry_hours" type="number" min="1" class="input w-full" />
          </div>
          <div>
            <label class="input-label" for="image-studio-retention">{{ t('admin.backup.imageStorage.studioRetentionDays') }}</label>
            <input id="image-studio-retention" v-model.number="imageStorageForm.studio_retention_days" type="number" min="0" class="input w-full" />
            <p class="mt-1 text-meta text-fg-muted">{{ t('admin.backup.imageStorage.studioRetentionDaysHint') }}</p>
          </div>
        </div>

        <div class="card-footer -mx-5 -mb-5 mt-5 flex flex-wrap justify-end gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="testingImageStorage" @click="testImageStorage">
            {{ testingImageStorage ? t('common.loading') : t('admin.backup.s3.testConnection') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" :disabled="savingImageStorage" @click="saveImageStorageConfig">
            {{ savingImageStorage ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Schedule Config -->
      <div class="card p-5" data-testid="backup-schedule">
        <div class="card-header -mx-5 -mt-5 mb-4">
          <h3 class="card-title">
            {{ t('admin.backup.schedule.title') }}
          </h3>
          <p class="mt-0.5 text-meta text-fg-muted">
            {{ t('admin.backup.schedule.description') }}
          </p>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="inline-flex items-center gap-2 text-sm text-fg md:col-span-2">
            <input v-model="scheduleForm.enabled" type="checkbox" />
            <span>{{ t('admin.backup.schedule.enabled') }}</span>
          </label>
          <div>
            <label class="input-label">{{ t('admin.backup.schedule.cronExpr') }}</label>
            <input v-model="scheduleForm.cron_expr" class="input w-full" placeholder="0 2 * * *" />
            <p class="input-hint">{{ t('admin.backup.schedule.cronHint') }}</p>
          </div>
          <h4 class="mt-2 text-sm font-medium text-fg md:col-span-2">{{ t('admin.backup.schedule.ordinaryRetention') }}</h4>
          <div>
            <label class="input-label">{{ t('admin.backup.schedule.retainDays') }}</label>
            <input v-model.number="scheduleForm.retain_days" data-testid="backup-retain-days" type="number" min="0" step="1" class="input w-full" />
            <p class="input-hint">{{ t('admin.backup.schedule.retainDaysHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.backup.schedule.retainCount') }}</label>
            <input v-model.number="scheduleForm.retain_count" data-testid="backup-retain-count" type="number" min="0" step="1" class="input w-full" />
            <p class="input-hint">{{ t('admin.backup.schedule.retainCountHint') }}</p>
          </div>
        </div>
        <p class="input-hint mt-2">{{ t('admin.backup.schedule.ordinaryHint') }}</p>
        <BackupArchiveSettings v-model="archiveForm" />
        <p v-if="scheduleValidationError" class="mt-3 text-sm text-danger" role="alert">{{ scheduleValidationError }}</p>
        <div class="mt-4 border border-border bg-surface-sunken p-3 text-sm text-fg" aria-live="polite">
          <p class="font-medium">{{ t('admin.backup.schedule.preview') }}</p>
          <p class="text-fg-muted">{{ retentionPreview }}</p>
        </div>
        <div class="card-footer -mx-5 -mb-5 mt-5 flex justify-end">
          <button type="button" class="btn btn-primary btn-sm" :disabled="savingSchedule || !!scheduleValidationError" @click="saveSchedule">
            {{ savingSchedule ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Backup Operations -->
      <div class="card p-5">
        <div class="card-header -mx-5 -mt-5 mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="card-title">
              {{ t('admin.backup.operations.title') }}
            </h3>
            <p class="mt-0.5 text-meta text-fg-muted">
              {{ t('admin.backup.operations.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="flex items-center gap-1">
              <label class="text-xs text-fg-muted">{{ t('admin.backup.operations.expireDays') }}</label>
              <input v-model.number="manualExpireDays" type="number" min="0" class="input w-20 text-xs" />
            </div>
            <button type="button" class="btn btn-primary btn-sm" :disabled="creatingBackup" @click="createBackup">
              {{ creatingBackup ? t('admin.backup.operations.backing') : t('admin.backup.operations.createBackup') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingBackups" @click="loadBackups">
              {{ loadingBackups ? t('common.loading') : t('common.refresh') }}
            </button>
          </div>
        </div>

        <DataTable
          :columns="backupColumns"
          :data="backups"
          row-key="id"
          :loading="loadingBackups && backups.length === 0"
        >
          <template #cell-id="{ value }">
            <span class="font-mono text-meta">{{ value }}</span>
          </template>
          <template #cell-status="{ row }">
            <span class="badge" :class="statusClass(row.status)">
              {{ row.status === 'running' && row.progress
                ? t(`admin.backup.progress.${row.progress}`)
                : t(`admin.backup.status.${row.status}`) }}
            </span>
          </template>
          <template #cell-file_name="{ row }">
            {{ row.file_name }}
            <span v-if="row.monthly_archive" class="badge badge-primary ml-1">{{ t('admin.backup.archive.badge') }}</span>
            <div v-if="row.monthly_archive" class="mt-1 text-meta text-fg-muted">{{ row.monthly_archive.dates.join(' / ') }}</div>
          </template>
          <template #cell-expires_at="{ row }">
            {{ row.monthly_archive
              ? row.monthly_archive.retain_count === 0 ? t('admin.backup.archive.forever') : t('admin.backup.archive.retainLatest', { count: row.monthly_archive.retain_count })
              : row.expires_at ? formatDate(row.expires_at) : t('admin.backup.neverExpire') }}
          </template>
          <template #cell-parts="{ row }">
            {{ row.parts?.length || (row.status === 'running' ? '-' : 1) }}
          </template>
          <template #cell-actions="{ row }">
            <div class="flex flex-wrap gap-1">
              <button
                v-if="row.status === 'completed'"
                type="button"
                class="btn btn-secondary btn-sm"
                @click="downloadBackup(row.id)"
              >
                {{ t('admin.backup.actions.download') }}
              </button>
              <button
                v-if="row.status === 'completed'"
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="restoringId === row.id"
                @click="restoreBackup(row.id)"
              >
                {{ restoringId === row.id ? t('common.loading') : t('admin.backup.actions.restore') }}
              </button>
              <button
                v-if="row.status !== 'running'"
                type="button"
                class="btn btn-danger btn-sm"
                @click="removeBackup(row.id)"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </template>
          <template #empty>
            <span class="text-body text-fg-muted">{{ t('admin.backup.empty') }}</span>
          </template>
        </DataTable>
      </div>
    </div>

    <!-- Cloudflare R2 Setup Guide Modal -->
    <BaseDialog
      :show="showR2Guide"
      :title="t('admin.backup.r2Guide.title')"
      width="wide"
      close-on-click-outside
      @close="showR2Guide = false"
    >
      <p class="mb-4 text-sm text-fg-muted">{{ t('admin.backup.r2Guide.intro') }}</p>

      <!-- Step 1 -->
      <div class="mb-5">
        <h3 class="mb-2 flex items-baseline gap-3 border-b border-border pb-1 text-h3 font-bold text-fg">
          <span class="w-5 shrink-0 text-right tabular-nums text-accent-strong">1</span>
          {{ t('admin.backup.r2Guide.step1.title') }}
        </h3>
        <ol class="ml-8 list-decimal space-y-1 text-sm text-fg-muted">
          <li>{{ t('admin.backup.r2Guide.step1.line1') }}</li>
          <li>{{ t('admin.backup.r2Guide.step1.line2') }}</li>
          <li>{{ t('admin.backup.r2Guide.step1.line3') }}</li>
        </ol>
      </div>

      <!-- Step 2 -->
      <div class="mb-5">
        <h3 class="mb-2 flex items-baseline gap-3 border-b border-border pb-1 text-h3 font-bold text-fg">
          <span class="w-5 shrink-0 text-right tabular-nums text-accent-strong">2</span>
          {{ t('admin.backup.r2Guide.step2.title') }}
        </h3>
        <ol class="ml-8 list-decimal space-y-1 text-sm text-fg-muted">
          <li>{{ t('admin.backup.r2Guide.step2.line1') }}</li>
          <li>{{ t('admin.backup.r2Guide.step2.line2') }}</li>
          <li>{{ t('admin.backup.r2Guide.step2.line3') }}</li>
          <li>{{ t('admin.backup.r2Guide.step2.line4') }}</li>
        </ol>
        <div class="ml-8 mt-2 border border-warning/40 bg-warning-weak px-3 py-2 text-meta text-warning-strong">
          {{ t('admin.backup.r2Guide.step2.warning') }}
        </div>
      </div>

      <!-- Step 3 -->
      <div class="mb-5">
        <h3 class="mb-2 flex items-baseline gap-3 border-b border-border pb-1 text-h3 font-bold text-fg">
          <span class="w-5 shrink-0 text-right tabular-nums text-accent-strong">3</span>
          {{ t('admin.backup.r2Guide.step3.title') }}
        </h3>
        <p class="ml-8 text-sm text-fg-muted">{{ t('admin.backup.r2Guide.step3.desc') }}</p>
        <code class="code-block ml-8 mt-1 block px-3 py-2">https://&lt;{{ t('admin.backup.r2Guide.step3.accountId') }}&gt;.r2.cloudflarestorage.com</code>
      </div>

      <!-- Step 4: Fill form -->
      <div class="mb-5">
        <h3 class="mb-2 flex items-baseline gap-3 border-b border-border pb-1 text-h3 font-bold text-fg">
          <span class="w-5 shrink-0 text-right tabular-nums text-accent-strong">4</span>
          {{ t('admin.backup.r2Guide.step4.title') }}
        </h3>
        <div class="table-container ml-8">
          <table class="table">
            <tbody>
              <tr v-for="(row, i) in r2ConfigRows" :key="i">
                <td class="whitespace-nowrap bg-accent-weak font-semibold text-accent-strong">{{ row.field }}</td>
                <td class="text-fg-muted"><code class="font-mono text-meta">{{ row.value }}</code></td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Free tier note -->
      <div class="border border-success/40 bg-success-weak px-3 py-2 text-meta text-success-strong">
        {{ t('admin.backup.r2Guide.freeTier') }}
      </div>

      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary btn-sm" @click="showR2Guide = false">{{ t('common.close') }}</button>
        </div>
      </template>
    </BaseDialog>

    <!-- 分卷下载链接 -->
    <BaseDialog
      :show="downloadPartsModalOpen"
      :title="t('admin.backup.actions.downloadParts')"
      width="normal"
      close-on-click-outside
      @close="closeDownloadParts"
    >
      <p class="mb-4 text-sm text-fg-muted">{{ t('admin.backup.actions.downloadPartsHint') }}</p>
      <div class="divide-y divide-border border-y border-border">
        <div
          v-for="part in downloadParts"
          :key="part.index"
          class="flex items-center justify-between gap-3 py-2"
        >
          <span class="text-sm text-fg">
            {{ t('admin.backup.actions.partLabel', { index: part.index }) }}
            <span class="ml-2 text-meta tabular-nums text-fg-muted">{{ formatSize(part.size_bytes) }}</span>
          </span>
          <a :href="part.url" class="btn btn-secondary btn-sm" rel="noopener">
            {{ t('admin.backup.actions.download') }}
          </a>
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end">
          <button type="button" class="btn btn-primary btn-sm" @click="closeDownloadParts">{{ t('common.close') }}</button>
        </div>
      </template>
    </BaseDialog>

    <!-- 恢复确认：管理员密码用 type=password 输入，不再用 window.prompt 明文显示 -->
    <BaseDialog
      :show="restoreTargetId !== ''"
      :title="t('admin.backup.actions.restore')"
      width="narrow"
      @close="closeRestoreDialog"
    >
      <form id="backup-restore-form" class="space-y-3" @submit.prevent="submitRestore">
        <p class="text-sm text-danger">{{ t('admin.backup.actions.restoreConfirm') }}</p>
        <label for="backup-restore-password" class="input-label">{{ t('admin.backup.actions.restorePasswordPrompt') }}</label>
        <input
          id="backup-restore-password"
          v-model="restorePassword"
          type="password"
          autocomplete="current-password"
          class="input w-full"
        />
      </form>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary btn-sm" @click="closeRestoreDialog">{{ t('common.cancel') }}</button>
          <button type="submit" form="backup-restore-form" class="btn btn-danger btn-sm" :disabled="!restorePassword">
            {{ t('admin.backup.actions.restore') }}
          </button>
        </div>
      </template>
    </BaseDialog>
    <TotpStepUpDialog :controller="backupStepUp" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import { useAppStore } from '@/stores'
import type {
  BackupS3Config,
  BackupScheduleConfig,
  BackupMonthlyArchiveConfig,
  BackupRecord,
  BackupDownloadPart,
  ImageStorageConfig,
} from '@/api/admin/backup'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import BackupArchiveSettings from '@/components/admin/BackupArchiveSettings.vue'

const { t } = useI18n()
const appStore = useAppStore()
const backupStepUp = useStepUp()

// 敏感操作被 2FA 门控拦截时的统一提示。
function reportStepUpBlocked(error: unknown): boolean {
  if (!isStepUpBlocked(error)) return false
  appStore.showError(
    stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
      ? t('stepUp.adminApiKeyForbidden')
      : t('stepUp.notEnabled')
  )
  return true
}

// S3 config
const s3Form = ref<BackupS3Config>({
  endpoint: '',
  region: 'auto',
  bucket: '',
  access_key_id: '',
  secret_access_key: '',
  prefix: 'backups/',
  force_path_style: false,
})
const s3SecretConfigured = ref(false)
const savingS3 = ref(false)
const testingS3 = ref(false)

// Async image object storage. Shares the S3 client with backups, so the default is
// to reuse the credentials configured above and only differ by prefix.
const imageStorageForm = ref<ImageStorageConfig>({
  enabled: false,
  reuse_backup_s3: true,
  bucket: '',
  prefix: 'images/',
  public_base_url: '',
  presign_expiry_hours: 24,
  max_download_bytes: 33554432,
  endpoint: '',
  region: 'auto',
  access_key_id: '',
  secret_access_key: '',
  force_path_style: false,
  studio_retention_days: 30,
})
const imageStorageSecretConfigured = ref(false)
const savingImageStorage = ref(false)
const testingImageStorage = ref(false)

// Schedule config
const scheduleForm = ref<BackupScheduleConfig>({
  enabled: false,
  cron_expr: '0 2 * * *',
  retain_days: 14,
  retain_count: 10,
})
const savingSchedule = ref(false)
const archiveForm = ref<BackupMonthlyArchiveConfig>({ enabled: false, days: [1], include_month_end: false, retain_count: 0 })
// Disabling hides the archive parameters, so a save while disabled keeps the
// persisted parameters and only turns the rule off.
const savedArchive = ref<BackupMonthlyArchiveConfig>(cloneArchive(archiveForm.value))
const archivePayload = computed<BackupMonthlyArchiveConfig>(() =>
  archiveForm.value.enabled ? cloneArchive(archiveForm.value) : { ...cloneArchive(savedArchive.value), enabled: false },
)
function cloneArchive(config: BackupMonthlyArchiveConfig): BackupMonthlyArchiveConfig {
  return { ...config, days: [...config.days] }
}
const scheduleValidationError = computed(() => {
  const counts = [scheduleForm.value.retain_days, scheduleForm.value.retain_count]
  if (archiveForm.value.enabled) counts.push(archiveForm.value.retain_count)
  if (!counts.every(value => Number.isSafeInteger(value) && value >= 0)) {
    return t('admin.backup.archive.invalidRetention')
  }
  if (archiveForm.value.enabled && !archiveForm.value.days.length && !archiveForm.value.include_month_end) {
    return t('admin.backup.archive.selectDates')
  }
  return ''
})
const retentionPreview = computed(() => {
  if (scheduleValidationError.value) return scheduleValidationError.value
  const { retain_days: days, retain_count: count } = scheduleForm.value
  const ordinary = days > 0 && count > 0
    ? t('admin.backup.schedule.previewBoth', { days, count })
    : days > 0 ? t('admin.backup.schedule.previewDays', { days })
      : count > 0 ? t('admin.backup.schedule.previewCount', { count }) : t('admin.backup.schedule.previewUnlimited')
  if (!archiveForm.value.enabled) return ordinary
  const dates = [
    ...[...archiveForm.value.days].sort((a, b) => a - b).map(day => t('admin.backup.archive.day', { day })),
    ...(archiveForm.value.include_month_end ? [t('admin.backup.archive.monthEnd')] : []),
  ].join(', ')
  const retention = archiveForm.value.retain_count === 0 ? t('admin.backup.archive.forever') : t('admin.backup.archive.retainLatest', { count: archiveForm.value.retain_count })
  return `${ordinary} ${t('admin.backup.archive.preview', { dates, retention })}`
})

// Backups
const backups = ref<BackupRecord[]>([])
const loadingBackups = ref(false)
const creatingBackup = ref(false)
const restoringId = ref('')
const manualExpireDays = ref(14)
const downloadParts = ref<BackupDownloadPart[]>([])
const downloadPartsModalOpen = ref(false)

const backupColumns = computed<Column[]>(() => [
  { key: 'id', label: 'ID' },
  { key: 'status', label: t('admin.backup.columns.status') },
  { key: 'file_name', label: t('admin.backup.columns.fileName') },
  { key: 'size_bytes', label: t('admin.backup.columns.size'), formatter: (value) => formatSize(value) },
  { key: 'parts', label: t('admin.backup.columns.parts') },
  { key: 'expires_at', label: t('admin.backup.columns.expiresAt') },
  {
    key: 'triggered_by',
    label: t('admin.backup.columns.triggeredBy'),
    formatter: (value) => (value === 'scheduled' ? t('admin.backup.trigger.scheduled') : t('admin.backup.trigger.manual')),
  },
  { key: 'started_at', label: t('admin.backup.columns.startedAt'), formatter: (value) => formatDate(value) },
  { key: 'actions', label: t('admin.backup.columns.actions') },
])

// Polling
const pollingTimer = ref<ReturnType<typeof setInterval> | null>(null)
const restoringPollingTimer = ref<ReturnType<typeof setInterval> | null>(null)
const MAX_POLL_COUNT = 900

function updateRecordInList(updated: BackupRecord) {
  const idx = backups.value.findIndex(r => r.id === updated.id)
  if (idx >= 0) {
    backups.value[idx] = updated
  }
}

function startPolling(backupId: string) {
  stopPolling()
  let count = 0
  pollingTimer.value = setInterval(async () => {
    if (count++ >= MAX_POLL_COUNT) {
      stopPolling()
      creatingBackup.value = false
      appStore.showWarning(t('admin.backup.operations.backupRunning'))
      return
    }
    try {
      const record = await adminAPI.backup.getBackup(backupId)
      updateRecordInList(record)
      if (record.status === 'completed' || record.status === 'failed') {
        stopPolling()
        creatingBackup.value = false
        if (record.status === 'completed') {
          appStore.showSuccess(t('admin.backup.operations.backupCreated'))
        } else {
          appStore.showError(record.error_message || t('admin.backup.operations.backupFailed'))
        }
        await loadBackups()
      }
    } catch {
      // 轮询失败时不中断
    }
  }, 2000)
}

function stopPolling() {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

function startRestorePolling(backupId: string) {
  stopRestorePolling()
  let count = 0
  restoringPollingTimer.value = setInterval(async () => {
    if (count++ >= MAX_POLL_COUNT) {
      stopRestorePolling()
      restoringId.value = ''
      appStore.showWarning(t('admin.backup.operations.restoreRunning'))
      return
    }
    try {
      const record = await adminAPI.backup.getBackup(backupId)
      updateRecordInList(record)
      if (record.restore_status === 'completed' || record.restore_status === 'failed') {
        stopRestorePolling()
        restoringId.value = ''
        if (record.restore_status === 'completed') {
          appStore.showSuccess(t('admin.backup.actions.restoreSuccess'))
        } else {
          appStore.showError(record.restore_error || t('admin.backup.operations.restoreFailed'))
        }
        await loadBackups()
      }
    } catch {
      // 轮询失败时不中断
    }
  }, 2000)
}

function stopRestorePolling() {
  if (restoringPollingTimer.value) {
    clearInterval(restoringPollingTimer.value)
    restoringPollingTimer.value = null
  }
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopPolling()
    stopRestorePolling()
  } else {
    // 标签页恢复时刷新列表，检查是否仍有活跃操作
    loadBackups().then(() => {
      const running = backups.value.find(r => r.status === 'running')
      if (running) {
        creatingBackup.value = true
        startPolling(running.id)
      }
      const restoring = backups.value.find(r => r.restore_status === 'running')
      if (restoring) {
        restoringId.value = restoring.id
        startRestorePolling(restoring.id)
      }
    })
  }
}

// R2 guide
const showR2Guide = ref(false)
const r2ConfigRows = computed(() => [
  { field: t('admin.backup.s3.endpoint'), value: 'https://<account_id>.r2.cloudflarestorage.com' },
  { field: t('admin.backup.s3.region'), value: 'auto' },
  { field: t('admin.backup.s3.bucket'), value: t('admin.backup.r2Guide.step4.bucketValue') },
  { field: t('admin.backup.s3.prefix'), value: 'backups/' },
  { field: 'Access Key ID', value: t('admin.backup.r2Guide.step4.fromStep2') },
  { field: 'Secret Access Key', value: t('admin.backup.r2Guide.step4.fromStep2') },
  { field: t('admin.backup.s3.forcePathStyle'), value: t('admin.backup.r2Guide.step4.unchecked') },
])

async function loadS3Config() {
  try {
    const cfg = await adminAPI.backup.getS3Config()
    s3Form.value = {
      endpoint: cfg.endpoint || '',
      region: cfg.region || 'auto',
      bucket: cfg.bucket || '',
      access_key_id: cfg.access_key_id || '',
      secret_access_key: '',
      prefix: cfg.prefix || 'backups/',
      force_path_style: cfg.force_path_style,
    }
    s3SecretConfigured.value = Boolean(cfg.access_key_id)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveS3Config() {
  savingS3.value = true
  try {
    await backupStepUp.run(() => adminAPI.backup.updateS3Config(s3Form.value))
    appStore.showSuccess(t('admin.backup.s3.saved'))
    await loadS3Config()
  } catch (error) {
    if (isStepUpCancelled(error)) {
      savingS3.value = false
      return
    }
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingS3.value = false
  }
}

async function loadImageStorageConfig() {
  try {
    const { config, secret_configured } = await adminAPI.backup.getImageStorageConfig()
    imageStorageForm.value = {
      ...config,
      prefix: config.prefix || 'images/',
      region: config.region || 'auto',
      secret_access_key: '',
      studio_retention_days: config.studio_retention_days ?? 30,
    }
    imageStorageSecretConfigured.value = secret_configured
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveImageStorageConfig() {
  savingImageStorage.value = true
  try {
    await backupStepUp.run(() => adminAPI.backup.updateImageStorageConfig(imageStorageForm.value))
    appStore.showSuccess(t('admin.backup.imageStorage.saved'))
    await loadImageStorageConfig()
  } catch (error) {
    if (isStepUpCancelled(error)) {
      savingImageStorage.value = false
      return
    }
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingImageStorage.value = false
  }
}

async function testImageStorage() {
  testingImageStorage.value = true
  try {
    const result = await adminAPI.backup.testImageStorageConnection(imageStorageForm.value)
    if (result.ok) {
      appStore.showSuccess(result.message || t('admin.backup.s3.testSuccess'))
    } else {
      appStore.showError(result.message || t('admin.backup.s3.testFailed'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingImageStorage.value = false
  }
}

async function testS3() {
  testingS3.value = true
  try {
    const result = await adminAPI.backup.testS3Connection(s3Form.value)
    if (result.ok) {
      appStore.showSuccess(result.message || t('admin.backup.s3.testSuccess'))
    } else {
      appStore.showError(result.message || t('admin.backup.s3.testFailed'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingS3.value = false
  }
}

async function loadSchedule() {
  try {
    const cfg = await adminAPI.backup.getSchedule()
    scheduleForm.value = {
      enabled: cfg.enabled,
      cron_expr: cfg.cron_expr || '0 2 * * *',
      retain_days: cfg.retain_days ?? 14,
      retain_count: cfg.retain_count ?? 10,
    }
    archiveForm.value = {
      enabled: cfg.monthly_archive?.enabled ?? false,
      days: cfg.monthly_archive?.days ?? [1],
      include_month_end: cfg.monthly_archive?.include_month_end ?? false,
      retain_count: cfg.monthly_archive?.retain_count ?? 0,
    }
    savedArchive.value = cloneArchive(archiveForm.value)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveSchedule() {
  if (scheduleValidationError.value) return
  savingSchedule.value = true
  try {
    const archive = archivePayload.value
    await adminAPI.backup.updateSchedule({ ...scheduleForm.value, monthly_archive: archive })
    savedArchive.value = cloneArchive(archive)
    appStore.showSuccess(t('admin.backup.schedule.saved'))
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingSchedule.value = false
  }
}

async function loadBackups() {
  loadingBackups.value = true
  try {
    const result = await adminAPI.backup.listBackups()
    backups.value = result.items || []
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    loadingBackups.value = false
  }
}

async function createBackup() {
  creatingBackup.value = true
  try {
    const record = await backupStepUp.run(() => adminAPI.backup.createBackup({ expire_days: manualExpireDays.value }))
    // 插入到列表顶部
    backups.value.unshift(record)
    startPolling(record.id)
  } catch (error: any) {
    if (isStepUpCancelled(error)) {
      creatingBackup.value = false
      return
    }
    if (reportStepUpBlocked(error)) {
      creatingBackup.value = false
      return
    }
    if (error?.response?.status === 409) {
      appStore.showWarning(t('admin.backup.operations.alreadyInProgress'))
    } else {
      appStore.showError(error?.message || t('errors.networkError'))
    }
    creatingBackup.value = false
  }
}

async function downloadBackup(id: string) {
  try {
    const result = await backupStepUp.run(() => adminAPI.backup.getDownloadURL(id))
    if (result.parts && result.parts.length > 0) {
      downloadParts.value = result.parts
      downloadPartsModalOpen.value = true
      return
    }
    if (!result.url) {
      throw new Error(t('admin.backup.actions.downloadFailed'))
    }
    // 预签名 URL 带 attachment disposition，同页 anchor 导航直接触发下载；
    // 不用 window.open：step-up 弹窗 await 会耗尽瞬态用户激活，新标签页会被浏览器拦截。
    const link = document.createElement('a')
    link.href = result.url
    link.rel = 'noopener'
    link.click()
  } catch (error) {
    if (isStepUpCancelled(error)) return
    if (reportStepUpBlocked(error)) return
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

function closeDownloadParts() {
  downloadPartsModalOpen.value = false
  downloadParts.value = []
}

const restoreTargetId = ref('')
const restorePassword = ref('')

function restoreBackup(id: string) {
  restoreTargetId.value = id
  restorePassword.value = ''
}

function closeRestoreDialog() {
  restoreTargetId.value = ''
  restorePassword.value = ''
}

async function submitRestore() {
  const id = restoreTargetId.value
  const password = restorePassword.value
  if (!id || !password) return
  closeRestoreDialog()
  restoringId.value = id
  try {
    const record = await backupStepUp.run(() => adminAPI.backup.restoreBackup(id, password))
    updateRecordInList(record)
    startRestorePolling(id)
  } catch (error: any) {
    restoringId.value = ''
    if (isStepUpCancelled(error)) return
    if (reportStepUpBlocked(error)) return
    // apiClient 拦截器把 HTTP 错误归一化为顶层 { status } 平面对象（无 response 字段）
    if (error?.status === 409 || error?.response?.status === 409) {
      appStore.showWarning(t('admin.backup.operations.restoreRunning'))
    } else {
      appStore.showError(error?.message || t('errors.networkError'))
    }
  }
}

async function removeBackup(id: string) {
  const archived = !!backups.value.find(record => record.id === id)?.monthly_archive
  if (!window.confirm(t(archived ? 'admin.backup.archive.deleteConfirm' : 'admin.backup.actions.deleteConfirm'))) return
  try {
    await adminAPI.backup.deleteBackup(id, archived)
    appStore.showSuccess(t('admin.backup.actions.deleted'))
    await loadBackups()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

function statusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'badge-success'
    case 'running':
      return 'badge-primary'
    case 'failed':
      return 'badge-danger'
    default:
      return 'badge-gray'
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

onMounted(async () => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  await Promise.all([loadS3Config(), loadImageStorageConfig(), loadSchedule(), loadBackups()])

  // 如果有正在 running 的备份，恢复轮询
  const runningBackup = backups.value.find(r => r.status === 'running')
  if (runningBackup) {
    creatingBackup.value = true
    startPolling(runningBackup.id)
  }
  const restoringBackup = backups.value.find(r => r.restore_status === 'running')
  if (restoringBackup) {
    restoringId.value = restoringBackup.id
    startRestorePolling(restoringBackup.id)
  }
})

onBeforeUnmount(() => {
  stopPolling()
  stopRestorePolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
