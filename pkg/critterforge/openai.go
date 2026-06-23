package critterforge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
)

const DefaultOpenAIImageModel = "gpt-image-1.5"

type OpenAIOptions struct {
	APIKey  string
	Model   string
	Size    string
	Quality string
}

type openAIModel struct {
	apiKey  string
	model   string
	size    string
	quality string
	client  *http.Client
}

func NewOpenAIModel(opts OpenAIOptions) (ImageModel, error) {
	if opts.APIKey == "" {
		return nil, errors.New("critterforge: OPENAI_API_KEY is empty")
	}
	if opts.Model == "" {
		opts.Model = DefaultOpenAIImageModel
	}
	if opts.Size == "" {
		opts.Size = "1024x1024"
	}
	if opts.Quality == "" {
		opts.Quality = "low"
	}
	return &openAIModel{
		apiKey:  opts.APIKey,
		model:   opts.Model,
		size:    opts.Size,
		quality: opts.Quality,
		client:  http.DefaultClient,
	}, nil
}

func (m *openAIModel) ID() string {
	return "openai:" + m.model + ":" + m.size + ":" + m.quality
}

func (m *openAIModel) GenerateSprite(ctx context.Context, prompt string, references ...[]byte) ([]byte, error) {
	if len(references) == 0 {
		return m.generate(ctx, prompt)
	}
	return m.edit(ctx, prompt, references...)
}

func (m *openAIModel) generate(ctx context.Context, prompt string) ([]byte, error) {
	body := map[string]any{
		"model":         m.model,
		"prompt":        prompt,
		"n":             1,
		"size":          m.size,
		"quality":       m.quality,
		"background":    "transparent",
		"output_format": "png",
	}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/generations", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", "application/json")
	return m.do(req)
}

func (m *openAIModel) edit(ctx context.Context, prompt string, references ...[]byte) ([]byte, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{
		"model":         m.model,
		"prompt":        prompt,
		"n":             "1",
		"size":          m.size,
		"quality":       m.quality,
		"background":    "transparent",
		"output_format": "png",
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return nil, err
		}
	}
	// OpenAI's images/edits expects field "image" for a single file and
	// "image[]" for the array form. Use the right one based on count.
	// We can't use multipart.Writer.CreateFormFile because it hard-codes
	// Content-Type: application/octet-stream, which OpenAI rejects with
	// "unsupported mimetype" — we need to set image/png explicitly.
	fieldName := "image"
	if len(references) > 1 {
		fieldName = "image[]"
	}
	for i, ref := range references {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", fmt.Sprintf(
			`form-data; name=%q; filename=%q`,
			fieldName,
			"reference-"+strconv.Itoa(i)+".png",
		))
		h.Set("Content-Type", "image/png")
		part, err := w.CreatePart(h)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write(ref); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.openai.com/v1/images/edits", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+m.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return m.do(req)
}

func (m *openAIModel) do(req *http.Request) ([]byte, error) {
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
		return nil, fmt.Errorf("openai image request failed: %s: %s", resp.Status, string(data))
	}
	var out struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode openai image response: %w", err)
	}
	if len(out.Data) == 0 || out.Data[0].B64JSON == "" {
		return nil, errors.New("openai image response contained no image data")
	}
	png, err := base64.StdEncoding.DecodeString(out.Data[0].B64JSON)
	if err != nil {
		return nil, fmt.Errorf("decode openai image: %w", err)
	}
	return png, nil
}
