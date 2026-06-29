import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { flushPromises, mount } from "@vue/test-utils";

const apiMocks = vi.hoisted(() => ({
  getEmailTemplates: vi.fn(),
  getEmailTemplate: vi.fn(),
  restoreOfficialEmailTemplate: vi.fn(),
  previewEmailTemplate: vi.fn(),
}));

const storeMocks = vi.hoisted(() => ({
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/api", () => ({
  adminAPI: {
    settings: apiMocks,
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => storeMocks,
}));

vi.mock("@/utils/apiError", () => ({
  extractApiErrorMessage: () => "error",
}));

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: "zh-CN" },
    }),
  };
});

vi.mock("@/components/common/ConfirmDialog.vue", () => ({
  default: defineComponent({
    name: "ConfirmDialog",
    props: {
      show: Boolean,
      title: String,
      message: String,
      confirmText: String,
      cancelText: String,
    },
    emits: ["confirm", "cancel"],
    setup(props, { emit }) {
      return () =>
        props.show
          ? h("div", { class: "confirm-dialog-stub" }, [
              h("h3", props.title),
              h("p", props.message),
              h("button", { type: "button", onClick: () => emit("cancel") }, props.cancelText),
              h("button", { type: "button", onClick: () => emit("confirm") }, props.confirmText),
            ])
          : null;
    },
  }),
}));

vi.mock("@/components/common/Select.vue", () => ({
  default: defineComponent({
    name: "SelectStub",
    props: {
      modelValue: {
        type: String,
        default: "",
      },
      options: {
        type: Array,
        default: () => [],
      },
    },
    emits: ["update:modelValue"],
    setup(props, { emit }) {
      return () =>
        h(
          "select",
          {
            value: props.modelValue,
            onChange: (event: Event) =>
              emit("update:modelValue", (event.target as HTMLSelectElement).value),
          },
          (props.options as Array<Record<string, string>>).map((option) =>
            h("option", { value: option.value }, option.label),
          ),
        );
    },
  }),
}));

import EmailTemplateEditor from "../EmailTemplateEditor.vue";

const template = {
  subject: "Verify",
  html: "<p>Verify</p>",
  is_custom: true,
  placeholders: ["recipient_name"],
};

async function mountLoadedEditor() {
  const wrapper = mount(EmailTemplateEditor);
  await flushPromises();
  return wrapper;
}

beforeEach(() => {
  vi.clearAllMocks();
  apiMocks.getEmailTemplates.mockResolvedValue({
    events: [{ value: "auth.verify_code", label: "Verify Code" }],
    locales: ["zh-CN"],
    placeholders: ["recipient_name"],
  });
  apiMocks.getEmailTemplate.mockResolvedValue(template);
  apiMocks.previewEmailTemplate.mockResolvedValue({
    subject: template.subject,
    html: template.html,
  });
  apiMocks.restoreOfficialEmailTemplate.mockResolvedValue({
    ...template,
    is_custom: false,
  });
});

describe("EmailTemplateEditor", () => {
  it("恢复官方模板使用统一 ConfirmDialog，取消时不调用恢复 API", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const wrapper = await mountLoadedEditor();

    const restoreButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "admin.settings.emailTemplates.restoreOfficial");

    expect(restoreButton).toBeTruthy();
    await restoreButton!.trigger("click");
    await flushPromises();

    expect(confirmSpy).not.toHaveBeenCalled();
    expect(wrapper.html()).toContain("admin.settings.emailTemplates.restoreConfirm");

    const cancelButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "common.cancel");

    expect(cancelButton).toBeTruthy();
    await cancelButton!.trigger("click");
    await flushPromises();

    expect(apiMocks.restoreOfficialEmailTemplate).not.toHaveBeenCalled();
    confirmSpy.mockRestore();
  });

  it("确认统一 ConfirmDialog 后恢复官方模板", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const wrapper = await mountLoadedEditor();

    const restoreButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "admin.settings.emailTemplates.restoreOfficial");

    expect(restoreButton).toBeTruthy();
    await restoreButton!.trigger("click");
    await flushPromises();

    expect(confirmSpy).not.toHaveBeenCalled();
    expect(apiMocks.restoreOfficialEmailTemplate).not.toHaveBeenCalled();

    const confirmButton = wrapper
      .findAll("button")
      .find((button) => button.text() === "common.confirm");

    expect(confirmButton).toBeTruthy();
    await confirmButton!.trigger("click");
    await flushPromises();

    expect(apiMocks.restoreOfficialEmailTemplate).toHaveBeenCalledWith("auth.verify_code", "zh-CN");
    confirmSpy.mockRestore();
  });
});
