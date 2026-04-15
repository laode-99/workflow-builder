// Package integrations provides shared error types used by all outbound
// integration clients (Retell, LeadSquared, Gupshup, Pinecone, OpenAI, 2Chat).
//
// Clients must wrap every network error into one of the three categories
// below so callers can make retry decisions without inspecting HTTP status
// codes or error strings directly.
package integrations

import "errors"

// ErrTransient is a temporary failure (HTTP 5xx, 429, timeout, network blip).
// The caller should retry with backoff within the task's retry cap.
var ErrTransient = errors.New("integrations: transient failure")

// ErrPermanent is a non-retryable failure (HTTP 400, 404, malformed payload,
// resource missing). The caller should move the task to DLQ immediately.
var ErrPermanent = errors.New("integrations: permanent failure")

// ErrAuth is an authentication/authorization failure (HTTP 401/403, invalid
// API key). The caller should auto-pause the project and email the operator.
var ErrAuth = errors.New("integrations: auth failure")
