# Multi-Project AI Flow Engine — Design Spec

**Date:** 2026-04-15
**Status:** Approved for planning (brainstorming complete)
**Repo:** `workflow-builder` (Go + Fiber + GORM + Asynq + Postgres + Redis + Next.js)
**Reference implementation:** Anandaya n8n project (stays on n8n; serves as the logic contract for this engine)

---

## 1. Context & Goals

### 1.1 Background

The company runs an AI-driven lead-nurturing pipeline (AI calls + WhatsApp chatbot) for property developers. The existing implementation (project "Anandaya") lives in n8n with state split across Google Sheets and Supabase. Anandaya is in production and will stay on n8n for now — this spec does **not** migrate Anandaya.

This spec describes a new multi-project engine in Go that will host **all future projects** following the same AI-call + chatbot pattern. Each new project is a config-only variant of the same fixed base flow.

### 1.2 Goals

1. **Centralize state.** Postgres is the single source of truth. No more Google Sheets as an operational datastore.
2. **One engine, many projects.** Adding a new project is an admin UI onboarding wizard, not a code change. Target: 5–7 minutes for a fresh project, ~90 seconds when cloning from an existing one.
3. **Behavior parity with Anandaya.** The state machine, guard conditions, timing rules, CRM field mappings, and chatbot logic match the Anandaya reference implementation exactly. Variations happen through per-project config, not forks.
4. **Per-project customization without code changes.** A flat `FlowConfig` jsonb column holds project-specific overrides (attempt limit, retry gap hours, business hours, spam threshold, enabled channels, etc.). Unset fields fall back to compiled defaults.
5. **First-class observability.** Dashboard, lead timeline, chat history, call outcomes, audit trail, DLQ visibility.
6. **Robust against real-world edge cases** that n8n currently handles implicitly via its 2-minute polling cadence: duplicate webhooks, race conditions between user replies and scheduled calls, in-flight state, partial failures.
7. **LeadSquared only** as CRM for MVP. Abstraction surface is a single Go interface so a future CRM can be added without rewriting callers.

### 1.3 Non-goals

- Migration of Anandaya off n8n. (Explicitly out of scope.)
- Generic no-code workflow builder. This engine hosts *one flow shape* (AI call + chatbot lead nurturing); changes to the flow shape itself are code changes, not config.
- Multi-user RBAC. Single shared operator password at MVP.
- Mobile-first UI. Desktop-first; tablet-friendly; not phone.
- Pluggable CRM providers at MVP. LeadSquared only; interface designed for future swap.
- WebSockets / server-push. React Query polling is sufficient for MVP.
- A/B testing, multi-agent orchestration, fine-tuned models, streaming LLM responses.
- Retell agent auto-provisioning via Retell API. Operators paste agent IDs in the wizard; API provisioning is Phase 2.
- LangChain-Go or any framework abstraction over OpenAI. Direct OpenAI SDK + custom Go interfaces for tool calling (tighter control, easier debugging).

### 1.4 Gupshup inbound webhook architecture (important)

Gupshup webhooks for **all projects** (including new ones) are registered into **n8n**, which does minimal processing and forwards to a Go endpoint. This is a deliberate choice: webhook URLs once registered with Gupshup are painful to change, and n8n is already the common registration point. n8n does not own any flow logic for new projects — it is purely a forwarder.

The Go endpoint authenticates forwards via HMAC-SHA256 (per-project shared secret stored as a `webhook_secret` credential), and dedupes by `gupshup_message_id`.

All other webhooks (Retell, LeadSquared, admin-triggered callbacks) are native Go endpoints.

---

## 2. Architecture Overview

### 2.1 Processes (unchanged from existing repo)

- **`cmd/api`** — Fiber HTTP server. REST admin API, webhooks (Retell, chat-inbound-from-n8n), health, session auth.
- **`cmd/worker`** — Asynq worker. Cron-scheduled tasks (ingestion, attempt manager, remarks generator, wa_group dispatch) + event-driven tasks (retell dispatch, gupshup send, chatbot turn, CRM sync, valid-number check).
- **`web/`** — Next.js 14 admin dashboard (App Router, shadcn/ui, Tailwind, React Query).

### 2.2 Module layout

```
internal/
  model/            GORM entities (redesigned; existing types preserved for backward-compat)
  repo/             data access: leadRepo, messageRepo, auditRepo, salesRepo, projectRepo, callRepo, intentRepo
  integrations/
    retell/         (exists; extend)
    leadsquared/    (new)
    gupshup/        (new — outbound send only; inbound via webhook handler)
    twochat/        (new)
    pinecone/       (new)
    openai/         (new)
  statemachine/     pure functions: Transition(lead, event, cfg) -> (patch, []Command, error)
  agent/            chatbot conversation loop
    prompt_builder.go
    tools/
      property_knowledge.go
      save_leads_data.go
    calendar.go
    intent_classifier.go
    spam_classifier.go
    remarks_generator.go
  workflows/
    mortgage/       (existing — preserved)
    n8n/            (existing — preserved)
    leadflow/       (new — the multi-project engine)
      init.go       (registers signatures via sdk.RegisterWorkflow)
      ingest.go     (leadflow.ingest)
      attempt_manager.go
      remarks.go
      wa_group_dispatch.go
  api/
    handlers/
      admin/        (project CRUD, wizard, credentials, prompts, sales, activate/deactivate)
      webhooks/     (retell, chat_inbound, leadsquared_callback)
    scheduler.go    (existing — extend to listen for Redis reload pokes)
pkg/
  phone/            canonical phone normalization
  hours/            business-hours utility (project-aware)
  crypto/           (exists — used by credential store)
  logger/           (exists — extend with context fields)
```

### 2.3 Key architectural decisions

1. **One codepath, many projects.** `leadflow` handlers are project-agnostic. Each handler reads the project ID from its Asynq task payload, loads `FlowConfig`, and executes.
2. **State machine is pure Go, not data.** `internal/statemachine` contains pure functions with no IO — takes current state + event, returns next state + list of side-effect commands. This is the parity contract with Anandaya and is unit-testable in isolation.
3. **Asynq for everything timed or queued.** No direct cron. Webhooks enqueue Asynq tasks rather than doing work inline. Retries, DLQ, rate limiting, and observability come for free.
4. **Existing `Business`/`Credential`/`Workflow`/`Execution`/`ExecutionLog` models are preserved** so the current `mortgage` and `n8n` workflows keep running. Leadflow builds alongside, reusing `Business` as the tenant concept and `Workflow`/`Execution` for cron run tracking only. Per-lead work is raw Asynq tasks without `Execution` rows.
5. **Enqueue after commit, always.** No Asynq `Enqueue` call happens inside a database transaction. Cron handlers are the self-healing backstop for lost enqueues.
6. **Optimistic locking via `version` column** is the single concurrency primitive for lead state writes. No Redis locks, no advisory locks, no application mutexes.

---

## 3. Data Model

### 3.1 Reused from existing repo

- **`businesses`** (the tenant table) — rename conceptually to "Project" in the UI but keep the Go type name so existing workflows compile. Schema additions:
  - `config jsonb NOT NULL DEFAULT '{}'` — `FlowConfig`
  - `timezone varchar(64) NOT NULL DEFAULT 'Asia/Jakarta'`
  - `status varchar(16) NOT NULL DEFAULT 'draft'` — values: `draft | active | paused | archived`
  - `activated_at timestamptz`
