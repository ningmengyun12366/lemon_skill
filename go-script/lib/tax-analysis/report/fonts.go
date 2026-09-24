package report

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	regularFontName = "NotoSansSC-Regular.ttf"
	boldFontName    = "NotoSansSC-Bold.ttf"
	fontLicenseName = "OFL.txt"
	regularFontHash = "147cb15dae4c1327ef1e135416f371b32033c8504b93588b110ee92992b3b046"
	boldFontHash    = "3d5f289b1cdfecc702c3eaf8464ce87b2b5b973d14cc33385cc26cf1e3b71cf5"
	licenseHash     = "eddf1ad0c3c9bfcdb14ca7e0fd276f0eb64e4a7978e8c12d05e697e09aa9572d"
)

type FontFiles struct {
	Directory   string
	RegularPath string
	BoldPath    string
	LicensePath string
	Regular     []byte
	Bold        []byte
}

// LoadFonts loads only the bundled Noto Sans SC assets and verifies every file
// against the release manifest. It never queries or falls back to system fonts.
func LoadFonts(explicitDir string) (FontFiles, error) {
	if explicitDir != "" {
		fonts, err := loadFontsFrom(explicitDir)
		if err != nil {
			return FontFiles{}, fmt.Errorf("报告生成失败：bundled Noto Sans SC 字体校验失败：%w", err)
		}
		return fonts, nil
	}
	for _, directory := range fontDirectories(explicitDir) {
		fonts, err := loadFontsFrom(directory)
		if err == nil {
			return fonts, nil
		}
	}
	return FontFiles{}, fmt.Errorf("报告生成失败：未找到校验通过的 bundled Noto Sans SC 字体；请按安装文档重新下载 assets/fonts")
}

func loadFontsFrom(directory string) (FontFiles, error) {
	regularPath := filepath.Join(directory, regularFontName)
	boldPath := filepath.Join(directory, boldFontName)
	licensePath := filepath.Join(directory, fontLicenseName)
	regular, err := readVerifiedFile(regularPath, regularFontHash, 1024)
	if err != nil {
		return FontFiles{}, err
	}
	bold, err := readVerifiedFile(boldPath, boldFontHash, 1024)
	if err != nil {
		return FontFiles{}, err
	}
	if _, err := readVerifiedFile(licensePath, licenseHash, 100); err != nil {
		return FontFiles{}, err
	}
	return FontFiles{Directory: directory, RegularPath: regularPath, BoldPath: boldPath, LicensePath: licensePath, Regular: regular, Bold: bold}, nil
}

func readVerifiedFile(path, expectedHash string, minimumSize int) ([]byte, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(payload) < minimumSize {
		return nil, fmt.Errorf("file too small: %s", path)
	}
	sum := sha256.Sum256(payload)
	if hex.EncodeToString(sum[:]) != expectedHash {
		return nil, fmt.Errorf("SHA-256 mismatch: %s", path)
	}
	return payload, nil
}

func fontDirectories(explicitDir string) []string {
	seen := map[string]bool{}
	var result []string
	add := func(directory string) {
		if directory == "" {
			return
		}
		directory = filepath.Clean(directory)
		if !seen[directory] {
			seen[directory] = true
			result = append(result, directory)
		}
	}
	add(explicitDir)
	var roots []string
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executable))
	}
	if workingDirectory, err := os.Getwd(); err == nil {
		roots = append(roots, workingDirectory)
	}
	for _, root := range roots {
		for current := filepath.Clean(root); current != ""; current = filepath.Dir(current) {
			if strings.EqualFold(filepath.Base(current), "tax-analysis") {
				add(filepath.Join(current, "assets", "fonts"))
				break
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return result
}
