export default {
  batchImageGuide: {
    title: 'Batch Image Generation',
    description: 'Submit multiple prompts in one job and download the generated images when complete'
  },
  // Home Page
  home: {
    viewOnGithub: 'View on GitHub',
    viewDocs: 'View Documentation',
    docs: 'Docs',
    switchToLight: 'Switch to Light Mode',
    switchToDark: 'Switch to Dark Mode',
    dashboard: 'Dashboard',
    login: 'Login',
    getStarted: 'Get Started',
    goToDashboard: 'Go to Dashboard',
    // User-focused value proposition
    heroSubtitle: 'One Key, All AI Models',
    heroDescription: 'No need to manage multiple subscriptions. Access Claude, GPT, Gemini and more with a single API key',
    tags: {
      subscriptionToApi: 'Subscription to API',
      stickySession: 'Session Persistence',
      realtimeBilling: 'Pay As You Go'
    },
    // Pain points section
    painPoints: {
      title: 'Sound Familiar?',
      items: {
        expensive: {
          title: 'High Subscription Costs',
          desc: 'Paying for multiple AI subscriptions that add up every month'
        },
        complex: {
          title: 'Account Chaos',
          desc: 'Managing scattered accounts and API keys across different platforms'
        },
        unstable: {
          title: 'Service Interruptions',
          desc: 'Single accounts hitting rate limits and disrupting your workflow'
        },
        noControl: {
          title: 'No Usage Control',
          desc: "Can't track where your money goes or limit team member usage"
        }
      }
    },
    // Solutions section
    solutions: {
      title: 'We Solve These Problems',
      subtitle: 'Three simple steps to stress-free AI access'
    },
    features: {
      unifiedGateway: 'One-Click Access',
      unifiedGatewayDesc: 'Get a single API key to call all connected AI models. No separate applications needed.',
      multiAccount: 'Always Reliable',
      multiAccountDesc: 'Smart routing across multiple upstream accounts with automatic failover. Say goodbye to errors.',
      balanceQuota: 'Pay What You Use',
      balanceQuotaDesc: 'Usage-based billing with quota limits. Full visibility into team consumption.'
    },
    // Comparison section
    comparison: {
      title: 'Why Choose Us?',
      headers: {
        feature: 'Comparison',
        official: 'Official Subscriptions',
        us: 'Our Platform'
      },
      items: {
        pricing: {
          feature: 'Pricing',
          official: 'Fixed monthly fee, pay even if unused',
          us: 'Pay only for what you use'
        },
        models: {
          feature: 'Model Selection',
          official: 'Single provider only',
          us: 'Switch between models freely'
        },
        management: {
          feature: 'Account Management',
          official: 'Manage each service separately',
          us: 'Unified key, one dashboard'
        },
        stability: {
          feature: 'Stability',
          official: 'Single account rate limits',
          us: 'Multi-account pool, auto-failover'
        },
        control: {
          feature: 'Usage Control',
          official: 'Not available',
          us: 'Quotas & detailed analytics'
        }
      }
    },
    providers: {
      title: 'Supported AI Models',
      description: 'One API, Multiple Choices',
      supported: 'Supported',
      soon: 'Soon',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'More'
    },
    // CTA section
    cta: {
      title: 'Ready to Get Started?',
      description: 'Sign up now and get free trial credits to experience seamless AI access',
      button: 'Sign Up Free'
    },
    footer: {
      allRightsReserved: 'All rights reserved.'
    },
    styles: {
      platforms: {
        claude: { name: 'Claude' },
        gpt: { name: 'GPT' },
        gemini: { name: 'Gemini' },
        antigravity: { name: 'Antigravity' }
      },
      editorial: {
        eyebrow: 'Model routing, simplified',
        titleLineOne: 'One interface.',
        titleLineTwo: 'Every great model.',
        viewExample: 'View request example',
        trust: {
          unifiedAuth: 'Unified authentication',
          sdkCompatible: 'Compatible with common SDKs',
          usageBilling: 'Usage-based billing'
        },
        terminalLabel: 'API request example',
        request: 'request',
        command: 'curl -X POST /v1/chat/completions',
        routeComment: 'Selecting an available route',
        yourApp: 'Your app',
        gateway: 'Gateway',
        model: 'Model',
        responseCode: '200 OK',
        responseBody: 'content: Ready',
        details: {
          model: 'Model',
          modelValue: 'Selected per request',
          route: 'Route',
          routeValue: 'Handled by gateway',
          interface: 'Interface',
          interfaceValue: 'API compatible'
        },
        capabilitiesLabel: 'Platform capabilities',
        capabilities: {
          sdk: {
            title: 'Keep your SDK',
            body: 'Change the base URL, not the way your application works.'
          },
          routing: {
            title: 'Unified routing',
            body: 'Reach configured model routes through one gateway.'
          },
          billing: {
            title: 'Clear usage',
            body: 'Requests, tokens, and cost records remain available to review.'
          }
        },
        directoryEyebrow: 'Model directory',
        directoryTitle: 'Switch models from one entry point',
        directoryDescription: 'Choose a connected platform for work ranging from everyday tasks to complex reasoning.',
        platformDescriptions: {
          claude: 'Reasoning, writing, and code',
          gpt: 'General and multimodal work',
          gemini: 'Multimodal and long-context work',
          antigravity: 'Flexible routed workloads'
        }
      },
      operations: {
        eyebrow: 'Gateway control plane',
        demoNotice: 'Routing capability demo, not live monitoring',
        capabilitiesLabel: 'Routing capability overview',
        workspaceLabel: 'Routing capability demonstration',
        metrics: {
          routing: {
            label: 'Routing mode',
            value: 'Unified entry',
            note: 'Selects upstreams by configuration'
          },
          providers: {
            label: 'Platform examples',
            value: 'Four',
            note: 'Static capability display'
          },
          protocol: {
            label: 'Protocol',
            value: 'HTTPS',
            note: 'Encrypted transport capability'
          },
          api: {
            label: 'API surface',
            value: '/v1',
            note: 'Compatible request paths'
          }
        },
        matrixTitle: 'Routing capability matrix',
        matrixCaption: 'Configuration example / not live',
        routable: 'Routable',
        routes: {
          claude: { name: 'Claude / primary route', mode: 'Policy: priority' },
          gpt: { name: 'GPT / balanced route', mode: 'Policy: balanced' },
          gemini: { name: 'Gemini / fast route', mode: 'Policy: low latency' },
          antigravity: { name: 'Antigravity / flexible route', mode: 'Policy: configured' }
        },
        quickAccess: 'Quick access',
        disclaimer: 'This interface describes configurable routing and protocol capabilities. It does not read live health, latency, or availability data.',
        footerLabel: 'Routing capabilities'
      },
      minimal: {
        eyebrow: 'Independent model access',
        established: 'Est. {year}',
        kicker: 'A quieter way to build with AI.',
        notes: {
          endpoint: {
            title: 'One endpoint',
            body: 'Use a familiar interface for every supported model.'
          },
          choice: {
            title: 'Your choice',
            body: 'Move between providers without changing your product.'
          },
          usage: {
            title: 'Measured use',
            body: 'Review clear usage and billing for every request.'
          }
        }
      },
      catalog: {
        eyebrow: 'Model directory',
        title: 'Choose the right model for the work.',
        staticNote: 'This is a static platform directory. Actual model access depends on the instance configuration.',
        familiesTitle: 'Platform directory',
        providerCount: 'Four platforms',
        listed: 'Listed',
        capabilityTags: 'Capability tags',
        models: {
          claude: { description: 'Suited to careful reasoning, writing, analysis, and production code.' },
          gpt: { description: 'Suited to general, multimodal product, and agent tasks.' },
          gemini: { description: 'Suited to multimodal tasks and broad-context workflows.' },
          antigravity: { description: 'Suited to model workloads that need flexible routing.' }
        },
        tags: {
          reasoning: 'Reasoning',
          code: 'Code',
          longContext: 'Long context',
          general: 'General',
          tools: 'Tools',
          vision: 'Vision',
          multimodal: 'Multimodal',
          fast: 'Fast',
          context: 'Context',
          routing: 'Routing',
          flexible: 'Flexible',
          api: 'API'
        }
      },
      studio: {
        modelsNav: 'Models',
        featuresNav: 'Capabilities',
        docsNav: 'Docs',
        start: 'Get Started',
        eyebrow: 'ChatGPT · Claude API',
        heroLineOne: 'One API key.',
        heroLineTwo: 'Reliable access to leading models.',
        description: 'Use ChatGPT and Claude through one gateway without managing separate provider accounts. Routes fail over automatically, and billing follows actual usage.',
        getKey: 'Get API Key',
        viewDocs: 'View Integration Docs',
        docsSoon: 'Integration docs are coming soon',
        terminalLabel: 'API request routing example',
        request: 'request',
        switching: 'Switching to the {model} route',
        routeComment: 'Available {model} route selected',
        yourApp: 'Your app',
        gateway: 'Gateway',
        responseCode: '200 OK',
        responseBody: 'Ready',
        details: {
          model: 'Model',
          route: 'Route',
          routeValue: 'Automatic failover',
          interface: 'Interface',
          interfaceValue: 'API compatible'
        },
        featuresLabel: 'Service capabilities',
        features: {
          unified: { title: 'One-key access', body: 'Use one API key for every connected AI model without separate applications.' },
          reliable: { title: 'Reliable by design', body: 'Smart upstream scheduling, automatic failover, and load balancing reduce errors.' },
          usage: { title: 'Pay for usage', body: 'Usage-based billing, quota controls, and clear team consumption records.' }
        },
        modelsTitle: 'Available Models',
        modelsSubtitle: 'One account, unified access',
        available: 'Available',
        modelPricing: 'View model versions and live pricing',
        serviceNormal: 'Service operational',
        terms: 'Terms',
        privacy: 'Privacy',
        support: 'Support',
        termsSoon: 'Terms are coming soon',
        privacySoon: 'Privacy policy is coming soon',
        supportSoon: 'Support channels are coming soon',
        close: 'Close',
        modalTitle: 'Get Started with {siteName}',
        modalDescription: 'Create an account and API key to access ChatGPT and Claude from one place.',
        modalAction: 'Continue to Registration',
        modalAccountPrompt: 'Already registered?',
        modalLogin: 'Sign in to the dashboard'
      }
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'API Key Usage',
    subtitle: 'Enter your API Key to view real-time spending and usage status',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Query',
    querying: 'Querying...',
    privacyNote: 'Your Key is processed locally in the browser and will not be stored',
    dateRange: 'Date Range:',
    dateRangeToday: 'Today',
    dateRange7d: '7 Days',
    dateRange30d: '30 Days',
    dateRange90d: '90 Days',
    dateRangeCustom: 'Custom',
    apply: 'Apply',
    used: 'Used',
    detailInfo: 'Detail Information',
    tokenStats: 'Token Statistics',
    dailyDetail: 'Daily Detail',
    modelStats: 'Model Usage Statistics',
    // Table headers
    date: 'Date',
    model: 'Model',
    requests: 'Requests',
    inputTokens: 'Input Tokens',
    outputTokens: 'Output Tokens',
    cacheCreationTokens: 'Cache Creation',
    cacheReadTokens: 'Cache Read',
    cacheWriteTokens: 'Cache Write',
    totalTokens: 'Total Tokens',
    cost: 'Cost',
    // Status
    quotaMode: 'Key Quota Mode',
    walletBalance: 'Wallet Balance',
    // Ring card titles
    totalQuota: 'Total Quota',
    limit5h: '5-Hour Limit',
    limitDaily: 'Daily Limit',
    limit7d: '7-Day Limit',
    limitWeekly: 'Weekly Limit',
    limitMonthly: 'Monthly Limit',
    // Detail rows
    remainingQuota: 'Remaining Quota',
    expiresAt: 'Expires At',
    todayExpires: '(expires today)',
    daysLeft: '({days} days)',
    usedQuota: 'Used Quota',
    resetNow: 'Resetting soon',
    subscriptionType: 'Subscription Type',
    subscriptionExpires: 'Subscription Expires',
    // Usage stat cells
    todayRequests: 'Today Requests',
    todayInputTokens: 'Today Input',
    todayOutputTokens: 'Today Output',
    todayTokens: 'Today Tokens',
    todayCacheCreation: 'Today Cache Creation',
    todayCacheRead: 'Today Cache Read',
    todayCost: 'Today Cost',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Total Requests',
    totalInputTokens: 'Total Input',
    totalOutputTokens: 'Total Output',
    totalTokensLabel: 'Total Tokens',
    totalCacheCreation: 'Total Cache Creation',
    totalCacheRead: 'Total Cache Read',
    totalCost: 'Total Cost',
    avgDuration: 'Avg Duration',
    // Messages
    enterApiKey: 'Please enter an API Key',
    querySuccess: 'Query successful',
    queryFailed: 'Query failed',
    queryFailedRetry: 'Query failed, please try again later',
    noDailyUsage: 'No daily usage data',
  },

  // Setup Wizard
  setup: {
    title: 'Sub2API Setup',
    description: 'Configure your Sub2API instance',
    database: {
      title: 'Database Configuration',
      description: 'Connect to your PostgreSQL database',
      host: 'Host',
      port: 'Port',
      username: 'Username',
      password: 'Password',
      databaseName: 'Database Name',
      sslMode: 'SSL Mode',
      passwordPlaceholder: 'Password',
      ssl: {
        disable: 'Disable',
        require: 'Require',
        verifyCa: 'Verify CA',
        verifyFull: 'Verify Full'
      }
    },
    redis: {
      title: 'Redis Configuration',
      description: 'Connect to your Redis server',
      host: 'Host',
      port: 'Port',
      username: 'Username (optional)',
      password: 'Password (optional)',
      database: 'Database',
      usernamePlaceholder: 'Leave empty for default user',
      passwordPlaceholder: 'Password',
      enableTls: 'Enable TLS',
      enableTlsHint: 'Use TLS when connecting to Redis (public CA certs)'
    },
    admin: {
      title: 'Admin Account',
      description: 'Create your administrator account',
      email: 'Email',
      password: 'Password',
      confirmPassword: 'Confirm Password',
      passwordPlaceholder: 'Min 8 characters',
      confirmPasswordPlaceholder: 'Confirm password',
      passwordMismatch: 'Passwords do not match'
    },
    ready: {
      title: 'Ready to Install',
      description: 'Review your configuration and complete setup',
      database: 'Database',
      redis: 'Redis',
      adminEmail: 'Admin Email'
    },
    status: {
      testing: 'Testing...',
      success: 'Connection Successful',
      testConnection: 'Test Connection',
      installing: 'Installing...',
      completeInstallation: 'Complete Installation',
      completed: 'Installation completed!',
      redirecting: 'Redirecting to login page...',
      restarting: 'Service is restarting, please wait...',
      timeout: 'Service restart is taking longer than expected. Please refresh the page manually.'
    }
  },

  // Common
}