- **`credentials`** — unchanged schema. The existing `is_global bool` column is used for the single OpenAI key and the Gmail SMTP credential, which are shared across all projects (`business_id=NULL, is_global=true`). Per-project credentials: `retell_ai`, `leadsquared`, `gupshup`, `twochat`, `pinecone`, `webhook_secret`. Global credentials: `openai`, `gmail_smtp`.
- **`workflows`**, **`executions`**, **`execution_logs`** — reused only for cron-scheduled handlers. One `workflows` row per project per cron kind.

### 3.2 New tables

#### `leads`

Replaces Google Sheets `AI_Call_data` as the authoritative lead record.

```sql
CREATE TABLE leads (
    id                      uuid         PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id             uuid         NOT NULL REFERENCES businesses(id),
    external_id             varchar(128) NOT NULL,        -- LeadSquared OpportunityId
    phone                   varchar(32)  NOT NULL,        -- canonical: "62xxx", no "+"
    name                    varchar(256),
    attempt                 int          NOT NULL DEFAULT 0,
    call_date               timestamptz,                  -- last call dispatch time
    disconnected_reason     varchar(64),                  -- Retell
    interest                varchar(128),                 -- from AI call analysis
    interest2               varchar(64),                  -- from chatbot intent classifier
    customer_type           varchar(64),
    svs_date                timestamptz,
    summary                 text         NOT NULL DEFAULT '',
    whatsapp_sent_at        timestamptz,
    whatsapp_reply_at       timestamptz,
    sent_to_dev             bool         NOT NULL DEFAULT false,
    sent_to_wa_group_at     timestamptz,
    leadsquared_pushed_at   timestamptz,
    valid_number            varchar(8),                   -- "Yes" | "No" | null
    name_alert_sent         bool         NOT NULL DEFAULT false,
    -- terminal flags
    terminal_invalid        bool         NOT NULL DEFAULT false,
    terminal_responded      bool         NOT NULL DEFAULT false,
    terminal_not_interested bool         NOT NULL DEFAULT false,
    terminal_spam           bool         NOT NULL DEFAULT false,
    terminal_agent          bool         NOT NULL DEFAULT false,
    terminal_completed      bool         NOT NULL DEFAULT false,
    -- concurrency + audit
    version                 int          NOT NULL DEFAULT 0,
    created_at              timestamptz  NOT NULL DEFAULT now(),
    updated_at              timestamptz  NOT NULL DEFAULT now(),
    deleted_at              timestamptz,

    UNIQUE (business_id, external_id)
);

CREATE INDEX idx_leads_project_phone ON leads (business_id, phone) WHERE deleted_at IS NULL;
CREATE INDEX idx_leads_attempt_cron  ON leads (business_id, attempt, call_date)
    WHERE deleted_at IS NULL
      AND NOT (terminal_invalid OR terminal_responded OR terminal_not_interested
               OR terminal_spam OR terminal_agent OR terminal_completed);
```

Soft delete via GORM's `DeletedAt`. Internal UUID PK + `(business_id, external_id)` unique is the "both" answer — UUID for internal references, external_id for CRM linkage.

#### `lead_messages`

Append-only chat message log (replaces Supabase `anandaya.conversation`).

```sql
CREATE TABLE lead_messages (
    id                   bigserial PRIMARY KEY,
    business_id          uuid NOT NULL REFERENCES businesses(id),
    lead_id              uuid NOT NULL REFERENCES leads(id),
    direction            varchar(16) NOT NULL,  -- inbound | outbound | system
    role                 varchar(16) NOT NULL,  -- user | assistant | tool | system
    content              text NOT NULL,
    provider_message_id  varchar(128),          -- gupshup_message_id (inbound); null for outbound/system
    token_usage          jsonb,
    created_at           timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, provider_message_id)   -- idempotency for Gupshup webhook replays
);

CREATE INDEX idx_messages_lead_time ON lead_messages (lead_id, created_at);
```

Retained forever (per explicit decision). Review if a project crosses ~10M rows.

#### `lead_audits`

Every Lead state change goes through `leadRepo.Transition()` which writes one row here in the same transaction.

```sql
CREATE TABLE lead_audits (
    id             bigserial PRIMARY KEY,
    business_id    uuid NOT NULL REFERENCES businesses(id),
    lead_id        uuid NOT NULL REFERENCES leads(id),
    actor          varchar(64) NOT NULL,   -- e.g. "attempt_manager", "retell_webhook", "chatbot", "admin_ui"
    event_type     varchar(64) NOT NULL,   -- e.g. "attempt_advanced", "terminal_flag_set", "crm_sync"
    changes        jsonb NOT NULL,         -- { "attempt": [2,3], "terminal_responded": [false,true] }
    correlation_id varchar(64),            -- asynq task id or http request id
    reason         text,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_audits_lead_time ON lead_audits (lead_id, created_at DESC);
CREATE INDEX idx_audits_project_time ON lead_audits (business_id, created_at DESC);
```

**Retention: archived every 6 months** by a daily singleton job `system.audit_archiver`. Rows older than 180 days move to `lead_audits_archive` (identical schema). The `/system/audit` UI page shows a persistent banner: *"Audit entries older than 6 months are automatically archived. Current view shows last 6 months; archived entries remain queryable via 'View archived'."*

#### `call_events`

Retell webhook dedupe + call history for UI.

```sql
CREATE TABLE call_events (
    id                   bigserial PRIMARY KEY,
    business_id          uuid NOT NULL REFERENCES businesses(id),
    lead_id              uuid NOT NULL REFERENCES leads(id),
    retell_call_id       varchar(128) NOT NULL,
    event                varchar(32) NOT NULL,   -- call_analyzed | call_ended | call_started
    status               varchar(32),
    disconnected_reason  varchar(64),
    call_summary         text,
    custom_analysis      jsonb,
    payload              jsonb NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, retell_call_id, event)
);

CREATE INDEX idx_call_events_lead ON call_events (lead_id, created_at DESC);
```

#### `chatbot_states`

1:1 with `leads`, isolates chatbot-specific state.

```sql
CREATE TABLE chatbot_states (
    lead_id        uuid PRIMARY KEY REFERENCES leads(id),
    business_id    uuid NOT NULL REFERENCES businesses(id),
    chat_total     int NOT NULL DEFAULT 0,
    spam_flag      bool NOT NULL DEFAULT false,
    last_chat_at   timestamptz,
    reset_remarks  bool NOT NULL DEFAULT false,
    session_key    varchar(128),
    chatbot_remarks text,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
```

#### `sales_assignments` + `lead_sales_assignments`

Per-project Sales/SPV roster and round-robin record.

```sql
CREATE TABLE sales_assignments (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id        uuid NOT NULL REFERENCES businesses(id),
    sales_name         varchar(128) NOT NULL,
    spv_name           varchar(128),
    wa_group_id        varchar(128),
    is_active          bool NOT NULL DEFAULT true,
    last_assigned_at   timestamptz,
    assign_count       int NOT NULL DEFAULT 0,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE lead_sales_assignments (
    lead_id              uuid NOT NULL REFERENCES leads(id),
    sales_assignment_id  uuid NOT NULL REFERENCES sales_assignments(id),
    assigned_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (lead_id)
);
```

Round-robin algorithm: pick the `is_active=true` row with oldest `last_assigned_at`, update `last_assigned_at=now()`, `assign_count=assign_count+1`. Ties broken by `id`.

#### `project_prompts`

Versioned system prompts per project. Solves the "Retell dashboard has no versioning" gap.

