import { describe, expect, it } from "vitest";

import {
  imagePricingPlatforms,
  supportsImagePricingPlatform,
} from "../groupsImagePricing";

describe("groups image pricing platform support", () => {
  it("includes Grok media groups", () => {
    expect(supportsImagePricingPlatform("grok")).toBe(true);
    expect(imagePricingPlatforms.has("grok")).toBe(true);
  });

  it("keeps non-media group platforms out of the image pricing controls", () => {
    expect(supportsImagePricingPlatform("anthropic")).toBe(false);
  });
});
