package httpapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenAPIListsPublicRoutes(t *testing.T) {
	contract, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "contracts", "openapi.yaml"))
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}

	for _, path := range []string{
		"/auth/dev/token:",
		"/healthz:",
		"/readyz:",
		"/v1/documents:",
		"/v1/documents/{id}:",
		"/v1/documents/{id}/download:",
		"/v1/documents/{id}/versions/{versionId}/status:",
		"/v1/documents/{id}/reindex:",
		"/v1/departments:",
		"/v1/chat/sessions:",
		"/v1/chat/sessions/{id}/messages:",
	} {
		if !strings.Contains(string(contract), "  "+path) {
			t.Errorf("OpenAPI contract does not document route %s", path)
		}
	}

	if strings.Contains(string(contract), "\n  /documents:") || strings.Contains(string(contract), "\n  /chat/sessions:") {
		t.Error("OpenAPI contract must use the /v1 route prefix")
	}
}