```sql
CREATE TABLE project_prompts (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id  uuid NOT NULL REFERENCES businesses(id),
    kind         varchar(64) NOT NULL,  -- chatbot_system | chatbot_faq | chatbot_tool_instructions
                                        -- | intent_classifier | spam_classifier | remarks_generator
                                        -- | retell_agent_1 | retell_agent_3
    version      int NOT NULL,
    content      text NOT NULL,
    is_active    bool NOT NULL DEFAULT false,
    created_by   varchar(128),
    created_at   timestamptz NOT NULL DEFAULT now(),

    UNIQUE (business_id, kind, version)
);

CREATE UNIQUE INDEX idx_prompts_active ON project_prompts (business_id, kind)
    WHERE is_active = true;
```

Only one version per `(business_id, kind)` may be active at a time.

#### `crm_sync_intents`

Outbox for CRM writes (ensures exactly-once CRM push).

```sql
CREATE TABLE crm_sync_intents (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    business_id  uuid NOT NULL REFERENCES businesses(id),
    lead_id      uuid NOT NULL REFERENCES leads(id),
    path         varchar(8) NOT NULL,    -- A | B
    payload      jsonb NOT NULL,
    status       varchar(16) NOT NULL DEFAULT 'pending',  -- pending | in_progress | done | failed
    attempts     int NOT NULL DEFAULT 0,
    last_error   text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz
);

CREATE INDEX idx_crm_intents_pending ON crm_sync_intents (status, updated_at)
    WHERE status IN ('pending', 'in_progress');
```

### 3.3 `FlowConfig` structure (stored in `businesses.config` jsonb)

Flat, optional fields. Any unset field falls back to a compiled default in `internal/statemachine/defaults.go`.

```json
{
  "attempt_limit":              5,
  "call_retry_gap_hours":       3,
  "max_out_grace_hours":        24,
  "remarks_delay_hours":        5,
  "spam_classify_threshold":    5,
  "voicemail_shortcut_to_last": true,
  "business_hours": {
    "start":    "07:00",
    "end":      "20:00",
    "timezone": "Asia/Jakarta"
  },
  "channels_enabled": ["call", "whatsapp"],
  "ingestion_cron":   "*/2 7-19 * * *",
  "language_code":    "id-ID",
  "crm": {
    "provider":       "leadsquared",
    "tag_filter":     "Anandaya",
    "activity_event": 12002
  },
  "chatbot": {
    "model":                   "gpt-4o-mini",
    "temperature":             0.3,
    "window_size":             4,
    "max_tool_iterations":     3,
    "max_turn_tokens":         8000,
    "pinecone_top_k":          5,
    "calendar_horizon_days":   28,
    "embedding_model":         "text-embedding-3-small",
    "intent_classifier_model": "gpt-4o-mini",
    "spam_classifier_model":   "gpt-4o-mini"
  }
}
```

**Design constraint:** the base logic shape is fixed — you can tune numbers and toggle features, but you cannot reshape branches via config alone. If a project genuinely needs different branching, that is a code change. This is the forcing function that keeps the engine honest.

---

## 4. State Machine

### 4.1 Lead state representation

`(attempt int ∈ [0..5], terminal_flags set, fields)`. Each `attempt` value is a distinct *action*, not a holding state:

| attempt | Meaning |
|---|---|
| 0 | Fresh from ingestion, nothing dispatched |
| 1 | AI call #1 has been dispatched |
| 2 | WA bridging message has been sent (or call 1 ended with sufficient conversation; no WA needed) |
| 3 | AI call #2 has been dispatched |
| 4 | AI call #3 has been dispatched |
| 5 | WA final message has been sent |

**Terminal flags** (mutually compatible):

- `terminal_invalid` — Retell `Invalid_destination`. Hard stop; CRM as "Not Valid".
- `terminal_responded` — `whatsapp_reply_at IS NOT NULL`. Chatbot owns the lead; no more calls.
- `terminal_not_interested` — intent classifier → `Tidak Tertarik`.
- `terminal_spam` — spam classifier fired.
- `terminal_agent` — intent classifier → `Agent`.
- `terminal_completed` — attempt reached `config.attempt_limit` and `max_out_grace_hours` elapsed with no reply.

The attempt manager cron only acts on leads where **no terminal flag is set**.

### 4.2 Guards (pure functions)

| Guard | Definition |
|---|---|
| `InBusinessHours(project)` | Now falls within project's business hours (in project TZ) |
| `HoursSinceLastCall ≥ N` | `now - call_date ≥ config.call_retry_gap_hours` (project TZ) |
| `HasWAReply` | `whatsapp_reply_at IS NOT NULL` |
| `AnyTerminalFlag` | any terminal_* flag is true |
| `HasSufficientConversation` | Retell `custom_analysis.interest != "tidak ada percakapan yang cukup"` |
| `IsInvalidDest` | `disconnected_reason = "Invalid_destination"` |
| `IsVoicemailOrIVR` | `disconnected_reason ∈ {voicemail_reached, ivr_reached}` OR `customer_type = "Voicemail"` |
| `IsHangup` | `disconnected_reason ∈ {user_hangup, agent_hangup}` |
| `IsNoAnswer` | `disconnected_reason ∈ {dial_no_answer, dial_busy}` |

### 4.3 Table A — CRON_TICK dispatch (attempt manager cycle)

Evaluated every cron tick over all leads with `!AnyTerminalFlag`, `project.status='active'`, using `SELECT ... FOR UPDATE SKIP LOCKED LIMIT N`. Each transition is a single DB update with optimistic version check.

| From | Required guards | To | Side effect |
|---|---|---|---|
| `attempt=0` | `InBusinessHours` | `attempt=1`, `call_date=now` | Enqueue `retell.dispatch_call(lead_id, attempt=1)` |
| `attempt=2` | `InBusinessHours` AND `!HasWAReply` AND `HoursSinceLastCall ≥ config.call_retry_gap_hours` | `attempt=3`, `call_date=now` | Enqueue `retell.dispatch_call(lead_id, attempt=3)` |
| `attempt=3` | same as above | `attempt=4`, `call_date=now` | Enqueue `retell.dispatch_call(lead_id, attempt=4)` |
| `attempt=4` | `InBusinessHours` AND `!HasWAReply` AND `HoursSinceLastCall ≥ config.call_retry_gap_hours` | `attempt=5`, `whatsapp_sent_at=now` | Enqueue `gupshup.send_final_wa(lead_id)` |
| `attempt=5` | `!HasWAReply` AND `now - whatsapp_sent_at ≥ config.max_out_grace_hours` (default 24h) | set `terminal_completed` | Enqueue `crm.sync(lead_id, path=B)` |

Leads at `attempt=1/3/4` are "waiting for Retell webhook" and receive no cron action.

### 4.4 Table B — CALL_ANALYZED event (Retell webhook for call dispatched at attempt N)

Dedupe via `call_events` unique `(business_id, retell_call_id, event)`. Duplicate webhooks return 200 OK without reprocessing. Handler updates `call_date`, `disconnected_reason`, `interest`, `customer_type`, `svs_date`, `summary` from payload, then:

