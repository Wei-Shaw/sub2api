export default {
  qualityTest: {
    title: 'Quality Test',
    description: 'Run one prompt against up to three accounts side by side and compare the output',
    platform: 'Platform',
    reasoningEffort: 'Reasoning effort',
    effortDefault: 'Model default',
    slot: 'Account {n}',
    selectAccount: 'Select account',
    selectModel: 'Select model',
    prompt: 'Prompt',
    run: 'Run',
    stop: 'Stop',
    duration: 'Duration',
    chars: '{n} chars',
    preview: 'Preview',
    source: 'Source',
    loadAccountsFailed: 'Failed to load accounts',
    loadModelsFailed: 'Failed to load models',
    status: {
      idle: 'Idle',
      running: 'Running',
      completed: 'Completed',
      error: 'Error',
      stopped: 'Stopped'
    }
  }
}
