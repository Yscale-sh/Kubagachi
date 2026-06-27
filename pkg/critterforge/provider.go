package critterforge

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// BuildImageModel selects an image provider from a shared set of flags so every
// CLI (generate, sheet, spriteanim) behaves identically. Gemini is the default
// — it's what the original critters were generated with and produces crisper,
// more consistent sheets. Provider credentials/config are read from the
// environment (GEMINI_API_KEY, OPENAI_API_KEY / OPEN_AI_API_KEY, FLUX_ENDPOINT,
// optional FLUX_API_KEY, and FLUX_TIMEOUT_SECONDS).
//
//	provider: "gemini" (default) | "openai" | "flux"
//	model:    "" → the provider's default model
//	size:     OpenAI WxH (e.g. "1536x1024"); mapped to a Gemini aspect ratio
//	quality:  "low" | "medium" | "high" (Gemini: 1K | 2K | 4K)
func BuildImageModel(provider, model, size, quality string) (ImageModel, error) {
	switch provider {
	case "", "gemini":
		key := geminiAPIKey()
		if key == "" {
			return nil, errors.New("GEMINI_API_KEY is not set (add it to .env or export it)")
		}
		if model == "" {
			model = DefaultGeminiImageModel
		}
		return NewGeminiModel(GeminiOptions{
			APIKey:      key,
			Model:       model,
			AspectRatio: sizeToAspect(size),
			ImageSize:   qualityToImageSize(quality),
		})
	case "openai":
		key := openAIAPIKey()
		if key == "" {
			return nil, errors.New("OPENAI_API_KEY or OPEN_AI_API_KEY is not set (add it to .env or export it)")
		}
		if model == "" {
			model = DefaultOpenAIImageModel
		}
		return NewOpenAIModel(OpenAIOptions{
			APIKey:  key,
			Model:   model,
			Size:    size,
			Quality: quality,
		})
	case "flux":
		timeoutSeconds, err := fluxTimeoutSeconds()
		if err != nil {
			return nil, err
		}
		return NewFluxModel(FluxOptions{
			Endpoint:       fluxEndpoint(),
			APIKey:         fluxAPIKey(),
			Model:          model,
			TimeoutSeconds: timeoutSeconds,
		})
	default:
		return nil, fmt.Errorf("unknown --provider %q (use gemini, openai, or flux)", provider)
	}
}

func openAIAPIKey() string {
	if v := strings.TrimSpace(os.Getenv("OPENAI_API_KEY")); v != "" {
		return v
	}
	return strings.TrimSpace(os.Getenv("OPEN_AI_API_KEY"))
}

func geminiAPIKey() string {
	return strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
}

func fluxEndpoint() string {
	if v := strings.TrimSpace(os.Getenv("FLUX_ENDPOINT")); v != "" {
		return v
	}
	return defaultFluxEndpoint
}

func fluxAPIKey() string {
	return strings.TrimSpace(os.Getenv("FLUX_API_KEY"))
}

func fluxTimeoutSeconds() (int, error) {
	v := strings.TrimSpace(os.Getenv("FLUX_TIMEOUT_SECONDS"))
	if v == "" {
		return defaultFluxTimeoutSeconds, nil
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("FLUX_TIMEOUT_SECONDS must be a positive integer, got %q", v)
	}
	return seconds, nil
}

// qualityToImageSize maps --quality onto Gemini's image-size tiers (high =
// sharpest/largest).
func qualityToImageSize(quality string) string {
	switch quality {
	case "high":
		return "4K"
	case "medium":
		return "2K"
	default:
		return "1K"
	}
}

// sizeToAspect maps an OpenAI-style WxH size onto a Gemini aspect ratio.
func sizeToAspect(size string) string {
	switch size {
	case "1536x1024":
		return "3:2"
	case "1024x1536":
		return "2:3"
	default:
		return "1:1"
	}
}
