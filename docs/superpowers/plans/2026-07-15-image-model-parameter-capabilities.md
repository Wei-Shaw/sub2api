# Image Model Parameter Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Return image-parameter capabilities per model, validate them on the backend, and render only supported controls in the image-generation plugin.

**Architecture:** Extend the existing code-defined `image_model_capabilities` registry with typed enum and integer descriptors. The config endpoint is the frontend's sole capability source; generation normalizes and validates settings before history creation, then forwards them through the OpenAI-compatible or Gemini batch path. Unknown models keep conservative limits and expose no advanced settings.

**Tech Stack:** Go, `net/http`, Vue 3 Composition API, TypeScript, Vitest, Vue Test Utils, Vite

---

## File Map

- `backend/model_capabilities.go`: descriptors, registry, defaults, validation.
- `backend/generation_service.go`: request fields, persistence, retry, provider forwarding.
- `backend/handlers.go`: HTTP 400 mapping for invalid capability values.
- Backend `*_test.go`: config, validation, forwarding, history, and retry coverage.
- `frontend/src/types/index.ts`: shared capability and request types.
- `frontend/src/composables/useImageGeneration.ts`: derived options and synchronized setting state.
- `frontend/src/components/PromptComposer.vue`: dynamic model-specific controls.
- `frontend/src/App.vue`: state-to-component wiring.
- Frontend specs and `styles/app.css`: behavior and responsive wrapping.
- `plugins/image-generation/web`: Vite-generated deployable assets.

### Task 1: Expand the Backend Capability Contract

**Files:**
- Modify: `plugin-service/plugins/image-generation/backend/model_capabilities.go`
- Test: `plugin-service/plugins/image-generation/backend/model_capabilities_test.go`
- Test: `plugin-service/plugins/image-generation/backend/handlers_test.go`

- [ ] **Step 1: Write failing descriptor and config tests**

```go
func TestImageModelCapabilityIncludesParameterDescriptors(t *testing.T) {
	capability, ok := configuredImageModelCapability("gemini-2.5-flash-image")
	if !ok { t.Fatal("gemini capability is missing") }
	want := []string{"1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"}
	if !reflect.DeepEqual(capability.AspectRatios.Values, want) {
		t.Fatalf("ratios = %#v, want %#v", capability.AspectRatios.Values, want)
	}
	if capability.Resolutions.Default != "1K" {
		t.Fatalf("resolution default = %q", capability.Resolutions.Default)
	}
}

func TestUnknownImageModelHasNoAdvancedDescriptors(t *testing.T) {
	capability := imageModelCapability("custom-image-model")
	if capability.Quality != nil || capability.AspectRatios != nil || capability.Resolutions != nil {
		t.Fatalf("unknown model exposes advanced settings: %#v", capability)
	}
}
```

Extend `TestHandlerConfigIncludesImageModelCapabilities` to assert GPT `sizes`, `quality`, and `output_formats`, plus all ten Gemini ratios.

- [ ] **Step 2: Verify the tests fail**

Run from `plugin-service`:

```powershell
go test ./plugins/image-generation/backend -run 'Test(ImageModelCapabilityIncludesParameterDescriptors|UnknownImageModelHasNoAdvancedDescriptors|HandlerConfigIncludesImageModelCapabilities)' -count=1
```

Expected: FAIL because descriptors and `configuredImageModelCapability` do not exist.

- [ ] **Step 3: Add descriptor types and registry values**

```go
type EnumCapability struct {
	Values  []string `json:"values"`
	Default string   `json:"default"`
}

type IntegerCapability struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Default int `json:"default"`
}

type ImageModelCapability struct {
	MaxReferenceImages int                `json:"max_reference_images"`
	MaxOutputImages    int                `json:"max_output_images"`
	Sizes              *EnumCapability    `json:"sizes,omitempty"`
	AspectRatios       *EnumCapability    `json:"aspect_ratios,omitempty"`
	Resolutions        *EnumCapability    `json:"resolutions,omitempty"`
	Quality            *EnumCapability    `json:"quality,omitempty"`
	OutputFormats      *EnumCapability    `json:"output_formats,omitempty"`
	OutputCompression  *IntegerCapability `json:"output_compression,omitempty"`
	Background         *EnumCapability    `json:"background,omitempty"`
	InputFidelity      *EnumCapability    `json:"input_fidelity,omitempty"`
}

func configuredImageModelCapability(modelName string) (ImageModelCapability, bool) {
	capability, ok := imageModelCapabilities[strings.TrimSpace(modelName)]
	return capability, ok
}
```

Use these exact initial values:

