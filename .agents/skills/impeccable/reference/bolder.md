When asked for "bolder," AI defaults to the same tired tricks: cyan/purple gradients, glassmorphism, neon accents on dark backgrounds, gradient text on metrics. These are the opposite of bold. Reject them first, then increase visual impact by making the existing design language more decisive, specific, and committed.

---

## Register

Brand: "bolder" means distinctive. Express a stronger point of view through hierarchy, pacing, proportion, copy, evidence, and one committed visual idea.

Product: "bolder" rarely means theatrics; those undermine trust. It means stronger hierarchy, clearer weight contrast, sharper information density, and more decisive prioritization. The amplification is in clarity, not drama.

---

## Assess Current State

Analyze what makes the design feel too safe or boring:

1. **Identify weakness sources**:
   - **Generic choices**: The page could belong to any product in the category.
   - **Timid scale**: Everything is medium-sized with no clear lead.
   - **Low contrast**: Important and supporting elements have similar visual weight.
   - **Static**: The surface has no meaningful moment of emphasis.
   - **Predictable**: The composition follows a default pattern without a point of view.
   - **Flat hierarchy**: Nothing stands out or commands attention.

2. **Understand the context**:
   - What is the brand personality?
   - What is the purpose of this surface?
   - Who is the audience?
   - What design system, tokens, components, and visual conventions already exist?

If any of these are unclear from the codebase, STOP and use Codex's structured user-input/question tool when available; if unavailable, ask directly in chat to clarify what you cannot infer.

**CRITICAL**: "Bolder" does not mean chaotic or garish. It means distinctive, memorable, and confident. Think intentional drama, not random noise.

**WARNING - AI SLOP TRAP**: Review ALL the DON'T guidelines from the parent impeccable skill (already loaded in this context) before proceeding. Bold means distinctive, not "more effects."

## Design-System Lock

If the project has `DESIGN.md`, tokens, theme variables, or established component styles, treat that system as the boundary. Make the existing language stronger before adding new language.

Do not invent new colors, gradients, radii, shadows, fonts, decorative backgrounds, or effects just because the request says "bolder." A bolder pass should usually change emphasis, proportion, rhythm, density, contrast, copy, artifact specificity, and layout relationships while staying inside the documented system.

If the existing system is genuinely too limited to express the bolder direction, stop and ask the user before expanding it. Name the exact additions, the role each would play, and why the current system cannot do the job. If the user approves expansion, update the design system or tokens alongside the implementation.

## Plan Amplification

Create a strategy to increase impact while maintaining coherence:

- **Focal point**: Pick one thing the viewer should remember, then make the rest support it.
- **System levers**: Identify which existing tokens, components, layout patterns, and copy structures can carry more weight.
- **Risk budget**: Decide how far the surface can push while still feeling like the same product or brand.
- **Hierarchy amplification**: Increase contrast between primary, secondary, and tertiary content instead of making every element louder.

**IMPORTANT**: Bold design must still be usable. Impact without function is just decoration.

## Amplify the Design

Systematically increase impact through intention, not a menu of effects:

### Typography Amplification
- Strengthen the existing type hierarchy before changing typefaces.
- Make important text meaningfully more dominant, and make supporting text quieter.
- Use weight, measure, spacing, and line breaks to sharpen the point of view.
- Add or replace fonts only after user-approved design-system expansion.

### Color Amplification
- Use the existing palette more decisively before adding colors.
- Shift the proportion, placement, and contrast of documented colors to clarify meaning.
- Treat any new color, gradient, or tint ramp as a design-system expansion that requires user approval.
- Keep color tied to hierarchy, state, or brand meaning; do not use it as surface decoration.

### Spatial Amplification
- Change proportion, density, alignment, and sequencing so the composition has a stronger point of view.
- Create clearer contrast between dense evidence and open breathing room.
- Let layout express priority and narrative order before adding ornament.
- Preserve responsive behavior and avoid text overflow at every breakpoint.

### Surface Amplification
- Use existing surface, border, radius, and shadow rules more deliberately.
- Remove timid half-measures: either give an element a clear role or simplify it.
- Add texture, depth, illustration, or decorative treatments only when already established by the system or explicitly approved.
- Make real product artifacts, imagery, data, or copy carry attention before reaching for effects.

### Motion & Animation
- Design one meaningful moment of emphasis when motion genuinely supports the point.
- Make interaction feedback feel more decisive without becoming distracting.
- Keep transitions smooth and intentional.
- **Bolder != scroll-fade-rise on every section.** That's the saturated AI default, the opposite of bold.

### Composition Boldness
- Make the dominant idea unmistakable.
- Use layout tension, sequencing, contrast, and restraint to create a stronger read.
- Let the page's structure communicate priority before adding decorative layers.
- If every element is louder, the composition is not bolder; it is flatter.

**NEVER**:
- Add undocumented design-system primitives without user approval
- Add effects randomly without purpose
- Hide weak hierarchy behind decoration
- Sacrifice readability for aesthetics
- Make everything bold; contrast is the point
- Ignore accessibility
- Overwhelm with motion
- Copy trendy aesthetics blindly

## Verify Quality

Ensure amplification maintains usability and coherence:

- **System-faithful**: Did the pass make the existing design language stronger before adding anything new?
- **No undocumented drift**: Are new colors, gradients, shadows, radii, fonts, and effects either absent or explicitly approved and documented?
- **NOT AI slop**: Does this look like every other AI-generated "bold" design? If yes, start over.
- **Still functional**: Can users accomplish tasks without distraction?
- **Coherent**: Does everything feel intentional and unified?
- **Memorable**: Will users remember this experience for the intended reason?
- **Performant and accessible**: Does the result stay fast, readable, responsive, and WCAG-conscious?

**The test**: If you showed this to someone and said "AI made this bolder," would they believe you immediately? If yes, you've failed. Bold means distinctive, not "more AI effects."

When the result feels right, hand off to `$impeccable polish` for the final pass.
