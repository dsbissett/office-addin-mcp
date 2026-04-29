package cdp

import (
	"context"
	"encoding/json"
	"fmt"
)

// EvaluateParams mirrors the inputs to Runtime.evaluate that we expose.
type EvaluateParams struct {
	Expression    string `json:"expression"`
	AwaitPromise  bool   `json:"awaitPromise,omitempty"`
	ReturnByValue bool   `json:"returnByValue,omitempty"`
	UserGesture   bool   `json:"userGesture,omitempty"`
}

// RemoteObject is a CDP Runtime.RemoteObject.
type RemoteObject struct {
	Type                string          `json:"type"`
	Subtype             string          `json:"subtype,omitempty"`
	ClassName           string          `json:"className,omitempty"`
	Value               json.RawMessage `json:"value,omitempty"`
	UnserializableValue string          `json:"unserializableValue,omitempty"`
	Description         string          `json:"description,omitempty"`
	ObjectID            string          `json:"objectId,omitempty"`
}

// ExceptionDetails is a CDP Runtime.ExceptionDetails.
type ExceptionDetails struct {
	ExceptionID  int           `json:"exceptionId"`
	Text         string        `json:"text"`
	LineNumber   int           `json:"lineNumber"`
	ColumnNumber int           `json:"columnNumber"`
	URL          string        `json:"url,omitempty"`
	Exception    *RemoteObject `json:"exception,omitempty"`
}

// String renders a human-readable summary of the exception.
func (e *ExceptionDetails) String() string {
	if e == nil {
		return ""
	}
	if e.Exception != nil && e.Exception.Description != "" {
		return e.Exception.Description
	}
	return e.Text
}

// EvaluateResult is the unmarshaled response from Runtime.evaluate.
type EvaluateResult struct {
	Result           *RemoteObject     `json:"result"`
	ExceptionDetails *ExceptionDetails `json:"exceptionDetails,omitempty"`
}

// Evaluate runs Runtime.evaluate inside the given session. sessionID must be
// non-empty — Runtime.evaluate requires a target session.
func (c *Connection) Evaluate(ctx context.Context, sessionID string, p EvaluateParams) (*EvaluateResult, error) {
	if sessionID == "" {
		return nil, fmt.Errorf("cdp evaluate: sessionID is required")
	}
	raw, err := c.Send(ctx, sessionID, "Runtime.evaluate", p)
	if err != nil {
		return nil, err
	}
	var out EvaluateResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decode evaluate: %w", err)
	}
	return &out, nil
}
