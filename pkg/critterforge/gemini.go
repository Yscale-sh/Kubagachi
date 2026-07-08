package critterforge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

// DefaultGeminiImageModel is the model the original critters were generated
// with — and the one the pipeline favours for crisp, consistent output.
const DefaultGeminiImageModel = "gemini-3-pro-image-preview"

// Pricing for gemini-3-pro-image-preview (Nano Banana Pro), USD per 1M tokens,
// per ai.google.dev/gemini-api/docs/pricing (2026): input $2, text+thinking
// output $12, image output $120 (≈$0.134/image at 1K-2K, $0.24 at 4K). We log
// the real per-call cost from usageMetadata so margin is observable, not guessed.
const (
	usdPerMTokInput       = 2.0
	usdPerMTokTextOutput  = 12.0
	usdPerMTokImageOutput = 120.0
)

// fallbackImageTokens estimates image output tokens by size when the response
// omits the per-modality breakdown. Measured: 2K≈1120, 4K≈2000, else ~1290.
func fallbackImageTokens(imageSize string) int {
	switch strings.ToUpper(imageSize) {
	case "4K":
		return 2000
	case "1K", "2K":
		return 1120
	default:
		return 1290
	}
}

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

type GeminiRequestError struct {
	StatusCode int
	Status     string
	Body       string
}

func (e *GeminiRequestError) Error() string {
	return fmt.Sprintf("gemini image request failed: %s: %s", e.Status, snippet([]byte(e.Body)))
}

func (e *GeminiRequestError) Retriable() bool {
	return e.StatusCode == 429 || (e.StatusCode >= 500 && e.StatusCode < 600)
}

func IsRetryableError(err error) bool {
	var requestErr *GeminiRequestError
	if errors.As(err, &requestErr) {
		return requestErr.Retriable()
	}
	return false
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
			// gemini-3-pro-image does an interleaved "thinking" pass before it
			// emits the image; a dense multi-image deck prompt can burn well past
			// the default output ceiling and finish MAX_TOKENS with no image. The
			// image itself is only ~1290 tokens, so give thinking generous room.
			"maxOutputTokens": 32768,
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
		return nil, &GeminiRequestError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Body:       string(data),
		}
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
		UsageMetadata struct {
			PromptTokenCount        int                 `json:"promptTokenCount"`
			CandidatesTokenCount    int                 `json:"candidatesTokenCount"`
			ThoughtsTokenCount      int                 `json:"thoughtsTokenCount"`
			TotalTokenCount         int                 `json:"totalTokenCount"`
			CandidatesTokensDetails []geminiTokenDetail `json:"candidatesTokensDetails"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode gemini response: %w", err)
	}
	m.logCost(out.UsageMetadata.PromptTokenCount, out.UsageMetadata.CandidatesTokenCount,
		out.UsageMetadata.ThoughtsTokenCount, out.UsageMetadata.TotalTokenCount,
		imageTokensFromDetails(out.UsageMetadata.CandidatesTokensDetails, m.imageSize))
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

// geminiTokenDetail is one per-modality slice of usageMetadata token counts.
type geminiTokenDetail struct {
	Modality   string `json:"modality"`
	TokenCount int    `json:"tokenCount"`
}

// imageTokensFromDetails sums the IMAGE-modality output tokens; falls back to a
// size-based estimate when the breakdown is absent.
func imageTokensFromDetails(details []geminiTokenDetail, imageSize string) int {
	img := 0
	for _, d := range details {
		if strings.EqualFold(d.Modality, "IMAGE") {
			img += d.TokenCount
		}
	}
	if img == 0 {
		return fallbackImageTokens(imageSize)
	}
	return img
}

// logCost prints the measured per-call token usage and the estimated USD cost,
// splitting output into image ($120/1M) vs text+thinking ($12/1M). One line per
// Gemini call so real generation cost and $10-price margin are observable in logs.
func (m *geminiModel) logCost(prompt, candidates, thoughts, total, imageTok int) {
	// text+thinking billed at $12/1M: visible text output (candidates minus the
	// image slice) plus the thinking pass.
	textOut := thoughts
	if candidates > imageTok {
		textOut += candidates - imageTok
	}
	usd := float64(prompt)*usdPerMTokInput/1e6 +
		float64(imageTok)*usdPerMTokImageOutput/1e6 +
		float64(textOut)*usdPerMTokTextOutput/1e6
	log.Printf("critterforge gemini cost: model=%s size=%s input_tok=%d image_tok=%d think_tok=%d text_out_tok=%d total_tok=%d est_usd=$%.4f",
		m.model, m.imageSize, prompt, imageTok, thoughts, textOut, total, usd)
}

func snippet(b []byte) string {
	const max = 400
	if len(b) > max {
		return string(b[:max]) + "…"
	}
	return string(b)
}
