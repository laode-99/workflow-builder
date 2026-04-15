package system

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/gorm"
)

// ArchiveSixMonthTask moves leads and messages older than 6 months to archive tables.
func ArchiveSixMonthTask(db *gorm.DB) asynq.HandlerFunc {
	return func(ctx context.Context, t *asynq.Task) error {
		cutoff := time.Now().AddDate(0, -6, 0)

		return db.Transaction(func(tx *gorm.DB) error {
			// 1. Archive Leads
			// Assuming tables leads_archived exist or we create them
			// For MVP, we will just move them within the same DB but with a 'is_archived' flag 
			// OR move to a dedicated table. The user requested 'archive', so dedicated table is better.
			
			// Ensure archive table exists (simplified for this migration)
			tx.Exec(`CREATE TABLE IF NOT EXISTS leads_archived AS SELECT * FROM leads WHERE 1=0`)
			tx.Exec(`CREATE TABLE IF NOT EXISTS chat_messages_archived AS SELECT * FROM chat_messages WHERE 1=0`)

			// Move Leads
			res := tx.Exec(`
				INSERT INTO leads_archived 
				SELECT * FROM leads 
				WHERE created_at < ?
			`, cutoff)
			if res.Error != nil {
				return fmt.Errorf("archive leads failed: %w", res.Error)
			}

			// Move Messages
			res = tx.Exec(`
				INSERT INTO chat_messages_archived 
				SELECT * FROM chat_messages 
				WHERE created_at < ?
			`, cutoff)
			if res.Error != nil {
				return fmt.Errorf("archive messages failed: %w", res.Error)
			}

			// Delete original messages first (due to FK)
			tx.Exec(`DELETE FROM chat_messages WHERE created_at < ?`, cutoff)
			tx.Exec(`DELETE FROM leads WHERE created_at < ?`, cutoff)

			return nil
		})
	}
}
