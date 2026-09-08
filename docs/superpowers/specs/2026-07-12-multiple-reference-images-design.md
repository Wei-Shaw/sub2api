# Multiple Reference Images Design

## Goal

Allow image-generation users to upload, review, and submit multiple reference images while enforcing each model's supported reference-image limit.

## Model Capabilities

The backend is the source of truth for reference-image limits. Add a focused capability registry in `plugin-service/plugins/image-generation/backend/model_capabilities.go` with an `ImageModelCapability` value containing `max_reference_images`.

Known image models have explicit entries. Unknown models use a conservative limit of one reference image. The plugin config response exposes the capability map so the frontend and backend use the same limits. Future limit changes are made in this registry only.

The generation service validates `reference_images` against the selected model before persisting history or calling an upstream provider. An over-limit request returns HTTP 400 with a message that names the model limit.

## Frontend State And Data Flow

Each conversation continues to own a `referenceImages` array. Uploading files appends successfully uploaded references instead of replacing the current array. Request serialization sends every selected reference and removes the existing one-item truncation.

The frontend loads model capabilities from the plugin config endpoint during initialization. It derives the active limit from the selected model and treats unknown models as single-reference models.

Generated images and historical reference images selected through "Use as reference" append to the active list. Duplicate IDs are not appended twice.

## Interaction Design

The file input supports `multiple`, so users may select several files at once or add more files later. The reference control keeps the original single-tile footprint beside the prompt instead of expanding the composer in its resting state. Selected references appear as a compact stack inside that tile, with a centered add affordance that remains available while the model has capacity.

With one selected reference, the stack stays compact and preserves the original composer layout. With two or more references, activating the numeric count badge expands the references upward in a lightly rotated fan. The count badge is the only expand/collapse control and exposes `aria-expanded`; clicking an image does not expand the fan. The fan is an overlay and does not participate in document flow, so it does not push the prompt, composer controls, or conversation history. Activating the count badge again, pressing Escape, or moving focus outside the fan collapses it.

Every expanded thumbnail has its own accessible remove control. Removing references updates the stack immediately and collapses it when fewer than two references remain. The centered plus control is always a file input affordance in both collapsed and expanded states; it never toggles the fan. This keeps adding references available without requiring a second click or a separate toolbar action.

The fan uses short transform-and-opacity transitions. Mobile layouts use smaller thumbnails and a tighter fan radius. Under `prefers-reduced-motion: reduce`, references switch states without animation.

The UI displays the current count and model limit when references are selected. If a model change makes the current selection exceed the new limit, all references remain selected. The composer shows a clear validation message and disables submission until enough references are removed or a compatible model is selected.

If a new selection would exceed the current limit, files within the remaining capacity are uploaded and the rest are rejected with a clear message. Existing selected references are never removed automatically.

## Error Handling

Uploads are processed independently. A failed upload reports its filename and does not discard successful uploads or existing references. Submission remains unavailable when the active selection exceeds the model limit.

The backend repeats the limit validation to cover stale clients and direct API requests. Existing validation for file size, content type, storage ownership, and reference availability remains unchanged.

## Compatibility

The request field remains `reference_images`, so persisted history and the batch client need no schema migration. Existing records with one reference continue to render. Unknown models retain the current one-image behavior.

The config endpoint gains an additive `image_model_capabilities` field. Clients that do not read it continue to work.

## Testing

Backend tests cover known and unknown model limits, config serialization, accepted requests at the limit, and rejected over-limit requests before upstream work begins.

Frontend tests cover multi-file emission, appended uploads, individual removal, duplicate prevention, complete request serialization, per-model limits, model-switch overflow behavior, partial acceptance at remaining capacity, and disabled submission with an explanatory message.

The final verification runs Go tests for the image-generation backend, frontend unit tests, type checking, the production frontend build, and generated-asset verification.
