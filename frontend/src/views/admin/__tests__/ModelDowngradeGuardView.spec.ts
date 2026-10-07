import { beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h } from "vue";
import { flushPromises, mount } from "@vue/test-utils";

import ModelDowngradeGuardView from "../ModelDowngradeGuardView.vue";

const {
  getModelDowngradeGuardSettings,
  updateModelDowngradeGuardSettings,
  getModelDowngradeGuardBlocked,
  releaseModelDowngradeGuardBlocked,
  applyModelDowngradeGuardBlocked,
  showError,
  showSuccess,
} = vi.hoisted(() => ({
  getModelDowngradeGuardSettings: vi.fn(),
  updateModelDowngradeGuardSettings: vi.fn(),
  getModelDowngradeGuardBlocked: vi.fn(),
  releaseModelDowngradeGuardBlocked: vi.fn(),
  applyModelDowngradeGuardBlocked: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}));

vi.mock("@/api", () => ({
  adminAPI: {
    settings: {
      getModelDowngradeGuardSettings,
      updateModelDowngradeGuardSettings,
      getModelDowngradeGuardBlocked,
      releaseModelDowngradeGuardBlocked,
      applyModelDowngradeGuardBlocked,
    },
  },
}));

vi.mock("@/stores", () => ({
  useAppStore: () => ({
    showError,
    showSuccess,
    showWarning: vi.fn(),
    showInfo: vi.fn(),
  }),
}));

vi.mock("@/utils/apiError", async () => {
  const actual =
    await vi.importActual<typeof import("@/utils/apiError")>(
      "@/utils/apiError",
    );
  return {
    ...actual,
    extractApiErrorMessage: () => "error",
  };
});

vi.mock("vue-i18n", async () => {
  const actual = await vi.importActual<typeof import("vue-i18n")>("vue-i18n");
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        params ? `${key}:${JSON.stringify(params)}` : key,
    }),
  };
});

const AppLayoutStub = { template: "<div><slot /></div>" };
const RouterLinkStub = { props: ["to"], template: "<a><slot /></a>" };
const ToggleStub = defineComponent({
  props: {
    modelValue: {
      type: Boolean,
      default: false,
    },
  },
  emits: ["update:modelValue"],
  inheritAttrs: false,
  setup(props, { attrs, emit }) {
    return () =>
      h("input", {
        ...attrs,
        class: "toggle-stub",
        type: "checkbox",
        checked: props.modelValue,
        onChange: (event: Event) => {
          emit("update:modelValue", (event.target as HTMLInputElement).checked);
        },
      });
  },
});

function mountView() {
  return mount(ModelDowngradeGuardView, {
    global: {
      stubs: {
        AppLayout: AppLayoutStub,
        Toggle: ToggleStub,
        RouterLink: RouterLinkStub,
      },
    },
  });
}

function guardSettingsFixture() {
  return {
    enabled: true,
    action: "model_block",
    pairs: [{ sent_model: "gpt-6-astra", response_model: "gpt-5.6-luna" }],
    threshold_count: 5,
    threshold_window_minutes: 30,
    block_hours: 24,
    max_blocked_ratio: 0.3,
  };
}

function resetMocks() {
  getModelDowngradeGuardSettings.mockReset();
  updateModelDowngradeGuardSettings.mockReset();
  getModelDowngradeGuardBlocked.mockReset();
  releaseModelDowngradeGuardBlocked.mockReset();
  applyModelDowngradeGuardBlocked.mockReset();
  showError.mockReset();
  showSuccess.mockReset();

  getModelDowngradeGuardSettings.mockResolvedValue(guardSettingsFixture());
  updateModelDowngradeGuardSettings.mockImplementation(async (payload) => payload);
}

