<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <form v-else @submit.prevent="save" class="space-y-6">
        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h1 class="text-lg font-semibold text-gray-900 dark:text-white">SEO配置</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              这里统一管理公开页面的标题、描述、规范链接、社交分享图、Twitter 卡片和搜索引擎抓取相关配置。
            </p>
          </div>
          <div class="space-y-6 p-6">
            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">默认 SEO 标题</label>
                <input
                  ref="seoDefaultTitleInput"
                  v-model="form.seo_default_title"
                  type="text"
                  class="input"
                  placeholder="示例：Sub2API - AI API 聚合平台"
                />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">首页 SEO 标题</label>
                <input
                  ref="seoHomeTitleInput"
                  v-model="form.seo_home_title"
                  type="text"
                  class="input"
                  placeholder="示例：高可用 AI API 中转服务"
                />
              </div>
            </div>

            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">默认 SEO 描述</label>
                <textarea
                  ref="seoDefaultDescriptionInput"
                  v-model="form.seo_default_description"
                  rows="4"
                  class="input"
                  placeholder="示例：统一接入 Claude、GPT、Gemini 等模型，支持按量计费、模型切换和稳定调度。"
                ></textarea>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">首页 SEO 描述</label>
                <textarea
                  ref="seoHomeDescriptionInput"
                  v-model="form.seo_home_description"
                  rows="4"
                  class="input"
                  placeholder="示例：一个密钥，接入多种 AI 模型，适合个人与团队统一使用。"
                ></textarea>
              </div>
            </div>

            <div class="grid grid-cols-1 gap-6 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">公开站点域名</label>
                <input
                  ref="frontendUrlInput"
                  v-model="form.frontend_url"
                  type="url"
                  class="input"
                  placeholder="示例：https://api.example.com"
                />
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  用于生成规范链接、og:url、robots.txt、sitemap.xml 和 pricing 接口中的 site_domain。
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">默认搜索引擎抓取规则</label>
                <select ref="seoDefaultRobotsSelect" v-model="form.seo_default_robots" class="input">
                  <option value="index, follow">允许收录并跟踪链接（推荐公开页面）</option>
                  <option value="noindex, nofollow">禁止收录且不跟踪链接（推荐隐藏页面）</option>
                  <option value="index, nofollow">允许收录但不跟踪链接</option>
                  <option value="noindex, follow">禁止收录但允许跟踪链接</option>
                </select>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  公开页面一般推荐选择“允许收录并跟踪链接”；测试页、临时页或不希望被搜索到的页面推荐选择“禁止收录且不跟踪链接”。
                </p>
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">首页搜索引擎抓取规则</label>
                <select ref="seoHomeRobotsSelect" v-model="form.seo_home_robots" class="input">
                  <option value="">继承默认配置</option>
                  <option value="index, follow">允许收录并跟踪链接</option>
                  <option value="noindex, nofollow">禁止收录且不跟踪链接</option>
                  <option value="index, nofollow">允许收录但不跟踪链接</option>
                  <option value="noindex, follow">禁止收录但允许跟踪链接</option>
                </select>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
                  首页如果需要与其他公开页不同的抓取策略，请在这里单独配置；留空时继承默认搜索引擎抓取规则。
                </p>
              </div>
            </div>

            <div>
              <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">默认社交分享图</label>
              <ImageUpload
                v-model="seoDefaultOgImage"
                mode="image"
                :upload-label="'上传图片'"
                :remove-label="'移除图片'"
                hint="示例：上传 1200×630 的分享图。留空时自动回退到动态 OG 图或站点 Logo。"
                :max-size="500 * 1024"
              />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">法律页面 SEO</h2>
          </div>
          <div class="space-y-4 p-6">
            <div
              v-for="(doc, index) in form.login_agreement_documents"
              :key="doc.id || index"
              class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
            >
              <div class="mb-3">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ doc.title || '未命名文档' }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">路径：/legal/{{ doc.id || index }}</p>
              </div>
              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">SEO 标题</label>
                  <input
                    v-model="doc.seo_title"
                    type="text"
                    class="input text-sm"
                    placeholder="示例：服务条款 - 示例站点"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">搜索引擎抓取规则</label>
                  <select v-model="doc.seo_robots" class="input text-sm">
                    <option value="">继承默认配置</option>
                    <option value="index, follow">允许收录并跟踪链接（推荐公开页面）</option>
                    <option value="noindex, nofollow">禁止收录且不跟踪链接（推荐隐藏页面）</option>
                    <option value="index, nofollow">允许收录但不跟踪链接</option>
                    <option value="noindex, follow">禁止收录但允许跟踪链接</option>
                  </select>
                  <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
                    如果这类法律页希望被搜索引擎收录，推荐选“继承默认配置”或“允许收录并跟踪链接”。
                  </p>
                </div>
                <div class="md:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">SEO 描述</label>
                  <textarea
                    v-model="doc.seo_description"
                    rows="3"
                    class="input text-sm"
                    placeholder="示例：查看本站服务条款、使用规则和相关说明。"
                  ></textarea>
                </div>
                <div class="md:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">社交分享图</label>
                  <input
                    v-model="doc.seo_og_image"
                    type="url"
                    class="input text-sm"
                    placeholder="示例：https://api.example.com/static/legal-terms-og.png"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">公开自定义页 SEO</h2>
          </div>
          <div class="space-y-4 p-6">
            <div
              v-if="form.custom_menu_items.length === 0"
              class="rounded-lg border border-dashed border-gray-300 bg-gray-50 px-4 py-4 text-sm text-gray-600 dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300"
            >
              <p class="font-medium text-gray-800 dark:text-white">当前还没有公开自定义页</p>
              <p class="mt-2">
                请先到“系统设置”里的自定义菜单项中新增一条普通用户可见的页面，并填写
                <span class="font-mono">page_slug</span>。
              </p>
              <p class="mt-2">
                <span class="font-medium">page_slug 是什么：</span>
                它对应服务器上的 markdown 文件名。比如
                <span class="font-mono">page_slug = guide</span>
                ，系统就会读取
                <span class="font-mono">guide.md</span>
                作为这条公开自定义页的正文内容。
              </p>
              <p class="mt-2">
                <span class="font-medium">配置示例：</span>
                菜单名称填“帮助指南”，页面路径
                <span class="font-mono">id = guide</span>
                ，正文文件
                <span class="font-mono">page_slug = guide</span>
                。
              </p>
            </div>

            <div
              v-for="(item, index) in form.custom_menu_items"
              :key="item.id || index"
              class="rounded-lg border border-gray-200 p-4 dark:border-dark-600"
            >
              <div class="mb-3">
                <p class="text-sm font-semibold text-gray-900 dark:text-white">{{ item.label || '未命名菜单' }}</p>
                <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">路径：/custom/{{ item.id || index }}</p>
              </div>
              <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">SEO 标题</label>
                  <input
                    v-model="item.seo_title"
                    type="text"
                    class="input text-sm"
                    placeholder="示例：帮助指南 - 示例站点"
                  />
                </div>
                <div>
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">搜索引擎抓取规则</label>
                  <select v-model="item.seo_robots" class="input text-sm">
                    <option value="">继承默认配置</option>
                    <option value="index, follow">允许收录并跟踪链接（推荐公开页面）</option>
                    <option value="noindex, nofollow">禁止收录且不跟踪链接（推荐隐藏页面）</option>
                    <option value="index, nofollow">允许收录但不跟踪链接</option>
                    <option value="noindex, follow">禁止收录但允许跟踪链接</option>
                  </select>
                  <p class="mt-1 text-[11px] text-gray-500 dark:text-gray-400">
                    公开帮助页、指南页一般推荐选“继承默认配置”或“允许收录并跟踪链接”。
                  </p>
                </div>
                <div class="md:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">SEO 描述</label>
                  <textarea
                    v-model="item.seo_description"
                    rows="3"
                    class="input text-sm"
                    placeholder="示例：这里是公开帮助页面，介绍使用方法、常见问题和接入说明。"
                  ></textarea>
                </div>
                <div class="md:col-span-2">
                  <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">社交分享图</label>
                  <input
                    v-model="item.seo_og_image"
                    type="url"
                    class="input text-sm"
                    placeholder="示例：https://api.example.com/static/custom-guide-og.png"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="flex justify-end">
          <button type="submit" :disabled="saving" class="btn btn-primary">
            {{ saving ? '保存中...' : '保存 SEO配置' }}
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, reactive, ref, type Ref } from 'vue'
import { adminAPI } from '@/api'
import type { SystemSettings, UpdateSettingsRequest } from '@/api/admin/settings'
import AppLayout from '@/components/layout/AppLayout.vue'
import ImageUpload from '@/components/common/ImageUpload.vue'
import { useAppStore } from '@/stores'

