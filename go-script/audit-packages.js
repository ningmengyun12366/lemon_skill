/** Audit packaged invoice verify skill zip contents deterministically. */

import { promises as fs } from "node:fs";
import { createHash } from "node:crypto";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";
import { gunzipSync, inflateRawSync } from "node:zlib";
import { taxAnalysisExternalAssets, createTaxAnalysisAssetManifest } from "./tax-analysis-assets.js";
import { assertArchiveTextPayload } from "./archive-text.js";

const moduleRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.dirname(moduleRoot);
const packageConfigPath = path.join(moduleRoot, "package-config.json");
const taxAnalysisDistName = "tax-analysis";
const taxAnalysisFontAssetPrefix = "assets/fonts/";
const taxAnalysisRequiredFontFiles = [
  "NotoSansSC-Regular.ttf",
  "NotoSansSC-Bold.ttf",
  "OFL.txt",
];
const taxAnalysisReportAssetManifestPath = path.join(
  "tax-analysis",
  "references",
  "report-asset-manifest.json",
);
const EOCD_SIGNATURE = 0x06054b50;
const CENTRAL_DIRECTORY_SIGNATURE = 0x02014b50;
const PLATFORM_TARGETS = [
  {
    binDir: "windows-amd64",
    goos: "windows",
    suffix: ".exe",
  },
  {
    binDir: "linux-amd64",
    goos: "linux",
    suffix: "",
  },
  {
    binDir: "linux-arm64",
    goos: "linux",
    suffix: "",
  },
  {
    binDir: "darwin-amd64",
    goos: "darwin",
    suffix: "",
  },
  {
    binDir: "darwin-arm64",
    goos: "darwin",
    suffix: "",
  },
];

const SKILLS = [
  {
    distName: "batch-invoice-verify",
    command: "batch-invoice-verify",
    installDoc: "batch-invoice-verify-install.md",
    skillFile: "SKILL.md",
    extraPaths: ["README.md"],
  },
  {
    distName: "invoice-reimbursement-assistant",
    command: "invoice-reimbursement-assistant",
    installDoc: "invoice-reimbursement-assistant-install.md",
    skillFile: "SKILL.md",
    extraPaths: ["README.md"],
  },
  {
    distName: "invoice-verify-analysis",
    command: "invoice-verify-analysis",
    installDoc: "invoice-verify-analysis-install.md",
    skillFile: "SKILL.md",
    externalAssets: [
      {
        path: "assets/fonts/NotoSansSC-Regular.otf",
        templateKey: "PDF_FONT_REGULAR",
      },
      {
        path: "assets/fonts/NotoSansSC-Bold.otf",
        templateKey: "PDF_FONT_BOLD",
      },
    ],
    extraPaths: [
      "README.md",
      "assets/font-README.md",
      "assets/font-OFL.txt",
    ],
  },
  {
    distName: "invoice-fetch-and-risk-analysis",
    command: "invoice-assistant",
    installDoc: "invoice-fetch-and-risk-analysis-install.md",
    skillFile: "SKILL.md",
    configPath: "runtime-config.json",
    binaryNames: {
      "windows-amd64": "invoice-assistant_windows_amd64.exe",
      "linux-amd64": "invoice-assistant_linux_amd64",
      "linux-arm64": "invoice-assistant_linux_arm64",
      "darwin-amd64": "invoice-assistant_darwin_amd64",
      "darwin-arm64": "invoice-assistant_darwin_arm64",
    },
    externalAssets: [
      {
        path: "assets/fonts/NotoSansSC-Regular.ttf",
        templateKey: "PDF_FONT_REGULAR",
      },
      {
        path: "assets/fonts/NotoSansSC-Bold.ttf",
        templateKey: "PDF_FONT_BOLD",
      },
    ],
    extraPaths: [
      "README.md",
      "assets/font-OFL.txt",
      "references/common-code-mappings.md",
      "references/common-handle-response-exception.md",
      "references/common-preflight-initialization-check.md",
      "references/common-query-enterprise-profile-by-uscc.md",
      "references/workflow.md",
      "references/field-mapping.md",
      "references/api-contracts.md",
      "references/risk-boundaries.md",
      "references/error-handling.md",
      "references/analyze-invoice-data.md",
      "references/get-summaries.md",
      "references/invoice-fetch-and-risk-analysis.md",
      "references/invoice-risk-analysis.md",
      "references/发票数据统计分析提示词.md",
      "references/发票风险分析提示词.md",
    ],
  },
  {
    distName: "invoice-issue",
    command: "invoice-assistant",
    installDoc: "invoice-issue-install.md",
    skillFile: "SKILL.md",
    configPath: "runtime-config.json",
    binaryNames: {
      "windows-amd64": "invoice-assistant_windows_amd64.exe",
      "linux-amd64": "invoice-assistant_linux_amd64",
      "linux-arm64": "invoice-assistant_linux_arm64",
      "darwin-amd64": "invoice-assistant_darwin_amd64",
      "darwin-arm64": "invoice-assistant_darwin_arm64",
    },
    extraPaths: [
      "README.md",
      "assets/common_dict.json",
      "assets/invoice_preview.html",
      "references/common-code-mappings.md",
      "references/common-handle-response-exception.md",
      "references/common-preflight-initialization-check.md",
      "references/common-query-enterprise-profile-by-uscc.md",
      "references/amount-calculation.md",
      "references/batch-invoice-info-check.md",
      "references/invoice-issue-auth-extend.md",
      "references/invoice-issue.md",
      "references/issue.md",
    ],
  },
  {
    distName: "tax-analysis",
    command: "tax-analysis",
    installDoc: "tax-analysis-install.md",
    skillFile: "SKILL.md",
    configPath: "runtime-config.json",
    externalAssets: taxAnalysisExternalAssets,
    extraPaths: [
      "README.md",
      "THIRD_PARTY_NOTICES.md",
      "licenses/GO-X-IMAGE-BSD-3-CLAUSE.txt",
      "licenses/GO-X-SYS-BSD-3-CLAUSE.txt",
      "licenses/GO-X-TEXT-BSD-3-CLAUSE.txt",
      "licenses/GOFPDI-MIT.txt",
      "licenses/GOLDMARK-MIT.txt",
      "licenses/GOPDF-MIT.txt",
      "licenses/NODE-LICENSE.txt",
      "licenses/OPENTYPE-JS-MIT.txt",
      "licenses/PKG-ERRORS-BSD-2-CLAUSE.txt",
      "licenses/RESVG-WASM-MPL-2.0.txt",
      "licenses/STRING-CODEPOINTAT-MIT.txt",
      "licenses/TINY-INFLATE-MIT.txt",
      "references/api.md",
      "references/chart-renderer-manifest.json",
      "references/asset-download-manifest.json",
      "references/fields.md",
      "references/font-manifest.json",
      "references/report-asset-manifest.json",
      "references/report-data-contract.md",
      "references/report-rules.md",
      "references/report.md",
      "references/third-party-license-manifest.json",
    ],
  },
  {
    distName: "tax-knowledge",
    command: "tax-knowledge",
    installDoc: "tax-knowledge-install.md",
    skillFile: "SKILL.md",
    configPath: "runtime-config.json",
    extraPaths: ["README.md", "references/retrieval.md"],
  },
];

