/**
 * English i18n messages for the Gemini gateway plugin.
 *
 * These keys are merged into the host vue-i18n instance via
 * sdk.i18n.registerNamespace(). Keys under admin.accounts.gemini.*
 * and admin.accounts.oauth.gemini.* are Gemini-specific and owned
 * by this plugin. Shared keys (baseUrl, apiKey, vertex*, etc.)
 * remain in the host.
 */
export default {
  admin: {
    accounts: {
      gemini: {
        helpButton: 'Help',
        helpDialog: {
          title: 'Gemini Usage Guide',
          apiKeySection: 'API Key Links'
        },
        modelPassthrough: 'Gemini Model Passthrough',
        modelPassthroughDesc:
          'All model requests are forwarded directly to the Gemini API without model restrictions or mappings.',
        baseUrlHint: 'Leave default for official Gemini API',
        apiKeyHint: 'Your Gemini API Key (starts with AIza)',
        tier: {
          label: 'Account Tier',
          hint: 'Tip: The system will try to auto-detect the tier first; if auto-detection is unavailable or fails, your selected tier is used as a fallback (simulated quota).',
          aiStudioHint:
            'AI Studio quotas are per-model (Pro/Flash are limited independently). If billing is enabled, choose Pay-as-you-go.',
          googleOne: {
            free: 'Google One Free',
            pro: 'Google One Pro',
            ultra: 'Google One Ultra'
          },
          gcp: {
            standard: 'GCP Standard',
            enterprise: 'GCP Enterprise'
          },
          aiStudio: {
            free: 'Google AI Free',
            paid: 'Google AI Pay-as-you-go'
          }
        },
        accountType: {
          oauthTitle: 'OAuth (Gemini)',
          oauthDesc: 'Authorize with your Google account and choose an OAuth type.',
          apiKeyTitle: 'API Key (AI Studio)',
          apiKeyDesc: 'Fastest setup. Use an AIza API key.',
          apiKeyNote:
            'Best for light testing. Free tier has strict rate limits and data may be used for training.',
          apiKeyLink: 'Get API Key',
          quotaLink: 'Quota guide'
        },
        oauthType: {
          googleOneDesc: 'Personal accounts with Google One subscription quota',
          codeAssistDesc: 'Enterprise, requires GCP project',
          advancedToggle: 'Advanced options',
          builtInTitle: 'Built-in OAuth (Gemini CLI / Code Assist)',
          builtInDesc: 'Uses Google built-in client ID. No admin configuration required.',
          builtInRequirement: 'Requires a GCP project and Project ID.',
          gcpProjectLink: 'Create project',
          customTitle: 'Custom OAuth (AI Studio OAuth)',
          customDesc: 'Uses admin-configured OAuth client for org management.',
          customRequirement: 'Admin must configure Client ID and add you as a test user.',
          badges: {
            recommended: 'Recommended',
            highConcurrency: 'High concurrency',
            noAdmin: 'No admin setup',
            orgManaged: 'Org managed',
            adminRequired: 'Admin required'
          }
        },
        rateLimit: {
          ok: 'Not rate limited',
          unlimited: 'Unlimited',
          limited: 'Rate limited {time}',
          now: 'now'
        },
        test: {
          imagePromptLabel: 'Image Prompt',
          imagePromptPlaceholder: 'Describe the image you want to generate...',
          imagePromptHint: 'Imagen models generate images from text descriptions.',
          imagePromptDefault: 'A serene mountain landscape at sunset with a reflective lake'
        }
      },
      oauth: {
        gemini: {
          oauthTypeLabel: 'OAuth Type',
          aiStudioNotConfiguredShort: 'Not configured',
          aiStudioNotConfiguredTip:
            'AI Studio OAuth is not configured: set GEMINI_OAUTH_CLIENT_ID / GEMINI_OAUTH_CLIENT_SECRET and add Redirect URI: http://localhost:1455/auth/callback (Consent screen scopes must include https://www.googleapis.com/auth/generative-language.retriever)',
          failedToGenerateUrl: 'Failed to generate Gemini auth URL',
          missingExchangeParams: 'Missing auth code, session ID, or state',
          failedToExchangeCode: 'Failed to exchange Gemini auth code',
          missingProjectId: 'GCP Project ID retrieval failed: Your Google account is not linked to an active GCP project. Please activate GCP and bind a credit card in Google Cloud Console, or manually enter the Project ID during authorization.'
        }
      }
    }
  }
}
