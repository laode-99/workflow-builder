package leadflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/workflow-builder/core/internal/integrations/twochat"
	"github.com/workflow-builder/core/internal/sdk"
)

// loadTwoChatClient decrypts the per-project 2chat credential and builds
// a client, or returns an error if the credential is missing/invalid.
// Used by both the WA group dispatcher and the internal-alert flow.
func loadTwoChatClient(ctx context.Context, businessID uuid.UUID) (twochat.Client, error) {
	raw, err := sdk.GetCredential(deps.DB, businessID, "twochat", deps.EncKey)
	if err != nil {
		return nil, fmt.Errorf("load 2chat cred: %w", err)
	}
	var creds twochat.Credentials
	if err := json.Unmarshal([]byte(raw), &creds); err != nil {
		return nil, fmt.Errorf("parse 2chat cred: %w", err)
	}
	return twochat.NewClient(creds), nil
}

// sendToInternalGroup dispatches a plain text message to an internal
// team WA group via 2Chat. Used for developer/sales hot-lead handoffs
// and internal alerts (e.g. empty-name notifications).
//
// Customer-facing WhatsApp goes through Gupshup instead — see
// sendGupshupReply in chatbot_agent.go. This split mirrors the Anandaya
// n8n topology.
func sendToInternalGroup(ctx context.Context, businessID uuid.UUID, waGroupID, message string) error {
	client, err := loadTwoChatClient(ctx, businessID)
	if err != nil {
		return err
	}
	return client.SendText(ctx, waGroupID, message)
}

// checkValidWANumber queries 2Chat's check-number endpoint. Used by the
// developer handoff to skip leads whose phone numbers aren't registered
// on WhatsApp.
func checkValidWANumber(ctx context.Context, businessID uuid.UUID, phone string) (bool, error) {
	client, err := loadTwoChatClient(ctx, businessID)
	if err != nil {
		return false, err
	}
	return client.CheckNumber(ctx, phone)
}
