# Bundled font policy

The dependency PoCs use repository-bundled Noto Sans CJK Simplified Chinese font files for CJK PDF generation:

- `NotoSansSC-Regular.otf`
- `NotoSansSC-Bold.otf`

The files are sourced from the Noto CJK Simplified Chinese OTF release and are kept under `assets/fonts/` so Go PDF code can load or embed them without looking up operating-system fonts. The accompanying `OFL.txt` must remain in this directory whenever these font files are shipped or referenced.
