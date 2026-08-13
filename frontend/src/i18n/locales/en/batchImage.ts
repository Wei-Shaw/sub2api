/**
 * The Codex skill document offered by the batch image usage guide.
 *
 * This is a *named* export, deliberately outside the message tree below, and its
 * zh twin lives at the same position in `locales/zh/batchImage.ts`. The reason is
 * that vue-i18n compiles every message in the tree at render time, and this
 * document is not UI copy — it is ~65 lines the user copies into their agent,
 * containing a JSON request body and six URL templates with a literal `{id}`
 * segment. In message syntax every one of those braces would have to be written
 * `{'{'}` / `{'}'}`, which turns the payload example into unreadable noise and
 * makes a silently-eaten brace the likeliest way this file ever breaks. Keeping
 * it a plain template literal means what is written here is byte-for-byte what
 * the user copies.
 *
 * Only the instructional prose is translated. Field names, parameter names,
 * URLs, JSON structure, literal values (`"1K"`, `"image/png"`, `"img_001"`),
 * numbers, and the line/indent layout are identical across locales — a mistake
 * there breaks the reader's workflow rather than merely reading badly.
 *
 * @param endpoint API base URL, already stripped of any trailing slash.
 */
export function batchImageAgentInstruction(endpoint: string): string {
  return `---
name: sub2api-batch-image
description: Use this when the user wants to batch-generate images with Gemini/Vertex, run a list of prompts in bulk, download batch image results, or retry failed images.
---

You are the batch image-generation agent inside Codex. The user does not have to fill in the page's form by hand; work out the task name, the prompt list and the output directory from the current chat, from the files or directories the user gave you, and from the surrounding context. Ask the user only when a decision you cannot make is genuinely missing.

Default endpoint:
${endpoint}

You are responsible for all of the following:
1. Extract the prompts from the chat or from the attachments. Keep the full text of every prompt and assign stable custom_id values in order, for example img_001, img_002.
2. Infer the task name from the user's request or from context; when there is no explicit name, build one from the current time.
3. Infer the output directory from the user's request or from context; ask the user only if they never said where to save the results.
4. Before submitting you must compute expected_output_count = the sum of output_count across all items. A single batch job is hard-capped at 200 output images; anything over 200 must be split into several jobs. Never submit one oversized job, and never mistake the reference-image attachment cap for a cap on the number of images generated.
5. If the user supplies reference images, bind each one to the specific item it is meant for. A reference image is an input attachment, not an output image. The per-item limit is set by the model and must be enforced per model: Gemini 2.5 Flash Image allows at most 3 reference images per item; Gemini 3 Pro Image allows at most 14 reference images per item. Do not read the backend's attachment guardrails as Pro's per-item capability: after expanding by output_count, the total number of reference-image attachments across all items has an internal protection threshold of 1000, and inline base64 reference images may total at most 128MB once decoded. That 1000 is only the threshold at which the server rejects abnormal requests, not a recommended size; split the work yourself when there are many reference images or the request body grows large.
6. Reference images are charged as input tokens once per output_count; for large jobs, for one reference image reused many times, or when the reference images are large in total, prefer gs:// file_uri or split the work into several jobs.
7. Choosing the API key and the model: first fetch the batch image keys/models currently available; if the user named a model and that key supports it, use the model the user named; otherwise use the default/first of the models available to that key. Never show or ask about internal provider names.
8. Call the batch image API to submit, poll and download; do not ask the user to type anything into the page.

API contract:
- Models: GET ${endpoint}/v1/images/batches/models
- Submit: POST ${endpoint}/v1/images/batches
- Status: GET ${endpoint}/v1/images/batches/{id}
- Items: GET ${endpoint}/v1/images/batches/{id}/items
- Download: GET ${endpoint}/v1/images/batches/{id}/download
- Cancel: POST ${endpoint}/v1/images/batches/{id}/cancel

Submit body:
{
  "model": "<one of the models available to the selected key>",
  "task_name": "<inferred from the chat; the current time when empty>",
  "image_size": "1K",
  "response_mime_type": "image/png",
  "items": [
    {
      "custom_id": "img_001",
      "prompt": "<the full text of the first prompt>",
      "output_count": 1,
      "reference_images": [
        {
          "id": "face",
          "type": "subject",
          "mime_type": "image/png",
          "data": "<base64, without the data:image/png;base64, prefix>"
        }
      ]
    }
  ]
}

You must:
- Never write an API key into the repository, into a log, into a commit, or into your final reply.
- Never write reference-image base64 into your final reply, into a log, or into a public file. The resume record keeps only the reference images' file names, purpose and count plus the path to the request JSON file; if that request JSON contains base64, keep it in the output directory the user chose and do not commit it.
- output_count is how many images the same prompt and reference images generate; it defaults to 1 and is at most 4 per item. It does not rely on Gemini returning several images from one request — the system expands it into that many real task items. Before submitting you must confirm that the expected total output does not exceed 200, and split the work into several jobs when it does. Never submit a job that would generate more than 200 images just because reference-image attachments have a higher internal protection threshold.
- Batch image generation is still billed to the user by the number of successfully generated images; reference images are not priced separately. You may explain to the user that reference images add a small amount of upstream input tokens and temporary storage cost, counted again for every output_count, and that the hold/settled amount shown on the page is computed from the number of output images.
- Immediately after a successful submit you must write a local resume record into the output directory, for example batch-image-resume.json. Never store an API key in the resume record.
- The resume record must contain at least: endpoint, task_name, batch_id, model, output_dir, request_file, submitted_at, last_status, status_url, items_url, download_url, prompt_count, expected_output_count, plus either a custom_id-to-prompt map or the path to the request JSON file so failed items can be retried.
- Update the resume record after every status check with last_checked_at, last_status, the success count, the failure count, the actual charge and a summary of the failures. If the session is interrupted or paused, that file alone must be enough to resume querying, downloading or retrying next time.
- Do not poll aggressively. Wait roughly 20 to 30 seconds before the first status check; poll a queued job every 60 to 120 seconds; if it is still queued after 3 checks in a row, stop polling for now, tell the user the job is still in the queue, keep the resume record, and either move on to other work or wait for the user to ask you to resume later.
- Poll a running job roughly every 60 seconds, and less often for large jobs or when the server is under load; states close to completion such as processing_results can be polled every 20 to 45 seconds.
- When the job finishes, report the task name, the task id, the success count, the failure count, the actual charge and where the files were saved.
- Download successful images only. On a partial failure, first show the failed custom_id values, the error codes, where each error came from and a short reason.
- Retry failed items only; never resubmit an item that already succeeded. If an older job did not save the prompts of its failed items, you must tell the user it cannot be retried automatically and ask whether they will supply the original prompts.
- Before cancelling a job you must warn the user that images already indexed as successful are still billed as successful items, and that the rest of the hold is released.
- Load image previews on demand; never bulk-load image content just to look at a list.`
}

