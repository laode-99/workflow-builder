-- Seed script for the first leadflow project in a freshly-migrated database.
--
-- Usage:
--   1. Edit the placeholder values below (marked <<<...>>>) to match your
--      new project: name, slug, CRM tag, Retell agent IDs, prompts, sales roster.
--   2. Run: psql $DATABASE_URL -f scripts/seed_project_one.sql
--   3. Insert credentials separately via the admin API (or a helper script)
--      because they must be AES-GCM encrypted with ENCRYPTION_KEY.
--   4. Verify with:
--        SELECT id, name, slug, status FROM businesses WHERE slug = '<<<slug>>>';
--
-- This script assumes the leadflow engine's tables are already created via
-- cmd/api's AutoMigrate.

BEGIN;

-- ---- 1. Project (Business) ----
-- `config` is the FlowConfig jsonb. Unset fields use compiled defaults from
-- internal/statemachine/defaults.go. Only override what differs per project.
INSERT INTO businesses (id, name, slug, config, timezone, status, activated_at, created_at, updated_at)
VALUES (
  gen_random_uuid(),
  '<<<Project Display Name>>>',
  '<<<project-slug>>>',
  '{
    "attempt_limit": 5,
    "call_retry_gap_hours": 3,
    "max_out_grace_hours": 24,
    "remarks_delay_hours": 5,
    "spam_classify_threshold": 5,
    "voicemail_shortcut_to_last": true,
    "business_hours": {"start": "07:00", "end": "20:00", "timezone": "Asia/Jakarta"},
    "channels_enabled": ["call", "whatsapp"],
    "ingestion_cron": "*/2 7-19 * * *",
    "language_code": "id-ID",
    "crm": {"provider": "leadsquared", "tag_filter": "<<<LeadSquared mx_Custom_1 value>>>", "activity_event": 12002}
  }'::jsonb::text,
  'Asia/Jakarta',
  'active',
  NOW(),
  NOW(),
  NOW()
);

-- Capture the new business ID in a psql variable for later inserts.
\gset
SELECT id AS new_business_id FROM businesses WHERE slug = '<<<project-slug>>>' \gset

-- ---- 2. Cron Workflows (one row per leadflow signature) ----
INSERT INTO workflows (id, business_id, signature, alias, is_active, trigger_cron, stop_time, variables, created_at, updated_at) VALUES
  (gen_random_uuid(), :'new_business_id', 'leadflow.ingest',            'Lead Ingestion',      true, '*/2 7-19 * * *', '20:00', '{}', NOW(), NOW()),
  (gen_random_uuid(), :'new_business_id', 'leadflow.attempt_manager',   'Attempt Manager',     true, '*/2 * * * *',    '',      '{}', NOW(), NOW()),
  (gen_random_uuid(), :'new_business_id', 'leadflow.remarks_generator', 'Remarks Generator',   true, '*/1 * * * *',    '',      '{}', NOW(), NOW()),
  (gen_random_uuid(), :'new_business_id', 'leadflow.wa_group_dispatch', 'WA Group Dispatcher', true, '*/2 * * * *',    '',      '{}', NOW(), NOW());

-- ---- 3. Prompts (extract Anandaya-equivalent content from docs/Chatbot Anandaya copy.json) ----
-- Each prompt row is versioned. is_active=true flags the live version.

