package agents

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestPool_PersistentJobQueueAndRecovery(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "agent_pool_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)

	logger := zap.NewNop()

	// 1. Boot pool with 0 workers to test initial DB migration and SubmitJob persistence
	InitPool(logger, db, nil, tempDir, 0, nil)
	assert.NotNil(t, Pool)
	assert.NotNil(t, Pool.CircuitBreaker)

	// Submit a job
	job1 := Job{ArticleID: 42, Type: JobTypePipeline}
	SubmitJob(job1)

	// Verify persistence in SQLite
	var dbJob GormAgentJob
	err = db.Where("article_id = ?", 42).First(&dbJob).Error
	assert.NoError(t, err)
	assert.Equal(t, "queued", dbJob.Status)
	assert.Equal(t, "pipeline", dbJob.Type)

	// Drain the job from the queue and execute it directly
	select {
	case j := <-Pool.Queue:
		assert.Equal(t, int64(42), j.ArticleID)
		assert.True(t, j.ID > 0, "job ID should be populated from persistent storage")
		Pool.executeJob(0, j)
	case <-time.After(1 * time.Second):
		t.Fatal("expected job in queue")
	}

	// Verify job transitioned to completed
	err = db.Where("article_id = ?", 42).First(&dbJob).Error
	assert.NoError(t, err)
	assert.Equal(t, "completed", dbJob.Status)

	// 2. Simulate interrupted job left in "processing" state and a queued job
	interruptedJob := GormAgentJob{
		ArticleID: 99,
		Type:      string(JobTypePipeline),
		Status:    "processing",
	}
	assert.NoError(t, db.Create(&interruptedJob).Error)

	queuedJob := GormAgentJob{
		ArticleID: 100,
		Type:      string(JobTypePipeline),
		Status:    "queued",
	}
	assert.NoError(t, db.Create(&queuedJob).Error)

	// 3. Restart Pool with the same database
	InitPool(logger, db, nil, tempDir, 0, nil)

	// Verify interrupted job was reset to "queued"
	var resetJob GormAgentJob
	assert.NoError(t, db.Where("id = ?", interruptedJob.ID).First(&resetJob).Error)
	assert.Equal(t, "queued", resetJob.Status)

	// Verify jobs were re-enqueued into Pool.Queue on boot
	reEnqueuedIDs := make(map[int64]bool)
	for i := 0; i < 2; i++ {
		select {
		case j := <-Pool.Queue:
			assert.True(t, j.ID > 0, "re-enqueued job ID must be populated")
			reEnqueuedIDs[j.ArticleID] = true
		case <-time.After(1 * time.Second):
			t.Fatalf("expected 2 re-enqueued jobs, timed out at %d", i)
		}
	}
	assert.True(t, reEnqueuedIDs[99], "interrupted job 99 should have been recovered")
	assert.True(t, reEnqueuedIDs[100], "queued job 100 should have been re-enqueued")

	// 4. Test Circuit Breaker interaction in executeJob
	// Trip circuit breaker by recording failures
	for i := 0; i < 5; i++ {
		Pool.CircuitBreaker.RecordFailure()
	}
	assert.False(t, Pool.CircuitBreaker.Allow(), "circuit breaker should be open")

	// Submit a new job while circuit breaker is open
	cbJob := Job{ArticleID: 200, Type: JobTypePipeline}
	SubmitJob(cbJob)

	// Execute job while circuit breaker is open
	select {
	case j := <-Pool.Queue:
		Pool.executeJob(0, j)
	case <-time.After(1 * time.Second):
		t.Fatal("expected job in queue")
	}

	var failedJob GormAgentJob
	assert.NoError(t, db.Where("article_id = ?", 200).First(&failedJob).Error)
	assert.Equal(t, "failed", failedJob.Status)
	assert.Equal(t, "circuit breaker open", failedJob.Error)
}

func TestPool_JobFailureUpdatesStatusAndTripsBreaker(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "failure_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)

	logger := zap.NewNop()
	InitPool(logger, db, nil, tempDir, 0, nil)

	// Create job with failing type
	job := Job{ArticleID: 300, Type: "failing_job"}
	SubmitJob(job)

	select {
	case j := <-Pool.Queue:
		Pool.executeJob(0, j)
	case <-time.After(1 * time.Second):
		t.Fatal("expected job in queue")
	}

	var failedJob GormAgentJob
	assert.NoError(t, db.Where("article_id = ?", 300).First(&failedJob).Error)
	assert.Equal(t, "failed", failedJob.Status)
	assert.Contains(t, failedJob.Error, "failing_job")
	assert.Equal(t, 1, Pool.CircuitBreaker.failureCount)
}

func TestPool_JobPanicUpdatesStatusAndTripsBreaker(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "panic_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)

	logger := zap.NewNop()
	InitPool(logger, db, nil, tempDir, 0, nil)

	job := Job{
		ArticleID: 301,
		Type:      JobTypePipeline,
		Payload:   map[string]interface{}{"panic": true},
	}
	SubmitJob(job)

	select {
	case j := <-Pool.Queue:
		Pool.executeJob(0, j)
	case <-time.After(1 * time.Second):
		t.Fatal("expected job in queue")
	}

	var panickedJob GormAgentJob
	assert.NoError(t, db.Where("article_id = ?", 301).First(&panickedJob).Error)
	assert.Equal(t, "failed", panickedJob.Status)
	assert.Contains(t, panickedJob.Error, "simulated panic in pipeline")
	assert.Equal(t, 1, Pool.CircuitBreaker.failureCount)
}

func TestPool_PipelineErrorPropagatesToCircuitBreaker(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "pipeline_error_test.db")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)

	logger := zap.NewNop()
	InitPool(logger, db, nil, tempDir, 0, nil)

	// Pipeline job with an active stage (e.g. summarizer), but no API key configured
	// processPipeline will fail with "API key not configured" error
	job := Job{
		ArticleID: 555,
		Type:      JobTypePipeline,
		Settings: PipelineSettings{
			Summarizer: true,
		},
	}
	SubmitJob(job)

	select {
	case j := <-Pool.Queue:
		assert.True(t, j.ID > 0, "job ID should be bound on submission")
		Pool.executeJob(0, j)
	case <-time.After(1 * time.Second):
		t.Fatal("expected job in queue")
	}

	var failedJob GormAgentJob
	assert.NoError(t, db.Where("article_id = ?", 555).First(&failedJob).Error)
	assert.Equal(t, "failed", failedJob.Status)
	assert.Contains(t, failedJob.Error, "API key not configured")
	assert.Equal(t, 1, Pool.CircuitBreaker.failureCount)
}