describe("ModelDowngradeGuardView settings", () => {
  beforeEach(() => {
    resetMocks();
    getModelDowngradeGuardBlocked.mockResolvedValue({
      items: [],
      summary: { blocked: 0, observed: 0, total_active: 0, max_blocked_ratio: 0.3 },
    });
  });

  it("renders the guard settings and saves the configured pairs", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(getModelDowngradeGuardSettings).toHaveBeenCalled();
    expect(wrapper.text()).toContain("admin.modelDowngradeGuard.title");

    const action = wrapper.find<HTMLSelectElement>(
      '[data-testid="model-downgrade-guard-action"]',
    );
    expect(action.element.value).toBe("model_block");
    // 默认动作「仅屏蔽该模型」排在第一个。
    expect(action.findAll("option")[0].attributes("value")).toBe("model_block");

    const thresholdInput = wrapper.find(
      '[data-testid="model-downgrade-guard-threshold-count"]',
    );
    expect(thresholdInput.exists()).toBe(true);
    await thresholdInput.setValue("8");

    await wrapper.find('[data-testid="model-downgrade-guard-save"]').trigger("click");
    await flushPromises();

    expect(updateModelDowngradeGuardSettings).toHaveBeenCalledWith({
      enabled: true,
      action: "model_block",
      pairs: [{ sent_model: "gpt-6-astra", response_model: "gpt-5.6-luna" }],
      threshold_count: 8,
      threshold_window_minutes: 30,
      block_hours: 24,
      max_blocked_ratio: 0.3,
    });
    expect(showSuccess).toHaveBeenCalled();
    wrapper.unmount();
  });

  it("saves the whole-account action when selected", async () => {
    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-action"]')
      .setValue("temp_unsched");
    await wrapper.find('[data-testid="model-downgrade-guard-save"]').trigger("click");
    await flushPromises();

    expect(updateModelDowngradeGuardSettings).toHaveBeenCalledWith(
      expect.objectContaining({ action: "temp_unsched" }),
    );
    wrapper.unmount();
  });

  it("appends and removes downgrade pairs", async () => {
    const wrapper = mountView();
    await flushPromises();

    await wrapper.find('[data-testid="model-downgrade-guard-add-pair"]').trigger("click");
    let sentModelInputs = wrapper.findAll<HTMLInputElement>(
      '[aria-label="admin.modelDowngradeGuard.pairSentModel"]',
    );
    expect(sentModelInputs).toHaveLength(2);

    await sentModelInputs[1].setValue("gpt-6-sol");
    await wrapper
      .findAll<HTMLInputElement>(
        '[aria-label="admin.modelDowngradeGuard.pairResponseModel"]',
      )[1]
      .setValue("gpt-5.6-luna");

    await wrapper.find('[data-testid="model-downgrade-guard-save"]').trigger("click");
    await flushPromises();

    expect(updateModelDowngradeGuardSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        pairs: [
          { sent_model: "gpt-6-astra", response_model: "gpt-5.6-luna" },
          { sent_model: "gpt-6-sol", response_model: "gpt-5.6-luna" },
        ],
      }),
    );

    const removeButtons = wrapper.findAll("button").filter(
      (button) => button.text() === "admin.modelDowngradeGuard.removePair",
    );
    expect(removeButtons).toHaveLength(2);
    await removeButtons[0].trigger("click");

    sentModelInputs = wrapper.findAll<HTMLInputElement>(
      '[aria-label="admin.modelDowngradeGuard.pairSentModel"]',
    );
    expect(sentModelInputs).toHaveLength(1);
    expect(sentModelInputs[0].element.value).toBe("gpt-6-sol");
    wrapper.unmount();
  });

  it("surfaces an error when saving fails", async () => {
    updateModelDowngradeGuardSettings.mockRejectedValueOnce(new Error("boom"));
    const wrapper = mountView();
    await flushPromises();

    await wrapper.find('[data-testid="model-downgrade-guard-save"]').trigger("click");
    await flushPromises();

    expect(showError).toHaveBeenCalled();
    expect(showSuccess).not.toHaveBeenCalled();
    wrapper.unmount();
  });
});

