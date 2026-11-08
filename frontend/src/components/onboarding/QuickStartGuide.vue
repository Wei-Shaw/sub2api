<template>
  <!-- 新手引导：第一次登录时自动弹一次，之后从右上角的问号按钮随时打开。
       参考 A6 的做法——把「装什么、填什么」直接摆在最显眼处，
       而不是让用户自己去角落里的「文档」里翻。 -->
  <BaseDialog :show="show" :title="t('quickStart.title')" width="wide" @close="close">
    <p class="mb-4 text-sm leading-relaxed text-gray-500 dark:text-gray-400">
      {{ t('quickStart.subtitle') }}
    </p>

    <ol class="flex flex-col gap-3">
      <li
        v-for="(step, index) in steps"
        :key="step.key"
        class="rounded-xl border border-gray-200 bg-white p-3.5 dark:border-dark-700 dark:bg-dark-800"
      >
        <div class="flex items-start gap-3">
          <span
            class="mt-0.5 grid h-6 w-6 shrink-0 place-items-center rounded-full bg-gradient-to-br from-primary-500 to-cyan-400 text-xs font-semibold text-white"
          >
            {{ index + 1 }}
          </span>
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h4 class="text-sm font-semibold text-gray-900 dark:text-white">{{ step.title }}</h4>
              <a
                v-if="step.href"
                :href="step.href"
                :target="step.internal ? undefined : '_blank'"
                rel="noopener noreferrer"
                class="inline-flex items-center gap-1 text-xs font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
              >
                {{ step.hrefText }}
                <Icon :name="step.internal ? 'chevronRight' : 'externalLink'" size="sm" />
              </a>
            </div>
            <p class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
              {{ step.desc }}
            </p>

            <!-- 可复制的内容块：命令或接口地址 -->
            <div
              v-if="step.copyValue"
              class="mt-2 flex items-center gap-2 rounded-lg bg-gray-50 px-3 py-2 dark:bg-dark-900"
            >
              <code class="min-w-0 flex-1 truncate font-mono text-xs text-gray-700 dark:text-dark-200">
                {{ step.copyValue }}
              </code>
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0"
                @click="copy(step.copyValue)"
              >
                <Icon :name="copiedKey === step.key ? 'check' : 'copy'" size="sm" class="mr-1" />
                {{ copiedKey === step.key ? t('quickStart.copied') : t('quickStart.copy') }}
              </button>
            </div>
          </div>
        </div>
      </li>
    </ol>

    <p class="mt-4 text-xs text-gray-400 dark:text-gray-500">
      {{ t('quickStart.moreHint') }}
      <!-- 2026-11-05：这里必须和首页那个「文档」走同一个去处。
           后台把文档配成了站内自定义页（custom_menu_items → md:support-guide），
           而 doc_url 是飞书外链、未登录会被拦在登录页——两处不一致用户会以为点坏了。 -->
      <a
        v-if="docUrl"
        :href="docUrl"
        target="_blank"
        rel="noopener noreferrer"
        class="font-medium text-primary-600 hover:text-primary-700 dark:text-primary-400"
        data-testid="quickstart-doc-link"
      >
        {{ t('nav.docs') }}
      </a>
    </p>

    <template #footer>
      <!-- 「今日不再提示」只压今天；「关闭」= 看过了，以后不再自动弹（仍可从右上角打开） -->
      <button type="button" class="btn btn-secondary" data-testid="quickstart-snooze" @click="emit('snooze')">
        {{ t('quickStart.later') }}
      </button>
      <button type="button" class="btn btn-primary" data-testid="quickstart-close" @click="emit('close')">
        {{ t('quickStart.gotIt') }}
      </button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

defineProps<{ show: boolean }>()
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'snooze'): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const copiedKey = ref('')

/** 平台接口地址：客户端（codex / CC Switch）里要填的就是它。 */
const apiBaseUrl = computed(() => appStore.apiBaseUrl || appStore.cachedPublicSettings?.api_base_url || '')
/**
 * 文档去处：和顶栏那个「文档」按钮**完全一致**（同 URL、同新标签打开）。
 *
 * 2026-11-05 实测：这个飞书链接在真实浏览器里能直接打开（走飞书访客流程），
 * 只有用无 Cookie 的脚本去抓才会被弹登录页——所以这里不要自作聪明换成站内页，
 * 否则用户从弹窗点进去和从顶栏点进去会是两个地方。
 */
const docUrl = computed(() => sanitizeUrl(appStore.docUrl))

const steps = computed(() => [
  {
    key: 'key',
    title: t('quickStart.step1.title'),
    desc: t('quickStart.step1.desc'),
    href: '/keys',
    hrefText: t('quickStart.step1.link'),
    internal: true,
    copyValue: '',
  },
  {
    key: 'node',
    title: t('quickStart.step2.title'),
    desc: t('quickStart.step2.desc'),
    href: 'https://nodejs.org/zh-cn/download',
    hrefText: t('quickStart.step2.link'),
    internal: false,
    copyValue: 'node -v',
  },
  {
    key: 'codex',
    title: t('quickStart.step3.title'),
    desc: t('quickStart.step3.desc'),
    href: '',
    hrefText: '',
    internal: false,
    copyValue: 'npm install -g @openai/codex',
  },
  {
    key: 'endpoint',
    title: t('quickStart.step4.title'),
    desc: t('quickStart.step4.desc'),
    href: '',
    hrefText: '',
    internal: false,
    copyValue: apiBaseUrl.value,
  },
])

async function copy(value: string): Promise<void> {
  if (!value) return
  const key = steps.value.find((s) => s.copyValue === value)?.key || ''
  try {
    await navigator.clipboard.writeText(value)
    copiedKey.value = key
    // 2 秒后把「已复制」收回去，避免一直挂着像没反应
    setTimeout(() => {
      if (copiedKey.value === key) copiedKey.value = ''
    }, 2000)
  } catch {
    copiedKey.value = ''
  }
}

function close(): void {
  emit('close')
}

// 右上角的 X 与点遮罩等同「关闭」，交给父组件决定后续
void close
</script>
