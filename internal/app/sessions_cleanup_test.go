package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"wherewhat/internal/port/mocks"
)

// errStub is the sentinel a case uses when it only cares that an error travels through untouched.
var errStub = errors.New("stub error")

// waitForCall blocks until calls receives a signal, failing the test if none arrives within a
// second — long enough for CI jitter, short enough that a real hang still fails fast.
func waitForCall(t *testing.T, calls <-chan struct{}, msg string) {
	t.Helper()

	select {
	case <-calls:
	case <-time.After(time.Second):
		t.Fatal(msg)
	}
}

// TestStartSessionCleanup_SweepsImmediatelyThenPeriodically checks the two-phase behavior an
// operator relies on: a sweep as soon as the process starts (rather than waiting a full interval for
// the first one), then another every interval after that.
func TestStartSessionCleanup_SweepsImmediatelyThenPeriodically(t *testing.T) {
	t.Parallel()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	sessions := mocks.NewMockSessionRepository(mc)

	calls := make(chan struct{}, 10)

	sessions.EXPECT().DeleteExpired(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, time.Time) (int64, error) {
			calls <- struct{}{}

			return 0, nil
		}).
		MinTimes(2)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	startSessionCleanup(ctx, sessions, 10*time.Millisecond)

	waitForCall(t, calls, "expected an immediate cleanup sweep on start")
	waitForCall(t, calls, "expected a periodic cleanup sweep after the interval elapsed")
}

// TestStartSessionCleanup_StopsOnContextCancellation guards against a goroutine leak: once ctx is
// canceled the loop must stop calling DeleteExpired, not keep sweeping (and logging errors) forever
// with an already-canceled context.
func TestStartSessionCleanup_StopsOnContextCancellation(t *testing.T) {
	t.Parallel()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	sessions := mocks.NewMockSessionRepository(mc)

	calls := make(chan struct{}, 10)

	sessions.EXPECT().DeleteExpired(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, time.Time) (int64, error) {
			calls <- struct{}{}

			return 0, nil
		}).
		MinTimes(1)

	ctx, cancel := context.WithCancel(context.Background())

	startSessionCleanup(ctx, sessions, 10*time.Millisecond)

	// Let the first (immediate) sweep happen, then cancel while the goroutine is parked waiting on
	// the ticker — the point where it should notice ctx is done and return instead of sweeping again.
	waitForCall(t, calls, "expected an immediate cleanup sweep on start")
	cancel()

	select {
	case <-calls:
		t.Fatal("expected no further cleanup sweeps after the context was canceled")
	case <-time.After(100 * time.Millisecond):
	}
}

// TestStartSessionCleanup_LogsErrorAndKeepsGoing checks that a failed sweep is only logged, not
// treated as fatal to the loop — the next tick must still attempt another sweep.
func TestStartSessionCleanup_LogsErrorAndKeepsGoing(t *testing.T) {
	t.Parallel()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	sessions := mocks.NewMockSessionRepository(mc)

	calls := make(chan struct{}, 10)

	sessions.EXPECT().DeleteExpired(gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, time.Time) (int64, error) {
			calls <- struct{}{}

			return 0, errStub
		}).
		MinTimes(2)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	startSessionCleanup(ctx, sessions, 10*time.Millisecond)

	waitForCall(t, calls, "expected an immediate cleanup sweep on start")
	waitForCall(t, calls, "expected the loop to keep going after a failed sweep")
}
