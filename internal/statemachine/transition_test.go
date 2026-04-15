package statemachine

import (
	"reflect"
	"testing"
	"time"
)

// Fixed time anchor for all test cases. 2026-04-15 12:00 Jakarta.
var now = time.Date(2026, 4, 15, 12, 0, 0, 0, time.FixedZone("WIB", 7*3600))

// Older timestamps used to simulate elapsed time since last call / wa sent.
var (
	threeHoursAgo = now.Add(-3 * time.Hour)
	twoHoursAgo   = now.Add(-2 * time.Hour)
	oneHourAgo    = now.Add(-1 * time.Hour)
	dayAgo        = now.Add(-24 * time.Hour)
)

// Helper: assert patches are deeply equal, showing a readable diff on mismatch.
func assertPatch(t *testing.T, got, want Patch) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("patch mismatch:\n  got:  %+v\n  want: %+v", got, want)
	}
}

func assertCmds(t *testing.T, got, want []Command) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("commands mismatch:\n  got:  %+v\n  want: %+v", got, want)
	}
}

// ---- CRON TICK (Table A) ----

func TestCronTick_Attempt0_InHours_DispatchesCall1(t *testing.T) {
	lead := Lead{Attempt: 0}
	cfg := Defaults()

	p, cmds, err := Transition(lead, EventCronTick{}, cfg, now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPatch(t, p, Patch{
		Attempt:  ptrInt(1),
		CallDate: ptrTime(now),
	})
	assertCmds(t, cmds, []Command{CmdEnqueueRetellCall{Attempt: 1}})
}

func TestCronTick_Attempt0_OutOfHours_NoOp(t *testing.T) {
	lead := Lead{Attempt: 0}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op, got patch=%+v cmds=%+v", p, cmds)
	}
}

func TestCronTick_Attempt2_GapNotMet_NoOp(t *testing.T) {
	lead := Lead{Attempt: 2, CallDate: &twoHoursAgo}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op when retry gap not met")
	}
}

func TestCronTick_Attempt2_GapMet_NoReply_AdvancesToCall2(t *testing.T) {
	lead := Lead{Attempt: 2, CallDate: &threeHoursAgo}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPatch(t, p, Patch{
		Attempt:  ptrInt(3),
		CallDate: ptrTime(now),
	})
	assertCmds(t, cmds, []Command{CmdEnqueueRetellCall{Attempt: 3}})
}

func TestCronTick_Attempt2_HasWAReply_NoOp(t *testing.T) {
	lead := Lead{
		Attempt:         2,
		CallDate:        &threeHoursAgo,
		WhatsappReplyAt: &oneHourAgo,
	}
	// Note: a lead with whatsapp_reply_at set would also have TerminalResponded
	// in reality, but the state machine should also respect WA reply directly.
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op when WA reply present")
	}
}

func TestCronTick_Attempt3_GapMet_AdvancesToCall3(t *testing.T) {
	lead := Lead{Attempt: 3, CallDate: &threeHoursAgo}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPatch(t, p, Patch{
		Attempt:  ptrInt(4),
		CallDate: ptrTime(now),
	})
	assertCmds(t, cmds, []Command{CmdEnqueueRetellCall{Attempt: 4}})
}

func TestCronTick_Attempt4_GapMet_AdvancesToWAFinal(t *testing.T) {
	lead := Lead{Attempt: 4, CallDate: &threeHoursAgo}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertPatch(t, p, Patch{
		Attempt:        ptrInt(5),
		WhatsappSentAt: ptrTime(now),
	})
	assertCmds(t, cmds, []Command{CmdEnqueueWAFinal{}})
}

func TestCronTick_Attempt1_NoOp(t *testing.T) {
	// Waiting for Retell webhook; cron does nothing.
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op at attempt 1")
	}
}

func TestCronTick_TerminalResponded_NoOp(t *testing.T) {
	lead := Lead{Attempt: 2, CallDate: &threeHoursAgo, TerminalResponded: true}
	p, cmds, err := Transition(lead, EventCronTick{}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op when terminal_responded")
	}
}

// ---- CALL ANALYZED (Table B) ----

func TestCallAnalyzed_InvalidDestination_TerminalInvalid(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonInvalidDestination,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TerminalInvalid == nil || !*p.TerminalInvalid {
		t.Errorf("expected terminal_invalid=true, got %+v", p.TerminalInvalid)
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathNoResponse, Status: CRMStatusNotValid}})
}

func TestCallAnalyzed_Voicemail_JumpsToAttempt5(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonVoicemailReached,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 5 {
		t.Errorf("expected attempt=5, got %v", p.Attempt)
	}
	if p.WhatsappSentAt == nil {
		t.Errorf("expected whatsapp_sent_at to be set")
	}
	assertCmds(t, cmds, []Command{CmdEnqueueWAFinal{}})
}