- `gpt-image-2` and `gpt-image-1`: sizes `1024x1024`, `1536x1024`, `1024x1536`; quality `auto`, `low`, `medium`, `high`; formats `png`, `jpeg`, `webp`; compression `0..100` default `100`; background `auto`, `transparent`, `opaque`; input fidelity `low`, `high` default `high`.
- `gemini-2.5-flash-image`: ratios `1:1`, `2:3`, `3:2`, `3:4`, `4:3`, `4:5`, `5:4`, `9:16`, `16:9`, `21:9`; resolutions `1K`, `2K`, `4K` default `1K`.

Unknown names continue returning only one-reference/one-output limits.

- [ ] **Step 4: Run tests and commit**

Run `go test ./plugins/image-generation/backend -run 'Test(ImageModelCapability|UnknownImageModel|HandlerConfig)' -count=1`.

Expected: PASS.

```powershell
git add plugins/image-generation/backend/model_capabilities.go plugins/image-generation/backend/model_capabilities_test.go plugins/image-generation/backend/handlers_test.go
git commit -m "feat(image-generation): expose model parameter capabilities"
```

Run Git commands from `plugin-service`.

### Task 2: Normalize and Validate Generation Settings

**Files:**
- Modify: `plugin-service/plugins/image-generation/backend/model_capabilities.go`
- Modify: `plugin-service/plugins/image-generation/backend/generation_service.go`
- Modify: `plugin-service/plugins/image-generation/backend/handlers.go`
- Test: `plugin-service/plugins/image-generation/backend/model_capabilities_test.go`
- Test: `plugin-service/plugins/image-generation/backend/generation_service_test.go`

- [ ] **Step 1: Write failing validation tests**

```go
func TestNormalizeImageParameters(t *testing.T) {
	compression := 82
	req := GenerateRequest{Model: "gpt-image-1", OutputFormat: "webp", OutputCompression: &compression}
	if err := normalizeImageParameters(&req); err != nil { t.Fatal(err) }
	if req.Size != "1024x1024" || req.Quality != "auto" || req.Background != "auto" {
		t.Fatalf("normalized request = %#v", req)
	}
}

func TestNormalizeImageParametersRejectsUnsupportedValue(t *testing.T) {
	req := GenerateRequest{Model: "gpt-image-1", Quality: "ultra"}
	if err := normalizeImageParameters(&req); !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("error = %v", err)
	}
}

func TestNormalizeImageParametersRejectsUnknownModelAdvancedField(t *testing.T) {
	req := GenerateRequest{Model: "custom-image-model", Quality: "high"}
	if err := normalizeImageParameters(&req); !errors.Is(err, ErrInvalidImageParameter) {
		t.Fatalf("error = %v", err)
	}
}
```

Also test compression outside `0..100` and compression with `png`.
Assert validation errors include the model, parameter name, supplied value, and accepted values or range.

- [ ] **Step 2: Verify the tests fail**

Run `go test ./plugins/image-generation/backend -run TestNormalizeImageParameters -count=1`.

Expected: FAIL because fields and normalizer do not exist.

- [ ] **Step 3: Add request fields and centralized normalization**

```go
Quality           string `json:"quality,omitempty"`
OutputFormat      string `json:"output_format,omitempty"`
OutputCompression *int   `json:"output_compression,omitempty"`
Background        string `json:"background,omitempty"`
InputFidelity     string `json:"input_fidelity,omitempty"`
AspectRatio       string `json:"aspect_ratio,omitempty"`
Resolution        string `json:"resolution,omitempty"`
```

Add `ErrInvalidImageParameter`. Implement enum membership and integer-range helpers that trim values, apply advertised defaults, reject unsupported fields, and allow compression only with `jpeg` or `webp`. Call `normalizeImageParameters(&req)` after model selection and before provider-key resolution/history creation. Add this sentinel to the HTTP 400 branch in `Handler.Generate`.

- [ ] **Step 4: Run tests and commit**

Run `go test ./plugins/image-generation/backend -run 'Test(NormalizeImageParameters|ValidateOutputCount|ValidateReferenceImageCount)' -count=1`.

Expected: PASS.

```powershell
git add plugins/image-generation/backend/model_capabilities.go plugins/image-generation/backend/model_capabilities_test.go plugins/image-generation/backend/generation_service.go plugins/image-generation/backend/generation_service_test.go plugins/image-generation/backend/handlers.go
git commit -m "feat(image-generation): validate model-specific parameters"
```

### Task 3: Persist, Retry, and Forward Parameters

**Files:**
- Modify: `plugin-service/plugins/image-generation/backend/generation_service.go`
- Test: `plugin-service/plugins/image-generation/backend/generation_service_test.go`

