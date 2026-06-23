package critterforge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// DefaultGeminiImageModel is the model the original critters were generated
// with — and the one the pipeline favours for crisp, consistent output.
const DefaultGeminiImageModel = "gemini-3-pro-image-preview"

// GeminiOptions configures the Gemini image model.
type GeminiOptions struct {
	APIKey      string
	Model       string // default: gemini-3-pro-image-preview
	AspectRatio string // "1:1", "3:2", "16:9", "21:9", … (default "1:1")
	ImageSize   string // "1K" | "2K" | "4K" (default "2K")
}

type geminiModel struct {
	apiKey      string
	model       string
	aspectRatio string
	imageSize   string
	client      *http.Client
}

// NewGeminiModel builds a Gemini image model implementing ImageModel via the
// generativelanguage REST API (generateContent with an IMAGE response
// modality). No SDK — plain HTTP, mirroring openAIModel.
func NewGeminiModel(opts GeminiOptions) (ImageModel, error) {
	if opts.APIKey == "" {
		return nil, errors.New("critterforge: GEMINI_API_KEY is empty")
	}
	if opts.Model == "" {
		opts.Model = DefaultGeminiImageModel
	}
	if opts.AspectRatio == "" {
		opts.AspectRatio = "1:1"
	}
	if opts.ImageSize == "" {
		opts.ImageSize = "2K"
	}
	return &geminiModel{
		apiKey:      opts.APIKey,
		model:       opts.Model,
		aspectRatio: opts.AspectRatio,
		imageSize:   opts.ImageSize,
		client:      http.DefaultClient,
	}, nil
}

func (m *geminiModel) ID() string {
	return "gemini:" + m.model + ":" + m.aspectRatio + ":" + m.imageSize
}

// geminiInlineData is the request/response inline-image envelope. The REST API
// accepts snake_case on the way in; on the way out it can be either casing, so
// the response type below decodes both.
type geminiInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type geminiPart struct {
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inline_data,omitempty"`
}

func (m *geminiModel) GenerateSprite(ctx context.Context, prompt string, references ...[]byte) ([]byte, error) {
	parts := []geminiPart{{Text: prompt}}
	for _, ref := range references {
		parts = append(parts, geminiPart{InlineData: &geminiInlineData{
			MimeType: "image/png",
			Data:     base64.StdEncoding.EncodeToString(ref),
		}})
	}
	body := map[string]any{
		"contents": []map[string]any{{
			"role":  "user",
			"parts": parts,
		}},
		"generationConfig": map[string]any{
			"responseModalities": []string{"TEXT", "IMAGE"},
			"imageConfig": map[string]any{
				"aspectRatio": m.aspectRatio,
				"imageSize":   m.imageSize,
			},
		},
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/" + m.model + ":generateContent"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", m.apiKey)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini image request failed: %s: %s", resp.Status, snippet(data))
	}

	// candidates[].content.parts[].inline_data.data (or inlineData on the way out).
	var out struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Snake *geminiInlineData `json:"inline_data"`
					Camel *geminiInlineData `json:"inlineData"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	for _, c := range out.Candidates {
		for _, p := range c.Content.Parts {
			var b64 string
			if p.Snake != nil {
				b64 = p.Snake.Data
			}
			if b64 == "" && p.Camel != nil {
				b64 = p.Camel.Data
			}
			if b64 != "" {
				png, derr := base64.StdEncoding.DecodeString(b64)
				if derr != nil {
					return nil, fmt.Errorf("decode gemini image: %w", derr)
				}
				return png, nil
			}
		}
	}
	return nil, fmt.Errorf("gemini response contained no image data: %s", snippet(data))
}

func snippet(b []byte) string {
	const max = 400
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
