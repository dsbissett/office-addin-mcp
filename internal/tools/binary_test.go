package tools

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteBinaryFieldOutput_Success(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "nested", "image.png")
	payload := []byte("hello-binary-bytes")
	b64 := base64.StdEncoding.EncodeToString(payload)
	raw, err := json.Marshal(map[string]string{"data": b64})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	res := WriteBinaryFieldOutput(raw, "data", "image/png", out)
	if res.Err != nil {
		t.Fatalf("unexpected error: %+v", res.Err)
	}
	bo, ok := res.Data.(BinaryOutput)
	if !ok {
		t.Fatalf("Data is %T want BinaryOutput", res.Data)
	}
	if bo.Path != out {
		t.Errorf("Path=%q want %q", bo.Path, out)
	}
	if bo.SizeBytes != int64(len(payload)) {
		t.Errorf("SizeBytes=%d want %d", bo.SizeBytes, len(payload))
	}
	if bo.MimeType != "image/png" {
		t.Errorf("MimeType=%q want image/png", bo.MimeType)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("written bytes=%q want %q", got, payload)
	}
}

func TestWriteBinaryFieldOutput_EmptyPath(t *testing.T) {
	res := WriteBinaryFieldOutput(json.RawMessage(`{"data":"x"}`), "data", "", "")
	if res.Err == nil || res.Err.Code != "output_path_empty" {
		t.Fatalf("want output_path_empty, got %+v", res.Err)
	}
	if res.Err.Category != CategoryValidation {
		t.Errorf("category=%q want validation", res.Err.Category)
	}
}

func TestWriteBinaryFieldOutput_NotAnObject(t *testing.T) {
	dir := t.TempDir()
	res := WriteBinaryFieldOutput(json.RawMessage(`["not","an","object"]`),
		"data", "", filepath.Join(dir, "x.bin"))
	if res.Err == nil || res.Err.Code != "binary_decode_envelope" {
		t.Fatalf("want binary_decode_envelope, got %+v", res.Err)
	}
	if res.Err.Category != CategoryProtocol {
		t.Errorf("category=%q want protocol", res.Err.Category)
	}
}

func TestWriteBinaryFieldOutput_FieldMissing(t *testing.T) {
	dir := t.TempDir()
	res := WriteBinaryFieldOutput(json.RawMessage(`{"other":"v"}`),
		"data", "", filepath.Join(dir, "x.bin"))
	if res.Err == nil || res.Err.Code != "binary_field_missing" {
		t.Fatalf("want binary_field_missing, got %+v", res.Err)
	}
}

func TestWriteBinaryFieldOutput_FieldNotString(t *testing.T) {
	dir := t.TempDir()
	res := WriteBinaryFieldOutput(json.RawMessage(`{"data":123}`),
		"data", "", filepath.Join(dir, "x.bin"))
	if res.Err == nil || res.Err.Code != "binary_field_not_string" {
		t.Fatalf("want binary_field_not_string, got %+v", res.Err)
	}
}

func TestWriteBinaryFieldOutput_BadBase64(t *testing.T) {
	dir := t.TempDir()
	res := WriteBinaryFieldOutput(json.RawMessage(`{"data":"!!!not base64!!!"}`),
		"data", "", filepath.Join(dir, "x.bin"))
	if res.Err == nil || res.Err.Code != "binary_decode_base64" {
		t.Fatalf("want binary_decode_base64, got %+v", res.Err)
	}
}

func TestWriteBinaryFieldOutput_MkdirFailure(t *testing.T) {
	// Create a regular file, then try to write underneath it as if it were a
	// directory — MkdirAll fails because a path component is a file.
	dir := t.TempDir()
	fileAsDir := filepath.Join(dir, "blocker")
	if err := os.WriteFile(fileAsDir, []byte("x"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	b64 := base64.StdEncoding.EncodeToString([]byte("data"))
	raw, err := json.Marshal(map[string]string{"data": b64})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out := filepath.Join(fileAsDir, "sub", "x.bin")
	res := WriteBinaryFieldOutput(raw, "data", "", out)
	if res.Err == nil {
		t.Fatalf("expected failure writing under a file, got Data=%+v", res.Data)
	}
	// On most platforms MkdirAll fails (output_mkdir_failed); if the OS allows
	// the mkdir but blocks the write it surfaces as output_write_failed. Both
	// are CategoryInternal — assert the category and that one of the two codes
	// fired.
	if res.Err.Category != CategoryInternal {
		t.Errorf("category=%q want internal", res.Err.Category)
	}
	if res.Err.Code != "output_mkdir_failed" && res.Err.Code != "output_write_failed" {
		t.Errorf("code=%q want output_mkdir_failed or output_write_failed", res.Err.Code)
	}
}

func TestWriteBinaryFieldOutput_WriteFailureDirTarget(t *testing.T) {
	// outputPath points at an existing directory: parent-dir MkdirAll succeeds
	// but WriteFile fails because the path is a directory.
	dir := t.TempDir()
	b64 := base64.StdEncoding.EncodeToString([]byte("data"))
	raw, err := json.Marshal(map[string]string{"data": b64})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res := WriteBinaryFieldOutput(raw, "data", "", dir)
	if res.Err == nil || res.Err.Code != "output_write_failed" {
		t.Fatalf("want output_write_failed, got %+v", res.Err)
	}
	if res.Err.Category != CategoryInternal {
		t.Errorf("category=%q want internal", res.Err.Category)
	}
}
