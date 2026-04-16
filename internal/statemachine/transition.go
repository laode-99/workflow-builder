package statemachine

import (
	"fmt"
	"time"
)

// Disconnect reason constants from Retell's call_analyzed payload.
const (
	ReasonInvalidDestination = "Invalid_destination"
	ReasonVoicemailReached   = "voicemail_reached"
	ReasonIVRReached         = "ivr_reached"
	ReasonUserHangup         = "user_hangup"
	ReasonAgentHangup        = "agent_hangup"
	ReasonDialNoAnswer       = "dial_no_answer"
	ReasonDialBusy           = "dial_busy"
)

// Retell intent literal used to indicate the AI agent never had a real
// conversation with the lead (call picked up but no useful exchange).
const InterestInsufficientConversation = "tidak ada percakapan yang cukup"

// Intent classifier output enums.
const (
	IntentCallback      = "Callback"
	IntentTidakTertarik = "Tidak Tertarik"
	IntentAgent         = "Agent"
)

// CRM path labels (matches the spec's Path A / Path B).
const (
	CRMPathResponded   = "A"
	CRMPathNoResponse  = "B"
	CRMStatusNotValid  = "Not Valid"
	CRMStatusSpam      = "Spam"
	CRMStatusAgent     = "Agent"
)

// Transition is the pure lead state machine entry point.
//
// Inputs:
//   - lead: a snapshot of the current lead state (fields the SM needs to read)
//   - event: what just happened
//   - cfg: the project's FlowConfig (defaults merged with per-project overrides)
//   - now: the current time (callable passes in time.Now() in production)
//   - inHours: whether `now` falls within the project's business-hours window
//
// Returns:
//   - patch: field changes to apply atomically (caller uses optimistic locking)
//   - cmds: side effects to enqueue *after* the patch commits
//   - err: only non-nil for programming errors (unknown event types); policy
//     decisions (e.g. "cron skipped because guard failed") return (Patch{}, nil, nil)
func Transition(lead Lead, event Event, cfg FlowConfig, now time.Time, inHours bool) (Patch, []Command, error) {
	switch ev := event.(type) {
	case EventCronTick:
		return transitionCronTick(lead, cfg, now, inHours)
	case EventCallAnalyzed:
		return transitionCallAnalyzed(lead, ev, cfg, now)
	case EventWAInbound:
		return transitionWAInbound(lead, ev, now)
	case EventIntentClassified:
		return transitionIntentClassified(lead, ev)
	case EventSpamClassified:
		return transitionSpamClassified(lead, ev)
	case EventMaxOutCheck:
		return transitionMaxOutCheck(lead, cfg, now)
	default:
		return Patch{}, nil, fmt.Errorf("statemachine: unknown event type %T", event)
	}
}

// anyTerminal reports whether the lead has any outbound-blocking flag set.
// terminal_responded blocks cron outbound but NOT chatbot turns; this helper
// returns true when *cron dispatch* should be skipped. Per-event handlers
// apply finer-grained rules as needed.
func anyTerminal(l Lead) bool {
	return l.TerminalInvalid || l.TerminalResponded ||
		l.TerminalNotInterested || l.TerminalSpam ||
		l.TerminalAgent || l.TerminalCompleted
}

