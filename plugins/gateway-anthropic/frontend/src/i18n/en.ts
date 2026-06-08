/**
 * Gateway Anthropic plugin i18n messages (English).
 *
 * Keys are merged flat into the host i18n dictionary via
 * sdk.i18n.registerNamespace(). They follow the same nesting
 * as the host admin.accounts.* namespace.
 */
export default {
  admin: {
    accounts: {
      anthropic: {
        apiKeyPassthrough: 'Auto passthrough (auth only)',
        apiKeyPassthroughDesc:
          'Only applies to Anthropic API Key accounts. When enabled, messages/count_tokens are forwarded in passthrough mode with auth replacement only, while billing/concurrency/audit and safety filtering are preserved. Disable to roll back immediately.',
        syncToStream: 'Sync to Stream',
        syncToStreamDesc:
          'When enabled, all synchronous (non-streaming) requests are sent to upstream as streaming, and the complete response is assembled before returning as a non-streaming response.',
        syncToStreamDefault: 'Default',
        syncToStreamEnabled: 'Enabled',
      },
      quotaControl: {
        title: 'Quota Control',
        hint: 'Configure cost windows, session limits and other scheduling controls.',
        windowCost: {
          label: '5h Window Cost Limit',
          hint: 'Limit account cost usage within the 5-hour window',
          limit: 'Cost Threshold',
          limitPlaceholder: '50',
          limitHint: 'Account will not participate in new scheduling after reaching threshold',
          stickyReserve: 'Sticky Reserve',
          stickyReservePlaceholder: '10',
          stickyReserveHint: 'Additional reserve for sticky sessions',
        },
        sessionLimit: {
          label: 'Session Count Limit',
          hint: 'Limit the number of active concurrent sessions',
          maxSessions: 'Max Sessions',
          maxSessionsPlaceholder: '3',
          maxSessionsHint: 'Maximum number of active concurrent sessions',
          idleTimeout: 'Idle Timeout',
          idleTimeoutPlaceholder: '5',
          idleTimeoutHint: 'Sessions will be released after idle timeout',
        },
        rpmLimit: {
          label: 'RPM Limit',
          hint: 'Limit requests per minute to protect upstream accounts',
          baseRpm: 'Base RPM',
          baseRpmPlaceholder: '15',
          baseRpmHint: 'Max requests per minute, 0 or empty means no limit',
          strategy: 'RPM Strategy',
          strategyTiered: 'Tiered Model',
          strategyStickyExempt: 'Sticky Exempt',
          strategyHint: 'Tiered: gradually restrict when exceeded; Sticky Exempt: existing sessions unrestricted',
          stickyBuffer: 'Sticky Buffer',
          stickyBufferPlaceholder: 'Default: 20% of base RPM',
          stickyBufferHint: 'Extra requests allowed for sticky sessions after exceeding base RPM. Leave empty to use default (20% of base RPM, min 1)',
          userMsgQueue: 'User Message Rate Control',
          userMsgQueueHint: 'Rate-limit user messages to avoid triggering upstream RPM limits',
          umqModeOff: 'Off',
          umqModeThrottle: 'Throttle',
          umqModeSerialize: 'Serialize',
        },
        tlsFingerprint: {
          label: 'TLS Fingerprint Simulation',
          hint: 'Simulate Node.js/Claude Code client TLS fingerprint',
          defaultProfile: 'Built-in Default',
          randomProfile: 'Random',
        },
        sessionIdMasking: {
          label: 'Session ID Masking',
          hint: 'When enabled, fixes the session ID in metadata.user_id for 15 minutes, making upstream think requests come from the same session',
        },
        cacheTTLOverride: {
          label: 'Cache TTL Override',
          hint: 'Force all cache creation tokens to be billed as the selected TTL tier (5m or 1h)',
          target: 'Target TTL',
          targetHint: 'Select the TTL tier for billing',
        },
        customBaseUrl: {
          label: 'Custom Relay URL',
          hint: 'Forward requests to a custom relay service. Proxy URL will be passed as a query parameter.',
          urlHint: 'Relay service URL (e.g., https://relay.example.com)',
        },
      },
      // Keys used by the form template that live under admin.accounts.*
      vertexAnthropicHint: 'Use a Google Cloud Service Account JSON to call Anthropic Claude via Vertex AI. It is recommended to configure model mapping to map client Claude model names to Vertex model IDs.',
      addMethod: 'Add Method',
      setupTokenLongLived: 'Setup Token (Long-lived)',
      baseUrl: 'Base URL',
      baseUrlHint: 'Leave default for official Anthropic API',
      apiKey: 'API Key',
      leaveEmptyToKeep: 'Leave empty to keep current key',
      pleaseEnterApiKey: 'Please enter API Key',
      webSearchEmulation: 'Web Search Emulation',
      webSearchEmulationDefault: 'Default',
      webSearchEmulationForceOn: 'Force On',
      webSearchEmulationForceOff: 'Force Off',
      interceptWarmupRequests: 'Intercept Warmup Requests',
      interceptWarmupRequestsDesc:
        'When enabled, warmup requests like title generation will return mock responses without consuming upstream tokens',
      // Bedrock keys
      bedrockAccessKeyId: 'AWS Access Key ID',
      bedrockSecretAccessKey: 'AWS Secret Access Key',
      bedrockSessionToken: 'AWS Session Token',
      bedrockRegion: 'AWS Region',
      bedrockRegionHint: 'e.g. us-east-1, us-west-2, eu-west-1',
      bedrockForceGlobal: 'Force Global cross-region inference',
      bedrockForceGlobalHint: 'When enabled, model IDs use the global. prefix (e.g. global.anthropic.claude-...), routing requests to any supported region worldwide for higher availability',
      bedrockAccessKeyIdRequired: 'Please enter AWS Access Key ID',
      bedrockSecretAccessKeyRequired: 'Please enter AWS Secret Access Key',
      bedrockSessionTokenHint: 'Optional, for temporary credentials',
      bedrockAuthMode: 'Authentication Mode',
      bedrockAuthModeSigv4: 'SigV4 Signing',
      bedrockAuthModeApikey: 'Bedrock API Key',
      bedrockApiKeyInput: 'API Key',
      bedrockApiKeyRequired: 'Please enter Bedrock API Key',
      // Vertex keys
      vertexSaJsonMissingFields: 'Service Account JSON is missing project_id, client_email, or private_key',
      vertexLocationRequired: 'Please enter a Vertex location',
      // OAuth keys
      oauth: {
        failedToGenerateUrl: 'Failed to generate auth URL',
      },
      types: {
        oauth: 'OAuth',
      },
    },
  },
}
