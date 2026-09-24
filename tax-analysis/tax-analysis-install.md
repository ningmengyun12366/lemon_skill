# tax-analysis 安装与更新

## 当前发布信息

- version: 0.1.1
- 发布包：`tax-analysis-0.1.1.zip`
- 发布包地址：{{ZIP_URL}}
- 安装文档地址：{{INSTALL_DOC_URL}}
- 运行时能力码：`tax_analysis`

## 版本检测与自动更新

1. 本 skill 会自动检测版本，版本不符合要求时会阻断执行。
2. 若命令返回包含“当前 skill 版本过低，无法执行”的报错，Agent 立即从报错提供的安装文档链接获取最新教程，备份当前 skill 后按教程更新与重装，无需等待用户再次确认。
3. 用户说“更新并重装skill”时，Agent 按同一流程立即执行。
4. 验证成功后删除备份并重试原业务；验证或安装失败时恢复备份并说明原因。

解压ZIP后，还需按下方下载表安装当前平台主程序、字体、封面及运行资源。各文件的保存路径和SHA-256见下载表及`references/asset-download-manifest.json`。

## 外置文件下载表

| 文件 | 下载地址 | 下载保存路径 |
| --- | --- | --- |
| Windows amd64 | {{BIN_URL_WINDOWS_AMD64}} | `bin/windows-amd64/tax-analysis.exe` |
| Linux amd64 | {{BIN_URL_LINUX_AMD64}} | `bin/linux-amd64/tax-analysis` |
| Linux arm64 | {{BIN_URL_LINUX_ARM64}} | `bin/linux-arm64/tax-analysis` |
| macOS amd64 | {{BIN_URL_DARWIN_AMD64}} | `bin/darwin-amd64/tax-analysis` |
| macOS arm64 | {{BIN_URL_DARWIN_ARM64}} | `bin/darwin-arm64/tax-analysis` |
{{TAX_ASSET_DOWNLOAD_ROWS}}

安装时只下载当前平台的主程序，并按当前 OS/CPU 选择清单中 `platform=all` 和当前平台的资源；不要下载其他平台的 Node。主程序按下一节先校验 SHA-256，再执行下方资源安装步骤。资源下载完成后在本地生成图表，不依赖系统 Node.js、Bun、浏览器、Word、LibreOffice 或在线图表服务。


## 主程序SHA-256校验

主程序按SHA-256校验。清单`references/binary-manifest.json`包含各平台的本地路径、发布地址和哈希。下载后、第一次执行主程序之前必须校验；哈希不符时重新下载，三次仍不符则安装失败并恢复备份。

| 平台 | 主程序SHA-256 |
| --- | --- |
| Windows amd64 | `{{BIN_SHA256_WINDOWS_AMD64}}` |
| Linux amd64 | `{{BIN_SHA256_LINUX_AMD64}}` |
| Linux arm64 | `{{BIN_SHA256_LINUX_ARM64}}` |
| macOS amd64 | `{{BIN_SHA256_DARWIN_AMD64}}` |
| macOS arm64 | `{{BIN_SHA256_DARWIN_ARM64}}` |

在已解压的Skill根目录，Windows使用系统PowerShell校验，不运行尚未验证的主程序：

```powershell
$manifest = Get-Content -LiteralPath 'references/binary-manifest.json' -Raw | ConvertFrom-Json
$binary = $manifest.binaries.'windows-amd64'
$actualHash = (Get-FileHash -LiteralPath $binary.path -Algorithm SHA256).Hash
if ($actualHash -ine $binary.sha256) { throw '主程序SHA-256不匹配，停止执行并重新下载' }
```

Linux用`sha256sum`、macOS用`shasum -a 256`计算当前平台下载文件的哈希，并与上表或清单逐字比较；匹配后才能增加执行权限并运行。

## 全部 assets 的下载与校验

