<template>
  <Modal :show="show" :title="title" size="sm" @close="handleCancel">
    <div class="space-y-4">
      <p class="text-sm text-gray-600 dark:text-gray-400">{{ message REDACTEDREDACTED</p>
    </div>

    <template #footer>
      <div class="flex justify-end space-x-3">
        <button
          @click="handleCancel"
          type="button"
          class="px-4 py-2 text-sm font-medium text-gray-700 dark:text-gray-200 bg-white dark:bg-dark-700 border border-gray-300 dark:border-dark-600 rounded-md hover:bg-gray-50 dark:hover:bg-dark-600 focus:outline-none focus:ring-2 focus:ring-offset-2 dark:focus:ring-offset-dark-800 focus:ring-primary-500"
        >
          {{ cancelText REDACTEDREDACTED
        </button>
        <button
          @click="handleConfirm"
          type="button"
          :class="[
            'px-4 py-2 text-sm font-medium text-white rounded-md focus:outline-none focus:ring-2 focus:ring-offset-2 dark:focus:ring-offset-dark-800',
            danger
              ? 'bg-red-600 hover:bg-red-700 focus:ring-red-500'
              : 'bg-primary-600 hover:bg-primary-700 focus:ring-primary-500'
          ]"
        >
          {{ confirmText REDACTEDREDACTED
        </button>
      </div>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import Modal from './Modal.vue'

interface Props {
  show: boolean
  title: string
  message: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
REDACTED

interface Emits {
  (e: 'confirm'): void
  (e: 'cancel'): void
REDACTED

const props = withDefaults(defineProps<Props>(), {
  confirmText: 'Confirm',
  cancelText: 'Cancel',
  danger: false
REDACTED)

const emit = defineEmits<Emits>()

const handleConfirm = () => {
  emit('confirm')
REDACTED

const handleCancel = () => {
  emit('cancel')
REDACTED
</script>
