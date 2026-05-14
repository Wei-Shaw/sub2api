<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ localText("企业微信登录", "WeCom Login") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{
          localText(
            "用于企业微信自建应用 OAuth 登录配置。",
            "Configure OAuth login for a self-built WeCom application.",
          )
        }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between">
        <div>
          <label class="font-medium text-gray-900 dark:text-white">
            {{ localText("启用企业微信登录", "Enable WeCom login") }}
          </label>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{
              localText(
                "启用后，登录页和注册页会展示企业微信入口。",
                "When enabled, the login and registration pages show the WeCom entry.",
              )
            }}
          </p>
        </div>
        <Toggle
          v-model="enabled"
          data-testid="wecom-oauth-enabled"
        />
      </div>

      <div
        v-if="enabled"
        class="space-y-6 border-t border-gray-100 pt-4 dark:border-dark-700"
      >
        <div
          class="rounded-lg border border-sky-200 bg-sky-50 px-4 py-3 text-sm text-sky-700 dark:border-sky-900/40 dark:bg-sky-900/10 dark:text-sky-300"
        >
          {{
            localText(
              "请在企业微信自建应用后台配置可信域名 / 授权回调域名，Secret 只会保存到后端且不会明文回显。",
              "Configure the trusted / authorization callback domain in the WeCom app console. The secret is stored only on the backend and is never returned in plaintext.",
            )
          }}
        </div>

        <div class="grid grid-cols-1 gap-6 lg:grid-cols-2">
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              CorpID
            </label>
            <input
              v-model="corpId"
              data-testid="wecom-oauth-corp-id"
              type="text"
              class="input font-mono text-sm"
              placeholder="wwxxxxxxxxxxxxxxxx"
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              AgentID
            </label>
            <input
              v-model="agentId"
              data-testid="wecom-oauth-agent-id"
              type="text"
              class="input font-mono text-sm"
              placeholder="1000002"
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              Secret
            </label>
            <input
              v-model="secret"
              data-testid="wecom-oauth-secret"
              type="password"
              class="input font-mono text-sm"
              :placeholder="
                secretConfigured
                  ? localText(
                      '密钥已配置，留空以保留当前值。',
                      'Secret configured. Leave empty to keep the current value.',
                    )
                  : localText('企业微信应用 Secret', 'WeCom app secret')
              "
            />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ localText("授权 Scope", "Authorization scope") }}
            </label>
            <input
              v-model="scope"
              data-testid="wecom-oauth-scope"
              type="text"
              class="input font-mono text-sm"
              placeholder="snsapi_base"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{
                localText(
                  "MVP 推荐使用 snsapi_base，仅识别企业成员身份。",
                  "MVP recommends snsapi_base to identify enterprise members only.",
                )
              }}
            </p>
          </div>
        </div>

        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ localText("后端回调 URL", "Backend callback URL") }}
          </label>
          <input
            v-model="redirectUrl"
            data-testid="wecom-oauth-redirect-url"
            type="url"
            class="input font-mono text-sm"
            placeholder="https://example.com/api/v1/auth/oauth/wecom/callback"
          />
        </div>

        <div>
          <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ localText("前端回跳地址", "Frontend callback URL") }}
          </label>
          <input
            v-model="frontendRedirectUrl"
            data-testid="wecom-oauth-frontend-redirect-url"
            type="text"
            class="input font-mono text-sm"
            placeholder="/auth/wecom/callback"
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import Toggle from "@/components/common/Toggle.vue";

defineProps<{
  secretConfigured: boolean;
}>();

const enabled = defineModel<boolean>("enabled", { required: true });
const corpId = defineModel<string>("corpId", { required: true });
const agentId = defineModel<string>("agentId", { required: true });
const secret = defineModel<string>("secret", { required: true });
const scope = defineModel<string>("scope", { required: true });
const redirectUrl = defineModel<string>("redirectUrl", { required: true });
const frontendRedirectUrl = defineModel<string>("frontendRedirectUrl", { required: true });

const { locale } = useI18n();
const isZhLocale = computed(() => locale.value.startsWith("zh"));

function localText(zh: string, en: string): string {
  return isZhLocale.value ? zh : en;
}
</script>
