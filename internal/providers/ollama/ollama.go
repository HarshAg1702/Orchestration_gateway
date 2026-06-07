package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Provider struct {
	baseURL string
	model   string
	client  *http.Client
}

func New(baseURL, model string) *Provider {
	return &Provider{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{},
	}
}

func (p *Provider) Name() string { return p.model }

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

func (p *Provider) Stream(ctx context.Context, prompt string, tokenCh chan<- string) error {
	defer close(tokenCh)

	body, _ := json.Marshal(generateRequest{Model: p.model, Prompt: prompt, Stream: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama %s: status %d: %s", p.model, resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk generateChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Response != "" {
			select {
			case tokenCh <- chunk.Response:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}

func (p *Provider) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama health check failed: %d", resp.StatusCode)
	}
	return nil
}
