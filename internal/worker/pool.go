package worker

import (
	"log/slog"

	"github.com/harshagae/orchestration-gateway/internal/queue"
)

// Pool is a fixed-size worker pool that pulls jobs from a queue.
type Pool struct {
	workers int
	queue   *queue.Queue
}

func New(workers int, q *queue.Queue) *Pool {
	return &Pool{workers: workers, queue: q}
}

// Start launches worker goroutines. Call once at startup.
func (p *Pool) Start() {
	slog.Info("[worker pool] starting", "workers", p.workers)
	for i := range p.workers {
		go p.run(i)
	}
}

func (p *Pool) run(id int) {
	slog.Info("[worker] started", "worker_id", id)
	for job := range p.queue.Jobs() {
		slog.Info("[worker] picked up job", "worker_id", id)
		job.Execute()
		p.queue.Done()
		slog.Info("[worker] job done", "worker_id", id)
	}
	slog.Info("[worker] shutting down", "worker_id", id)
}