`references/asset-download-manifest.json` 的每项包含：最终本地路径 `path`、下载文件路径 `downloadPath`、固定 `url`、下载内容 `sha256`、安装后 `installedSha256`、适用 `platform` 和可选 `compression=gzip`。最终路径保持原来的 assets 目录结构，服务器哈希路径不能直接用作本地目录。

安装时复用安装后 SHA-256 一致的文件；只有缺失或哈希不符才下载。每项最多三次尝试，临时文件校验通过后才替换目标，失败不得覆盖已有完整文件。Node gzip 通过已校验的主程序 `unpack-runtime` 解压并校验；首次安装所有共用资源及当前平台 Node 都必须完成。

Windows 在 Skill 根目录执行以下 PowerShell，可保存为临时 `.ps1` 后运行；该脚本不使用系统 Node：

```powershell
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$assetRoot = (Get-Location).Path
$platform = 'windows-amd64'
$downloads = Get-Content -LiteralPath 'references/asset-download-manifest.json' -Raw | ConvertFrom-Json
$binaries = Get-Content -LiteralPath 'references/binary-manifest.json' -Raw | ConvertFrom-Json
$program = $binaries.binaries.$platform
$binaryPath = Join-Path $assetRoot $program.path
if ((Get-FileHash -LiteralPath $binaryPath -Algorithm SHA256).Hash -ine $program.sha256) {
  throw '主程序 SHA-256 不匹配，停止安装'
}
function Resolve-AssetFile([string]$relative) {
  $absolute = [IO.Path]::GetFullPath((Join-Path $assetRoot $relative))
  $prefix = $assetRoot.TrimEnd('\', '/') + [IO.Path]::DirectorySeparatorChar
  if (-not $relative.StartsWith('assets/') -or -not $absolute.StartsWith($prefix, [StringComparison]::OrdinalIgnoreCase)) {
    throw "资产路径超出安装目录：$relative"
  }
  return $absolute
}
foreach ($asset in $downloads.assets) {
  if ($asset.platform -ne 'all' -and $asset.platform -ne $platform) { continue }
  $installed = Resolve-AssetFile $asset.path
  if ((Test-Path -LiteralPath $installed -PathType Leaf) -and
      (Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -ieq $asset.installedSha256) {
    continue
  }
  $download = Resolve-AssetFile $asset.downloadPath
  New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($download)) -Force | Out-Null
  for ($attempt = 1; $attempt -le 3; $attempt++) {
    $partial = $download + '.' + [Guid]::NewGuid().ToString('N') + '.part'
    try {
      Invoke-WebRequest -Uri $asset.url -OutFile $partial -UseBasicParsing -TimeoutSec 180
      if ((Get-FileHash -LiteralPath $partial -Algorithm SHA256).Hash -ine $asset.sha256) {
        throw "资源 SHA-256 不匹配：$($asset.path)"
      }
      if ($asset.compression -eq 'gzip') {
        & $binaryPath unpack-runtime --archive $partial --output $installed --pretty
        if ($LASTEXITCODE -ne 0) { throw "运行时解压失败：$($asset.path)" }
      } elseif (Test-Path -LiteralPath $installed) {
        [IO.File]::Replace($partial, $installed, [NullString]::Value)
      } else {
        [IO.File]::Move($partial, $installed)
      }
      if ((Get-FileHash -LiteralPath $installed -Algorithm SHA256).Hash -ine $asset.installedSha256) {
        throw "资源安装后校验失败：$($asset.path)"
      }
      break
    } catch {
      if ($attempt -eq 3) { throw }
    } finally {
      if (Test-Path -LiteralPath $partial) { Remove-Item -LiteralPath $partial -Force }
    }
  }
}
```

Linux/macOS 按同一清单执行相同步骤：根据当前平台选择主程序及资源，用 `curl --fail --location` 下载到同目录临时文件，使用 `sha256sum` 或 `shasum -a 256` 校验，匹配后再原子替换。gzip 使用对应主程序的 `unpack-runtime`。已校验的主程序和 Node 需有执行权限；不依赖 jq、Python 或系统 Node，Agent 可直接读取 JSON 清单组织命令。

