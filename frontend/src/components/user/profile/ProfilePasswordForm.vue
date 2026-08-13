<template>
  <div :class="props.embedded ? 'space-y-4' : 'rounded border border-line bg-surface'">
    <div v-if="!props.embedded" class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.changePassword') }}
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-4 py-4'">
      <form class="space-y-3" @submit.prevent="handleChangePassword">
        <p v-if="props.embedded" class="text-sm font-semibold text-ink">
          {{ t('profile.changePassword') }}
        </p>

        <!--
          Both validation failures ("passwords do not match", "too short") used
          to reach the user only as a toast, so the field at fault was never
          marked and the message expired on a timer. They are inline now — the
          toast is kept because the form can be scrolled out of view.
        -->
        <FormField
          id="old_password"
          :label="t('profile.currentPassword')"
          :error="errors.old_password"
        >
          <template #default="{ describedBy, invalid }">
            <input
              id="old_password"
              v-model="form.old_password"
              type="password"
              required
              autocomplete="current-password"
              class="input"
              :class="{ 'input-error': errors.old_password }"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
            />
          </template>
        </FormField>

        <FormField
          id="new_password"
          :label="t('profile.newPassword')"
          :hint="t('profile.passwordHint')"
          :error="errors.new_password"
        >
          <template #default="{ describedBy, invalid }">
            <input
              id="new_password"
              v-model="form.new_password"
              type="password"
              required
              autocomplete="new-password"
              class="input"
              :class="{ 'input-error': errors.new_password }"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
            />
          </template>
        </FormField>

        <FormField
          id="confirm_password"
          :label="t('profile.confirmNewPassword')"
          :error="errors.confirm_password"
        >
          <template #default="{ describedBy, invalid }">
            <input
              id="confirm_password"
              v-model="form.confirm_password"
              type="password"
              required
              autocomplete="new-password"
              class="input"
              :class="{ 'input-error': errors.confirm_password }"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
            />
          </template>
        </FormField>

        <div class="flex justify-end">
          <!--
            The label no longer swaps to "Changing…" mid-press: that is a wider
            string, so the button used to resize under the cursor.
          -->
          <Button type="submit" tone="accent" variant="solid" size="md" :loading="loading">
            {{ t('profile.changePasswordButton') }}
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'

const { t } = useI18n()
const appStore = useAppStore()
const props = withDefaults(defineProps<{
  embedded?: boolean
}>(), {
  embedded: false,
})

const loading = ref(false)
const form = ref({
  old_password: '',
  new_password: '',
  confirm_password: ''
})
const errors = reactive({
  old_password: '',
  new_password: '',
  confirm_password: ''
})

function resetErrors(): void {
  errors.old_password = ''
  errors.new_password = ''
  errors.confirm_password = ''
}

const handleChangePassword = async () => {
  resetErrors()

  if (form.value.new_password !== form.value.confirm_password) {
    errors.confirm_password = t('profile.passwordsNotMatch')
    appStore.showError(errors.confirm_password)
    return
  }

  if (form.value.new_password.length < 8) {
    errors.new_password = t('profile.passwordTooShort')
    appStore.showError(errors.new_password)
    return
  }

  loading.value = true
  try {
    await userAPI.changePassword(form.value.old_password, form.value.new_password)
    form.value = { old_password: '', new_password: '', confirm_password: '' }
    appStore.showSuccess(t('profile.passwordChangeSuccess'))
  } catch (error: unknown) {
    const detail = (error as { response?: { data?: { detail?: string } } }).response?.data?.detail
    appStore.showError(detail || t('profile.passwordChangeFailed'))
  } finally {
    loading.value = false
  }
}
</script>
