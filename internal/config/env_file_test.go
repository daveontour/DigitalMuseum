package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertEnvVarInContent(t *testing.T) {
	base := "FOO=bar\nLOCALAI_MODEL_NAME=old\n"
	got := UpsertEnvVarInContent(base, "LOCALAI_MODEL_NAME", "gemma4:e2b")
	if !strings.Contains(got, "LOCALAI_MODEL_NAME=gemma4:e2b") {
		t.Fatalf("expected updated model line, got %q", got)
	}
	if !strings.Contains(got, "FOO=bar") {
		t.Fatalf("expected FOO preserved, got %q", got)
	}

	got2 := UpsertEnvVarInContent("A=1\n", "CUDA_VISIBLE_DEVICES", "-1")
	if !strings.Contains(got2, "CUDA_VISIBLE_DEVICES=-1") {
		t.Fatalf("expected inserted cuda line, got %q", got2)
	}
}

func TestRemoveEnvVarFromContent(t *testing.T) {
	base := "A=1\nCUDA_VISIBLE_DEVICES=-1\nB=2\n"
	got := RemoveEnvVarFromContent(base, "CUDA_VISIBLE_DEVICES")
	if strings.Contains(got, "CUDA_VISIBLE_DEVICES") {
		t.Fatalf("expected cuda removed, got %q", got)
	}
	if !strings.Contains(got, "B=2") {
		t.Fatalf("expected B preserved, got %q", got)
	}
}

func TestLocalAIRuntimeApply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)

	InitLocalAIRuntime()
	rt := LocalAIRuntimeStore()
	if err := rt.Apply("gemma4:latest", true); err != nil {
		t.Fatal(err)
	}
	if rt.ChatModel() != "gemma4:latest" {
		t.Fatalf("chat model %q", rt.ChatModel())
	}
	if !rt.CudaCPUOnly() {
		t.Fatal("expected cuda cpu only")
	}
	path := filepath.Join(dir, "Digital Museum", ".env")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "LOCALAI_MODEL_NAME=gemma4:latest") {
		t.Fatalf("env file %q", body)
	}
	if !strings.Contains(body, "CUDA_VISIBLE_DEVICES=-1") {
		t.Fatalf("env file %q", body)
	}

	if err := rt.Apply("llama3.2", false); err != nil {
		t.Fatal(err)
	}
	b2, _ := os.ReadFile(path)
	if strings.Contains(string(b2), "CUDA_VISIBLE_DEVICES") {
		t.Fatalf("expected cuda removed, got %q", string(b2))
	}
}
