import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

describe('OpenAI Fast/Flex policy locale keys', () => {
  it('exposes user scope copy at the runtime zh path', () => {
    expect(zh.admin.settings.openaiFastPolicy).toMatchObject({
      userIds: '指定用户',
      userIdsHint: '输入任意邮箱关键词进行模糊搜索。留空表示对全部 Sub2API 用户生效；选中用户的 API Key 请求优先匹配用户规则。',
      userSearchPlaceholder: '输入用户邮箱搜索',
      userSearchEmpty: '未找到匹配用户',
      userDeleted: '（已删除）',
      userIdFallback: '用户 #{id}',
      removeUser: '移除用户'
    })
  })

  it('exposes user scope copy at the runtime en path', () => {
    expect(en.admin.settings.openaiFastPolicy).toMatchObject({
      userIds: 'Specific users',
      userIdsHint: 'Type any part of a user email to search. Leave empty to apply to all Sub2API users. Selected users match requests from their API keys and take precedence over global rules.',
      userSearchPlaceholder: 'Search by user email',
      userSearchEmpty: 'No matching users found',
      userDeleted: '(deleted)',
      userIdFallback: 'User #{id}',
      removeUser: 'Remove user'
    })
  })
})
