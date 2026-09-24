/** Package invoice verify Go skills. */

import { ZipArchive } from "archiver";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import { createWriteStream } from "node:fs";
import { promisify } from "node:util";
import path from "node:path";
import process from "node:process";
import { fileURLToPath, pathToFileURL } from "node:url";
import { constants as zlibConstants, gunzipSync, gzipSync } from "node:zlib";
import {
  loadTaxAnalysisReportVersions,
  renderTaxAnalysisReportVersions,
  writeTaxAnalysisBinaryManifest,
  validateTaxAnalysisPackageDocs,
} from "./tax-analysis-build.js";
import { taxAnalysisExternalAssets, createTaxAnalysisAssetManifest, taxAnalysisAssetDownloadRows, validateTaxAnalysisAssetInventory } from "./tax-analysis-assets.js";
import { assertArchiveTextPayload } from "./archive-text.js";

const execFileAsync = promisify(execFile);

const SKILL_FILE = "SKILL.md";

const moduleRoot = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.dirname(moduleRoot);
const commonRoot = path.join(moduleRoot, "lib", "invoice-verify-common");
const invoiceAssistantRoot = path.join(moduleRoot, "lib", "invoice-assistant-go");
const taxAnalysisChartRendererRoot = path.join(
  moduleRoot,
  "lib",
  "tax-analysis",
  "chart-renderer",
);
const configRoot = path.join(moduleRoot, "config");
const packageConfigPath = path.join(moduleRoot, "package-config.json");
const skillVersionConstantsPath = path.join(moduleRoot, "lib", "skillupdate", "versions.go");
const distRoot = path.join(repoRoot, "dist");
const RUNTIME_CONFIG_FILE = "runtime-config.json";
const NODE_RUNTIME_CACHE_DIR = path.join(
  repoRoot,
  "node_modules",
  ".cache",
  "tax-analysis-node-runtime",
);
const OPEN_API_SKILL_BASE_URL_KEY = "open_api_skill_base_url";
const OPEN_API_PAGE_URL_KEY = "open_api_page_url";
const OPEN_API_CONTACT_SERVICE_URL_KEY = "open_api_contact_service_url";
const PLATFORM_TARGETS = [
  {
    binDir: "windows-amd64",
    goarch: "amd64",
    goos: "windows",
    suffix: ".exe",
  },
  {
    binDir: "linux-amd64",
    goarch: "amd64",
    goos: "linux",
    suffix: "",
  },
  {
    binDir: "linux-arm64",
    goarch: "arm64",
    goos: "linux",
    suffix: "",
  },
  {
    binDir: "darwin-amd64",
    goarch: "amd64",
    goos: "darwin",
    suffix: "",
  },
  {
    binDir: "darwin-arm64",
    goarch: "arm64",
    goos: "darwin",
    suffix: "",
  },
];

const goSkillDir = {
  "batch-invoice-verify": {
    name: "柠檬云发票查验查重及台账",
    distName: "batch-invoice-verify",
    command: "batch-invoice-verify",
    installDoc: "batch-invoice-verify-install.md",
    versionConstant: "BatchInvoiceVerifyVersion",
  },
  "invoice-reimbursement-assistant": {
    name: "柠檬云发票整理、查验及报销助手",
    distName: "invoice-reimbursement-assistant",
    command: "invoice-reimbursement-assistant",
    installDoc: "invoice-reimbursement-assistant-install.md",
    versionConstant: "InvoiceReimbursementAssistantVersion",
  },
  "invoice-verify-analysis": {
    name: "柠檬云企业发票核验与征信分析",
    distName: "invoice-verify-analysis",
    command: "invoice-verify-analysis",
    installDoc: "invoice-verify-analysis-install.md",
    versionConstant: "InvoiceVerifyAnalysisVersion",
    assets: [
      "assets/fonts/NotoSansSC-Regular.otf",
      "assets/fonts/NotoSansSC-Bold.otf",
      { source: "assets/fonts/README.md", target: "assets/font-README.md" },
      { source: "assets/fonts/OFL.txt", target: "assets/font-OFL.txt" },
    ],
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
  },
  "invoice-fetch-and-risk-analysis": {
    name: "柠檬云发票获取及风险分析",
    distName: "invoice-fetch-and-risk-analysis",
    command: "invoice-assistant",
    entryPackage: "invoice-assistant",
    installDoc: "invoice-fetch-and-risk-analysis-install.md",
    versionConstant: "InvoiceFetchAndRiskAnalysisVersion",
    capabilityCode: "invoice_batch_get",
    runtimeConfig: true,
    assetRoot: "invoice-assistant-go",
    assets: [
      "assets/fonts/NotoSansSC-Regular.ttf",
      "assets/fonts/NotoSansSC-Bold.ttf",
      { source: "assets/fonts/OFL.txt", target: "assets/font-OFL.txt" },
    ],
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
    binaryNameByPlatform: {
      "windows-amd64": "invoice-assistant_windows_amd64.exe",
      "linux-amd64": "invoice-assistant_linux_amd64",
      "linux-arm64": "invoice-assistant_linux_arm64",
      "darwin-amd64": "invoice-assistant_darwin_amd64",
      "darwin-arm64": "invoice-assistant_darwin_arm64",
    },
  },
  "invoice-issue": {
    name: "柠檬云全场景开票",
    distName: "invoice-issue",
    command: "invoice-assistant",
    entryPackage: "invoice-assistant",
    installDoc: "invoice-issue-install.md",
    versionConstant: "InvoiceIssueVersion",
    capabilityCode: "invoice_issue",
    runtimeConfig: true,
    assetRoot: "invoice-assistant-go",
    assets: [
      { source: "assets/issue/common_dict.json", target: "assets/common_dict.json" },
      { source: "assets/issue/invoice_preview.html", target: "assets/invoice_preview.html" },
    ],
    binaryNameByPlatform: {
      "windows-amd64": "invoice-assistant_windows_amd64.exe",
      "linux-amd64": "invoice-assistant_linux_amd64",
      "linux-arm64": "invoice-assistant_linux_arm64",
      "darwin-amd64": "invoice-assistant_darwin_amd64",
      "darwin-arm64": "invoice-assistant_darwin_arm64",
    },
  },
  "tax-knowledge": {
    name: "柠檬云AI财税专家咨询",
    distName: "tax-knowledge",
    command: "tax-knowledge",
    installDoc: "tax-knowledge-install.md",
    versionConstant: "TaxKnowledgeVersion",
    capabilityCode: "tax_knowledge",
    runtimeConfig: true,
  },
  "tax-analysis": {
    name: "柠檬云企业税务风险分析",
    distName: "tax-analysis",
    command: "tax-analysis",
    installDoc: "tax-analysis-install.md",
    versionConstant: "TaxAnalysisVersion",
    capabilityCode: "tax_analysis",
    runtimeConfig: true,
    chartRenderer: true,
    assetRoot: "skill",
    assets: [
      "assets/fonts/NotoSansSC-Regular.ttf",
      "assets/fonts/NotoSansSC-Bold.ttf",
      "assets/fonts/OFL.txt",
      "assets/report-cover.png",
    ],
    externalAssets: taxAnalysisExternalAssets,
  },
};