function fail(message) {
  throw new Error(message);
}

async function readPackageConfig() {
  const raw = await fs.readFile(packageConfigPath, "utf8");
  return JSON.parse(raw);
}

function renderTemplate(template, values) {
  return template.replace(/\$\{([A-Za-z0-9_]+)\}/g, (match, key) => {
    if (!Object.hasOwn(values, key)) {
      fail(`package-config 模板变量未定义: ${key}`);
    }
    return values[key];
  });
}

function getBinaryName(skill, target) {
  return skill.binaryNames?.[target.binDir] ?? `${skill.command}${target.suffix}`;
}

function getVersionedBinaryName(skill, target) {
  const binaryName = getBinaryName(skill, target);

  if (target.suffix && binaryName.endsWith(target.suffix)) {
    return `${binaryName.slice(0, -target.suffix.length)}-${skill.version}${target.suffix}`;
  }

  return `${binaryName}-${skill.version}`;
}

function normalizePath(value) {
  return value.replace(/\\/g, "/").replace(/^\.\//, "").replace(/^\//, "");
}

function getExternalAssetName(asset) {
	return path.posix.basename(normalizePath(asset.releasePath ?? asset.path));
}

function getReleaseExternalAssetRelativePath(_skill, asset) {
	return normalizePath(asset.releasePath ?? asset.path);
}

function parseSkillVersion(content, skillName) {
  const match = content.match(/^version:\s*([^\r\n]+)\s*$/m);
  if (!match) {
    fail(`${skillName} 的 SKILL.md 缺少 version 字段。`);
  }
  return match[1].trim().replace(/^['"]|['"]$/g, "");
}

async function loadSkillVersion(skill) {
  const content = await fs.readFile(
    path.join(repoRoot, skill.distName, skill.skillFile),
    "utf8",
  );
  return parseSkillVersion(content, skill.distName);
}

function getReleaseConfig(packageConfig, environment, skill) {
  const environmentConfig = packageConfig.environments?.[environment];
  if (!environmentConfig?.prefix) {
    fail(`package-config 缺少环境 prefix: ${environment}`);
  }
  const publicBaseUrl = String(environmentConfig.publicBaseUrl ?? "")
    .trim()
    .replace(/\/+$/, "");
  if (!publicBaseUrl) {
    fail(`package-config 缺少环境 publicBaseUrl: ${environment}`);
  }
  const values = {
    baseUrl: publicBaseUrl,
    prefix: environmentConfig.prefix,
    skill: skill.distName,
    version: skill.version,
  };
  const binaryUrls = Object.fromEntries(
    PLATFORM_TARGETS.map((target) => {
      const binaryValues = {
        ...values,
        binary: getVersionedBinaryName(skill, target),
        platform: target.binDir,
      };

      return [target.binDir, renderTemplate(packageConfig.urls.bin, binaryValues)];
    }),
  );
  const externalAssetUrls = Object.fromEntries(
    (skill.externalAssets ?? []).map((asset) => {
      const assetPath = getReleaseExternalAssetRelativePath(skill, asset);
      return [
        asset.templateKey,
        renderTemplate(packageConfig.urls.asset, { ...values, asset: assetPath }),
      ];
    }),
  );

  return {
    archiveName: renderTemplate(packageConfig.archiveName, values),
    binaryUrls,
    externalAssetUrls,
    installDocUrl: renderTemplate(packageConfig.urls.installDoc, values),
    prefix: environmentConfig.prefix,
    zipUrl: renderTemplate(packageConfig.urls.zip, values),
  };
}

function parseArgs(argv, configuredEnvironments) {
  let environment = "all";
  let simulatedMissingSpec = "";

  for (let index = 0; index < argv.length; index += 1) {
    const argument = argv[index];

    if (argument === "--env") {
      environment = argv[index + 1] ?? "";
      index += 1;
      continue;
    }

    if (argument === "--simulate-missing") {
      simulatedMissingSpec = argv[index + 1] ?? "";
      index += 1;
      continue;
    }

    fail(`未知参数: ${argument}`);
  }

  if (environment !== "all" && !configuredEnvironments.includes(environment)) {
    fail(`Invalid environment: ${environment}. Expected one of: ${[...configuredEnvironments, "all"].join(", ")}.`);
  }

  if (!simulatedMissingSpec) {
    return { environment, simulatedMissing: null };
  }

  const separatorIndex = simulatedMissingSpec.indexOf("::");

  if (separatorIndex <= 0 || separatorIndex === simulatedMissingSpec.length - 2) {
    fail(
      "--simulate-missing 需要使用 '<zip-relative-path>::<zip-entry-path>' 格式。",
    );
  }

  return {
    environment,
    simulatedMissing: {
      zipRelativePath: simulatedMissingSpec.slice(0, separatorIndex).replace(/\\/g, "/"),
      entryPath: simulatedMissingSpec.slice(separatorIndex + 2).replace(/\\/g, "/"),
    },
  };
}

async function buildAuditPlan(environment, packageConfig) {
  const environments = environment === "all" ? Object.keys(packageConfig.environments ?? {}) : [environment];
  const plans = [];

  for (const currentEnv of environments) {
    for (const skill of SKILLS) {
      const version = await loadSkillVersion(skill);
      const resolvedSkill = { ...skill, version };
      const release = getReleaseConfig(packageConfig, currentEnv, resolvedSkill);
      plans.push({
        ...resolvedSkill,
        environment: currentEnv,
        binaryUrls: release.binaryUrls,
        externalAssetUrls: release.externalAssetUrls,
        installDocRelativePath: `dist/${release.prefix}/${skill.distName}/${skill.installDoc}`,
        installDocUrl: release.installDocUrl,
        prefix: release.prefix,
        skillDocRelativePath: `dist/${release.prefix}/${skill.distName}/${skill.distName}.md`,
        zipRelativePath: `dist/${release.prefix}/${skill.distName}/${release.archiveName}`,
        zipUrl: release.zipUrl,
      });
    }
  }

  return plans;
}

function formatMode(mode) {
  return `0o${mode.toString(8)}`;
}

function findEndOfCentralDirectory(buffer) {
  const minOffset = Math.max(0, buffer.length - 65557);

  for (let offset = buffer.length - 22; offset >= minOffset; offset -= 1) {
    if (buffer.readUInt32LE(offset) === EOCD_SIGNATURE) {
      return offset;
    }
  }

  fail("ZIP 缺少 end of central directory record。");
}

function parseCentralDirectory(buffer) {
  const eocdOffset = findEndOfCentralDirectory(buffer);
  const entryCount = buffer.readUInt16LE(eocdOffset + 10);
  const centralDirectorySize = buffer.readUInt32LE(eocdOffset + 12);
  const centralDirectoryOffset = buffer.readUInt32LE(eocdOffset + 16);
  const entries = [];
  let cursor = centralDirectoryOffset;
  const endOffset = centralDirectoryOffset + centralDirectorySize;

  while (cursor < endOffset) {
    if (buffer.readUInt32LE(cursor) !== CENTRAL_DIRECTORY_SIGNATURE) {
      fail(`central directory header 签名无效，offset=${cursor}`);
    }

    const fileNameLength = buffer.readUInt16LE(cursor + 28);
    const extraLength = buffer.readUInt16LE(cursor + 30);
    const commentLength = buffer.readUInt16LE(cursor + 32);
    const externalAttributes = buffer.readUInt32LE(cursor + 38);
    const fileNameOffset = cursor + 46;
    const fileName = buffer
      .subarray(fileNameOffset, fileNameOffset + fileNameLength)
      .toString("utf8");

    entries.push({
      externalAttributes,
      fileName,
      compression: buffer.readUInt16LE(cursor + 10),
      compressedSize: buffer.readUInt32LE(cursor + 20),
      uncompressedSize: buffer.readUInt32LE(cursor + 24),
      localOffset: buffer.readUInt32LE(cursor + 42),
    });

    cursor += 46 + fileNameLength + extraLength + commentLength;
  }

  if (entries.length !== entryCount) {
    fail(
      `central directory entry 数量不匹配：EOCD=${entryCount} actual=${entries.length}`,
    );
  }

  return entries;
}

function getUnixMode(entry) {
  return (entry.externalAttributes >>> 16) & 0xffff;
}

function isExecutableUnixEntry(entry) {
  const unixMode = getUnixMode(entry);
  const typeBits = unixMode & 0o170000;
  const permissionBits = unixMode & 0o777;
  const regularFileOrUnspecified = typeBits === 0 || typeBits === 0o100000;

  return regularFileOrUnspecified && (permissionBits & 0o111) !== 0;
}

function getExpectedPaths(skill) {
  const prefix = `${skill.distName}/`;
  const configPath = skill.configPath ?? "config/verify-api.json";

  return [
    `${prefix}SKILL.md`,
    `${prefix}${skill.installDoc}`,
    `${prefix}${configPath}`,
    ...(skill.extraPaths ?? []).map((relativePath) => `${prefix}${relativePath}`),
  ];
}

async function loadZipEntries(zipAbsolutePath) {
  const buffer = await fs.readFile(zipAbsolutePath);
  const entries = parseCentralDirectory(buffer);
  for (const entry of entries) {
    if (entry.fileName.endsWith("/")) continue;
    const offset = entry.localOffset;
    if (offset + 30 > buffer.length || buffer.readUInt32LE(offset) !== 0x04034b50) fail("ZIP local header invalid");
    const start = offset + 30 + buffer.readUInt16LE(offset + 26) + buffer.readUInt16LE(offset + 28);
    if (start + entry.compressedSize > buffer.length) fail("ZIP entry is truncated");
    const compressed = buffer.subarray(start, start + entry.compressedSize);
    if (entry.compression !== 0 && entry.compression !== 8) fail("Unsupported ZIP compression");
    entry.payload = entry.compression === 8 ? inflateRawSync(compressed) : compressed;
    if (entry.payload.length !== entry.uncompressedSize) fail("ZIP entry length mismatch");
    try { assertArchiveTextPayload(entry.fileName, entry.payload); }
    catch (error) { entry.binaryError = error.message; }
  }
  return entries;
}

async function auditRenderedInstallDoc(plan, lines) {
  const absolutePath = path.join(repoRoot, plan.installDocRelativePath);
  const content = await fs.readFile(absolutePath, "utf8");

  for (const expected of [
    plan.installDocUrl,
    plan.zipUrl,
    ...Object.values(plan.binaryUrls),
    ...Object.values(plan.externalAssetUrls),
  ]) {
    if (!content.includes(expected)) {
      fail(`${plan.installDocRelativePath} 缺少链接: ${expected}`);
    }
    lines.push(`PASS ${plan.installDocRelativePath} contains ${expected}`);
  }

  for (const expectedText of ["只下载当前平台", "当前 OS/CPU"]) {
    if (!content.includes(expectedText)) {
      fail(`${plan.installDocRelativePath} 缺少当前平台下载说明: ${expectedText}`);
    }
    lines.push(`PASS ${plan.installDocRelativePath} contains ${expectedText}`);
  }
}

async function auditRenderedSkillDoc(plan, lines) {
  const absolutePath = path.join(repoRoot, plan.skillDocRelativePath);
  const content = await fs.readFile(absolutePath, "utf8");
  const expected = plan.installDocUrl;

  if (!content.includes(expected)) {
    fail(`${plan.skillDocRelativePath} 缺少链接: ${expected}`);
  }
  lines.push(`PASS ${plan.skillDocRelativePath} contains ${expected}`);
}

function collectEntries(entries, simulatedMissing, zipRelativePath, lines) {
  const byPath = new Map();

  for (const entry of entries) {
    if (
      simulatedMissing &&
      simulatedMissing.zipRelativePath === zipRelativePath &&
      simulatedMissing.entryPath === entry.fileName
    ) {
      lines.push(`SIMULATE ${zipRelativePath} remove ${entry.fileName}`);
      continue;
    }

    byPath.set(entry.fileName, entry);
  }

  return byPath;
}

function auditZip(plan, entriesByPath, lines) {
  let failed = false;
  let workBuddyLayoutFailed = false;
  const expectedPaths = getExpectedPaths(plan);
  if (plan.distName === taxAnalysisDistName) {
    const entry = entriesByPath.get(plan.distName + "/references/asset-download-manifest.json");
    if (entry?.payload) {
      const expected = createTaxAnalysisAssetManifest(plan, plan.externalAssetUrls);
      if (JSON.stringify(JSON.parse(entry.payload.toString("utf8"))) !== JSON.stringify(expected)) {
        lines.push("FAIL " + plan.zipRelativePath + " asset download manifest mismatch");
        failed = true;
      }
    }
  }
  const binaryPrefix = `${plan.distName}/bin/`;
  const scriptsPrefix = `${plan.distName}/scripts/`;
  const externalAssetPaths = new Set(
    (plan.externalAssets ?? []).map(
      (asset) => `${plan.distName}/${normalizePath(asset.path)}`,
    ),
  );

  lines.push(`ZIP ${plan.zipRelativePath} env=${plan.environment}`);

  for (const entryPath of entriesByPath.keys()) {
    const entry = entriesByPath.get(entryPath);
    if (entry.binaryError) {
      lines.push("FAIL " + plan.zipRelativePath + " " + entry.binaryError);
      failed = true;
    }
    if (plan.distName === taxAnalysisDistName &&
      (entryPath === plan.distName + "/assets" || entryPath.startsWith(plan.distName + "/assets/"))) {
      lines.push("FAIL " + plan.zipRelativePath + " contains assets entry " + entryPath);
      failed = true;
    }
    const normalizedEntryPath = entryPath.replace(/\/+$/, "");
    if (
      normalizedEntryPath &&
      normalizedEntryPath.split("/").length > 3
    ) {
      lines.push(
        `FAIL ${plan.zipRelativePath} exceeds WorkBuddy root/second-level/file layout: ${entryPath}`,
      );
      failed = true;
      workBuddyLayoutFailed = true;
    }

    if (entryPath === `${plan.distName}/bin` || entryPath.startsWith(binaryPrefix)) {
      lines.push(`FAIL ${plan.zipRelativePath} contains externalized binary ${entryPath}`);
      failed = true;
    }

    if (entryPath === `${plan.distName}/scripts` || entryPath.startsWith(scriptsPrefix)) {
      lines.push(`FAIL ${plan.zipRelativePath} contains externalized scripts ${entryPath}`);
      failed = true;
    }

    if (externalAssetPaths.has(entryPath)) {
      lines.push(`FAIL ${plan.zipRelativePath} contains externalized asset ${entryPath}`);
      failed = true;
    }
  }

  if (!workBuddyLayoutFailed) {
    lines.push(
      `PASS ${plan.zipRelativePath} satisfies WorkBuddy root/second-level/file layout`,
    );
  }

  for (const requiredPath of expectedPaths) {
    if (!entriesByPath.has(requiredPath)) {
      lines.push(`FAIL ${plan.zipRelativePath} missing ${requiredPath}`);
      failed = true;
      continue;
    }

    lines.push(`PASS ${plan.zipRelativePath} has ${requiredPath}`);
  }

  return !failed;
}

async function auditReleaseBinaries(plan, lines) {
  let failed = false;

  for (const target of PLATFORM_TARGETS) {
    const relativePath = `dist/${plan.prefix}/${plan.distName}/bin/${target.binDir}/${getVersionedBinaryName(plan, target)}`;
    const absolutePath = path.join(repoRoot, relativePath);
    let stats;

    try {
      stats = await fs.stat(absolutePath);
    } catch {
      lines.push(`FAIL ${relativePath} missing external binary`);
      failed = true;
      continue;
    }

    if (!stats.isFile()) {
      lines.push(`FAIL ${relativePath} is not a file`);
      failed = true;
      continue;
    }

    lines.push(`PASS ${relativePath} exists`);

    if (target.goos !== "windows" && process.platform !== "win32") {
      const mode = stats.mode & 0o777;

      if ((mode & 0o111) === 0) {
        lines.push(`FAIL ${relativePath} non-executable mode=${formatMode(stats.mode)}`);
        failed = true;
        continue;
      }

      lines.push(`PASS ${relativePath} executable mode=${formatMode(stats.mode)}`);
    }
  }

  return !failed;
}

/**
 * Load and validate the tax-analysis font manifest and its source assets.
 *
 * @param {string[]} lines Human-readable audit evidence lines.
 * @returns {Promise<Map<string, {name: string, size: number, sha256: string}>>}
 *   Validated manifest entries keyed by filename.
 */
async function loadTaxAnalysisFontManifest(lines) {
  const manifestPath = path.join(
    repoRoot,
    "tax-analysis",
    "references",
    "font-manifest.json",
  );
  const manifest = JSON.parse(await fs.readFile(manifestPath, "utf8"));
  if (
    manifest.family !== "Noto Sans SC" ||
    manifest.sourceVersion !== "Noto Sans CJK 2.004 (Sans2.004)" ||
    manifest.license !== "SIL Open Font License 1.1" ||
    !Number.isSafeInteger(manifest.sourceSize) ||
    manifest.sourceSize <= 0 ||
    !/^[a-f0-9]{64}$/.test(manifest.sourceSha256 ?? "") ||
    !String(manifest.derivation ?? "").includes("wght=400/700") ||
    !String(manifest.sourceUrl ?? "").includes("/Sans2.004/") ||
    !String(manifest.licenseUrl ?? "").includes("/Sans2.004/LICENSE")
  ) {
    fail("tax-analysis 字体清单缺少固定来源版本或 OFL 1.1 许可证信息");
  }

  const entries = new Map();
  for (const entry of manifest.files ?? []) {
    if (
      !entry ||
      typeof entry.name !== "string" ||
      !Number.isSafeInteger(entry.size) ||
      entry.size <= 0 ||
      !/^[a-f0-9]{64}$/.test(entry.sha256 ?? "") ||
      entries.has(entry.name)
    ) {
      fail("tax-analysis 字体清单存在无效或重复条目");
    }
    entries.set(entry.name, entry);
  }

  for (const filename of taxAnalysisRequiredFontFiles) {
    const expected = entries.get(filename);
    if (!expected) {
      fail(`tax-analysis 字体清单缺少 ${filename}`);
    }
    const sourcePath = path.join(
      repoRoot,
      "tax-analysis",
      "assets",
      "fonts",
      filename,
    );
    const payload = await fs.readFile(sourcePath);
    const actualHash = createHash("sha256").update(payload).digest("hex");
    if (payload.length !== expected.size || actualHash !== expected.sha256) {
      fail(`tax-analysis 源字体资产与清单不一致: ${filename}`);
    }
    lines.push(`PASS tax-analysis source font manifest verified ${filename}`);
  }

  const licenseText = await fs.readFile(
    path.join(repoRoot, "tax-analysis", "assets", "fonts", "OFL.txt"),
    "utf8",
  );
  const notices = await fs.readFile(
    path.join(repoRoot, "tax-analysis", "THIRD_PARTY_NOTICES.md"),
    "utf8",
  );
  for (const requiredText of [
    "SIL OPEN FONT LICENSE Version 1.1",
    "Reserved Font Name 'Source'",
  ]) {
    if (!licenseText.includes(requiredText)) {
      fail(`tax-analysis OFL.txt 缺少许可证声明: ${requiredText}`);
    }
  }
  for (const requiredText of [
    "Noto Sans SC",
    "SIL Open Font License 1.1",
    "assets/fonts/OFL.txt",
    "references/font-manifest.json",
    "Node.js 24.8.0",
    "licenses/NODE-LICENSE.txt",
    "@resvg/resvg-wasm 2.6.2",
    "licenses/RESVG-WASM-MPL-2.0.txt",
    "licenses/GO-X-IMAGE-BSD-3-CLAUSE.txt",
    "licenses/GO-X-SYS-BSD-3-CLAUSE.txt",
    "licenses/GO-X-TEXT-BSD-3-CLAUSE.txt",
    "references/third-party-license-manifest.json",
  ]) {
    if (!notices.includes(requiredText)) {
      fail(`tax-analysis THIRD_PARTY_NOTICES.md 缺少字体声明: ${requiredText}`);
    }
  }
  lines.push("PASS tax-analysis OFL license and third-party notice verified");
  return entries;
}

/**
 * Validate source and separately released Figma-derived assets against the manifest.
 *
 * @param {object} plan Release environment and skill paths.
 * @param {string[]} lines Human-readable audit evidence lines.
 * @returns {Promise<void>}
 */
async function auditTaxAnalysisReportAssets(plan, lines) {
  const manifestAbsolutePath = path.join(
    repoRoot,
    taxAnalysisReportAssetManifestPath,
  );
  const manifest = JSON.parse(await fs.readFile(manifestAbsolutePath, "utf8"));
  if (
    manifest.designFileKey !== "LdaPSdpOUIJZAYy8bJb4Fa" ||
    manifest.designNodes?.cover !== "30:21" ||
    manifest.designNodes?.toc !== "0:663" ||
    manifest.designNodes?.body !== "7:21" ||
    manifest.sourceCanvas !== "1190x1684" ||
    !String(manifest.componentPolicy ?? "").includes("shared design tokens") ||
    !Array.isArray(manifest.assets) ||
    manifest.assets.length === 0
  ) {
    fail("tax-analysis 报告资产清单缺少固定Figma来源信息");
  }

  const seenPaths = new Set();
  for (const asset of manifest.assets) {
    if (
      !asset ||
      typeof asset.path !== "string" ||
      asset.path !== "assets/report-cover.png" ||
      path.posix.basename(asset.path) !== asset.filename ||
      !Number.isSafeInteger(asset.size) ||
      asset.size <= 0 ||
      !/^[a-f0-9]{64}$/.test(asset.sha256 ?? "") ||
      seenPaths.has(asset.path)
    ) {
      fail("tax-analysis 报告资产清单存在无效或重复条目");
    }
    seenPaths.add(asset.path);
    const absolutePath = path.join(
      repoRoot,
      "tax-analysis",
      ...asset.path.split("/"),
    );
    const payload = await fs.readFile(absolutePath);
    const actualHash = createHash("sha256").update(payload).digest("hex");
    if (payload.length !== asset.size || actualHash !== asset.sha256) {
      fail(`tax-analysis 报告资产与清单不一致: ${asset.path}`);
    }
    lines.push(`PASS tax-analysis report asset verified ${asset.path}`);
    const external = plan.externalAssets.find((item) => item.path === asset.path);
    if (!external) fail("报告资产未登记外置下载：" + asset.path);
    const releasePath = path.join(repoRoot, "dist", plan.prefix, plan.distName, external.releasePath);
    const released = await fs.readFile(releasePath);
    if (
      released.length !== asset.size ||
      createHash("sha256").update(released).digest("hex") !== asset.sha256
    ) {
      fail(`tax-analysis 外置报告资产与清单不一致: ${releasePath}`);
    }
    lines.push(`PASS tax-analysis external report asset hash verified ${releasePath}`);
  }
}

async function auditReleaseExternalAssets(plan, lines) {
  let failed = false;
  let rendererManifest = null;
  let fontManifestEntries = null;
  if (plan.distName === taxAnalysisDistName) {
    rendererManifest = JSON.parse(
      await fs.readFile(
        path.join(repoRoot, "tax-analysis", "references", "chart-renderer-manifest.json"),
        "utf8",
      ),
    );
    fontManifestEntries = await loadTaxAnalysisFontManifest(lines);
    await auditTaxAnalysisReportAssets(plan, lines);
  }

  for (const asset of plan.externalAssets ?? []) {
    const releasePath = getReleaseExternalAssetRelativePath(plan, asset);
    const relativePath = `dist/${plan.prefix}/${plan.distName}/${releasePath}`;
    const absolutePath = path.join(repoRoot, relativePath);
    let stats;

    try {
      stats = await fs.stat(absolutePath);
    } catch {
      lines.push(`FAIL ${relativePath} missing external asset`);
      failed = true;
      continue;
    }

    if (!stats.isFile() || stats.size === 0) {
      lines.push(`FAIL ${relativePath} is not a usable external asset`);
      failed = true;
      continue;
    }

    lines.push(`PASS ${relativePath} exists`);
    if (
      plan.distName === taxAnalysisDistName &&
      asset.path.startsWith(taxAnalysisFontAssetPrefix)
    ) {
      const expected = fontManifestEntries?.get(path.basename(releasePath));
      const payload = await fs.readFile(absolutePath);
      const actualHash = createHash("sha256").update(payload).digest("hex");
      if (
        !expected ||
        expected.size !== payload.length ||
        expected.sha256 !== actualHash
      ) {
        lines.push(`FAIL ${relativePath} font manifest mismatch`);
        failed = true;
        continue;
      }
      lines.push(`PASS ${relativePath} font hash verified`);
    }
    if (asset.sha256) {
      const digest = createHash("sha256").update(await fs.readFile(absolutePath)).digest("hex");
      if (digest !== asset.sha256) {
        lines.push("FAIL " + relativePath + " asset SHA-256 mismatch");
        failed = true;
        continue;
      }
    }
    if (asset.path.startsWith("assets/chart-renderer/runtime/")) {
      const platform = asset.platform;
      const expectedArchive = rendererManifest?.runtimeArchives?.[platform];
      const expectedRuntime = rendererManifest?.runtimeFiles?.[platform];
      const payload = await fs.readFile(absolutePath);
      const actualHash = createHash("sha256").update(payload).digest("hex");
      if (
        !expectedArchive ||
        expectedArchive.path !== asset.downloadPath ||
        expectedArchive.size !== payload.length ||
        expectedArchive.sha256 !== actualHash
      ) {
        lines.push(`FAIL ${relativePath} renderer archive manifest mismatch`);
        failed = true;
        continue;
      }
      let runtimePayload;
      try {
        runtimePayload = gunzipSync(payload);
      } catch {
        lines.push(`FAIL ${relativePath} renderer archive cannot be decompressed`);
        failed = true;
        continue;
      }
      const runtimeHash = createHash("sha256").update(runtimePayload).digest("hex");
      if (
        !expectedRuntime ||
        expectedArchive.runtimePath !== expectedRuntime.path ||
        expectedRuntime.size !== runtimePayload.length ||
        expectedRuntime.sha256 !== runtimeHash
      ) {
        lines.push(`FAIL ${relativePath} decompressed renderer runtime mismatch`);
        failed = true;
        continue;
      }
      lines.push(`PASS ${relativePath} renderer archive and runtime hashes verified`);
    }
  }

  return !failed;
}

async function main() {
  const packageConfig = await readPackageConfig();
  const configuredEnvironments = Object.keys(packageConfig.environments ?? {});
  const options = parseArgs(process.argv.slice(2), configuredEnvironments);
  const plans = await buildAuditPlan(options.environment, packageConfig);
  const lines = [];
  let overallPassed = true;

  for (const plan of plans) {
    const zipAbsolutePath = path.join(repoRoot, plan.zipRelativePath);

    try {
      await auditRenderedInstallDoc(plan, lines);
      await auditRenderedSkillDoc(plan, lines);
      await fs.access(zipAbsolutePath);
      lines.push(`PASS ${plan.zipRelativePath} exists`);
    } catch (error) {
      lines.push(
        `FAIL ${plan.zipRelativePath} preflight ${error instanceof Error ? error.message : String(error)}`,
      );
      overallPassed = false;
      continue;
    }

    try {
      const entries = await loadZipEntries(zipAbsolutePath);
      const entriesByPath = collectEntries(
        entries,
        options.simulatedMissing,
        plan.zipRelativePath,
        lines,
      );

      if (!auditZip(plan, entriesByPath, lines)) {
        overallPassed = false;
      }

      if (!(await auditReleaseBinaries(plan, lines))) {
        overallPassed = false;
      }

      if (!(await auditReleaseExternalAssets(plan, lines))) {
        overallPassed = false;
      }
    } catch (error) {
      lines.push(
        `FAIL ${plan.zipRelativePath} audit-error ${error instanceof Error ? error.message : String(error)}`,
      );
      overallPassed = false;
    }
  }

  lines.push(overallPassed ? "PASS audit complete" : "FAIL audit complete");

  for (const line of lines) {
    console.log(line);
  }

  if (!overallPassed) {
    process.exitCode = 1;
  }
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}

export { auditZip, loadZipEntries };
