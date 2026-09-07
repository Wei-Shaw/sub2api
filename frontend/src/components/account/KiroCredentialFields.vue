<template>
  <div class="space-y-4">
    <!-- social: 只需要粘贴 refreshToken + region -->
    <template v-if="modelValue.authMethod === 'social'">
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.refreshTokenLabel') }}</label>
        <textarea
          data-test="kiro-refresh-token"
          :value="modelValue.refreshToken"
          rows="4"
          class="input font-mono text-xs"
          :placeholder="t('admin.accounts.oauth.kiro.refreshTokenPlaceholderSocial')"
          @input="update('refreshToken', ($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p v-if="hasExistingSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.regionLabel') }}</label>
        <input
          data-test="kiro-region"
          type="text"
          :value="modelValue.region"
          class="input"
          placeholder="us-east-1"
          @input="update('region', ($event.target as HTMLInputElement).value)"
        />
      </div>
    </template>

    <!-- builder_id: Refresh Token / Client ID / Client Secret / Region -->
    <template v-else-if="modelValue.authMethod === 'builder_id'">
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.refreshTokenLabel') }}</label>
        <textarea
          data-test="kiro-refresh-token"
          :value="modelValue.refreshToken"
          rows="4"
          class="input font-mono text-xs"
          :placeholder="t('admin.accounts.oauth.kiro.refreshTokenPlaceholderBuilderId')"
          @input="update('refreshToken', ($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p class="input-hint">{{ t('admin.accounts.oauth.kiro.refreshTokenAutoHint') }}</p>
        <p v-if="hasExistingSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.clientIdLabel') }}</label>
        <input
          data-test="kiro-client-id"
          type="text"
          :value="modelValue.clientId"
          class="input"
          @input="update('clientId', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.clientSecretLabel') }}</label>
        <input
          data-test="kiro-client-secret"
          type="password"
          :value="modelValue.clientSecret"
          class="input"
          autocomplete="off"
          @input="update('clientSecret', ($event.target as HTMLInputElement).value)"
        />
        <p v-if="hasExistingClientSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.regionLabel') }}</label>
        <input
          data-test="kiro-region"
          type="text"
          :value="modelValue.region"
          class="input"
          placeholder="us-east-1"
          @input="update('region', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.profileArnLabel') }}</label>
        <input
          data-test="kiro-profile-arn"
          type="text"
          :value="modelValue.profileArn"
          class="input font-mono text-xs"
          placeholder="arn:aws:codewhisperer:<region>:<account-id>:profile/xxxxxxxxxxxx"
          @input="update('profileArn', ($event.target as HTMLInputElement).value)"
        />
        <p class="input-hint">
          {{ t('admin.accounts.oauth.kiro.profileArnHintBuilderId') }}
        </p>
      </div>
    </template>

    <!-- idc: 在 builder_id 基础上加 SSO 门户地址 -->
    <template v-else-if="modelValue.authMethod === 'idc'">
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.issuerUrlLabel') }}</label>
        <input
          data-test="kiro-issuer-url"
          type="text"
          :value="modelValue.issuerUrl"
          class="input"
          placeholder="https://d-xxxxxxxxx.awsapps.com/start"
          @input="update('issuerUrl', ($event.target as HTMLInputElement).value)"
        />
        <p class="input-hint">{{ t('admin.accounts.oauth.kiro.issuerUrlHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.refreshTokenLabel') }}</label>
        <textarea
          data-test="kiro-refresh-token"
          :value="modelValue.refreshToken"
          rows="4"
          class="input font-mono text-xs"
          :placeholder="t('admin.accounts.oauth.kiro.refreshTokenPlaceholderIdc')"
          @input="update('refreshToken', ($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p class="input-hint">{{ t('admin.accounts.oauth.kiro.refreshTokenAutoHint') }}</p>
        <p v-if="hasExistingSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.clientIdLabel') }}</label>
        <input
          data-test="kiro-client-id"
          type="text"
          :value="modelValue.clientId"
          class="input"
          @input="update('clientId', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.clientSecretLabel') }}</label>
        <input
          data-test="kiro-client-secret"
          type="password"
          :value="modelValue.clientSecret"
          class="input"
          autocomplete="off"
          @input="update('clientSecret', ($event.target as HTMLInputElement).value)"
        />
        <p v-if="hasExistingClientSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.regionLabel') }}</label>
        <input
          data-test="kiro-region"
          type="text"
          :value="modelValue.region"
          class="input"
          placeholder="us-east-1"
          @input="update('region', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.profileArnLabel') }}</label>
        <input
          data-test="kiro-profile-arn"
          type="text"
          :value="modelValue.profileArn"
          class="input font-mono text-xs"
          placeholder="arn:aws:codewhisperer:<region>:<account-id>:profile/xxxxxxxxxxxx"
          @input="update('profileArn', ($event.target as HTMLInputElement).value)"
        />
        <p class="input-hint">
          {{ t('admin.accounts.oauth.kiro.profileArnHintIdc') }}
        </p>
      </div>
    </template>

    <!-- api_key: 只需要密钥 + region -->
    <template v-else-if="modelValue.authMethod === 'api_key'">
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.apiKeyLabel') }}</label>
        <input
          data-test="kiro-api-key"
          type="password"
          :value="modelValue.apiKey"
          class="input"
          autocomplete="off"
          @input="update('apiKey', ($event.target as HTMLInputElement).value)"
        />
        <p v-if="hasExistingSecret" class="input-hint">{{ t('admin.accounts.oauth.kiro.leaveEmptyHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.oauth.kiro.regionLabel') }}</label>
        <input
          data-test="kiro-region"
          type="text"
          :value="modelValue.region"
          class="input"
          placeholder="us-east-1"
          @input="update('region', ($event.target as HTMLInputElement).value)"
        />
      </div>
    </template>

    <!-- 假思考开关：底部统一放置，覆盖全部接入方式 -->
    <div class="flex items-start justify-between gap-4 rounded-lg border border-gray-200 p-3 dark:border-dark-600">
      <div>
        <p class="text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.accounts.oauth.kiro.fakeThinkingTitle') }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.oauth.kiro.fakeThinkingDesc') }}
        </p>
      </div>
      <button
        type="button"
        role="switch"
        data-test="kiro-fake-thinking"
        :aria-checked="fakeThinking"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          fakeThinking ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
        @click="update('fakeThinking', !fakeThinking)"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            fakeThinking ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { KiroCredentialForm } from './kiroCredentials'

interface Props {
  modelValue: Partial<KiroCredentialForm>
  /** 编辑态：该账号已配置 refreshToken/apiKey，留空提交代表不修改而非清空。 */
  hasExistingSecret?: boolean
  /** 编辑态：该账号已配置 clientSecret（idc/builder_id），留空提交代表不修改而非清空。 */
  hasExistingClientSecret?: boolean
}

const { t } = useI18n()
const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: Partial<KiroCredentialForm>]
}>()

const fakeThinking = computed(() => props.modelValue.fakeThinking === true)

function update<K extends keyof KiroCredentialForm>(key: K, value: KiroCredentialForm[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}
</script>
