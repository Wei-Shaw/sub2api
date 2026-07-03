<template>
  <BaseDialog
    :show="show"
    :title="t('admin.tlsFingerprintProfiles.title')"
    width="wide"
    @close="$emit('close')"
  >
    <div class="space-y-4">
      <!-- Header -->
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintProfiles.description') }}
        </p>
        <button @click="showCreateModal = true" class="btn btn-primary btn-sm">
          <Icon name="plus" size="sm" class="mr-1" />
          {{ t('admin.tlsFingerprintProfiles.createProfile') }}
        </button>
      </div>

      <!-- Collector -->
      <div class="rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div class="flex items-center gap-2">
              <Icon name="beaker" size="sm" class="text-primary-600 dark:text-primary-400" />
              <h4 class="text-sm font-medium text-gray-900 dark:text-white">
                {{ t('admin.tlsFingerprintProfiles.collector.title') }}
              </h4>
              <span
                :class="[
                  'rounded-full px-2 py-0.5 text-xs font-medium',
                  isCollectorRunning
                    ? 'bg-green-100 text-green-700 dark:bg-green-900/40 dark:text-green-300'
                    : 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
                ]"
              >
                {{ isCollectorRunning ? t('admin.tlsFingerprintProfiles.collector.running') : t('admin.tlsFingerprintProfiles.collector.stopped') }}
              </span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.collector.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="collectorLoading"
              @click="loadCollectorStatus"
            >
              <Icon name="refresh" size="sm" :class="['mr-1', collectorLoading ? 'animate-spin' : '']" />
              {{ t('common.refresh') }}
            </button>
            <button
              type="button"
              :class="['btn btn-sm', isCollectorRunning ? 'btn-secondary' : 'btn-primary']"
              :disabled="collectorActionLoading"
              @click="toggleCollector"
            >
              <Icon
                :name="collectorActionLoading ? 'refresh' : (isCollectorRunning ? 'x' : 'play')"
                size="sm"
                :class="['mr-1', collectorActionLoading ? 'animate-spin' : '']"
              />
              {{ isCollectorRunning ? t('admin.tlsFingerprintProfiles.collector.stop') : t('admin.tlsFingerprintProfiles.collector.start') }}
            </button>
          </div>
        </div>

        <div class="mt-3 grid gap-3 text-xs sm:grid-cols-3">
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.listenAddress') }}</div>
            <div class="mt-1 font-mono text-gray-900 dark:text-gray-100">{{ collectorStatus?.listen_address || '—' }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.publicBaseURL') }}</div>
            <div class="mt-1 break-all font-mono text-gray-900 dark:text-gray-100">{{ collectorStatus?.public_base_url || '—' }}</div>
          </div>
          <div>
            <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.certificate') }}</div>
            <div class="mt-1 text-gray-900 dark:text-gray-100">
              {{ collectorStatus?.using_generated_cert ? t('admin.tlsFingerprintProfiles.collector.generatedCert') : t('admin.tlsFingerprintProfiles.collector.configuredCert') }}
            </div>
          </div>
        </div>

        <p v-if="collectorStatus?.last_error" class="mt-3 text-xs text-red-600 dark:text-red-400">
          {{ collectorStatus.last_error }}
        </p>

        <div v-if="isCollectorRunning" class="mt-4 space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <button
              type="button"
              class="btn btn-primary btn-sm"
              :disabled="collectorSessionLoading"
              @click="createCollectorSession"
            >
              <Icon name="plus" size="sm" class="mr-1" />
              {{ t('admin.tlsFingerprintProfiles.collector.createSession') }}
            </button>
            <button
              v-if="collectorSession"
              type="button"
              class="btn btn-secondary btn-sm"
              :disabled="collectorCapturesLoading"
              @click="refreshCollectorCaptures"
            >
              <Icon name="refresh" size="sm" :class="['mr-1', collectorCapturesLoading ? 'animate-spin' : '']" />
              {{ t('admin.tlsFingerprintProfiles.collector.refreshCaptures') }}
            </button>
          </div>

          <div v-if="collectorSession" class="space-y-3 rounded-md border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900/60">
            <div class="grid gap-3 text-xs sm:grid-cols-2">
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.captureURL') }}</div>
                <div class="mt-1 break-all font-mono text-gray-900 dark:text-gray-100">{{ collectorSession.capture_url }}</div>
              </div>
              <div>
                <div class="text-gray-500 dark:text-gray-400">{{ t('admin.tlsFingerprintProfiles.collector.expiresAt') }}</div>
                <div class="mt-1 text-gray-900 dark:text-gray-100">{{ formatDateTime(collectorSession.expires_at) }}</div>
              </div>
            </div>

            <div class="grid gap-3 lg:grid-cols-3">
              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between">
                  <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.tlsFingerprintProfiles.collector.claudeCommand') }}</span>
                  <button type="button" class="btn btn-secondary btn-xs" @click="copyText(claudeCommand)">
                    <Icon name="copy" size="xs" class="mr-1" />
                    {{ t('common.copy') }}
                  </button>
                </div>
                <pre class="max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded bg-gray-900 p-2 text-xs leading-relaxed text-gray-100">{{ claudeCommand }}</pre>
              </div>
              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between">
                  <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.tlsFingerprintProfiles.collector.codexCommand') }}</span>
                  <button type="button" class="btn btn-secondary btn-xs" @click="copyText(codexCommand)">
                    <Icon name="copy" size="xs" class="mr-1" />
                    {{ t('common.copy') }}
                  </button>
                </div>
                <pre class="max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded bg-gray-900 p-2 text-xs leading-relaxed text-gray-100">{{ codexCommand }}</pre>
              </div>
              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between">
                  <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('admin.tlsFingerprintProfiles.collector.codexConfig') }}</span>
                  <button type="button" class="btn btn-secondary btn-xs" @click="copyText(codexConfigSnippet)">
                    <Icon name="copy" size="xs" class="mr-1" />
                    {{ t('common.copy') }}
                  </button>
                </div>
                <pre class="max-h-32 overflow-y-auto whitespace-pre-wrap break-all rounded bg-gray-900 p-2 text-xs leading-relaxed text-gray-100">{{ codexConfigSnippet }}</pre>
              </div>
            </div>

            <div v-if="activeCAPEM" class="flex flex-wrap items-center gap-2">
              <button type="button" class="btn btn-secondary btn-sm" @click="copyText(activeCAPEM)">
                <Icon name="copy" size="sm" class="mr-1" />
                {{ t('admin.tlsFingerprintProfiles.collector.copyCA') }}
              </button>
              <button type="button" class="btn btn-secondary btn-sm" @click="downloadCA">
                <Icon name="download" size="sm" class="mr-1" />
                {{ t('admin.tlsFingerprintProfiles.collector.downloadCA') }}
              </button>
            </div>
          </div>

          <div v-if="collectorSession" class="space-y-2" data-testid="tls-collector-captures">
            <div class="flex items-center justify-between">
              <h5 class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.collector.captures') }}
              </h5>
              <span class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.collector.captureCount', { count: collectorCaptures.length }) }}
              </span>
            </div>
            <div v-if="collectorCaptures.length === 0" class="rounded-md border border-dashed border-gray-300 px-3 py-4 text-center text-xs text-gray-500 dark:border-dark-600 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.collector.noCaptures') }}
            </div>
            <div
              v-for="record in collectorCaptures"
              :key="record.id"
              class="rounded-md border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-900/60"
            >
              <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0 text-xs">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="font-medium text-gray-900 dark:text-white">{{ formatClientKind(record.client_kind) }}</span>
                    <span class="font-mono text-gray-500 dark:text-gray-400">{{ record.ja3_hash }}</span>
                  </div>
                  <div class="mt-1 break-all text-gray-500 dark:text-gray-400">
                    {{ record.user_agent || '—' }}
                  </div>
                  <div class="mt-1 text-gray-500 dark:text-gray-400">
                    {{ formatDateTime(record.captured_at) }} · {{ record.http_proto || '—' }} · ALPN {{ record.negotiated_alpn || '—' }}
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="btn btn-secondary btn-xs" @click="copyText(record.yaml)">
                    <Icon name="copy" size="xs" class="mr-1" />
                    {{ t('admin.tlsFingerprintProfiles.collector.copyYaml') }}
                  </button>
                  <button type="button" class="btn btn-primary btn-xs" @click="applyCapture(record)">
                    <Icon name="check" size="xs" class="mr-1" />
                    {{ t('admin.tlsFingerprintProfiles.collector.applyCapture') }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Profiles Table -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <Icon name="refresh" size="lg" class="animate-spin text-gray-400" />
      </div>

      <div v-else-if="profiles.length === 0" class="py-8 text-center">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
          <Icon name="shield" size="lg" class="text-gray-400" />
        </div>
        <h4 class="mb-1 text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.tlsFingerprintProfiles.noProfiles') }}
        </h4>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('admin.tlsFingerprintProfiles.createFirstProfile') }}
        </p>
      </div>

      <div v-else class="max-h-96 overflow-auto rounded-lg border border-gray-200 dark:border-dark-600">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-700">
          <thead class="sticky top-0 bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.id') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.name') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.description') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.grease') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.alpn') }}
              </th>
              <th class="px-3 py-2 text-left text-xs font-medium uppercase text-gray-500 dark:text-gray-400">
                {{ t('admin.tlsFingerprintProfiles.columns.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-700 dark:bg-dark-800">
            <tr v-for="profile in profiles" :key="profile.id" class="hover:bg-gray-50 dark:hover:bg-dark-700">
              <td class="px-3 py-2">
                <span
                  class="cursor-pointer font-mono text-sm text-gray-700 dark:text-gray-300"
                  :title="t('common.copy')"
                  @click="copyText(String(profile.id))"
                  >{{ profile.id }}</span
                >
              </td>
              <td class="px-3 py-2">
                <div class="font-medium text-gray-900 dark:text-white text-sm">{{ profile.name }}</div>
              </td>
              <td class="px-3 py-2">
                <div v-if="profile.description" class="text-sm text-gray-500 dark:text-gray-400 max-w-xs truncate">
                  {{ profile.description }}
                </div>
                <div v-else class="text-xs text-gray-400 dark:text-gray-600">—</div>
              </td>
              <td class="px-3 py-2">
                <Icon
                  :name="profile.enable_grease ? 'check' : 'lock'"
                  size="sm"
                  :class="profile.enable_grease ? 'text-green-500' : 'text-gray-400'"
                />
              </td>
              <td class="px-3 py-2">
                <div v-if="profile.alpn_protocols?.length" class="flex flex-wrap gap-1">
                  <span
                    v-for="proto in profile.alpn_protocols.slice(0, 3)"
                    :key="proto"
                    class="badge badge-primary text-xs"
                  >
                    {{ proto }}
                  </span>
                  <span v-if="profile.alpn_protocols.length > 3" class="text-xs text-gray-500">
                    +{{ profile.alpn_protocols.length - 3 }}
                  </span>
                </div>
                <div v-else class="text-xs text-gray-400 dark:text-gray-600">—</div>
              </td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-1">
                  <button
                    @click="handleEdit(profile)"
                    class="p-1 text-gray-500 hover:text-primary-600 dark:hover:text-primary-400"
                    :title="t('common.edit')"
                  >
                    <Icon name="edit" size="sm" />
                  </button>
                  <button
                    @click="handleDelete(profile)"
                    class="p-1 text-gray-500 hover:text-red-600 dark:hover:text-red-400"
                    :title="t('common.delete')"
                  >
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="$emit('close')" class="btn btn-secondary">
          {{ t('common.close') }}
        </button>
      </div>
    </template>

    <!-- Create/Edit Modal -->
    <BaseDialog
      :show="showCreateModal || showEditModal"
      :title="showEditModal ? t('admin.tlsFingerprintProfiles.editProfile') : t('admin.tlsFingerprintProfiles.createProfile')"
      width="wide"
      :z-index="60"
      @close="closeFormModal"
    >
      <form @submit.prevent="handleSubmit" class="space-y-4">
        <!-- Paste YAML -->
        <div>
          <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.pasteYaml') }}</label>
          <textarea
            v-model="yamlInput"
            rows="4"
            class="input font-mono text-xs"
            :placeholder="t('admin.tlsFingerprintProfiles.form.pasteYamlPlaceholder')"
            @paste="handleYamlPaste"
          />
          <div class="mt-1 flex items-center gap-2">
            <button type="button" @click="parseYamlInput" class="btn btn-secondary btn-sm">
              {{ t('admin.tlsFingerprintProfiles.form.parseYaml') }}
            </button>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.form.pasteYamlHint') }}
              <a href="https://tls.sub2api.org" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300 underline">{{ t('admin.tlsFingerprintProfiles.form.openCollector') }}</a>
            </p>
          </div>
        </div>

        <hr class="border-gray-200 dark:border-dark-600" />

        <!-- Basic Info -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.name') }}</label>
            <input
              v-model="form.name"
              type="text"
              required
              class="input"
              :placeholder="t('admin.tlsFingerprintProfiles.form.namePlaceholder')"
            />
          </div>
          <div>
            <label class="input-label">{{ t('admin.tlsFingerprintProfiles.form.description') }}</label>
            <input
              v-model="form.description"
              type="text"
              class="input"
              :placeholder="t('admin.tlsFingerprintProfiles.form.descriptionPlaceholder')"
            />
          </div>
        </div>

        <!-- GREASE Toggle -->
        <div class="flex items-center gap-3">
          <button
            type="button"
            @click="form.enable_grease = !form.enable_grease"
            :class="[
              'relative inline-flex h-5 w-9 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
              form.enable_grease ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
            ]"
          >
            <span
              :class="[
                'pointer-events-none inline-block h-4 w-4 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
                form.enable_grease ? 'translate-x-4' : 'translate-x-0'
              ]"
            />
          </button>
          <div>
            <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.tlsFingerprintProfiles.form.enableGrease') }}
            </span>
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.tlsFingerprintProfiles.form.enableGreaseHint') }}
            </p>
          </div>
        </div>

        <!-- TLS Array Fields - 2 column grid -->
        <div class="grid grid-cols-2 gap-4">
          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.cipherSuites') }}</label>
            <textarea
              v-model="fieldInputs.cipher_suites"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x1301, 0x1302, 0xc02c'"
            />
            <p class="input-hint text-xs">{{ t('admin.tlsFingerprintProfiles.form.cipherSuitesHint') }}</p>
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.curves') }}</label>
            <textarea
              v-model="fieldInputs.curves"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'29, 23, 24'"
            />
            <p class="input-hint text-xs">{{ t('admin.tlsFingerprintProfiles.form.curvesHint') }}</p>
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.signatureAlgorithms') }}</label>
            <textarea
              v-model="fieldInputs.signature_algorithms"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0403, 0x0804, 0x0401'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.supportedVersions') }}</label>
            <textarea
              v-model="fieldInputs.supported_versions"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0304, 0x0303'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.keyShareGroups') }}</label>
            <textarea
              v-model="fieldInputs.key_share_groups"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'29, 23'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.extensions') }}</label>
            <textarea
              v-model="fieldInputs.extensions"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0x0000, 0x0005, 0x000a'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.pointFormats') }}</label>
            <textarea
              v-model="fieldInputs.point_formats"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'0'"
            />
          </div>

          <div>
            <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.pskModes') }}</label>
            <textarea
              v-model="fieldInputs.psk_modes"
              rows="2"
              class="input font-mono text-xs"
              :placeholder="'1'"
            />
          </div>
        </div>

        <!-- ALPN Protocols - full width -->
        <div>
          <label class="input-label text-xs">{{ t('admin.tlsFingerprintProfiles.form.alpnProtocols') }}</label>
          <textarea
            v-model="fieldInputs.alpn_protocols"
            rows="2"
            class="input font-mono text-xs"
            :placeholder="'h2, http/1.1'"
          />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button @click="closeFormModal" type="button" class="btn btn-secondary">
            {{ t('common.cancel') }}
          </button>
          <button @click="handleSubmit" :disabled="submitting" class="btn btn-primary">
            <Icon v-if="submitting" name="refresh" size="sm" class="mr-1 animate-spin" />
            {{ showEditModal ? t('common.update') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.tlsFingerprintProfiles.deleteProfile')"
      :message="t('admin.tlsFingerprintProfiles.deleteConfirmMessage', { name: deletingProfile?.name })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type {
  TLSFingerprintCaptureRecord,
  TLSFingerprintCollectorSession,
  TLSFingerprintCollectorStatus,
  TLSFingerprintProfile
} from '@/api/admin/tlsFingerprintProfile'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

// eslint-disable-next-line @typescript-eslint/no-unused-vars
void emit // suppress unused warning - emit is used via $emit in template

const { t } = useI18n()
const appStore = useAppStore()

const profiles = ref<TLSFingerprintProfile[]>([])
const loading = ref(false)
const submitting = ref(false)
const showCreateModal = ref(false)
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const editingProfile = ref<TLSFingerprintProfile | null>(null)
const deletingProfile = ref<TLSFingerprintProfile | null>(null)
const yamlInput = ref('')
const collectorStatus = ref<TLSFingerprintCollectorStatus | null>(null)
const collectorSession = ref<TLSFingerprintCollectorSession | null>(null)
const collectorCaptures = ref<TLSFingerprintCaptureRecord[]>([])
const collectorLoading = ref(false)
const collectorActionLoading = ref(false)
const collectorSessionLoading = ref(false)
const collectorCapturesLoading = ref(false)
let collectorPollTimer: number | null = null

// Raw string inputs for array fields
const fieldInputs = reactive({
  cipher_suites: '',
  curves: '',
  point_formats: '',
  signature_algorithms: '',
  alpn_protocols: '',
  supported_versions: '',
  key_share_groups: '',
  psk_modes: '',
  extensions: ''
})

const form = reactive({
  name: '',
  description: null as string | null,
  enable_grease: false
})

const isCollectorRunning = computed(() => Boolean(collectorStatus.value?.running))
const activeCAPEM = computed(() => collectorSession.value?.ca_pem || collectorStatus.value?.ca_pem || '')
const claudeCommand = computed(() => {
  if (!collectorSession.value) return ''
  const tlsPrefix = activeCAPEM.value ? 'NODE_TLS_REJECT_UNAUTHORIZED=0 ' : ''
  const settings = {
    env: {
      ANTHROPIC_BASE_URL: collectorSession.value.capture_url,
      ANTHROPIC_AUTH_TOKEN: collectorSession.value.token
    }
  }
  // Claude Code prefers its own settings.env, so pass --settings to force the capture URL over any global config.
  return `${tlsPrefix}claude --settings '${JSON.stringify(settings)}' "test"`
})
const codexCommand = computed(() => {
  if (!collectorSession.value) return ''
  const caPrefix = activeCAPEM.value ? 'CODEX_CA_CERTIFICATE=/path/to/sub2api-tls-collector-ca.pem ' : ''
  const captureURL = collectorSession.value.capture_url
  return `${caPrefix}codex -c 'openai_base_url="${captureURL}"' -c 'chatgpt_base_url="${captureURL}"'`
})
const codexConfigSnippet = computed(() => {
  if (!collectorSession.value) return ''
  return [
    `openai_base_url = "${collectorSession.value.capture_url}"`,
    `chatgpt_base_url = "${collectorSession.value.capture_url}"`
  ].join('\n')
})

// Load profiles and collector status when the dialog opens; stop polling when it closes.
watch(() => props.show, (newVal) => {
  if (newVal) {
    loadProfiles()
    loadCollectorStatus()
  } else {
    stopCollectorPolling()
  }
})

onUnmounted(() => {
  stopCollectorPolling()
})

const loadProfiles = async () => {
  loading.value = true
  try {
    profiles.value = await adminAPI.tlsFingerprintProfiles.list()
  } catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.loadFailed'))
    console.error('Error loading TLS fingerprint profiles:', error)
  } finally {
    loading.value = false
  }
}