describe("ModelDowngradeGuardView blocked accounts table", () => {
  const until = new Date(Date.now() + 5 * 60 * 60 * 1000).toISOString();
  const triggeredAt = new Date(Date.now() - 30 * 60 * 1000).toISOString();

  beforeEach(() => {
    resetMocks();
    releaseModelDowngradeGuardBlocked.mockResolvedValue({
      account_id: 101,
      scope: "account",
    });
    getModelDowngradeGuardBlocked.mockResolvedValue({
      items: [
        {
          account_id: 101,
          account_name: "openai-pool-1",
          scope: "account",
          status: "blocked",
          sent_model: "gpt-6-astra",
          response_model: "gpt-5.6-luna",
          trigger_count: 5,
          trigger_threshold: 5,
          trigger_window_minutes: 30,
          triggered_at: triggeredAt,
          until,
        },
        {
          account_id: 102,
          account_name: "openai-pool-2",
          scope: "model",
          status: "blocked",
          model: "gpt-6-astra",
          sent_model: "gpt-6-astra",
          response_model: "gpt-5.6-luna",
          trigger_count: 5,
          trigger_threshold: 5,
          trigger_window_minutes: 30,
          triggered_at: triggeredAt,
          until,
        },
      ],
      summary: { blocked: 2, observed: 0, total_active: 20, max_blocked_ratio: 0.3 },
    });
  });

  it("renders one row per blocked entry with the summary line", async () => {
    const wrapper = mountView();
    await flushPromises();

    expect(getModelDowngradeGuardBlocked).toHaveBeenCalled();

    const rows = wrapper.findAll(
      '[data-testid="model-downgrade-guard-blocked-row"]',
    );
    expect(rows).toHaveLength(2);
    expect(rows[0].text()).toContain("#101 openai-pool-1");
    expect(rows[0].text()).toContain("gpt-6-astra → gpt-5.6-luna");
    expect(rows[0].text()).toContain(
      "admin.modelDowngradeGuard.blocked.scopeAccount",
    );
    expect(rows[1].text()).toContain(
      "admin.modelDowngradeGuard.blocked.scopeModel",
    );
    // Both scopes can be released early through the guard-specific endpoint.
    expect(
      rows[1].find('[data-testid="model-downgrade-guard-blocked-restore"]')
        .exists(),
    ).toBe(true);

    const summary = wrapper.find(
      '[data-testid="model-downgrade-guard-blocked-summary"]',
    );
    expect(summary.text()).toContain("admin.modelDowngradeGuard.blocked.summary");
    expect(summary.text()).toContain('"blocked":2');
    expect(summary.text()).toContain('"observed":0');
    expect(summary.text()).toContain('"total":20');
    expect(summary.text()).toContain('"pct":"10"');
    expect(summary.text()).toContain('"max":"30"');

    wrapper.unmount();
  });

  it("clears the temporary block and reloads after confirming an early restore", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    const wrapper = mountView();
    await flushPromises();
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(1);

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-restore"]')
      .trigger("click");
    await flushPromises();

    expect(confirmSpy.mock.calls[0][0]).toContain(
      "admin.modelDowngradeGuard.blocked.restoreConfirm",
    );
    // The guard-specific endpoint is used so unrelated model rate limits survive.
    expect(releaseModelDowngradeGuardBlocked).toHaveBeenCalledWith(
      101,
      "account",
      undefined,
    );
    expect(showSuccess).toHaveBeenCalled();
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(2);

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("releases only the blocked model for a model-scope row", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);

    const wrapper = mountView();
    await flushPromises();

    const rows = wrapper.findAll(
      '[data-testid="model-downgrade-guard-blocked-row"]',
    );
    await rows[1]
      .find('[data-testid="model-downgrade-guard-blocked-restore"]')
      .trigger("click");
    await flushPromises();

    expect(confirmSpy.mock.calls[0][0]).toContain(
      "admin.modelDowngradeGuard.blocked.restoreConfirmModel",
    );
    expect(releaseModelDowngradeGuardBlocked).toHaveBeenCalledWith(
      102,
      "model",
      "gpt-6-astra",
    );
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(2);

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("skips the restore call when the confirmation is dismissed", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);

    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-restore"]')
      .trigger("click");
    await flushPromises();

    expect(releaseModelDowngradeGuardBlocked).not.toHaveBeenCalled();
    expect(showSuccess).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
    wrapper.unmount();
  });
});