func TestCallAnalyzed_IVR_JumpsToAttempt5(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonIVRReached,
	}
	p, _, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 5 {
		t.Errorf("expected attempt=5 (IVR shortcut), got %v", p.Attempt)
	}
}

func TestCallAnalyzed_VoicemailCustomerType_JumpsToAttempt5(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonUserHangup,
		CustomerType:       "Voicemail",
	}
	p, _, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 5 {
		t.Errorf("expected attempt=5 (voicemail customer type), got %v", p.Attempt)
	}
}

func TestCallAnalyzed_HangupSufficientConvo_AdvancesTo2_CRMPathA(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonUserHangup,
		HasSufficientConvo: true,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 2 {
		t.Errorf("expected attempt=2, got %v", p.Attempt)
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded}})
}

func TestCallAnalyzed_HangupInsufficientConvo_EnqueuesBridging(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonUserHangup,
		HasSufficientConvo: false,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 2 {
		t.Errorf("expected attempt=2, got %v", p.Attempt)
	}
	assertCmds(t, cmds, []Command{CmdEnqueueWABridging{}})
}

func TestCallAnalyzed_NoAnswer_EnqueuesBridging(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonDialNoAnswer,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt == nil || *p.Attempt != 2 {
		t.Errorf("expected attempt=2, got %v", p.Attempt)
	}
	assertCmds(t, cmds, []Command{CmdEnqueueWABridging{}})
}

func TestCallAnalyzed_Attempt3_SufficientConvo_FiresCRMPathA_NoAdvance(t *testing.T) {
	lead := Lead{Attempt: 3, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            3,
		DisconnectedReason: ReasonUserHangup,
		HasSufficientConvo: true,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt != nil {
		t.Errorf("expected no attempt change at attempt 3, got %v", *p.Attempt)
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded}})
}

func TestCallAnalyzed_Attempt3_InsufficientConvo_NoCRMSync(t *testing.T) {
	lead := Lead{Attempt: 3, CallDate: &oneHourAgo}
	ev := EventCallAnalyzed{
		Attempt:            3,
		DisconnectedReason: ReasonDialNoAnswer,
		HasSufficientConvo: false,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt != nil {
		t.Errorf("expected no attempt change, got %v", *p.Attempt)
	}
	if cmds != nil {
		t.Errorf("expected no commands, got %v", cmds)
	}
}

func TestCallAnalyzed_TerminalLead_RecordsFieldsOnly(t *testing.T) {
	lead := Lead{Attempt: 1, CallDate: &oneHourAgo, TerminalInvalid: true}
	ev := EventCallAnalyzed{
		Attempt:            1,
		DisconnectedReason: ReasonUserHangup,
	}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Attempt != nil {
		t.Errorf("expected no attempt change for terminal lead")
	}
	if cmds != nil {
		t.Errorf("expected no commands for terminal lead, got %v", cmds)
	}
	if p.DisconnectedReason == nil {
		t.Errorf("expected disconnected_reason to be recorded even when terminal")
	}
}

// ---- WA INBOUND (Table C) ----

func TestWAInbound_FirstReply_SetsTerminalResponded(t *testing.T) {
	lead := Lead{Attempt: 2}
	ev := EventWAInbound{IsFirstReply: true}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.WhatsappReplyAt == nil {
		t.Errorf("expected whatsapp_reply_at to be set")
	}
	if p.TerminalResponded == nil || !*p.TerminalResponded {
		t.Errorf("expected terminal_responded=true")
	}
	if cmds != nil {
		t.Errorf("expected no commands from state machine; chatbot dispatch is webhook-handler-owned")
	}
}

func TestWAInbound_SubsequentReply_NoOp(t *testing.T) {
	replyTime := now.Add(-10 * time.Minute)
	lead := Lead{Attempt: 2, WhatsappReplyAt: &replyTime, TerminalResponded: true}
	ev := EventWAInbound{IsFirstReply: false}
	p, cmds, err := Transition(lead, ev, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op for subsequent replies")
	}
}

func TestWAInbound_TerminalSpam_Ignored(t *testing.T) {
	lead := Lead{TerminalSpam: true}
	p, cmds, err := Transition(lead, EventWAInbound{IsFirstReply: true}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected spam leads to be ignored")
	}
}

// ---- INTENT CLASSIFIED (Table D) ----

func TestIntentClassified_Callback_SetsInterest2(t *testing.T) {
	p, cmds, err := Transition(Lead{}, EventIntentClassified{Intent: IntentCallback}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Interest2 == nil || *p.Interest2 != IntentCallback {
		t.Errorf("expected interest2=Callback")
	}
	if cmds != nil {
		t.Errorf("expected no commands for Callback")
	}
}

func TestIntentClassified_TidakTertarik_SetsTerminalNotInterestedAndCRM(t *testing.T) {
	p, cmds, err := Transition(Lead{}, EventIntentClassified{Intent: IntentTidakTertarik}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TerminalNotInterested == nil || !*p.TerminalNotInterested {
		t.Errorf("expected terminal_not_interested=true")
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusSpam}})
}

func TestIntentClassified_Agent_SetsTerminalAgentAndCRM(t *testing.T) {
	p, cmds, err := Transition(Lead{}, EventIntentClassified{Intent: IntentAgent}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TerminalAgent == nil || !*p.TerminalAgent {
		t.Errorf("expected terminal_agent=true")
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusAgent}})
}