const loadCollectorStatus = async () => {
  collectorLoading.value = true
  try {
    collectorStatus.value = await adminAPI.tlsFingerprintProfiles.collectorStatus()
  } catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.collector.statusFailed'))
    console.error('Error loading TLS fingerprint collector status:', error)
  } finally {
    collectorLoading.value = false
  }
}

const toggleCollector = async () => {
  collectorActionLoading.value = true
  try {
    if (isCollectorRunning.value) {
      await adminAPI.tlsFingerprintProfiles.stopCollector()
      collectorSession.value = null
      collectorCaptures.value = []
      stopCollectorPolling()
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.stopSuccess'))
    } else {
      collectorStatus.value = await adminAPI.tlsFingerprintProfiles.startCollector()
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.startSuccess'))
    }
    await loadCollectorStatus()
  } catch (error: any) {
    const message = error?.message || error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.collector.actionFailed')
    appStore.showError(message)
    console.error('Error toggling TLS fingerprint collector:', error)
  } finally {
    collectorActionLoading.value = false
  }
}

const createCollectorSession = async () => {
  collectorSessionLoading.value = true
  try {
    collectorSession.value = await adminAPI.tlsFingerprintProfiles.createCollectorSession()
    collectorCaptures.value = []
    startCollectorPolling()
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.sessionCreated'))
  } catch (error: any) {
    const message = error?.message || error?.response?.data?.detail || t('admin.tlsFingerprintProfiles.collector.sessionFailed')
    appStore.showError(message)
    console.error('Error creating TLS fingerprint collector session:', error)
  } finally {
    collectorSessionLoading.value = false
  }
}

