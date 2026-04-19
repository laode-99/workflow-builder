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
	loc := time.FixedZone("Asia/Jakarta", 7*3600)
	c := cron.New(cron.WithLocation(loc))

	return &Scheduler{
		repo:  repo,
		asynq: asynqClient,
		cron:  c,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	log.Println("[SCHEDULER] Starting background cron monitor (Asia/Jakarta)...")
	
	ticker := time.NewTicker(1 * time.Minute)
	// System jobs ticker (e.g. CRM Sync Poller)
	systemTicker := time.NewTicker(1 * time.Minute)

	// Maintenance Schedules (Asia/Jakarta)
	s.cron.AddFunc("0 0 * * 0", func() { s.enqueueSystemJob("archive:six_month") }) // Weekly Sunday
	s.cron.AddFunc("0 * * * *", func() { s.enqueueSystemJob("system:dlq_monitor") }) // Hourly
	s.cron.Start()

	go func() {
		for {
			select {
			case <-ctx.Done():
				s.cron.Stop()
				return
			case <-ticker.C:
				s.tick()
			case <-systemTicker.C:
				s.enqueueSystemJob("leadflow:crm_sync_poller")
			}
		}
	}()
}

func (s *Scheduler) enqueueSystemJob(taskName string) {
	task := asynq.NewTask(taskName, nil)
	_, err := s.asynq.Enqueue(task, asynq.Queue("critical"))
	if err != nil {
		log.Printf("[SCHEDULER] Failed to enqueue system job %s: %v", taskName, err)
	}
}

func (s *Scheduler) tick() {
	var workflows []model.Workflow
	// Find active workflows with a cron schedule
	err := s.repo.db.Where("is_active = true AND trigger_cron != ''").Find(&workflows).Error
	if err != nil {
		log.Printf("[SCHEDULER] Error fetching workflows: %v", err)
		return
	}

	loc := time.FixedZone("Asia/Jakarta", 7*3600)
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
			// GLOBAL SAFETY CONSTRAINT: Prevent any automated cron workflows from running between 9 PM and 8 AM
			// to protect against nighttime dispatches causing customer issues.
			hour := now.Hour()
			if hour >= 21 || hour < 8 {
				log.Printf("[SCHEDULER] BLOCKED workflow '%s' (Nighttime Pause active: %02d:%02d)", wf.Alias, hour, now.Minute())
				continue
			}

			// SPECIFIC CONSTRAINT: N8NTriggerWorkflow is restricted to 8 AM - 6 PM (18:00)
			if wf.Signature == "N8NTriggerWorkflow" && hour >= 18 {
				log.Printf("[SCHEDULER] BLOCKED N8NTriggerWorkflow '%s' (Operating Hours restricted: 8 AM - 6 PM)", wf.Alias)
				continue
			}

			log.Printf("[SCHEDULER] Triggering workflow '%s' (cron: %s)", wf.Alias, wf.TriggerCron)
			s.trigger(wf)
		}
	}
}

func (s *Scheduler) trigger(wf model.Workflow) {
	execID := uuid.New()
	execution := &model.Execution{
		ID:              execID,
		WorkflowID:      wf.ID,
		Status:          "queued",
		TriggeredByType: "system",
	}

	if err := s.repo.CreateExecution(execution); err != nil {
		log.Printf("[SCHEDULER] Failed to create execution for %s: %v", wf.ID, err)
		return
	}

	// Create Audit Log for System Trigger
	s.repo.CreateAuditLog(&model.AuditLog{
		BusinessID: wf.BusinessID,
		Action:     "START_WORKFLOW",
		TargetID:   execID,
		TargetType: "execution",
		Details:    `{"workflow_alias": "` + wf.Alias + `", "trigger": "system_cron"}`,
	})

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
