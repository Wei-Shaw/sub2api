import { mount } from "@vue/test-utils";
import { defineComponent, ref } from "vue";
import { describe, expect, it } from "vitest";

import KiroCacheRatioField from "../KiroCacheRatioField.vue";

const mountField = (modelValue: number) =>
  mount(KiroCacheRatioField, {
    props: {
      idPrefix: "test-ratio",
      modelValue,
      label: "Cache ratio",
      hint: "Choose a percentage",
    },
  });

describe("KiroCacheRatioField", () => {
  it.each([
    [0, "0"],
    [0.5, "50"],
    [1, "100"],
  ])("displays ratio %s as %s%%", (ratio, percentage) => {
    const wrapper = mountField(ratio as number);

    expect(wrapper.get('[data-testid="ratio-input"]').element).toHaveProperty(
      "value",
      percentage,
    );
    expect(wrapper.get('[data-testid="ratio-slider"]').element).toHaveProperty(
      "value",
      percentage,
    );
  });

  it("keeps the slider, number input, and ratio model in sync", async () => {
    const Host = defineComponent({
      components: { KiroCacheRatioField },
      setup() {
        return { ratio: ref(0.25) };
      },
      template: `
        <KiroCacheRatioField
          id-prefix="host-ratio"
          v-model="ratio"
          label="Cache ratio"
        />
      `,
    });
    const wrapper = mount(Host);
    const slider = wrapper.get('[data-testid="ratio-slider"]');
    const input = wrapper.get('[data-testid="ratio-input"]');

    await slider.setValue("60");
    expect(input.element).toHaveProperty("value", "60");
    expect((wrapper.vm as unknown as { ratio: number }).ratio).toBe(0.6);

    await input.setValue("35");
    expect(slider.element).toHaveProperty("value", "35");
    expect((wrapper.vm as unknown as { ratio: number }).ratio).toBe(0.35);
  });

  it("clamps manual input to the supported range", async () => {
    const wrapper = mountField(0.5);
    const input = wrapper.get('[data-testid="ratio-input"]');

    await input.setValue("150");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([1]);
    expect(input.element).toHaveProperty("value", "100");

    await input.setValue("-20");
    expect(wrapper.emitted("update:modelValue")?.at(-1)).toEqual([0]);
    expect(input.element).toHaveProperty("value", "0");
  });

  it("restores empty input and renders invalid model values without NaN", async () => {
    const wrapper = mountField(0.5);
    const input = wrapper.get('[data-testid="ratio-input"]');

    await input.setValue("");
    expect(wrapper.emitted("update:modelValue")).toBeUndefined();
    await input.trigger("blur");
    expect(input.element).toHaveProperty("value", "50");

    await wrapper.setProps({ modelValue: Number.NaN });
    expect(input.element).toHaveProperty("value", "0");
    expect(wrapper.get('[data-testid="ratio-slider"]').element).toHaveProperty(
      "value",
      "0",
    );
  });
});
