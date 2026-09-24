package reportpdf

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	regularFontFilename = "NotoSansSC-Regular.otf"
	boldFontFilename    = "NotoSansSC-Bold.otf"
)

type bundledFonts struct {
	RegularPath string
	BoldPath    string
	Regular     []byte
	Bold        []byte
}

func loadBundledFonts() (bundledFonts, error) {
	for _, dir := range fontSearchDirs() {
		regularPath := filepath.Join(dir, regularFontFilename)
		boldPath := filepath.Join(dir, boldFontFilename)
		regular, regularOK := readFontFile(regularPath)
		bold, boldOK := readFontFile(boldPath)
		if regularOK && boldOK {
			return bundledFonts{
				RegularPath: regularPath,
				BoldPath:    boldPath,
				Regular:     regular,
				Bold:        bold,
			}, nil
		}
	}
	return bundledFonts{}, fmt.Errorf("PDF生成失败：未找到 bundled Noto Sans SC 字体")
}

func readFontFile(path string) ([]byte, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() < 1024 {
		return nil, false
	}
	payload, err := os.ReadFile(path)
	if err != nil || len(payload) < 1024 {
		return nil, false
	}
	return payload, true
}

func fontSearchDirs() []string {
	var roots []string
	if wd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Clean(wd))
	}
	if executable, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(executable))
	}
	return fontSearchDirsFromRoots(roots...)
}

func fontSearchDirsFromRoots(roots ...string) []string {
	seen := map[string]bool{}
	var dirs []string
	addFrom := func(root string) {
		for root != "" {
			for _, candidate := range []string{
				filepath.Join(root, "assets", "fonts"),
				filepath.Join(root, "lib", "invoice-verify-common", "assets", "fonts"),
				filepath.Join(root, "invoice-verify-common", "assets", "fonts"),
			} {
				candidate = filepath.Clean(candidate)
				if !seen[candidate] {
					seen[candidate] = true
					dirs = append(dirs, candidate)
				}
			}
			parent := filepath.Dir(root)
			if parent == root {
				break
			}
			root = parent
		}
	}

	for _, root := range roots {
		addFrom(filepath.Clean(root))
	}
	return dirs
}