func TestIntentClassified_Unknown_Errors(t *testing.T) {
	_, _, err := Transition(Lead{}, EventIntentClassified{Intent: "Bogus"}, Defaults(), now, true)
	if err == nil {
		t.Errorf("expected error for unknown intent")
	}
}

// ---- SPAM CLASSIFIED ----

func TestSpamClassified_True_SetsTerminalSpam(t *testing.T) {
	p, cmds, err := Transition(Lead{}, EventSpamClassified{IsSpam: true}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TerminalSpam == nil || !*p.TerminalSpam {
		t.Errorf("expected terminal_spam=true")
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathResponded, Status: CRMStatusSpam}})
}

func TestSpamClassified_False_NoOp(t *testing.T) {
	p, cmds, err := Transition(Lead{}, EventSpamClassified{IsSpam: false}, Defaults(), now, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op for non-spam")
	}
}

// ---- MAX OUT CHECK (Table A last row) ----

func TestMaxOutCheck_Attempt5_GraceMet_SetsCompleted(t *testing.T) {
	lead := Lead{Attempt: 5, WhatsappSentAt: &dayAgo}
	p, cmds, err := Transition(lead, EventMaxOutCheck{}, Defaults(), now, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.TerminalCompleted == nil || !*p.TerminalCompleted {
		t.Errorf("expected terminal_completed=true")
	}
	assertCmds(t, cmds, []Command{CmdEnqueueCRMSync{Path: CRMPathNoResponse}})
}

func TestMaxOutCheck_Attempt5_GraceNotMet_NoOp(t *testing.T) {
	lead := Lead{Attempt: 5, WhatsappSentAt: &twoHoursAgo}
	p, cmds, err := Transition(lead, EventMaxOutCheck{}, Defaults(), now, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op when grace not met")
	}
}

func TestMaxOutCheck_Attempt5_HasReply_NoOp(t *testing.T) {
	reply := dayAgo.Add(1 * time.Hour)
	lead := Lead{Attempt: 5, WhatsappSentAt: &dayAgo, WhatsappReplyAt: &reply}
	p, cmds, err := Transition(lead, EventMaxOutCheck{}, Defaults(), now, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() || cmds != nil {
		t.Errorf("expected no-op when lead has replied")
	}
}

func TestMaxOutCheck_NotAttempt5_NoOp(t *testing.T) {
	lead := Lead{Attempt: 3, WhatsappSentAt: &dayAgo}
	p, _, err := Transition(lead, EventMaxOutCheck{}, Defaults(), now, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !p.IsEmpty() {
		t.Errorf("expected no-op for attempt != 5")
	}
}

// ---- DEFAULTS + CONFIG LOADING ----

func TestLoadConfig_Empty_ReturnsDefaults(t *testing.T) {
	cfg, err := LoadConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AttemptLimit != 5 {
		t.Errorf("expected default attempt_limit=5, got %d", cfg.AttemptLimit)
	}
	if cfg.BusinessHours.Timezone != "Asia/Jakarta" {
		t.Errorf("expected default tz Asia/Jakarta, got %q", cfg.BusinessHours.Timezone)
	}
}

func TestLoadConfig_PartialOverride_PreservesDefaults(t *testing.T) {
	overrides := []byte(`{"call_retry_gap_hours": 6, "business_hours": {"start": "08:00"}}`)
	cfg, err := LoadConfig(overrides)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.CallRetryGapHours != 6 {
		t.Errorf("expected override gap=6, got %d", cfg.CallRetryGapHours)
	}
	// Overridden field in nested struct.
	if cfg.BusinessHours.Start != "08:00" {
		t.Errorf("expected start=08:00, got %q", cfg.BusinessHours.Start)
	}
	// Non-overridden nested fields stay at default.
	if cfg.BusinessHours.End != "20:00" {
		t.Errorf("expected end=20:00 (default), got %q", cfg.BusinessHours.End)
	}
	if cfg.BusinessHours.Timezone != "Asia/Jakarta" {
		t.Errorf("expected tz=Asia/Jakarta (default), got %q", cfg.BusinessHours.Timezone)
	}
	// Untouched top-level field.
	if cfg.AttemptLimit != 5 {
		t.Errorf("expected attempt_limit=5 (default), got %d", cfg.AttemptLimit)
	}
}

func TestLoadConfig_InvalidJSON_Errors(t *testing.T) {
	_, err := LoadConfig([]byte("{not json}"))
	if err == nil {
		t.Errorf("expected error for invalid JSON")
	}
}
