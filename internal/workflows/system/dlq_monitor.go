package system

import (
	"context"
	"log"

	"github.com/hibiken/asynq"
)

// DLQMonitorTask inspects the Asynq inspector for failed/retrying tasks and logs alerts.
func DLQMonitorTask(inspector *asynq.Inspector) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		// 1. Check default queue
		q, err := inspector.GetQueueInfo("default")
		if err != nil {
			return err
		}

		if q.Size > 1000 {
			log.Printf("[CRITICAL] Queue 'default' is backing up: %d tasks", q.Size)
		}

		// 2. Check failed tasks (Aggregated/Archived)
		// Archived tasks in Asynq are the DLQ
		if q.Archived > 50 {
			log.Printf("[CRITICAL] Dead-Letter Queue (Archived) has %d failed tasks. Manual intervention required.", q.Archived)
		}

		return nil
	}
}
