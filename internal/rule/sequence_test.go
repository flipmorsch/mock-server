package rule_test

import (
	"strings"
	"testing"

	. "github.com/flipmorsch/mock-server/internal/rule"
)

func TestSequencedResponsesValidation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string // substring; "" means expect success
	}{
		{
			name: "valid sequenced rule",
			yaml: `rules:
  - id: poll
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 200, body: done}`,
		},
		{
			name: "single-element list is allowed",
			yaml: `rules:
  - id: once
    request: {method: GET, path: /x}
    responses:
      - {status: 200}`,
		},
		{
			name: "missing id is rejected",
			yaml: `rules:
  - request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 200}`,
			wantErr: "explicit id",
		},
		{
			name: "response and responses together is rejected",
			yaml: `rules:
  - id: both
    request: {method: GET, path: /job}
    response: {status: 200}
    responses:
      - {status: 202}`,
			wantErr: "mutually exclusive",
		},
		{
			name: "empty responses list is rejected",
			yaml: `rules:
  - id: empty
    request: {method: GET, path: /job}
    responses: []`,
			wantErr: "at least one element",
		},
		{
			name: "bad status in an element is rejected",
			yaml: `rules:
  - id: bad
    request: {method: GET, path: /job}
    responses:
      - {status: 202}
      - {status: 999}`,
			wantErr: "out of range",
		},
		{
			name: "body and body_file in an element is rejected",
			yaml: `rules:
  - id: bad
    request: {method: GET, path: /job}
    responses:
      - {status: 200, body: x, body_file: /tmp/y}`,
			wantErr: "mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(tt.yaml))
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}
