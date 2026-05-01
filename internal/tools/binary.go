package tools

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// BinaryOutput is the envelope shape generated tools return when the caller
// supplied outputPath: the raw base64 field is decoded to disk and the user
// only sees these metadata fields.
type BinaryOutput struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"sizeBytes"`
	MimeType  string `json:"mimeType,omitempty"`
}

// WriteBinaryFieldOutput is called by generated CDP tools when their manifest
// declares binaryField + a non-empty outputPath comes in via params. It pulls
// the named base64 field out of the raw CDP result, decodes it, writes the
// bytes to outputPath, and returns a BinaryOutput envelope. Failures map to
// validation/internal categories so the caller can distinguish bad input
// (path errors) from CDP-side issues (which would have surfaced earlier).
//
// outputPath is taken at face value — the caller is trusted (this is a
// developer-facing CLI/daemon, not a multi-tenant service). The parent
// directory is created if missing.
func WriteBinaryFieldOutput(rawCDPResult json.RawMessage, fieldName, mimeType, outputPath string) Result {
	if outputPath == "" {
		return Fail(CategoryValidation, "output_path_empty",
			"outputPath must be a non-empty filesystem path", false)
	}

	var probe map[string]json.RawMessage
	if err := json.Unmarshal(rawCDPResult, &probe); err != nil {
		return Fail(CategoryProtocol, "binary_decode_envelope",
			fmt.Sprintf("CDP result not a JSON object: %v", err), false)
	}
	encoded, ok := probe[fieldName]
	if !ok {
		return Fail(CategoryProtocol, "binary_field_missing",
			fmt.Sprintf("CDP result has no field %q", fieldName), false)
	}
	var b64 string
	if err := json.Unmarshal(encoded, &b64); err != nil {
		return Fail(CategoryProtocol, "binary_field_not_string",
			fmt.Sprintf("CDP %q field is not a JSON string: %v", fieldName, err), false)
	}
	bytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return Fail(CategoryProtocol, "binary_decode_base64",
			fmt.Sprintf("CDP %q field is not valid base64: %v", fieldName, err), false)
	}

	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Fail(CategoryInternal, "output_mkdir_failed",
				fmt.Sprintf("create %s: %v", dir, err), false)
		}
	}
	if err := os.WriteFile(outputPath, bytes, 0o644); err != nil {
		return Fail(CategoryInternal, "output_write_failed",
			fmt.Sprintf("write %s: %v", outputPath, err), false)
	}
	return OK(BinaryOutput{
		Path:      outputPath,
		SizeBytes: int64(len(bytes)),
		MimeType:  mimeType,
	})
}
