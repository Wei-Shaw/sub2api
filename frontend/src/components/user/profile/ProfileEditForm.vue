<template>
  <div :class="props.embedded ? 'space-y-4' : 'rounded border border-line bg-surface'">
    <div v-if="!props.embedded" class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.editProfile') }}
      </h2>
    </div>
    <div :class="props.embedded ? '' : 'px-4 py-4'">
      <form class="space-y-3" @submit.prevent="handleUpdateProfile">
        <p v-if="props.embedded" class="text-sm font-semibold text-ink">
          {{ t('profile.editProfile') }}
        </p>

        <!--
          `usernameRequired` used to exist only as a toast: the field that was
          wrong got no marking at all, and the message vanished on a timer. It
          is an inline error now, and the toast stays for the case where the
          form is scrolled out of view.
        -->
        <FormField id="username" :label="t('profile.username')" :error="usernameError">
          <template #default="{ describedBy, invalid }">
            <input
              id="username"
              v-model="username"
              type="text"
              class="input"
              :class="{ 'input-error': usernameError }"
              :aria-describedby="describedBy"
              :aria-invalid="invalid || undefined"
              :placeholder="t('profile.enterUsername')"
            />
          </template>
        </FormField>

        <div class="flex justify-end">
          <Button type="submit" tone="accent" variant="solid" size="md" :loading="loading">
            {{ t('profile.updateProfile') }}
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'

const props = withDefaults(defineProps<{
  initialUsername: string
  embedded?: boolean
}>(), {
  embedded: false,
})

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const username = ref(props.initialUsername)
const usernameError = ref('')
const loading = ref(false)

watch(() => props.initialUsername, (val) => {
  username.value = val
})

watch(username, () => {
  usernameError.value = ''
})

const handleUpdateProfile = async () => {
  if (!username.value.trim()) {
    usernameError.value = t('profile.usernameRequired')
    appStore.showError(usernameError.value)
    return
  }

  usernameError.value = ''
  loading.value = true
  try {
    const updatedUser = await userAPI.updateProfile({
      username: username.value
    })
    authStore.user = updatedUser
    appStore.showSuccess(t('profile.updateSuccess'))
  } catch (error: unknown) {
    const detail = (error as { response?: { data?: { detail?: string } } }).response?.data?.detail
    appStore.showError(detail || t('profile.updateFailed'))
  } finally {
    loading.value = false
  }
}
</script>