export default {
  batchImage: {
    columns: {
      taskName: 'Task name',
      model: 'Model',
      apiKey: 'API key',
      result: 'Results',
      cost: 'Cost',
      downloadStatus: 'Download status',
    },
    status: {
      queued: 'Queued',
      running: 'Generating',
      processingResults: 'Processing results',
      settling: 'Settling',
      completed: 'Completed',
      failed: 'Failed',
      cancelled: 'Cancelled',
      outputDeleted: 'Results deleted',
      partialSuccess: 'Partially succeeded',
      allFailed: 'All failed',
    },
    itemStatus: {
      pending: 'Queued',
      succeeded: 'Succeeded',
      failed: 'Failed',
      cancelled: 'Cancelled',
      recovered: 'Recovered by retry',
    },
    filters: {
      searchTaskName: 'Search task name',
      allApiKeys: 'All API keys',
      allStatuses: 'All statuses',
      allDownloadStates: 'All download states',
      downloaded: 'Downloaded',
      notDownloaded: 'Not downloaded',
    },
    actions: {
      usageGuide: 'Usage guide',
      createJob: 'Create batch job',
      downloadSelected: 'Download selected',
      deleteRecords: 'Delete records',
      retryFailedItems: 'Retry failed items',
      cancelJob: 'Cancel job',
      downloadZip: 'Download ZIP',
      viewDetail: 'View details',
      download: 'Download',
      moreActions: 'More actions',
      copyInstruction: 'Copy instructions',
      submitJob: 'Submit job',
    },
    list: {
      selectedJobs: 'Selected {count} job | Selected {count} jobs',
      expandChildren: 'Expand {n} subtask | Expand {n} subtasks',
      collapseChildren: 'Collapse subtasks',
      childCount: '{n} subtask | {n} subtasks',
      childBadge: 'Subtask',
      keyNotRecorded: 'Not recorded',
      totalCount: 'of {n}',
      notDownloaded: 'Not downloaded',
      empty: 'No batch jobs yet',
      emptyHint: 'Use the button in the top-right corner to create a batch job.',
    },
    pagination: {
      pageNumber: 'Page {page}',
      pageItems: '{count} on this page',
    },
    promptPopover: {
      title: 'Full prompt',
      copied: 'Prompt copied',
    },
    detail: {
      title: 'Job details',
      aggregatedResult: 'Combined results',
      result: 'Results',
      cost: 'Cost',
      downloadStatus: 'Download status',
      items: 'Items',
      customId: 'Custom ID',
      prompt: 'Prompt',
      preview: 'Preview',
      previewZoom: 'Zoom compressed preview {id}',
      previewReload: 'Reload compressed preview',
      previewLoad: 'Load compressed preview',
      previewUnavailable: 'Preview unavailable',
      noImage: 'No image',
      loadingItems: 'Loading items...',
      noItems: 'No items yet',
      noItemsHint: 'Queued or generating jobs show submitted prompts first; image statuses update once results are processed.',
      mainTask: 'Main job: {name}',
      childTask: 'Subtask: {name}',
      holdCost: 'Hold {amount}',
    },
    itemResult: {
      recoveredByRetry: 'Previous failure recovered by a retry subtask',
      readyPreview: 'Image generated. Click to preview.',
      readyDownload: 'Image generated and ready to download.',
      noUsableImage: 'No usable image was generated.',
      cancelled: 'Job cancelled.',
      waiting: 'Waiting for results.',
      emptyImageOutput: 'The upstream returned a result, but this item has no image content. This usually means a single Gemini/Vertex generation failed or was blocked by safety policies.',
      providerItemFailed: 'The upstream result for this item has no usable image.',
    },
    imagePreview: {
      title: 'Image preview',
      notice: 'This is a compressed thumbnail cached locally in your browser, so quality is reduced. Download the ZIP to view the original image.',
    },
    create: {
      title: 'Create batch job',
      taskName: 'Task name',
      taskNamePlaceholder: 'Defaults to the current time if left empty',
      loadingKeys: 'Loading API keys...',
      selectKeyPlaceholder: 'Select a Gemini API key',
      noKeysHint: 'No Gemini API key is available for batch image generation. Create one and bind it to a Gemini group with batch image generation enabled first.',
      model: 'Model',
      imageSize: 'Image size',
      imageSizeHint: 'Batch jobs are currently submitted at a fixed 1K image size.',
      outputFormat: 'Output format',
      estimatedOutput: 'Estimated output',
      estimatedOutputValue: '{images} images / {prompts} prompts',
      promptSection: 'Prompt',
      promptAdded: '{count} added',
      promptPlaceholder: 'Paste a prompt, then add it to the list below',
      customIdPlaceholder: 'Custom ID (optional)',
      outputCountPerPrompt: 'Images per prompt',
      outputCountOption: '{n} image | {n} images',
      referenceImage: 'Reference images',
      removeReferenceImage: 'Remove reference image',
      limitsHint: 'Up to {maxPerItem} images per prompt and {maxPerJob} per job. The current model allows up to {refLimit} reference images per prompt; reference images consume input tokens once per generated image.',
      referenceCount: '{n} reference image | {n} reference images',
      noPrompts: 'No prompts added yet.',
      cancelNotice: 'Cancelling requests an upstream cancellation. Images already indexed as successful will still be billed, and the remaining hold will be released.',
      submittingNotice: 'Creating the upstream batch job. This usually takes a few seconds; please do not submit again.',
      modelNoReferenceImages: 'The current model does not support reference images.',
      refLimitReached: 'The current model allows up to {limit} reference images per prompt.',
      refLimitExceededIgnored: 'The current model allows up to {limit} reference images per prompt. Extra files were ignored.',
      refFormatUnsupported: 'Reference images must be PNG, JPEG, or WebP.',
      refFileTooLarge: '{name} exceeds 10MB and was ignored.',
    },
    guide: {
      title: 'Batch Image Generation Guide',
      uiTitle: 'How to use this page',
      step1: '1. Select a Gemini API key with batch image generation enabled. The model list shows the models available to that key’s group.',
      step2: '2. The task name can be left empty; the current time is used automatically on submit. Prompts are added to the list one by one, and each prompt can carry reference images and a repeat count.',
      step3: '3. After submitting, the job is queued first and the item list shows the submitted prompts. Image previews are not loaded by default; click the preview button on an item to load a single image.',
      step4: '4. Once completed you can download the ZIP. If some items failed, the More menu lets you retry only the failed items. Billing is still based on the number of successfully generated images; reference images are not billed separately.',
      skillTitle: 'Skill instructions for Codex',
      skillDesc: 'Tells Codex how to organize prompts, submit jobs, and download results on behalf of the user.',
      /*
       * Stands in for the API endpoint when neither a configured base URL nor a
       * `window.location` is available. It is printed inside the skill document
       * below, so it keeps the same angle-bracket "fill this in" shape as the
       * placeholders in that document.
       */
      endpointFallback: '<your Sub2API API endpoint>',
    },
    messages: {
      loadKeysFailed: 'Failed to load API keys.',
      loadModelsFailed: 'Failed to load available models.',
      loadJobsFailed: 'Failed to load batch jobs.',
      selectApiKey: 'Select an available Gemini API key.',
      noModelsForKey: 'This key has no available batch image models.',
      selectModel: 'Select a model.',
      promptRequired: 'Enter at least one prompt.',
      submitted: 'Batch job submitted.',
      submitFailed: 'Failed to submit the batch job.',
      refreshFailed: 'Failed to refresh the job.',
      cancelConfirm: 'Cancellation will be sent upstream. Images already indexed as successful will still be billed, and the remaining hold will be released. Continue?',
      cancelled: 'Cancellation requested.',
      cancelFailed: 'Failed to cancel the job.',
      batchDownloadStarted: 'Downloads for the selected jobs have started.',
      downloadFailed: 'Failed to download the result.',
      retrySubmitted: 'Retry job submitted for failed items.',
      retryFailed: 'Failed to retry failed items.',
      retryMissingPrompts: 'This job does not have saved prompts for failed items, so it cannot be retried automatically. Recreate it with the original prompt.',
      retryTaskNameSuffix: 'Retry failed items',
      deleteConfirm: 'This hides the job from your list while keeping billing records. Delete it?',
      deleteSelectedConfirm: 'This hides the selected jobs from your list while keeping billing records. Delete them?',
      deleted: 'Job record deleted.',
      deleteFailed: 'Failed to delete the job record.',
      loadItemsFailed: 'Failed to load item details.',
      loadPreviewFailed: 'Failed to load the image preview.',
      copiedInstruction: 'Batch image instructions copied.',
      loadingModels: 'Loading available models...',
      noModels: 'No available models',
      noModelsHint: 'This key’s group has no models configured for batch image generation.',
      noCompatibleAccount: 'No usable upstream batch image account is available for this key’s group. Contact an administrator to check the group’s schedulable Gemini API key or Vertex service account and model support.',
      unsupportedProvider: 'The batch image provider for this job is not available. Contact an administrator to check the batch image provider configuration.',
      providerSubmitFailed: 'The upstream batch image job failed to submit. Contact an administrator to check the upstream account, model permission, or provider status.',
      vertexGcsBucketMissing: 'Vertex batch image generation is missing the managed GCS bucket configuration. Contact an administrator to configure BATCH_IMAGE_VERTEX_MANAGED_GCS_BUCKET before submitting again.',
      queueFailed: 'The task queue is temporarily unavailable, so the batch job was not queued. Contact an administrator to check the queue service.',
      billingHoldFailed: 'The cost hold failed, so the batch job was not submitted. Contact an administrator to check billing or balance hold service.',
      groupDisabled: 'Batch image generation is not enabled for this key’s group. Choose another enabled key or contact an administrator.',
      pricingMissing: 'The selected model does not have batch image pricing configured. Contact an administrator to add pricing first.',
      insufficientBalance: 'Insufficient balance to hold the estimated batch image cost.',
      invalidModel: 'Select a batch image model available for the current key.',
      invalidItems: 'The prompt list is invalid. Check that it is not empty, within the item limit, and still using 1K image size.',
      duplicateCustomId: 'Custom IDs in the prompt list must be unique.',
      promptTooLong: 'One prompt is too long. Shorten it and try again.',
      invalidReferenceImage: 'A reference image is invalid. Use PNG, JPEG, or WebP under 10 MB.',
      tooManyReferenceImages: 'Too many reference images. Flash Image allows up to 3 per item, Pro Image allows up to 14, and each job allows up to 1000 total.',
      referenceImagesTooLarge: 'Reference images are too large. Inline reference images are limited to 128 MB per job; use gs:// file_uri or split the job for large batches.',
      tooManyOutputImages: 'Too many expected output images. Each prompt can request up to 4 images, and each job can generate up to 200 images.',
      idempotencyConflict: 'This submission conflicts with a previous request ID. Refresh the page and submit again.',
      notReady: 'The job is not complete yet. Download will be available after completion.',
      outputDeleted: 'The result files for this job have already been cleaned up.',
      resultMissing: 'The result file is unavailable. It may have been cleaned up, storage permissions may be broken, or storage settings may have changed. Contact an administrator to check the result file.',
      itemFailed: 'This item has no successful image to preview.',
      itemImageIndexOutOfRange: 'This item has no previewable image.',
      downloadLimited: 'Too many download requests are active. Please try again later.',
      downloadTooLarge: 'This ZIP is too large for a single download. Download fewer items at once or ask an administrator to raise the batch download limit.',
      deleteNotReady: 'Job records can only be deleted after the job finishes.',
      disabled: 'Batch image generation is currently disabled.',
      authRequired: 'The current API key is unavailable or expired. Select the key again.',
      adminReference: 'Send the error code and request ID to an administrator for troubleshooting.',
      errorReference: 'Error detail',
      errorCodeRef: 'code: {code}',
      requestIdRef: 'request ID: {id}',
      httpStatusRef: 'HTTP status: {status}',
    },
  },
}