const refreshCollectorCaptures = async () => {
  if (!collectorSession.value) return
  collectorCapturesLoading.value = true
  try {
    collectorCaptures.value = await adminAPI.tlsFingerprintProfiles.listCollectorCaptures(collectorSession.value.token)
  } catch (error: any) {
    console.error('Error loading TLS fingerprint captures:', error)
  } finally {
    collectorCapturesLoading.value = false
  }
}

const startCollectorPolling = () => {
  stopCollectorPolling()
  collectorPollTimer = window.setInterval(() => {
    refreshCollectorCaptures()
  }, 3000)
  refreshCollectorCaptures()
}

const stopCollectorPolling = () => {
  if (collectorPollTimer) {
    window.clearInterval(collectorPollTimer)
    collectorPollTimer = null
  }
}

const applyCapture = (record: TLSFingerprintCaptureRecord) => {
  if (!record.profile) return
  fillFormFromProfile(record.profile)
  yamlInput.value = record.yaml || ''
  editingProfile.value = null
  showEditModal.value = false
  showCreateModal.value = true
  appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.applied'))
}

const copyText = async (text: string) => {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.collector.copied'))
  } catch (error) {
    appStore.showError(t('admin.tlsFingerprintProfiles.collector.copyFailed'))
    console.error('Error copying text:', error)
  }
}