const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const seoDefaultTitleInput = ref<HTMLInputElement | null>(null)
const seoHomeTitleInput = ref<HTMLInputElement | null>(null)
const seoDefaultDescriptionInput = ref<HTMLTextAreaElement | null>(null)
const seoHomeDescriptionInput = ref<HTMLTextAreaElement | null>(null)
const frontendUrlInput = ref<HTMLInputElement | null>(null)
const seoDefaultRobotsSelect = ref<HTMLSelectElement | null>(null)
const seoHomeRobotsSelect = ref<HTMLSelectElement | null>(null)

const form = reactive<SystemSettings>({
  registration_enabled: true,
  email_verify_enabled: false,
  registration_email_suffix_whitelist: [],
  promo_code_enabled: true,
  password_reset_enabled: false,
  frontend_url: '',
  invitation_code_enabled: false,
  totp_enabled: false,
  totp_encryption_key_configured: false,
  login_agreement_enabled: false,
  login_agreement_mode: 'modal',
  login_agreement_updated_at: '',
  login_agreement_documents: [],
  default_balance: 0,
  default_concurrency: 0,
  default_user_rpm_limit: 0,
  default_subscriptions: [],
  site_name: 'Sub2API',
  site_logo: '',
  site_subtitle: '',
  api_base_url: '',
  contact_info: '',
  doc_url: '',
  home_content: '',
  seo_default_title: '',
  seo_home_title: '',
  seo_default_description: '',
  seo_home_description: '',
  seo_default_og_image: '',
  seo_default_robots: 'index, follow',
  seo_home_robots: '',
  hide_ccs_import_button: false,
  table_default_page_size: 20,
  table_page_size_options: [10, 20, 50, 100],
  backend_mode_enabled: false,
  custom_menu_items: [],
  custom_endpoints: [],
  smtp_host: '',
  smtp_port: 587,
  smtp_username: '',
  smtp_password_configured: false,
  smtp_from_email: '',
  smtp_from_name: '',
  smtp_use_tls: true,
  turnstile_enabled: false,
  turnstile_site_key: '',
  turnstile_secret_key_configured: false,
  linuxdo_connect_enabled: false,
  linuxdo_connect_client_id: '',
  linuxdo_connect_client_secret_configured: false,
  linuxdo_connect_redirect_url: '',
  wechat_connect_enabled: false,
  wechat_connect_app_id: '',
  wechat_connect_app_secret_configured: false,
  wechat_connect_mode: 'open',
  wechat_connect_scopes: '',
  wechat_connect_redirect_url: '',
  wechat_connect_frontend_redirect_url: '',
  oidc_connect_enabled: false,
  oidc_connect_provider_name: 'OIDC',
  oidc_connect_client_id: '',
  oidc_connect_client_secret_configured: false,
  oidc_connect_issuer_url: '',
  oidc_connect_discovery_url: '',
  oidc_connect_authorize_url: '',
  oidc_connect_token_url: '',
  oidc_connect_userinfo_url: '',
  oidc_connect_jwks_url: '',
  oidc_connect_scopes: '',
  oidc_connect_redirect_url: '',
  oidc_connect_frontend_redirect_url: '',
  oidc_connect_token_auth_method: '',
  oidc_connect_use_pkce: false,
  oidc_connect_validate_id_token: false,
  oidc_connect_allowed_signing_algs: '',
  oidc_connect_clock_skew_seconds: 0,
  oidc_connect_require_email_verified: false,
  oidc_connect_userinfo_email_path: '',
  oidc_connect_userinfo_id_path: '',
  oidc_connect_userinfo_username_path: '',
  github_oauth_enabled: false,
  github_oauth_client_id: '',
  github_oauth_client_secret_configured: false,
  github_oauth_redirect_url: '',
  github_oauth_frontend_redirect_url: '',
  google_oauth_enabled: false,
  google_oauth_client_id: '',
  google_oauth_client_secret_configured: false,
  google_oauth_redirect_url: '',
  google_oauth_frontend_redirect_url: '',
  enable_model_fallback: false,
  fallback_model_anthropic: '',
  fallback_model_openai: '',
  fallback_model_gemini: '',
  fallback_model_antigravity: '',
  enable_identity_patch: false,
  identity_patch_prompt: '',
  ops_monitoring_enabled: false,
  ops_realtime_monitoring_enabled: false,
  ops_query_mode_default: 'auto',
  ops_metrics_interval_seconds: 60,
  min_claude_code_version: '',
  max_claude_code_version: '',
  allow_ungrouped_key_scheduling: false,
  enable_fingerprint_unification: false,
  enable_metadata_passthrough: false,
  enable_cch_signing: false,
  enable_anthropic_cache_ttl_1h_injection: false,
  payment_enabled: false,
  payment_min_amount: 0,
  payment_max_amount: 0,
  payment_daily_limit: 0,
  payment_order_timeout_minutes: 0,
  payment_max_pending_orders: 0,
  payment_enabled_types: [],
  payment_balance_disabled: false,
  payment_balance_recharge_multiplier: 1,
  payment_recharge_fee_rate: 0,
  payment_load_balance_strategy: 'round-robin',
  payment_product_name_prefix: '',
  payment_product_name_suffix: '',
  payment_help_image_url: '',
  payment_help_text: '',
  payment_cancel_rate_limit_enabled: false,
  payment_cancel_rate_limit_max: 0,
  payment_cancel_rate_limit_window: 0,
  payment_cancel_rate_limit_unit: 'day',
  payment_cancel_rate_limit_window_mode: 'rolling',
  rewrite_message_cache_control: false,
  antigravity_user_agent_version: '',
  balance_low_notify_enabled: false,
  balance_low_notify_threshold: 0,
  balance_low_notify_recharge_url: '',
  account_quota_notify_enabled: false,
  account_quota_notify_emails: [],
  risk_control_enabled: false,
  channel_monitor_enabled: false,
  channel_monitor_default_interval_seconds: 60,
  available_channels_enabled: false,
})

