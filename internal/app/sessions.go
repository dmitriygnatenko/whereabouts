package app

import (
	"context"
	"log"
	"time"

	"wherewhat/internal/port"
)

// sessionCleanupInterval is how often expired sessions are swept from storage.
const sessionCleanupInterval = time.Hour

// startSessionCleanup periodically deletes expired sessions in the background. Sessions are
// otherwise only ever removed lazily — the one time an expired token happens to be presented again
// (see authenticatesession.UseCase) — so without this sweep the sessions table grows forever with
// rows nobody ever came back to use.
func startSessionCleanup(ctx context.Context, sessions port.SessionRepository) {
	go func() {
		ticker := time.NewTicker(sessionCleanupInterval)
		defer ticker.Stop()

		for {
			if n, err := sessions.DeleteExpired(ctx, time.Now().UTC()); err != nil {
				log.Printf("session cleanup: %v", err)
			} else if n > 0 {
				log.Printf("session cleanup: removed %d expired session(s)", n)
			}

			<-ticker.C
		}
	}()
}
