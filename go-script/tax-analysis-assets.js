/** Shared installation/release asset contract; hashes come from pinned manifests. */
import { readFileSync } from "node:fs";
import { promises as fs } from "node:fs";
import path from "node:path";

function manifest(name) {
  return JSON.parse(readFileSync(new URL("../tax-analysis/references/" + name, import.meta.url), "utf8"));
}

function assetSpec(localPath, pinned, templateKey, extra = {}) {
  const sha256 = extra.downloadSha256 ?? pinned.sha256;
  if (!/^[a-f0-9]{64}$/.test(sha256)) throw new Error("Invalid pinned asset SHA-256: " + localPath);
  const downloadPath = extra.compression === "gzip" ? localPath + ".gz" : localPath;
  return {
    path: localPath, downloadPath, templateKey,
    releasePath: "assets/sha256/" + sha256 + "/" + path.posix.basename(downloadPath),
    sha256, installedSha256: pinned.sha256, platform: "all", ...extra,
  };
}

const fonts = manifest("font-manifest.json");
const cover = manifest("report-asset-manifest.json");
const renderer = manifest("chart-renderer-manifest.json");
const fontKeys = {
  "NotoSansSC-Regular.ttf": "PDF_FONT_REGULAR",
  "NotoSansSC-Bold.ttf": "PDF_FONT_BOLD",
  "OFL.txt": "FONT_LICENSE",
};

export const taxAnalysisExternalAssets = [
  ...fonts.files.map((item) => assetSpec("assets/fonts/" + item.name, item, fontKeys[item.name])),
  ...cover.assets.map((item) => assetSpec(item.path, item, "REPORT_COVER")),
  ...Object.entries(renderer.runtimeFiles).map(([platform, item]) => assetSpec(
    item.path, item, "CHART_RUNTIME_" + platform.toUpperCase().replaceAll("-", "_"),
    { platform, compression: "gzip", archiveKey: platform, downloadSha256: renderer.runtimeArchives[platform].sha256 },
  )),
  ...Object.entries(renderer.rendererFiles).map(([name, item]) => assetSpec(
    item.path, item, "CHART_FILE_" + name.toUpperCase().replaceAll(/[^A-Z0-9]/g, "_"),
  )),
];

export function createTaxAnalysisAssetManifest(skill, urls) {
  return {
    schemaVersion: 1, skill: "tax-analysis", version: skill.version,
    assets: taxAnalysisExternalAssets.map((item) => ({
      path: item.path, downloadPath: item.downloadPath, url: urls[item.templateKey],
      sha256: item.sha256, installedSha256: item.installedSha256,
      platform: item.platform, ...(item.compression ? { compression: item.compression } : {}),
    })),
  };
}

export function taxAnalysisAssetDownloadRows(urls) {
  return taxAnalysisExternalAssets.map((item) =>
    "| " + path.posix.basename(item.path) + (item.platform === "all" ? "（共用）" : "（" + item.platform + "）") +
    " | " + urls[item.templateKey] + " | " + item.downloadPath + " |").join("\n");
}

/** Do not silently omit a newly introduced asset from the server release. */
export async function validateTaxAnalysisAssetInventory(sceneOutputDir) {
  const expected = new Set(taxAnalysisExternalAssets.map((item) => item.path));
  async function walk(directory) {
    for (const entry of await fs.readdir(directory, { withFileTypes: true })) {
      const absolute = path.join(directory, entry.name);
      if (entry.isDirectory()) { await walk(absolute); continue; }
      const relative = path.relative(sceneOutputDir, absolute).replaceAll("\\", "/");
      if (!entry.isFile() || !expected.delete(relative)) throw new Error("外置资产未登记或不是普通文件：" + relative);
    }
  }
  await walk(path.join(sceneOutputDir, "assets"));
  if (expected.size) throw new Error("外置资产缺失：" + [...expected].join(", "));
}
