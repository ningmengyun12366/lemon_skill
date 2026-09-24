package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

type ReportAssets struct {
	Directory       string
	CoverBackground []byte
}

// LoadReportAssets reads the immutable Figma-derived report assets adjacent to
// the bundled font directory and validates their hashes.
//
// Parameter fontDirectory is the verified assets/fonts directory. Returns the
// validated report assets or a fail-closed error; system or online fallback is
// never attempted.
func LoadReportAssets(fontDirectory string) (ReportAssets, error) {
	assetDirectory := filepath.Dir(filepath.Clean(fontDirectory))
	path := filepath.Join(assetDirectory, coverBackgroundFilename)
	payload, err := os.ReadFile(path)
	if err != nil {
		return ReportAssets{}, fmt.Errorf("报告生成失败：未找到Figma封面资产 %s：%w", path, err)
	}
	if len(payload) != coverBackgroundSize {
		return ReportAssets{}, fmt.Errorf("报告生成失败：Figma封面资产大小不符：%s", path)
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != coverBackgroundHash {
		return ReportAssets{}, fmt.Errorf("报告生成失败：Figma封面资产SHA-256校验失败：%s", path)
	}
	return ReportAssets{Directory: assetDirectory, CoverBackground: payload}, nil
}
