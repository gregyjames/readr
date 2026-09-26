package agents

import (
	"sync"
	"testing"
	"time"
)

func TestCircuitBreaker_OpensAfterFailures(t *testing.T) {
	cb := NewCircuitBreaker(3, 500*time.Millisecond)

	if !cb.Allow() {
		t.Fatal("circuit breaker should allow on initial state")
	}

	// Record 3 failures
	cb.RecordFailure()
	cb.RecordFailure()
	if !cb.Allow() {
		t.Fatal("circuit breaker should allow before threshold")
	}
	cb.RecordFailure()

	// Threshold reached (3)
	if cb.Allow() {
		t.Fatal("circuit breaker should be OPEN after 3 failures")
	}

	// Wait for cooldown period
	time.Sleep(600 * time.Millisecond)

	// Half-open attempt allowed
	if !cb.Allow() {
		t.Fatal("circuit breaker should allow trial request after cooldown")
	}

	// Success closes the circuit
	cb.RecordSuccess()
	if !cb.Allow() {
		t.Fatal("circuit breaker should be CLOSED after trial success")
	}
}

func TestCircuitBreaker_HalfOpenFailureReopens(t *testing.T) {
	cb := NewCircuitBreaker(2, 200*time.Millisecond)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.Allow() {
		t.Fatal("circuit breaker should be OPEN")
	}
	if cb.State() != StateOpen {
		t.Fatalf("expected state %s, got %s", StateOpen, cb.State())
	}

	// Cooldown
	time.Sleep(250 * time.Millisecond)

	// Allow transitions to half-open
	if !cb.Allow() {
		t.Fatal("circuit breaker should allow trial request")
	}

	// Failure in half-open should immediately reopen
	cb.RecordFailure()
	if cb.Allow() {
		t.Fatal("circuit breaker should immediately reopen after failure in half-open state")
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	cb := NewCircuitBreaker(10, 50*time.Millisecond)
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = cb.Allow()
		}()
		go func() {
			defer wg.Done()
			cb.RecordFailure()
		}()
		go func() {
			defer wg.Done()
			cb.RecordSuccess()
		}()
	}

	wg.Wait()
	_ = cb.State()
}

func TestCircuitBreaker_DefaultParameters(t *testing.T) {
	cb := NewCircuitBreaker(0, 0)
	if cb.threshold != 5 {
		t.Fatalf("expected default threshold 5, got %d", cb.threshold)
	}
	if cb.cooldown != 2*time.Minute {
		t.Fatalf("expected default cooldown 2m, got %v", cb.cooldown)
	}
}
