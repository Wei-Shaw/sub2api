import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

import enAdmin from "../../../i18n/locales/en/admin/overview";
import zhAdmin from "../../../i18n/locales/zh/admin/overview";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

describe("groups endpoint default routing", () => {
  it("shows the setting only for composite create and edit forms", () => {
    expect(groupsViewSource).toContain(
      `v-if="createForm.platform === 'composite'"`,
    );
    expect(groupsViewSource).toContain(
      `v-if="editForm.platform === 'composite'"`,
    );
    expect(groupsViewSource).toContain(
      "createForm.endpoint_default_routing_enabled",
    );
    expect(groupsViewSource).toContain(
      "editForm.endpoint_default_routing_enabled",
    );
  });

  it("documents the fallback precedence and unsupported media endpoints in both locales", () => {
    const zhTooltip = zhAdmin.groups.endpointDefaultRouting.tooltip;
    const enTooltip = enAdmin.groups.endpointDefaultRouting.tooltip;

    // The tooltip must not promise that the endpoint default overrides the
    // detector: it runs last, so enabling it cannot retarget working traffic.
    expect(zhTooltip).toContain("内置模型名识别的优先级都高于该兜底");
    expect(zhTooltip).toContain("不会改变任何原本就能正常路由的请求");
    expect(zhTooltip).toContain("Images、Videos");
    expect(enTooltip).toContain("outrank this fallback");
    expect(enTooltip).toContain("never changes a request that already routed");
    expect(enTooltip).toContain("Images and Videos");
  });

  it("labels the endpoint-default routing source in both locales", () => {
    expect(zhAdmin.groups.compositeRoutes.sources.endpointDefault).toBe(
      "端点默认",
    );
    expect(enAdmin.groups.compositeRoutes.sources.endpointDefault).toBe(
      "Endpoint Default",
    );
  });
});