| Guards (evaluated in order) | Effect | Side effect |
|---|---|---|
| `IsInvalidDest` | set `terminal_invalid` | Enqueue `crm.sync(path=B, status="Not Valid")` |
| `N=1` AND `IsVoicemailOrIVR` | **jump attempt 1 → 5**, `whatsapp_sent_at=now` | Enqueue `gupshup.send_final_wa(lead_id)` |
| `N=1` AND `IsHangup` AND `HasSufficientConversation` | **advance 1 → 2** (no WA bridging; conversation was enough) | Enqueue `crm.sync(path=A)` |
| `N=1` AND ((`IsHangup` AND `!HasSufficientConversation`) OR `IsNoAnswer`) | **advance 1 → 2** | Enqueue `gupshup.send_bridging_wa(lead_id)` |
| `N ∈ {3, 4}` AND `HasSufficientConversation` | **no attempt change** (counter advances only via cron) | Enqueue `crm.sync(path=A)` |
| `N ∈ {3, 4}` AND `!HasSufficientConversation` | no attempt change | no CRM sync; wait for next cron cycle |

**Invariant:** retry calls (attempts 3 & 4) never trigger a WA bridging message. If call 2 or 3 is unanswered, the cron will advance to the next attempt on its next tick (subject to the 3-hour retry gap). Only attempt 1's outcome can produce a bridging WA.

### 4.5 Table C — WA_INBOUND event

Triggered by `POST /api/webhooks/chat-inbound/:project_slug` (HMAC-authed forward from n8n). Dedupe via `lead_messages` unique `(business_id, provider_message_id)`:

| Guards | Effect | Side effect |
|---|---|---|
| First inbound from this phone, `whatsapp_reply_at IS NULL` | Set `whatsapp_reply_at=now`, set `terminal_responded` | Enqueue `chatbot.process_turn(lead_id, message_id)` |
| Second+ inbound | No state change | Enqueue `chatbot.process_turn` |
| `terminal_spam` or `terminal_not_interested` already set | No state change | **Do not enqueue agent**; audit as `ignored_terminal` |

Setting `terminal_responded` guarantees the attempt manager will not dispatch further calls, even if a call was in-flight at the moment of the reply. An in-flight call's `CALL_ANALYZED` webhook still arrives and updates fields, but cannot escalate the attempt counter (that's cron-only).

### 4.6 Table D — CHAT_INTENT_CLASSIFIED

Runs after every user turn (confirmed: every turn, not gated by message count).

| Classifier output | Effect | Side effect |
|---|---|---|
| `Callback` | set `interest2="Callback"` | — |
| `Tidak Tertarik` | set `interest2="Tidak Tertarik"`, set `terminal_not_interested` | Enqueue `crm.sync(status="Spam")` (per Anandaya's CRM mapping) |
| `Agent` | set `interest2="Agent"`, set `terminal_agent` | Enqueue `crm.sync(contact_result="Agent")` |

Spam classifier fires separately when `chat_total ≥ config.spam_classify_threshold`. On `spam` output: set `chatbot_states.spam_flag=true`, set `terminal_spam`, write audit, fire `crm.sync(status="Spam")`.

### 4.7 Table E — REMARKS_GENERATED (cron)

Query: `chatbot_states` where `reset_remarks = false AND now - last_chat_at ≥ config.remarks_delay_hours`.

For each: run LLM summarizer, write `chatbot_states.chatbot_remarks`, set `reset_remarks=true`. On next user reply, chatbot handler resets `reset_remarks=false` so the next cron will re-summarize.

**Fallback (plugs a gap in the Anandaya implementation):** if a lead has any terminal flag but `reset_remarks=false`, summarization is forced within 10 minutes regardless of the 5-hour rule. Ensures terminal leads always have a remark before their final CRM sync.

### 4.8 Interest precedence rule

When both `interest` (from Retell) and `interest2` (from chatbot) are populated, **chronology wins**: the later write is authoritative. In practice this means the chatbot's `interest2` overrides the earlier Retell `interest` because chatbot conversations happen after the call. The CRM sync path reads both but the MAIN SWITCH branch (matching Anandaya's n8n logic) is ordered so that `interest2` checks short-circuit before `interest` checks, mirroring chronology.

### 4.9 Concurrency invariant

Every write to a `leads` row goes through `leadRepo.Transition()`:

```go
func (r *leadRepo) Transition(ctx context.Context,
    leadID uuid.UUID, expectedVersion int,
    patch LeadPatch, audit AuditEntry) (*Lead, error)
```

Implemented as:

```sql
UPDATE leads
   SET <patch fields>, version = version + 1, updated_at = now()
 WHERE id = $1 AND version = $2 AND deleted_at IS NULL
 RETURNING *;
```

If row count = 0, returns `ErrVersionConflict`. The caller re-reads the lead and re-decides — it does NOT blindly retry. The audit row is inserted in the same transaction.

---

## 5. Idempotency, Concurrency & Error Handling

### 5.1 Webhook idempotency

| Endpoint | Dedupe key | Mechanism | On duplicate |
|---|---|---|---|
| `POST /api/webhooks/retell/:project_slug` | `(business_id, retell_call_id, event)` | Unique index on `call_events` | `INSERT ... ON CONFLICT DO NOTHING`; return 200 |
| `POST /api/webhooks/chat-inbound/:project_slug` | `(business_id, provider_message_id)` | Unique index on `lead_messages` | Same pattern; return 200 |

Both endpoints verify `X-Signature: HMAC-SHA256(body, project_webhook_secret)`. Bad signature → `401`, logged, no further processing.

**Handler ordering:** (1) write dedupe row in a DB transaction, (2) update lead row via `Transition()`, (3) commit, (4) enqueue Asynq follow-up. **Enqueue-after-commit is non-negotiable.**

### 5.2 Asynq task policies

| Task | Task key (dedupe) | Uniqueness window | Retry cap | DLQ after |
|---|---|---|---|---|
| `retell.dispatch_call` | `retell:{lead_id}:{attempt}` | 1h | 3 | 3 failures |
| `gupshup.send_bridging_wa` | `wa-bridging:{lead_id}` | 24h | 3 | 3 failures |
| `gupshup.send_final_wa` | `wa-final:{lead_id}` | 24h | 3 | 3 failures |
| `chatbot.process_turn` | `chat-turn:{lead_message_id}` | 24h | 2 | 2 failures |
| `chatbot.classify_intent` | `intent:{lead_id}:{msg_count}` | 1h | 2 | 2 failures |
| `chatbot.classify_spam` | `spam:{lead_id}:{msg_count}` | 1h | 2 | 2 failures |
| `crm.sync` | `crm:{lead_id}:{attempt}:{path}` | 24h | 5 | 5 failures |
| `twochat.valid_number_check` | `valid-num:{lead_id}` | 24h | 3 | 3 failures |
| `wa_group.send_to_developer` | `wa-group:{lead_id}` | 24h | 3 | 3 failures |
| `leadflow.ingest` (cron) | — | — | 1 | immediate |
| `leadflow.attempt_manager` (cron) | — | — | 1 | immediate |
| `leadflow.remarks_generator` (cron) | — | — | 1 | immediate |
| `leadflow.wa_group_dispatch` (cron) | — | — | 1 | immediate |

CRM sync gets 5 retries because LeadSquared is the most twitchy. Retell and WA get 3 to avoid double-charging on successful-but-slow responses.

### 5.3 Task pre-execution safety checks

Before every external call, the worker:

1. Re-reads the lead with the `expected_version` from the task payload
2. Bails cleanly (no error, audit as `skipped_stale`) if:
   - Version has moved (something else changed state)
   - Any terminal flag is now set
   - For call dispatch only: `HasWAReply` is now true

These are expected outcomes, not errors. They do not count against the retry budget.

### 5.4 The "enqueue after commit" pattern

