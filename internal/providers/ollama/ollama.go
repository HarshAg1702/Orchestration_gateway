package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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

	slog.Info("[ollama] starting stream", "model", p.model, "prompt", prompt)

	body, _ := json.Marshal(generateRequest{Model: p.model, Prompt: prompt, Stream: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		slog.Error("[ollama] failed to build request", "model", p.model, "err", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Info("[ollama] sending request to ollama", "model", p.model, "url", p.baseURL+"/api/generate")
	resp, err := p.client.Do(req)
	if err != nil {
		slog.Error("[ollama] http request failed", "model", p.model, "err", err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		slog.Error("[ollama] non-200 response", "model", p.model, "status", resp.StatusCode, "body", string(b))
		return fmt.Errorf("ollama %s: status %d: %s", p.model, resp.StatusCode, string(b))
	}

	slog.Info("[ollama] response stream started", "model", p.model)
	tokenCount := 0
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk generateChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Response != "" {
			tokenCount++
			select {
			case tokenCh <- chunk.Response:
			case <-ctx.Done():
				slog.Warn("[ollama] context cancelled mid-stream", "model", p.model, "tokens_sent", tokenCount)
				return ctx.Err()
			}
		}
		if chunk.Done {
			slog.Info("[ollama] stream complete", "model", p.model, "total_tokens", tokenCount)
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
