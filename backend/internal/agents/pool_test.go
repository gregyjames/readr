package agents

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestAgentPoolWorkerRecoversFromPanic(t *testing.T) {
	logger := zap.NewNop()
	pool := &AgentPool{
		Queue:      make(chan Job, 10),
		logger:     logger,
		activeJobs: make(map[int]ActiveJobInfo),
	}

	// 1. Verify executeJob directly recovers from a panicking job
	panickingJob := Job{
		ArticleID: 999,
		Type:      JobTypePipeline, // with no DB/Repo configured, processPipeline will safely return or panic
	}

	// executeJob must not panic even when internal operations panic
	assert.NotPanics(t, func() {
		pool.executeJob(0, panickingJob)
	})

	// Active jobs must be cleaned up after execution (even on panic/failure)
	pool.mu.RLock()
	assert.Empty(t, pool.activeJobs, "activeJobs must be cleared after executeJob completes")
	pool.mu.RUnlock()

	// 2. Verify worker goroutine survives a job and continues processing subsequent jobs
	var wg sync.WaitGroup
	wg.Add(2)
	pool.InvalidateGraphCache = func() {
		wg.Done()
	}

	// Start worker in background
	go pool.worker(1)

	// Send two jobs through the queue
	pool.Queue <- Job{ArticleID: 101, Type: "test_job_1"}
	pool.Queue <- Job{ArticleID: 102, Type: "test_job_2"}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for worker to process jobs")
	}
	pool.mu.RLock()
	assert.Empty(t, pool.activeJobs, "worker should have processed jobs and cleaned up active tracking")
	pool.mu.RUnlock()

	close(pool.Queue)
}