const downloadCA = () => {
  if (!activeCAPEM.value) return
  const blob = new Blob([activeCAPEM.value], { type: 'application/x-pem-file' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'sub2api-tls-collector-ca.pem'
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const formatDateTime = (value?: string) => {
  if (!value) return '—'
  return new Date(value).toLocaleString()
}

const formatClientKind = (kind: string) => {
  if (kind === 'claude_code') return 'Claude Code'
  if (kind === 'codex') return 'Codex CLI'
  return t('admin.tlsFingerprintProfiles.collector.unknownClient')
}

const resetForm = () => {
  form.name = ''
  form.description = null
  form.enable_grease = false
  fieldInputs.cipher_suites = ''
  fieldInputs.curves = ''
  fieldInputs.point_formats = ''
  fieldInputs.signature_algorithms = ''
  fieldInputs.alpn_protocols = ''
  fieldInputs.supported_versions = ''
  fieldInputs.key_share_groups = ''
  fieldInputs.psk_modes = ''
  fieldInputs.extensions = ''
  yamlInput.value = ''
}

/**
 * Parse YAML output from tls-fingerprint-web and fill form fields.
 * Expected format:
 *   # comment lines
 *   profile_key:
 *     name: "Profile Name"
 *     enable_grease: false
 *     cipher_suites: [4866, 4867, ...]
 *     alpn_protocols: ["h2", "http/1.1"]
 *     ...
 */
const parseYamlInput = () => {
  const text = yamlInput.value.trim()
  if (!text) return

  // Simple YAML parser for flat key-value structure
  // Extracts "key: value" lines, handling arrays like [1, 2, 3] and ["h2", "http/1.1"]
  const lines = text.split('\n')

  let foundName = false

  for (const line of lines) {
    const trimmed = line.trim()
    // Skip comments and empty lines
    if (!trimmed || trimmed.startsWith('#')) continue

    // Match "key: value" pattern (must have at least 2 leading spaces to be a property)
    const match = trimmed.match(/^(\w+):\s*(.+)$/)
    if (!match) continue

    const [, key, rawValue] = match
    const value = rawValue.trim()

    switch (key) {
      case 'name': {
        // Remove surrounding quotes
        const unquoted = value.replace(/^["']|["']$/g, '')
        if (unquoted) {
          form.name = unquoted
          foundName = true
        }
        break
      }
      case 'enable_grease':
        form.enable_grease = value === 'true'
        break
      case 'cipher_suites':
      case 'curves':
      case 'point_formats':
      case 'signature_algorithms':
      case 'supported_versions':
      case 'key_share_groups':
      case 'psk_modes':
      case 'extensions': {
        // Parse YAML array: [1, 2, 3] — values are decimal integers from tls-fingerprint-web
        const arrMatch = value.match(/^\[(.*)?\]$/)
        if (arrMatch) {
          const inner = arrMatch[1] || ''
          fieldInputs[key as keyof typeof fieldInputs] = inner
            .split(',')
            .map(s => s.trim())
            .filter(s => s.length > 0)
            .join(', ')
        }
        break
      }
      case 'alpn_protocols': {
        // Parse string array: ["h2", "http/1.1"]
        const arrMatch = value.match(/^\[(.*)?\]$/)
        if (arrMatch) {
          const inner = arrMatch[1] || ''
          fieldInputs.alpn_protocols = inner
            .split(',')
            .map(s => s.trim().replace(/^["']|["']$/g, ''))
            .filter(s => s.length > 0)
            .join(', ')
        }
        break
      }
    }
  }

  if (foundName) {
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.form.yamlParsed'))
  } else {
    appStore.showError(t('admin.tlsFingerprintProfiles.form.yamlParseFailed'))
  }
}

// Auto-parse on paste event
const handleYamlPaste = () => {
  // Use nextTick to ensure v-model has updated
  setTimeout(() => parseYamlInput(), 50)
}

const closeFormModal = () => {
  showCreateModal.value = false
  showEditModal.value = false
  editingProfile.value = null
  resetForm()
}

// Parse a comma-separated string of numbers supporting both hex (0x...) and decimal
const parseNumericArray = (input: string): number[] => {
  if (!input.trim()) return []
  return input
    .split(',')
    .map(s => s.trim())
    .filter(s => s.length > 0)
    .map(s => s.startsWith('0x') || s.startsWith('0X') ? parseInt(s, 16) : parseInt(s, 10))
    .filter(n => !isNaN(n))
}

// Parse a comma-separated string of string values
const parseStringArray = (input: string): string[] => {
  if (!input.trim()) return []
  return input
    .split(',')
    .map(s => s.trim())
    .filter(s => s.length > 0)
}

// Format a number as hex with 0x prefix and 4-digit padding
const formatHex = (n: number): string => '0x' + n.toString(16).padStart(4, '0')

// Format numeric arrays for display in textarea (null-safe)
const formatNumericArray = (arr: number[] | null | undefined): string => (arr ?? []).map(formatHex).join(', ')

// For point_formats and psk_modes (uint8), show as plain numbers (null-safe)
const formatPlainNumericArray = (arr: number[] | null | undefined): string => (arr ?? []).join(', ')

// Populate the create/edit form fields from an existing profile.
const fillFormFromProfile = (profile: TLSFingerprintProfile) => {
  form.name = profile.name
  form.description = profile.description
  form.enable_grease = profile.enable_grease
  fieldInputs.cipher_suites = formatNumericArray(profile.cipher_suites)
  fieldInputs.curves = formatPlainNumericArray(profile.curves)
  fieldInputs.point_formats = formatPlainNumericArray(profile.point_formats)
  fieldInputs.signature_algorithms = formatNumericArray(profile.signature_algorithms)
  fieldInputs.alpn_protocols = (profile.alpn_protocols ?? []).join(', ')
  fieldInputs.supported_versions = formatNumericArray(profile.supported_versions)
  fieldInputs.key_share_groups = formatPlainNumericArray(profile.key_share_groups)
  fieldInputs.psk_modes = formatPlainNumericArray(profile.psk_modes)
  fieldInputs.extensions = formatNumericArray(profile.extensions)
}

const handleEdit = (profile: TLSFingerprintProfile) => {
  editingProfile.value = profile
  fillFormFromProfile(profile)
  showEditModal.value = true
}

const handleDelete = (profile: TLSFingerprintProfile) => {
  deletingProfile.value = profile
  showDeleteDialog.value = true
}

const handleSubmit = async () => {
  if (!form.name.trim()) {
    appStore.showError(t('admin.tlsFingerprintProfiles.form.name') + ' ' + t('common.required'))
    return
  }

  submitting.value = true
  try {
    const data = {
      name: form.name.trim(),
      description: form.description?.trim() || null,
      enable_grease: form.enable_grease,
      cipher_suites: parseNumericArray(fieldInputs.cipher_suites),
      curves: parseNumericArray(fieldInputs.curves),
      point_formats: parseNumericArray(fieldInputs.point_formats),
      signature_algorithms: parseNumericArray(fieldInputs.signature_algorithms),
      alpn_protocols: parseStringArray(fieldInputs.alpn_protocols),
      supported_versions: parseNumericArray(fieldInputs.supported_versions),
      key_share_groups: parseNumericArray(fieldInputs.key_share_groups),
      psk_modes: parseNumericArray(fieldInputs.psk_modes),
      extensions: parseNumericArray(fieldInputs.extensions)
    }

    if (showEditModal.value && editingProfile.value) {
      await adminAPI.tlsFingerprintProfiles.update(editingProfile.value.id, data)
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.updateSuccess'))
    } else {
      await adminAPI.tlsFingerprintProfiles.create(data)
      appStore.showSuccess(t('admin.tlsFingerprintProfiles.createSuccess'))
    }

    closeFormModal()
    loadProfiles()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.tlsFingerprintProfiles.saveFailed'))
    console.error('Error saving TLS fingerprint profile:', error)
  } finally {
    submitting.value = false
  }
}

const confirmDelete = async () => {
  if (!deletingProfile.value) return

  try {
    await adminAPI.tlsFingerprintProfiles.delete(deletingProfile.value.id)
    appStore.showSuccess(t('admin.tlsFingerprintProfiles.deleteSuccess'))
    showDeleteDialog.value = false
    deletingProfile.value = null
    loadProfiles()
  } catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.tlsFingerprintProfiles.deleteFailed'))
    console.error('Error deleting TLS fingerprint profile:', error)
  }
}
</script>
