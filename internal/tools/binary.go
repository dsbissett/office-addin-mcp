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

	bytes, failure := decodeBinaryField(rawCDPResult, fieldName)
	if failure != nil {
		return *failure
	}
	if failure := writeBinaryFile(outputPath, bytes); failure != nil {
		return *failure
	}
	return OK(BinaryOutput{
		Path:      outputPath,
		SizeBytes: int64(len(bytes)),
		MimeType:  mimeType,
	})
}

// decodeBinaryField pulls fieldName out of the raw CDP object and base64-decodes
// it. On success it returns the bytes and a nil failure; on any error it returns
// nil bytes and a populated failure Result.
func decodeBinaryField(rawCDPResult json.RawMessage, fieldName string) ([]byte, *Result) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(rawCDPResult, &probe); err != nil {
		return nil, failPtr(CategoryProtocol, "binary_decode_envelope",
			fmt.Sprintf("CDP result not a JSON object: %v", err))
	}
	encoded, ok := probe[fieldName]
	if !ok {
		return nil, failPtr(CategoryProtocol, "binary_field_missing",
			fmt.Sprintf("CDP result has no field %q", fieldName))
	}
	var b64 string
	if err := json.Unmarshal(encoded, &b64); err != nil {
		return nil, failPtr(CategoryProtocol, "binary_field_not_string",
			fmt.Sprintf("CDP %q field is not a JSON string: %v", fieldName, err))
	}
	bytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, failPtr(CategoryProtocol, "binary_decode_base64",
			fmt.Sprintf("CDP %q field is not valid base64: %v", fieldName, err))
	}
	return bytes, nil
}

// writeBinaryFile creates the parent directory if needed and writes bytes to
// outputPath. Returns a populated failure Result on error, nil on success.
func writeBinaryFile(outputPath string, bytes []byte) *Result {
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return failPtr(CategoryInternal, "output_mkdir_failed",
				fmt.Sprintf("create %s: %v", dir, err))
		}
	}
	if err := os.WriteFile(outputPath, bytes, 0o644); err != nil {
		return failPtr(CategoryInternal, "output_write_failed",
			fmt.Sprintf("write %s: %v", outputPath, err))
	}
	return nil
}

// failPtr builds a non-retryable failure Result and returns its address.
func failPtr(category, code, msg string) *Result {
	r := Fail(category, code, msg, false)
	return &r
}