describe("ModelDowngradeGuardView observation records", () => {
  const until = new Date(Date.now() + 5 * 60 * 60 * 1000).toISOString();
  const triggeredAt = new Date(Date.now() - 30 * 60 * 1000).toISOString();

  beforeEach(() => {
    resetMocks();
    getModelDowngradeGuardSettings.mockResolvedValue({
      ...guardSettingsFixture(),
      action: "none",
    });
    releaseModelDowngradeGuardBlocked.mockResolvedValue({
      account_id: 103,
      scope: "observed",
      model: "gpt-6-astra",
    });
    applyModelDowngradeGuardBlocked.mockResolvedValue({
      account_id: 103,
      scope: "model",
      model: "gpt-6-astra",
      until,
    });
    getModelDowngradeGuardBlocked.mockResolvedValue({
      items: [
        {
          account_id: 103,
          account_name: "openai-pool-3",
          scope: "observed",
          status: "observed",
          cause: "dry_run",
          model: "gpt-6-astra",
          sent_model: "gpt-6-astra",
          response_model: "gpt-5.6-luna",
          trigger_count: 5,
          trigger_threshold: 5,
          trigger_window_minutes: 30,
          triggered_at: triggeredAt,
          until,
        },
        {
          account_id: 104,
          account_name: "openai-pool-4",
          scope: "observed",
          status: "ratio_capped",
          cause: "ratio_cap",
          model: "gpt-6-sol",
          sent_model: "gpt-6-sol",
          response_model: "gpt-5.6-luna",
          trigger_count: 7,
          trigger_threshold: 5,
          trigger_window_minutes: 30,
          triggered_at: triggeredAt,
          until,
          blocked: 3,
          total: 10,
          max_blocked_ratio: 0.3,
        },
      ],
      summary: {
        blocked: 1,
        observed: 2,
        total_active: 20,
        max_blocked_ratio: 0.3,
      },
    });
  });

  it("renders observation rows with their own status badge and scope label", async () => {
    const wrapper = mountView();
    await flushPromises();

    const rows = wrapper.findAll(
      '[data-testid="model-downgrade-guard-blocked-row"]',
    );
    expect(rows).toHaveLength(2);

    const badges = wrapper.findAll(
      '[data-testid="model-downgrade-guard-blocked-status"]',
    );
    expect(badges[0].text()).toContain(
      "admin.modelDowngradeGuard.blocked.statusObserved",
    );
    expect(badges[1].text()).toContain(
      "admin.modelDowngradeGuard.blocked.statusRatioCapped",
    );
    // 上限拦下的那一行 hover 时要能看到当时的分子分母。
    expect(badges[1].attributes("title")).toContain(
      "admin.modelDowngradeGuard.blocked.statusRatioCappedTitle",
    );
    expect(badges[1].attributes("title")).toContain('"blocked":3');
    expect(badges[1].attributes("title")).toContain('"total":10');
    expect(badges[1].attributes("title")).toContain('"max":"30"');

    expect(rows[0].text()).toContain(
      "admin.modelDowngradeGuard.blocked.scopeObserved",
    );

    // 观察行没有被限制过，操作列给的是「立即处理」和「清除记录」。
    expect(
      rows[0].find('[data-testid="model-downgrade-guard-blocked-restore"]').exists(),
    ).toBe(false);
    expect(
      rows[0].find('[data-testid="model-downgrade-guard-blocked-apply"]').exists(),
    ).toBe(true);
    expect(
      rows[0].find('[data-testid="model-downgrade-guard-blocked-clear"]').exists(),
    ).toBe(true);

    wrapper.unmount();
  });

  it("includes the observation count in the summary line", async () => {
    const wrapper = mountView();
    await flushPromises();

    const summary = wrapper.find(
      '[data-testid="model-downgrade-guard-blocked-summary"]',
    );
    expect(summary.text()).toContain("admin.modelDowngradeGuard.blocked.summary");
    expect(summary.text()).toContain('"blocked":1');
    expect(summary.text()).toContain('"observed":2');
    // 占比仍然只按真实受限算：1/20 = 5%。
    expect(summary.text()).toContain('"pct":"5"');

    wrapper.unmount();
  });

  it("clears an observation record through the observed scope", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const wrapper = mountView();
    await flushPromises();
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(1);

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-clear"]')
      .trigger("click");
    await flushPromises();

    expect(confirmSpy.mock.calls[0][0]).toContain(
      "admin.modelDowngradeGuard.blocked.clearConfirm",
    );
    expect(releaseModelDowngradeGuardBlocked).toHaveBeenCalledWith(
      103,
      "observed",
      "gpt-6-astra",
    );
    expect(showSuccess).toHaveBeenCalled();
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(2);

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("turns an observation record into a real block through the apply endpoint", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-apply"]')
      .trigger("click");
    await flushPromises();

    expect(confirmSpy.mock.calls[0][0]).toContain(
      "admin.modelDowngradeGuard.blocked.applyConfirm",
    );
    expect(applyModelDowngradeGuardBlocked).toHaveBeenCalledWith(
      103,
      "gpt-6-astra",
    );
    expect(releaseModelDowngradeGuardBlocked).not.toHaveBeenCalled();
    expect(showSuccess).toHaveBeenCalled();
    expect(getModelDowngradeGuardBlocked).toHaveBeenCalledTimes(2);

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("explains the blocked-ratio cap when applying returns 409", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    // api/client.ts 的拦截器 reject 的是这个顶层形状，不是原始 Axios 错误。
    applyModelDowngradeGuardBlocked.mockRejectedValueOnce({
      status: 409,
      reason: "MODEL_DOWNGRADE_BLOCK_RATIO_CAPPED",
      message: "model downgrade block rejected by the blocked-ratio cap",
      metadata: { blocked: "3", total: "10", max_blocked_ratio: "0.3" },
    });

    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-apply"]')
      .trigger("click");
    await flushPromises();

    expect(showError).toHaveBeenCalled();
    const message = showError.mock.calls[0][0] as string;
    expect(message).toContain("admin.modelDowngradeGuard.blocked.applyRatioCapped");
    expect(message).toContain('"blocked":"3"');
    expect(message).toContain('"total":"10"');
    expect(message).toContain('"max":"30"');
    expect(showSuccess).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("explains an already-blocked account instead of a zero ratio when applying returns 409", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    // 已被限制的 409 也带 metadata（scope），不能被当成比例上限拦下渲染成 0/0。
    applyModelDowngradeGuardBlocked.mockRejectedValueOnce({
      status: 409,
      reason: "MODEL_DOWNGRADE_BLOCK_ALREADY_ACTIVE",
      message: "account is already blocked by the model downgrade guard",
      metadata: { scope: "model" },
    });

    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-apply"]')
      .trigger("click");
    await flushPromises();

    expect(showError).toHaveBeenCalledWith(
      "admin.modelDowngradeGuard.blocked.applyAlreadyBlocked",
    );
    expect(showSuccess).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("falls back to the generic message when the 409 carries no metadata", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
    applyModelDowngradeGuardBlocked.mockRejectedValueOnce({
      status: 409,
      reason: "MODEL_DOWNGRADE_BLOCK_RATIO_CAPPED",
      message: "model downgrade block rejected by the blocked-ratio cap",
    });

    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-apply"]')
      .trigger("click");
    await flushPromises();

    expect(showError).toHaveBeenCalled();
    const message = showError.mock.calls[0][0] as string;
    expect(message).not.toContain(
      "admin.modelDowngradeGuard.blocked.applyRatioCapped",
    );
    expect(message).toBe("error");
    expect(showSuccess).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
    wrapper.unmount();
  });

  it("skips both observation actions when the confirmation is dismissed", async () => {
    const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(false);
    const wrapper = mountView();
    await flushPromises();

    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-apply"]')
      .trigger("click");
    await wrapper
      .find('[data-testid="model-downgrade-guard-blocked-clear"]')
      .trigger("click");
    await flushPromises();

    expect(applyModelDowngradeGuardBlocked).not.toHaveBeenCalled();
    expect(releaseModelDowngradeGuardBlocked).not.toHaveBeenCalled();

    confirmSpy.mockRestore();
    wrapper.unmount();
  });
});
