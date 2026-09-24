package report

const (
	chartRendererDirectory        = "chart-renderer"
	chartRendererRuntimeDirectory = "runtime"
	chartRendererRuntimeBaseName  = "node"
	chartRendererScriptName       = "chart-renderer.mjs"
	chartPixelWidth               = 1200
	chartPixelHeight              = 500
	runtimeExecutableMode         = 0o755
	runtimeDirectoryMode          = 0o755
)

type chartRendererAsset struct {
	Size   int64
	SHA256 string
}

// chartRendererRuntimeAssets pins the official portable Node.js runtime used
// by each supported report platform. Values are mirrored by the public asset
// manifest and verified again at build and installation time.
var chartRendererRuntimeAssets = map[string]chartRendererAsset{
	"windows-amd64": {Size: 89631216, SHA256: "3a9ac860c566b7aebf144be914f95e3ec36dc88040aa7acb5d9d1e6e0c7afd5e"},
	"linux-amd64":   {Size: 129318464, SHA256: "85ede4ce63fdae514aea9f1d93db8fa53a13ed9eeb2f1b212cd0fd1df5f07e9e"},
	"linux-arm64":   {Size: 126278720, SHA256: "3e74ab04f89a2396eaf14721e6cd47ef9a33dba9edd409600ff0f7783d2f6e29"},
	"darwin-amd64":  {Size: 119545280, SHA256: "74a32318f5ad5a500a07047a22139a45b33a72bb07a7bbf0119908866a11d4c4"},
	"darwin-arm64":  {Size: 117216384, SHA256: "63980caabf84bf344205aa3b005a84e342a944d9c15905c0c2888540e81b8f14"},
}

// chartRendererRuntimeArchiveAssets pins the gzip files downloaded during
// installation. Each archive expands to the corresponding entry in
// chartRendererRuntimeAssets and is never used directly for rendering.
var chartRendererRuntimeArchiveAssets = map[string]chartRendererAsset{
	"windows-amd64": {Size: 34368222, SHA256: "e363261b27e7ff774bea4364c346824374c2b5ffb5d785e84d72ca5300e8f899"},
	"linux-amd64":   {Size: 46505482, SHA256: "37ed501d8c05c93406e338d488365a75c50e56d310a3e2f2f27750744cbbb432"},
	"linux-arm64":   {Size: 46383855, SHA256: "92383892b1c51b5ff66cc34c0e6f319c27676797935474755b58dbee82cedff0"},
	"darwin-amd64":  {Size: 39672015, SHA256: "c193879fe7de06ef17f9f86d22f9b1380b44302b34e9ce66adbab949d30f6341"},
	"darwin-arm64":  {Size: 38706817, SHA256: "165b862010a03336c423831ae5b3c52994999061125563683aa8fe5dc0ed2557"},
}

// chartRendererBundleAssets pins every executable JavaScript/WASM input. The
// runtime never resolves packages from PATH or from a system installation.
var chartRendererBundleAssets = map[string]chartRendererAsset{
	"chart-renderer-package.json":  {Size: 191, SHA256: "7933228e43b6a0ce3f234eac369a343c27a8c5a1b3fc7146fc6eea46e85d9e5e"},
	"chart-renderer-constants.mjs": {Size: 6409, SHA256: "395ae97c611185853502af5f6960ba5bb17c50deb7cc93466f8403530927fccf"},
	"chart-renderer.mjs":           {Size: 64327, SHA256: "974d72a5e4bed382b008159544c38e7fcd349b6e993897db0d0e4d3aa6d6e454"},
	"resvg-wasm.mjs":               {Size: 17048, SHA256: "10170d02d816f02ec76f9bc095b01d9becf536e7b1e12e5aa616652c84b237a1"},
	"resvg-wasm-bg.wasm":           {Size: 2478606, SHA256: "22bf6e9f9a100d972da0411a69c5ba504367fc1fa87b3b64e3f35e53926d2d70"},
	"resvg-wasm-package.json":      {Size: 903, SHA256: "4a9efcb54b2670726ccf9b91086413676c3de0fae607cce3b8a5a0d25abaff48"},
	"opentype.cjs":                 {Size: 477325, SHA256: "aca3813e716a07e06c2e32c736d841508b15451cf39483f289acbd879c64d09f"},
	"opentype-package.json":        {Size: 1618, SHA256: "55f5100e9f5bbf19b4193fe90730da32c585b941a486159ac8dbc1d18219d7db"},
}