```go
err := db.Transaction(func(tx *gorm.DB) error {
    // 1. Insert dedupe row (for webhooks)
    // 2. Write lead update via Transition()
    // 3. Write crm_sync_intent row if applicable
    return nil
})
if err != nil { return err }

// 4. ONLY AFTER commit: enqueue Asynq tasks
for _, task := range tasks {
    asynqClient.Enqueue(task, opts...)
}
```

**Self-healing for lost enqueues:** if the Asynq enqueue fails post-commit (Redis down), cron handlers re-scan state every tick and re-enqueue missed work. No transactional outbox is needed for cron-driven flows. Event-driven flows (CRM sync) use the `crm_sync_intents` outbox table.

### 5.5 CRM sync outbox flow

1. Transition writes `crm_sync_intents` row (status=`pending`) inside the lead-update transaction
2. `crm.sync_outbox_poller` singleton runs every 30s:
   - `UPDATE crm_sync_intents SET status='in_progress' WHERE id=? AND status='pending' RETURNING *`
   - Call LeadSquared
   - On success: `status='done'`, set `leadsquared_pushed_at` on lead, write audit
   - On transient failure: `status='pending'`, `attempts++`
   - On permanent failure: `status='failed'`, audit, alert
3. Stale `in_progress` rows (older than 5 minutes) are reclaimed on the next poll

LeadSquared is **assumed non-idempotent**, so duplicate calls are avoided via the outbox status machine; retries happen only after explicit pending re-queue. Every LeadSquared call logs request + response to a debug log for postmortem.

### 5.6 External API error taxonomy

| Type | Example | Retry? | Action |
|---|---|---|---|
| `ErrTransient` | HTTP 5xx, timeout, 429 | Yes (with backoff) | Asynq retries within cap |
| `ErrPermanent` | HTTP 400, 404 (not-found), invalid payload | No | Immediate DLQ + audit |
| `ErrAuth` | HTTP 401/403 | No | Immediate DLQ + **auto-pause project** + **email alert** |

`ErrAuth` auto-pauses the project (`status='paused'`, all cron `workflows.is_active=false`) and sends an email alert so an operator can fix the credential without 10,000 tasks piling up in retries.

### 5.7 Rate limiting

Enforced at the client wrapper level via `golang.org/x/time/rate`:

| Client | Limit | Reason |
|---|---|---|
| Retell | 1 call/sec/project | Retell docs + n8n's 20s loop delay |
| Gupshup send | 5 msg/sec/project | WA platform etiquette |
| 2Chat valid-check | 1 req/2sec/project | Matches n8n |
| LeadSquared update | 2 req/sec/project | Conservative |
| OpenAI | 50 req/min/project | Runaway cost guard |
| Pinecone query | 20 req/sec/project | Well below their limits |

On rate-limit trip, the client returns `ErrTransient` → Asynq retries. No circuit breaker for MVP; Phase 2 addition if outages prove painful.

### 5.8 DLQ monitoring & alerts

Singleton `system.dlq_monitor` runs every 5 minutes:

1. Queries Asynq's archived queue, groups by task type and project
2. If any group exceeds threshold (default 10, configurable in `/system/alerts`), sends an email to configured recipients
3. Alert includes: project slug, task type, sample task payloads, link to `/system/dlq`

Email delivery uses an SMTP configuration stored in env vars (no per-project override needed at MVP).

### 5.9 Observability middleware

All Asynq tasks wrapped in middleware that:

- Injects `{project_id, lead_id, task_id, correlation_id}` into a context logger
- Emits structured JSON log lines for `task_enqueued`, `task_started`, `task_completed`, `task_failed`, `task_skipped_stale`
- Records task duration (Prometheus histogram — Phase 2; not needed for MVP)

---

## 6. Chatbot Agent

### 6.1 Turn lifecycle

One `chatbot.process_turn(lead_id, message_id)` task per inbound message:

```
1. Re-read lead + chatbot_state (fail-fast if terminal_spam or terminal_not_interested)
2. Insert inbound message into lead_messages (deduped at webhook; safety net here)
3. If whatsapp_reply_at IS NULL → set it, set terminal_responded, write audit
4. Load conversation window (last config.window_size turns)
5. Build system prompt (versioned project_prompts + calendar + FAQ + tool instructions)
6. Run OpenAI chat.completions with tools [property_knowledge, save_leads_data]
7. Handle tool calls (up to config.max_tool_iterations)
8. Insert bot reply into lead_messages (role=assistant)
9. Dispatch reply via Gupshup (synchronous call; built-in retry inside the task)
10. Update chatbot_state: chat_total++, last_chat_at=now, reset_remarks=false
11. If chat_total >= config.spam_classify_threshold → enqueue chatbot.classify_spam
12. Enqueue chatbot.classify_intent
```

Steps 1–10 run inside one unit of work. Steps 11 & 12 are enqueue-after-commit. Gupshup send (step 9) is **synchronous inside the task** with retry; this guarantees the reply is delivered before the task marks complete.

### 6.2 System prompt construction

Assembled in cache-friendly order:

```
[ base system prompt ][ FAQ block ][ tool instructions ]   ← cacheable prefix (~3KB)
[ calendar block ]                                          ← changes daily
[ conversation history ]                                    ← changes every turn
[ current user message ]
```

Components:

1. **Base system prompt** — `project_prompts` where `kind='chatbot_system' AND is_active=true`. Plain text, editable in UI.
2. **Calendar block** — generated fresh per turn by `internal/agent/calendar.go`: 28 days starting today, Indonesian week labels (`Hari Ini`, `Besok`, `Minggu Ini`, `Minggu Depan`, `2 Minggu Lagi`, ...). Horizon configurable via `config.chatbot.calendar_horizon_days`.
3. **FAQ block** — `project_prompts` where `kind='chatbot_faq'`. Rendered as "JAWABAN INSTAN (JANGAN PAKAI RAG):".
4. **Tool instructions** — `project_prompts` where `kind='chatbot_tool_instructions'`. Describes when to call `save_leads_data` and `property_knowledge`.

OpenAI prompt caching (50% discount on cached prefix ≥1024 tokens) applies to the static prefix — cuts steady-state cost ~40%. **The prompt assembly order is enforced in `internal/agent/prompt_builder.go` and must not be changed casually.**

### 6.3 Conversation window

`config.chatbot.window_size` turns (default 4) loaded from `lead_messages`. Older messages retained in DB forever but not passed to OpenAI.

```sql
SELECT * FROM lead_messages
 WHERE lead_id = $1 AND role IN ('user', 'assistant')
 ORDER BY created_at DESC
 LIMIT $2 * 2;
-- then reverse in application code
```

### 6.4 Tool: `property_knowledge` (Pinecone RAG)

1. Embed query with `text-embedding-3-small`
2. Query Pinecone index (from project credential) with `top_k=config.chatbot.pinecone_top_k`
3. Return concatenated chunk text to the model
4. Log tool call with role=`tool` in `lead_messages`

### 6.5 Tool: `save_leads_data` (local Go function)

Replaces n8n's WF3 sub-workflow. Schema matches Anandaya exactly (prevents prompt drift):

```json
{
  "name": "save_leads_data",
  "parameters": {
    "nama": "string", "hp": "string", "email": "string",
    "tanggal": "string", "bulan": "string", "tahun": "string", "jam": "string",
    "summary": "string",
    "interest": "enum",
    "interest2": "enum"
  }
}
```

Handler (`internal/agent/tools/save_leads_data.go`):