function getEntryPackage(skillConfig) {
  return skillConfig.entryPackage ?? skillConfig.command;
}

function getBinaryName(skillConfig, target) {
  return skillConfig.binaryNameByPlatform?.[target.binDir] ?? `${skillConfig.command}${target.suffix}`;
}

function getTemplateKeyForPlatform(target) {
  return target.binDir.toUpperCase().replaceAll("-", "_");
}

function getVersionedBinaryName(skillConfig, target) {
  const binaryName = getBinaryName(skillConfig, target);
  const version = skillConfig.version;

  if (!version) {
    throw new Error(`${skillConfig.distName} 缺少 version，无法生成外置二进制文件名。`);
  }

  if (target.suffix && binaryName.endsWith(target.suffix)) {
    return `${binaryName.slice(0, -target.suffix.length)}-${version}${target.suffix}`;
  }

  return `${binaryName}-${version}`;
}

function getLocalBinaryRelativePath(skillConfig, target) {
  return path.posix.join("bin", target.binDir, getBinaryName(skillConfig, target));
}

function getReleaseBinaryRelativePath(skillConfig, target) {
  return path.posix.join(
    "bin",
    target.binDir,
    getVersionedBinaryName(skillConfig, target),
  );
}

function getConfigOutputPath(skillConfig) {
  return skillConfig.runtimeConfig ? RUNTIME_CONFIG_FILE : "config/verify-api.json";
}

function getAssetRoot(skillConfig, skillSourceDir) {
  if (skillConfig.assetRoot === "invoice-assistant-go") {
    return invoiceAssistantRoot;
  }
  if (skillConfig.assetRoot === "skill") {
    return skillSourceDir;
  }

  return commonRoot;
}

function getLocalExternalAssetRelativePath(asset) {
  return normalizePath(asset.path);
}

function getExternalAssetName(asset) {
	const releasePath = normalizePath(asset.releasePath ?? asset.path);
	return path.posix.basename(releasePath);
}

function getReleaseExternalAssetRelativePath(_skillConfig, asset) {
	return normalizePath(asset.releasePath ?? asset.path);
}

function normalizePath(value) {
  return value.replace(/\\/g, "/").replace(/^\.\//, "").replace(/^\//, "");
}

function escapeRegex(value) {
  return value.replace(/[|\\{}()[\]^$+?.]/g, "\\$&");
}

function globToRegexSource(pattern) {
  let source = "";

  for (let index = 0; index < pattern.length; index += 1) {
    const current = pattern[index];

    if (current === "*") {
      if (pattern[index + 1] === "*") {
        source += ".*";
        index += 1;
      } else {
        source += "[^/]*";
      }
      continue;
    }

    if (current === "?") {
      source += ".";
      continue;
    }

    if (current === "[") {
      const classEnd = pattern.indexOf("]", index + 1);

      if (classEnd !== -1) {
        source += pattern.slice(index, classEnd + 1);
        index = classEnd;
        continue;
      }
    }

    source += escapeRegex(current);
  }

  return source;
}

function compileIgnorePattern(pattern) {
  const trimmed = pattern.trim();

  if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("!")) {
    return null;
  }

  const directoryOnly = trimmed.endsWith("/");
  const normalized = normalizePath(
    directoryOnly ? trimmed.slice(0, -1) : trimmed,
  );

  if (!normalized) {
    return null;
  }

  const hasSlash = normalized.includes("/");
  const prefix = hasSlash ? "^" : "(^|.*/)";
  const suffix = directoryOnly ? "(/.*)?$" : "$";

  return {
    raw: trimmed,
    regex: new RegExp(`${prefix}${globToRegexSource(normalized)}${suffix}`),
  };
}

async function exists(targetPath) {
  try {
    await fs.access(targetPath);
    return true;
  } catch {
    return false;
  }
}

async function readIgnoreRules() {
  const gitignorePath = path.join(repoRoot, ".gitignore");

  if (!(await exists(gitignorePath))) {
    return [];
  }

  const content = await fs.readFile(gitignorePath, "utf8");

  return content
    .split(/\r?\n/)
    .map((line) => compileIgnorePattern(line))
    .filter(Boolean);
}

function shouldIgnoreFactory(ignoreRules) {
  return (sourcePath) => {
    const relativePath = normalizePath(path.relative(repoRoot, sourcePath));

    if (!relativePath) {
      return false;
    }

    return ignoreRules.some((rule) => rule.regex.test(relativePath));
  };
}

async function ensureDirectory(targetPath) {
  await fs.mkdir(targetPath, { recursive: true });
}

async function readPackageConfig() {
  const raw = await fs.readFile(packageConfigPath, "utf8");
  return JSON.parse(raw);
}

async function readEnvironmentConfig(environment) {
  const raw = await fs.readFile(path.join(configRoot, `${environment}.json`), "utf8");
  return JSON.parse(raw);
}

function renderTemplate(template, values) {
  return template.replace(/\$\{([A-Za-z0-9_]+)\}/g, (match, key) => {
    if (!Object.hasOwn(values, key)) {
      throw new Error(`package-config 模板变量未定义: ${key}`);
    }
    return values[key];
  });
}

