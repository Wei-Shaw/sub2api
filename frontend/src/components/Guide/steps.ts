import { DriveStep } from 'driver.js'

/**
 * 管理员完整引导流程
 * 交互式引导：指引用户实际操作
 * @param t 国际化函数
 * @param isSimpleMode 是否为简易模式（简易模式下会过滤分组相关步骤）
 */
export const getAdminSteps = (t: (key: string) => string, isSimpleMode = false): DriveStep[] => {
  const allSteps: DriveStep[] = [
  // ========== 欢迎介绍 ==========
  {
    popover: {
      title: t('onboarding.admin.welcome.title'),
      description: t('onboarding.admin.welcome.description'),
      align: 'center',
      nextBtnText: t('onboarding.admin.welcome.nextBtn'),
      prevBtnText: t('onboarding.admin.welcome.prevBtn')
    }
  },

  // ========== 第一部分：创建分组 ==========
  {
    element: '#sidebar-group-manage',
    popover: {
      title: t('onboarding.admin.groupManage.title'),
      description: t('onboarding.admin.groupManage.description'),
      side: 'right',
      align: 'center',
      showButtons: ['close'],
    }
  },
  {
    element: '[data-tour="groups-create-btn"]',
    popover: {
      title: t('onboarding.admin.createGroup.title'),
      description: t('onboarding.admin.createGroup.description'),
      side: 'bottom',
      align: 'end',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="group-form-name"]',
    popover: {
      title: t('onboarding.admin.groupName.title'),
      description: t('onboarding.admin.groupName.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="group-form-platform"]',
    popover: {
      title: t('onboarding.admin.groupPlatform.title'),
      description: t('onboarding.admin.groupPlatform.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="group-form-multiplier"]',
    popover: {
      title: t('onboarding.admin.groupMultiplier.title'),
      description: t('onboarding.admin.groupMultiplier.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="group-form-exclusive"]',
    popover: {
      title: t('onboarding.admin.groupExclusive.title'),
      description: t('onboarding.admin.groupExclusive.description'),
      side: 'top',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="group-form-submit"]',
    popover: {
      title: t('onboarding.admin.groupSubmit.title'),
      description: t('onboarding.admin.groupSubmit.description'),
      side: 'left',
      align: 'center',
      showButtons: ['close']
    }
  },

  // ========== 第二部分：创建账号授权 ==========
  {
    element: '#sidebar-channel-manage',
    popover: {
      title: t('onboarding.admin.accountManage.title'),
      description: t('onboarding.admin.accountManage.description'),
      side: 'right',
      align: 'center',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="accounts-create-btn"]',
    popover: {
      title: t('onboarding.admin.createAccount.title'),
      description: t('onboarding.admin.createAccount.description'),
      side: 'bottom',
      align: 'end',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="account-form-name"]',
    popover: {
      title: t('onboarding.admin.accountName.title'),
      description: t('onboarding.admin.accountName.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="account-form-platform"]',
    popover: {
      title: t('onboarding.admin.accountPlatform.title'),
      description: t('onboarding.admin.accountPlatform.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="account-form-type"]',
    popover: {
      title: t('onboarding.admin.accountType.title'),
      description: t('onboarding.admin.accountType.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="account-form-priority"]',
    popover: {
      title: t('onboarding.admin.accountPriority.title'),
      description: t('onboarding.admin.accountPriority.description'),
      side: 'top',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="account-form-groups"]',
    popover: {
      title: t('onboarding.admin.accountGroups.title'),
      description: t('onboarding.admin.accountGroups.description'),
      side: 'top',
      align: 'center',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="account-form-submit"]',
    popover: {
      title: t('onboarding.admin.accountSubmit.title'),
      description: t('onboarding.admin.accountSubmit.description'),
      side: 'left',
      align: 'center',
      showButtons: ['close']
    }
  },

  // ========== 第三部分：创建API密钥 ==========
  {
    element: '[data-tour="sidebar-my-keys"]',
    popover: {
      title: t('onboarding.admin.keyManage.title'),
      description: t('onboarding.admin.keyManage.description'),
      side: 'right',
      align: 'center',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="keys-create-btn"]',
    popover: {
      title: t('onboarding.admin.createKey.title'),
      description: t('onboarding.admin.createKey.description'),
      side: 'bottom',
      align: 'end',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="key-form-name"]',
    popover: {
      title: t('onboarding.admin.keyName.title'),
      description: t('onboarding.admin.keyName.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="key-form-group"]',
    popover: {
      title: t('onboarding.admin.keyGroup.title'),
      description: t('onboarding.admin.keyGroup.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="key-form-submit"]',
    popover: {
      title: t('onboarding.admin.keySubmit.title'),
      description: t('onboarding.admin.keySubmit.description'),
      side: 'left',
      align: 'center',
      showButtons: ['close']
    }
  }
  ]

  // 简易模式下过滤分组相关步骤
  if (isSimpleMode) {
    return allSteps.filter(step => {
      const element = step.element as string | undefined
      // 过滤掉分组管理和账号分组选择相关步骤
      return !element || (
        !element.includes('sidebar-group-manage') &&
        !element.includes('groups-create-btn') &&
        !element.includes('group-form-') &&
        !element.includes('account-form-groups')
      )
    })
  }

  return allSteps
}

/**
 * 普通用户引导流程
 */
export const getUserSteps = (t: (key: string) => string): DriveStep[] => [
  {
    popover: {
      title: t('onboarding.user.welcome.title'),
      description: t('onboarding.user.welcome.description'),
      align: 'center',
      nextBtnText: t('onboarding.user.welcome.nextBtn'),
      prevBtnText: t('onboarding.user.welcome.prevBtn')
    }
  },
  {
    element: '[data-tour="sidebar-my-keys"]',
    popover: {
      title: t('onboarding.user.keyManage.title'),
      description: t('onboarding.user.keyManage.description'),
      side: 'right',
      align: 'center',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="keys-create-btn"]',
    popover: {
      title: t('onboarding.user.createKey.title'),
      description: t('onboarding.user.createKey.description'),
      side: 'bottom',
      align: 'end',
      showButtons: ['close']
    }
  },
  {
    element: '[data-tour="key-form-name"]',
    popover: {
      title: t('onboarding.user.keyName.title'),
      description: t('onboarding.user.keyName.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="key-form-group"]',
    popover: {
      title: t('onboarding.user.keyGroup.title'),
      description: t('onboarding.user.keyGroup.description'),
      side: 'right',
      align: 'start',
      showButtons: ['next', 'previous']
    }
  },
  {
    element: '[data-tour="key-form-submit"]',
    popover: {
      title: t('onboarding.user.keySubmit.title'),
      description: t('onboarding.user.keySubmit.description'),
      side: 'left',
      align: 'center',
      showButtons: ['close']
    }
  }
]