1. Normalize `bulan` via static Indonesian-month map (Januari=1..Desember=12)
2. Normalize `jam` 24h → 12h AM/PM; empty → `12:00:00 AM`
3. Build `svs_date` string in `M/D/YYYY H:MM:SS AM/PM` format
4. Upsert via `leadRepo.Transition()`:
   - If `interest = "Tertarik Site Visit..."`: set `interest`, `svs_date`, prepend to `summary` (newest-on-top): `{Jakarta timestamp} --- {new}\n\n{existing}`
   - Else (Warm/Cold): set `interest`, `interest2`, prepend to `summary`; do not update `svs_date`
5. Write audit row with `event_type='save_leads_data_tool'`
6. Return `{"status":"ok"}` to the model (silent mode — model never echoes tool I/O to user)

**Summary ordering decision: newest on top.** Sales scan the most recent entry first; older context trails below. Matches the "journal style" of the Anandaya CRM reads.

### 6.6 Intent classifier (`chatbot.classify_intent`)

Separate LLM call, runs **after every user turn**. Inputs: last 4 user messages + existing `interest2`. Model: `gpt-4o-mini`, temperature 0. Rules prompt stored in `project_prompts` with `kind='intent_classifier'`.

Priority-ordered rules (Anandaya WF2-B):

1. SUPER PRIORITY: detail property questions → `Callback`
2. Wrong project without detail questions → `Tidak Tertarik`
3. Affirmative follow-ups ("iya", "masih", "betul", "y") → `Callback`
4. Delaying context ("nanti dikabari", "pikir-pikir") → `Callback`
5. Change-of-mind positive → `Callback`
6. Explicit rejection ("enggak", "tidak", "stop") → `Tidak Tertarik`
7. Agent/marketing claim → `Agent`

Writes `interest2` via `Transition()`; sets terminal flags per Table D.

### 6.7 Spam classifier (`chatbot.classify_spam`)

Fires only when `chat_total ≥ config.spam_classify_threshold` (default 5). Binary output: `spam` / `not_spam`. Model: `gpt-4o-mini`, temperature 0. Rules prompt stored as `kind='spam_classifier'`.

On `spam`: set `chatbot_states.spam_flag=true`, set `terminal_spam`, set `customer_type='Spam'`, write audit, fire `crm.sync(status='Spam')`. No further turns processed.

### 6.8 Cost controls

`config.chatbot.max_turn_tokens` (default 8000): if any single turn's assembled prompt exceeds this, the conversation window is progressively truncated and a warning is logged. Prevents pathological conversations from blowing a project's OpenAI bill.

`config.chatbot.max_tool_iterations` (default 3): limits a runaway tool-call loop to 3 rounds per turn. If the model keeps calling tools past this, the loop breaks and a graceful fallback message is sent.

---

## 7. Admin Dashboard & UX

### 7.1 Tech stack (builds on existing `web/`)

- Next.js 14 App Router, server components where possible
- shadcn/ui (already installed), Tailwind, Lucide icons, sonner toasts
- `react-hook-form` + `zod` (add)
- `@tanstack/react-query` (add) for polling + optimistic updates
- `@tanstack/react-table` (add) for lead lists
- `next-auth` with credentials provider (single shared operator password)

### 7.2 Information architecture

```
Sidebar
  ◉ Overview (all projects)
  ◉ Projects
  ◉ Leads            ─┐
  ◉ Conversations     │  All scoped by topbar project switcher
  ◉ Calls             │  URL prefix: /p/:slug/...
  ◉ Workflows         │
  ◉ Settings         ─┘
    └ Flow config
    └ Prompts
    └ Sales roster
    └ Credentials
    └ Cron schedules
  ◉ System           — global, no project scope
    └ DLQ
    └ Audit log
    └ Alerts
```

**Project switcher** is a `Command` palette (⌘K) in the topbar. Selecting a project scopes every page under `/p/:slug/...`.

### 7.3 Project onboarding wizard

Multi-step, auto-saves after each step, resumable. Creates a draft `businesses` row at Step 1; `is_active=false`, `status='draft'` until final activation.

**Step 1 — Basics:** name, slug, timezone, language, business hours
**Step 2 — Flow config:** all fields pre-filled with Anandaya defaults; operator changes only what differs. "Clone from existing project" button copies another project's config + prompts in one click (credentials and sales excluded).
**Step 3 — Credentials:** one password field + "Verify" button per integration. Verify calls the integration's `Verify()` method; green check on success, red cross + error on failure. Required: Retell, LeadSquared, Gupshup, OpenAI, Pinecone. Optional: 2Chat, webhook HMAC (auto-generated if blank).
**Step 4 — Prompts:** four large textareas with version counters (chatbot system, FAQ, intent classifier, spam classifier). Each has a "Test prompt" button that runs the real OpenAI call against a canned conversation (small cost, catches bugs early).
**Step 5 — Retell agents:** paste agent IDs (attempt 1 agent, retry-attempts agent, from-number). Operator provisions agents in Retell dashboard manually for MVP.
**Step 6 — Sales roster:** inline editable table (name, SPV, WA group ID, active). At least one row required.
**Step 7 — Review & activate:** everything on one page with edit links, then the big "Activate" button.

### 7.4 Project dashboard

Route: `/p/:slug`. The default landing page after selecting a project.

**Stat cards:** leads today (with delta), responded today (count + %), visits scheduled today, calls attempted today, active conversations, DLQ count (red if > 0).

**Attempt funnel:** horizontal bar chart of today's leads by current attempt (0/1/2/3/4/5/terminal). Clicking a bar filters the lead list.

**Activity feed:** real-time stream of audit events, polled every 5s via React Query. 20 most recent.

**Cron tick indicators:** countdowns showing time until next ingestion, attempt manager, remarks generator, wa_group dispatch.

### 7.5 Lead list

Route: `/p/:slug/leads`. Server-paginated `@tanstack/react-table`:

- Columns: phone, name, current attempt, interest, interest2, customer_type, last call, last message, terminal flag badges, created_at
- Filters: attempt, interest, terminal flag, phone/name search, date range
- Sort: any column
- Bulk actions: export CSV only (bulk state changes deliberately not offered)

### 7.6 Lead detail page (the most important single page)

Route: `/p/:slug/leads/:lead_id`. Unified timeline view.

**Header card:** name, phone, external_id (link out to LeadSquared), current attempt, terminal flag badges, call_date, svs_date. Actions: "Force CRM sync now", "Mark not interested", "Mark spam" (each audited).

**Tabs:**
1. **Timeline (default)** — merged chronological view: inbound/outbound messages, call events (with `call_summary` + disconnect reason), state transitions from audit log, remarks generated, tool calls (with `save_leads_data` params). Color-coded by event type.
2. **Messages only** — plain chat transcript
3. **Calls only** — Retell events with expanded `custom_analysis`
4. **Audit log** — raw `lead_audits` rows
5. **Raw** — lead JSON for debugging

### 7.7 Conversations page

Route: `/p/:slug/conversations`. Lists active chatbot conversations (sorted by `last_chat_at DESC`), last-message preview per row, click → lead's messages tab. WhatsApp-Web-style view for operators.

### 7.8 Settings subpages

- **Flow config:** same form as wizard Step 2, editable anytime; changes apply on next cron tick
- **Prompts:** versioned editor with history, diff viewer, rollback button, "Make active"
- **Credentials:** rotate with inline verify; warning dialog before saving (rotating may invalidate in-flight tasks)
- **Sales roster:** inline editable table
- **Cron schedules:** per-cron toggle within the project (granular). Per-project pause via the header action "Pause project". Both granularities supported.

