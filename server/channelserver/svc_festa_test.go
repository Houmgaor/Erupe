package channelserver

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestFestaService(mock *mockFestaRepo) *FestaService {
	logger, _ := zap.NewDevelopment()
	return NewFestaService(mock, logger)
}

// --- EnsureActiveEvent tests ---

func TestFestaService_EnsureActiveEvent_StillActive(t *testing.T) {
	mock := &mockFestaRepo{}
	svc := newTestFestaService(mock)

	now := time.Unix(1000000, 0)
	start := uint32(now.Unix() - 100) // started 100s ago, well within lifespan

	result, err := svc.EnsureActiveEvent(start, now, now.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != start {
		t.Errorf("start = %d, want %d (unchanged)", result, start)
	}
	if mock.cleanupCalled {
		t.Error("CleanupAll should not be called when event is active")
	}
}

func TestFestaService_EnsureActiveEvent_Expired(t *testing.T) {
	mock := &mockFestaRepo{}
	svc := newTestFestaService(mock)

	now := time.Unix(10000000, 0)
	expiredStart := uint32(1) // long expired
	nextMidnight := now.Add(24 * time.Hour)

	result, err := svc.EnsureActiveEvent(expiredStart, now, nextMidnight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.cleanupCalled {
		t.Error("CleanupAll should be called for expired event")
	}
	if result != uint32(nextMidnight.Unix()) {
		t.Errorf("start = %d, want %d (next midnight)", result, uint32(nextMidnight.Unix()))
	}
	if mock.insertedStart != uint32(nextMidnight.Unix()) {
		t.Errorf("insertedStart = %d, want %d", mock.insertedStart, uint32(nextMidnight.Unix()))
	}
}

func TestFestaService_EnsureActiveEvent_NoEvent(t *testing.T) {
	mock := &mockFestaRepo{}
	svc := newTestFestaService(mock)

	now := time.Unix(1000000, 0)
	nextMidnight := now.Add(24 * time.Hour)

	result, err := svc.EnsureActiveEvent(0, now, nextMidnight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !mock.cleanupCalled {
		t.Error("CleanupAll should be called when no event exists")
	}
	if result != uint32(nextMidnight.Unix()) {
		t.Errorf("start = %d, want %d", result, uint32(nextMidnight.Unix()))
	}
}

func TestFestaService_EnsureActiveEvent_CleanupError(t *testing.T) {
	mock := &mockFestaRepo{cleanupErr: errors.New("db error")}
	svc := newTestFestaService(mock)

	now := time.Unix(10000000, 0)
	_, err := svc.EnsureActiveEvent(0, now, now.Add(24*time.Hour))
	if err == nil {
		t.Fatal("expected error from cleanup failure")
	}
}

func TestFestaService_EnsureActiveEvent_InsertError(t *testing.T) {
	mock := &mockFestaRepo{insertErr: errors.New("db error")}
	svc := newTestFestaService(mock)

	now := time.Unix(10000000, 0)
	_, err := svc.EnsureActiveEvent(0, now, now.Add(24*time.Hour))
	if err == nil {
		t.Fatal("expected error from insert failure")
	}
}

// --- SubmitSouls tests ---

func TestFestaService_SubmitSouls_FiltersZeros(t *testing.T) {
	mock := &mockFestaRepo{}
	svc := newTestFestaService(mock)

	err := svc.SubmitSouls(1, 10, []uint16{0, 5, 0, 3, 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should call repo with the full slice (repo does batch insert)
	if mock.submittedSouls == nil {
		t.Fatal("SubmitSouls should be called on repo")
	}
}

func TestFestaService_SubmitSouls_AllZeros(t *testing.T) {
	mock := &mockFestaRepo{}
	svc := newTestFestaService(mock)

	err := svc.SubmitSouls(1, 10, []uint16{0, 0, 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.submittedSouls != nil {
		t.Error("SubmitSouls should not call repo when all zeros")
	}
}

func TestFestaService_SubmitSouls_RepoError(t *testing.T) {
	mock := &mockFestaRepo{submitErr: errors.New("db error")}
	svc := newTestFestaService(mock)

	err := svc.SubmitSouls(1, 10, []uint16{5, 0, 3})
	if err == nil {
		t.Fatal("expected error from repo failure")
	}
}