function parseSkillVersion(content, skillName) {
  const match = content.match(/^version:\s*([^\r\n]+)\s*$/m);
  if (!match) {
    throw new Error(`${skillName} 的 SKILL.md 缺少 version 字段。`);
  }
  return match[1].trim().replace(/^['"]|['"]$/g, "");
}

function parseInstallDocVersion(content, skillName) {
  const matches = [...content.matchAll(/^- version:\s*(\S+)\s*$/gm)];
  if (matches.length !== 1) {
    throw new Error(`${skillName} 安装文档必须且只能包含一行 "- version: <SemVer>"。`);
  }
  return matches[0][1];
}

function parseGoVersionConstants(content) {
  return new Map(
    [...content.matchAll(/^\s*([A-Za-z][A-Za-z0-9]*Version)\s*=\s*"([^"]+)"\s*$/gm)].map(
      (match) => [match[1], match[2]],
    ),
  );
}

/**
 * Resolve one release version and reject drift between the skill, install document, and Go binary.
 */
function validateSkillVersionValues(
  skillConfig,
  skillMarkdown,
  installMarkdown,
  versionConstantsSource,
) {
  const constants = parseGoVersionConstants(versionConstantsSource);
  const versions = {
    "SKILL.md": parseSkillVersion(skillMarkdown, skillConfig.distName),
    "安装文档": parseInstallDocVersion(installMarkdown, skillConfig.distName),
    "脚本常量": constants.get(skillConfig.versionConstant),
  };
  if (!versions["脚本常量"]) {
    throw new Error(`${skillConfig.distName} 缺少脚本版本常量 ${skillConfig.versionConstant}。`);
  }
  if (new Set(Object.values(versions)).size !== 1) {
    throw new Error(
      `${skillConfig.distName} 版本不一致：${Object.entries(versions)
        .map(([source, version]) => `${source}=${version}`)
        .join(", ")}`,
    );
  }
  return versions["SKILL.md"];
}

async function validateSkillVersionSources(skillConfig, skillMarkdown) {
  const installMarkdown = await fs.readFile(
    path.join(repoRoot, skillConfig.distName, skillConfig.installDoc),
    "utf8",
  );
  const versionConstantsSource = await fs.readFile(skillVersionConstantsPath, "utf8");
  return validateSkillVersionValues(
    skillConfig,
    skillMarkdown,
    installMarkdown,
    versionConstantsSource,
  );
}

function getReleaseConfig(packageConfig, environment, environmentConfig, skillConfig) {
  const releaseEnvironment = packageConfig.environments?.[environment];
  if (!releaseEnvironment?.prefix) {
    throw new Error(`package-config 缺少环境 prefix: ${environment}`);
  }
  const publicBaseUrl = String(releaseEnvironment.publicBaseUrl ?? "")
    .trim()
    .replace(/\/+$/, "");
  if (!publicBaseUrl) {
    throw new Error(`package-config 缺少环境 publicBaseUrl: ${environment}`);
  }
  const values = {
    baseUrl: publicBaseUrl,
    prefix: releaseEnvironment.prefix,
    skill: skillConfig.distName,
    version: skillConfig.version,
  };
  const binaryUrls = Object.fromEntries(
    PLATFORM_TARGETS.map((target) => {
      const binaryValues = {
        ...values,
        binary: getVersionedBinaryName(skillConfig, target),
        platform: target.binDir,
      };

      return [target.binDir, renderTemplate(packageConfig.urls.bin, binaryValues)];
    }),
  );
  const externalAssetUrls = Object.fromEntries(
    (skillConfig.externalAssets ?? []).map((asset) => {
      const assetPath = getReleaseExternalAssetRelativePath(skillConfig, asset);
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
    openApiContactServiceUrl: requiredEnvironmentValue(
      environmentConfig,
      environment,
      OPEN_API_CONTACT_SERVICE_URL_KEY,
    ),
    openApiPageUrl: requiredEnvironmentValue(
      environmentConfig,
      environment,
      OPEN_API_PAGE_URL_KEY,
    ),
    openApiSkillBaseUrl: requiredEnvironmentValue(
      environmentConfig,
      environment,
      OPEN_API_SKILL_BASE_URL_KEY,
    ),
    prefix: releaseEnvironment.prefix,
    releaseDir: path.join(distRoot, releaseEnvironment.prefix, skillConfig.distName),
    zipUrl: renderTemplate(packageConfig.urls.zip, values),
  };
}

function renderPackageText(content, releaseConfig, skillConfig) {
  if (skillConfig.reportVersions) {
    content = renderTaxAnalysisReportVersions(content, skillConfig.reportVersions);
  }
  let rendered = content
    .replaceAll("{{INSTALL_DOC_URL}}", releaseConfig.installDocUrl)
    .replaceAll("{{ZIP_URL}}", releaseConfig.zipUrl)
    .replaceAll("{{SKILL_VERSION}}", skillConfig.version)
    .replaceAll("{{SKILL_NAME}}", skillConfig.distName)
    .replaceAll("{{PUBLISH_PREFIX}}", releaseConfig.prefix)
    .replaceAll("{{TAX_ASSET_DOWNLOAD_ROWS}}", skillConfig.distName === "tax-analysis" ? taxAnalysisAssetDownloadRows(releaseConfig.externalAssetUrls) : "")
    .replaceAll("{{OPEN_API_SKILL_BASE_URL}}", releaseConfig.openApiSkillBaseUrl)
    .replaceAll("{{OPEN_API_PAGE_URL}}", releaseConfig.openApiPageUrl)
    .replaceAll(
      "{{OPEN_API_CONTACT_SERVICE_URL}}",
      releaseConfig.openApiContactServiceUrl,
    );

  for (const target of PLATFORM_TARGETS) {
    const key = getTemplateKeyForPlatform(target);
    rendered = rendered
      .replaceAll(`{{BIN_URL_${key}}}`, releaseConfig.binaryUrls[target.binDir])
      .replaceAll(`{{BIN_LOCAL_NAME_${key}}}`, getBinaryName(skillConfig, target))
      .replaceAll(`{{BIN_RELEASE_NAME_${key}}}`, getVersionedBinaryName(skillConfig, target))
      .replaceAll(`{{BIN_LOCAL_PATH_${key}}}`, getLocalBinaryRelativePath(skillConfig, target))
      .replaceAll(`{{BIN_RELEASE_PATH_${key}}}`, getReleaseBinaryRelativePath(skillConfig, target));
    if (skillConfig.binarySHA256?.[target.binDir]) {
      rendered = rendered.replaceAll(`{{BIN_SHA256_${key}}}`, skillConfig.binarySHA256[target.binDir]);
    }
  }

  for (const asset of skillConfig.externalAssets ?? []) {
    const key = asset.templateKey;
    const localPath = getLocalExternalAssetRelativePath(asset);
    const releasePath = getReleaseExternalAssetRelativePath(skillConfig, asset);
    rendered = rendered
      .replaceAll(`{{ASSET_URL_${key}}}`, releaseConfig.externalAssetUrls[key])
      .replaceAll(`{{ASSET_LOCAL_PATH_${key}}}`, localPath)
      .replaceAll(`{{ASSET_RELEASE_PATH_${key}}}`, releasePath)
      .replaceAll(`{{ASSET_LOCAL_NAME_${key}}}`, path.posix.basename(localPath))
      .replaceAll(`{{ASSET_RELEASE_NAME_${key}}}`, path.posix.basename(releasePath));
  }

  return rendered;
}

async function copyEntry(sourcePath, targetPath, shouldIgnore) {
  if (!(await exists(sourcePath)) || shouldIgnore(sourcePath)) {
    return;
  }

  const stats = await fs.lstat(sourcePath);

  if (stats.isDirectory()) {
    await ensureDirectory(targetPath);
    const children = await fs.readdir(sourcePath, { withFileTypes: true });

    for (const child of children) {
      await copyEntry(
        path.join(sourcePath, child.name),
        path.join(targetPath, child.name),
        shouldIgnore,
      );
    }
    return;
  }

  await ensureDirectory(path.dirname(targetPath));
  await fs.copyFile(sourcePath, targetPath);
}

async function copyRenderedText(sourcePath, targetPath, shouldIgnore, render) {
  if (!(await exists(sourcePath)) || shouldIgnore(sourcePath)) {
    return;
  }
  const content = await fs.readFile(sourcePath, "utf8");
  await ensureDirectory(path.dirname(targetPath));
  await fs.writeFile(targetPath, render(content), "utf8");
}

async function findGoExecutable() {
  const candidates = [process.env.GO?.trim(), "go"].filter(Boolean);

  for (const candidate of candidates) {
    try {
      await execFileAsync(candidate, ["version"]);
      return candidate;
    } catch {
      // Try the next configured lookup mechanism.
    }
  }

  throw new Error(
    "未找到 Go 可执行文件；请通过 GO 环境变量指定 go 可执行文件，或将 go 加入 PATH。",
  );
}

function getBuildCacheKey(skillConfig, target) {
  return `${getEntryPackage(skillConfig)}:${target.goos}:${target.goarch}`;
}

async function buildPlatformBinary(
  goExecutable,
  skillConfig,
  sceneOutputDir,
  target,
  binaryCache,
) {
  const binDir = path.join(sceneOutputDir, "bin", target.binDir);
  const outputPath = path.join(binDir, getBinaryName(skillConfig, target));
  const cacheKey = getBuildCacheKey(skillConfig, target);
  const cachedOutputPath = binaryCache.get(cacheKey);
  const packagePath = `./skill/${getEntryPackage(skillConfig)}`;
  const env = {
    ...process.env,
    CGO_ENABLED: "0",
    GOARCH: target.goarch,
    GOOS: target.goos,
  };

  await ensureDirectory(binDir);
  if (cachedOutputPath) {
    await fs.copyFile(cachedOutputPath, outputPath);
    if (target.goos !== "windows") {
      await fs.chmod(outputPath, 0o755);
    }
    return;
  }

  await execFileAsync(
    goExecutable,
    [
      "build",
      "-trimpath",
      "-ldflags",
      "-s -w",
      "-o",
      outputPath,
      packagePath,
    ],
    { cwd: moduleRoot, env },
  );

  if (target.goos !== "windows") {
    await fs.chmod(outputPath, 0o755);
  }
  binaryCache.set(cacheKey, outputPath);
}

async function buildPlatformBinaries(skillConfig, sceneOutputDir, binaryCache) {
  const goExecutable = await findGoExecutable();
  const currentBinaryCache = binaryCache ?? new Map();

  for (const target of PLATFORM_TARGETS) {
    await buildPlatformBinary(goExecutable, skillConfig, sceneOutputDir, target, currentBinaryCache);
  }
}

function flattenChartRendererSource(content) {
  const replacements = [
    ['from "@resvg/resvg-wasm"', 'from "./resvg-wasm.mjs"'],
    ['from "opentype.js"', 'from "./opentype.cjs"'],
    ['from "./renderer.constants.js"', 'from "./chart-renderer-constants.mjs"'],
    [
      'fileURLToPath(import.meta.resolve("@resvg/resvg-wasm/index_bg.wasm"))',
      'fileURLToPath(new URL("./resvg-wasm-bg.wasm", import.meta.url))',
    ],
  ];
  let rendered = content;
  for (const [source, target] of replacements) {
    if (!rendered.includes(source)) {
      throw new Error(`组件库渲染入口缺少待扁平化内容：${source}`);
    }
    rendered = rendered.replace(source, target);
  }
  return rendered;
}

async function buildChartRendererAssets(skillConfig, sceneOutputDir) {
  if (!skillConfig.chartRenderer) {
    return;
  }
  const manifest = JSON.parse(
    await fs.readFile(
      path.join(repoRoot, "tax-analysis", "references", "chart-renderer-manifest.json"),
      "utf8",
    ),
  );
  const rendererSources = new Map([
    ["chart-renderer-package.json", { source: path.join(taxAnalysisChartRendererRoot, "package.json") }],
    ["chart-renderer-constants.mjs", { source: path.join(taxAnalysisChartRendererRoot, "renderer.constants.js") }],
    [
      "chart-renderer.mjs",
      {
        source: path.join(taxAnalysisChartRendererRoot, "renderer.js"),
        render: (payload) => Buffer.from(flattenChartRendererSource(payload.toString("utf8")), "utf8"),
      },
    ],
    ["resvg-wasm.mjs", { source: path.join(repoRoot, "node_modules", "@resvg", "resvg-wasm", "index.mjs") }],
    ["resvg-wasm-bg.wasm", { source: path.join(repoRoot, "node_modules", "@resvg", "resvg-wasm", "index_bg.wasm") }],
    ["resvg-wasm-package.json", { source: path.join(repoRoot, "node_modules", "@resvg", "resvg-wasm", "package.json") }],
    ["opentype.cjs", { source: path.join(repoRoot, "node_modules", "opentype.js", "dist", "opentype.js") }],
    ["opentype-package.json", { source: path.join(repoRoot, "node_modules", "opentype.js", "package.json") }],
  ]);

  for (const [outputName, rendererSource] of rendererSources) {
    const expected = manifest.rendererFiles?.[outputName];
    const sourcePayload = await fs.readFile(rendererSource.source);
    const payload = rendererSource.render
      ? rendererSource.render(sourcePayload)
      : sourcePayload;
    verifyPinnedPayload(payload, expected, `组件库渲染文件 ${outputName}`);
    const outputPath = path.join(sceneOutputDir, "assets", outputName);
    await ensureDirectory(path.dirname(outputPath));
    await fs.writeFile(outputPath, payload);
  }

  for (const target of PLATFORM_TARGETS) {
    const expected = manifest.runtimeFiles?.[target.binDir];
    if (!expected) {
      throw new Error(`便携 Node.js 运行时清单缺少平台：${target.binDir}`);
    }
    const source = manifest.sources?.[expected.source];
    if (!source) {
      throw new Error(`便携 Node.js 运行时清单缺少来源：${expected.source}`);
    }
    const sourcePayload = await downloadVerifiedAsset(source);
    const runtimePayload = expected.archiveEntry
      ? extractTarEntry(gunzipSync(sourcePayload), expected.archiveEntry)
      : sourcePayload;
    verifyPinnedPayload(runtimePayload, expected, `便携 Node.js 运行时 ${target.binDir}`);
    const outputPath = path.join(
      sceneOutputDir,
      ...expected.path.split("/"),
    );
    await ensureDirectory(path.dirname(outputPath));
    await fs.writeFile(outputPath, runtimePayload);
    if (target.goos !== "windows") {
      await fs.chmod(outputPath, 0o755);
    }
  }

  await buildTaxAnalysisLicenseBundle(sceneOutputDir);
}

function verifyPinnedPayload(payload, expected, label) {
  const actualHash = createHash("sha256").update(payload).digest("hex");
  if (!expected || expected.size !== payload.length || expected.sha256 !== actualHash) {
    throw new Error(
      `${label}清单不匹配：size=${payload.length} sha256=${actualHash}`,
    );
  }
}

async function downloadVerifiedAsset(source) {
  const cachePath = path.join(
    NODE_RUNTIME_CACHE_DIR,
    `${source.sha256}-${path.basename(new URL(source.url).pathname)}`,
  );
  try {
    const cached = await fs.readFile(cachePath);
    verifyPinnedPayload(cached, source, `缓存下载文件 ${source.url}`);
    return cached;
  } catch {
    await fs.rm(cachePath, { force: true });
  }

  let lastError;
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    try {
      const response = await fetch(source.url, {
        signal: AbortSignal.timeout(10 * 60 * 1000),
      });
      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`);
      }
      const payload = Buffer.from(await response.arrayBuffer());
      verifyPinnedPayload(payload, source, `下载文件 ${source.url}`);
      await ensureDirectory(path.dirname(cachePath));
      const temporaryPath = `${cachePath}.${process.pid}.${attempt}.tmp`;
      await fs.writeFile(temporaryPath, payload);
      await fs.rename(temporaryPath, cachePath);
      return payload;
    } catch (error) {
      lastError = error;
    }
  }
  throw new Error(
    `下载固定运行资产失败：${source.url}：${lastError instanceof Error ? lastError.message : String(lastError)}`,
  );
}

function extractTarEntry(archive, expectedEntry) {
  let offset = 0;
  while (offset + 512 <= archive.length) {
    const header = archive.subarray(offset, offset + 512);
    if (header.every((value) => value === 0)) {
      break;
    }
    const name = readTarText(header.subarray(0, 100));
    const prefix = readTarText(header.subarray(345, 500));
    const fullName = prefix ? `${prefix}/${name}` : name;
    const sizeText = readTarText(header.subarray(124, 136)).trim();
    const size = Number.parseInt(sizeText || "0", 8);
    if (!Number.isFinite(size) || size < 0) {
      throw new Error(`Node.js 归档包含无效条目大小：${fullName}`);
    }
    const payloadStart = offset + 512;
    const payloadEnd = payloadStart + size;
    if (payloadEnd > archive.length) {
      throw new Error(`Node.js 归档条目越界：${fullName}`);
    }
    if (fullName === expectedEntry) {
      return Buffer.from(archive.subarray(payloadStart, payloadEnd));
    }
    offset = payloadStart + Math.ceil(size / 512) * 512;
  }
  throw new Error(`Node.js 归档缺少条目：${expectedEntry}`);
}

function readTarText(buffer) {
  const terminator = buffer.indexOf(0);
  return buffer.subarray(0, terminator < 0 ? buffer.length : terminator).toString("utf8");
}

async function buildTaxAnalysisLicenseBundle(sceneOutputDir) {
  const manifest = JSON.parse(
    await fs.readFile(
      path.join(repoRoot, "tax-analysis", "references", "third-party-license-manifest.json"),
      "utf8",
    ),
  );
  const goExecutable = await findGoExecutable();
  const { stdout: goModuleCacheOutput } = await execFileAsync(goExecutable, ["env", "GOMODCACHE"]);
  const goModuleCache = goModuleCacheOutput.trim();

  for (const [filename, expected] of Object.entries(manifest.files ?? {})) {
    let payload;
    if (expected.sourceUrl) {
      payload = await downloadVerifiedAsset({
        url: expected.sourceUrl,
        size: expected.size,
        sha256: expected.sha256,
      });
    } else if (expected.repoPath) {
      payload = await fs.readFile(path.join(repoRoot, ...expected.repoPath.split("/")));
    } else if (expected.goModule && expected.version && expected.modulePath) {
      const segments = expected.goModule.split("/");
      segments[segments.length - 1] += `@${expected.version}`;
      payload = await fs.readFile(
        path.join(goModuleCache, ...segments, ...expected.modulePath.split("/")),
      );
    } else {
      throw new Error(`第三方许可证清单缺少来源：${filename}`);
    }
    verifyPinnedPayload(payload, expected, `第三方许可证 ${filename}`);
    const outputPath = path.join(sceneOutputDir, "licenses", filename);
    await ensureDirectory(path.dirname(outputPath));
    await fs.writeFile(outputPath, payload);
  }
}

function getPlatformBinaryPath(sceneOutputDir, skillConfig, target) {
  return path.join(
    sceneOutputDir,
    "bin",
    target.binDir,
    getBinaryName(skillConfig, target),
  );
}

async function copyReleaseBinaries(sceneOutputDir, skillConfig, releaseConfig) {
  const releaseBinDir = path.join(releaseConfig.releaseDir, "bin");

  await fs.rm(releaseBinDir, { recursive: true, force: true });

  for (const target of PLATFORM_TARGETS) {
    const sourcePath = getPlatformBinaryPath(sceneOutputDir, skillConfig, target);
    const targetPath = path.join(
      releaseConfig.releaseDir,
      "bin",
      target.binDir,
      getVersionedBinaryName(skillConfig, target),
    );

    await ensureDirectory(path.dirname(targetPath));
    await fs.copyFile(sourcePath, targetPath);

    if (target.goos !== "windows") {
      await fs.chmod(targetPath, 0o755);
    }
  }
}

async function copyReleaseExternalAssets(sceneOutputDir, skillConfig, releaseConfig) {
  const externalAssets = skillConfig.externalAssets ?? [];
	let chartRendererManifest = null;
	if (externalAssets.some((asset) => asset.compression === "gzip")) {
		chartRendererManifest = JSON.parse(
			await fs.readFile(
				path.join(repoRoot, "tax-analysis", "references", "chart-renderer-manifest.json"),
				"utf8",
			),
		);
	}
  const releaseAssetDirs = new Set(
    externalAssets.map((asset) =>
      path.dirname(
        path.join(
          releaseConfig.releaseDir,
          getReleaseExternalAssetRelativePath(skillConfig, asset),
        ),
      ),
    ),
  );

  for (const releaseAssetDir of releaseAssetDirs) {
    if (skillConfig.distName !== "tax-analysis") await fs.rm(releaseAssetDir, { recursive: true, force: true });
  }

  for (const asset of externalAssets) {
    const sourcePath = path.join(
      sceneOutputDir,
      getLocalExternalAssetRelativePath(asset),
    );
    const targetPath = path.join(
      releaseConfig.releaseDir,
      getReleaseExternalAssetRelativePath(skillConfig, asset),
    );

		await ensureDirectory(path.dirname(targetPath));
		if (asset.compression === "gzip") {
			const sourcePayload = await fs.readFile(sourcePath);
			const archivePayload = gzipSync(sourcePayload, {
				level: zlibConstants.Z_BEST_COMPRESSION,
				mtime: 0,
			});
			const expected = chartRendererManifest?.runtimeArchives?.[asset.archiveKey];
			verifyPinnedPayload(
				archivePayload,
				expected,
				`便携 Node.js 压缩发布资产 ${asset.archiveKey}`,
			);
			await fs.writeFile(targetPath, archivePayload);
			continue;
		}
		if (asset.compression) {
			throw new Error(`不支持的外置资产压缩格式：${asset.compression}`);
		}
		if (asset.sha256) {
			const payload = await fs.readFile(sourcePath);
			if (createHash("sha256").update(payload).digest("hex") !== asset.sha256) {
				throw new Error("外置资产 SHA-256 不匹配：" + asset.path);
			}
		}
		await fs.copyFile(sourcePath, targetPath);
  }
}

async function copySkillFiles(sceneOutputDir, skillConfig, shouldIgnore, releaseConfig) {
  const skillSourceDir = path.join(repoRoot, skillConfig.distName);
  const render = (content) => renderPackageText(content, releaseConfig, skillConfig);

  await copyRenderedText(
    path.join(skillSourceDir, SKILL_FILE),
    path.join(sceneOutputDir, SKILL_FILE),
    shouldIgnore,
    render,
  );
  await copyRenderedText(
    path.join(skillSourceDir, SKILL_FILE),
    path.join(releaseConfig.releaseDir, `${skillConfig.distName}.md`),
    shouldIgnore,
    render,
  );
  await copyRenderedText(
    path.join(skillSourceDir, skillConfig.installDoc),
    path.join(sceneOutputDir, skillConfig.installDoc),
    shouldIgnore,
    render,
  );
  await copyRenderedText(
    path.join(skillSourceDir, skillConfig.installDoc),
    path.join(releaseConfig.releaseDir, skillConfig.installDoc),
    shouldIgnore,
    render,
  );
  await copyRenderedText(
    path.join(skillSourceDir, "README.md"),
    path.join(sceneOutputDir, "README.md"),
    shouldIgnore,
    render,
  );
  await copyRenderedText(
    path.join(skillSourceDir, "THIRD_PARTY_NOTICES.md"),
    path.join(sceneOutputDir, "THIRD_PARTY_NOTICES.md"),
    shouldIgnore,
    render,
  );

  await copyEntry(
    path.join(skillSourceDir, "references"),
    path.join(sceneOutputDir, "references"),
    shouldIgnore,
  );

  if (skillConfig.reportVersions) {
    const referencesDir = path.join(sceneOutputDir, "references");
    for (const entry of await fs.readdir(referencesDir)) {
      if (!entry.endsWith(".md")) continue;
      const filename = path.join(referencesDir, entry);
      await fs.writeFile(filename, render(await fs.readFile(filename, "utf8")), "utf8");
    }
  }

  const assetRoot = getAssetRoot(skillConfig, skillSourceDir);
  for (const assetEntry of skillConfig.assets ?? []) {
    const sourceRelativePath =
      typeof assetEntry === "string" ? assetEntry : assetEntry.source;
    const targetRelativePath =
      typeof assetEntry === "string" ? assetEntry : assetEntry.target;
    await copyEntry(
      path.join(assetRoot, sourceRelativePath),
      path.join(sceneOutputDir, targetRelativePath),
      shouldIgnore,
    );
  }
}

async function copyRuntimeConfig(
  sceneOutputDir,
  skillConfig,
  environment,
  shouldIgnore,
  releaseConfig,
) {
  if (!skillConfig.runtimeConfig) {
    await copyVerifyRuntimeConfig(
      sceneOutputDir,
      skillConfig,
      skillConfig.environmentConfig,
      environment,
      shouldIgnore,
      releaseConfig,
    );
    return;
  }

  const environmentConfig = skillConfig.environmentConfig;
  const payload = {
    environment,
    default_base_url: requiredEnvironmentValue(
      environmentConfig,
      environment,
      OPEN_API_SKILL_BASE_URL_KEY,
    ),
    portal_base_url: requiredEnvironmentValue(
      environmentConfig,
      environment,
      OPEN_API_PAGE_URL_KEY,
    ),
    capability_code: skillConfig.capabilityCode,
    skill_name: skillConfig.distName,
    install_doc_url: releaseConfig.installDocUrl,
  };
  await ensureDirectory(sceneOutputDir);
  await fs.writeFile(
    path.join(sceneOutputDir, getConfigOutputPath(skillConfig)),
    `${JSON.stringify(payload, null, 2)}\n`,
    "utf8",
  );
}

function requiredEnvironmentValue(environmentConfig, environment, key) {
  const value = String(environmentConfig[key] ?? "").trim();
  if (!value) {
    throw new Error(`${environment}.json 缺少 ${key} 配置。`);
  }
  return value;
}

async function copyVerifyRuntimeConfig(
  sceneOutputDir,
  skillConfig,
  environmentConfig,
  environment,
  shouldIgnore,
  releaseConfig,
) {
  const sourcePath = path.join(configRoot, `${environment}.json`);
  if (shouldIgnore(sourcePath)) {
    return;
  }

  const payload = {
    environment: environmentConfig.environment,
    verify_api_endpoint: environmentConfig.verify_api_endpoint,
    invoice_check_code: environmentConfig.invoice_check_code,
    skill_name: skillConfig.distName,
    install_doc_url: releaseConfig.installDocUrl,
  };
  await ensureDirectory(path.join(sceneOutputDir, "config"));
  await fs.writeFile(
    path.join(sceneOutputDir, "config", "verify-api.json"),
    `${JSON.stringify(payload, null, 2)}\n`,
    "utf8",
  );
}

async function createSkillArchive(skillConfig, sceneOutputDir, releaseConfig) {
  const archivePath = path.join(releaseConfig.releaseDir, releaseConfig.archiveName);

  await fs.rm(archivePath, { force: true });

  const externalAssetPaths = (skillConfig.externalAssets ?? []).map((asset) =>
    getLocalExternalAssetRelativePath(asset),
  );
  if (skillConfig.distName === "tax-analysis") externalAssetPaths.push("assets", "assets/**");
  await validateWorkBuddyArchiveLayout(sceneOutputDir, externalAssetPaths);

  await new Promise((resolve, reject) => {
    const output = createWriteStream(archivePath);
    const archive = new ZipArchive({ zlib: { level: 9 } });

    output.on("close", resolve);
    output.on("error", reject);
    archive.on("error", reject);

    archive.pipe(output);
    archive.glob(
      "**/*",
      {
        cwd: sceneOutputDir,
        dot: true,
        nodir: true,
        ignore: ["bin", "bin/**", "scripts", "scripts/**", ...externalAssetPaths],
      },
      { prefix: skillConfig.distName },
    );

    archive.finalize().catch(reject);
  });

  return archivePath;
}

function isSkillArchiveExcluded(relativePath, externalAssetPaths) {
  if (
    relativePath === "bin" ||
    relativePath.startsWith("bin/") ||
    relativePath === "scripts" ||
    relativePath.startsWith("scripts/")
  ) {
    return true;
  }
  return externalAssetPaths.includes(relativePath) ||
    externalAssetPaths.includes("assets/**") && relativePath.startsWith("assets/");
}

async function validateWorkBuddyArchiveLayout(sceneOutputDir, externalAssetPaths) {
  const pending = [sceneOutputDir];
  const violations = [];
  while (pending.length > 0) {
    const current = pending.pop();
    const entries = await fs.readdir(current, { withFileTypes: true });
    for (const entry of entries) {
      const absolutePath = path.join(current, entry.name);
      const relativePath = normalizePath(path.relative(sceneOutputDir, absolutePath));
      if (isSkillArchiveExcluded(relativePath, externalAssetPaths)) {
        continue;
      }
      if (entry.isDirectory()) {
        pending.push(absolutePath);
        continue;
      }
      if (entry.isFile() && relativePath.split("/").length > 2) {
        violations.push(relativePath);
      }
      if (!entry.isFile()) throw new Error("安装 ZIP 条目必须是普通文件：" + relativePath);
      assertArchiveTextPayload(relativePath, await fs.readFile(absolutePath));
    }
  }
  if (violations.length > 0) {
    throw new Error(
      `${path.basename(sceneOutputDir)} 的 WorkBuddy Skill 包存在三级子目录：${violations.sort().join(", ")}`,
    );
  }
}

async function buildGoSkill(sceneKey, skillConfig, ignoreRules, options) {
  const skillSourceDir = path.join(repoRoot, skillConfig.distName);
  const skillMarkdown = await fs.readFile(path.join(skillSourceDir, SKILL_FILE), "utf8");
  const resolvedSkillConfig = {
    ...skillConfig,
    environmentConfig: options.environmentConfig,
    version: await validateSkillVersionSources(skillConfig, skillMarkdown),
    reportVersions: skillConfig.distName === "tax-analysis"
      ? await loadTaxAnalysisReportVersions(moduleRoot)
      : undefined,
  };
  const releaseConfig = getReleaseConfig(
    options.packageConfig,
    options.environment,
    options.environmentConfig,
    resolvedSkillConfig,
  );
  const sceneOutputDir = path.join(distRoot, skillConfig.distName);
  const shouldIgnore = shouldIgnoreFactory(ignoreRules);

  await fs.rm(sceneOutputDir, { recursive: true, force: true });
  await ensureDirectory(sceneOutputDir);
  await ensureDirectory(releaseConfig.releaseDir);

  await buildPlatformBinaries(resolvedSkillConfig, sceneOutputDir, options.binaryCache);
  if (resolvedSkillConfig.distName === "tax-analysis") {
    resolvedSkillConfig.binarySHA256 = await writeTaxAnalysisBinaryManifest(
      sceneOutputDir,
      resolvedSkillConfig,
      PLATFORM_TARGETS.map((target) => ({
        platform: target.binDir,
        path: getLocalBinaryRelativePath(resolvedSkillConfig, target),
        releasePath: getReleaseBinaryRelativePath(resolvedSkillConfig, target),
        url: releaseConfig.binaryUrls[target.binDir],
      })),
    );
  }
  await copySkillFiles(sceneOutputDir, resolvedSkillConfig, shouldIgnore, releaseConfig);
  await copyRuntimeConfig(
    sceneOutputDir,
    resolvedSkillConfig,
    options.environment,
    shouldIgnore,
    releaseConfig,
  );
  await buildChartRendererAssets(resolvedSkillConfig, sceneOutputDir);
  if (resolvedSkillConfig.distName === "tax-analysis") {
    await validateTaxAnalysisAssetInventory(sceneOutputDir);
    await fs.writeFile(path.join(sceneOutputDir, "references", "asset-download-manifest.json"),
      JSON.stringify(createTaxAnalysisAssetManifest(resolvedSkillConfig, releaseConfig.externalAssetUrls), null, 2) + "\n", "utf8");
  }
  await copyReleaseBinaries(sceneOutputDir, resolvedSkillConfig, releaseConfig);
  await copyReleaseExternalAssets(sceneOutputDir, resolvedSkillConfig, releaseConfig);

  if (resolvedSkillConfig.distName === "tax-analysis") await validateTaxAnalysisPackageDocs(sceneOutputDir);

  const archivePath = options.zipEnabled
    ? await createSkillArchive(resolvedSkillConfig, sceneOutputDir, releaseConfig)
    : null;

  return {
    key: sceneKey,
    name: resolvedSkillConfig.name,
    distName: resolvedSkillConfig.distName,
    output: sceneOutputDir,
    archive: archivePath,
    installDoc: path.join(releaseConfig.releaseDir, resolvedSkillConfig.installDoc),
    skillDoc: path.join(releaseConfig.releaseDir, `${resolvedSkillConfig.distName}.md`),
  };
}

function getBuildOptions(packageConfig) {
  const requestedKeys = [];
  let zipEnabled = true;
  let environment = "prod";
  const args = process.argv.slice(2);

  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];

    if (argument === "--no-zip") {
      zipEnabled = false;
      continue;
    }

    if (argument === "--env") {
      environment = args[index + 1] ?? "";
      index += 1;
      continue;
    }

    requestedKeys.push(argument);
  }

  const validEnvironments = new Set(Object.keys(packageConfig.environments ?? {}));
  if (!validEnvironments.has(environment)) {
    throw new Error(
      `Invalid environment: ${environment}. Expected one of: ${[...validEnvironments].join(", ")}.`,
    );
  }

  const unknownKeys = requestedKeys.filter((sceneKey) => !goSkillDir[sceneKey]);

  if (unknownKeys.length > 0) {
    throw new Error(`未知发票查验技能: ${unknownKeys.join(", ")}`);
  }

  const entries =
    requestedKeys.length === 0
      ? Object.entries(goSkillDir)
      : requestedKeys.map((sceneKey) => [sceneKey, goSkillDir[sceneKey]]);

  return { entries, environment, zipEnabled };
}

async function main() {
  const packageConfig = await readPackageConfig();
  const options = getBuildOptions(packageConfig);
  options.packageConfig = packageConfig;
  options.environmentConfig = await readEnvironmentConfig(options.environment);
  const ignoreRules = await readIgnoreRules();
  const results = [];

  console.log(
    `Packaging invoice verify skills env=${options.environment} zip=${options.zipEnabled}`,
  );

  await ensureDirectory(distRoot);
  options.binaryCache = new Map();

  for (const [sceneKey, sceneConfig] of options.entries) {
    results.push(await buildGoSkill(sceneKey, sceneConfig, ignoreRules, options));
  }

  for (const result of results) {
    console.log(`Packaged ${result.key} (${result.distName}) -> ${result.output}`);

    if (result.archive) {
      console.log(`Created archive -> ${result.archive}`);
    }
    console.log(`Rendered install doc -> ${result.installDoc}`);
    console.log(`Rendered skill doc -> ${result.skillDoc}`);
  }
}

const directRun =
  process.argv[1] &&
  import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href;

if (directRun) {
  main().catch((error) => {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  });
}

export {
  PLATFORM_TARGETS,
  copyRuntimeConfig,
  getBinaryName,
  getConfigOutputPath,
  getEntryPackage,
  getExternalAssetName,
  getLocalExternalAssetRelativePath,
  getLocalBinaryRelativePath,
  getReleaseConfig,
  getReleaseBinaryRelativePath,
  getReleaseExternalAssetRelativePath,
  getVersionedBinaryName,
  renderPackageText,
  requiredEnvironmentValue,
  validateSkillVersionValues,
  goSkillDir,
};