### 7.9 System pages

- **DLQ** (`/system/dlq`) — grouped by task type + project, row actions "Replay"/"Discard", payload viewer
- **Audit log** (`/system/audit`) — global audit stream, filterable. **Persistent info banner: "Audit entries older than 6 months are automatically archived. Current view shows last 6 months; archived entries remain queryable via 'View archived'."**
- **Alerts** (`/system/alerts`) — email recipients, thresholds, "Send test email" button

### 7.10 Real-time updates

React Query polling only (no WebSockets). Intervals:

- Dashboard activity feed: 5s
- Lead list: 15s
- Lead timeline: 10s
- DLQ: 30s

### 7.11 Auth

Single shared operator password via `next-auth` credentials provider. Cookie-based session. One role. RBAC / SSO is Phase 2.

### 7.12 Explicit non-MVP

No multi-user/RBAC, no mobile layout, no in-browser LLM playground, no operator-reply override, no A/B prompt testing, no Pinecone browser.

---

## 8. Project Onboarding Backend Flow

### 8.1 API endpoints (under `/api/admin/*`, session-authed)

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/projects` | Create draft project (wizard Step 1) |
| `GET` | `/projects/:slug` | Load draft for resume |
| `PATCH` | `/projects/:slug` | Save partial wizard step |
| `POST` | `/projects/:slug/clone-from/:source_slug` | Copy config + prompts from source |
| `POST` | `/projects/:slug/credentials/verify` | Test integration credential without persisting |
| `POST` | `/projects/:slug/credentials` | Persist encrypted credential |
| `POST` | `/projects/:slug/prompts` | Save prompt version row |
| `POST` | `/projects/:slug/prompts/:id/test` | Real OpenAI call against canned conversation |
| `POST` | `/projects/:slug/sales` | Upsert sales roster |
| `POST` | `/projects/:slug/activate` | Preflight + transactional activate |
| `POST` | `/projects/:slug/deactivate` | Idempotent pause (status='paused') |
| `POST` | `/workflows/:workflow_id/toggle` | Per-cron enable/disable |

Every mutating endpoint writes to `lead_audits` with `actor='admin_ui'`.

### 8.2 Wizard state persistence

Draft `businesses` row exists from Step 1 with `status='draft'`. Wizard detects resume point by inspecting which fields / credentials / prompts / sales exist. Operator can close the tab and return later.

### 8.3 Credential storage

Uses existing `pkg/crypto` (AES-GCM with 32-byte `ENCRYPTION_KEY`). Flow:

1. Wizard POSTs plaintext to `/credentials/verify` (tested, not persisted)
2. On success, POSTs to `/credentials` (server encrypts, stores in `credentials.data_enc`, never returned in any GET)
3. Plaintext held in memory only for request duration
4. Rotation: new credential row created, old marked `is_active=false` (kept for audit)

**`data_enc` protection:** implemented as an unexported field in its own Go package. Only `pkg/crypto` functions in the same package boundary can read or write the encrypted bytes. This gives compile-time protection: external packages physically cannot reference `data_enc`. No linter grep needed.

### 8.4 The activate flow

Handler for `POST /projects/:slug/activate`:

```
Preflight (outside DB transaction):
1. Load project; confirm status='draft'
2. Verify all required credentials present
   (retell_ai, leadsquared, gupshup, openai, pinecone)
3. Call each integration's Verify() method; fail if any errors
4. Run dry-run LeadSquared ingestion query
   → 0 results: WARN (yellow banner in UI); not a hard fail
   → API error: hard fail
5. Confirm at least one active prompt for each required kind
   (chatbot_system, intent_classifier, spam_classifier)
6. Confirm at least one active sales_assignment row

Transaction:
7. UPDATE businesses SET status='active', activated_at=now() WHERE id=?
   (status is the single source of truth for project activation;
    no separate is_active column on businesses)
8. INSERT 4 workflows rows (one per leadflow signature):
     - leadflow.ingest            cron='*/2 7-19 * * *'
     - leadflow.attempt_manager   cron='*/2 * * * *'
     - leadflow.remarks_generator cron='*/1 * * * *'
     - leadflow.wa_group_dispatch cron='*/2 * * * *'
9. INSERT lead_audits row (event='project_activated')

Post-commit (best-effort):
10. Publish Redis pub/sub message "scheduler:reload" so the running
    scheduler re-reads its active workflow list instantly (instead
    of waiting for its 30s poll)
11. Enqueue a one-off leadflow.ingest task immediately so the
    operator sees first leads flow in within ~1 minute
