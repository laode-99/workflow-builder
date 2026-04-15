# Anandaya AI Call System — Complete Technical Documentation

> **Terakhir diperbarui**: 10 April 2026
> **Tujuan dokumen**: Dokumentasi lengkap sistem AI Call + Chatbot otomatis untuk project perumahan Anandaya. Dokumen ini adalah **source of truth** — ditujukan untuk developer, AI, atau siapa pun yang perlu memahami, maintain, atau mengembangkan sistem ini.
> **Business context**: Perusahaan kami menyaring raw leads dari portal properti (Rumah123, dll) menggunakan AI sebelum menyerahkan leads yang valid/qualified ke developer. Anandaya adalah project pertama yang menggunakan sistem ini dan sudah **berjalan di production daily** dengan hasil bisnis yang positif. Rencana ke depan: **scale ke project developer lainnya**.

---

## Daftar Isi

1. [Overview Arsitektur](#1-overview-arsitektur)
2. [Inventory Workflow & File](#2-inventory-workflow--file)
3. [Data Model](#3-data-model)
4. [Fase 0: Get Data — Ingestion dari LeadSquared](#4-fase-0-get-data--ingestion-dari-leadsquared)
5. [Fase 1: Attempt 1 — AI Call via Retell](#5-fase-1-attempt-1--ai-call-via-retell)
6. [Webhook Retell — Hasil Call Masuk](#6-webhook-retell--hasil-call-masuk)
7. [Fase 2: Attempt 2 — WA Chatbot Trigger + Bridging Message](#7-fase-2-attempt-2--wa-chatbot-trigger--bridging-message)
8. [Fase 3: Chatbot Anandaya — Lina AI](#8-fase-3-chatbot-anandaya--lina-ai)
9. [Tool — Simpan Visit Anandaya (Sub-workflow)](#9-tool--simpan-visit-anandaya-sub-workflow)
10. [Fase 4-5: Attempt 3 & 4 — AI Call Retry](#10-fase-4-5-attempt-3--4--ai-call-retry)
11. [Fase 6: Attempt 5 — WA Chatbot Final](#11-fase-6-attempt-5--wa-chatbot-final)
12. [Valid Number Check — 2Chat WhatsApp Verification](#12-valid-number-check--2chat-whatsapp-verification)
13. [Update Data ke LeadSquared CRM](#13-update-data-ke-leadsquared-crm)
14. [Send Valid Leads ke WA Group Developer](#14-send-valid-leads-ke-wa-group-developer)
15. [Alur Attempt Keseluruhan (End-to-End)](#15-alur-attempt-keseluruhan-end-to-end)
16. [Guard Flags & Anti-Duplikasi](#16-guard-flags--anti-duplikasi)
17. [Sales Assignment & Round Robin](#17-sales-assignment--round-robin)
18. [Counter & Remarks Tracking](#18-counter--remarks-tracking)
19. [Credentials & External Services](#19-credentials--external-services)
20. [Known Issues & Gotchas](#20-known-issues--gotchas)
21. [Panduan Monitoring & Debugging](#21-panduan-monitoring--debugging)
22. [Knowledge Base (Pinecone & RAG)](#22-knowledge-base-pinecone--rag)
23. [Strategi Refactoring & Scaling Multi-Project](#23-strategi-refactoring--scaling-multi-project)
24. [Glossary](#24-glossary)

---

## 1. Overview Arsitektur

Sistem ini adalah **mesin lead nurturing otomatis** untuk project perumahan developer **Anandaya** (Cluster Nala, Serpong Selatan / Parung Panjang). Sistem mengelola siklus hidup lead dari masuk hingga dikirim ke tim developer melalui kombinasi AI Phone Call dan WhatsApp Chatbot.

### Alur Besar

```
LeadSquared CRM (sumber lead, event code 12002, project "Anandaya")
        │
        ▼ [tiap 1-2 menit, pull via API]
┌────────────────────────────────────────────────────────────────────┐
│  CENTRAL DATA STORES                                              │
│  ┌──────────────────────────────────┐  ┌────────────────────────┐ │
│  │ Google Sheet: AI_Call_data       │  │ PostgreSQL (Supabase): │ │
│  │ [Anandaya] Developer Call        │  │ public.anandaya        │ │
│  │ Placeholder                      │  │ (chatbot state)        │ │
│  │ + chatbot_data (log)             │  │                        │ │
│  │ + fix data for group wa (log)    │  │ + counter table        │ │
│  └───────────────┬──────────────────┘  └───────────┬────────────┘ │
└──────────────────┼─────────────────────────────────┼──────────────┘
                   │                                 │
        ┌──────────┴──────────┐            ┌────────┴────────┐
        ▼                     ▼            ▼                 ▼
  ┌──────────┐    ┌────────────────┐  ┌──────────┐  ┌──────────────┐
  │ AI CALL  │    │ WA CHATBOT     │  │ Chatbot  │  │ Tool Simpan  │
  │ (Retell) │    │ Trigger +      │  │ "Lina AI"│  │ Visit        │
  │ Att 1,3,4│    │ Bridging Msg   │  │ (GPT-4o) │  │ (sub-wf)     │
  │          │    │ (Gupshup)      │  │          │  │              │
  └────┬─────┘    │ Att 2, 5       │  └────┬─────┘  └──────────────┘
       │          └───────┬────────┘       │
       │                  │                │
       ▼                  ▼                ▼
  ┌──────────────────────────────────────────────┐
  │  Hasil: interest, customer_type, summary      │
  │  → Update Google Sheet                        │
  └───────────────────┬──────────────────────────┘
                      │
            ┌─────────┴──────────┐
            ▼                    ▼
  ┌──────────────────┐  ┌──────────────────────────┐
  │ Valid Number     │  │ Update LeadSquared CRM   │
  │ Check (2Chat)   │  │ (API)                    │
  └────────┬─────────┘  └────────────┬─────────────┘
           │                         │
           ▼                         ▼
  ┌──────────────────────────────────────────────┐
  │ Kirim ke WA Group Developer (2Chat)          │
  │ + Internal alert jika nama kosong            │
  └──────────────────────────────────────────────┘
```

### Platform & Integrasi

| Platform | Fungsi dalam Sistem |
|----------|-------------------|
| **N8N** | Workflow automation engine (self-hosted di Tencent Cloud), dimonitor oleh team engineer |
| **LeadSquared** | CRM — sumber lead awal & tujuan update akhir. Event code `12002` = project developer (termasuk Anandaya) |
| **Retell AI** | AI Phone Call — melakukan panggilan telepon otomatis ke lead |
| **Google Sheets** | Central data store — staging area utama antar semua workflow |
| **PostgreSQL (Supabase)** | Data store khusus chatbot (chat history, spam tracking, conversation log, counter) |
| **Gupshup** | WhatsApp — **mengirim** pesan ke lead individual (reply chatbot + bridging message pertama). Dipilih karena resmi partner Meta sehingga tidak mudah kena ban |
| **2Chat** | WhatsApp — **menerima** pesan masuk dari lead (webhook forward ke n8n) + kirim notifikasi ke WA Group developer + valid number check |
| **OpenAI (GPT-4o-mini)** | LLM untuk chatbot, lead classifier, spam classifier, remarks generator |
| **Pinecone** | Vector store untuk RAG knowledge base properti Anandaya |
| **Google Drive** | Tempat file knowledge base (dokumen properti Anandaya) |

### Kenapa 2 Platform WhatsApp?

- **Gupshup**: Digunakan untuk **mengirim** pesan ke lead (bridging message + reply chatbot). Alasan: Gupshup adalah partner resmi Meta sehingga tidak mudah kena ban WhatsApp.
- **2Chat**: Digunakan untuk (a) **menerima** pesan WA dari lead via webhook (forward ke n8n), (b) **mengirim** notifikasi ke WA Group developer internal, (c) **check valid number** (apakah nomor terdaftar di WhatsApp).
- Webhook di workflow Chatbot diberi nama `2chat-input-anandaya` tapi sebenarnya menerima forward dari **Gupshup** juga. Penamaan ini legacy dari setup awal.

---

## 2. Inventory Workflow & File

Sistem terdiri dari **4 workflow N8N** yang saling terhubung:

| # | File JSON | Nama di N8N | ID Workflow | Fungsi |
|---|-----------|-------------|------------|--------|
| 0 | `get data anandaya.json` | `get data` | `oDlNkggxCDMS11V4` | Ingestion: Pull lead baru dari LeadSquared → GSheet + PostgreSQL |
| 1 | `Anandaya Project copy.json` | `Anandaya Project copy` | *(dalam file)* | Workflow utama: AI Call, Webhook Retell, WA Trigger, Bridging Message, CRM Update, Valid Number Check, WA Group |
| 2 | `Chatbot Anandaya copy.json` | `Chatbot Anandaya copy` | `eXmeTmHZUKOsVLip` | Chatbot WA "Lina AI": terima pesan, reply via Gupshup, classify lead |
| 3 | `Tool - Simpan Visit-Anandaya.json` | `Tool - Simpan Visit-Anandaya` | `ffZtHJBr31a8NZXn` | Sub-workflow: dipanggil oleh Chatbot AI Agent untuk simpan data visit |

### Hubungan Antar Workflow

```
[WF0: Get Data] ──write──► [Google Sheet + PostgreSQL]
                                    │
                         ┌──────────┴──────────┐
                         ▼                      ▼
                [WF1: Anandaya Project]  [WF2: Chatbot Anandaya]
                    │                        │
                    ├─ Retell AI Call         ├─ Gupshup webhook (receive)
                    ├─ Retell Webhook         ├─ Gupshup API (send reply)
                    ├─ Gupshup Bridging Msg   ├─ OpenAI + Pinecone
                    ├─ 2Chat Valid Check      ├──call──► [WF3: Tool Simpan Visit]
                    ├─ LeadSquared API update  │                │
                    ├─ 2Chat WA Group send     │           write ▼
                    └─ Google Sheet r/w        └──────► [Google Sheet]
```

**CATATAN PENTING**: Di production, section "Get Data (NEW)" sudah **terintegrasi di dalam WF1** (Anandaya Project). File `get data anandaya.json` terpisah adalah copy dari logic yang sama — disediakan sebagai referensi karena di JSON export WF1 section tersebut mungkin tidak lengkap.

---

## 3. Data Model

### 3.1 Google Sheet: `[Anandaya] Developer Call Placeholder`

**Spreadsheet ID**: `13miTlnFRQxeMoEkbBPAobMB1DIGMRINefnhNy2va9Ew`

#### Sheet 1: `AI_Call_data` (gid=0) — MASTER RECORD

Ini adalah **source of truth** untuk semua workflow. Setiap row = 1 lead.

| Kolom | Tipe | Diisi Oleh | Deskripsi |
|-------|------|-----------|-----------|
| `id` | string | WF0 Get Data | ID OpportunityId dari LeadSquared |
| `phone` | string | WF0 Get Data | Nomor telepon (format: 62xxx, tanpa +) |
| `name` | string | WF0 Get Data / Manual | Nama lead. **Bisnis penting**: jika kosong, lead TIDAK boleh dikirim ke grup developer — harus dicari tahu dulu oleh tim internal |
| `call_date` | string | WF1 Webhook Retell | Tanggal/waktu terakhir call (format: dd-MMM-yy HH:mm WIB) |
| `disconnected_reason` | string | WF1 Webhook Retell | Alasan call terputus: `user_hangup`, `agent_hangup`, `Invalid_destination`, `ivr_reached`, `voicemail_reached`, `dial_no_answer`, `dial_busy` |
| `interest` | string | WF1 Webhook Retell / WF3 Tool | Level ketertarikan dari AI Call atau Chatbot. Lihat tabel di bawah |
| `interest2` | string | WF2 Chatbot Classifier | Klasifikasi chatbot: `Callback`, `Tidak Tertarik`, `Agent`, `Tertarik`, `Mau Diinformasikan` |
| `customer_type` | string | WF1 Webhook / WF2 Chatbot | Tipe: `Interest`, `Callback`, `Unqualified`, `Spam`, `Agent`, `Double Number`, `Inactivity`, `Voicemail` |
| `attempt` | number/string | WF1 | Counter attempt saat ini (1-5) |
| `svs_date` | string | WF1 Webhook / WF3 Tool | Tanggal site visit (format: M/D/YYYY H:mm:ss AM/PM) |
| `summary` | string | WF1 Webhook / WF3 Tool | Summary percakapan. **Append-only** — setiap update ditambahkan dengan timestamp, tidak di-overwrite |
| `whatsapp` | string | WF1 Attempt 2/5 | Timestamp kapan WA bridging message pertama dikirim |
| `whatsapp_reply` | string | WF2 Chatbot | **Guard flag**: `"1"` = lead sudah reply WA → mencegah re-call |
| `sent_to_dev` | string | WF1 CRM Update | `"Yes"` setelah berhasil push ke LeadSquared |
| `sent_to_wa_group` | string | WF1 WA Group | Timestamp kirim ke WA group developer |
| `leadsquared_timestamp` | string | WF1 CRM Update | **Guard flag**: timestamp push ke LeadSquared → mencegah double push |
| `lead_chat_history` | string | WF2 Chatbot | Kumpulan pesan user di WA |
| `Chatbot_Remarks` | string | WF2 Remarks Generator | Ringkasan percakapan 3 kalimat gaya sales analyst. **Bisnis penting**: ini diupdate ke LeadSquared agar telesales/developer selalu punya context percakapan terkini |
| `Last_Leads_Chat` | string | WF2 Chatbot | Timestamp chat terakhir |
| `Valid_Number` | string | WF1 2Chat Check | `"Yes"` = nomor terdaftar di WhatsApp, `"No"` = tidak terdaftar (ditandai spam) |
| `internal_alert` | string | WF1 WA Group | `"1"` = sudah kirim alert "nama kosong" ke grup internal |
| `Sales` | string | **Spreadsheet Formula** | Nama sales yang di-assign. Diisi otomatis oleh rumus VLOOKUP round-robin dari sheet `fix data for group wa` |
| `SPV` | string | **Spreadsheet Formula** | Nama supervisor. Sama mechanism dengan Sales |

#### Nilai-nilai `interest`

| Nilai | Kategori | Sumber |
|-------|----------|--------|
| `Tertarik Site Visit (memberikan jadwal) (Hot Leads)` | Hot | AI Call / Chatbot |
| `tertarik di informasikan dulu (Warm Leads)` | Warm | AI Call |
| `Tertarik untuk dihubungi (Warm Leads)` | Warm | AI Call / Chatbot |
| `tidak mau atau tidak tertarik (Cold Leads)` | Cold | AI Call / Chatbot |
| `tidak ada percakapan yang cukup` | Insufficient | AI Call (percakapan terlalu singkat) |

#### Sheet 2: `chatbot_data` (gid=267212040) — RAW LOG

Append-only log setiap kali chatbot AI Agent memanggil tool `save_leads_data`.

| Kolom | Deskripsi |
|-------|-----------|
| `nama`, `hp`, `email` | Data lead |
| `tanggal`, `bulan`, `tahun`, `jam` | Jadwal visit (dari AI) |
| `summary`, `interest`, `interest2` | Hasil klasifikasi |

#### Sheet 3: `fix data for group wa` (gid=1390135134)

- Log ID leads yang sudah diproses untuk WA group
- **Berisi daftar nama Sales** di kolom 15 — digunakan oleh rumus VLOOKUP di sheet `AI_Call_data` untuk round-robin assignment

### 3.2 PostgreSQL (Supabase): `public.anandaya`

**Host**: Supabase (bukan Tencent Cloud)
**Credential di n8n**: `Alex` (ID: `dykHiiKfuGabdrmA`)

| Kolom | Tipe | Deskripsi |
|-------|------|-----------|
| `id` | string (PK) | Sama dengan OpportunityId dari LeadSquared |
| `phone` | string | Nomor telepon (62xxx) |
| `name` | string | Nama lead |
| `chat_history` | string | Kumpulan pesan user — comma-separated, append setiap pesan baru |
| `chat_total` | number | Counter jumlah pesan user |
| `spam` | string | `"1"` = lead spam → diblokir dari chatbot |
| `conversation` | string | Full log "User: xxx\n\nBot: yyy" — digunakan untuk generate remarks |
| `last_chat` | datetime | Timestamp pesan terakhir |
| `reset_remarks` | number | `1` = sudah diringkas, `NULL` = belum/perlu ringkas ulang |

### 3.3 PostgreSQL (Supabase): `counter`

| Kolom | Tipe | Deskripsi |
|-------|------|-----------|
| `id` | string (PK) | `'counter_anandaya'` |
| `total_sent` | number | Counter total pesan WA yang terkirim — untuk tracking volume |

**Kenapa ada PostgreSQL selain Sheet?** Google Sheets terkadang lag dan out-of-sync saat diakses via API secara intensif. PostgreSQL (Supabase) digunakan sebagai data store yang lebih reliable untuk chatbot yang memerlukan read/write cepat dan konsisten.

---

## 4. Fase 0: Get Data — Ingestion dari LeadSquared

**File**: `get data anandaya.json` (juga embedded di WF1 production)
**Trigger**: Schedule — tiap 1-2 menit (production: `tiap 2 menit jam 7 pagi sampai 8 malam setiap hari`)
**Status**: Active di production (JSON export menunjukkan `active: false` karena artifact export)

### Flow

```
Cron Trigger (tiap 1-2 menit, 07:00-20:00 WIB)
  │
  ├──► [JALUR A: Tarik data BARU dari LeadSquared]
  │    │
  │    ▼
  │    HTTP Request20
  │    POST .../OpportunityManagement.svc/Retrieve/BySearchParameter
  │    Filter:
  │      - ActivityEvent = 12002 (event code project developer)
  │      - CreatedOn = hari ini
  │      - mx_Custom_1 = "Anandaya" (filter project spesifik)
  │    Paging: PageIndex 1, PageSize 100
  │    │
  │    ▼
  │    Code: Extract OpportunityId, RelatedProspectId
  │    │
  │    ▼
  │    HTTP Request21
  │    GET .../LeadManagement.svc/Leads.GetById?id={RelatedProspectId}
  │    → Ambil Phone, FirstName
  │    │
  │    ▼
  │    Edit Fields:
  │      id = OpportunityId
  │      phone = Phone.replace(/\D/g, '').replace(/^0/, '62')
  │      name = FirstName
  │
  ├──► [JALUR B: Ambil data EXISTING dari PostgreSQL]
  │    SELECT * FROM public.anandaya → Edit Fields4 (id, phone, name)
  │
  └──► Compare Datasets (by field 'id')
       Output 1: Lead BARU (hanya di LeadSquared)
       Output 3: Lead BERUBAH (ada di keduanya)
       │
       ▼ Merge1 (gabung baru + berubah)
       │
       ▼ Loop Over Items5 (1-per-1)
       │
       ▼ if name empty?
       ├── Kosong → appendOrUpdate Sheet: id + phone saja (JANGAN overwrite nama manual)
       └── Ada → appendOrUpdate Sheet: id + phone + name
            │
            ▼ UPSERT PostgreSQL: id, phone, name
            │
            ▼ [loop next]
```

### Logic Notes

1. **Dedup**: Compare Datasets memastikan hanya lead baru/berubah yang diproses
2. **Dual write**: Setiap lead → Google Sheet + PostgreSQL (agar chatbot juga punya data)
3. **Proteksi nama**: Jika LeadSquared tidak punya FirstName, nama TIDAK ditulis ke sheet — mencegah overwrite nama yang sudah diisi manual oleh tim
4. **Phone normalisasi**: Strip non-digit, `0` → `62` (kode Indonesia)
5. **Paging limit**: Max 100 per run — lead >100/hari bisa terlewat (volume saat ini 10-30 lead/hari per project, jadi cukup)
6. **Event code 12002**: Ini identifier di LeadSquared untuk semua project developer (bukan hanya Anandaya). Filter `mx_Custom_1 = "Anandaya"` yang membedakan per project

---

## 5. Fase 1: Attempt 1 — AI Call via Retell

**File**: `Anandaya Project copy.json`
**Trigger Production**: Cron (di n8n live). Ada juga Manual Trigger untuk testing.
**Tujuan**: Telepon lead baru menggunakan AI voice call

### Retell Configuration

| Parameter | Nilai |
|-----------|-------|
| **API Endpoint** | `https://api.retellai.com/v2/create-phone-call` |
| **API Key** | `key_6ac9bfd5de6da98d3e5777654461` |
| **Agent ID** | `agent_89c6e8e3b6d7ebcd52a50bc023` |
| **From Number** | `+622130071990` |
| **Batch Size** | 1 lead per batch, Wait 20 detik antar call (cukup untuk volume 10-30 leads/hari) |

**Retell AI Agent prompt**: Disimpan di dashboard Retell (BUKAN di n8n). Intinya: AI memberitahu informasi dan keunggulan project Anandaya ke calon buyer untuk menyaring mereka sebelum diberikan ke developer.

### Dynamic Variables ke Retell

| Variable | Sumber | Deskripsi |
|----------|--------|-----------|
| `first_name` | Sheet `name` | AI agent menyapa dengan nama ini |
| `leads_id` | Sheet `id` | Dikirim balik via webhook untuk matching |
| `attempt` | Computed | Nomor attempt |
| `tanggal_call` | Generated | Format: "10 April 2026, 14:30" |
| `calendar_context` | Generated | Kalender 21 hari ke depan (bahasa Indonesia) |
| `hari_ini` | Generated | Nama hari (Senin/Selasa/etc) |
| `tanggal_hari_ini` | Generated | Tanggal lengkap |

### Flow (Attempt 1 — Lead Baru)

```
Read Sheet CSV → Parse → Filter: attempt KOSONG + phone terisi
→ Set attempt=1 di sheet
→ Loop per lead:
    → Generate calendar (21 hari, bahasa Indonesia)
    → POST Retell API create-phone-call
    → Wait 20 detik
    → [loop next]
```

---

## 6. Webhook Retell — Hasil Call Masuk

**Trigger**: Webhook POST path `df045007-6674-43bc-aa20-9d0904c72c6c`

```
Retell webhook → If event == "call_analyzed" →
  → Parse datetime ke WIB (dd-MMM-yy HH:mm)
  → Clean phone (hapus +)
  → appendOrUpdate Sheet (match by id):
      interest, svs_date, summary, disconnected_reason,
      customer_type, phone, name, attempt, call_date
```

### Data dari Retell `custom_analysis_data`

| Field Retell | → Kolom Sheet | Deskripsi |
|-------------|---------------|-----------|
| `['site visit or no']` | `interest` | Hot/Warm/Cold/Insufficient |
| `['site visit date']` | `svs_date` | Tanggal visit (jika setuju) |
| `call_summary` | `summary` | Ringkasan percakapan |
| `['customer_type']` | `customer_type` | Klasifikasi lead |
| `disconnection_reason` | `disconnected_reason` | Alasan putus |

### Dampak `disconnected_reason` pada Flow

| Nilai | Artinya | Next Step |
|-------|---------|-----------|
| `user_hangup` | Lead tutup telepon (ada interaksi) | → CRM Update |
| `agent_hangup` | AI selesai bicara (ada interaksi) | → CRM Update |
| `dial_no_answer` / `dial_busy` | Tidak diangkat | → Attempt 2 (WA) |
| `voicemail_reached` | Masuk voicemail | → Langsung attempt 5 |
| `ivr_reached` | Masuk IVR/mesin | → Langsung attempt 5 |
| `Invalid_destination` | Nomor tidak valid | → STOP semua |

---

## 7. Fase 2: Attempt 2 — WA Chatbot Trigger + Bridging Message

**File**: `Anandaya Project copy.json`
**Trigger**: Cron tiap 2 menit
**Tujuan**: Kirim WA pertama (bridging message) ke lead yang tidak mengangkat call

### Flow

```
Cron tiap 2 menit → Read Sheet CSV → Parse
│
▼
If6: Filter Attempt 2:
  ✅ whatsapp KOSONG (belum pernah kirim WA)
  ✅ disconnected_reason terisi (sudah pernah call)
  ✅ attempt = 1
  ✅ phone terisi
  ✅ disconnected_reason ≠ Invalid_destination
│
├── TRUE → Cek user/agent hangup?
│   ├── Diangkat + percakapan kurang → Update attempt=2, timestamp whatsapp
│   │   → ★ KIRIM BRIDGING MESSAGE via Gupshup (HTTP Request1) ★
│   │   → Simpan pesan bot ke PostgreSQL conversation
│   │   → Increment counter
│   │
│   ├── Diangkat + percakapan cukup → Update attempt=2, timestamp whatsapp
│   │   → TIDAK kirim WA (sudah cukup interaksi, lanjut ke attempt 3 call)
│   │
│   └── Tidak diangkat → Update attempt=2, timestamp whatsapp
│       → ★ KIRIM BRIDGING MESSAGE via Gupshup (HTTP Request1) ★
│       → Simpan pesan bot ke PostgreSQL conversation
│       → Increment counter
│
├── If15: Voicemail / IVR?
│   ├── TRUE + no reply + attempt < 5:
│   │   → Set attempt=5 (SKIP semua retry!)
│   │   → ★ KIRIM BRIDGING MESSAGE via Gupshup (HTTP Request14) ★
│   │
│   └── FALSE: (lanjut normal)
```

### Bridging Message (Pesan WA Pertama ke Lead)

Dikirim via **Gupshup API** (`mediaapi.smsgupshup.com`). Isi pesan (decoded):

> "Halo Kak, Saya Lina AI Rumah123 mewakili perumahan Anandaya harga mulai 400 jutaan di Parung Panjang, Serpong Selatan.
>
> Terimakasih, Kakak baru saja mengklik iklan kami, Apakah kakak tertarik dengan perumahan Anandaya dan mau diinfokan lebih lanjut?"

**Node-node terkait**:
- `HTTP Request1`: Bridging message untuk attempt 2 (dari If6, path user/agent hangup)
- `HTTP Request13`: Bridging message untuk attempt 5 (dari If7, path user/agent hangup)
- `HTTP Request14`: Bridging message untuk voicemail/IVR case
- `update bot conversation`: Simpan pesan bridging ke PostgreSQL kolom `conversation` (agar chatbot punya konteks awal)

Setelah bridging terkirim, sistem **menunggu lead reply**. Jika lead reply → Gupshup forward ke webhook → WF2 (Chatbot) aktif.

---

## 8. Fase 3: Chatbot Anandaya — Lina AI

**File**: `Chatbot Anandaya copy.json`
**Trigger**: Webhook POST path `769b7e00-c257-49a7-af87-5a52c31bfe94` (dari Gupshup forward)

### Arsitektur AI

| Komponen | Detail |
|----------|--------|
| **LLM** | GPT-4o-mini (temperature 0.3) |
| **Memory** | Window Buffer 4 pesan, session key: `{phone}alex12345678910` |
| **Knowledge Base** | Pinecone index `anandaya-update-2` + OpenAI Embeddings |
| **Tool 1** | `property_knowledge` — RAG untuk jawab pertanyaan properti |
| **Tool 2** | `save_leads_data` → panggil WF3 (simpan visit/interest) |

### Flow Lengkap

```
Gupshup webhook (lead reply WA)
│
├──► [LANGSUNG] Clean phone → Set whatsapp_reply="1" di Sheet
│    ⚡ Guard flag: memberitahu WF1 bahwa lead sudah reply
│
├──► [PROSES] Lookup PostgreSQL by phone → Spam check
│    │
│    ├── Spam/no match → No Operation (ignore)
│    │
│    └── OK → [PARALEL]:
│        │
│        ├── [AI Agent "Lina"]
│        │   Calendar context (28 hari) → GPT-4o-mini + Pinecone RAG
│        │   → Reply via Gupshup API
│        │   → Log User + Bot ke PostgreSQL conversation
│        │
│        ├── [Lead Classifier] (setiap pesan)
│        │   Append chat_history + increment chat_total
│        │   → GPT-4o-mini classify → interest2: Callback/Tidak Tertarik/Agent
│        │   → Update Sheet: interest2, Last_Leads_Chat
│        │   → If Agent → customer_type="Agent"
│        │   → If Callback → customer_type="Callback"
│        │   → Reset remarks (reset_remarks=NULL) agar bisa di-summary ulang
│        │
│        └── [Spam Classifier] (jika chat_total >= 5)
│            GPT-4o-mini: spam/not_spam
│            → If spam: PostgreSQL spam="1" + Sheet customer_type="Spam"
```

### System Prompt Lina AI — Aturan Kunci

**Persona**: "Lina AI — Asisten Virtual dari Rumah123 mewakili perumahan Anandaya"

**Strategi Penawaran Site Visit (Cascading)**:
1. "Kalau hari ini, Kakak bisa di jam berapa?"
2. Ditolak → "Besok gimana? Ada promo DP 0%, booking 3 juta..."
3. Ditolak → "Kalau weekend, hari apa?"
4. Ditolak → "Kapan Kakak bisa? Nanti saya reminder H-1"

**Klasifikasi Interest via Tool Call**:
- Setuju visit → `"Tertarik Site Visit (memberikan jadwal) (Hot Leads)"`
- Mau diinfo → `"Tertarik untuk dihubungi (Warm Leads)"`
- Tolak → `"Tidak mau atau tidak tertarik (Cold Leads)"`

**FAQ Hardcoded** (tanpa RAG):
- Ready/indent? → "Indent 6-12 bulan"
- Legalitas? → "SHM (Bisa AJB dan balik nama pecah sertifikat)"
- Pricelist? → "Akan diberikan oleh sales inhouse Anandaya"

### Lead Classifier Rules (Prioritas tinggi ke rendah)

1. **SUPER PRIORITAS**: Pertanyaan detail properti → `Callback`
2. Sebut proyek lain tanpa tanya detail → `Tidak Tertarik`
3. Afirmasi ("iya", "masih", "betul") → `Callback`
4. Menunda ("nanti dikabari", "insyaallah") → `Callback`
5. Berubah pikiran (awal tolak, akhir positif) → `Callback`
6. Penolakan tegas ("enggak", "stop") → `Tidak Tertarik`
7. Mengaku marketing/agent lain → `Agent`

### Chatbot Remarks Generator (Background)

```
Cron tiap 1 menit → SELECT WHERE reset_remarks IS NULL
→ If last_chat >= 5 jam lalu:
    → GPT-4o-mini: ringkasan 3 kalimat gaya sales analyst
    → Update Sheet: Chatbot_Remarks
    → PostgreSQL: reset_remarks = 1
```

**Bisnis penting**: Remarks ini diupdate ke LeadSquared agar telesales/developer **selalu punya context terkini** tentang percakapan lead. Jika lead chat lagi, `reset_remarks` di-reset ke NULL sehingga bisa diringkas ulang.

---

## 9. Tool — Simpan Visit Anandaya (Sub-workflow)

**File**: `Tool - Simpan Visit-Anandaya.json` | **ID**: `ffZtHJBr31a8NZXn`
**Dipanggil oleh**: AI Agent Chatbot saat lead memberikan data

### Input (10 parameter dari AI Agent)

`nama, hp, email, tanggal, bulan, tahun, jam, summary, interest, interest2`

### Flow

```
Input dari AI Agent
│
▼ SELALU: Append ke sheet chatbot_data (raw log)
│
▼ If interest == "Tertarik Site Visit (Hot Leads)"?
├── HOT: Lookup existing → Upsert Sheet AI_Call_data:
│   - phone, interest, summary (append timestamped), svs_date
│   - svs_date format: M/D/YYYY H:mm:ss AM/PM
│   - Bulan Indonesia → angka, jam 24h → 12h
│
└── WARM/COLD: Lookup existing → Upsert Sheet AI_Call_data:
    - phone, interest, summary (append timestamped)
    - svs_date TIDAK diupdate
```

**Summary append format**: `"{existing}\n\n{Jakarta datetime} --- {new summary}"`

---

## 10. Fase 4-5: Attempt 3 & 4 — AI Call Retry

**Trigger**: Cron (production)

### Filter Guard (SEMUA harus terpenuhi)

```
✅ phone terisi
✅ attempt bukan 0 atau 1 (sudah lewat WA phase)
✅ ≥ 3 jam sejak call_date terakhir
✅ whatsapp_reply ≠ "1" (lead BELUM reply WA)
✅ attempt < 4
✅ disconnected_reason ≠ Invalid_destination
✅ interest2 ≠ "Tidak Tertarik" (chatbot belum classify cold)
✅ disconnected_reason ≠ ivr_reached
✅ disconnected_reason ≠ voicemail_reached
```

→ Sort by attempt → Loop → Increment attempt → Retell call → Update sheet → Wait 20s → loop

**Key insight**: Guard flags dari chatbot (`whatsapp_reply` dan `interest2`) **mencegah call yang tidak perlu** — jika lead sudah engaging via WA, system respects itu.

---

## 11. Fase 6: Attempt 5 — WA Chatbot Final

**Trigger**: Cron tiap 2 menit

```
If7: attempt=4 + disconnected_reason terisi + whatsapp_reply kosong + bukan Invalid
→ Set attempt=5, whatsapp timestamp
→ KIRIM BRIDGING MESSAGE via Gupshup (HTTP Request13)
→ Simpan ke PostgreSQL conversation
```

---

## 12. Valid Number Check — 2Chat WhatsApp Verification

**Di dalam**: WF1 Anandaya Project, section CRM Update
**Tujuan**: Verifikasi apakah nomor lead terdaftar di WhatsApp sebelum kirim ke developer

### Flow

```
Loop Over Items → 2Chat "Check a phone number for whatsapp account"
→ Wait 2 detik (rate limit)
→ If on_whatsapp == true?
│
├── TRUE → Append/Update Sheet: Valid_Number = "Yes"
│
└── FALSE → Update Sheet:
    - Valid_Number = "No"
    - sent_to_dev = "No"
    - interest2 = "Tidak Tertarik"
    - customer_type = "Spam"
    → Lead TIDAK akan dikirim ke developer
```

Ada **5 instance** node check ini tersebar di berbagai section CRM Update (untuk setiap routing path). `Valid_Number` digunakan sebagai **gate** di section WA Group — lead dengan `Valid_Number` kosong (belum dicek) masih bisa lewat, tapi `"No"` akan diblokir.

---

## 13. Update Data ke LeadSquared CRM

**Trigger**: Google Sheets Trigger (poll tiap 1 menit, detect changes)

### LeadSquared API

| Detail | Nilai |
|--------|-------|
| **Endpoint** | `POST .../OpportunityManagement.svc/Update?accessKey=...&secretKey=...&leadId=...` |
| **Auth** | accessKey + secretKey di query params |

### Field Mapping

| LeadSquared Field | Schema | Value |
|-------------------|--------|-------|
| Contact Status | `mx_Custom_25` | Connected / Not Connected / Not Valid |
| Contact Date | `mx_Custom_26` | call_date |
| Contact Result | `mx_Custom_54` | Interest Project / No Pick Up / Spam / Agent / etc |
| Contact Result (copy) | `mx_Custom_81` | Same as mx_Custom_54 |
| Is Interested | `mx_Custom_56` | Yes / No |
| Last Contact Date | `mx_Custom_57` | call_date |
| Summary Notes | `mx_Custom_75` → `mx_CustomObject_121` | "Call Notes: {summary} \| Chat Notes: {Chatbot_Remarks}" |
| Visit Status | `mx_Custom_28` | "Visit Scheduled" (Hot only) |
| Visit Date | `mx_Custom_29` | svs_date (Hot only) |

### Routing Logic

```
Sheet change → Filter: id + attempt terisi
│
▼ Diangkat / WA reply? (agent_hangup / user_hangup / whatsapp_reply="1")
│
├── ADA RESPONS:
│   ├── "tidak ada percakapan cukup" → Connected - Valid Number
│   ├── Hot (Site Visit) → Interest Project + svs_date → mx_Custom_56=Yes
│   ├── Warm → Interest Project (no visit) → mx_Custom_56=Yes
│   ├── Cold → No Pick Up → mx_Custom_56=No
│   ├── Chatbot Tertarik/Mau Info → Interest Project
│   ├── Chatbot Tidak Tertarik → as mapped
│   └── customer_type routing: Spam/Agent/Unqualified/Callback/Double Data
│
└── TIDAK ADA RESPONS:
    ├── Invalid destination → Not Valid
    └── Lainnya → Not Connected - No Pick Up
│
▼ POST LeadSquared API
▼ Update Sheet: sent_to_dev="Yes", leadsquared_timestamp=now
▼ Log ke sheet "fix data for group wa"
```

**Dedup**: `leadsquared_timestamp` kosong = belum pernah push → proses. Sudah terisi → SKIP.

---

## 14. Send Valid Leads ke WA Group Developer

**Trigger**: Cron tiap 2 menit | **Platform**: 2Chat (sendToGroup)

### Filter

```
✅ sent_to_dev == "Yes"
✅ sent_to_wa_group KOSONG
✅ phone terisi
✅ interest2 ≠ Agent, Spam, Double Data
✅ attempt terisi
✅ Valid_Number kosong atau "Yes" (bukan "No")
✅ name terisi (untuk kirim ke grup developer)
```

### Template Pesan ke WA Group

| Kondisi | Pesan |
|---------|-------|
| **Hot + svs_date ada** | "New Leads ANANDAYA\n{name} {phone}\nMinat visit tanggal/jam: {svs_date}\nTanggal Inquiry: {date}\nSumber: Rumah123\nSales:" |
| **Warm + tanpa jadwal** (Callback/Mau Info) | "New Leads ANANDAYA\n{name} {phone}\nNotes: Tertarik diinfokan PL, brochure, dan promo\n..." |
| **Callback/Unqualified** | "New Leads ANANDAYA\n{name} {phone}\nTanggal Inquiry: ...\nSumber: Rumah123" |

### Handling Nama Kosong

Jika `name` kosong dan `internal_alert` belum di-set:
1. Kirim ke **grup internal** (bukan developer): "Projek Anandaya\nNo tlf: +{phone}\nNamanya masih kosong, tolong bantu diisi ya."
2. Set `internal_alert = 1` (cegah double alert)
3. Tim internal mencari tahu nama lead
4. Setelah nama diisi manual di sheet → **cron berikutnya** akan mendeteksi dan kirim ke grup developer

---

## 15. Alur Attempt Keseluruhan (End-to-End)

```
J+0 (Lead Masuk):
┌──────────────────────────────────────────────────────────────┐
│ Get Data: LeadSquared → Sheet + PostgreSQL (tiap 1-2 menit) │
└──────────────────────────┬───────────────────────────────────┘
                           ▼
              ┌─────────────────────┐
              │ ATTEMPT 1: AI CALL  │  Retell telepon lead
              └──────────┬──────────┘
                         ▼
             ┌───────────┴───────────┐
             │ Diangkat?             │
             ├─ YA + cukup ─────────── ✅ → CRM → WA Group (SELESAI)
             ├─ YA + kurang ────────── → Bridging WA (Gupshup) ↓
             └─ TIDAK ─────────────── → Bridging WA (Gupshup) ↓
                         ▼
              ┌───────────────────────────┐
              │ ATTEMPT 2: WA CHATBOT     │  Gupshup kirim bridging
              │ Chatbot "Lina AI" handle  │  Lead reply → Chatbot aktif
              └──────────┬────────────────┘
                         ▼
             ┌───────────┴───────────┐
             │ Reply WA?             │  whatsapp_reply = "1"
             ├─ YA ──────────────────── ✅ Chatbot classify → CRM → WA Group
             │                            (TIDAK di-call lagi)
             └─ TIDAK ──────────────── ↓
                         ▼
              ┌─────────────────────┐
  J+0 (3jam+):│ ATTEMPT 3: AI CALL  │  Re-call (guard: 3+ jam, no WA reply)
              └──────────┬──────────┘
                         ▼
              ┌─────────────────────┐
  J+0 (6jam+):│ ATTEMPT 4: AI CALL  │  Re-call lagi
              └──────────┬──────────┘
                         ▼
             ┌───────────┴───────────┐
             │ Ada respons?          │
             ├─ YA ──────────────────── ✅ → CRM → WA Group (SELESAI)
             └─ TIDAK ──────────────── ↓
                         ▼
              ┌───────────────────────────┐
              │ ATTEMPT 5: WA CHATBOT     │  Kesempatan terakhir
              └──────────┬────────────────┘
                         ▼
             ├─ Reply ───────────────── ✅ → CRM → WA Group
             └─ No reply ──────────── ❌ Lead maxed out

SPECIAL CASES:
  Voicemail/IVR di Attempt 1 → LANGSUNG attempt 5 (skip 2,3,4)
  Invalid Destination → STOP semua (CRM: Not Valid)
  Chatbot classify "Tidak Tertarik" → STOP re-call
  Lead reply WA → STOP re-call (engaging via chat)
```

---

## 16. Guard Flags & Anti-Duplikasi

| Flag | Lokasi | Trigger | Efek |
|------|--------|---------|------|
| `whatsapp_reply = "1"` | Sheet | WF2 saat lead reply | STOP re-call attempt 3/4 |
| `interest2 = "Tidak Tertarik"` | Sheet | WF2 classifier | STOP re-call |
| `leadsquared_timestamp` terisi | Sheet | WF1 setelah CRM push | SKIP CRM update (cegah double) |
| `sent_to_wa_group` terisi | Sheet | WF1 setelah WA group | SKIP WA group (cegah double) |
| `sent_to_dev = "Yes"` | Sheet | WF1 setelah CRM push | Gate untuk WA group |
| `spam = "1"` | PostgreSQL | WF2 spam classifier | BLOCK chatbot |
| `reset_remarks = 1` | PostgreSQL | WF2 remarks generator | SKIP remarks (cegah double) |
| `reset_remarks = NULL` | PostgreSQL | WF2 saat lead chat lagi | ENABLE remarks ulang |
| `attempt` kosong | Sheet | WF0 saat lead baru | Trigger attempt 1 |
| `disconnected_reason = Invalid_destination` | Sheet | Retell webhook | STOP semua |
| `Valid_Number = "No"` | Sheet | WF1 2Chat check | BLOCK kirim ke developer |
| `internal_alert = 1` | Sheet | WF1 WA group | Cegah double alert nama kosong |

---

## 17. Sales Assignment & Round Robin

Kolom `Sales` di sheet `AI_Call_data` diisi **otomatis oleh rumus spreadsheet** (bukan n8n):

```
=ARRAYFORMULA(IF(A2:A="", "", IFERROR(VLOOKUP(A2:A, 'fix data for group wa'!$A:$O, 15, 0), "")))
```

- Sheet `fix data for group wa` berisi daftar nama Sales yang di-assign ke project Anandaya
- Setiap project developer punya **list sales yang berbeda**
- Round-robin assignment di level spreadsheet
- Kolom `SPV` (Supervisor) menggunakan mechanism serupa

Ada logic di WF1 WA Group (`if sales not empty`) yang mengecek apakah Sales sudah terisi — ini mempengaruhi template pesan WA yang dikirim.

---

## 18. Counter & Remarks Tracking

### Counter Table (PostgreSQL)

```sql
UPDATE counter SET total_sent = total_sent + 1 WHERE id = 'counter_anandaya';
```

- Dijalankan setiap kali bridging message atau chatbot reply terkirim
- Tracking volume pesan WA untuk monitoring

### Remarks Flow (Bisnis Context Penting)

`Chatbot_Remarks` bukan sekadar ringkasan — ini adalah **context update** yang dikirim ke LeadSquared agar telesales/developer **selalu tahu percakapan terkini** dengan lead:

1. Lead chat dengan AI → conversation disimpan di PostgreSQL
2. Lead diam 5+ jam → AI generate ringkasan 3 kalimat
3. Ringkasan ditulis ke sheet (`Chatbot_Remarks`)
4. CRM Update push ke LeadSquared (`mx_Custom_75` → `mx_CustomObject_121`)
5. Jika lead chat **lagi** → `reset_remarks` di-reset → ringkasan baru di-generate
6. CRM Update push ulang dengan ringkasan terbaru

---

## 19. Credentials & External Services

| Service | Type | Identifier | Catatan |
|---------|------|-----------|---------|
| **Retell AI** | API Key | `key_6ac9bfd5de6da98d3e5777654461` | Hardcoded di HTTP node |
| **Retell Agent** | Agent ID | `agent_89c6e8e3b6d7ebcd52a50bc023` | Prompt ada di dashboard Retell |
| **LeadSquared** | API Keys | `accessKey: u$ra3055f0e...`, `secretKey: a5a33f66dbe...` | Hardcoded di URL |
| **Google Sheets** | OAuth2 | `Alex M Google Sheet` (ID: `kpYEZ2RamB9L1fvc`) | n8n Credentials |
| **2Chat** | API | `2Chat account` (ID: `GlTrCO1nOhVMsDNa`), WA: `+6285371025338` | n8n Credentials |
| **Gupshup** | API | userid: `2000258121`, password: `FptIsos0y` | Hardcoded di HTTP node |
| **OpenAI** | API | `OpenAi account` (ID: `C0ZiLuV41AdV6Nr8`) | n8n Credentials |
| **Pinecone** | API | `PineconeApi Alex Mario account` (ID: `Hk6B1LnTYJkMIRIC`) | n8n Credentials |
| **PostgreSQL** | Connection | `Alex` (ID: `dykHiiKfuGabdrmA`) | Supabase hosted |
| **Google Drive** | OAuth2 | `Google Drive account Alex Mario` | Knowledge base files |

> **SECURITY WARNING**: API keys Retell, LeadSquared, dan Gupshup di-hardcode dalam workflow. Mitigasi: pindahkan ke n8n Credentials Manager atau environment variables.

---

## 20. Known Issues & Gotchas

### Arsitektur

1. **Google Sheet sebagai database**: Rate limiting, data inconsistency, no transactions. Untuk scale multi-project, pertimbangkan migrasi ke database proper.
2. **Dual data store (Sheet + Supabase)**: Bisa out-of-sync. Chatbot baca Supabase, tapi CRM update baca Sheet.
3. **No error alerting**: API gagal hanya terlihat di n8n execution log.
4. **Paging limit 100**: Get Data max 100/run. Saat ini cukup (10-30 leads/hari), tapi perlu pagination jika scale.

### Workflow

5. **Race condition**: Chatbot set `whatsapp_reply=1` bersamaan dengan AI Call batch — lead di tengah batch masih bisa kena call.
6. **Chat memory 4 pesan**: Percakapan panjang kehilangan konteks awal.
7. **Calendar inconsisten**: Retell AI Call = 21 hari, Chatbot = 28 hari.
8. **Disabled nodes belum dihapus**: Legacy dari developer sebelumnya (handover). Contoh: counter nodes, beberapa node di section WA Group.
9. **Phone format inconsisten**: Get Data `0→62`, Webhook `strip +`, beberapa node pakai format berbeda.
10. **Summary unbounded append**: Tanpa limit, bisa sangat panjang untuk lead aktif.

### Data

11. **Nama kosong blocking**: Lead tanpa nama TIDAK dikirim ke developer — harus dicari manual dulu. Ini by design tapi bisa delay.
12. **API keys hardcoded**: Security risk — perlu migrasi ke credential manager.

---

## 21. Panduan Monitoring & Debugging

### Cek Status Lead

1. **PostgreSQL**: `SELECT * FROM anandaya WHERE phone = '628xxx';`
   - Tidak ada → masalah di Get Data
   - Ada → cek `attempt`, `spam`, `last_chat`
2. **Google Sheet**: Cek `disconnected_reason`, `whatsapp_reply`, `attempt`
   - `whatsapp_reply = "1"` → lead tidak akan dicall lagi
   - `Invalid_destination` → nomor mati
   - `Valid_Number = "No"` → tidak dikirim ke developer
3. **Retell Dashboard**: Cari nomor → lihat audio & transkrip call
4. **n8n Execution Log**: Cek error di workflow yang relevan

### Bottleneck Umum

- **Sheet Sync Delay**: Google Sheets trigger kadang lambat detect perubahan
- **Retell Rate Limit**: Pastikan Wait 20s antar call tetap ada
- **Chatbot Idle**: Cek koneksi OpenAI + Pinecone index status
- **Gupshup Block**: Jika reply tidak terkirim, cek status akun Gupshup

---

## 22. Knowledge Base (Pinecone & RAG)

### Konfigurasi

| Parameter | Nilai |
|-----------|-------|
| **Index Active** | `anandaya-update-2` |
| **Index Legacy** | `anandaya` (tidak dipakai) |
| **Embedding** | `text-embedding-3-small` (OpenAI) |
| **Chunk overlap** | 100 |

### Data Properti (Hardcoded di Code Node)

5 kategori:
1. **Kawasan dan Akses**: Lokasi, akses jalan, transportasi
2. **Fasilitas Internal**: Fasilitas dalam cluster
3. **Produk dan Harga**: 3 tipe unit:
   - **Arca**: 467 juta
   - **Bima**: 513 juta
   - **Devara**: 741 juta
4. **Pembayaran dan Legalitas**: DP 0%, KPR (BCA/OCBC/Mandiri/BSI/BCA Syariah), SHM
5. **Timeline dan USP**: USP project, timeline delivery

### Update Knowledge Base

1. Update file dokumen di Google Drive
2. Jalankan workflow upsert ke Pinecone (atau update hardcoded data di Code node)
3. Pastikan chatbot system prompt konsisten dengan data terbaru

---

## 23. Strategi Refactoring & Scaling Multi-Project

### Context

Sistem Anandaya ini **sudah proven di production** dan ingin di-scale ke project developer lainnya. Setiap project memiliki:
- List sales yang berbeda
- Prompt AI yang berbeda (properti, harga, lokasi)
- Knowledge base yang berbeda
- WA Group developer yang berbeda
- Tapi flow logic yang SAMA

### Rekomendasi Arsitektur untuk Multi-Project

| Komponen Sekarang | Rekomendasi Scale |
|--------------------|--------------------|
| 1 Sheet per project | Database PostgreSQL dengan tabel per-project atau multi-tenant |
| Hardcoded agent ID / prompt | Config table: project → retell_agent_id, gupshup_config, dll |
| Hardcoded WA Group UUID | Config table: project → wa_group_id |
| Separate workflow per project | Template workflow + parameterized project config |
| Sales VLOOKUP di Sheet | Database table: project_sales_assignment |
| Pinecone index per project | Multi-index atau namespace per project |

### Pemetaan Komponen (n8n → Custom Platform)

| Komponen n8n | Pengganti Custom |
|--------------|------------------|
| Cron Trigger | Worker/Queue (Asynq/BullMQ) |
| Google Sheets | PostgreSQL (single source of truth) |
| HTTP Request Nodes | API Client modules |
| Code Nodes | Native logic (services/repository) |
| Vector Store | LangChain / Pinecone SDK |
| AI Agent Node | LangChain / custom LLM wrapper |
| State via flags | State Machine pattern |

### Key Changes for Scale

1. **Single Source of Truth**: Gantikan Sheet dengan PostgreSQL
2. **State Machine**: Gantikan filter-based attempt tracking dengan explicit state transitions
3. **Queue System**: Redis/message queue untuk call dan WA — anti race condition, easy retry
4. **Centralized Credentials**: Environment variables / secrets manager
5. **Phone Normalization**: Satu kali di ingestion, simpan E.164 standar
6. **Project Config**: Satu workflow template, config per project di database

---

## 24. Glossary

| Istilah | Definisi |
|---------|----------|
| **Lead** | Calon pembeli properti yang datanya masuk dari portal (Rumah123, dll) ke LeadSquared |
| **Attempt** | Percobaan kontak ke lead (1-5). Ganjil = AI Call, Genap = WA Chatbot (kecuali shortcut) |
| **Interest** | Level ketertarikan dari AI Call: Hot/Warm/Cold/Insufficient |
| **Interest2** | Klasifikasi dari chatbot WA: Callback/Tidak Tertarik/Agent |
| **Customer Type** | Tipe lead: Interest/Callback/Unqualified/Spam/Agent/Double Number/Inactivity/Voicemail |
| **SVS Date** | Site Visit Schedule Date — tanggal kunjungan ke lokasi |
| **Disconnected Reason** | Alasan call Retell terputus |
| **Bridging Message** | Pesan WA pertama yang dikirim ke lead (memperkenalkan Lina AI) |
| **Guard Flag** | Field yang mencegah aksi duplikat atau mengontrol flow |
| **RAG** | Retrieval-Augmented Generation — AI jawab berdasarkan knowledge base |
| **Pinecone** | Vector database untuk embedding knowledge base |
| **Retell** | Platform AI Phone Call |
| **Gupshup** | Platform WA API — kirim pesan ke lead (resmi Meta, anti-ban) |
| **2Chat** | Platform WA — terima pesan + kirim ke group + valid number check |
| **LeadSquared** | CRM sumber lead & tujuan update |
| **Opportunity** | Entitas di LeadSquared = 1 peluang penjualan |
| **Supabase** | Hosting PostgreSQL yang digunakan untuk chatbot data store |
| **Event Code 12002** | Identifier di LeadSquared untuk project developer |
| **Round Robin** | Sistem assignment sales secara bergiliran menggunakan rumus spreadsheet |

---

> **Dokumen ini terakhir diverifikasi**: 10 April 2026
> **Sumber**: Analisis mendalam 4 file JSON workflow n8n + klarifikasi langsung dari PIC project
> **Status sistem**: Production (daily active)
> **Rencana**: Scale ke multi-project developer
