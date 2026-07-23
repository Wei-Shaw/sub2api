import { mount } from "@vue/test-utils";
import { describe, expect, it, vi } from "vitest";

import ReasoningEffortPolicyFields from "../ReasoningEffortPolicyFields.vue";
import {
  createModelReasoningEffortRuleRow,
  createReasoningEffortMappingRow,
} from "@/views/admin/groupsReasoningEffort";

vi.mock("vue-i18n", () => ({
  useI18n: () => ({
    t: (key: string) => key,
  }),
}));

const SelectStub = {
  props: ["id", "modelValue"],
  emits: ["update:modelValue"],
  template:
    '<button type="button" class="select-stub" :data-id="id" @click="$emit(\'update:modelValue\', null)">{{ modelValue }}</button>',
};

function mountFields(
  modelRules = [] as ReturnType<typeof createModelReasoningEffortRuleRow>[],
  mappings = [] as ReturnType<typeof createReasoningEffortMappingRow>[],
) {
  return mount(ReasoningEffortPolicyFields, {
    props: {
      idPrefix: "test-reasoning",
      platform: "openai",
      maxEffort: "",
      mappings,
      modelRules,
    },
    global: {
      stubs: {
        Icon: true,
        Select: SelectStub,
      },
    },
  });
}

describe("ReasoningEffortPolicyFields", () => {
  it("adds an exact model rule with an explicit unlimited override", async () => {
    const wrapper = mountFields();
    const addButton = wrapper
      .findAll("button")
      .find((button) =>
        button.text().includes("admin.groups.form.addModelReasoningEffortRule"),
      );

    expect(addButton).toBeDefined();
    await addButton!.trigger("click");

    const emitted = wrapper.emitted("update:modelRules");
    expect(emitted).toHaveLength(1);
    expect(emitted![0][0]).toEqual([
      expect.objectContaining({
        model: "",
        max_reasoning_effort: "",
        reasoning_effort_mappings: [],
      }),
    ]);
  });

  it("clears a model ceiling without removing the override rule", async () => {
    const rule = createModelReasoningEffortRuleRow({
      model: "gpt-5.6-luna",
      max_reasoning_effort: "high",
      reasoning_effort_mappings: [],
    });
    const wrapper = mountFields([rule]);

    await wrapper
      .get(`[data-id="test-reasoning-${rule.id}-max-effort"]`)
      .trigger("click");

    const emitted = wrapper.emitted("update:modelRules");
    expect(emitted).toHaveLength(1);
    expect(emitted![0][0]).toEqual([
      expect.objectContaining({
        id: rule.id,
        model: "gpt-5.6-luna",
        max_reasoning_effort: "",
      }),
    ]);
  });

  it("accepts explicit unlimited rules and rejects duplicate models", () => {
    const first = createModelReasoningEffortRuleRow({
      model: "gpt-5.6-luna",
      max_reasoning_effort: "",
      reasoning_effort_mappings: [],
    });
    const validWrapper = mountFields([first]);
    expect((validWrapper.vm as unknown as { validate: () => boolean }).validate()).toBe(true);

    const second = createModelReasoningEffortRuleRow({
      model: "gpt-5.6-luna",
      max_reasoning_effort: "medium",
      reasoning_effort_mappings: [],
    });
    const invalidWrapper = mountFields([first, second]);
    expect((invalidWrapper.vm as unknown as { validate: () => boolean }).validate()).toBe(false);
  });
});
