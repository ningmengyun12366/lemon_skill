package report

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// reportContentDigest fingerprints the complete validated report model used by
// both renderers. It covers titles, text, findings, table values and chart data
// and therefore provides a machine-checkable DOCX/PDF content-parity marker.
//
// Parameter model is the final post-Agent model. Return value is a lowercase
// SHA-256 string; JSON encoding failures return an empty string because Model
// contains only supported value types.
func reportContentDigest(model Model) string {
	payload, err := json.Marshal(model)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// reportDocumentSubject returns identical document metadata for DOCX and PDF.
// Parameter model is the shared final model; the return value includes its
// content digest without exposing source records or credentials.
func reportDocumentSubject(model Model) string {
	return "财税风险分析报告；内容摘要=" + reportContentDigest(model)
}
