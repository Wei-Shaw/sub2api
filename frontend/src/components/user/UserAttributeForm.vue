<template>
  <div v-if="attributes.length > 0" class="space-y-4">
    <div v-for="attr in attributes" :key="attr.id">
      <label class="input-label">
        {{ attr.name REDACTEDREDACTED
        <span v-if="attr.required" class="text-red-500">*</span>
      </label>

      <!-- Text Input -->
      <input
        v-if="attr.type === 'text' || attr.type === 'email' || attr.type === 'url'"
        v-model="localValues[attr.id]"
        :type="attr.type === 'text' ? 'text' : attr.type"
        :required="attr.required"
        :placeholder="attr.placeholder"
        class="input"
        @input="emitChange"
      />

      <!-- Number Input -->
      <input
        v-else-if="attr.type === 'number'"
        v-model.number="localValues[attr.id]"
        type="number"
        :required="attr.required"
        :placeholder="attr.placeholder"
        :min="attr.validation?.min"
        :max="attr.validation?.max"
        class="input"
        @input="emitChange"
      />

      <!-- Date Input -->
      <input
        v-else-if="attr.type === 'date'"
        v-model="localValues[attr.id]"
        type="date"
        :required="attr.required"
        class="input"
        @input="emitChange"
      />

      <!-- Textarea -->
      <textarea
        v-else-if="attr.type === 'textarea'"
        v-model="localValues[attr.id]"
        :required="attr.required"
        :placeholder="attr.placeholder"
        rows="3"
        class="input"
        @input="emitChange"
      />

      <!-- Select -->
      <select
        v-else-if="attr.type === 'select'"
        v-model="localValues[attr.id]"
        :required="attr.required"
        class="input"
        @change="emitChange"
      >
        <option value="">{{ t('common.selectOption') REDACTEDREDACTED</option>
        <option v-for="opt in attr.options" :key="opt.value" :value="opt.value">
          {{ opt.label REDACTEDREDACTED
        </option>
      </select>

      <!-- Multi-Select (Checkboxes) -->
      <div v-else-if="attr.type === 'multi_select'" class="space-y-2">
        <label
          v-for="opt in attr.options"
          :key="opt.value"
          class="flex items-center gap-2"
        >
          <input
            type="checkbox"
            :value="opt.value"
            :checked="isOptionSelected(attr.id, opt.value)"
            @change="toggleMultiSelectOption(attr.id, opt.value)"
            class="h-4 w-4 rounded border-gray-300 text-primary-600"
          />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ opt.label REDACTEDREDACTED</span>
        </label>
      </div>

      <!-- Description -->
      <p v-if="attr.description" class="input-hint">{{ attr.description REDACTEDREDACTED</p>
    </div>
  </div>

  <!-- Loading State -->
  <div v-else-if="loading" class="flex justify-center py-4">
    <svg class="h-5 w-5 animate-spin text-gray-400" fill="none" viewBox="0 0 24 24">
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
    </svg>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type { UserAttributeDefinition, UserAttributeValuesMap REDACTED from '@/types'

const { t REDACTED = useI18n()

interface Props {
  userId?: number
  modelValue: UserAttributeValuesMap
REDACTED

interface Emits {
  (e: 'update:modelValue', value: UserAttributeValuesMap): void
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const loading = ref(false)
const attributes = ref<UserAttributeDefinition[]>([])
const localValues = ref<UserAttributeValuesMap>({REDACTED)

const loadAttributes = async () => {
  loading.value = true
  try {
    attributes.value = await adminAPI.userAttributes.listEnabledDefinitions()
  REDACTED catch (error) {
    console.error('Failed to load attributes:', error)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const loadUserValues = async () => {
  if (!props.userId) return

  try {
    const values = await adminAPI.userAttributes.getUserAttributeValues(props.userId)
    const valuesMap: UserAttributeValuesMap = {REDACTED
    values.forEach(v => {
      valuesMap[v.attribute_id] = v.value
    REDACTED)
    localValues.value = { ...valuesMap REDACTED
    emit('update:modelValue', localValues.value)
  REDACTED catch (error) {
    console.error('Failed to load user attribute values:', error)
  REDACTED
REDACTED

const emitChange = () => {
  emit('update:modelValue', { ...localValues.value REDACTED)
REDACTED

const isOptionSelected = (attrId: number, optionValue: string): boolean => {
  const value = localValues.value[attrId]
  if (!value) return false
  try {
    const arr = JSON.parse(value)
    return Array.isArray(arr) && arr.includes(optionValue)
  REDACTED catch {
    return false
  REDACTED
REDACTED

const toggleMultiSelectOption = (attrId: number, optionValue: string) => {
  let arr: string[] = []
  const value = localValues.value[attrId]
  if (value) {
    try {
      arr = JSON.parse(value)
      if (!Array.isArray(arr)) arr = []
    REDACTED catch {
      arr = []
    REDACTED
  REDACTED

  const index = arr.indexOf(optionValue)
  if (index > -1) {
    arr.splice(index, 1)
  REDACTED else {
    arr.push(optionValue)
  REDACTED

  localValues.value[attrId] = JSON.stringify(arr)
  emitChange()
REDACTED

watch(() => props.modelValue, (newVal) => {
  if (newVal && Object.keys(newVal).length > 0) {
    localValues.value = { ...newVal REDACTED
  REDACTED
REDACTED, { immediate: true REDACTED)

watch(() => props.userId, (newUserId) => {
  if (newUserId) {
    loadUserValues()
  REDACTED else {
    // Reset for new user
    localValues.value = {REDACTED
  REDACTED
REDACTED, { immediate: true REDACTED)

onMounted(() => {
  loadAttributes()
REDACTED)
</script>
