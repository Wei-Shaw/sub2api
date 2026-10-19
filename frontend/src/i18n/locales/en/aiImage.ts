export default {
  aiImage: {
    title: 'AI Image',
    subtitle: 'Paste a key created on this platform, then enter a prompt to generate or edit images.',

    modules: {
      title: 'Modules',
      hint: 'Click to expand, click again to collapse. Only one opens at a time.',
      primary: 'Main',
      primaryHint: 'Drop in one image and generate — just write the prompt below.',
      secondary: 'Secondary',
    },

    /**
     * Single image.
     * The owner asked for it to be a toggled page too, no longer an always-on
     * form, because "no two feature pages may be open at the same time,
     * single-image generation included".
     */
    single: {
      title: 'Single image',
      hint: 'Write a prompt; a reference image is optional.',
    },

    key: {
      sectionTitle: 'Image key',
      label: 'Platform key',
      placeholder: 'Paste a key created on this platform (starts with sk- / cns-)',
      hint: 'Keys come from this platform\u2019s "API Keys" page and are stored only in your browser.',
      save: 'Save key',
      configure: 'Enter key',
      change: 'Change key',
      saved: 'Saved',
      clear: 'Clear key',
      checking: 'Validating key\u2026',
      valid: 'Key is usable: {group}',
      validNoGroup: 'Key is usable',
      invalid: 'The key is invalid or disabled. Please check and retry.',
      notImageModel: 'This key is not an image-generation model',
      notImageModelHint:
        'The key\u2019s group ({group}) does not have image generation enabled, so it cannot be used for AI image generation. Pick a key from an image-enabled group on the "API Keys" page, or ask an administrator to enable it for the group.',
      notOwned:
        'This key does not belong to the current account, so it cannot be validated here. Use a key created by this account.',
      empty: 'Please enter a key first.',
      loadFailed: 'Failed to load your key list. Please retry later.',
    },

    /**
     * Vision-model key.
     *
     * The platform isolates models per group: an image group usually only binds
     * image channels, so it has no vision model at all. Calling a vision model
     * with an image key gets rejected at the gateway's routing stage (503) and
     * never reaches the upstream. So selling-point extraction and hot-style
     * breakdown each need a key from a group that can reach a vision model.
     */
    visionKey: {
      sectionTitle: 'Vision model key',
      // Must differ from the image key's "Save key": two identical save buttons
      // on one page leave the user unsure which one they clicked.
      save: 'Save vision key',
      placeholder: 'Paste a key that can call a vision model (Gemini, …)',
      // Owner asked for this single line only; all extra explanation removed.
      hint: 'Use a key whose group includes a vision-model channel such as Gemini.',
      valid: 'Vision key works: {group}',
      validNoGroup: 'Vision key works',
      usingImageKey: 'Empty — using the image key',
      missing: 'Empty',
      invalidTitle: 'Vision key is invalid',
      invalid: 'This key is invalid or disabled. Please check and retry.',
      modelLabel: 'Vision model',
      // Hint the owner explicitly asked for: pick a model the Gemini channel serves.
      modelHint:
        'Pick a model the Gemini channel serves (gemini-3.5-flash / gemini-3.8-flash / gemini-3.1-pro-preview, …). Vision and image models live in different groups — the right model with a key from the wrong group still cannot be routed.',
      needed: 'Please fill in the "Vision model key" first — selling-point extraction and hot-style breakdown need it.',
      failedRouting:
        'Vision call failed: the group this key belongs to cannot reach a vision model (the request was rejected at the platform routing stage and never reached the upstream). Switch to a key whose group includes a vision model such as Gemini — changing the model or retrying later will not help.',
    },

    prompt: {
      label: 'Prompt',
      placeholder:
        'Describe what to generate or change, e.g. put this product on a white background, 3:4 portrait, add Russian selling points',
      generate: 'Generate',
      generating: 'Generating\u2026',
      clear: 'Clear',
    },

    template: {
      title: 'Prompt templates',
      label: 'Prompt templates',
      placeholder: 'Pick a saved template\u2026',
      promptLabel: 'Prompt (used both for saving and for applying)',
      generateHint: 'To generate, open Single image and hit generate — the prompt is already there.',
      save: 'Save as template',
      delete: 'Delete template',
      namePlaceholder: 'Name this template',
      confirm: 'Save',
      cancel: 'Cancel',
      applied: 'Applied template "{name}"',
      saved: 'Template "{name}" saved',
      overwritten: 'Template "{name}" updated',
      deleted: 'Template "{name}" deleted',
      saveFailed: 'The template could not be saved: browser storage is unavailable or full.',
      needPrompt: 'Write a prompt first, then save it as a template.',
      needName: 'Please give the template a name.',
    },

    hot: {
      title: 'Bestseller clone',
      hint:
        'Upload a few competitor bestseller images; the AI extracts the layout (9 traits: layout, palette, copy placement\u2026) and applies it to your own product.',
      pick: 'Add bestseller images',
      clear: 'Clear',
      picked: 'Added {count} bestseller image(s).',
      analyzing: 'Analyzing\u2026',
      analyze: 'Extract bestseller layout',
      progress: '{done}/{total} analyzed',
      styleTitle: 'Extracted layout (shared traits across images)',
      analyzed: 'Analyzed {count} bestseller image(s); shared traits merged.',
      extras: 'Also generate 4 more (multi-angle side view / macro detail / usage scene / FAQ compare)',
      buildScript: 'Build script from this layout',
      scriptBuilt: 'Built {count} cards from the bestseller layout \u2014 generate them below.',
      needRef: 'Add at least one bestseller image first.',
      needProduct: 'Fill in your product name first (the "Product name" field right below this step).',
      needTemplate:
        'No layout template available yet \u2014 either upload bestseller images above and click "Extract bestseller layout", or pick a saved template under "Prompt templates".',
      templateReady: 'Ready: the template for this clone is set. Fill in the product name and you can generate.',
      noResult: 'None of these images produced a result. Try different images, or check the vision model.',
      oneFailed: 'One image failed and was skipped (the rest continued): {reason}',
      step2: 'Step 2: add your own product photo and fill in the details (independent of the "Image set" panel)',
      // Your own product photo: cloning means "apply the reference layout to MY product".
      pickProductImage: 'Pick my product photo',
      replaceProductImage: 'Replace my product photo',
      noProductImage: 'No product photo yet (it becomes the product in the generated images)',
      // Selling points come from the vision model; pain/differentiation come from comparing this photo.
      extract: 'AI-detect selling points / pain / differentiation',
      extracting: 'Detecting\u2026',
      extractHint:
        'With a product photo in place, hit detect: the vision model writes the selling points, and pain/differentiation come from comparing this item against ordinary ones in the same category. You can still edit everything afterwards.',
      extracted: 'Selling points, pain and differentiation detected and filled in (editable).',
      extractFailed: 'Detection failed',
      needProductImage: 'Pick your own product photo first \u2014 detection needs it.',
    },

    composite: {
      title: 'One product, many views',
      pick: 'Pick several product photos',
      working: 'Combining\u2026',
      hint: 'Pick 2+ images to lay out side by side as one reference, so the model sees every angle at once.',
      done: 'Combined {count} images into one reference.',
      tooMany: 'At most {max} images; the rest were ignored (too wide gets rejected by the image API).',
      failed: 'Combine failed: {reason}',
      // The combined image is the reference for Single image; say where to go next.
      nextStep: 'Once combined, open Single image — it is already in the reference slot, just hit generate.',
    },

    watermark: {
      title: 'Watermark',
      enable: 'Add watermark when downloading',
      text: 'Text',
      alpha: 'Strength',
      fontSize: 'Font size',
      auto: '(0 = auto)',
      download: 'Download with watermark',
      done: 'Watermarked and downloading.',
      corsBlocked:
        'The image server does not allow this page to read the file, so it cannot be watermarked in the browser. The original was opened in a new tab \u2014 right-click it and choose "Save image as". To restore "Download with watermark", add a CORS rule for img.chenshuapi.com.',
      hint: 'The watermark is stamped only on download and never alters a generated image. Use the "Download with watermark" button under an image.',
      note:
        'Two deliberate differences from the desktop app: the watermark is a straight staggered grid (the desktop settings window says "diagonal", but its code never rotates), and references always use the clean pre-watermark image so the result carries only one watermark instead of two.',
    },

    filter: {
      label: 'Status',
      all: 'All',
      completed: 'Completed',
      generating: 'Generating',
      failed: 'Failed',
      empty: 'No records under the current filter',
    },

    reference: {
      label: 'Reference image',
      pick: 'Choose reference',
      replace: 'Replace reference',
      clear: 'Remove reference',
      hint: 'With a reference image the request uses image editing; without one it is text-to-image.',
      none: 'None (text-to-image)',
    },

    batch: {
      concurrency: 'Concurrency',
      generateAll: 'Generate all ({count})',
      running: 'Batch generating\u2026',
      progress: 'Done {done} / {total}',
      failedCount: '{count} failed',
      paused: 'Paused',
      pause: 'Pause',
      resume: 'Resume',
      stop: 'Stop',
      stopHint: 'Stopping submits no new jobs; the one in flight runs to completion.',
      needScript:
        'No script yet \u2014 build one in "Bestseller clone" or "Image set" to enable generate-all, pause and stop.',
      fromHot: 'These cards came from "Bestseller clone" (the extracted bestseller layout was applied).',
      fromHotSaved:
        'These cards came from "Bestseller clone" (the template selected under "Prompt templates" was applied).',
      fromOzon: 'These cards came from the "Image set" panel.',
    },

    options: {
      model: 'Model',
      size: 'Size',
      count: 'Count',
      countHint: 'Up to {max} images',
    },

    result: {
      sectionTitle: 'Results',
      currentRound: 'This round',
      history: 'History',
      empty: 'Generated images will appear here',
      emptyHint: 'Use the status filter on the left to narrow the history.',
      // Title of the per-feature result area: only this feature's images.
      thisPanel: 'This feature’s results',
      panelEmpty: 'This feature has not generated any images yet',
      failedReason: 'Failure reason',
      imageNotRetained:
        'This image only lived in the current session (the API returned base64), so it must be regenerated after a refresh.',
      retry: 'Retry',
      download: 'Download',
      // Fallback copy when the image host blocks cross-origin reads.
      downloadBlocked:
        'The image server does not allow this page to read the file, so the original was opened in a new tab. Right-click the image there and choose "Save image as". To restore normal downloads, add a CORS rule for img.chenshuapi.com.',
      remove: 'Delete',
      copyPrompt: 'Copy prompt',
      copied: 'Copied',
      promptLabel: 'Prompt',
      modelLabel: 'Model',
      sizeLabel: 'Size',
      timeLabel: 'Time',
      openInNewTab: 'Open in new tab',
      downloadAll: 'Download all',
    },

    product: {
      label: 'Product ID',
      placeholder: 'Your own label, e.g. the Ozon article number',
      hint: 'Just a label for your own reference so you can tell which images belong to which product. The client stores images under its own internal product ID, which you do not need to fill in — what you actually hand to the client is the image URL below.',
      copied: 'Image URL copied — paste it into the client to list the item.',
      copyFailed: 'Copy failed, please select and copy manually.',
      copyUrl: 'Copy image URL',
      copyAll: 'Copy all URLs ({count})',
      copiedAll:
        'Copied {count} image URLs in order — pasting them into the client stores them as images 1, 2, 3\u2026',
      urlLabel: 'Image URL',
      noUrl: 'This response returned base64, so there is no URL the client can fetch.',
    },

    lock: {
      label: 'Lock product',
      hint: 'When on, only background, lighting, layout and text change — the product itself stays identical, so the image matches the item shown in the client.',
      on: 'Locked',
      off: 'Not locked',
      needsReference: 'Product lock only takes effect once a reference image is uploaded.',
    },

    script: {
      title: 'Image set',
      toggle: 'Expand / collapse',
      product: 'Product name',
      productPlaceholder: 'e.g. 316 stainless steel thermos',
      sp1: 'Selling point 1',
      sp2: 'Selling point 2',
      sp3: 'Selling point 3',
      pain: 'Customer pain',
      diff: 'Differentiator',
      scene: 'Auto-matched scene',
      scenePlaceholder: 'Filled automatically from the product name',
      build: 'Build the 10 prompts',
      built: 'Built {count} prompts. Click any one to expand and edit it.',
      applyAll: 'Load all (newline separated)',
      fillOne: 'Load into prompt',
      // Script card editing (owner: "generate what I edited, not the original template").
      // Title of the script list inside each feature panel (above "this feature's results").
      cardsTitle: 'Scripts from this run',
      editedTag: 'Edited',
      saveEdit: 'Save changes',
      generateThis: 'Generate just this one',
      restoreCard: 'Restore this one',
      resetEdits: 'Restore all',
      editHint:
        'Hit save when done. "Generate all" uses this text, not the original template. The model receives English; this Chinese text is sent together with the English hard rules.',
      editSaved: 'Saved: {title}',
      restored: 'Restored: {title}',
      editsReset: 'All prompts restored to the template text.',
      aiExtract: 'AI extract selling points',
      aiExtracting: 'Extracting…',
      aiExtracted: 'AI filled the fields automatically — please review and adjust.',
      printedRu: 'Russian copy that will be printed on the image:',
      printedRuNone:
        'No Russian selling points yet: AI extraction creates them; if you type Chinese by hand, the image will show Chinese.',
      aiNeedsImage: 'Upload a product image before asking the AI to extract selling points.',
      aiFailed: 'AI extraction failed',
      clear: 'Clear',
    },

    errors: {
      missingKey: 'Please enter and save an image key first.',
      missingPrompt: 'Please enter a prompt.',
      badKey: 'The key is invalid or disabled.',
      noImagePermission: 'This key is not an image-generation model: its group has image generation disabled.',
      rateLimited: 'Too many requests. Please retry later.',
      insufficient: 'Insufficient balance or quota. Please top up first.',
      timeout: 'Generation timed out. Please retry later.',
      network: 'Network error. Please check your connection and retry.',
      unknown: 'Generation failed. Please retry later.',
    },
  },
}