INSERT INTO project_prompts (id, business_id, kind, version, content, is_active, created_by, created_at) VALUES
  (gen_random_uuid(), :'new_business_id', 'chatbot_system',           1, $prompt$<<<Paste the full Anandaya chatbot system prompt from docs/Chatbot Anandaya copy.json — the node named "AI Agent" has the prompt in its parameters>>>$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'chatbot_faq',              1, $prompt$<<<Paste the FAQ block: ready/indent, legalitas SHM, pricelist/brosur, video/sales name, akses & lokasi>>>$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'chatbot_tool_instructions',1, $prompt$<<<Paste the tool-calling instructions for save_leads_data and property_knowledge>>>$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'intent_classifier',        1, $prompt$Anda adalah intent classifier untuk lead properti. Berdasarkan pesan terakhir user, klasifikasikan sebagai SATU dari:
- Callback: User bertanya detail (harga, fasilitas, lokasi, cicilan) ATAU menunjukkan keraguan dengan potensi positif ("pikir-pikir", "nanti dikabari", "iya", "betul") ATAU berubah pikiran dari menolak ke positif.
- Tidak Tertarik: User secara eksplisit menolak ("enggak", "tidak", "stop") DAN tidak menanyakan detail lebih lanjut.
- Agent: User mengaku dari perusahaan/marketing lain.

PRIORITAS: Pertanyaan detail properti ALWAYS = Callback, bahkan jika user menyebut proyek kompetitor.

Respond with EXACTLY one of: Callback | Tidak Tertarik | Agent$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'spam_classifier',          1, $prompt$Anda adalah spam classifier. Berdasarkan 5+ pesan terakhir user, tentukan apakah ini percakapan spam:
- spam: Pesan tidak masuk akal berulang, konten promosi, topik tidak relevan, karakter acak.
- not_spam: Pertanyaan properti, jawaban yang masuk akal meski singkat, konteks pembelian/sewa.

Respond with EXACTLY one word: spam OR not_spam$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'remarks_generator',        1, $prompt$Buat ringkasan 3 kalimat gaya sales analyst dari percakapan berikut. Format: (1) Ringkasan intent user, (2) Detail spesifik yang user tanyakan atau minati, (3) Rekomendasi tindak lanjut untuk sales.$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'wa_bridging',              1, $prompt$Halo Kak, Saya Lina AI mewakili <<<nama proyek>>>. Terimakasih kakak baru saja mengklik iklan kami. Apakah kakak tertarik dengan <<<nama proyek>>> dan mau diinfokan lebih lanjut?$prompt$, true, 'seed', NOW()),
  (gen_random_uuid(), :'new_business_id', 'wa_final',                 1, $prompt$Halo Kak, saya Lina AI dari <<<nama proyek>>>. Sebelumnya kami sudah mencoba menghubungi via telepon namun tidak tersambung. Apakah kakak masih tertarik untuk mendapat informasi properti <<<nama proyek>>>?$prompt$, true, 'seed', NOW());

-- ---- 4. Sales roster (round-robin pool) ----
-- Add at least one row. WA group IDs come from your 2Chat or Gupshup account.
INSERT INTO sales_assignments (id, business_id, sales_name, spv_name, wa_group_id, is_active, assign_count, created_at, updated_at) VALUES
  (gen_random_uuid(), :'new_business_id', '<<<Sales Name 1>>>', '<<<SPV Name>>>', '<<<wa_group_id_1>>>', true, 0, NOW(), NOW());

COMMIT;

-- ---- 5. Credentials (MANUAL STEP) ----
-- Credentials must be AES-GCM encrypted with ENCRYPTION_KEY before insertion.
-- Write a short Go helper or use the admin API to add:
--
--   Per-project:
--     retell_ai      JSON: {"api_key": "...", "agent_id_1": "...", "agent_id_3": "...", "from_number": "..."}
--     leadsquared    JSON: {"access_key": "...", "secret_key": "..."}
--     gupshup        JSON: {"UserID": "...", "Password": "...", "AppName": "...", "SrcNumber": "..."}
--     pinecone       JSON: {"api_key": "...", "host": "https://...pinecone.io"}
--     webhook_secret Plain: "<hex-random-32-bytes>"
--
--   Global (is_global=true, business_id=NULL):
--     openai         Plain: "sk-..."
--     gmail_smtp     JSON: {"host": "smtp.gmail.com", "port": 587, "user": "...", "password": "<app_password>"}
--
-- See internal/sdk/vault.go for the credential model and encryption flow.
