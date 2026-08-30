export default {
  officialPricing: {
    title: 'Official Price Management',
    description: 'Maintain official base prices for token-metered models and review every diff before applying sync candidates.',
    eyebrow: 'Token price workspace',
    check: 'Check official prices',
    checking: 'Checking',
    refresh: 'Refresh models',
    tokenOnly: 'Only official prices for token-metered models are managed here. Per-request prices remain manually priced and are never included in official-price sync.',
    isolationNotice: 'Official base prices are used only for the customer price catalogue and multiplier calculations. They do not directly change channel costs, group pricing, or actual billing.',
    loadFailed: 'Failed to load official prices',
    search: 'Search models or providers',
    modelCount: '{count} token models',
    providerCount: '{count} providers',
    providerModels: '{count} models',
    noModels: 'No matching token-metered models',
    manual: {
      title: 'Set official prices manually',
      hint: 'Edit the four official price fields inline. Blank means no official price is available. Manual saves are marked as administrator-maintained while multipliers, notes, and other billing settings remain unchanged.',
      save: 'Save prices',
      saving: 'Saving',
      saved: 'Official prices saved for {model}',
      saveFailed: 'Failed to save official prices',
      invalid: 'An official price must be blank or greater than or equal to zero'
    },
    fields: {
      model: 'Model',
      input: 'Input',
      output: 'Output',
      cacheWrite: 'Cache write',
      cacheRead: 'Cache read',
      source: 'Source and time',
      actions: 'Actions',
      unit: 'per 1M tokens'
    },
    metadata: {
      neverSynced: 'Never synced',
      syncedAt: 'Synced {time}',
      sourceLink: 'View source',
      sourceUnknown: 'Source not recorded'
    },
    sync: {
      title: 'Official-price sync diff',
      hint: 'A check creates candidates only and never changes prices automatically. Review evidence, timestamps, and diffs before selecting models to apply.',
      fetchedAt: 'Checked: {time}',
      candidates: '{count} candidates',
      changed: '{count} changed',
      applicable: '{count} applicable',
      selected: '{count} selected',
      current: 'Current official price',
      proposed: 'Candidate official price',
      evidence: 'Source evidence',
      status: 'Status',
      applySelected: 'Apply selected',
      applying: 'Applying',
      applied: 'Applied official prices for {count} models',
      noSelection: 'Select at least one applicable price candidate',
      empty: 'No official-price candidates were returned',
      checkFailed: 'Failed to check official prices',
      applyFailed: 'Failed to apply official prices. The candidates may be stale; check again.',
      selectAll: 'Select every applicable item',
      changedStatus: 'Price changed',
      noChange: 'No price change',
      applicableStatus: 'Applicable',
      notApplicable: 'Not applicable',
      reasonFallback: 'Source data is incomplete or the model version does not match',
      sourceUpdatedAt: 'Source updated: {time}',
      reference: 'Official reference',
      unverifiedWarning: 'Aggregated sources are candidate leads only and are not provider-verified official prices. Check the reference before applying.',
    },
    source: {
      herohao_aggregate: 'Aggregated candidate · unverified',
      provider_official: 'Official provider source',
      manual: 'Maintained manually',
      unknown: 'Unknown source'
    },
    confidence: {
      high: 'High confidence',
      medium: 'Medium confidence',
      low: 'Low confidence',
      unverified: 'Unverified',
      unknown: 'Unknown confidence'
    },
    reason: {
      unsupported_billing_mode: 'This model is not token-metered and is excluded from official-price sync',
      currency_mismatch: 'The candidate currency does not match the model display currency',
      candidate_not_found: 'No exact model match was found in the aggregated source',
      provider_mismatch: 'The model provider does not match the candidate source',
      candidate_disabled: 'This model is disabled in the source',
      candidate_price_missing: 'The source did not provide any applicable official price fields'
    }
  }
}
