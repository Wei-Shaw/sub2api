import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const channelsViewSource = readFileSync(
  resolve(currentDir, "../ChannelsView.vue"),
  "utf8",
);

describe("composite channel platforms", () => {
  it.each(["atlascloud", "apiz"])(
    "makes composite groups available to %s channel sections",
    (platform) => {
      const declaration = channelsViewSource.match(
        /const compositePlatforms:[^=]+=(.*)/,
      )?.[1];

      expect(declaration).toContain(`'${platform}'`);
    },
  );
});
