package qdrantcache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Cache struct {
	baseURL    string
	collection string
	threshold  float64
	ttlDays    int
	client     *http.Client
}

func New(addr, collection string, threshold float64, ttlDays int) *Cache {
	return &Cache{
		baseURL:    "http://" + addr,
		collection: collection,
		threshold:  threshold,
		ttlDays:    ttlDays,
		client:     &http.Client{},
	}
}

type SearchRequest struct {
	Vector     []float32 `json:"vector"`
	Limit      int       `json:"limit"`
	WithPayload bool     `json:"with_payload"`
	ScoreThreshold float64 `json:"score_threshold"`
}

type SearchResult struct {
	Result []struct {
		Score   float64 `json:"score"`
		Payload map[string]interface{} `json:"payload"`
	} `json:"result"`
}

func (c *Cache) Search(ctx context.Context, embedding []float32) (string, string, error) {
	slog.Info("[qdrant] searching semantic cache", "collection", c.collection, "threshold", c.threshold)

	sr := SearchRequest{
		Vector:         embedding,
		Limit:          3,
		WithPayload:    true,
		ScoreThreshold: c.threshold,
	}
	b, _ := json.Marshal(sr)

	url := fmt.Sprintf("%s/collections/%s/points/search", c.baseURL, c.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		slog.Error("[qdrant] failed to build search request", "err", err)
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		slog.Error("[qdrant] search http request failed", "err", err)
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		slog.Error("[qdrant] non-200 response", "status", resp.StatusCode, "body", string(rb))
		return "", "", fmt.Errorf("qdrant search: status %d: %s", resp.StatusCode, string(rb))
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("[qdrant] failed to decode search response", "err", err)
		return "", "", err
	}

	if len(result.Result) == 0 {
		slog.Info("[qdrant] cache miss — no results above threshold")
		return "", "", nil
	}

	payload := result.Result[0].Payload
	response, _ := payload["response"].(string)
	model, _ := payload["model"].(string)
	slog.Info("[qdrant] cache hit", "score", result.Result[0].Score, "model", model)
	return response, model, nil
}

type UpsertRequest struct {
	Points []Point `json:"points"`
}

type Point struct {
	ID      string                 `json:"id"`
	Vector  []float32              `json:"vector"`
	Payload map[string]interface{} `json:"payload"`
}

func (c *Cache) Store(ctx context.Context, embedding []float32, prompt, response, model string) error {
	slog.Info("[qdrant] storing response", "collection", c.collection, "model", model, "prompt", prompt)
	pt := Point{
		ID:     uuid.New().String(),
		Vector: embedding,
		Payload: map[string]interface{}{
			"prompt":     prompt,
			"response":   response,
			"model":      model,
			"created_at": time.Now().Unix(),
		},
	}

	ur := UpsertRequest{Points: []Point{pt}}
	b, _ := json.Marshal(ur)

	url := fmt.Sprintf("%s/collections/%s/points", c.baseURL, c.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert: status %d: %s", resp.StatusCode, string(rb))
	}
	return nil
}

func (c *Cache) EnsureCollection(ctx context.Context, vectorSize int) error {
	checkURL := fmt.Sprintf("%s/collections/%s", c.baseURL, c.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil // already exists
	}

	createBody := map[string]interface{}{
		"vectors": map[string]interface{}{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	b, _ := json.Marshal(createBody)
	createReq, err := http.NewRequestWithContext(ctx, http.MethodPut, checkURL, bytes.NewReader(b))
	if err != nil {
		return err
	}
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := c.client.Do(createReq)
	if err != nil {
		return err
	}
	createResp.Body.Close()
	return nil
}