## 字体固定清单

详细来源、大小和 SHA-256 见 `references/font-manifest.json`。安装必须逐项校验：

| 文件 | 字节数 | SHA-256 |
| --- | ---: | --- |
| `NotoSansSC-Regular.ttf` | 10596340 | `147cb15dae4c1327ef1e135416f371b32033c8504b93588b110ee92992b3b046` |
| `NotoSansSC-Bold.ttf` | 10585860 | `3d5f289b1cdfecc702c3eaf8464ce87b2b5b973d14cc33385cc26cf1e3b71cf5` |
| `OFL.txt` | 4379 | `eddf1ad0c3c9bfcdb14ca7e0fd276f0eb64e4a7978e8c12d05e697e09aa9572d` |

任一文件为空、大小不符、哈希不符或许可证缺失时安装失败并恢复备份。运行时也会重新校验三个文件；禁止使用系统字体代替。

Linux AMD64/ARM64 需要glibc环境；`0.1.1`不支持Alpine/musl，应使用glibc Linux容器或主机。安装时无需另行安装系统中文字体、Node.js、浏览器、Word或LibreOffice。

## 封面图片固定清单

封面图片从上方下载表的服务器地址下载，不包含在 ZIP 中。下载后按 `references/report-asset-manifest.json` 校验本地路径、文件大小和 SHA-256；报告生成时会再次校验，使用本地图片，不在渲染时访问网络。

在已解压的 Skill 根目录，Windows 使用 PowerShell 校验：

```powershell
$assetManifest = Get-Content -LiteralPath 'references/report-asset-manifest.json' -Raw | ConvertFrom-Json
foreach ($asset in $assetManifest.assets) {
  $assetFile = Get-Item -LiteralPath $asset.path -ErrorAction Stop
  $assetHash = (Get-FileHash -LiteralPath $asset.path -Algorithm SHA256).Hash
  if ($assetFile.Length -ne $asset.size -or $assetHash -ine $asset.sha256) {
    throw "封面图片校验失败，重新下载：$($asset.path)"
  }
}
```

Linux/macOS 用 `wc -c` 校验字节数，用 `sha256sum` 或 `shasum -a 256` 校验哈希，与清单逐项比较。缺失或校验失败时重新下载，三次仍失败则恢复备份，不得跳过封面或用其他图片代替。

## 图表运行时固定清单

按`references/chart-renderer-manifest.json`安装当前平台的便携运行文件及全部图表组件，并核对文件大小和SHA-256。手动下载gzip文件时，使用`unpack-runtime`完成校验和解压。各平台运行文件的校验值如下：

| 平台 | gzip 字节数 | gzip SHA-256 | 解压后字节数 | 解压后 SHA-256 |
| --- | ---: | --- | ---: | --- |
| Windows amd64 | 34368222 | `e363261b27e7ff774bea4364c346824374c2b5ffb5d785e84d72ca5300e8f899` | 89631216 | `3a9ac860c566b7aebf144be914f95e3ec36dc88040aa7acb5d9d1e6e0c7afd5e` |
| Linux amd64 | 46505482 | `37ed501d8c05c93406e338d488365a75c50e56d310a3e2f2f27750744cbbb432` | 129318464 | `85ede4ce63fdae514aea9f1d93db8fa53a13ed9eeb2f1b212cd0fd1df5f07e9e` |
| Linux arm64 | 46383855 | `92383892b1c51b5ff66cc34c0e6f319c27676797935474755b58dbee82cedff0` | 126278720 | `3e74ab04f89a2396eaf14721e6cd47ef9a33dba9edd409600ff0f7783d2f6e29` |
| macOS amd64 | 39672015 | `c193879fe7de06ef17f9f86d22f9b1380b44302b34e9ce66adbab949d30f6341` | 119545280 | `74a32318f5ad5a500a07047a22139a45b33a72bb07a7bbf0119908866a11d4c4` |
| macOS arm64 | 38706817 | `165b862010a03336c423831ae5b3c52994999061125563683aa8fe5dc0ed2557` | 117216384 | `63980caabf84bf344205aa3b005a84e342a944d9c15905c0c2888540e81b8f14` |