// transitionCronTick implements Table A from the spec: the attempt manager
// evaluating a lead for escalation.
func transitionCronTick(lead Lead, cfg FlowConfig, now time.Time, inHours bool) (Patch, []Command, error) {
	if anyTerminal(lead) {
		return Patch{}, nil, nil
	}
	if !inHours {
		return Patch{}, nil, nil
	}

	gap := time.Duration(cfg.CallRetryGapHours) * time.Hour

	switch lead.Attempt {
	case 0:
		// Fresh lead → dispatch call 1.
		p := Patch{
			Attempt:  ptrInt(1),
			CallDate: ptrTime(now),
		}
		return p, []Command{CmdEnqueueRetellCall{Attempt: 1}}, nil

	case 2:
		// WA bridging sent; time to dispatch call 2 if gap has elapsed.
		if lead.WhatsappReplyAt != nil {
			return Patch{}, nil, nil
		}
		if lead.CallDate == nil || now.Sub(*lead.CallDate) < gap {
			return Patch{}, nil, nil
		}
		p := Patch{
			Attempt:  ptrInt(3),
			CallDate: ptrTime(now),
		}
		return p, []Command{CmdEnqueueRetellCall{Attempt: 3}}, nil

	case 3:
		// Call 2 dispatched; advance to call 3 after gap.
		if lead.WhatsappReplyAt != nil {
			return Patch{}, nil, nil
		}
		if lead.CallDate == nil || now.Sub(*lead.CallDate) < gap {
			return Patch{}, nil, nil
		}
		p := Patch{
			Attempt:  ptrInt(4),
			CallDate: ptrTime(now),
		}
		return p, []Command{CmdEnqueueRetellCall{Attempt: 4}}, nil

	case 4:
		// Call 3 dispatched; advance to final WA after gap.
		if lead.WhatsappReplyAt != nil {
			return Patch{}, nil, nil
		}
		if lead.CallDate == nil || now.Sub(*lead.CallDate) < gap {
			return Patch{}, nil, nil
		}
		p := Patch{
			Attempt:        ptrInt(5),
			WhatsappSentAt: ptrTime(now),
		}
		return p, []Command{CmdEnqueueWAFinal{}}, nil

	default:
		// Attempts 1 (waiting for call 1 webhook) and 5 (waiting for reply
		// or max-out check) receive no cron-tick action here.
		return Patch{}, nil, nil
	}
}

// transitionCallAnalyzed implements Table B: Retell webhook delivers a call outcome.
func transitionCallAnalyzed(lead Lead, ev EventCallAnalyzed, cfg FlowConfig, now time.Time) (Patch, []Command, error) {
	// Always record call outcome fields, even for terminal leads (audit value).
	p := Patch{
		CallDate:           ptrTime(now),
		DisconnectedReason: ptrString(ev.DisconnectedReason),
		Interest:           ptrString(ev.Interest),
		CustomerType:       ptrString(ev.CustomerType),
	}
	if ev.SvsDate != nil {
		p.SvsDate = ev.SvsDate
	}

	// Terminal leads record fields but trigger no new side effects.
	if anyTerminal(lead) {
		return p, nil, nil
	}

	isInvalid := ev.DisconnectedReason == ReasonInvalidDestination
	isVoicemailOrIVR := ev.DisconnectedReason == ReasonVoicemailReached ||
		ev.DisconnectedReason == ReasonIVRReached ||
		ev.CustomerType == "Voicemail"
	isHangup := ev.DisconnectedReason == ReasonUserHangup ||
		ev.DisconnectedReason == ReasonAgentHangup
	isNoAnswer := ev.DisconnectedReason == ReasonDialNoAnswer ||
		ev.DisconnectedReason == ReasonDialBusy

	if isInvalid {
		p.TerminalInvalid = ptrBool(true)
		return p, []Command{CmdEnqueueCRMSync{Path: CRMPathNoResponse, Status: CRMStatusNotValid}}, nil
	}

	if ev.Attempt == 1 {
		if isVoicemailOrIVR && cfg.VoicemailShortcutToLast {
			// Jump straight to the final WA message.
			p.Attempt = ptrInt(5)
			p.WhatsappSentAt = ptrTime(now)
			return p, []Command{CmdEnqueueWAFinal{}}, nil
		}
		if isHangup && ev.HasSufficientConvo {
			// Conversation was substantive; CRM Path A, no WA bridging.
			p.Attempt = ptrInt(2)
			return p, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded}}, nil
		}
		if (isHangup && !ev.HasSufficientConvo) || isNoAnswer {
			// Not enough contact; send the bridging WA message.
			p.Attempt = ptrInt(2)
			return p, []Command{CmdEnqueueWABridging{}}, nil
		}
		// Unknown reason at attempt 1 — advance conservatively and send bridging.
		p.Attempt = ptrInt(2)
		return p, []Command{CmdEnqueueWABridging{}}, nil
	}

	if ev.Attempt == 3 || ev.Attempt == 4 {
		// Retry call outcomes never advance the counter (cron does that).
		// Fire CRM Path A immediately if the conversation was substantive.
		if ev.HasSufficientConvo {
			return p, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded}}, nil
		}
		return p, nil, nil
	}

	// Attempt values 0, 2, 5 shouldn't receive CallAnalyzed events in practice.
	// Record the fields and take no action.
	return p, nil, nil
}

