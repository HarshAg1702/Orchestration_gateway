package models

// ChatRequest is the payload received from WebSocket clients.
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse is the payload sent back to WebSocket clients.
type ChatResponse struct {
	Token  string `json:"token,omitempty"`
	Done   bool   `json:"done"`
	Error  string `json:"error,omitempty"`
	Source string `json:"source,omitempty"` // "redis", "qdrant", "model"
	Model  string `json:"model,omitempty"`
}

// CachedResponse is what gets stored in Redis.
type CachedResponse struct {
	Response string `json:"response"`
	Model    string `json:"model"`
}
