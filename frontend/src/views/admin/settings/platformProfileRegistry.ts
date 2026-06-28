export interface PlatformGuideCopyBlock {
  id?: string;
  client_id?: string;
  os?: string;
  path?: string;
  hint?: string;
  language?: string;
  content_template?: string;
}

export interface PlatformGuideClient {
  id?: string;
  label?: string;
  os?: string[];
}

export interface PlatformGuideMetadata {
  profile_id?: string;
  title?: string;
  description?: string;
  note?: string;
  docs_url?: string;
  default_client?: string;
  clients?: PlatformGuideClient[];
  copy_blocks?: PlatformGuideCopyBlock[];
}

export interface PlatformProfile {
  platform?: string;
  provider_id?: string;
  provider_name?: string;
  api_style?: string;
  guide?: PlatformGuideMetadata;
}

export interface PlatformProfileRegistry {
  version?: number;
  profiles?: PlatformProfile[];
  [key: string]: unknown;
}

export interface PlatformProfileRegistryValidation {
  ok: boolean;
  isEmpty: boolean;
  registry: PlatformProfileRegistry | null;
  errors: string[];
  warnings: string[];
}

export const platformProfileRegistryPlaceholders = [
  "{{base_url}}",
  "{{base_root}}",
  "{{api_base_url}}",
  "{{api_key}}",
  "{{openai_model}}",
  "{{gemini_model}}",
  "{{gemini_base_url}}",
  "{{antigravity_base_url}}",
  "{{antigravity_gemini_base_url}}",
] as const;

export const platformProfileRegistryPlaceholderSamples: Record<string, string> = {
  "{{base_url}}": "https://token.v-claw.org/v1",
  "{{base_root}}": "https://token.v-claw.org",
  "{{api_base_url}}": "https://token.v-claw.org/v1",
  "{{api_key}}": "sk-vcl...cted",
  "{{openai_model}}": "gpt-5.5",
  "{{gemini_model}}": "gemini-2.5-pro",
  "{{gemini_base_url}}": "https://token.v-claw.org/gemini/v1beta",
  "{{antigravity_base_url}}": "https://token.v-claw.org/antigravity/v1",
  "{{antigravity_gemini_base_url}}": "https://token.v-claw.org/antigravity/gemini/v1beta",
};

export const platformProfileRegistryPlaceholderSampleValues = Object.fromEntries(
  Object.entries(platformProfileRegistryPlaceholderSamples).map(([placeholder, value]) => [
    placeholder.replace(/^\{\{\s*/, "").replace(/\s*\}\}$/, ""),
    value,
  ]),
) as Record<string, string>;

export const platformProfileRegistryOSOptions = ["unix", "windows", "cmd", "powershell"] as const;

export const platformProfileRegistryLanguageOptions = [
  "toml",
  "json",
  "shell",
  "bash",
  "bat",
  "powershell",
  "text",
] as const;

export function renderPlatformProfileGuideTemplate(
  template: string,
  values: Record<string, string>,
): string {
  return template.replace(/\{\{\s*([a-zA-Z0-9_]+)\s*\}\}/g, (_match, key: string) => values[key] ?? "");
}

const defaultPlatformIDs = ["openai", "anthropic", "gemini"] as const;

export function createEmptyPlatformProfileRegistry(): PlatformProfileRegistry {
  return {
    version: 1,
    profiles: [],
  };
}

