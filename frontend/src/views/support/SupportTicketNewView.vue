<template>
  <AppLayout>
    <div class="mx-auto max-w-3xl space-y-6">
      <!-- 顶部返回 + 标题 -->
      <div>
        <button
          type="button"
          class="inline-flex items-center text-sm font-medium text-gray-500 hover:text-primary-600 dark:text-dark-400 dark:hover:text-primary-400"
          @click="goBack"
        >
          <Icon name="arrowLeft" size="sm" class="mr-1" />
          {{ t('support.common.backToList') }}
        </button>
        <h1 class="mt-2 text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('support.new.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ t('support.new.description') }}
        </p>
      </div>

      <!-- 提交表单 -->
      <form class="card space-y-5 p-6" @submit.prevent="handleSubmit">
        <!-- 标题 -->
        <div>
          <label for="ticket-title" class="input-label">
            {{ t('support.new.titleLabel') }}
            <span class="text-red-500">*</span>
          </label>
          <input
            id="ticket-title"
            v-model="form.title"
            type="text"
            maxlength="255"
            :placeholder="t('support.new.titlePlaceholder')"
            :disabled="submitting"
            class="input mt-1"
            required
          />
        </div>

        <!-- 分类 -->
        <div>
          <label for="ticket-category" class="input-label">
            {{ t('support.new.categoryLabel') }}
            <span class="text-red-500">*</span>
          </label>
          <select
            id="ticket-category"
            v-model="form.category"
            :disabled="submitting || categoriesLoading"
            class="input mt-1"
            required
          >
            <option value="" disabled>{{ t('support.new.categoryPlaceholder') }}</option>
            <option v-for="cat in categories" :key="cat" :value="cat">{{ cat }}</option>
          </select>
        </div>

        <!-- 描述 -->
        <div>
          <label for="ticket-content" class="input-label">
            {{ t('support.new.contentLabel') }}
            <span class="text-red-500">*</span>
          </label>
          <textarea
            id="ticket-content"
            v-model="form.content"
            rows="10"
            maxlength="16384"
            :placeholder="t('support.new.contentPlaceholder')"
            :disabled="submitting"
            class="input mt-1 font-mono text-sm"
            required
          />
          <p class="input-hint">{{ form.content.length }} / 16384</p>
        </div>

        <!-- chat_context 命中提示（不展示原文，避免占用屏幕，只标记"已附带"） -->
        <div
          v-if="form.chat_context"
          class="flex items-start gap-3 rounded-xl bg-primary-50 p-4 dark:bg-primary-900/20"
        >
          <Icon name="infoCircle" size="md" class="mt-0.5 text-primary-600 dark:text-primary-400" />
          <div class="flex-1 text-sm text-primary-800 dark:text-primary-200">
            {{ t('support.common.contextHint') }}
            <button
              type="button"
              class="ml-2 underline hover:no-underline"
              @click="form.chat_context = ''"
            >
              {{ t('common.cancel') }}
            </button>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="flex justify-end gap-3 border-t border-gray-100 pt-4 dark:border-dark-700">
          <button type="button" class="btn btn-secondary" :disabled="submitting" @click="goBack">
            {{ t('common.cancel') }}
          </button>
          <button type="submit" class="btn btn-primary" :disabled="submitting || !canSubmit">
            <svg
              v-if="submitting"
              class="-ml-1 mr-2 h-4 w-4 animate-spin"
              fill="none"
              viewBox="0 0 24 24"
            >
              <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
              <path
                class="opacity-75"
                fill="currentColor"
                d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
              />
            </svg>
            {{ submitting ? t('support.common.submitting') : t('support.new.submit') }}
          </button>
        </div>
      </form>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
