# Image Model Parameter Capabilities Design

## Goal

Make the image-generation plugin obtain model-specific image parameters from its
config API. The frontend must render only the controls supported by the selected
model, and the backend must validate and forward only supported values.

This extends the existing `image_model_capabilities` response instead of adding a
second model metadata endpoint.

## Capability Contract

`GET /plugins/image-generation/api/config` continues to return
`image_model_capabilities`, keyed by the exact model name. Each optional parameter
uses a typed descriptor that includes its accepted values and default. Numeric
parameters use range descriptors.

Example:

```json
{
  "image_model_capabilities": {
    "gpt-image-2": {
      "max_reference_images": 16,
      "max_output_images": 10,
      "sizes": {
        "values": ["1024x1024", "1536x1024", "1024x1536"],
        "default": "1024x1024"
      },
      "aspect_ratios": {
        "values": ["1:1", "3:2", "2:3"],
        "default": "1:1"
      },
      "quality": {
        "values": ["auto", "low", "medium", "high"],
        "default": "auto"
      },
      "output_formats": {
        "values": ["png", "jpeg", "webp"],
        "default": "png"
      },
      "output_compression": {
        "min": 0,
        "max": 100,
        "default": 100
      },
      "background": {
        "values": ["auto", "transparent", "opaque"],
        "default": "auto"
      },
      "input_fidelity": {
        "values": ["low", "high"],
        "default": "high"
      }
    },
    "gemini-2.5-flash-image": {
      "max_reference_images": 10,
      "max_output_images": 4,
      "aspect_ratios": {
        "values": ["1:1", "2:3", "3:2", "3:4", "4:3", "4:5", "5:4", "9:16", "16:9", "21:9"],
        "default": "1:1"
      },
      "resolutions": {
        "values": ["1K", "2K", "4K"],
        "default": "1K"
      }
    }
  }
}
```

The descriptors are optional. Absence means the model does not expose that
parameter. The exact values in the registry are the source of truth and must be
covered by backend contract tests.

`sizes`, `aspect_ratios`, and `resolutions` remain separate because upstream APIs
give them different meanings:

- `sizes` contains pixel dimensions such as `1024x1024`.
- `aspect_ratios` contains shape ratios such as `16:9`.
- `resolutions` contains output tiers such as `1K`, `2K`, or `4K`.

## Request Contract

The plugin generation request gains these optional fields:

- `quality`
- `output_format`
- `output_compression`
- `background`
- `input_fidelity`
- `aspect_ratio`
- `resolution`

The existing `size` field remains for compatibility. Optional fields are omitted
when the selected model does not advertise the corresponding capability.

The backend validates every supplied optional field against the selected model's
descriptor before creating history or calling the upstream service. Unsupported
fields and values return HTTP 400. `output_compression` must be within its
advertised range and is accepted only when the selected output format supports
compression.

Generation and edit request builders forward validated fields using each
upstream's expected JSON or multipart field names. Retry reconstructs the same
validated settings from history so a retry is behaviorally equivalent to the
original request.

## Unknown Models

Models absent from `image_model_capabilities` use the current conservative
fallback:

- one reference image;
- one output image;
- the existing base `size` value of `1024x1024`;
- no advanced parameter controls;
- no optional advanced fields forwarded upstream.

The backend rejects advanced fields supplied manually for unknown models. This
keeps frontend hiding and backend enforcement consistent.

## Frontend Behavior

The composable stores the capability map returned by `/config` and derives the
active capability from the selected model. The composer renders controls only for
descriptors present on that capability.

On initialization and model changes:

1. Keep a selected value when it is allowed by the new descriptor.
2. Otherwise replace it with the descriptor default.
3. Clear optional state when the descriptor is absent.
4. Continue clamping output count and reference count with the existing limits.

The size selector is populated from `sizes.values` instead of three hard-coded
options. Aspect ratio and resolution receive their own selectors when advertised.
Compression is shown only when the capability advertises it and the selected
format is compressible. Input fidelity is shown only when reference images are
present because it affects image edits rather than text-only generation.

The submitted request and the message's request-settings summary contain only
visible, active settings. A model switch cannot leave stale hidden values in the
request.

## Error Handling

Capability validation errors use a dedicated sentinel error and identify the
model, parameter, supplied value, and accepted values or range. The generation
handler maps them to HTTP 400. Upstream errors retain the existing passthrough
behavior.

If `/config` cannot be loaded, initialization continues with the conservative
unknown-model behavior and reports the existing initialization error path. No
advanced controls are guessed client-side.

## Testing

Backend tests cover:

- the complete config JSON shape for representative GPT Image and Gemini models;
- unknown-model fallback behavior;
- accepted and rejected enum/range values;
- rejection of advanced fields for unknown models;
- generation, edit multipart, history persistence, and retry forwarding.

Frontend tests cover:

- controls and options derived from the selected model capability;
- more than the current three aspect-ratio options;
- model-switch defaulting and stale-value removal;
- hidden advanced controls for unknown models;
- request serialization of visible settings only;
- conditional compression and input-fidelity controls.

Existing output-count, reference-image, history, and API client tests remain in
place and are updated for the expanded capability type.

## Scope

The capability registry remains code-defined in the image-generation plugin for
this change. Admin editing, database persistence, and automatic upstream model
introspection are out of scope. Adding a model or correcting provider support is a
backend registry change exposed automatically through the existing config API.