```

If step 10 or 11 fails, the project is already active — log the error and show a non-blocking warning. The regular cron schedule will pick up normally.

### 8.5 Cron registration via existing scheduler

`internal/api/scheduler.go` already polls `workflows` periodically and syncs Asynq scheduled tasks. No new scheduler code. The `leadflow` package registers its four signatures in `init()`:

```go
func init() {
    sdk.RegisterWorkflow("leadflow.ingest",            WorkflowDef{Handler: handleIngest, ...})
    sdk.RegisterWorkflow("leadflow.attempt_manager",   WorkflowDef{Handler: handleAttemptManager, ...})
    sdk.RegisterWorkflow("leadflow.remarks_generator", WorkflowDef{Handler: handleRemarks, ...})
    sdk.RegisterWorkflow("leadflow.wa_group_dispatch", WorkflowDef{Handler: handleWAGroup, ...})
}
```

Each handler reads `project_id` from its task payload, loads `FlowConfig`, and executes scoped to that project.

### 8.6 Singleton background jobs (not per-project)

Started once per worker process at boot, not as `workflows` rows:

| Job | Cadence | Purpose |
|---|---|---|
| `crm.sync_outbox_poller` | every 30s | Processes pending `crm_sync_intents` |
| `system.dlq_monitor` | every 5 min | Email alerts on DLQ threshold |
| `system.audit_archiver` | daily at 02:00 WIB | Archive `lead_audits` > 180 days to `lead_audits_archive` |

### 8.7 Clone-from-existing flow

`POST /projects/:slug/clone-from/:source_slug`:

1. Create draft project with basics from request body
2. Copy source's `config` jsonb as-is
3. Copy `project_prompts` (all active versions) → new rows at version 1 on new project
4. **Do NOT copy** credentials, Retell agent IDs, `sales_assignments`
5. Return wizard position at Step 3 (credentials)

Target onboarding time when cloning: ~90 seconds.

### 8.8 Deactivation and archival

- `deactivate` → `status='paused'`, `workflows.is_active=false`. In-flight tasks complete naturally (they check `project.status` at start). No data deleted. Re-activate with one click, no re-wizard.
- Archive (separate action with typed confirmation) → soft-delete via `gorm.DeletedAt`. Accessible under "Archived projects" view. No hard delete ever.

---

## 9. Decisions Register

Decisions made during brainstorming, baked into this spec:

| # | Decision | Rationale |
|---|---|---|
| 1 | Anandaya stays on n8n; this engine is for new projects only | User directive; no migration/backfill/cutover needed |
| 2 | Gupshup inbound flows via n8n → HMAC-authed forward to Go | Webhook URLs already registered in n8n; too painful to re-register |
| 3 | Google Sheets dropped as operational datastore | Single source of truth in Postgres |
| 4 | Identical base flow across projects; per-project overrides via `FlowConfig` jsonb | User wants fixed shape, tunable numbers |
| 5 | State machine implemented as pure Go functions in `internal/statemachine` | Testable, parity-verifiable, single source of truth |
| 6 | Lead PK = internal UUID + `(business_id, external_id)` unique | Both: safer internally, linkable externally |
| 7 | Soft delete on leads via `gorm.DeletedAt` | Audit safety |
| 8 | `lead_messages` retained forever | Explicit user directive |
| 9 | `lead_audits` archived every 6 months to `lead_audits_archive` | Bounded active-table growth with persistent banner in UI |
| 10 | Summary append ordering: newest on top | Sales scan most recent first |
| 11 | Intent classifier runs every user turn | Cheap (~$0.0002), matches n8n |
| 12 | Outbound Gupshup send is synchronous inside chatbot turn with retry | Guaranteed delivery before task completes |
| 13 | Optimistic locking via `version` column is the sole concurrency primitive for lead writes | Simple, effective, no Redis/advisory locks |
| 14 | Enqueue-after-commit pattern enforced everywhere | Avoids tasks running against rolled-back state |
| 15 | CRM sync outbox pattern via `crm_sync_intents` table | LeadSquared is assumed non-idempotent |
| 16 | LeadSquared-only CRM for MVP; swappable interface | User directive |
| 17 | Custom Go interfaces over OpenAI SDK; no LangChain-Go | Tighter control, easier debugging |
| 18 | Retell retry calls (attempts 3/4) never trigger WA bridging | Only attempt 1's failure does |
| 19 | `terminal_responded` set at first WA inbound, blocks all future call attempts | Matches Anandaya behavior |
| 20 | Retry calls that end with sufficient conversation fire CRM sync immediately | Matches Anandaya n8n behavior |
| 21 | `max_out_grace_hours` = 24 after final WA before setting `terminal_completed` | Chosen default; configurable |
| 22 | Webhooks always process, even outside business hours | Only new *dispatches* respect business hours |
| 23 | `ErrAuth` auto-pauses the project + emails alert recipients | Prevents runaway retries on bad credentials |
| 24 | MVP auth: single shared operator password via next-auth credentials | Simplest path |
| 25 | "Test prompt" button in wizard Step 4 hits real OpenAI | Costs cents, catches bugs fast |
| 26 | Cron toggle granularity: both per-project and per-cron | Maximum operator control |
| 27 | Preflight "dry-run LeadSquared query returns 0 rows" → warn, not fail | Some projects start with no leads |
| 28 | Redis pub/sub for instant scheduler reload on project activate | Better onboarding UX than 30s poll wait |
| 29 | `data_enc` field unexported, in its own package boundary | Compile-time credential protection |
| 30 | Newest-on-top summary append + prompt-caching-friendly prompt order | Cost and readability optimization |
| 31 | No circuit breaker at MVP; rate limiting only | Simpler; breaker is Phase 2 if needed |
| 32 | Fallback for remarks on terminal leads: force within 10 min regardless of 5h rule | Plugs Anandaya edge case |

---

## 10. Verification Plan

### 10.1 Unit tests

- **State machine** — `internal/statemachine` tests use table-driven fixtures. Each row = (initial lead state, event, FlowConfig) → (expected patch, expected commands). Goal: 100% coverage of the transition matrix. These are the parity contract with Anandaya.
- **Phone normalization** — `pkg/phone` tested against every format variant seen in n8n code nodes.
- **Business hours utility** — `pkg/hours` tested with DST/non-DST, Asia/Jakarta (no DST) and one DST zone for sanity.
- **Prompt builder** — asserts the cache-friendly ordering is maintained; asserts calendar block renders correctly for any date.
- **Tool handlers** — `save_leads_data` tested with all month/hour permutations; output matches Anandaya's exact string format.

### 10.2 Integration tests (httptest + postgres via testcontainers)

- **Webhook idempotency** — replay identical Retell and chat-inbound payloads 10 times; assert exactly one row created, exactly one task enqueued.
- **Concurrency race** — simulate attempt manager picking a lead at version N and WA_INBOUND setting `terminal_responded` at version N+1 simultaneously; assert the losing transition fails cleanly and the winning side's effect is observed.
- **Chatbot turn** — mocked OpenAI + Pinecone + Gupshup clients; run a turn end-to-end; assert messages inserted, audit written, CRM intent created when applicable.
- **Activate flow** — mock all verify calls; run full activate; assert 4 workflow rows created, audit written, Redis pub/sub message published, one ingest task enqueued.

### 10.3 End-to-end smoke (manual for MVP)

1. Create a test project in the wizard (5 minutes target)
2. Ingest one fake LeadSquared opportunity via a test-mode endpoint
3. Observe `leadflow.attempt_manager` advance the lead to attempt=1
4. Fire a mock Retell `call_analyzed` webhook
5. Observe advancement to attempt=2 or jump to 5 (depending on reason)
6. Fire a mock chat inbound
7. Observe chatbot turn, intent classification, CRM intent
8. Observe `crm.sync_outbox_poller` pushing to a mocked LeadSquared

Automated E2E against a real sandbox LeadSquared is Phase 2.

### 10.4 Parity benchmark (Phase 2)

Record a week of Anandaya n8n webhook inputs and n8n actions (calls dispatched, messages sent, CRM updates). Replay the webhook inputs into a Go test harness with the Anandaya-equivalent `FlowConfig`. Assert Go's action list matches n8n's action list row-for-row. This is aspirational for MVP; useful pre-Phase-2 to validate the engine before onboarding a second project.

---

## 11. Open Items for Implementation Phase

None for design. The following will be addressed during implementation planning:

- Exact `leadsquared` client method signatures (pending reading their OpenAPI/docs)
- Exact Gupshup send API method (template vs session messages)
- Exact Pinecone index schema assumptions (metadata fields used for filtering, if any)
- Exact CRM field mapping dictionary for `mx_Custom_*` per CRM sync path (will be codified as a static map in `internal/integrations/leadsquared/crm_mapping.go`)
- Asynq server concurrency + queue weights per task type
- Default email SMTP config (env var names, TLS settings)
- Initial seed `FlowConfig` defaults file (`internal/statemachine/defaults.go`)
- Prompt template for intent classifier, spam classifier, remarks generator (will be pre-filled in wizard based on Anandaya equivalents)

---

## 12. Implementation Sequencing (rough, to be refined by writing-plans)

1. **Data model & migrations** — new tables, `Business` extensions, `pkg/crypto` reorg for unexported `data_enc`
2. **Statemachine package** — pure functions, unit tests
3. **Integrations layer** — retell (extend), leadsquared, gupshup, twochat, pinecone, openai clients + verify methods
4. **Leadflow cron handlers** — ingest, attempt_manager, remarks, wa_group_dispatch
5. **Webhook endpoints** — retell, chat_inbound with HMAC + idempotency
6. **Chatbot agent** — prompt builder, tools, classifiers, remarks generator
7. **CRM sync outbox + poller**
8. **Admin API handlers** — project CRUD, wizard endpoints, activate flow, DLQ, audit
9. **Auth middleware** — next-auth credentials + shared password
10. **Admin dashboard** — onboarding wizard, project dashboard, lead list, lead detail timeline, settings, system pages
11. **Singleton jobs** — dlq_monitor, audit_archiver
12. **Integration tests** — webhook idempotency, concurrency, activate flow
13. **Manual smoke test** — end-to-end on a test project
14. **Documentation & operator runbook**

Exact phasing and task breakdown will be produced by the writing-plans skill in the next step.
