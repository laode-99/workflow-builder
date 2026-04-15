// Package statemachine implements the pure lead state machine for the
// multi-project AI flow engine. It is the parity contract with Anandaya:
// every transition maps 1:1 to an n8n node path in the reference system.
//
// All functions in this package are pure — they take a Lead snapshot, an
// Event, a FlowConfig, and "now", and return a Patch + list of Commands
// to enqueue. No IO, no database access, no network calls. This lets the
// transition matrix be unit-tested exhaustively before any integration
// code runs.
package statemachine

import (
	"time"

	"github.com/workflow-builder/core/internal/model"
)

// Lead is a minimal snapshot of a lead's state as seen by the state machine.
// It contains only the fields the transition logic needs to read; writers
// use Patch to describe field changes.
type Lead struct {
	Attempt               int
	CallDate              *time.Time
	WhatsappSentAt        *time.Time
	WhatsappReplyAt       *time.Time
	DisconnectedReason    string
	Interest              string
	Interest2             string
	CustomerType          string
	TerminalInvalid       bool
	TerminalResponded     bool
	TerminalNotInterested bool
	TerminalSpam          bool
	TerminalAgent         bool
	TerminalCompleted     bool
	Summary               string
}

// Event is anything that happens to a lead. Concrete event types implement
// this interface via eventMarker(); Go's type switch in Transition dispatches
// on the concrete type.
type Event interface {
	eventMarker()
}

// EventCronTick is the attempt manager cron evaluating this lead for escalation.
type EventCronTick struct{}

// EventCallAnalyzed is the Retell webhook delivering a call outcome.
// Attempt is the attempt level at which the call was dispatched (1, 3, or 4).
type EventCallAnalyzed struct {
	Attempt            int
	DisconnectedReason string
	Interest           string
	CustomerType       string
	// HasSufficientConvo is derived from Retell's custom_analysis:
	// interest != "tidak ada percakapan yang cukup".
	HasSufficientConvo bool
}

// EventWAInbound is an inbound WhatsApp message from the lead.
// IsFirstReply is true when whatsapp_reply_at is currently NULL on the lead
// (derived by the webhook handler before constructing the event).
type EventWAInbound struct {
	IsFirstReply bool
}

// EventIntentClassified is the result of the chatbot's intent classifier.
// Intent must be one of: "Callback", "Tidak Tertarik", "Agent".
type EventIntentClassified struct {
	Intent string
}

// EventSpamClassified is the result of the spam classifier.
type EventSpamClassified struct {
	IsSpam bool
}

// EventMaxOutCheck is a cron-driven probe for attempt=5 leads whose grace
// period has expired without a reply.
type EventMaxOutCheck struct{}

func (EventCronTick) eventMarker()         {}
func (EventCallAnalyzed) eventMarker()     {}
func (EventWAInbound) eventMarker()        {}
func (EventIntentClassified) eventMarker() {}
func (EventSpamClassified) eventMarker()   {}
func (EventMaxOutCheck) eventMarker()      {}

// Patch holds the set of field changes a transition wants to apply to a lead.
// All fields are pointers so that "unset" (nil) is distinguishable from
// "set to zero value". The repository layer applies non-nil fields atomically
// using optimistic locking.
type Patch struct {
	Attempt               *int
	CallDate              *time.Time
	WhatsappSentAt        *time.Time
	WhatsappReplyAt       *time.Time
	DisconnectedReason    *string
	Interest              *string
	Interest2             *string
	CustomerType          *string
	TerminalInvalid       *bool
	TerminalResponded     *bool
	TerminalNotInterested *bool
	TerminalSpam          *bool
	TerminalAgent         *bool
	TerminalCompleted     *bool
	Summary               *string
	SentToDev             *bool
	SentToWaGroupAt       *time.Time
}

// IsEmpty reports whether the patch would leave the lead unchanged.
func (p Patch) IsEmpty() bool {
	return p.Attempt == nil && p.CallDate == nil && p.WhatsappSentAt == nil &&
		p.WhatsappReplyAt == nil && p.DisconnectedReason == nil &&
		p.Interest == nil && p.Interest2 == nil && p.CustomerType == nil &&
		p.TerminalInvalid == nil && p.TerminalResponded == nil &&
		p.TerminalNotInterested == nil && p.TerminalSpam == nil &&
		p.TerminalAgent == nil && p.TerminalCompleted == nil &&
		p.Summary == nil && p.SentToDev == nil && p.SentToWaGroupAt == nil
}

// Command is a side effect to enqueue after the patch commits. Concrete
// command types implement commandMarker().
type Command interface {
	commandMarker()
}

// CmdEnqueueRetellCall asks the dispatcher to start a Retell call for this lead
// at the given attempt level (1, 3, or 4).
type CmdEnqueueRetellCall struct {
	Attempt int
}

// CmdEnqueueWABridging asks the dispatcher to send the Anandaya-style bridging
// WA message (sent after attempt 1 if the call didn't reach the lead).
type CmdEnqueueWABridging struct{}

// CmdEnqueueWAFinal asks the dispatcher to send the final WA message (the
// last outreach attempt before the lead is considered maxed out).
type CmdEnqueueWAFinal struct{}

// CmdEnqueueCRMSync asks the CRM sync outbox to push the lead's current
// state to LeadSquared. Path is "A" (responded) or "B" (no response).
// Status is optional and overrides the default contact_status mapping.
type CmdEnqueueCRMSync struct {
	Path   string // "A" or "B"
	Status string // optional override, e.g. "Not Valid" | "Spam" | "Agent"
}

func (CmdEnqueueRetellCall) commandMarker() {}
func (CmdEnqueueWABridging) commandMarker() {}
func (CmdEnqueueWAFinal) commandMarker()    {}
func (CmdEnqueueCRMSync) commandMarker()    {}

// LeadToStatemachine converts a DB model Lead to a pure statemachine Lead snapshot.
func LeadToStatemachine(l model.Lead) Lead {
	return Lead{
		Attempt:               l.Attempt,
		CallDate:              l.CallDate,
		WhatsappSentAt:        l.WhatsappSentAt,
		WhatsappReplyAt:       l.WhatsappReplyAt,
		DisconnectedReason:    l.DisconnectedReason,
		Interest:              l.Interest,
		Interest2:             l.Interest2,
		CustomerType:          l.CustomerType,
		TerminalInvalid:       l.TerminalInvalid,
		TerminalResponded:     l.TerminalResponded,
		TerminalNotInterested: l.TerminalNotInterested,
		TerminalSpam:          l.TerminalSpam,
		TerminalAgent:         l.TerminalAgent,
		TerminalCompleted:     l.TerminalCompleted,
		Summary:               l.Summary,
	}
}