- [ ] **Step 1: Write failing payload and retry tests**

```go
func TestNewGenerationRequestForwardsAdvancedParameters(t *testing.T) {
	compression := 82
	svc := NewGenerationService(nil, GenerationServiceOptions{})
	req, err := svc.newGenerationRequest(context.Background(), "https://provider.example", GenerateRequest{
		Model: "gpt-image-1", Prompt: "cat", Size: "1024x1024", ResponseFormat: "b64_json",
		OutputCount: 1, Quality: "high", OutputFormat: "webp", OutputCompression: &compression,
		Background: "transparent",
	})
	if err != nil { t.Fatal(err) }
	var payload map[string]any
	if err := json.NewDecoder(req.Body).Decode(&payload); err != nil { t.Fatal(err) }
	if payload["quality"] != "high" || payload["output_format"] != "webp" || payload["output_compression"] != float64(82) {
		t.Fatalf("payload = %#v", payload)
	}
}
```

Add equivalent tests for remote-edit JSON and local-image multipart, including selected `input_fidelity`. Add a Gemini batch assertion for `AspectRatio == "16:9"` and `ImageSize == "2K"`. Add history/retry tests proving every optional field survives reconstruction.

- [ ] **Step 2: Verify the tests fail**

Run `go test ./plugins/image-generation/backend -run 'Test(NewGenerationRequestForwardsAdvancedParameters|NewEditRequest|Batch.*Aspect|Retry.*Parameter)' -count=1`.

Expected: FAIL because fields are not forwarded or persisted.

- [ ] **Step 3: Implement forwarding and persistence**

```go
func addOptionalImageParameters(payload map[string]any, req GenerateRequest) {
	if req.Quality != "" { payload["quality"] = req.Quality }
	if req.OutputFormat != "" { payload["output_format"] = req.OutputFormat }
	if req.OutputCompression != nil { payload["output_compression"] = *req.OutputCompression }
	if req.Background != "" { payload["background"] = req.Background }
	if req.InputFidelity != "" { payload["input_fidelity"] = req.InputFidelity }
}
```

Use this helper for generation and remote-edit JSON. Add the same non-empty fields to multipart edits and remove the hard-coded `input_fidelity=high`. In `submitBatch`, use `req.AspectRatio` and `req.Resolution`; retain `batchDimensions(req.Size)` only as a compatibility fallback when explicit values are absent. Store fields in `requestPayload`, reconstruct them in `retry`, and include active settings in result metadata.

- [ ] **Step 4: Run backend tests and commit**

Run `go test ./plugins/image-generation/backend -count=1`.

Expected: PASS.

```powershell
git add plugins/image-generation/backend/generation_service.go plugins/image-generation/backend/generation_service_test.go
git commit -m "feat(image-generation): forward model image parameters"
```

### Task 4: Derive Frontend State from Capabilities

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/types/index.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`
- Test: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`

- [ ] **Step 1: Write failing composable tests**

```ts
it('derives options and defaults from the selected model', async () => {
  const api = createApi()
  const state = useImageGeneration({ api, loadKeys: async () => [key] })
  await state.initialize()
  expect(state.availableSizes.value).toEqual(['1024x1024', '1536x1024', '1024x1536'])
  expect(state.availableQualities.value).toEqual(['auto', 'low', 'medium', 'high'])
  expect(state.quality.value).toBe('auto')
})

it('clears advanced values for an unconfigured model', async () => {
  const api = createApi()
  const state = useImageGeneration({ api, loadKeys: async () => [key] })
  await state.initialize()
  state.quality.value = 'high'
  state.model.value = 'custom-image-model'
  await nextTick()
  expect(state.quality.value).toBe('')
})
```

Add a Gemini assertion for all ten ratio options, model-switch default replacement, and request omission for unsupported fields.
Add a config-load failure case proving advanced option arrays remain empty and no advanced fields are guessed.

- [ ] **Step 2: Verify the tests fail**

Run from `plugin-service/plugins/image-generation/frontend`:

```powershell
npm run test -- --run src/composables/useImageGeneration.spec.ts
```

Expected: FAIL because descriptor state does not exist.

- [ ] **Step 3: Implement typed synchronized state**

```ts
export interface EnumCapability { values: string[]; default: string }
export interface IntegerCapability { min: number; max: number; default: number }

function selectSupported(current: string, descriptor?: EnumCapability): string {
  if (!descriptor) return ''
  return descriptor.values.includes(current) ? current : descriptor.default
}
```

