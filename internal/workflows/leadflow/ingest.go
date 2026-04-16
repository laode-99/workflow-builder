package leadflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/workflow-builder/core/internal/integrations/leadsquared"
	"github.com/workflow-builder/core/internal/model"
	"github.com/workflow-builder/core/internal/sdk"
	"github.com/workflow-builder/core/internal/statemachine"
	"github.com/workflow-builder/core/pkg/phone"
	"gorm.io/gorm/clause"
)

func handleIngest(ctx context.Context, exec sdk.Execution) error {
	log := exec.Logger()
	bizID := exec.BusinessID()
	db := exec.GetDB()

	var business model.Business
	if err := db.WithContext(ctx).First(&business, "id = ?", bizID).Error; err != nil {
		return fmt.Errorf("load business: %w", err)
	}

	if business.Status != "active" {
		log.Infof("Project %s is not active, skipping ingestion", bizID)
		return nil
	}

	var flowCfg statemachine.FlowConfig
	if err := json.Unmarshal([]byte(business.Config), &flowCfg); err != nil {
		return fmt.Errorf("parse FlowConfig: %w", err)
	}

	// Retrieve LeadSquared credentials
	credsRaw, err := exec.GetCredential("leadsquared")
	if err != nil {
		return fmt.Errorf("get leadsquared creds: %w", err)
	}
	
	// Assuming creds are JSON stored: `{"access_key": "...", "secret_key": "..."}`
	var lsCreds struct {
		AccessKey string `json:"access_key"`
		SecretKey string `json:"secret_key"`
	}
	if err := json.Unmarshal([]byte(credsRaw), &lsCreds); err != nil {
		return fmt.Errorf("parse leadsquared creds: %w", err)
	}

	client := leadsquared.NewClient(leadsquared.Credentials{
		AccessKey: lsCreds.AccessKey,
		SecretKey: lsCreds.SecretKey,
	})

	// Fetch leads created in the last 15 minutes (overlaps cron to avoid gaps)
	since := time.Now().Add(-15 * time.Minute)
	log.Infof("Fetching LeadSquared opportunities since %v", since.Format(time.RFC3339))

	opps, err := client.FetchOpportunitiesByTag(ctx, flowCfg.CRM.TagFilter, flowCfg.CRM.ActivityEvent, since)
	if err != nil {
		return fmt.Errorf("fetch opportunities: %w", err)
	}

	if len(opps) == 0 {
		log.Infof("No new opportunities found for project %s", bizID)
		return nil
	}

	log.Infof("Found %d opportunities. Ingesting...", len(opps))
	
	inserted := 0
	for _, opp := range opps {
		// Only fetch details for new ones
		var existing model.Lead
		err := db.WithContext(ctx).Select("id").Where("business_id = ? AND external_id = ?", bizID, opp.OpportunityID).First(&existing).Error
		if err == nil {
			// Already exists
			continue
		}

		// Fetch lead details (phone, name) to complete the Lead model
		details, err := client.GetLeadDetails(ctx, opp.RelatedProspectID)
		if err != nil {
			log.Errorf("Failed to get Lead Details for %s: %v", opp.RelatedProspectID, err)
			continue
		}

		// Canonical phone normalization (pkg/phone handles leading 0 → 62,
		// strips "+", and validates minimum length). Leads whose phones
		// can't be normalized are dropped entirely — they can't be dialed
		// or messaged anyway.
		cleanPhone, err := phone.NormalizeID(details.Phone)
		if err != nil {
			log.Errorf("Lead %s has invalid phone %q: %v", details.ID, details.Phone, err)
			continue
		}

		name := strings.TrimSpace(details.FirstName + " " + details.LastName)

		newLead := model.Lead{
			BusinessID: bizID,
			ExternalID: opp.OpportunityID,
			Phone:      cleanPhone,
			Name:       name,
			Attempt:    0,
		}

		res := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&newLead)
		if res.Error != nil {
			log.Errorf("Failed to insert lead %s: %v", opp.OpportunityID, res.Error)
			continue
		}
		if res.RowsAffected > 0 {
			inserted++
			db.Create(&model.LeadAudit{
				BusinessID: bizID,
				LeadID:     newLead.ID,
				Actor:      "ingest",
				EventType:  "lead_ingested",
				Changes:    `{"source": "leadsquared"}`,
			})

			// Empty-name handling: fire internal alert once per lead and
			// mark name_alert_sent=true so we don't re-alert. The lead
			// itself stays in the DB; GetDueForDispatch's name filter
			// will hold it out of escalation until a human fills the name.
			if name == "" {
				if err := fireInternalNameAlert(ctx, &newLead); err != nil {
					log.Warnf("internal alert failed for lead %s: %v", newLead.ID, err)
				}
			}
		}
	}

	log.Infof("Ingestion complete. Inserted %d new leads.", inserted)
	return nil
}