export function clonePlatformProfileRegistry(
  registry: PlatformProfileRegistry,
): PlatformProfileRegistry {
  return JSON.parse(JSON.stringify(registry)) as PlatformProfileRegistry;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function readString(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

function readStringArray(value: unknown): string[] {
  if (!Array.isArray(value)) return [];
  return value
    .filter((item): item is string => typeof item === "string")
    .map((item) => item.trim())
    .filter(Boolean);
}

function normalizePlatformID(value: string): string {
  return value.trim().toLowerCase();
}

export function validatePlatformProfileRegistryInput(
  raw: string,
): PlatformProfileRegistryValidation {
  const trimmed = raw.trim();
  if (!trimmed) {
    return {
      ok: true,
      isEmpty: true,
      registry: null,
      errors: [],
      warnings: [
        "Để trống rồi lưu sẽ để backend ghi lại registry mặc định OpenAI, Anthropic, Gemini vào DB.",
      ],
    };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch (error) {
    return {
      ok: false,
      isEmpty: false,
      registry: null,
      errors: [
        `JSON không hợp lệ: ${
          error instanceof Error ? error.message : "không parse được nội dung"
        }`,
      ],
      warnings: [],
    };
  }

  const errors: string[] = [];
  const warnings: string[] = [];

  if (!isRecord(parsed)) {
    return {
      ok: false,
      isEmpty: false,
      registry: null,
      errors: ["Registry phải là một JSON object có dạng { version, profiles }."],
      warnings: [],
    };
  }

  const registry = parsed as PlatformProfileRegistry;
  const profilesRaw = registry.profiles;
  if (!Array.isArray(profilesRaw) || profilesRaw.length === 0) {
    errors.push("profiles phải là mảng và có ít nhất một platform profile.");
  }

  const seenPlatforms = new Set<string>();
  const normalizedProfiles: PlatformProfile[] = [];

  if (Array.isArray(profilesRaw)) {
    profilesRaw.forEach((profileRaw, profileIndex) => {
      const profilePath = `profiles[${profileIndex}]`;
      if (!isRecord(profileRaw)) {
        errors.push(`${profilePath} phải là object.`);
        return;
      }

      const profile = profileRaw as PlatformProfile;
      const platform = normalizePlatformID(readString(profile.platform));
      const providerID = readString(profile.provider_id);
      const providerName = readString(profile.provider_name);
      const apiStyle = readString(profile.api_style);

      if (!platform) {
        errors.push(`${profilePath}.platform là bắt buộc.`);
      } else if (seenPlatforms.has(platform)) {
        errors.push(`platform bị trùng: ${platform}.`);
      } else {
        seenPlatforms.add(platform);
      }

      if (!isRecord(profile.guide)) {
        errors.push(`${profilePath}.guide là bắt buộc.`);
        normalizedProfiles.push({
          ...profile,
          platform,
          provider_id: providerID,
          provider_name: providerName,
          api_style: apiStyle,
        });
        return;
      }

      const guide = profile.guide as PlatformGuideMetadata;
      const title = readString(guide.title);
      const description = readString(guide.description);
      const profileID = readString(guide.profile_id) || platform;
      const docsURL = readString(guide.docs_url);
      const defaultClient = readString(guide.default_client);
      const clientsRaw = guide.clients;
      const copyBlocksRaw = guide.copy_blocks;

      if (!title) {
        errors.push(`${profilePath}.guide.title là bắt buộc.`);
      }
      if (!description) {
        errors.push(`${profilePath}.guide.description là bắt buộc.`);
      }

      const clientIDs = new Set<string>();
      const normalizedClients: PlatformGuideClient[] = [];
      if (clientsRaw !== undefined && !Array.isArray(clientsRaw)) {
        errors.push(`${profilePath}.guide.clients phải là mảng nếu được khai báo.`);
      }
      if (Array.isArray(clientsRaw)) {
        clientsRaw.forEach((clientRaw, clientIndex) => {
          const clientPath = `${profilePath}.guide.clients[${clientIndex}]`;
          if (!isRecord(clientRaw)) {
            errors.push(`${clientPath} phải là object.`);
            return;
          }
          const clientID = readString(clientRaw.id);
          const label = readString(clientRaw.label);
          if (!clientID || !label) {
            errors.push(`${clientPath}.id và ${clientPath}.label là bắt buộc.`);
          } else if (clientIDs.has(clientID)) {
            errors.push(`${clientPath}.id bị trùng: ${clientID}.`);
          } else {
            clientIDs.add(clientID);
          }
          normalizedClients.push({
            ...(clientRaw as PlatformGuideClient),
            id: clientID,
            label,
            os: readStringArray(clientRaw.os),
          });
        });
      }

      if (defaultClient && clientIDs.size > 0 && !clientIDs.has(defaultClient)) {
        warnings.push(
          `${profilePath}.guide.default_client (${defaultClient}) chưa có trong guide.clients.`,
        );
      }

      const blockIDs = new Set<string>();
      const normalizedBlocks: PlatformGuideCopyBlock[] = [];
      if (copyBlocksRaw !== undefined && !Array.isArray(copyBlocksRaw)) {
        errors.push(`${profilePath}.guide.copy_blocks phải là mảng nếu được khai báo.`);
      }
      if (Array.isArray(copyBlocksRaw)) {
        copyBlocksRaw.forEach((blockRaw, blockIndex) => {
          const blockPath = `${profilePath}.guide.copy_blocks[${blockIndex}]`;
          if (!isRecord(blockRaw)) {
            errors.push(`${blockPath} phải là object.`);
            return;
          }
          const blockID = readString(blockRaw.id);
          const clientID = readString(blockRaw.client_id);
          const path = readString(blockRaw.path);
          const contentTemplate = readString(blockRaw.content_template);
          if (!blockID || !clientID || !path || !contentTemplate) {
            errors.push(
              `${blockPath}.id, client_id, path, content_template là bắt buộc.`,
            );
          } else if (blockIDs.has(blockID)) {
            errors.push(`${blockPath}.id bị trùng: ${blockID}.`);
          } else {
            blockIDs.add(blockID);
          }
          if (clientID && clientIDs.size > 0 && !clientIDs.has(clientID)) {
            warnings.push(
              `${blockPath}.client_id (${clientID}) chưa có trong guide.clients.`,
            );
          }
          normalizedBlocks.push({
            ...(blockRaw as PlatformGuideCopyBlock),
            id: blockID,
            client_id: clientID,
            os: readString(blockRaw.os).toLowerCase(),
            path,
            hint: readString(blockRaw.hint),
            language: readString(blockRaw.language),
            content_template: typeof blockRaw.content_template === "string"
              ? blockRaw.content_template
              : contentTemplate,
          });
        });
      }

      normalizedProfiles.push({
        ...profile,
        platform,
        provider_id: providerID,
        provider_name: providerName,
        api_style: apiStyle,
        guide: {
          ...guide,
          profile_id: profileID,
          title,
          description,
          note: readString(guide.note),
          docs_url: docsURL,
          default_client: defaultClient,
          clients: normalizedClients,
          copy_blocks: normalizedBlocks,
        },
      });
    });
  }

  const missingDefaults = defaultPlatformIDs.filter(
    (platform) => !seenPlatforms.has(platform),
  );
  if (missingDefaults.length > 0) {
    warnings.push(
      `Registry đang thiếu default platform: ${missingDefaults.join(
        ", ",
      )}. Nếu cố ý chỉ override một phần thì backend vẫn lưu được, nhưng default baseline nên có đủ 3 nền tảng.`,
    );
  }

  const version = Number(registry.version || 1);
  const normalizedRegistry: PlatformProfileRegistry = {
    ...registry,
    version: Number.isFinite(version) && version > 0 ? version : 1,
    profiles: normalizedProfiles,
  };

  return {
    ok: errors.length === 0,
    isEmpty: false,
    registry: normalizedRegistry,
    errors,
    warnings,
  };
}

export function formatPlatformProfileRegistryInput(raw: string): string | null {
  const validation = validatePlatformProfileRegistryInput(raw);
  if (!validation.ok || !validation.registry) return null;
  return JSON.stringify(validation.registry, null, 2);
}

export function createPlatformProfileTemplate(platform: string): PlatformProfile {
  const normalizedPlatform = normalizePlatformID(platform) || "new-platform";
  const clientID = `${normalizedPlatform}-client`;
  return {
    platform: normalizedPlatform,
    provider_id: `${normalizedPlatform}-provider`,
    provider_name: normalizedPlatform,
    api_style: "openai-compatible",
    guide: {
      profile_id: normalizedPlatform,
      title: `${normalizedPlatform} delivery guide`,
      description: "Operator inserted guide metadata stored in platform_profile_registry.",
      note: "Edit clients and copy_blocks so the user modal can render this platform without hardcoded UI changes.",
      default_client: clientID,
      clients: [
        {
          id: clientID,
          label: `${normalizedPlatform} client`,
          os: ["unix", "windows"],
        },
      ],
      copy_blocks: [
        {
          id: `${normalizedPlatform}-config`,
          client_id: clientID,
          os: "unix",
          path: "Terminal",
          hint: "Default inserted block",
          language: "shell",
          content_template: `export API_BASE_URL="{{api_base_url}}"\nexport API_KEY="{{api_key}}"`,
        },
      ],
    },
  };
}

export function appendPlatformProfileTemplate(raw: string): {
  value: string;
  platform: string;
} | null {
  const trimmed = raw.trim();
  let root: PlatformProfileRegistry;
  if (!trimmed) {
    root = { version: 1, profiles: [] };
  } else {
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (!isRecord(parsed)) return null;
      root = parsed as PlatformProfileRegistry;
    } catch (_error) {
      return null;
    }
  }

  const profiles = Array.isArray(root.profiles) ? [...root.profiles] : [];
  const existingPlatforms = new Set(
    profiles
      .map((profile) => normalizePlatformID(readString(profile.platform)))
      .filter(Boolean),
  );
  let nextPlatform = "new-platform";
  let suffix = 2;
  while (existingPlatforms.has(nextPlatform)) {
    nextPlatform = `new-platform-${suffix}`;
    suffix += 1;
  }

  profiles.push(createPlatformProfileTemplate(nextPlatform));
  const nextRegistry: PlatformProfileRegistry = {
    ...root,
    version: Number(root.version || 1),
    profiles,
  };

  return {
    value: JSON.stringify(nextRegistry, null, 2),
    platform: nextPlatform,
  };
}
