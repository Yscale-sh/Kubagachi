package critterforge

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultFluxImageModel     = "kontext"
	defaultFluxEndpoint       = "http://localhost:8000"
	defaultFluxTimeoutSeconds = 300
)

// FluxOptions configures the self-hosted FLUX image model.
type FluxOptions struct {
	Endpoint       string
	APIKey         string
	Model          string
	TimeoutSeconds int
}

type fluxModel struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

// NewFluxModel builds a FLUX image model implementing ImageModel via a
// self-hosted HTTP inference server.
func NewFluxModel(opts FluxOptions) (ImageModel, error) {
	endpoint, err := normalizeFluxEndpoint(opts.Endpoint)
	if err != nil {
		return nil, err
	}
	if opts.Model == "" {
		opts.Model = DefaultFluxImageModel
	}
	if opts.TimeoutSeconds == 0 {
		opts.TimeoutSeconds = defaultFluxTimeoutSeconds
	}
	if opts.TimeoutSeconds < 0 {
		return nil, fmt.Errorf("critterforge: FLUX_TIMEOUT_SECONDS must be positive, got %d", opts.TimeoutSeconds)
	}
	return &fluxModel{
		endpoint: endpoint,
		apiKey:   strings.TrimSpace(opts.APIKey),
		model:    opts.Model,
		client: &http.Client{
			Timeout: time.Duration(opts.TimeoutSeconds) * time.Second,
		},
	}, nil
}

func (m *fluxModel) ID() string {
	return "flux:" + m.model
}

func (m *fluxModel) GenerateSprite(ctx context.Context, prompt string, references ...[]byte) ([]byte, error) {
	refs := make([]string, 0, len(references))
	for _, ref := range references {
		refs = append(refs, base64.StdEncoding.EncodeToString(ref))
	}
	body := struct {
		Prompt     string   `json:"prompt"`
		References []string `json:"references"`
	}{
		Prompt:     prompt,
		References: refs,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(body); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.endpoint+"/generate", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
	}

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
		return nil, fmt.Errorf("flux image request failed: %s: %s", resp.Status, snippet(data))
	}
	return data, nil
}

func normalizeFluxEndpoint(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = defaultFluxEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("critterforge: invalid FLUX_ENDPOINT %q: %w", endpoint, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("critterforge: invalid FLUX_ENDPOINT %q: expected http or https URL", endpoint)
	}
	if u.Host == "" {
		return "", fmt.Errorf("critterforge: invalid FLUX_ENDPOINT %q: missing host", endpoint)
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("critterforge: invalid FLUX_ENDPOINT %q: query and fragment are not supported", endpoint)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}
