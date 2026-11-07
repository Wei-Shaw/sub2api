import type { DriveStep } from 'driver.js'

type Translate = (key: string) => string

/**
 * 新手引导步骤（概览式）
 *
 * 只高亮入口、说明用途，不打开弹窗、不提交表单、不创建任何数据；
 * 每一步都可以前进、后退或跳过。目标元素不存在（功能开关关闭）时由调用方过滤。
 */

const escapeHtml = (text: string): string =>
  text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')

const paragraph = (text: string) => `<p>${escapeHtml(text)}</p>`
const orderedList = (items: string[]) =>
  `<ol>${items.map((item) => `<li>${escapeHtml(item)}</li>`).join('')}</ol>`
const infoBox = (text: string) => `<p class="tour-info-box">${escapeHtml(text)}</p>`

const description = (...parts: string[]) =>
  `<div class="tour-step-description">${parts.join('')}</div>`

const sidebarLink = (path: string) => `aside.sidebar a[href="${path}"]`

/** 头部用户菜单按钮，通过其 aria-label 定位 */
const userMenuSelector = (t: Translate) =>
  `header button[aria-label="${t('common.userMenu').replace(/"/g, '\\"')}"]`

export const getAdminSteps = (t: Translate): DriveStep[] => [
  {
    popover: {
      title: t('onboarding.admin.welcome.title'),
      description: description(
        paragraph(t('onboarding.admin.welcome.intro')),
        orderedList([
          t('onboarding.admin.welcome.stepGroups'),
          t('onboarding.admin.welcome.stepAccounts'),
          t('onboarding.admin.welcome.stepKeys')
        ]),
        infoBox(t('onboarding.admin.welcome.note'))
      )
    }
  },
  {
    element: sidebarLink('/admin/groups'),
    popover: {
      title: t('onboarding.admin.groups.title'),
      description: description(
        paragraph(t('onboarding.admin.groups.body')),
        infoBox(t('onboarding.admin.groups.tip'))
      ),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/admin/accounts'),
    popover: {
      title: t('onboarding.admin.accounts.title'),
      description: description(
        paragraph(t('onboarding.admin.accounts.body')),
        infoBox(t('onboarding.admin.accounts.tip'))
      ),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/keys'),
    popover: {
      title: t('onboarding.admin.keys.title'),
      description: description(
        paragraph(t('onboarding.admin.keys.body')),
        infoBox(t('onboarding.admin.keys.tip'))
      ),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/admin/usage'),
    popover: {
      title: t('onboarding.admin.usage.title'),
      description: description(paragraph(t('onboarding.admin.usage.body'))),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/admin/settings'),
    popover: {
      title: t('onboarding.admin.settings.title'),
      description: description(paragraph(t('onboarding.admin.settings.body'))),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: userMenuSelector(t),
    popover: {
      title: t('onboarding.admin.replay.title'),
      description: description(paragraph(t('onboarding.admin.replay.body'))),
      side: 'bottom',
      align: 'end'
    }
  }
]

export const getUserSteps = (t: Translate): DriveStep[] => [
  {
    popover: {
      title: t('onboarding.user.welcome.title'),
      description: description(
        paragraph(t('onboarding.user.welcome.intro')),
        orderedList([
          t('onboarding.user.welcome.stepKeys'),
          t('onboarding.user.welcome.stepCopy'),
          t('onboarding.user.welcome.stepUse')
        ])
      )
    }
  },
  {
    element: sidebarLink('/keys'),
    popover: {
      title: t('onboarding.user.keys.title'),
      description: description(
        paragraph(t('onboarding.user.keys.body')),
        infoBox(t('onboarding.user.keys.tip'))
      ),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/usage'),
    popover: {
      title: t('onboarding.user.usage.title'),
      description: description(paragraph(t('onboarding.user.usage.body'))),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/purchase'),
    popover: {
      title: t('onboarding.user.purchase.title'),
      description: description(paragraph(t('onboarding.user.purchase.body'))),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: sidebarLink('/redeem'),
    popover: {
      title: t('onboarding.user.redeem.title'),
      description: description(paragraph(t('onboarding.user.redeem.body'))),
      side: 'right',
      align: 'start'
    }
  },
  {
    element: userMenuSelector(t),
    popover: {
      title: t('onboarding.user.replay.title'),
      description: description(paragraph(t('onboarding.user.replay.body'))),
      side: 'bottom',
      align: 'end'
    }
  }
]
