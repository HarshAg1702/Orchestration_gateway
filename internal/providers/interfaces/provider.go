package interfaces

import "context"

// Provider abstracts an LLM backend.
type Provider interface {
	// Stream sends the prompt and writes tokens to the tokenCh channel.
	// It closes tokenCh when done or on error.
	Stream(ctx context.Context, prompt string, tokenCh chan<- string) error
	Health(ctx context.Context) error
	Name() string
}
