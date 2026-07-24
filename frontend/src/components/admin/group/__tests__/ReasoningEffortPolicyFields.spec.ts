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
  name: "SelectStub",
  props: ["id", "modelValue", "options", "disabled"],
  emits: ["update:modelValue"],
  template:
    '<button type="button" class="select-stub" :data-id="id" @click="$emit(\'update:modelValue\', null)">{{ modelValue }}</button>',
};

function mountFields(
  modelRules = [] as ReturnType<typeof createModelReasoningEffortRuleRow>[],
  mappings = [] as ReturnType<typeof createReasoningEffortMappingRow>[],
  modelOptions = ["gpt-5.6-luna", "gpt-5.6-sol"],
  modelOptionsLoading = false,
) {
  return mount(ReasoningEffortPolicyFields, {
    props: {
      idPrefix: "test-reasoning",
      platform: "openai",
      maxEffort: "",
      mappings,
      modelRules,
      modelOptions,
      modelOptionsLoading,
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

  it("selects exact models from the provided candidates", async () => {
    const rule = createModelReasoningEffortRuleRow();
    const wrapper = mountFields([rule]);
    const modelSelect = wrapper
      .findAllComponents(SelectStub)
      .find(
        (select) =>
          select.props("id") === `test-reasoning-${rule.id}-model`,
      );

    expect(modelSelect).toBeDefined();
    expect(modelSelect!.props("options")).toEqual([
      { value: "gpt-5.6-luna", label: "gpt-5.6-luna", disabled: false },
      { value: "gpt-5.6-sol", label: "gpt-5.6-sol", disabled: false },
    ]);
    expect(
      wrapper.find('input[placeholder="admin.groups.form.reasoningEffortModelPlaceholder"]').exists(),
    ).toBe(false);

    modelSelect!.vm.$emit("update:modelValue", "gpt-5.6-sol");
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted("update:modelRules")?.[0][0]).toEqual([
      expect.objectContaining({
        id: rule.id,
        model: "gpt-5.6-sol",
      }),
    ]);
  });

  it("disables models already selected by another rule", () => {
    const first = createModelReasoningEffortRuleRow({
      model: "gpt-5.6-luna",
    });
    const second = createModelReasoningEffortRuleRow();
    const wrapper = mountFields([first, second]);
    const secondModelSelect = wrapper
      .findAllComponents(SelectStub)
      .find(
        (select) =>
          select.props("id") === `test-reasoning-${second.id}-model`,
      );

    expect(secondModelSelect!.props("options")).toContainEqual({
      value: "gpt-5.6-luna",
      label: "gpt-5.6-luna",
      disabled: true,
    });
  });

  it("requires a model before configuring its ceiling or mappings", () => {
    const rule = createModelReasoningEffortRuleRow();
    const wrapper = mountFields([rule]);
    const selects = wrapper.findAllComponents(SelectStub);
    const modelSelect = selects.find(
      (select) =>
        select.props("id") === `test-reasoning-${rule.id}-model`,
    );
    const maxEffortSelect = selects.find(
      (select) =>
        select.props("id") === `test-reasoning-${rule.id}-max-effort`,
    );
    const addMappingButton = wrapper
      .findAll("button")
      .find(
        (button) =>
          button
            .text()
            .includes("admin.groups.form.addReasoningEffortMapping") &&
          button.attributes("disabled") !== undefined,
      );

    expect(modelSelect!.props("disabled")).toBe(false);
    expect(maxEffortSelect!.props("disabled")).toBe(true);
    expect(addMappingButton!.attributes("disabled")).toBeDefined();
  });

  it("preserves an existing unknown model while candidates load", () => {
    const rule = createModelReasoningEffortRuleRow({
      model: "gpt-custom-legacy",
    });
    const wrapper = mountFields([rule], [], [], true);
    const modelSelect = wrapper
      .findAllComponents(SelectStub)
      .find(
        (select) =>
          select.props("id") === `test-reasoning-${rule.id}-model`,
      );

    expect(modelSelect!.props("disabled")).toBe(true);
    expect(modelSelect!.props("options")).toEqual([
      {
        value: "gpt-custom-legacy",
        label: "gpt-custom-legacy",
        disabled: false,
      },
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
