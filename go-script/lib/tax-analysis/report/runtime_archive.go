package report

import (
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// InstallChartRendererRuntimeArchive 校验并解压当前平台的便携 Node.js 发布资产。
//
// @param archivePath 已下载的当前平台 gzip 文件路径。
// @param outputPath 安装后的 Node.js 可执行文件完整路径。
// @returns 原子安装且再次校验后的绝对运行时路径；任何校验失败都不会留下半成品。
func InstallChartRendererRuntimeArchive(archivePath, outputPath string) (string, error) {
	archivePath = strings.TrimSpace(archivePath)
	outputPath = strings.TrimSpace(outputPath)
	if archivePath == "" || outputPath == "" {
		return "", fmt.Errorf("便携 Node.js 安装失败：archive 和 output 路径不能为空")
	}
	platform, err := chartRendererPlatform(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", err
	}
	archiveExpected, archiveExists := chartRendererRuntimeArchiveAssets[platform]
	runtimeExpected, runtimeExists := chartRendererRuntimeAssets[platform]
	if !archiveExists || !runtimeExists {
		return "", fmt.Errorf("便携 Node.js 安装失败：缺少当前平台清单：%s", platform)
	}
	absoluteArchive, err := filepath.Abs(filepath.Clean(archivePath))
	if err != nil {
		return "", fmt.Errorf("便携 Node.js 安装失败：压缩包路径无效：%w", err)
	}
	absoluteOutput, err := filepath.Abs(filepath.Clean(outputPath))
	if err != nil {
		return "", fmt.Errorf("便携 Node.js 安装失败：输出路径无效：%w", err)
	}
	if absoluteArchive == absoluteOutput {
		return "", fmt.Errorf("便携 Node.js 安装失败：压缩包和输出路径不能相同")
	}
	if err := validateRuntimeArchiveAsset(absoluteArchive, archiveExpected); err != nil {
		return "", err
	}
	if info, statErr := os.Stat(absoluteOutput); statErr == nil {
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("便携 Node.js 安装失败：输出路径不是普通文件：%s", absoluteOutput)
		}
		if err := validateChartRendererAsset(absoluteOutput, runtimeExpected, "已安装便携 Node.js 运行时"); err != nil {
			return "", fmt.Errorf("便携 Node.js 安装失败：目标文件已存在但校验失败，请移走后重试：%w", err)
		}
		return absoluteOutput, nil
	} else if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("便携 Node.js 安装失败：无法检查输出路径：%w", statErr)
	}
	if err := extractVerifiedRuntimeGzip(absoluteArchive, absoluteOutput, runtimeExpected); err != nil {
		return "", err
	}
	if err := validateChartRendererAsset(absoluteOutput, runtimeExpected, "便携 Node.js 运行时"); err != nil {
		return "", fmt.Errorf("便携 Node.js 安装失败：解压后校验失败：%w", err)
	}
	return absoluteOutput, nil
}

// validateRuntimeArchiveAsset 校验下载的 gzip 文件大小和 SHA-256。
//
// @param path gzip 文件路径。
// @param expected 固定的压缩文件大小和哈希。
// @returns 文件与清单一致时返回 nil，否则返回安装错误。
func validateRuntimeArchiveAsset(path string, expected chartRendererAsset) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：未找到压缩包 %s：%w", path, err)
	}
	if !info.Mode().IsRegular() || info.Size() != expected.Size {
		return fmt.Errorf("便携 Node.js 安装失败：压缩包大小校验失败：%s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：无法读取压缩包：%w", err)
	}
	hash := sha256.New()
	_, copyErr := io.Copy(hash, file)
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("便携 Node.js 安装失败：读取压缩包失败：%w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("便携 Node.js 安装失败：关闭压缩包失败：%w", closeErr)
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("便携 Node.js 安装失败：压缩包 SHA-256 校验失败：%s", path)
	}
	return nil
}

// extractVerifiedRuntimeGzip 将 gzip 内容写入同目录临时文件并原子安装。
//
// @param archivePath 已完成外层校验的 gzip 文件。
// @param outputPath 最终运行时路径。
// @param expected 解压后文件的固定大小和哈希。
// @returns 解压、内层校验和原子改名全部成功时返回 nil。
func extractVerifiedRuntimeGzip(archivePath, outputPath string, expected chartRendererAsset) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), runtimeDirectoryMode); err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：创建运行时目录失败：%w", err)
	}
	archive, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：打开压缩包失败：%w", err)
	}
	defer archive.Close()
	reader, err := gzip.NewReader(archive)
	if err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：gzip 格式无效：%w", err)
	}
	defer reader.Close()
	temporary, err := os.CreateTemp(filepath.Dir(outputPath), ".node-runtime-*")
	if err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：创建临时文件失败：%w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		_ = temporary.Close()
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(temporary, hash), io.LimitReader(reader, expected.Size+1))
	if copyErr != nil {
		return fmt.Errorf("便携 Node.js 安装失败：解压运行时失败：%w", copyErr)
	}
	if closeErr := reader.Close(); closeErr != nil {
		return fmt.Errorf("便携 Node.js 安装失败：gzip 校验失败：%w", closeErr)
	}
	if written != expected.Size || fmt.Sprintf("%x", hash.Sum(nil)) != expected.SHA256 {
		return fmt.Errorf("便携 Node.js 安装失败：解压内容与运行时清单不一致")
	}
	if err := temporary.Chmod(runtimeExecutableMode); err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：设置运行时权限失败：%w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：同步运行时文件失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：关闭运行时文件失败：%w", err)
	}
	if _, statErr := os.Stat(outputPath); statErr == nil {
		return fmt.Errorf("便携 Node.js 安装失败：输出文件在安装期间已存在：%s", outputPath)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("便携 Node.js 安装失败：无法检查输出文件：%w", statErr)
	}
	if err := os.Rename(temporaryPath, outputPath); err != nil {
		return fmt.Errorf("便携 Node.js 安装失败：原子写入运行时失败：%w", err)
	}
	committed = true
	return nil
}