手动下载 gzip 时，可使用以下解压命令；已执行上方 PowerShell 完整安装脚本的无需再次解压。

Windows amd64：

```powershell
bin\windows-amd64\tax-analysis.exe unpack-runtime `
  --archive "assets\chart-renderer\runtime\windows-amd64\node.exe.gz" `
  --output "assets\chart-renderer\runtime\windows-amd64\node.exe" `
  --pretty
```

Linux/macOS 使用对应平台主程序、压缩包和输出路径，例如 Linux amd64：

```bash
chmod +x bin/linux-amd64/tax-analysis
bin/linux-amd64/tax-analysis unpack-runtime \
  --archive assets/chart-renderer/runtime/linux-amd64/node.gz \
  --output assets/chart-renderer/runtime/linux-amd64/node \
  --pretty
```

命令成功后 stdout 返回 `status=success` 和安装后的绝对 `runtimePath`。此时可删除 `.gz` 临时下载文件；删除不会影响报告运行。

## 安装流程

1. 确认 Agent 实际加载的 Skill 根目录，下载并解压 `tax-analysis-0.1.1.zip`，最终目录必须为 `<Skill根>/tax-analysis/`。
2. 已有安装时先备份旧目录；不要覆盖后立即删除备份。
3. 下载当前平台主程序并按 binary-manifest.json 校验 SHA-256，然后按 asset-download-manifest.json 下载并验证全部共用资源及当前平台运行时；逐项复用正确文件，失败最多尝试三次。
4. 手动下载 gzip 时，用已校验的主程序执行 `unpack-runtime`，成功后删除 `.gz` 临时文件；上方 PowerShell 完整安装脚本已包含解压和清理，不必重复执行。
5. 按下载清单确认每项安装后 SHA-256 均一致，并按字体、封面和图表固定清单复核，确认 `assets/fonts/OFL.txt` 存在。Linux/macOS 为主程序和解压后的便携 Node.js 增加执行权限，然后执行 `bin/<platform>/tax-analysis --help`。
6. 确认 `SKILL.md`、`README.md`、`runtime-config.json`、`THIRD_PARTY_NOTICES.md`、`licenses/`、`assets/chart-renderer.mjs`、`assets/chart-renderer-constants.mjs`、`assets/resvg-wasm.mjs`、`assets/resvg-wasm-bg.wasm`、`assets/opentype.cjs`、`assets/report-cover.png`、`references/report.md`、`references/report-data-contract.md`、`references/report-rules.md`、`references/binary-manifest.json`、`references/asset-download-manifest.json`、`references/font-manifest.json`、`references/report-asset-manifest.json`、`references/chart-renderer-manifest.json` 和 `references/third-party-license-manifest.json` 均存在。
7. 验证成功后删除备份并刷新 Skill；失败时恢复备份并说明具体失败项。
8. 安装完成后立即完整原样输出已安装目录中的 `README.md`，不得概括或删减。

## 更新和失败边界

- 本 Skill 版本为 `0.1.1`。版本检测阻断时按错误中的安装文档执行同样的备份、覆盖、验证和恢复流程。
- 字体采用 SIL Open Font License 1.1，许可证允许商业使用、再分发和文档嵌入；许可证原文必须随安装目录保留。
- 图表运行时及其第三方依赖的许可证和来源见随包 `THIRD_PARTY_NOTICES.md` 和 `licenses/`；不得改用系统 Node.js、系统浏览器或未固定版本的在线渲染服务。
- 任一外置地址缺失、下载三次仍失败、平台不受支持、字体、封面图片、便携 Node.js 或 JS/WASM 组件校验失败、命令不可执行时，安装失败且不得报告成功。
