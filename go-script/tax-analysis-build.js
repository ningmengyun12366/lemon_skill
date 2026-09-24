import { createHash } from "node:crypto";
import { promises as fs } from "node:fs";
import path from "node:path";

/** Read the executable's contract versions; runtime JSON cannot override them. */
export async function loadTaxAnalysisReportVersions(moduleRoot) {
  const source = await fs.readFile(
    path.join(moduleRoot, "lib/tax-analysis/report/workflow_constants.go"),
    "utf8"
  );
  const rule = source.match(/\bRuleVersion\s*=\s*"([^"]+)"/)?.[1];
  const ai = source.match(/\bAIContractVersion\s*=\s*"([^"]+)"/)?.[1];
  if (!rule || !ai) throw new Error("税务分析缺少报告规则或AI契约版本常量");
  return { rule, ai };
}

/** Prevent independently maintained report versions in current package docs. */
export function renderTaxAnalysisReportVersions(content, versions) {
  if (/2026\.09\.v\d+|tax-analysis-ai\.v\d+/.test(content)) {
    throw new Error("税务分析运行文档不能手写报告版本，请使用构建占位符");
  }
  return content
    .replaceAll("{{REPORT_RULE_VERSION}}", versions.rule)
    .replaceAll("{{AI_CONTRACT_VERSION}}", versions.ai);
}

/** Bind every external executable to its release URL with SHA-256 only. */
export async function writeTaxAnalysisBinaryManifest(
  outputDir,
  skill,
  descriptors
) {
  const binaries = {};
  for (const entry of descriptors) {
    const bytes = await fs.readFile(path.join(outputDir, entry.path));
    binaries[entry.platform] = {
      path: entry.path,
      releasePath: entry.releasePath,
      url: entry.url,
      sha256: createHash("sha256").update(bytes).digest("hex"),
    };
  }
  const dir = path.join(outputDir, "references");
  await fs.mkdir(dir, { recursive: true });
  await fs.writeFile(
    path.join(dir, "binary-manifest.json"),
    JSON.stringify(
      { skill: skill.distName, version: skill.version, binaries },
      null,
      2
    ) + "\n"
  );
  return Object.fromEntries(
    Object.entries(binaries).map(([platform, value]) => [
      platform,
      value.sha256,
    ])
  );
}

/** Verify the shipped references are self-contained and fully rendered. */
export async function validateTaxAnalysisPackageDocs(outputDir) {
  const refs = path.join(outputDir, "references");
  const docs = [
    "SKILL.md",
    "README.md",
    "tax-analysis-install.md",
    ...(await fs.readdir(refs))
      .filter((entry) => entry.endsWith(".md"))
      .map((entry) => `references/${entry}`),
  ];
  for (const doc of docs) {
    const source = await fs.readFile(path.join(outputDir, doc), "utf8");
    if (
      /\{\{(?:REPORT_RULE_VERSION|AI_CONTRACT_VERSION|BIN_SHA256_[A-Z_]+)\}\}/.test(
        source
      )
    ) {
      throw new Error(`${doc}仍含未渲染版本或主程序哈希`);
    }
    for (const match of source.matchAll(/\[[^\]\n]*\]\(([^)\n]+)\)/g)) {
      const link = match[1];
      if (/^(?:https?:|#|\{\{)/.test(link)) continue;
      const target = path.resolve(
        outputDir,
        path.dirname(doc),
        link.split("#")[0]
      );
      if (!target.startsWith(path.resolve(outputDir) + path.sep))
        throw new Error(`${doc}引用包外资料: ${link}`);
      try {
        await fs.access(target);
      } catch {
        throw new Error(`${doc}引用不存在: ${link}`);
      }
    }
  }
}
