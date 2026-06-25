package tapetest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// RecordedRequest holds the captured request data for documentation generation.
type RecordedRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`
	Query   map[string]string `json:"query,omitempty"`
	// Files records multipart/form-data file field names -> original filename.
	// Used by the OpenAPI generator to build binary upload fields.
	Files map[string]string `json:"files,omitempty"`
}

// RecordedResponse holds the captured response data for documentation generation.
type RecordedResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    interface{}       `json:"body,omitempty"`
}

// RecordedExchange represents a complete request/response pair captured during testing.
type RecordedExchange struct {
	Test      string           `json:"test"`
	Request   RecordedRequest  `json:"request"`
	Response  RecordedResponse `json:"response"`
	Timestamp string           `json:"timestamp"`
	// DocOrder controls example ordering in the generated documentation.
	// When nil the exchange keeps its default (recording) order.
	// 0 (or positive) puts it first, negative values put it last.
	DocOrder *int `json:"docOrder,omitempty"`
	// ExcludeFromDocs omits this exchange from the generated documentation
	// entirely. Set by DocOrder(nil).
	ExcludeFromDocs bool `json:"excludeFromDocs,omitempty"`
	// Server is the optional logical name of the backend service this
	// exchange was recorded against (e.g. "Admin API"). When set together
	// with ServerURL, the generated OpenAPI document emits a per-operation
	// servers entry so that Swagger UI's "Try it out" routes the request to
	// the correct service.
	Server string `json:"server,omitempty"`
	// ServerURL is the URL of the service this exchange was recorded against.
	// It may be relative (e.g. "/api/v1") or absolute
	// (e.g. "https://user-api.example.com"). Relative URLs are resolved by
	// Swagger UI against wherever the docs are hosted and are also stripped
	// from recorded request paths; absolute URLs are emitted verbatim. Used
	// together with Server to tag per-service recordings.
	ServerURL string `json:"serverUrl,omitempty"`
}

// Recorder manages recording of test request/response exchanges.
// It writes recorded data to a .tapetest/ directory in the project root.
type Recorder struct {
	mu        sync.Mutex
	dir       string
	exchanges []RecordedExchange
	enabled   bool
}

// globalRecorder is the singleton recorder instance.
var globalRecorder = &Recorder{
	enabled: false,
}

// EnableRecording turns on request/response recording.
// Records are stored in the given directory (typically ".tapetest").
//
//	EnableRecording(".tapetest")
func EnableRecording(dir string) {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()
	globalRecorder.dir = dir
	globalRecorder.enabled = true
	globalRecorder.exchanges = nil
}

// DisableRecording turns off request/response recording.
func DisableRecording() {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()
	globalRecorder.enabled = false
}

// IsRecordingEnabled returns whether recording is currently active.
func IsRecordingEnabled() bool {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()
	return globalRecorder.enabled
}

// Record captures a request/response exchange.
// Called internally by the Client after each request.
//
// This is a convenience wrapper around RecordExchange for exchanges that do
// not carry per-service server metadata. It is kept for backwards
// compatibility.
func Record(testName string, req RecordedRequest, resp RecordedResponse) {
	RecordExchange(RecordedExchange{
		Test:      testName,
		Request:   req,
		Response:  resp,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	})
}

// RecordExchange records a fully-formed exchange, including optional
// per-service Server/ServerURL metadata. A missing Timestamp is filled in
// automatically. It is the lowest-level recording entry point used by the
// Client so it can tag each recording with its service.
func RecordExchange(ex RecordedExchange) {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()

	if !globalRecorder.enabled {
		return
	}

	if ex.Timestamp == "" {
		ex.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	globalRecorder.exchanges = append(globalRecorder.exchanges, ex)
}

// SetLastExchangeDocOrder applies documentation-ordering metadata to the most
// recently recorded exchange. It is invoked by (*Response).DocOrder.
//
//	order == nil → the exchange is excluded from the generated docs
//	order != nil → the pointed-to int controls example priority
//	   (0/positive = first, negative = last)
func SetLastExchangeDocOrder(order *int) {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()

	if !globalRecorder.enabled || len(globalRecorder.exchanges) == 0 {
		return
	}

	last := &globalRecorder.exchanges[len(globalRecorder.exchanges)-1]
	if order == nil {
		last.ExcludeFromDocs = true
		last.DocOrder = nil
		return
	}
	o := *order
	last.DocOrder = &o
	last.ExcludeFromDocs = false
}

// FlushRecording writes all recorded exchanges to the .tapetest/ directory.
// Call this after all tests complete (e.g., in TestMain).
//
//	FlushRecording()
func FlushRecording() error {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()

	if !globalRecorder.enabled || len(globalRecorder.exchanges) == 0 {
		return nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(globalRecorder.dir, 0755); err != nil {
		return fmt.Errorf("tapetest: failed to create recording dir: %w", err)
	}

	// Write exchanges as JSON
	data, err := json.MarshalIndent(globalRecorder.exchanges, "", "  ")
	if err != nil {
		return fmt.Errorf("tapetest: failed to marshal recordings: %w", err)
	}

	path := filepath.Join(globalRecorder.dir, "recordings.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("tapetest: failed to write recordings: %w", err)
	}

	return nil
}

// LoadRecordings reads recorded exchanges from the .tapetest/ directory.
// Used by the documentation generator.
func LoadRecordings(dir string) ([]RecordedExchange, error) {
	path := filepath.Join(dir, "recordings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tapetest: failed to read recordings: %w", err)
	}

	var exchanges []RecordedExchange
	if err := json.Unmarshal(data, &exchanges); err != nil {
		return nil, fmt.Errorf("tapetest: failed to parse recordings: %w", err)
	}

	return exchanges, nil
}

// GetRecordings returns the in-memory recorded exchanges (without flushing to disk).
func GetRecordings() []RecordedExchange {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()
	return globalRecorder.exchanges
}

// ClearRecordings removes all in-memory recordings and deletes the recording files.
func ClearRecordings() error {
	globalRecorder.mu.Lock()
	defer globalRecorder.mu.Unlock()

	globalRecorder.exchanges = nil

	if globalRecorder.dir != "" {
		path := filepath.Join(globalRecorder.dir, "recordings.json")
		return os.Remove(path)
	}
	return nil
}