// transitionWAInbound implements Table C: first inbound WA reply sets the
// terminal_responded flag. Subsequent messages just deliver to the chatbot.
func transitionWAInbound(lead Lead, ev EventWAInbound, now time.Time) (Patch, []Command, error) {
	// Leads already classified as spam or not-interested do not accept
	// further engagement — the webhook handler will audit "ignored_terminal"
	// and skip the chatbot dispatch.
	if lead.TerminalSpam || lead.TerminalNotInterested {
		return Patch{}, nil, nil
	}
	if !ev.IsFirstReply {
		return Patch{}, nil, nil
	}
	p := Patch{
		WhatsappReplyAt:   ptrTime(now),
		TerminalResponded: ptrBool(true),
	}
	return p, nil, nil
}

// transitionIntentClassified implements Table D: intent classifier output.
func transitionIntentClassified(_ Lead, ev EventIntentClassified) (Patch, []Command, error) {
	switch ev.Intent {
	case IntentCallback:
		return Patch{Interest2: ptrString(IntentCallback)}, nil, nil
	case IntentTidakTertarik:
		p := Patch{
			Interest2:             ptrString(IntentTidakTertarik),
			TerminalNotInterested: ptrBool(true),
		}
		// Anandaya's CRM mapping sends "Spam" status for Tidak Tertarik leads
		// (per the reference MAIN SWITCH). Path A because this is a chatbot
		// interaction, not a no-response timeout.
		return p, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusSpam}}, nil
	case IntentAgent:
		p := Patch{
			Interest2:     ptrString(IntentAgent),
			TerminalAgent: ptrBool(true),
		}
		return p, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusAgent}}, nil
	default:
		return Patch{}, nil, fmt.Errorf("statemachine: unknown intent %q", ev.Intent)
	}
}

// transitionSpamClassified implements the spam-classifier side of Table D.
func transitionSpamClassified(_ Lead, ev EventSpamClassified) (Patch, []Command, error) {
	if !ev.IsSpam {
		return Patch{}, nil, nil
	}
	p := Patch{
		TerminalSpam: ptrBool(true),
		CustomerType: ptrString(CRMStatusSpam),
	}
	return p, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusSpam}}, nil
}

// transitionMaxOutCheck implements the attempt=5 grace-period expiry check
// from Table A's last row.
func transitionMaxOutCheck(lead Lead, cfg FlowConfig, now time.Time) (Patch, []Command, error) {
	if anyTerminal(lead) || lead.Attempt != 5 {
		return Patch{}, nil, nil
	}
	if lead.WhatsappReplyAt != nil {
		return Patch{}, nil, nil
	}
	if lead.WhatsappSentAt == nil {
		return Patch{}, nil, nil
	}
	grace := time.Duration(cfg.MaxOutGraceHours) * time.Hour
	if now.Sub(*lead.WhatsappSentAt) < grace {
		return Patch{}, nil, nil
	}
	p := Patch{TerminalCompleted: ptrBool(true)}
	return p, []Command{CmdEnqueueCRMSync{Path: CRMPathNoResponse}}, nil
}

// Pointer helpers keep transition code terse without scattering anonymous
// helper functions. They exist only in this package.

func ptrInt(v int) *int          { return &v }
func ptrBool(v bool) *bool       { return &v }
func ptrString(v string) *string { return &v }
func ptrTime(v time.Time) *time.Time {
	return &v
}
