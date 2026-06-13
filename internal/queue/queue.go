package queue

import (
	"errors"
	"log/slog"
	"sync/atomic"
)

// Job is a unit of work the worker pool executes.
type Job struct {
	Execute func()
}

// Queue is a bounded channel-based job queue.
type Queue struct {
	jobs    chan Job
	size    int
	pending atomic.Int64
}

var ErrQueueFull = errors.New("queue full")

func New(size int) *Queue {
	slog.Info("[queue] initialized", "size", size)
	return &Queue{
		jobs: make(chan Job, size),
		size: size,
	}
}

// Submit enqueues a job. Returns ErrQueueFull immediately if the queue is at capacity.
func (q *Queue) Submit(job Job) error {
	select {
	case q.jobs <- job:
		pending := q.pending.Add(1)
		slog.Info("[queue] job enqueued", "pending", pending, "capacity", q.size)
		return nil
	default:
		slog.Warn("[queue] queue full — rejecting job", "capacity", q.size)
		return ErrQueueFull
	}
}

// Jobs returns the channel workers read from.
func (q *Queue) Jobs() <-chan Job {
	return q.jobs
}

// Done decrements the pending counter — called by workers after execution.
func (q *Queue) Done() {
	pending := q.pending.Add(-1)
	slog.Info("[queue] job completed", "pending", pending)
}

// Close shuts down the queue channel.
func (q *Queue) Close() {
	close(q.jobs)
}
