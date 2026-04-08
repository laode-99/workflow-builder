package api

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
	"github.com/workflow-builder/core/internal/model"
)

type Scheduler struct {
	repo   *Repo
	asynq  *asynq.Client
	cron   *cron.Cron
}

func NewScheduler(repo *Repo, asynqClient *asynq.Client) *Scheduler {
	// Initialize cron with Jakarta time
	loc, _ := time.LoadLocation("Asia/Jakarta")
	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		repo:  repo,
		asynq: asynqClient,
		cron:  c,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	log.Println("[SCHEDULER] Starting background cron monitor (Asia/Jakarta)...")
	
	// Check every minute for workflows that need to be triggered
	ticker := time.NewTicker(1 * time.Minute)
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.cron.Stop()
				return
			case <-ticker.C:
				s.tick()
			}
		}
	}()
}

func (s *Scheduler) tick() {
	var workflows []model.Workflow
	// Find active workflows with a cron schedule
	err := s.repo.db.Where("is_active = true AND trigger_cron != ''").Find(&workflows).Error
	if err != nil {
		log.Printf("[SCHEDULER] Error fetching workflows: %v", err)
		return
	}

	loc, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Now().In(loc)

	for _, wf := range workflows {
		sched, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(wf.TriggerCron)
		if err != nil {
			continue
		}

		// Check if it was supposed to run in the last minute
		nextRun := sched.Next(now.Add(-1 * time.Minute))
		
		// If the next calculated run time is within the current minute windows, trigger it
		if nextRun.After(now.Add(-1 * time.Minute)) && nextRun.Before(now.Add(1 * time.Second)) {
			log.Printf("[SCHEDULER] Triggering workflow '%s' (cron: %s)", wf.Alias, wf.TriggerCron)
			s.trigger(wf)
		}
	}
}

func (s *Scheduler) trigger(wf model.Workflow) {
	execID := uuid.New()
	execution := &model.Execution{
		ID:         execID,
		WorkflowID: wf.ID,
		Status:     "queued",
	}

	if err := s.repo.CreateExecution(execution); err != nil {
		log.Printf("[SCHEDULER] Failed to create execution for %s: %v", wf.ID, err)
		return
	}

	payload, _ := json.Marshal(map[string]string{
		"workflow_id":  wf.ID.String(),
		"execution_id": execID.String(),
	})

	task := asynq.NewTask("workflow:execute", payload)
	_, err := s.asynq.Enqueue(task, asynq.Queue("executions"))
	if err != nil {
		log.Printf("[SCHEDULER] Failed to enqueue task for %s: %v", wf.ID, err)
	}
}
