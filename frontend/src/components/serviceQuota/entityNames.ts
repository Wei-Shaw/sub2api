/**
 * entityNames.ts — 渠道 / 分组 / 账号 ID → 名称 的本地解析缓存。
 *
 * 用于 PathChevron 把 channel_id / group_id / account_id 渲染成可读名称。
 * 进程内 Map 缓存：同一 id 只发一次请求；并发请求自动去重；
 * 失败保留占位（ch#id / g#id / acc#id），不抛错让 UI 卡死。
 *
 * 不上 Pinia：缓存无需跨页面共享业务态，简单 Map + Ref 已够；
 * Vue 自动追踪 ref，名称解析回填后 chevron 自然刷新。
 */

import { ref, type Ref } from 'vue'
import adminAPI from '@/api/admin'

export type EntityKind = 'channel' | 'group' | 'account' | 'user'

const cache = new Map<string, Ref<string>>()
const inFlight = new Set<string>()

function fallback(kind: EntityKind, id: number): string {
  if (kind === 'channel') return `ch#${id}`
  if (kind === 'group') return `g#${id}`
  if (kind === 'user') return `#${id}`
  return `acc#${id}`
}

async function fetchName(kind: EntityKind, id: number): Promise<string> {
  if (kind === 'channel') {
    const ch = await adminAPI.channels.getById(id)
    return ch.name || fallback(kind, id)
  }
  if (kind === 'group') {
    const g = await adminAPI.groups.getById(id)
    return g.name || fallback(kind, id)
  }
  if (kind === 'user') {
    const u = await adminAPI.users.getById(id)
    // 监控/配置页绑定用户列以邮箱为主标识：管理员习惯用邮箱区分账号，
    // username 在多账号体系中可能重复或缺失。email 缺失才退化到 username。
    return u.email || u.username || fallback(kind, id)
  }
  const a = await adminAPI.accounts.getById(id)
  return a.name || fallback(kind, id)
}

/**
 * 返回一个 Ref<string>，初始值为占位（ch#42 / g#3 / acc#7），
 * 后台异步加载完成后更新为真实名称。
 *
 * id <= 0 / null / undefined 返回空字符串 ref（调用方判空）。
 * 同一 (kind, id) 共用同一 Ref；并发调用自动去重。
 */
export function useEntityName(kind: EntityKind, id: number | null | undefined): Ref<string> {
  if (id == null || id <= 0) {
    return ref('')
  }
  const key = `${kind}:${id}`
  const existing = cache.get(key)
  if (existing) return existing
  const r = ref(fallback(kind, id))
  cache.set(key, r)
  if (!inFlight.has(key)) {
    inFlight.add(key)
    fetchName(kind, id)
      .then((name) => {
        r.value = name
      })
      .catch(() => {
        // 保留占位
      })
      .finally(() => {
        inFlight.delete(key)
      })
  }
  return r
}