Expand `ImageModelCapability` with optional descriptors and `GenerateRequest` with optional request fields. Add refs/computed options for quality, output format, compression, background, input fidelity, aspect ratio, and resolution. Synchronize them immediately after config loading and whenever `model` changes. Serialize only non-empty fields with conditional spreads so stale hidden values cannot be sent.

- [ ] **Step 4: Run tests and commit**

Run `npm run test -- --run src/composables/useImageGeneration.spec.ts`.

Expected: PASS.

```powershell
git add src/types/index.ts src/composables/useImageGeneration.ts src/composables/useImageGeneration.spec.ts
git commit -m "feat(image-generation): derive settings from model capabilities"
```

### Task 5: Render Dynamic Parameter Controls

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/PromptComposer.vue`
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/App.vue`
- Modify: `plugin-service/plugins/image-generation/frontend/src/App.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/styles/app.css`
- Modify: `plugin-service/plugins/image-generation/frontend/src/styles/app.spec.ts`

- [ ] **Step 1: Write failing component tests**

```ts
expect(wrapper.findAll('[data-testid="image-aspect-ratio-select"] option')).toHaveLength(10)
await wrapper.get('[data-testid="image-aspect-ratio-select"]').setValue('21:9')
expect(wrapper.emitted('update:aspectRatio')).toEqual([['21:9']])
expect(wrapper.find('[data-testid="image-quality-select"]').exists()).toBe(false)
```

Add cases for GPT quality/format controls, compression only for `jpeg`/`webp`, input fidelity only with references, unknown-model hiding, and `App.vue` prop/event wiring.

- [ ] **Step 2: Verify the tests fail**

Run `npm run test -- --run src/components/components.spec.ts src/App.spec.ts src/styles/app.spec.ts`.

Expected: FAIL because controls and events do not exist.

- [ ] **Step 3: Implement dynamic controls and wrapping layout**

```vue
<label v-if="aspectRatioOptions.length" class="composer-select">
  <span class="sr-only">图片比例</span>
  <span class="composer-select-width" aria-hidden="true">{{ aspectRatio }}</span>
  <select :value="aspectRatio" data-testid="image-aspect-ratio-select"
    @change="emit('update:aspectRatio', ($event.target as HTMLSelectElement).value)">
    <option v-for="item in aspectRatioOptions" :key="item" :value="item">{{ item }}</option>
  </select>
</label>
```

Replace hard-coded size options with capability values. Add equivalent selectors for resolution, quality, output format, background, and input fidelity; use a range input with a percentage label for compression. Wire all props/events through `App.vue`. Keep the existing wrapping tool row, stable 40px control heights, bounded widths, and a non-shrinking send button. Add active values to the request-settings summary.

- [ ] **Step 4: Run tests and commit**

Run `npm run test -- --run src/components/components.spec.ts src/App.spec.ts src/styles/app.spec.ts`.

Expected: PASS.

```powershell
git add src/components/PromptComposer.vue src/components/components.spec.ts src/App.vue src/App.spec.ts src/styles/app.css src/styles/app.spec.ts
git commit -m "feat(image-generation): render model-specific image controls"
```

### Task 6: Full Verification and Generated Assets

**Files:**
- Regenerate: `plugin-service/plugins/image-generation/web/index.html`
- Regenerate: `plugin-service/plugins/image-generation/web/assets/app.js`
- Regenerate: `plugin-service/plugins/image-generation/web/assets/app.css`

- [ ] **Step 1: Format and test the backend**

From `plugin-service` run:

```powershell
gofmt -w plugins/image-generation/backend/model_capabilities.go plugins/image-generation/backend/model_capabilities_test.go plugins/image-generation/backend/handlers.go plugins/image-generation/backend/handlers_test.go plugins/image-generation/backend/generation_service.go plugins/image-generation/backend/generation_service_test.go
go test ./plugins/image-generation/backend -count=1
go test ./plugins/image-generation/... -count=1
```

Expected: all tests PASS.

- [ ] **Step 2: Test, type-check, and build the frontend**

From `plugin-service/plugins/image-generation/frontend` run:

```powershell
npm run test
npm run typecheck
npm run build
npm run verify:generated
```

Expected: Vitest and `vue-tsc` pass; reproducibility check prints `Generated frontend output is reproducible.`

- [ ] **Step 3: Inspect and commit generated assets**

```powershell
git diff --check
git diff --stat
git status --short
git add ../web
git commit -m "build(image-generation): publish parameter capability UI"
```

Expected: no whitespace errors and only planned source, test, and generated asset changes.

- [ ] **Step 4: Re-run focused verification after the final commit**

Run the backend package test, frontend test suite, and `git status --short` again. Expected: all tests PASS and the worktree is clean.
