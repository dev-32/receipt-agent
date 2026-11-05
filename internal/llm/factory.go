package llm

import (
	"fmt"
	"os"
)

// FromEnv picks a provider. Default is Groq's free tier. Every default can be overridden.
//
//	LLM_PROVIDER = groq | gemini | ollama | openrouter | anthropic   (default groq)
//	LLM_API_KEY  = key for that provider (not needed for ollama)
//	LLM_MODEL / LLM_BASE_URL override the defaults below (free-tier model names change, check the provider's docs)
func FromEnv() (Client, string, error) {
	provider := getenv("LLM_PROVIDER", "groq")
	key := os.Getenv("LLM_API_KEY")
	type preset struct {
		base, model string
		needKey     bool
	}
	presets := map[string]preset{
		"groq":       {"https://api.groq.com/openai/v1", "llama-3.3-70b-versatile", true},
		"gemini":     {"https://generativelanguage.googleapis.com/v1beta/openai", "gemini-2.0-flash", true},
		"openrouter": {"https://openrouter.ai/api/v1", "meta-llama/llama-3.3-70b-instruct:free", true},
		"ollama":     {"http://localhost:11434/v1", "qwen2.5:7b", false},
	}
	if provider == "anthropic" {
		if key == "" {
			key = os.Getenv("ANTHROPIC_API_KEY")
		}
		if key == "" {
			return nil, "", fmt.Errorf("LLM_API_KEY required for anthropic")
		}
		m := getenv("LLM_MODEL", "claude-sonnet-5-5")
		return NewAnthropic(key, m), "anthropic/" + m, nil
	}
	p, ok := presets[provider]
	if !ok {
		return nil, "", fmt.Errorf("unknown LLM_PROVIDER %q", provider)
	}
	if p.needKey && key == "" {
		return nil, "", fmt.Errorf("LLM_API_KEY required for %s (free key, no card needed)", provider)
	}
	model := getenv("LLM_MODEL", p.model)
	return NewOpenAICompat(getenv("LLM_BASE_URL", p.base), key, model), provider + "/" + model, nil
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
