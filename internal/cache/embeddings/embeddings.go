package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type Service struct {
	baseURL string
	model   string
	client  *http.Client
}

func New(baseURL, model string) *Service {
	return &Service{baseURL: baseURL, model: model, client: &http.Client{}}
}

type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

func (s *Service) Embed(ctx context.Context, text string) ([]float32, error) {
	start := time.Now()
	slog.Info("[embeddings] generating embedding", "model", s.model, "text", text)

	body, _ := json.Marshal(embedRequest{Model: s.model, Prompt: text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		slog.Error("[embeddings] failed to build request", "err", err)
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		slog.Error("[embeddings] http request failed", "model", s.model, "err", err)
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		slog.Error("[embeddings] non-200 response", "status", resp.StatusCode, "body", string(b))
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(b))
	}

	var er embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		slog.Error("[embeddings] failed to decode response", "err", err)
		return nil, err
	}

	slog.Info("[embeddings] embedding generated", "model", s.model, "dimensions", len(er.Embedding), "latency_ms", time.Since(start).Milliseconds())
	return er.Embedding, nil
}
