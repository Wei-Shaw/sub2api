# Multi-Image Output Design

## Goal

Allow one prompt to generate multiple images while enforcing a model-specific output limit in both the frontend and backend.

## Capability Model

Extend `ImageModelCapability` with `max_output_images`. The initial registry is:

| Model | Maximum reference images | Maximum output images |
| --- | ---: | ---: |
| `gpt-image-2` | 16 | 10 |
| `gpt-image-1` | 16 | 10 |
| `gemini-2.5-flash-image` | 10 | 4 |
| Unknown model | 1 | 1 |

The `/config` response remains the only frontend source of these limits. Unknown models use the backend defaults.

## Request And Validation

Add `output_count` to the public generation request. Values must be between 1 and the selected model's `max_output_images`. Missing or zero values retain backward compatibility by resolving to 1. Invalid values return a client error before any provider request is sent.

OpenAI-compatible generation and edit requests map `output_count` to `n`. Gemini batch submissions map it to the existing `output_count` item field. The request stored in history includes the resolved count.

## Frontend Behavior

Add a quantity select beside model and size. Its options range from 1 through the current model limit. The initial value is 1. When a user switches to a model with a lower limit, the selected quantity is reduced to that limit.

Submitting a prompt sends the selected quantity as `output_count`. The optimistic user message and restored history display the real requested count. Existing result rendering already supports multiple images and continues to use its responsive grid.

## Errors And Compatibility

The backend is authoritative. The frontend limits ordinary input but does not replace server validation. Existing clients that omit `output_count` continue generating one image. Unknown models are restricted to one output until explicitly added to the capability registry.

## Verification

- Backend unit tests cover capability lookup, defaulting, over-limit rejection, OpenAI `n`, edit `n`, and Gemini batch `output_count`.
- Frontend tests cover configuration parsing, quantity options, model-switch clamping, request serialization, and history labels.
- Browser smoke testing covers selecting multiple outputs and confirms the control fits desktop and mobile composer layouts.

