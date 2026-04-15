# AI Call & WA Chatbot — Scalable Flow Logic Blueprint

> **Tujuan**: Dokumentasi logic flow yang **project-agnostic** — bisa diterapkan ke project properti manapun.
> **Fokus**: Decision logic, state transitions, guard flags, data flow, dan branching rules.
> **Sumber**: Deep analysis dari 4 workflow JSON production + system documentation.
> **Dibuat**: 15 April 2026

---

## Daftar Isi

1. [Arsitektur Workflow](#1-arsitektur-workflow)
2. [Data Model & State Fields](#2-data-model--state-fields)
3. [State Machine — Lead Lifecycle](#3-state-machine--lead-lifecycle)
4. [WF0: Get Data — Ingestion Logic](#4-wf0-get-data--ingestion-logic)
5. [WF1-A: Attempt 1 — AI Call Logic](#5-wf1-a-attempt-1--ai-call-logic)
6. [WF1-B: Webhook Handler — Call Result Processing](#6-wf1-b-webhook-handler--call-result-processing)
7. [WF1-C: Attempt 2 — WA Bridging Message Logic](#7-wf1-c-attempt-2--wa-bridging-message-logic)
8. [WF1-D: Attempt 3 & 4 — AI Call Retry Logic](#8-wf1-d-attempt-3--4--ai-call-retry-logic)
9. [WF1-E: Attempt 5 — WA Final Logic](#9-wf1-e-attempt-5--wa-final-logic)
10. [WF1-F: CRM Update Logic](#10-wf1-f-crm-update-logic)
11. [WF1-G: Valid Number Check Logic](#11-wf1-g-valid-number-check-logic)
12. [WF1-H: Send to Developer Group Logic](#12-wf1-h-send-to-developer-group-logic)
13. [WF2: Chatbot — Message Processing Logic](#13-wf2-chatbot--message-processing-logic)
14. [WF2-B: Lead Classifier Logic](#14-wf2-b-lead-classifier-logic)
15. [WF2-C: Spam Classifier Logic](#15-wf2-c-spam-classifier-logic)
16. [WF2-D: Remarks Generator Logic](#16-wf2-d-remarks-generator-logic)
17. [WF3: Tool Simpan Visit — Sub-workflow Logic](#17-wf3-tool-simpan-visit--sub-workflow-logic)
18. [Guard Flags — Complete Reference](#18-guard-flags--complete-reference)
19. [Interest & Customer Type — Value Taxonomy](#19-interest--customer-type--value-taxonomy)
20. [CRM Field Mapping — Decision Matrix](#20-crm-field-mapping--decision-matrix)
21. [WA Group Message — Template Decision Tree](#21-wa-group-message--template-decision-tree)
22. [Parameterization Guide — What Changes Per Project](#22-parameterization-guide--what-changes-per-project)

---

## 1. Arsitektur Workflow

Sistem terdiri dari **4 workflow** yang saling terhubung:

```
┌─────────────────────────────────────────────────────────────────────┐
│                        WORKFLOW ARCHITECTURE                         │
│                                                                      │
│  WF0: GET DATA              WF1: MAIN PROJECT WORKFLOW               │
│  ┌──────────────┐           ┌──────────────────────────────────────┐ │
│  │ CRM Pull     │──write──▶ │ A. Attempt 1 (AI Call)              │ │
│  │ Dedup        │           │ B. Webhook Handler (Call Result)     │ │
│  │ Dual Write   │           │ C. Attempt 2 (WA Bridging)          │ │
│  │ (Sheet + DB) │           │ D. Attempt 3 & 4 (AI Call Retry)    │ │
│  └──────────────┘           │ E. Attempt 5 (WA Final)             │ │
│                             │ F. CRM Update                       │ │
│                             │ G. Valid Number Check                │ │
│                             │ H. Send to Developer WA Group       │ │
│                             └────────────┬─────────────────────────┘ │
│                                          │                           │
│  WF2: CHATBOT                            │                           │
│  ┌──────────────────────────┐            │                           │
│  │ Webhook Receiver         │◀───────────┘ (WA reply triggers)       │
│  │ AI Agent (LLM + RAG)    │                                        │
│  │ Lead Classifier          │                                        │
│  │ Spam Classifier          │                                        │
│  │ Remarks Generator        │──call──▶ WF3: TOOL SIMPAN VISIT       │
│  └──────────────────────────┘           ┌──────────────────┐        │
│                                         │ Save visit data  │        │
│                                         │ Update Sheet     │        │
│                                         └──────────────────┘        │
└─────────────────────────────────────────────────────────────────────┘
```

### Trigger Schedule Summary

| Section | Trigger Type | Interval |
|---------|-------------|----------|
| WF0: Get Data | Cron | Tiap 1-2 menit (jam 07:00-20:00) |
| WF1-A: Attempt 1 | Cron | Tiap 2 menit (jam 07:00-20:00) |
| WF1-B: Webhook | HTTP Webhook | Real-time (event-driven) |
| WF1-C: Attempt 2 | Cron | Tiap 2 menit |
| WF1-D: Attempt 3&4 | Cron | Tiap 2 menit |
| WF1-E: Attempt 5 | Cron | Tiap 2 menit |
| WF1-F: CRM Update | Sheet Trigger | Poll tiap 1 menit |
| WF1-H: WA Group | Cron | Tiap 2 menit |
| WF2: Chatbot | HTTP Webhook | Real-time (event-driven) |
| WF2-D: Remarks | Cron | Tiap 1 menit |

---

## 2. Data Model & State Fields

### Central Record (1 row = 1 lead)

Setiap lead yang masuk direpresentasikan oleh satu row dengan field-field berikut. Ini adalah **keseluruhan state** dari satu lead.

```
┌─────────────────────────────────────────────────────────────────┐
│ LEAD RECORD                                                      │
├──────────────────────┬──────────┬────────────────────────────────┤
│ Field                │ Type     │ Fungsi                          │
├──────────────────────┼──────────┼────────────────────────────────┤
│ id                   │ string   │ Primary key (dari CRM)          │
│ phone                │ string   │ Nomor HP (format: 62xxx)        │
│ name                 │ string   │ Nama lead                       │
│ attempt              │ number   │ Counter attempt saat ini (1-5)  │
│ call_date            │ datetime │ Waktu terakhir di-call          │
│ disconnected_reason  │ string   │ Alasan call terputus            │
│ interest             │ string   │ Hasil klasifikasi AI Call       │
│ interest2            │ string   │ Hasil klasifikasi Chatbot       │
│ customer_type        │ string   │ Tipe customer final             │
│ svs_date             │ datetime │ Jadwal site visit               │
│ summary              │ text     │ Summary percakapan (append)     │
│ whatsapp             │ datetime │ Timestamp WA pertama dikirim    │
│ whatsapp_reply       │ string   │ "1" = sudah reply WA            │
│ sent_to_dev          │ string   │ "Yes" = sudah push ke CRM       │
│ sent_to_wa_group     │ datetime │ Timestamp kirim ke WA group     │
│ leadsquared_timestamp│ datetime │ Guard: timestamp CRM push       │
│ lead_chat_history    │ text     │ Kumpulan pesan user             │
│ Chatbot_Remarks      │ text     │ Ringkasan 3 kalimat             │
│ Last_Leads_Chat      │ datetime │ Timestamp chat terakhir         │
│ Valid_Number          │ string   │ "Yes"/"No" (WA check)           │
│ internal_alert       │ string   │ "1" = alert nama kosong sent    │
│ Sales                │ string   │ Nama sales (auto-assign)        │
│ SPV                  │ string   │ Nama supervisor                 │
└──────────────────────┴──────────┴────────────────────────────────┘
```

### Chatbot State (Separate DB)

```
┌─────────────────────┬──────────┬─────────────────────────────────┐
│ Field               │ Type     │ Fungsi                           │
├─────────────────────┼──────────┼─────────────────────────────────┤
│ id                  │ string   │ PK — sama dengan lead id         │
│ phone               │ string   │ Nomor HP                         │
│ name                │ string   │ Nama lead                        │
│ chat_history        │ text     │ Kumpulan pesan user (comma-sep)  │
│ chat_total          │ number   │ Counter jumlah pesan             │
│ spam                │ string   │ "1" = lead spam                  │
│ conversation        │ text     │ Full log User+Bot                │
│ last_chat           │ datetime │ Timestamp pesan terakhir         │
│ reset_remarks       │ number   │ 1 = sudah diringkas              │
└─────────────────────┴──────────┴─────────────────────────────────┘
```

---

## 3. State Machine — Lead Lifecycle

```
                    ┌──────────────┐
                    │  LEAD MASUK  │  attempt = (empty)
                    │  (from CRM)  │
                    └──────┬───────┘
                           │
                    ┌──────▼───────┐
                    │  ATTEMPT 1   │  attempt = 1
                    │  AI CALL     │
                    └──────┬───────┘
                           │
              ┌────────────┼────────────┐
              ▼            ▼            ▼
        ┌──────────┐ ┌──────────┐ ┌──────────────┐
        │ ANSWERED │ │ NO ANSWR │ │ INVALID/     │
        │ (hangup) │ │(no_answ, │ │ VOICEMAIL/   │
        │          │ │ busy)    │ │ IVR          │
        └────┬─────┘ └────┬─────┘ └──────┬───────┘
             │            │              │
     ┌───────┴──┐         │       ┌──────┴───────┐
     │Sufficient│         │       │Invalid_dest? │
     │conversat.│         │       ├──YES──▶ STOP │
     ├──YES─────┤         │       └──NO: VM/IVR──┤
     │→CRM→Done │         │          ┌────────────┘
     ├──NO──────┤         │          │
     │→Attempt 2│         │          ▼
     └──────────┘         │    ┌──────────┐
                          │    │ ATTEMPT 5 │  SKIP 2,3,4
                          │    │ WA FINAL  │
                          │    └──────────┘
                    ┌─────▼──────┐
                    │ ATTEMPT 2  │  attempt = 2
                    │ WA BRIDGE  │
                    └─────┬──────┘
                          │
                  ┌───────┴───────┐
                  ▼               ▼
            ┌──────────┐    ┌──────────┐
            │ WA REPLY │    │ NO REPLY │
            │(reply=1) │    │          │
            └────┬─────┘    └────┬─────┘
                 │               │
                 ▼               ▼
           ┌──────────┐   ┌──────────┐
           │ CHATBOT  │   │ ATTEMPT 3│  attempt = 3 (≥3hr gap)
           │ HANDLES  │   │ AI CALL  │
           │→CRM→Done │   └────┬─────┘
           └──────────┘        │
                               ▼
                         ┌──────────┐
                         │ ATTEMPT 4│  attempt = 4 (≥3hr gap)
                         │ AI CALL  │
                         └────┬─────┘
                              │
                      ┌───────┴───────┐
                      ▼               ▼
                ┌──────────┐    ┌──────────┐
                │ ANSWERED │    │ NO REPLY │
                │→CRM→Done│    │          │
                └──────────┘    └────┬─────┘
                                     ▼
                               ┌──────────┐
                               │ ATTEMPT 5│  attempt = 5
                               │ WA FINAL │
                               └────┬─────┘
                                    │
                            ┌───────┴───────┐
                            ▼               ▼
                      ┌──────────┐    ┌──────────┐
                      │ WA REPLY │    │ NO REPLY │
                      │→Chatbot  │    │ MAXED OUT│
                      │→CRM→Done│    │ ❌ END   │
                      └──────────┘    └──────────┘
```

---

## 4. WF0: Get Data — Ingestion Logic

### Trigger
```
Cron: */2 7-19 * * *  (tiap 2 menit, 07:00-20:00 setiap hari)
```

### Decision Flow

```
START
  │
  ├──[PARALLEL PATH A] ─── Pull data BARU dari CRM API
  │   │
  │   ▼
  │   HTTP POST: CRM Search API
  │   Filter:
  │     - ActivityEvent = {PROJECT_EVENT_CODE}
  │     - CreatedOn = hari ini
  │     - ProjectFilter = {PROJECT_NAME}
  │   Paging: PageIndex=1, PageSize=100
  │   │
  │   ▼
  │   Extract: id (OpportunityId), RelatedProspectId
  │   │
  │   ▼
  │   HTTP GET: CRM Lead Detail API (by ProspectId)
  │   Extract: Phone, FirstName
  │   │
  │   ▼
  │   Transform:
  │     id = OpportunityId
  │     phone = Phone.replace(/\D/g, '').replace(/^0/, '62')
  │     name = FirstName
  │
  ├──[PARALLEL PATH B] ─── Ambil data EXISTING dari Database
  │   SELECT * FROM {project_table}
  │   Transform: id, phone, name
  │
  └──[MERGE] ─── Compare Datasets (by field 'id')
       │
       ▼
       Output: BARU (hanya di CRM) + BERUBAH (ada di keduanya)
       │
       ▼
       Loop per item:
       │
       ▼
       IF name == empty?
       ├── YES ──▶ Upsert Sheet: id + phone SAJA
       │          (JANGAN overwrite nama yang sudah diisi manual)
       └── NO  ──▶ Upsert Sheet: id + phone + name
                   │
                   ▼
                   UPSERT Database: id, phone, name
```

### Key Logic Rules
1. **Deduplication**: Compare existing DB data vs CRM data by `id` — only process new/changed
2. **Dual write**: Sheet + Database (chatbot needs DB for fast read/write)
3. **Name protection**: Empty name from CRM → don't overwrite manually filled name
4. **Phone normalization**: Strip non-digits, leading `0` → `62`

---

## 5. WF1-A: Attempt 1 — AI Call Logic

### Trigger
```
Cron: */2 7-19 * * *  (tiap 2 menit, 07:00-20:00)
```

### Decision Flow

```
READ Sheet → Parse CSV
  │
  ▼
FILTER: attempt empty AND phone not empty
  ├── TRUE  ──▶ IF name not empty?
  │              ├── YES ──▶ [PROCEED TO CALL]
  │              └── NO  ──▶ IF internal_alert empty?
  │                          ├── YES ──▶ Send alert to internal group:
  │                          │          "Nama masih kosong, tolong bantu diisi"
  │                          │          Set internal_alert = 1
  │                          └── NO  ──▶ (skip, alert already sent)
  │
  └── FALSE ──▶ (skip — already processed or no phone)

[PROCEED TO CALL]:
  Set attempt = 1 in Sheet
  │
  ▼
  Loop per lead (batch size = 1):
    │
    ▼
    Generate calendar context (21 days ahead, locale language)
    │
    ▼
    POST AI Call API: create-phone-call
    Body:
      from_number: {PROJECT_PHONE}
      to_number: +{lead.phone}
      override_agent_id: {PROJECT_AGENT_ID}
      retell_llm_dynamic_variables:
        first_name: {lead.name}
        leads_id: {lead.id}
        attempt: "1"
        tanggal_call: {formatted_datetime}
        calendar_context: {21_day_calendar}
        hari_ini: {day_name}
        tanggal_hari_ini: {full_date}
    │
    ▼
    Wait 20 seconds (rate limit protection)
    │
    ▼
    [loop next]
```

### Parameterizable Values (per project)
- `PROJECT_PHONE` — nomor telepon pengirim
- `PROJECT_AGENT_ID` — AI agent prompt di platform AI Call
- Calendar locale & length
- Batch size & wait time

---

## 6. WF1-B: Webhook Handler — Call Result Processing

### Trigger
```
HTTP Webhook POST: /{WEBHOOK_PATH}
```

### Decision Flow

```
Webhook received (AI Call platform callback)
  │
  ▼
IF event == "call_analyzed"?
  ├── NO  ──▶ (ignore other events)
  └── YES ──▶ Continue
       │
       ▼
       Parse datetime → convert to local timezone
       Clean phone: strip '+' prefix
       │
       ▼
       Upsert Sheet (match by id):
         - interest       ← custom_analysis_data['site visit or no']
         - svs_date       ← custom_analysis_data['site visit date']
         - summary        ← call_summary
         - customer_type  ← custom_analysis_data['customer_type']
         - disconnected_reason ← disconnection_reason
         - phone          ← cleaned phone
         - name           ← from call data
         - attempt        ← from dynamic variables
         - call_date      ← formatted start_timestamp
```

### AI Call Result Fields (dari platform callback)

| Field dari AI Call | Map ke | Deskripsi |
|-------------------|--------|-----------|
| `custom_analysis_data['site visit or no']` | `interest` | Klasifikasi interest |
| `custom_analysis_data['site visit date']` | `svs_date` | Tanggal visit jika setuju |
| `call_summary` | `summary` | Ringkasan percakapan |
| `custom_analysis_data['customer_type']` | `customer_type` | Tipe customer |
| `disconnection_reason` | `disconnected_reason` | Alasan call terputus |

---

## 7. WF1-C: Attempt 2 — WA Bridging Message Logic

### Trigger
```
Cron: */2 * * * *  (tiap 2 menit, sepanjang hari)
```

### Decision Flow (SANGAT DETAIL)

```
READ Sheet → Parse CSV
  │
  ▼
IF6: Filter Attempt 2 Entry Guard
  ALL conditions must be TRUE:
    ✅ whatsapp == empty         (belum pernah kirim WA)
    ✅ disconnected_reason != empty   (sudah pernah call)
    ✅ attempt == 1              (baru selesai attempt 1)
    ✅ phone != empty            (ada nomor)
    ✅ disconnected_reason != "Invalid_destination"   (nomor valid)
  │
  ├── FALSE ──▶ [CHECK VOICEMAIL/IVR PATH - see below]
  │
  └── TRUE ──▶ IF user_hangup OR agent_hangup?     ◄── "if user/agent hangup1"
               (Apakah call attempt 1 diangkat?)
               │
               ├── TRUE (DIANGKAT) ──▶ IF interest == "tidak ada percakapan yang cukup"?
               │                       ◄── "if tidak ada percakapan yg cukup1"
               │                       │
               │                       ├── YES (percakapan kurang) ──▶
               │                       │     Update Sheet:
               │                       │       attempt = 2
               │                       │       whatsapp = {timestamp}
               │                       │     ★ SEND BRIDGING MESSAGE via WA API ★
               │                       │     Save bot message to DB conversation
               │                       │     Increment message counter in DB
               │                       │
               │                       └── NO (percakapan cukup) ──▶
               │                             Update Sheet:
               │                               attempt = 2
               │                               whatsapp = {timestamp}
               │                             (TIDAK kirim WA — sudah cukup interaksi)
               │
               └── FALSE (TIDAK DIANGKAT) ──▶
                     Update Sheet:
                       attempt = 2
                       whatsapp = {timestamp}
                     ★ SEND BRIDGING MESSAGE via WA API ★
                     Save bot message to DB conversation
                     Increment message counter in DB

---

[CHECK VOICEMAIL/IVR PATH] ◄── Dari IF6 FALSE branch
  │
  ▼
  IF: voicemail/IVR path (from Attempt 2 filter)
    Filter:
      attempt == 1
      whatsapp == empty
      disconnected_reason = "voicemail_reached" OR "ivr_reached"
    │
    ├── TRUE ──▶ IF whatsapp_reply == empty (no reply)?
    │            ├── YES ──▶ IF attempt < 5?
    │            │           ├── YES ──▶ Update Sheet:
    │            │           │             attempt = 5   ◄── SKIP langsung ke 5!
    │            │           │             whatsapp = {timestamp}
    │            │           │           ★ SEND BRIDGING MESSAGE ★
    │            │           │           Save bot message to DB conversation
    │            │           └── NO ──▶ (already at max)
    │            └── NO ──▶ (already replied, chatbot handles)
    │
    └── FALSE ──▶ (continue to other sections)
```

### Bridging Message Template (Parameterizable)
```
"Halo Kak, Saya {BOT_NAME} {COMPANY_NAME} mewakili perumahan {PROJECT_NAME}
harga mulai {STARTING_PRICE} di {LOCATION}.

Terimakasih, Kakak baru saja mengklik iklan kami,
Apakah kakak tertarik dengan perumahan {PROJECT_NAME}
dan mau diinfokan lebih lanjut?"
```

---

## 8. WF1-D: Attempt 3 & 4 — AI Call Retry Logic

### Trigger
```
Cron: */2 * * * *  (tiap 2 menit)
```

### Filter Guard (9 conditions — ALL must pass)

```
FILTER: "filter attempt only 2 and 3 passed"
  ALL conditions must be TRUE:
    ✅ phone != empty
    ✅ attempt NOT MATCH regex /^(?:|0|1)$/     (bukan empty, 0, atau 1)
    ✅ hours_since(call_date) >= 3               (minimal 3 jam sejak call terakhir)
    ✅ whatsapp_reply != "1"                     (lead BELUM reply WA)
    ✅ attempt < 4                               (belum mencapai attempt 4)
    ✅ disconnected_reason != "Invalid_destination"
    ✅ interest2 != "Tidak Tertarik"             (chatbot belum classify cold)
    ✅ disconnected_reason != "ivr_reached"
    ✅ disconnected_reason != "voicemail_reached"
```

### Decision Flow

```
READ Sheet → Parse CSV → Apply Filter Guard (9 conditions)
  │
  ▼
  Sort by attempt (ascending — attempt 2 duluan, baru 3)
  │
  ▼
  IF user_hangup OR agent_hangup?    ◄── "if user/agent hangup"
  │
  ├── TRUE (ada interaksi sebelumnya) ──▶
  │     IF interest == "tidak ada percakapan yang cukup"?
  │     │
  │     ├── YES ──▶ [RE-CALL with incremented attempt]
  │     │           Loop:
  │     │             Set: attempt = current_attempt + 1
  │     │             Generate calendar context
  │     │             POST AI Call API
  │     │             Update Sheet: attempt, call_date
  │     │             Wait 20s
  │     │
  │     └── NO ──▶ [RE-CALL with incremented attempt]
  │                (same as above — percakapan before was sufficient
  │                 but lead needs follow-up)
  │
  └── FALSE (tidak pernah interaksi) ──▶
        [RE-CALL with incremented attempt]
        (same loop as above)
```

### Key Insight: Guard Flag Interplay

```
┌─────────────────────────────────────────────────────────┐
│ CHATBOT sets whatsapp_reply = "1"                        │
│     → BLOCKS re-call attempt 3/4                         │
│     → Respects lead's choice to engage via chat          │
│                                                          │
│ CHATBOT sets interest2 = "Tidak Tertarik"                │
│     → BLOCKS re-call attempt 3/4                         │
│     → Lead explicitly not interested                     │
│                                                          │
│ VOICEMAIL/IVR at attempt 1                               │
│     → BLOCKS attempt 3/4 (already skipped to 5)          │
│     → Nomor kemungkinan bukan personal                   │
└─────────────────────────────────────────────────────────┘
```

---

## 9. WF1-E: Attempt 5 — WA Final Logic

### Trigger
```
Cron: */2 * * * *  (tiap 2 menit)
```

### Decision Flow

```
READ Sheet → Parse CSV
  │
  ▼
IF: "attempt = 4 and wa reply empty"
  ALL conditions:
    ✅ disconnected_reason != empty
    ✅ attempt == 4
    ✅ whatsapp_reply == empty        (belum reply WA)
    ✅ phone != empty
    ✅ disconnected_reason != "Invalid_destination"
  │
  ├── FALSE ──▶ (skip)
  │
  └── TRUE ──▶ IF user_hangup OR agent_hangup?     ◄── "if user/agent hangup2"
               │
               ├── TRUE ──▶ IF interest == "tidak ada percakapan yang cukup"?
               │            ├── YES ──▶ Update attempt=5, whatsapp=timestamp
               │            │          ★ SEND BRIDGING MESSAGE ★
               │            │          Save to DB conversation
               │            │
               │            └── NO ──▶ Update attempt=5, whatsapp=timestamp
               │                       ★ SEND BRIDGING MESSAGE ★
               │                       Save to DB conversation
               │
               └── FALSE ──▶ Update attempt=5, whatsapp=timestamp
                             ★ SEND BRIDGING MESSAGE ★
                             Save to DB conversation
```

---

## 10. WF1-F: CRM Update Logic

### Trigger
```
Google Sheets Trigger: Poll changes tiap 1 menit
```

### Master Decision Tree

```
Sheet data change detected
  │
  ▼
IF ID NOT EMPTY: id != empty AND attempt != empty?
  ├── FALSE ──▶ (ignore — incomplete data)
  └── TRUE ──▶
       │
       ▼
       IF diangkat / wa reply?    ◄── "if diangkat / wa reply"
       (disconnected_reason == "agent_hangup"
        OR disconnected_reason == "user_hangup"
        OR whatsapp_reply IS NOT EMPTY)
       │
       ├── TRUE (ADA RESPONS) ──▶ [CRM UPDATE PATH A: Responded]
       │
       └── FALSE (TIDAK ADA RESPONS) ──▶ [CRM UPDATE PATH B: No Response]
```

### PATH A: Responded Lead

```
[CRM UPDATE PATH A]
  │
  ▼
  Switch3: interest == "tidak ada percakapan yang cukup"?
  │
  ├── YES ──▶ IF interest2 empty?     ◄── Chatbot belum classify
  │           ├── YES ──▶ IF attempt == 5?
  │           │           ├── YES ──▶ SET: "Connected - Valid Numbers"
  │           │           │          → CRM Push
  │           │           └── NO ──▶ [MAIN SWITCH - see below]
  │           │
  │           └── NO ──▶ [MAIN SWITCH]
  │
  └── NO ──▶ IF interest has a value (Hot/Warm/Cold)?
             ├── YES ──▶ [MAIN SWITCH]
             └── NO ──▶ IF customer_type has value?
                        ├── YES ──▶ Switch1 (customer_type routing)
                        └── NO ──▶ SET: "Spam" → CRM Push

─────────────────────────────────────────────

[MAIN SWITCH] — Interest + Interest2 + Customer_type routing:
  │
  ├── Output 1: interest == "Tertarik Site Visit (Hot Leads)"
  │   → IF customer_type routing:
  │     ├── Double Number/Agent/Spam → Switch1 (type-specific)
  │     └── Else → SET:
  │           contact_status = "Connected"
  │           contact_result = "Interest Project"
  │           mx_Custom_56 = "Yes"           (Is Interested = Yes)
  │           mx_Custom_28 = "Visit Scheduled"
  │           mx_Custom_29 = svs_date
  │         → IF leadsquared_timestamp empty? → CRM Push
  │
  ├── Output 2: interest == "tertarik di informasikan dulu (Warm)"
  │   → IF agent? → Switch1 : else → calback or not
  │     ├── Callback → SET contact_result = "Callback"
  │     └── Not Callback → SET contact_result = "Interest Project" (no visit)
  │   → mx_Custom_56 = "Yes"
  │   → mx_Custom_28 = "" (no visit scheduled)
  │   → IF leadsquared_timestamp empty? → CRM Push
  │
  ├── Output 3: interest == "Tertarik untuk dihubungi (Warm)"
  │   → Same as Output 2
  │
  ├── Output 4: interest == "tidak mau (Cold Leads)"
  │   → IF customer_type not empty?
  │     ├── YES → Switch1 (customer_type routing)
  │     └── NO → SET "Spam" → CRM Push
  │
  ├── Output 5: interest2 == "Mau Diinformasikan" (Chatbot)
  │   → IF summary kosong?
  │     ├── YES → SET "Connected - Interest - no visit"
  │     └── NO → Switch1 (customer_type routing)
  │
  ├── Output 6: interest2 == "Tertarik" (Chatbot)
  │   → Same as Output 5
  │
  ├── Output 7: interest2 == "Tidak Tertarik" (Chatbot)
  │   → SET "Spam" → CRM Push
  │
  ├── Output 8: customer_type == "Inactivity"
  │   → IF customer_type empty? → invalid or not routing
  │   → else → Switch1
  │
  └── Output 9: customer_type == "Interest"
      → SET "Callback" → CRM Push
```

### PATH B: No Response Lead

```
[CRM UPDATE PATH B]
  │
  ▼
  IF customer_type empty?
  │
  ├── YES ──▶ IF disconnected_reason == "Invalid_destination"?
  │           ├── YES ──▶ SET: "Not Valid" → CRM Push
  │           └── NO  ──▶ SET: "Not Connected - No Pick Up" → CRM Push
  │
  └── NO ──▶ IF disconnected_reason == "Invalid_destination"?
             ├── YES ──▶ SET: "Not Valid" → CRM Push
             └── NO  ──▶ IF voicemail OR inactivity?
                         ├── YES ──▶ SET: "Not Connected - No Pick Up" → CRM Push
                         └── NO  ──▶ Switch1 (customer_type routing)
```

### Switch1: Customer Type Sub-routing

```
Switch1: customer_type value routing
  │
  ├── "Spam"          → SET contact_result = "Spam"
  ├── "Agent"         → SET contact_result = "Agent"
  ├── "Unqualified"   → IF interest != Cold? → SET "Unqualified" (mx_Custom_56=Yes)
  │                     IF interest == Cold? → SET (mx_Custom_56=No)
  ├── "Callback"      → SET contact_result = "Callback"
  └── "Double Number"  → SET contact_result = "Double Data"
```

### CRM Push — Common Fields

Setiap SET node menyiapkan data berikut sebelum HTTP POST ke CRM:

```javascript
// Fields yang selalu dikirim:
{
  contact_status,        // "Connected" / "Not Connected" / "Not Valid"
  contact_result,        // "Interest Project" / "No Pick Up" / "Spam" / etc
  id,                    // OpportunityId
  contact_attempt,       // attempt number
  call_date,             // Excel serial → formatted datetime
  summary                // "Call Notes : {summary} | Chat Notes : {Chatbot_Remarks}"
}
```

### Post-CRM-Push Actions

```
After CRM Push:
  │
  ▼
  Update Sheet:
    sent_to_dev = "Yes"
    leadsquared_timestamp = {current_timestamp}
  │
  ▼
  Log ke sheet "fix data for group wa" (tracking)
  │
  ▼
  [Continue to Valid Number Check]
```

---

## 11. WF1-G: Valid Number Check Logic

### Flow (embedded in CRM Update section)

```
After CRM Push + Sheet Update:
  │
  ▼
  IF name NOT EMPTY AND Valid_Number IS EMPTY?     ◄── Gate
  │
  ├── FALSE ──▶ (skip valid check — name missing or already checked)
  │
  └── TRUE ──▶ Loop Over Items:
               │
               ▼
               WA Platform: "Check a phone number for whatsapp account"
               │
               ▼
               Wait 2 seconds (rate limit)
               │
               ▼
               IF on_whatsapp == true?
               │
               ├── TRUE ──▶ Upsert Sheet:
               │             - Valid_Number = "Yes"
               │           Upsert "fix data for group wa" sheet (log)
               │
               └── FALSE ──▶ Update Sheet:
                              - Valid_Number = "No"
                              - sent_to_dev = "No"
                              - interest2 = "Tidak Tertarik"
                              - customer_type = "Spam"
                            (Lead BLOCKED dari developer group)
```

### Key: 5 Instances

Ada **4-5 instance** dari Valid Number Check di workflow, masing-masing untuk routing path berbeda dari CRM Update. Logic identik, hanya source data berbeda.

---

## 12. WF1-H: Send to Developer Group Logic

### Trigger
```
Cron: */2 * * * *  (tiap 2 menit)
```

### Master Filter

```
READ Sheet → Parse CSV
  │
  ▼
IF: "if sent to dev Yes & sent_to_wa_group empty"
  ALL conditions:
    ✅ sent_to_dev == "Yes"
    ✅ sent_to_wa_group == empty           (belum pernah kirim)
    ✅ phone != empty
    ✅ interest2 != "Agent"
    ✅ interest2 != "Spam"
    ✅ interest2 != "Double Data"
    ✅ attempt != empty
  │
  ├── FALSE ──▶ (skip)
  │
  └── TRUE ──▶ IF name filled?      ◄── "if name filled"
               │
               ├── TRUE ──▶ IF sales not empty?    ◄── Check assignment
               │            │
               │            ├── TRUE ──▶ [SWITCH4: WA GROUP TEMPLATE]
               │            │            See Section 21 for template logic
               │            │
               │            └── FALSE ──▶ [SWITCH4: WA GROUP TEMPLATE]
               │                         (same but without sales name)
               │
               └── FALSE ──▶ IF internal_alert empty?
                             │
                             ├── YES ──▶ Send to INTERNAL group:
                             │          "{Project_Name}
                             │           No tlf: +{phone}
                             │           Namanya masih kosong, tolong bantu diisi ya."
                             │          Set internal_alert = 1
                             │
                             └── NO ──▶ (skip, alert sudah dikirim sebelumnya)
```

### Switch4: WA Group Template Selection

```
Switch4:
  │
  ├── Output 1: svs_date NOT EMPTY (ada jadwal visit)
  │   → Send HOT template to developer group
  │   → Update sent_to_wa_group = {timestamp}
  │
  ├── Output 2: svs_date EMPTY (no jadwal)
  │   → IF tertarik/mau info?
  │     ├── Callback/Mau Diinformasikan → WARM template
  │     └── Not Callback → DEFAULT template
  │   → IF callback/unqualified?
  │     ├── YES → CALLBACK template
  │     └── NO → DEFAULT template
  │   → Update sent_to_wa_group = {timestamp}
  │
  └── Output 3: interest == "tidak ada percakapan yang cukup"
      → BASIC template (minimal info)
      → Update sent_to_wa_group = {timestamp}
```

---

## 13. WF2: Chatbot — Message Processing Logic

### Trigger
```
HTTP Webhook POST: /{CHATBOT_WEBHOOK_PATH}
(Triggered when lead replies to WA bridging message)
```

### Decision Flow

```
WA Message Received (via webhook)
  │
  ├──[IMMEDIATE — PARALLEL 1]
  │   Clean phone: strip non-digits
  │   Update Sheet: whatsapp_reply = "1"    ◄── Guard flag!
  │   (This immediately tells WF1 to STOP re-calling this lead)
  │
  ├──[IMMEDIATE — PARALLEL 2]
  │   Increment message counter in DB
  │
  └──[PROCESS — PARALLEL 3]
      │
      ▼
      SELECT FROM {project_table} WHERE phone = {cleaned_phone}
      │
      ▼
      IF2: spam != "1" AND phone matches DB record?
      │
      ├── FALSE ──▶ No Operation (ignore spam/unknown)
      │
      └── TRUE ──▶ [3 PARALLEL PROCESSES]:
                   │
                   ├── [A: AI AGENT RESPONSE]
                   │   Generate 28-day calendar context
                   │   │
                   │   ▼
                   │   AI Agent "Lina" (LLM + RAG):
                   │     input: user message
                   │     tools: property_knowledge (RAG), save_leads_data (WF3)
                   │     memory: window buffer 4 messages, key: {phone}{salt}
                   │   │
                   │   ▼
                   │   Send reply via WA API (Gupshup)
                   │   │
                   │   ▼
                   │   Log to DB:
                   │     User message → conversation append
                   │     Bot reply → conversation append
                   │     Update last_chat = now
                   │
                   ├── [B: LEAD CLASSIFIER] (setiap pesan)
                   │   See Section 14
                   │
                   └── [C: SPAM CLASSIFIER] (conditional)
                       See Section 15
```

### AI Agent Architecture

```
┌─────────────────────────────────────────────────┐
│ AI AGENT                                         │
│                                                  │
│ LLM: GPT-4o-mini (temperature 0.3)             │
│ Memory: Window Buffer (4 messages)               │
│ Session Key: {phone}{SALT}                       │
│                                                  │
│ Tools:                                           │
│ ┌────────────────────┐ ┌──────────────────────┐ │
│ │ property_knowledge │ │ save_leads_data      │ │
│ │ (Pinecone RAG)     │ │ (calls WF3)          │ │
│ │ Vector search      │ │ Saves visit/interest │ │
│ └────────────────────┘ └──────────────────────┘ │
│                                                  │
│ System Prompt Structure (Parameterizable):       │
│ 1. Calendar context (dynamic, 21-28 days)       │
│ 2. Tool calling rules                           │
│ 3. Date logic rules                             │
│ 4. Knowledge source rules                       │
│ 5. Main task (site visit persuasion)            │
│ 6. Language style                               │
│ 7. Bridging context                             │
│ 8. CTA rules                                   │
│ 9. Scenario handling                            │
│ 10. FAQ overrides (hardcoded answers)           │
└─────────────────────────────────────────────────┘
```

---

## 14. WF2-B: Lead Classifier Logic

### Trigger: Setiap pesan masuk (inline, parallel with AI Agent)

```
After DB lookup success (spam!=1, phone matches):
  │
  ▼
  Update DB:
    chat_total += 1
    chat_history = append(user_message)
  │
  ▼
  LLM Chain (Basic LLM Chain):
    Input: chat_history (all user messages)
    System prompt: Lead Classifier (see below)
    Output: ONE of "Callback" | "Tidak Tertarik" | "Agent"
  │
  ▼
  Update Sheet:
    interest2 = {classifier_output}
    Last_Leads_Chat = {timestamp}
  │
  ▼
  Update DB: reset_remarks = NULL    (trigger re-summarization)
  │
  ▼
  Switch: interest2 value?
  │
  ├── "Agent" ──▶ Update Sheet: customer_type = "Agent"
  │               Update DB column
  │
  └── "Callback" ──▶ Update Sheet: customer_type = "Callback"
                     Update DB column
```

### Classifier Rules (Priority Order — Parameterizable)

```
PRIORITAS TERTINGGI → TERENDAH:

1. SUPER PRIORITAS — ACTIVE INQUIRY
   IF user asks detail property questions (harga, fasilitas, lokasi, cicilan)
   → "Callback" (EVEN IF they mention competitor projects)

2. WRONG PROJECT WITHOUT QUESTIONS
   IF user mentions competitor project AND no detail questions
   → "Tidak Tertarik"

3. AFFIRMATIVE FOLLOW-UP
   IF user responds with affirmation ("iya", "masih", "betul", "y")
   AND does NOT mention other projects
   → "Callback"

4. DELAYING CONTEXT
   IF user asks for time ("nanti dikabari", "pikir-pikir", "insyaallah")
   AND does NOT mention other projects
   → "Callback"

5. CHANGE OF MIND
   IF initially rejected BUT later shows positive interest
   → "Callback"

6. EXPLICIT REJECTION
   IF user firmly rejects ("enggak", "tidak", "stop")
   → "Tidak Tertarik"

7. AGENT/MARKETING
   IF user claims to be marketing/agent from another company
   → "Agent"
```

---

## 15. WF2-C: Spam Classifier Logic

### Trigger: Conditional (only when chat_total >= 5)

```
After DB chat_total update:
  │
  ▼
  IF chat_total >= 5?
  │
  ├── FALSE ──▶ (skip — not enough messages to determine spam)
  │
  └── TRUE ──▶ LLM Chain (AI Agent1):
               Input: chat_history
               System prompt: Binary classifier → "spam" / "not_spam"
               │
               ▼
               IF output == "spam"?
               │
               ├── TRUE ──▶ Update DB: spam = "1"
               │            Update Sheet: customer_type = "Spam"
               │            (Lead BLOCKED from chatbot going forward)
               │
               └── FALSE ──▶ (no action — not spam)
```

### Spam Classification Rules
```
"not_spam" IF:
  - Message is about buying/selling/asking about property
  - Specifically about the project
  - First-time property inquiry

"spam" IF:
  - Repeated nonsensical messages
  - Promotional content
  - Unrelated topics after 5+ messages
```

---

## 16. WF2-D: Remarks Generator Logic

### Trigger
```
Cron: */1 * * * *  (tiap 1 menit)
```

### Decision Flow

```
SELECT FROM {project_table} WHERE reset_remarks IS NULL
  │
  ▼
  Loop per lead:
    │
    ▼
    IF hours_since(last_chat) >= 5 AND reset_remarks IS NULL?
    │
    ├── FALSE ──▶ (skip — masih aktif chat atau sudah di-summarize)
    │
    └── TRUE ──▶ LLM Chain (Basic LLM Chain3):
                 Input: conversation (full User+Bot log)
                 System prompt: Sales Analyst Summarizer
                   - Max 3 kalimat
                   - Bahasa Indonesia
                   - Gaya catatan sales
                   - Fokus: interest level, info requests, visit status, last state
                   - If no response: "Leads sudah dihubungi, belum ada respon"
                 │
                 ▼
                 Update Sheet: Chatbot_Remarks = {summary_output}
                 Update DB: reset_remarks = 1
```

### Re-trigger Mechanism
```
Lead chats again → WF2 sets reset_remarks = NULL
→ Next cron cycle: Remarks Generator detects NULL
→ Waits 5 hours after last_chat
→ Re-generates updated summary
→ CRM Update picks up new Chatbot_Remarks on next push
```

---

## 17. WF3: Tool Simpan Visit — Sub-workflow Logic

### Trigger: Called by AI Agent (WF2) via tool `save_leads_data`

### Input Parameters (10 fields)
```
nama, hp, email, tanggal, bulan, tahun, jam, summary, interest, interest2
```

### Decision Flow

```
Input from AI Agent
  │
  ▼
  ALWAYS: Append to chatbot_data sheet (raw log — audit trail)
  │
  ▼
  IF interest == "Tertarik Site Visit (memberikan jadwal) (Hot Leads)"?
  │
  ├── TRUE (HOT) ──▶
  │     Lookup existing data in Sheet by phone
  │     │
  │     ▼
  │     Upsert Sheet AI_Call_data:
  │       phone, interest, svs_date (formatted), summary (append with timestamp)
  │     svs_date format: M/D/YYYY H:mm:ss AM/PM
  │     Month conversion: Indonesian name → number
  │     Hour conversion: 24h → 12h AM/PM
  │
  └── FALSE (WARM/COLD) ──▶
        Lookup existing data in Sheet by phone
        │
        ▼
        Upsert Sheet AI_Call_data:
          phone, interest, summary (append with timestamp)
        (svs_date TIDAK diupdate — no visit scheduled)
```

### Summary Append Format
```
"{existing_summary}\n\n{Jakarta_datetime} --- {new_summary}"
```

---

## 18. Guard Flags — Complete Reference

```
┌────────────────────────────┬──────────────┬──────────────────────────┬──────────────────────────────┐
│ Flag                       │ Set By       │ Trigger                  │ Effect                       │
├────────────────────────────┼──────────────┼──────────────────────────┼──────────────────────────────┤
│ attempt = (empty)          │ WF0          │ Lead baru masuk           │ Triggers Attempt 1           │
│ attempt = 1                │ WF1-A        │ Before AI Call            │ Marks as "in progress"       │
│ attempt = 2                │ WF1-C        │ WA bridging sent          │ Blocks re-entry to Att 1     │
│ attempt = 3,4              │ WF1-D        │ Re-call made              │ Tracks retry progress        │
│ attempt = 5                │ WF1-C/E      │ Voicemail OR att 4 done   │ Final attempt (WA)           │
│                            │              │                          │                              │
│ whatsapp_reply = "1"       │ WF2          │ Lead replies WA           │ BLOCKS re-call att 3/4       │
│ interest2 = "Tidak Tertarik"│ WF2 classif.│ Chatbot classifies cold   │ BLOCKS re-call att 3/4       │
│                            │              │                          │                              │
│ leadsquared_times## 22. New Project Setup — Complete Checklist & Gap Analysis

Bagian ini menjelaskan **langkah by langkah** setup project baru di n8n, termasuk analisis item yang mungkin terlewat.

### 22.1 Yang Kamu Sudah Lakukan (5 area)

#### ✅ 1. LeadSquared HTTP Request

Setup baru HTTP Request untuk CRM pull data sesuai project:
- `HTTP Request20`: URL endpoint CRM Search API
- Filter: `mx_Custom_1` = `{PROJECT_NAME}` (beda per project)
- `HTTP Request21`: URL endpoint CRM Get Lead by ID
- **7 HTTP Request nodes CRM Update** (HTTP Request2,3,5,6,10,11,12): URL endpoint update dengan `leadId` yang sama tapi `ProspectOpportunityId` dari data dinamis

#### ✅ 2. Retell AI Call HTTP Request

Update 2 node — `AI CALL(RETELL)` dan `AI CALL(RETELL)1`:
```json
{
  "from_number": "{PROJECT_PHONE}",
  "to_number": "+{{ lead.phone }}",
  "call_type": "phone_call",
  "override_agent_id": "{PROJECT_AGENT_ID}",
  "retell_llm_dynamic_variables": {
    "first_name": "{{ lead.name }}",
    "leads_id": "{{ lead.id }}",
    "attempt": "{N}",
    "tanggal_call": "{{ formatted_datetime }}",
    "calendar_context": "{{ calendar_output }}",
    "hari_ini": "{{ day_name }}",
    "tanggal_hari_ini": "{{ full_date }}"
  }
}
```
**Yang berubah per project**: `from_number`, `override_agent_id`
**Yang SAMA**: struktur `retell_llm_dynamic_variables` (semua key tetap sama, value dari data lead)

#### ✅ 3. Gupshup & 2Chat Configuration

| Komponen | Yang Diubah | Node di n8n |
|----------|------------|-------------|
| Gupshup webhook (receive) | Webhook path baru | `2chat-input-{project}` (di WF2) |
| Gupshup API send bridging | Message template + credentials | `send wa message`, `initial wa message`, `send wa message1` |
| 2Chat send to group | `wa_group_id` berbeda | `Send a message to a whatsapp group` (6,7,8,2 + internal) |
| 2Chat WA number | Assignment nomor WA | 2Chat Trigger node |

**Bridging message** yang perlu diubah (3 node):
```
"Halo Kak, Saya {BOT_NAME} {COMPANY} mewakili perumahan {PROJECT_NAME}
harga mulai {PRICE} di {LOCATION}.

Terimakasih, Kakak baru saja mengklik iklan kami,
Apakah kakak tertarik dengan perumahan {PROJECT_NAME}
dan mau diinfokan lebih lanjut?"
```

**WA Group message templates** (4-5 node):
```
"New Leads {PROJECT_NAME}
{name} {phone}
..."
```
```
"Projek {PROJECT_NAME}
No tlf: +{phone}
Namanya masih kosong, tolong bantu diisi ya."
```

#### ✅ 4. Data Storage Configuration

| Storage | Yang Diubah | Jumlah Node Terdampak |
|---------|------------|----------------------|
| Google Sheets (main) | `spreadsheet_id` baru, sheet names sama | **~31 nodes** di WF1 |
| Google Sheets (chatbot) | Sheet di WF2 + WF3 | **~7 nodes** di WF2, **~5 nodes** di WF3 |
| Google Sheets CSV reads | URL export dengan spreadsheet_id baru | **3 nodes** (`get csv data`, `get csv data1`, `get csv data2`) |
| Google Sheets Trigger | `documentId` baru | **1 node** (`Google Sheets Trigger`) |
| PostgreSQL table | Beda table name (e.g., `public.{project}`) | **~16 nodes** across WF1 + WF2 |
| Sales assignment | Daftar sales berbeda di sheet `fix data for group wa` | Spreadsheet formula |

#### ✅ 5. Pinecone Vector Database

| Komponen | Yang Diubah |
|----------|------------|
| Index name | `{project}-update-{version}` (baru per project) |
| Knowledge data | Dokumen properti baru diupload ke Google Drive |
| Embeddings | Upsert ke Pinecone index baru |
| RAG tool | `property_knowledge` node di WF2 → point ke index baru |

---

### 22.2 Yang KURANG / Mungkin Terlewat (⚠️ Gap Analysis)

Berdasarkan deep analysis JSON workflow, ada **6 item tambahan** yang juga perlu diubah tapi belum disebutkan:

#### ⚠️ 6. Chatbot System Prompt (WF2: AI Agent node)

**INI SANGAT PENTING** — System prompt chatbot di node `AI Agent` punya **banyak hardcode project-specific**:

```
Yang perlu diubah di system prompt:
├── Nama bot: "Lina AI" → {BOT_NAME}
├── Nama project: "Anandaya" → {PROJECT_NAME}  (disebut 15+ kali!)
├── Harga: "400 jutaan" → {STARTING_PRICE}
├── Lokasi: "Parung Panjang, Serpong Selatan" → {LOCATION}
├── FAQ hardcoded:
│   ├── Ready/indent: "Indent 6-12 bulan" → {READY_STATUS}
│   ├── Legalitas: "SHM (Bisa AJB...)" → {LEGALITY}
│   ├── Pricelist answer → {PRICELIST_ANSWER}
│   ├── Akses & lokasi detail → {ACCESS_INFO}
│   └── Cicilan info → {INSTALLMENT_INFO}
├── Bridging context (pesan awal) → harus match bridging message
├── Scenario responses (skenario awal) → project-aware
└── Tool `save_leads_data` → workflowId ke WF3 yang baru
```

**Node affected**: `AI Agent` (system prompt ~3000+ karakter)

#### ⚠️ 7. Lead Classifier Prompt (WF2: Basic LLM Chain node)

Prompt classifier juga menyebut nama project:

```
"Kamu adalah Lead Classifier System untuk penjualan properti
khusus Proyek {PROJECT_NAME}..."
```

**Node affected**: `Basic LLM Chain` (lead classifier system message)

#### ⚠️ 8. Counter ID di PostgreSQL

SQL query untuk counter tracking **hardcode** nama counter:

```sql
-- SEKARANG:
UPDATE counter SET total_sent = total_sent + 1 WHERE id = 'counter_anandaya';

-- HARUS JADI:
UPDATE counter SET total_sent = total_sent + 1 WHERE id = 'counter_{project}';
```

**Nodes affected**: 
- `update counter total sent chat in DB` (WF1)
- `Execute a SQL query` (WF2) — 2 instance

**Action**: Buat row baru di tabel `counter` untuk setiap project baru.

#### ⚠️ 9. Bot Conversation SQL Queries (WF1 + WF2)

Semua SQL query di PostgreSQL **hardcode table name** `anandaya`:

```sql
-- WF1 (3 nodes):
UPDATE anandaya SET conversation = ... WHERE phone = $1;

-- WF2 (6+ nodes):
SELECT * FROM anandaya WHERE phone = ...;
UPDATE anandaya SET conversation = ... WHERE phone = $1;
UPDATE anandaya SET last_chat = ..., reset_remarks = NULL;
UPDATE anandaya SET spam = '1' WHERE phone = ...;
```

**Nodes affected (WF1)**: `update bot conversation`
**Nodes affected (WF2)**: 
- `Select rows from a table` (2 instance)
- `Update rows in a table` (3 instance)
- `Execute a SQL query` (5 instance — SQL queries)

**Total**: ~11 PostgreSQL nodes perlu ganti table name

#### ⚠️ 10. Tool Simpan Visit Sub-workflow (WF3)

WF3 (`Tool - Simpan Visit-Anandaya`) perlu **diduplikasi per project** karena:
- Sheet IDs berbeda (5 Google Sheets nodes)
- Workflow ID berbeda → node `save_leads_data` di WF2 harus point ke WF3 yang baru

```
WF2 (AI Agent) → save_leads_data tool → workflowId: "ffZtHJBr31a8NZXn"
                                         ↑ HARUS GANTI ke WF3 project baru
```

**Action**: 
1. Duplicate WF3
2. Update semua Sheet ID nodes di WF3 baru
3. Update `save_leads_data` → `workflowId` di WF2 baru

#### ⚠️ 11. Chatbot Webhook Path (WF2)

Webhook `2chat-input-anandaya` punya path yang unique:
```
path: "769b7e00-c257-49a7-af87-5a52c31bfe94"
```

Setiap project butuh webhook path berbeda agar message routing benar.
**Juga**: nama webhook node disebut di banyak expression reference di WF2:
```javascript
$('2chat-input-anandaya').item.json.body.text
$('2chat-input-anandaya').item.json.body.mobile
```

**Action**: 
1. Buat webhook baru dengan path baru
2. **Rename** webhook node atau update SEMUA `$('2chat-input-...')` reference di WF2

---

### 22.3 Complete Node-by-Node Inventory — Apa yang Berubah Per Project

#### WF0: Get Data (embedded di WF1)

| # | Node Name | Yang Diubah | Tipe |
|---|-----------|------------|------|
| 1 | `HTTP Request20` | CRM API URL + project filter (`mx_Custom_1`) | HTTP |
| 2 | `HTTP Request21` | CRM API URL (sama endpoint, beda auth jika beda account) | HTTP |
| 3 | `cek nomor yg sudah ada1` | PostgreSQL table name | Postgres |
| 4 | `Insert or update rows in a table1` | PostgreSQL table name | Postgres |

#### WF1: Main Project Workflow

| Section | Node Name | Yang Diubah |
|---------|-----------|------------|
| **Attempt 1** | `update attempt 1` | Sheet ID |
| | `AI CALL(RETELL)` | `from_number`, `agent_id` |
| | `get csv data1` | CSV export URL (sheet ID) |
| **Webhook** | `Webhook` | Webhook path |
| | `update data from retell webhook` | Sheet ID |
| **Attempt 2** | `get csv data` | CSV export URL (sheet ID) |
| | `send wa message` | Gupshup credentials + message template |
| | `initial wa message` | Gupshup credentials + message template |
| | `update bot conversation` | PostgreSQL SQL (table name) |
| | `update counter total sent chat in DB` | PostgreSQL SQL (counter_id) |
| | `update attempt and last call date` | Sheet ID |
| | `+1 attempt and update last wa date` | Sheet ID |
| **Attempt 3&4** | `AI CALL(RETELL)1` | `from_number`, `agent_id` |
| **Attempt 5** | `send wa message1` | Gupshup credentials + message template |
| | `update attempt 5 and last wa date` | Sheet ID |
| | `+1 attempt and update last wa date1` | Sheet ID |
| **CRM Update** | `Google Sheets Trigger` | Sheet ID (documentId) |
| | `HTTP Request2,3,5,6,10,11,12` (7 nodes) | CRM API URL (shared) |
| | `Append or update row in sheet` (6+ nodes) | Sheet ID |
| | `Update row in sheet` (8+ nodes) | Sheet ID |
| **Valid Check** | `Check a phone number for whatsapp account` (4 nodes) | 2Chat credentials |
| **WA Group** | `get csv data2` | CSV export URL (sheet ID) |
| | `Send a message to a whatsapp group` (5 nodes) | Group ID + message template |
| | `internal alert = 1` | Sheet ID |

**Total WF1**: ~50+ nodes need project-specific updates

#### WF2: Chatbot Workflow

| # | Node Name | Yang Diubah |
|---|-----------|------------|
| 1 | `2chat-input-{project}` (webhook) | Webhook path + name |
| 2 | `Code in JavaScript3` | Phone cleaning reference (`$('2chat-input-...')`) |
| 3 | `update whatsapp_reply` | Sheet ID |
| 4 | `Select rows from a table` | PostgreSQL table |
| 5 | `If2` | Expression reference (`$('2chat-input-...')`) |
| 6 | `Code in JavaScript2` | Expression reference |
| 7 | `AI Agent` | **System prompt** (project name, FAQ, bridging context) |
| 8 | `property_knowledge` (RAG tool) | Pinecone index name |
| 9 | `save_leads_data` (tool) | `workflowId` → WF3 baru |
| 10 | `Window Buffer Memory` | Session key salt (optional) |
| 11 | `Gupshup SMS Request` | Gupshup credentials |
| 12 | `Update rows in a table` (3 nodes) | PostgreSQL table |
| 13 | `Execute a SQL query` (5+ nodes) | PostgreSQL SQL (table name + counter_id) |
| 14 | `Basic LLM Chain` (classifier) | System prompt (project name) |
| 15 | `AI Agent1` (spam classifier) | System prompt (project name, optional) |
| 16 | `Basic LLM Chain3` (remarks) | System prompt (project-agnostic, OK) |
| 17 | `Update interest` / `Update chat_history` / `Update data to agent` | Sheet ID (4+ nodes) |
| 18 | `Pinecone Vector Store` (3 nodes) | Index name + embeddings |

**Total WF2**: ~30+ nodes need project-specific updates

#### WF3: Tool Simpan Visit

| # | Node Name | Yang Diubah |
|---|-----------|------------|
| 1 | `Append row in sheet` | Sheet ID (chatbot_data log) |
| 2 | `get summary1` / `get summary2` | Sheet ID (lookup existing) |
| 3 | `Append or update row in sheet` | Sheet ID (AI_Call_data upsert) |
| 4 | `Append or update row in sheet1` | Sheet ID (AI_Call_data upsert) |

**Total WF3**: 5 nodes need Sheet ID update

---

### 22.4 Step-by-Step Setup Protocol untuk Project Baru

```
┌─────────────────────────────────────────────────────────────────┐
│              NEW PROJECT SETUP PROTOCOL                          │
│              Estimasi waktu: 2-4 jam per project                │
└─────────────────────────────────────────────────────────────────┘

FASE 1: PERSIAPAN DATA (30-60 menit)
═══════════════════════════════════════
□ 1.1  Kumpulkan info project:
       - Nama project, lokasi, harga mulai
       - Daftar sales team
       - WA Group developer (group ID)
       - Knowledge base dokumen properti

□ 1.2  Buat Retell AI Agent baru di dashboard Retell:
       - Upload prompt baru sesuai project
       - Catat agent_id yang baru
       - Assign from_number

□ 1.3  Siapkan Google Sheet baru:
       - Duplikasi template sheet dengan format kolom sama
       - Sheet 1: AI_Call_data (gid=0) — kolom identik
       - Sheet 2: chatbot_data — kolom identik
       - Sheet 3: fix data for group wa — + daftar Sales baru
       - Setup VLOOKUP formula di kolom Sales & SPV
       - Catat spreadsheet_id baru

□ 1.4  Siapkan PostgreSQL:
       - CREATE TABLE public.{project} (sama schema dengan anandaya)
       - INSERT INTO counter (id, total_sent) VALUES ('counter_{project}', 0)

□ 1.5  Siapkan Pinecone:
       - Buat index baru: {project}-update-1
       - Upload knowledge base dokumen ke Google Drive
       - Jalankan workflow upsert embeddings

□ 1.6  Setup Gupshup:
       - Konfigurasi webhook forward untuk nomor WA project
       - Catat credentials (jika beda account)


FASE 2: DUPLIKASI WORKFLOW (30-60 menit)
═══════════════════════════════════════
□ 2.1  Duplikasi WF1 (Anandaya Project copy)
       → Rename: "{Project} Project"

□ 2.2  Duplikasi WF2 (Chatbot Anandaya copy)
       → Rename: "Chatbot {Project}"

□ 2.3  Duplikasi WF3 (Tool - Simpan Visit-Anandaya)
       → Rename: "Tool - Simpan Visit-{Project}"
       → Catat workflow ID baru


FASE 3: UPDATE WF3 — TOOL SIMPAN VISIT (10 menit)
═══════════════════════════════════════
□ 3.1  Update 5 Google Sheets nodes → spreadsheet_id baru
□ 3.2  Test dengan manual trigger


FASE 4: UPDATE WF2 — CHATBOT (30-60 menit)
═══════════════════════════════════════
□ 4.1  Webhook: Rename + update path
       Node: 2chat-input-{project}

□ 4.2  Update SEMUA $('2chat-input-...') references
       Nodes: Code in JavaScript3, If2, Code in JavaScript2
       (Search & replace di expression editor)

□ 4.3  AI Agent system prompt:
       - Ganti semua "Anandaya" → {project_name}
       - Ganti "Lina AI" → {bot_name}
       - Ganti harga, lokasi, FAQ answers
       - Update bridging context
       - Update FAQ overrides

□ 4.4  AI Agent → save_leads_data tool:
       - Update workflowId → WF3 baru

□ 4.5  property_knowledge (RAG tool):
       - Update Pinecone index name

□ 4.6  Basic LLM Chain (lead classifier):
       - Update project name di prompt

□ 4.7  Gupshup SMS Request:
       - Update credentials jika beda
       - Update source number

□ 4.8  Update semua Google Sheets nodes (7+) → spreadsheet_id baru

□ 4.9  Update semua PostgreSQL nodes (11+):
       - Table name: anandaya → {project}
       - Counter query: counter_anandaya → counter_{project}

□ 4.10 Window Buffer Memory:
       - Update session key salt (optional, beda per project)


FASE 5: UPDATE WF1 — MAIN WORKFLOW (30-60 menit)
═══════════════════════════════════════
□ 5.1  Get Data section:
       - HTTP Request20: Update CRM filter (mx_Custom_1 = {project})
       - PostgreSQL nodes: Update table name (2 nodes)

□ 5.2  Attempt 1 section:
       - AI CALL(RETELL): Update from_number + agent_id
       - get csv data1: Update spreadsheet_id di URL
       - update attempt 1: Update Sheet ID

□ 5.3  Webhook section:
       - Webhook: Buat path baru (atau gunakan auto-generated)
       - update data from retell webhook: Update Sheet ID
       - ⚠️ Update Retell Dashboard: set webhook URL ke path baru

□ 5.4  Attempt 2 section:
       - get csv data: Update spreadsheet_id
       - 3 Gupshup nodes: Update credentials + bridging message
       - update bot conversation: Update PostgreSQL SQL (table name)
       - update counter: Update SQL (counter_id)
       - Sheet update nodes: Update Sheet ID (3+ nodes)

□ 5.5  Attempt 3&4 section:
       - AI CALL(RETELL)1: Update from_number + agent_id

□ 5.6  Attempt 5 section:
       - Gupshup node: Update credentials + message
       - Sheet update nodes: Update Sheet ID

□ 5.7  CRM Update section:
       - Google Sheets Trigger: Update documentId
       - 7 HTTP Request nodes: CRM API URL (biasanya sama jika satu account)
       - 15+ Sheet nodes: Update Sheet ID
       - WA Group message templates: Update project name (5 nodes)

□ 5.8  Valid Number Check:
       - 4 Check nodes: Update 2Chat credentials (jika beda account)

□ 5.9  WA Group section:
       - get csv data2: Update spreadsheet_id
       - 5 Send to group nodes: Update group UUID + message template
       - internal alert: Update Sheet ID


FASE 6: TESTING (30 menit)
═══════════════════════════════════════
□ 6.1  Test WF0/Get Data: Manual trigger → cek Sheet + DB terisi
□ 6.2  Test WF1-A: Manual trigger → cek Retell call keluar
□ 6.3  Test WF1-B: Simulasi webhook → cek Sheet terupdate
□ 6.4  Test WF2: Kirim WA → cek chatbot reply + DB log
□ 6.5  Test WF3: Via chatbot mention jadwal → cek Sheet terupdate
□ 6.6  Test CRM Update: Edit Sheet → cek LeadSquared terupdate
□ 6.7  Test WA Group: Pastikan pesan masuk ke group yang benar


FASE 7: GO LIVE
═══════════════════════════════════════
□ 7.1  Aktifkan semua Schedule Trigger
□ 7.2  Aktifkan Webhook nodes
□ 7.3  Monitor 24 jam pertama di n8n execution log
□ 7.4  Verifikasi leads masuk end-to-end
```

---

### 22.5 Config yang BERBEDA vs SAMA per Project

#### Config yang BERBEDA per Project

| # | Parameter | Contoh (Anandaya) | Dimana Dipakai | Jumlah Node |
|---|-----------|-------------------|----------------|-------------|
| 1 | `PROJECT_NAME` | "Anandaya" | WA messages, prompts, templates | ~20 nodes |
| 2 | `CRM_FILTER_VALUE` | mx_Custom_1 = "Anandaya" | CRM ingestion query | 1 node |
| 3 | `AI_CALL_AGENT_ID` | agent_ba5eee... / agent_89c6e8... | Retell API body | 2 nodes |
| 4 | `AI_CALL_FROM_NUMBER` | 02184283770 / 622130071990 | Retell API body | 2 nodes |
| 5 | `SPREADSHEET_ID` | 13miTl... / 1m8ufZ... | All Sheet nodes | ~38 nodes |
| 6 | `DB_TABLE_NAME` | public.anandaya | All Postgres nodes | ~16 nodes |
| 7 | `COUNTER_ID` | 'counter_anandaya' | Counter SQL queries | 2-3 nodes |
| 8 | `WA_GROUP_ID` | UUID | 2Chat sendToGroup | 5 nodes |
| 9 | `CHATBOT_SYSTEM_PROMPT` | Full prompt (3000+ chars) | AI Agent node | 1 node |
| 10 | `LEAD_CLASSIFIER_PROMPT` | "...khusus Proyek Anandaya..." | Basic LLM Chain | 1 node |
| 11 | `PINECONE_INDEX` | "anandaya-update-2" | Vector store nodes | 3 nodes |
| 12 | `WA_BRIDGING_MESSAGE` | "Halo Kak, Saya Lina AI..." | Gupshup API nodes | 3 nodes |
| 13 | `WA_GROUP_TEMPLATES` | "New Leads ANANDAYA..." | 2Chat send nodes | 5 nodes |
| 14 | `WEBHOOK_PATH_RETELL` | UUID | Webhook trigger WF1 | 1 node |
| 15 | `WEBHOOK_PATH_CHATBOT` | UUID | Webhook trigger WF2 | 1 node |
| 16 | `WF3_WORKFLOW_ID` | ffZtHJBr31a8NZXn | save_leads_data tool | 1 node |
| 17 | `SALES_LIST` | [nama1, nama2, ...] | Spreadsheet formula | Sheet formula |
| 18 | `GUPSHUP_CREDENTIALS` | userid, password | WA send nodes | 3-4 nodes |
| 19 | `TWOCHAT_WA_NUMBER` | +6285371025371 | 2Chat Trigger | 1 node |
| 20 | `FAQ_OVERRIDES` | {ready, legality, pricelist, access} | Chatbot system prompt | 1 node |
| 21 | `STARTING_PRICE` | "400 jutaan" | Bridging msg + prompt | 4 nodes |
| 22 | `LOCATION` | "Parung Panjang, Serpong Selatan" | Messages + prompt | 4 nodes |
| 23 | `KNOWLEDGE_DATA` | Dokumen properti | Google Drive + Pinecone | External |

**Total: ~85+ node instances** yang perlu diubah per project baru (di 4 workflow).

#### Config yang SAMA untuk semua Project (Logic Core)

| Aspek | Keterangan | Apakah Perlu Diubah? |
|-------|-----------|---------------------|
| Attempt flow (1→2→3→4→5) | Identical logic | ❌ Tidak |
| Guard flag pattern | Same fields, same checks | ❌ Tidak |
| CRM field mapping (mx_Custom_25,26,54,etc) | Same schema names | ❌ Tidak |
| 3-hour gap rule (attempt 3/4) | Same timing | ❌ Tidak |
| Bridging → chatbot → classify pipeline | Same flow | ❌ Tidak |
| Valid number check logic | Same 2Chat API | ❌ Tidak |
| Spam classifier threshold (≥5 messages) | Same | ❌ Tidak |
| Remarks generator timing (5hr idle) | Same | ❌ Tidak |
| Summary append pattern | Same format | ❌ Tidak |
| Name-empty blocking | Same business rule | ❌ Tidak |
| Dedup via Compare Datasets | Same mechanism | ❌ Tidak |
| Switch routing logic (interest/interest2/customer_type) | Same branches | ❌ Tidak |
| Filter guard conditions (9 conditions attempt 3/4) | Same logic | ❌ Tidak |
| CRM decision matrix (contact_status, contact_result) | Same mapping | ❌ Tidak |
| Excel serial → datetime conversion | Same code | ❌ Tidak |
| Phone normalization (0→62, strip +) | Same logic | ❌ Tidak |
| Calendar context generation (21/28 days) | Same code | ❌ Tidak |
| Rate limiting (Wait 20s, Wait 2s) | Same timing | ❌ Tidak |

---

### 22.6 Recommended Project Config Schema

Jika nanti migrasi ke custom platform (bukan n8n), semua config per project bisa disimpan di database sebagai 1 JSON:

```json
{
  "project_id": "anandaya",
  "project_name": "Anandaya",
  "company_name": "Rumah123",
  "bot_name": "Lina AI",
  "starting_price": "400 jutaan",
  "location": "Parung Panjang, Serpong Selatan",

  "crm": {
    "event_code": 12002,
    "filter_field": "mx_Custom_1",
    "filter_value": "Anandaya",
    "access_key": "...",
    "secret_key": "..."
  },

  "ai_call": {
    "agent_id_attempt1": "agent_ba5eee908dfde96549c4372229",
    "agent_id_attempt3_4": "agent_89c6e8e3b6d7ebcd52a50bc023",
    "from_number_attempt1": "02184283770",
    "from_number_attempt3_4": "622130071990",
    "api_key": "key_6ac9bfd5de6da98d3e5777654461",
    "wait_between_calls_seconds": 20,
    "calendar_days": 21
  },

  "whatsapp": {
    "gupshup_userid": "2000258121",
    "gupshup_password": "FptIsos0y",
    "twochat_wa_number": "+6285371025371",
    "wa_group_developer_id": "uuid-developer-group",
    "wa_group_internal_id": "uuid-internal-group"
  },

  "chatbot": {
    "system_prompt": "Kamu adalah {bot_name} — Asisten Virtual...",
    "classifier_prompt": "...khusus Proyek {project_name}...",
    "pinecone_index": "anandaya-update-2",
    "llm_model": "gpt-4o-mini",
    "temperature": 0.3,
    "memory_window": 4,
    "calendar_days": 28,
    "spam_threshold": 5,
    "remarks_idle_hours": 5,
    "session_salt": "alex12345678910"
  },

  "data_store": {
    "spreadsheet_id_main": "13miTlnFRQxeMoEkbBPAobMB1DIGMRINefnhNy2va9Ew",
    "spreadsheet_id_csv": "1m8ufZ_9jp6kCsMkLmf7dkNcSZQijZdWWchYm9Gu2jvU",
    "db_table": "public.anandaya",
    "counter_id": "counter_anandaya"
  },

  "messages": {
    "bridging": "Halo Kak, Saya {bot_name} {company_name} mewakili perumahan {project_name} harga mulai {starting_price} di {location}.\n\nTerimakasih, Kakak baru saja mengklik iklan kami, Apakah kakak tertarik dengan perumahan {project_name} dan mau diinfokan lebih lanjut?",
    "wa_group_hot": "New Leads {PROJECT_NAME}\n{name} {phone}\nMinat visit tanggal/jam: {svs_date}\nTanggal Inquiry: {date}\nSumber: {LEAD_SOURCE}\nSales: {Sales}",
    "wa_group_warm": "New Leads {PROJECT_NAME}\n{name} {phone}\nNotes: Tertarik diinfokan PL, brochure, dan promo\nTanggal Inquiry: {date}\nSumber: {LEAD_SOURCE}\nSales: {Sales}",
    "wa_group_default": "New Leads {PROJECT_NAME}\n{name} {phone}\nTanggal Inquiry: {date}\nSumber: {LEAD_SOURCE}",
    "internal_alert": "Projek {PROJECT_NAME}\nNo tlf: +{phone}\nNamanya masih kosong, tolong bantu diisi ya."
  },

  "sales_team": ["Sales A", "Sales B", "Sales C"],

  "faq_overrides": {
    "ready_status": "Indent 6-12 bulan",
    "legality": "SHM (Bisa AJB dan balik nama pecah sertifikat)",
    "pricelist": "Akan diberikan oleh sales inhouse {project_name}",
    "video_sales": "Nanti akan diberikan oleh sales inhouse {project_name} setelah chat ini ya.",
    "access_info": "Ke Stasiun Parung Panjang sekitar 5-10 menit...",
    "installment_info": "Estimasi cicilan mulai 2 jutaan"
  },

  "n8n_workflow_ids": {
    "wf1_main": "...",
    "wf2_chatbot": "...",
    "wf3_tool_visit": "ffZtHJBr31a8NZXn",
    "retell_webhook_path": "a493c213-4e39-494a-a38a-dd1d9bef0155",
    "chatbot_webhook_path": "769b7e00-c257-49a7-af87-5a52c31bfe94"
  }
}
```

---

> **Catatan**: Dokumen ini menjelaskan LOGIC MURNI — tanpa hardcode ke project tertentu.
> Semua nilai yang project-specific ditandai dengan `{PARAMETER_NAME}` dan bisa diparameterisasi melalui config per project.
> Logic core (attempt flow, guard flags, decision trees) **identik** untuk semua project.
> 
> **Total effort per project baru (di n8n)**: ~2-4 jam setup, ~85+ node updates across 4 workflows.
> **Rekomendasi jangka panjang**: Migrasi ke custom platform dimana config per project disimpan di database — eliminasi kebutuhan duplikasi workflow manual.