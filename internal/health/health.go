package health

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Checker interface {
	Ping(ctx context.Context) error
}

type ProviderChecker interface {
	Health(ctx context.Context) error
	Name() string
}

type Status struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Services  map[string]string `json:"services"`
}

func Handler(redis Checker, providers []ProviderChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		services := make(map[string]string)
		overall := "ok"

		if err := redis.Ping(ctx); err != nil {
			services["redis"] = "down: " + err.Error()
			overall = "degraded"
		} else {
			services["redis"] = "ok"
		}

		for _, p := range providers {
			if err := p.Health(ctx); err != nil {
				services[p.Name()] = "down: " + err.Error()
				overall = "degraded"
			} else {
				services[p.Name()] = "ok"
			}
		}

		status := Status{
			Status:    overall,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Services:  services,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	}
}
