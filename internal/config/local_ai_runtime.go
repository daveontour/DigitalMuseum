package config

import (
	"os"
	"strings"
	"sync"
)

// LocalAIRuntime holds hot-reloadable machine-wide Local AI settings (backed by user .env).
type LocalAIRuntime struct {
	mu sync.RWMutex

	chatModel   string
	cudaCPUOnly bool
}

var defaultLocalAIRuntime LocalAIRuntime

// InitLocalAIRuntime seeds the runtime store from the current process environment.
func InitLocalAIRuntime() {
	defaultLocalAIRuntime.mu.Lock()
	defer defaultLocalAIRuntime.mu.Unlock()
	defaultLocalAIRuntime.chatModel = strings.TrimSpace(os.Getenv("LOCALAI_MODEL_NAME"))
	if defaultLocalAIRuntime.chatModel == "" {
		defaultLocalAIRuntime.chatModel = "local-model"
	}
	defaultLocalAIRuntime.cudaCPUOnly = strings.TrimSpace(os.Getenv("CUDA_VISIBLE_DEVICES")) == "-1"
}

// LocalAIRuntimeStore returns the process-wide Local AI runtime config.
func LocalAIRuntimeStore() *LocalAIRuntime {
	return &defaultLocalAIRuntime
}

// LocalAISettings is the API shape for GET/POST /api/local-ai/settings.
type LocalAISettings struct {
	ChatModel   string `json:"chat_model"`
	CudaCPUOnly bool   `json:"cuda_cpu_only"`
}

// Snapshot returns the current effective settings.
func (r *LocalAIRuntime) Snapshot() LocalAISettings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return LocalAISettings{
		ChatModel:   r.chatModel,
		CudaCPUOnly: r.cudaCPUOnly,
	}
}

// ChatModel returns the configured Ollama chat model name.
func (r *LocalAIRuntime) ChatModel() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.chatModel
}

// CudaCPUOnly reports whether CUDA_VISIBLE_DEVICES=-1 should be used for Ollama.
func (r *LocalAIRuntime) CudaCPUOnly() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cudaCPUOnly
}

// Apply updates runtime state, the current process environment, and the user .env file.
func (r *LocalAIRuntime) Apply(chatModel string, cudaCPUOnly bool) error {
	chatModel = strings.TrimSpace(chatModel)
	if chatModel == "" {
		return errLocalAIChatModelRequired
	}

	upserts := map[string]string{
		"LOCALAI_MODEL_NAME": chatModel,
	}
	var remove []string
	if cudaCPUOnly {
		upserts["CUDA_VISIBLE_DEVICES"] = "-1"
	} else {
		remove = append(remove, "CUDA_VISIBLE_DEVICES")
	}
	if err := WriteUserEnvKeys(upserts, remove); err != nil {
		return err
	}

	_ = os.Setenv("LOCALAI_MODEL_NAME", chatModel)
	if cudaCPUOnly {
		_ = os.Setenv("CUDA_VISIBLE_DEVICES", "-1")
	} else {
		_ = os.Unsetenv("CUDA_VISIBLE_DEVICES")
	}

	r.mu.Lock()
	r.chatModel = chatModel
	r.cudaCPUOnly = cudaCPUOnly
	r.mu.Unlock()
	return nil
}

type localAIApplyError string

func (e localAIApplyError) Error() string { return string(e) }

const errLocalAIChatModelRequired localAIApplyError = "chat_model must not be empty"