const seoDefaultOgImage = computed({
  get: () => form.seo_default_og_image ?? '',
  set: (value: string) => {
    form.seo_default_og_image = value
  },
})

async function load() {
  loading.value = true
  try {
    Object.assign(form, await adminAPI.settings.getSettings())
  } finally {
    loading.value = false
  }
}

function syncFormFieldFromElement(target: Ref<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null>, setter: (value: string) => void) {
  const element = target.value
  if (!element) {
    return
  }
  setter(element.value)
}

function syncFormFromInputs() {
  syncFormFieldFromElement(seoDefaultTitleInput, (value) => {
    form.seo_default_title = value
  })
  syncFormFieldFromElement(seoHomeTitleInput, (value) => {
    form.seo_home_title = value
  })
  syncFormFieldFromElement(seoDefaultDescriptionInput, (value) => {
    form.seo_default_description = value
  })
  syncFormFieldFromElement(seoHomeDescriptionInput, (value) => {
    form.seo_home_description = value
  })
  syncFormFieldFromElement(frontendUrlInput, (value) => {
    form.frontend_url = value
  })
  syncFormFieldFromElement(seoDefaultRobotsSelect, (value) => {
    form.seo_default_robots = value
  })
  syncFormFieldFromElement(seoHomeRobotsSelect, (value) => {
    form.seo_home_robots = value
  })
}

async function save() {
  saving.value = true
  try {
    syncFormFromInputs()
    const payload: UpdateSettingsRequest = {
      ...form,
      frontend_url: form.frontend_url,
      seo_default_title: form.seo_default_title,
      seo_home_title: form.seo_home_title,
      seo_default_description: form.seo_default_description,
      seo_home_description: form.seo_home_description,
      seo_default_og_image: form.seo_default_og_image,
      seo_default_robots: form.seo_default_robots,
      seo_home_robots: form.seo_home_robots,
      login_agreement_documents: form.login_agreement_documents,
      custom_menu_items: form.custom_menu_items,
    }
    Object.assign(form, await adminAPI.settings.updateSettings(payload))
    await appStore.fetchPublicSettings(true)
    appStore.showSuccess('SEO配置保存成功')
  } finally {
    saving.value = false
  }
}

load()
</script>
