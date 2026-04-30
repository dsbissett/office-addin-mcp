// Package tools defines the registry, dispatcher, JSON-Schema-validated tool
// boundary, and uniform result envelope shared by every office-addin-mcp tool.
//
// The envelope shape is the public contract for agents — see PLAN.md §3 and §10
// (the "tool-call output stability" risk). Changes to the envelope require
// bumping EnvelopeVersion and updating the golden-JSON fixtures in testdata.
package tools

// EnvelopeVersion is stamped into Diagnostics.EnvelopeVersion. Bump on any
// breaking change to Envelope, EnvelopeError, or Diagnostics field semantics.
//
//	v0.1 — initial uniform envelope (Phase 3).
//	v0.2 — sessionId now means the user/Phase-5 session; cdpSessionId carries
//	       the CDP flatten session id; cdpRoundTrips diagnostic added.
const EnvelopeVersion = "v0.2"

// Error categories. New categories must also be documented in
// docs/tool-contracts.md (Phase 6) and added to the golden-JSON fixtures if a
// scenario can produce them.
const (
	CategoryValidation  = "validation"
	CategoryNotFound    = "not_found"
	CategoryTimeout     = "timeout"
	CategoryConnection  = "connection"
	CategoryProtocol    = "protocol"
	CategoryUnsupported = "unsupported"
	CategoryOfficeJS    = "office_js"
	CategoryInternal    = "internal"
)

// Envelope is the uniform tool result. Either Data or Error is set, never both.
type Envelope struct {
	OK          bool           `json:"ok"`
	Data        any            `json:"data,omitempty"`
	Error       *EnvelopeError `json:"error,omitempty"`
	Diagnostics Diagnostics    `json:"diagnostics"`
}

// EnvelopeError is the failure payload.
type EnvelopeError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Category  string         `json:"category"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

// Diagnostics carries observability fields populated by every tool. Variable
// fields (DurationMs, CDPRoundTrips) are stamped by the dispatcher; the tool
// fills in TargetID/CDPSessionID/Endpoint where relevant.
type Diagnostics struct {
	Tool            string `json:"tool"`
	EnvelopeVersion string `json:"envelopeVersion"`
	// SessionID is the user-facing (Phase 5) session id. For one-shot calls
	// this is empty unless the caller explicitly named one.
	SessionID string `json:"sessionId,omitempty"`
	// CDPSessionID is the CDP flatten session id assigned by Target.attachToTarget.
	CDPSessionID string `json:"cdpSessionId,omitempty"`
	TargetID     string `json:"targetId,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	// CDPRoundTrips is the count of CDP commands issued during this tool call.
	// Drops sharply on session reuse (PLAN.md §7 Phase 5 deliverable).
	CDPRoundTrips int64 `json:"cdpRoundTrips"`
	DurationMs    int64 `json:"durationMs"`
}