/**
 * SupportTicketNewView —— 用户侧"新建工单"页。
 *
 * 设计要点：
 *
 * 1) 分类下拉数据来源是 GET `/support/categories`，由 `listCategories()` 拉取；
 *    feature_disabled 时该接口返回 404，本页通过 toast 提示并禁用提交按钮。
 *    `default_priority` 由后端在创建时自动应用，前端不允许用户手动选择优先级。
 *
 * 2) URL query 协议（与浮窗对接）：
 *      `/support/tickets/new?from=chat&session=<key>`
 *    当 `from=chat` 且 `session` 存在时，从 `localStorage[<key>]` 读取浮窗
 *    序列化好的对话历史，拼成 Markdown 草稿填入 `content`，并把原文塞进
 *    `form.chat_context`（hidden 字段）。读不到时 silent skip，仅 console.warn，
 *    避免阻塞用户手动新建工单。
 *
 * 3) 长度上限与后端对齐：
 *      title    255      （ent schema unique-ish 单列上限）
 *      content  16 384   （SupportTicketContentMaxLen）
 *      chat_context 50 000（SupportTicketChatContextMaxLen, 服务端校验为准）
 *    前端只对 title/content 加 maxlength，chat_context 不在 UI 暴露上限即可。
 */
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { createTicket, listCategories, type CreateTicketRequest } from '@/api/support'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const submitting = ref(false)
const categoriesLoading = ref(false)
const categories = ref<string[]>([])

const form = reactive<CreateTicketRequest>({
  title: '',
  content: '',
  category: '',
  // chat_context 仅在浮窗 query 命中时填充，永远不在 UI 上让用户自行输入
  chat_context: '',
})

const canSubmit = computed(() => {
  return (
    form.title.trim() !== '' &&
    form.content.trim() !== '' &&
    form.category !== '' &&
    !categoriesLoading.value
  )
})

async function loadCategories() {
  categoriesLoading.value = true
  try {
    const res = await listCategories()
    categories.value = res.categories || []
    // 不预选分类——空字符串配合 <option disabled> 可作为 placeholder。
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
    categories.value = []
  } finally {
    categoriesLoading.value = false
  }
}

/**
 * 解析 URL query：当 `from=chat&session=<key>` 命中时，从 localStorage 拉浮窗
 * 序列化好的对话快照，并把它拼成 Markdown 草稿同时塞到 hidden chat_context。
 * 任何失败（key 不存在 / JSON.parse 失败 / 内容空）都 silent skip，console.warn 提示。
 */
function tryHydrateFromChatContext() {
  const from = (route.query.from as string | undefined) ?? ''
  const sessionKey = (route.query.session as string | undefined) ?? ''
  if (from !== 'chat' || sessionKey === '') return

  let raw: string | null = null
  try {
    raw = window.localStorage.getItem(sessionKey)
  } catch (err) {
    console.warn('[support] localStorage 读取失败:', err)
    return
  }
  if (!raw) {
    appStore.showWarning(t('support.new.contextNotFound'))
    return
  }

  // 浮窗约定写入的是 JSON 数组，每项 `{role: 'user'|'assistant', content: string}`；
  // 但即使是裸字符串也优雅降级。
  let snapshot: string = raw
  try {
    const parsed = JSON.parse(raw)
    if (Array.isArray(parsed)) {
      snapshot = parsed
        .map((m: unknown) => {
          if (!m || typeof m !== 'object') return ''
          const obj = m as { role?: string; content?: string }
          const role = obj.role === 'assistant' ? 'AI' : obj.role === 'user' ? 'User' : 'Note'
          return `**${role}:**\n\n${obj.content ?? ''}`
        })
        .filter(Boolean)
        .join('\n\n---\n\n')
    } else if (typeof parsed === 'string') {
      snapshot = parsed
    }
  } catch {
    // 非 JSON 直接当 markdown 文本用
  }

  if (snapshot.trim() === '') {
    return
  }

  // 内容草稿默认引用对话快照；hidden chat_context 保存原文（含 metadata）
  form.content = `## 对话上下文\n\n${snapshot}\n\n## 我的问题\n\n`
  form.chat_context = snapshot
  appStore.showInfo?.(t('support.new.contextLoaded'))
}

async function handleSubmit() {
  if (!canSubmit.value || submitting.value) return
  submitting.value = true
  try {
    const payload: CreateTicketRequest = {
      title: form.title.trim(),
      content: form.content,
      category: form.category,
    }
    // 仅当浮窗带过来的 chat_context 非空时才提交（不在 UI 暴露的 hidden 字段）
    if (form.chat_context && form.chat_context.trim() !== '') {
      payload.chat_context = form.chat_context
    }
    const created = await createTicket(payload)
    appStore.showSuccess(t('support.new.submitSuccess'))
    router.replace(`/support/tickets/${created.id}`)
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'support.errors', t('common.error')))
  } finally {
    submitting.value = false
  }
}

function goBack() {
  router.push('/support/tickets')
}

onMounted(() => {
  loadCategories()
  tryHydrateFromChatContext()
})
</script>
