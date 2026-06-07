package embeddings

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	body, _ := json.Marshal(embedRequest{Model: s.model, Prompt: text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/api/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embed: status %d: %s", resp.StatusCode, string(b))
	}

	var er embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&er); err != nil {
		return nil, err
	}
	return er.Embedding, nil
}
