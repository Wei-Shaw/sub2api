import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import ReasoningEffortPolicyFields from "../ReasoningEffortPolicyFields.vue";
import { createReasoningEffortMappingRow } from "@/views/admin/groupsReasoningEffort";

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  };
});

describe("ReasoningEffortPolicyFields", () => {
  it("renders model scope fields for each mapping", () => {
    const mapping = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
      model: "gpt",
    });
    const wrapper = mount(ReasoningEffortPolicyFields, {
      props: {
        idPrefix: "create-group-reasoning",
        platform: "openai",
        maxEffort: "",
        overLimit: "downgrade",
        mappings: [mapping],
      },
      global: {
        stubs: {
          Icon: { props: ["name"], template: '<i :data-icon="name" />' },
          Select: {
            props: ["id", "modelValue", "placeholder"],
            template:
              '<input :id="id" :value="modelValue ?? \'\'" :placeholder="placeholder" />',
          },
        },
      },
    });

    expect(wrapper.text()).toContain("admin.groups.form.reasoningEffortMatchType");
    expect(wrapper.text()).toContain("admin.groups.form.reasoningEffortModel");
    expect(wrapper.text()).toContain("admin.groups.form.reasoningEffortFrom");
    expect(wrapper.text()).toContain("admin.groups.form.reasoningEffortTo");
    expect(wrapper.text()).toContain("admin.groups.form.addReasoningEffortPair");

    const modelInput = wrapper.get(
      `#create-group-reasoning-${mapping.id}-model`,
    );
    expect(modelInput.element).toMatchObject({
      value: "gpt",
      placeholder: "admin.groups.form.reasoningEffortModelPlaceholder",
    });
  });

  it("emits a new mapping group with one empty pair", async () => {
    const wrapper = mount(ReasoningEffortPolicyFields, {
      props: {
        idPrefix: "edit-group-reasoning",
        platform: "openai",
        maxEffort: "",
        overLimit: "downgrade",
        mappings: [],
      },
      global: {
        stubs: {
          Icon: { props: ["name"], template: '<i :data-icon="name" />' },
          Select: { template: "<div />" },
        },
      },
    });

    await wrapper.get("button").trigger("click");

    const emitted = wrapper.emitted("update:mappings");
    expect(emitted).toHaveLength(1);
    expect(emitted?.[0]?.[0]).toEqual([
      expect.objectContaining({
        match_type: "",
        model: "",
        pairs: [
          expect.objectContaining({
            from: "",
            to: "",
          }),
        ],
      }),
    ]);
  });
});
