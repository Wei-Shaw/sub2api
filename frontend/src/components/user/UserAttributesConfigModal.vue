<template>
  <BaseDialog :show="show" :title="t('admin.users.attributes.title')" width="wide" @close="emit('close')">
    <div class="space-y-4">
      <!-- Header with Add Button -->
      <div class="flex items-center justify-between">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.users.attributes.description') REDACTEDREDACTED
        </p>
        <button @click="openCreateModal" class="btn btn-primary btn-sm">
          <svg class="mr-1.5 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          {{ t('admin.users.attributes.addAttribute') REDACTEDREDACTED
        </button>
      </div>

      <!-- Loading State -->
      <div v-if="loading" class="flex justify-center py-12">
        <svg class="h-8 w-8 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
          <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
          <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
      </div>

      <!-- Empty State -->
      <div v-else-if="attributes.length === 0" class="py-12 text-center">
        <svg class="mx-auto h-12 w-12 text-gray-400" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9.568 3H5.25A2.25 2.25 0 003 5.25v4.318c0 .597.237 1.17.659 1.591l9.581 9.581c.699.699 1.78.872 2.607.33a18.095 18.095 0 005.223-5.223c.542-.827.369-1.908-.33-2.607L11.16 3.66A2.25 2.25 0 009.568 3z" />
          <path stroke-linecap="round" stroke-linejoin="round" d="M6 6h.008v.008H6V6z" />
        </svg>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ t('admin.users.attributes.noAttributes') REDACTEDREDACTED
        </p>
        <p class="text-xs text-gray-400 dark:text-dark-500">
          {{ t('admin.users.attributes.noAttributesHint') REDACTEDREDACTED
        </p>
      </div>

      <!-- Attributes List -->
      <div v-else class="max-h-96 space-y-2 overflow-y-auto">
        <div
          v-for="attr in attributes"
          :key="attr.id"
          class="flex items-center gap-3 rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-600 dark:bg-dark-800"
        >
          <!-- Drag Handle -->
          <div class="cursor-move text-gray-400 hover:text-gray-600 dark:hover:text-gray-300" :title="t('admin.users.attributes.dragToReorder')">
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M3.75 6.75h16.5M3.75 12h16.5m-16.5 5.25h16.5" />
            </svg>
          </div>

          <!-- Attribute Info -->
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-white">{{ attr.name REDACTEDREDACTED</span>
              <span class="rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs text-gray-500 dark:bg-dark-700 dark:text-dark-400">
                {{ attr.key REDACTEDREDACTED
              </span>
              <span v-if="attr.required" class="badge badge-danger text-xs">
                {{ t('admin.users.attributes.required') REDACTEDREDACTED
              </span>
              <span v-if="!attr.enabled" class="badge badge-gray text-xs">
                {{ t('common.disabled') REDACTEDREDACTED
              </span>
            </div>
            <div class="mt-0.5 flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
              <span class="badge badge-gray">{{ t(`admin.users.attributes.types.${attr.typeREDACTED`) REDACTEDREDACTED</span>
              <span v-if="attr.description" class="truncate">{{ attr.description REDACTEDREDACTED</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center gap-1">
            <button
              @click="openEditModal(attr)"
              class="rounded-lg p-1.5 text-gray-500 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
              :title="t('common.edit')"
            >
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M16.862 4.487l1.687-1.688a1.875 1.875 0 112.652 2.652L10.582 16.07a4.5 4.5 0 01-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 011.13-1.897l8.932-8.931zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0115.75 21H5.25A2.25 2.25 0 013 18.75V8.25A2.25 2.25 0 015.25 6H10" />
              </svg>
            </button>
            <button
              @click="confirmDelete(attr)"
              class="rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
              :title="t('common.delete')"
            >
              <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M14.74 9l-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 01-2.244 2.077H8.084a2.25 2.25 0 01-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 00-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 013.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 00-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 00-7.5 0" />
              </svg>
            </button>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button @click="emit('close')" class="btn btn-secondary">
          {{ t('common.close') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Create/Edit Attribute Modal -->
  <BaseDialog
    :show="showEditModal"
    :title="editingAttribute ? t('admin.users.attributes.editAttribute') : t('admin.users.attributes.addAttribute')"
    width="normal"
    @close="closeEditModal"
  >
    <form id="attribute-form" @submit.prevent="handleSave" class="space-y-4">
      <!-- Key -->
      <div>
        <label class="input-label">{{ t('admin.users.attributes.key') REDACTEDREDACTED</label>
        <input
          v-model="form.key"
          type="text"
          required
          pattern="^[a-zA-Z][a-zA-Z0-9_]*$"
          class="input font-mono"
          :placeholder="t('admin.users.attributes.keyHint')"
          :disabled="!!editingAttribute"
        />
        <p class="input-hint">{{ t('admin.users.attributes.keyHint') REDACTEDREDACTED</p>
      </div>

      <!-- Name -->
      <div>
        <label class="input-label">{{ t('admin.users.attributes.name') REDACTEDREDACTED</label>
        <input
          v-model="form.name"
          type="text"
          required
          class="input"
          :placeholder="t('admin.users.attributes.nameHint')"
        />
      </div>

      <!-- Type -->
      <div>
        <label class="input-label">{{ t('admin.users.attributes.type') REDACTEDREDACTED</label>
        <Select
          v-model="form.type"
          :options="attributeTypes.map(type => ({ value: type, label: t(`admin.users.attributes.types.${typeREDACTED`) REDACTED))"
        />
      </div>

      <!-- Options (for select/multi_select) -->
      <div v-if="form.type === 'select' || form.type === 'multi_select'" class="space-y-2">
        <label class="input-label">{{ t('admin.users.attributes.options') REDACTEDREDACTED</label>
        <div v-for="(option, index) in form.options" :key="index" class="flex items-center gap-2">
          <input
            v-model="option.value"
            type="text"
            class="input flex-1 font-mono text-sm"
            :placeholder="t('admin.users.attributes.optionValue')"
            required
          />
          <input
            v-model="option.label"
            type="text"
            class="input flex-1 text-sm"
            :placeholder="t('admin.users.attributes.optionLabel')"
            required
          />
          <button
            type="button"
            @click="removeOption(index)"
            class="rounded-lg p-1.5 text-gray-500 hover:bg-red-50 hover:text-red-600"
          >
            <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
            </svg>
          </button>
        </div>
        <button type="button" @click="addOption" class="btn btn-secondary btn-sm">
          <svg class="mr-1 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M12 4.5v15m7.5-7.5h-15" />
          </svg>
          {{ t('admin.users.attributes.addOption') REDACTEDREDACTED
        </button>
      </div>

      <!-- Description -->
      <div>
        <label class="input-label">{{ t('admin.users.attributes.fieldDescription') REDACTEDREDACTED</label>
        <input
          v-model="form.description"
          type="text"
          class="input"
          :placeholder="t('admin.users.attributes.fieldDescriptionHint')"
        />
      </div>

      <!-- Placeholder -->
      <div>
        <label class="input-label">{{ t('admin.users.attributes.placeholder') REDACTEDREDACTED</label>
        <input
          v-model="form.placeholder"
          type="text"
          class="input"
          :placeholder="t('admin.users.attributes.placeholderHint')"
        />
      </div>

      <!-- Required & Enabled -->
      <div class="flex items-center gap-6">
        <label class="flex items-center gap-2">
          <input v-model="form.required" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.users.attributes.required') REDACTEDREDACTED</span>
        </label>
        <label class="flex items-center gap-2">
          <input v-model="form.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.users.attributes.enabled') REDACTEDREDACTED</span>
        </label>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button @click="closeEditModal" type="button" class="btn btn-secondary">
          {{ t('common.cancel') REDACTEDREDACTED
        </button>
        <button type="submit" form="attribute-form" :disabled="saving" class="btn btn-primary">
          <svg v-if="saving" class="-ml-1 mr-2 h-4 w-4 animate-spin" fill="none" viewBox="0 0 24 24">
            <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
            <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {{ saving ? t('common.saving') : (editingAttribute ? t('common.update') : t('common.create')) REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>

  <!-- Delete Confirmation -->
  <ConfirmDialog
    :show="showDeleteDialog"
    :title="t('admin.users.attributes.deleteAttribute')"
    :message="t('admin.users.attributes.deleteConfirm', { name: deletingAttribute?.name REDACTED)"
    :confirm-text="t('common.delete')"
    :cancel-text="t('common.cancel')"
    :danger="true"
    @confirm="handleDelete"
    @cancel="showDeleteDialog = false"
  />
</template>

<script setup lang="ts">
import { ref, reactive, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { UserAttributeDefinition, UserAttributeType, UserAttributeOption REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'

const { t REDACTED = useI18n()
const appStore = useAppStore()

interface Props {
  show: boolean
REDACTED

interface Emits {
  (e: 'close'): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const attributeTypes: UserAttributeType[] = ['text', 'textarea', 'number', 'email', 'url', 'date', 'select', 'multi_select']

const loading = ref(false)
const saving = ref(false)
const attributes = ref<UserAttributeDefinition[]>([])
const showEditModal = ref(false)
const showDeleteDialog = ref(false)
const editingAttribute = ref<UserAttributeDefinition | null>(null)
const deletingAttribute = ref<UserAttributeDefinition | null>(null)

const form = reactive({
  key: '',
  name: '',
  type: 'text' as UserAttributeType,
  description: '',
  placeholder: '',
  required: false,
  enabled: true,
  options: [] as UserAttributeOption[]
REDACTED)

const loadAttributes = async () => {
  loading.value = true
  try {
    attributes.value = await adminAPI.userAttributes.listDefinitions()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.attributes.failedToLoad'))
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const openCreateModal = () => {
  editingAttribute.value = null
  form.key = ''
  form.name = ''
  form.type = 'text'
  form.description = ''
  form.placeholder = ''
  form.required = false
  form.enabled = true
  form.options = []
  showEditModal.value = true
REDACTED

const openEditModal = (attr: UserAttributeDefinition) => {
  editingAttribute.value = attr
  form.key = attr.key
  form.name = attr.name
  form.type = attr.type
  form.description = attr.description || ''
  form.placeholder = attr.placeholder || ''
  form.required = attr.required
  form.enabled = attr.enabled
  form.options = attr.options ? [...attr.options] : []
  showEditModal.value = true
REDACTED

const closeEditModal = () => {
  showEditModal.value = false
  editingAttribute.value = null
REDACTED

const addOption = () => {
  form.options.push({ value: '', label: '' REDACTED)
REDACTED

const removeOption = (index: number) => {
  form.options.splice(index, 1)
REDACTED

const handleSave = async () => {
  if (!form.key.trim()) {
    appStore.showError(t('admin.users.attributes.keyRequired'))
    return
  REDACTED
  if (!form.name.trim()) {
    appStore.showError(t('admin.users.attributes.nameRequired'))
    return
  REDACTED
  if ((form.type === 'select' || form.type === 'multi_select') && form.options.length === 0) {
    appStore.showError(t('admin.users.attributes.optionsRequired'))
    return
  REDACTED
  saving.value = true
  try {
    const data = {
      key: form.key,
      name: form.name,
      type: form.type,
      description: form.description || undefined,
      placeholder: form.placeholder || undefined,
      required: form.required,
      enabled: form.enabled,
      options: (form.type === 'select' || form.type === 'multi_select') ? form.options : undefined
    REDACTED

    if (editingAttribute.value) {
      await adminAPI.userAttributes.updateDefinition(editingAttribute.value.id, data)
      appStore.showSuccess(t('admin.users.attributes.updated'))
    REDACTED else {
      await adminAPI.userAttributes.createDefinition(data)
      appStore.showSuccess(t('admin.users.attributes.created'))
    REDACTED

    closeEditModal()
    loadAttributes()
  REDACTED catch (error: any) {
    const msg = editingAttribute.value
      ? t('admin.users.attributes.failedToUpdate')
      : t('admin.users.attributes.failedToCreate')
    appStore.showError(error.response?.data?.detail || msg)
  REDACTED finally {
    saving.value = false
  REDACTED
REDACTED

const confirmDelete = (attr: UserAttributeDefinition) => {
  deletingAttribute.value = attr
  showDeleteDialog.value = true
REDACTED

const handleDelete = async () => {
  if (!deletingAttribute.value) return

  try {
    await adminAPI.userAttributes.deleteDefinition(deletingAttribute.value.id)
    appStore.showSuccess(t('admin.users.attributes.deleted'))
    showDeleteDialog.value = false
    deletingAttribute.value = null
    loadAttributes()
  REDACTED catch (error: any) {
    appStore.showError(error.response?.data?.detail || t('admin.users.attributes.failedToDelete'))
  REDACTED
REDACTED

watch(() => props.show, (isShow) => {
  if (isShow) {
    loadAttributes()
  REDACTED
REDACTED)
</script>
