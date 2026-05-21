package service

import (
	"net/http"
	"testing"
)

func TestResolveOpenAIResponsesProbeSupportDecision_SupportedStatuses(t *testing.T) {
	tests := []int{
		http.StatusOK,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusUnprocessableEntity,
	}

	for _, status := range tests {
		supported, persist := resolveOpenAIResponsesProbeSupportDecision(status)
		if !persist {
			t.Fatalf("status %d persist = false, want true", status)
		}
		if !supported {
			t.Fatalf("status %d supported = false, want true", status)
		}
	}
}

func TestResolveOpenAIResponsesProbeSupportDecision_NotFoundStatuses(t *testing.T) {
	tests := []int{http.StatusNotFound, http.StatusMethodNotAllowed}

	for _, status := range tests {
		supported, persist := resolveOpenAIResponsesProbeSupportDecision(status)
		if !persist {
			t.Fatalf("status %d persist = false, want true", status)
		}
		if supported {
			t.Fatalf("status %d supported = true, want false", status)
		}
	}
}

func TestResolveOpenAIResponsesProbeSupportDecision_5xxDoesNotPersist(t *testing.T) {
	tests := []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}

	for _, status := range tests {
		supported, persist := resolveOpenAIResponsesProbeSupportDecision(status)
		if persist {
			t.Fatalf("status %d persist = true, want false", status)
		}
		if supported {
			t.Fatalf("status %d supported = true, want false when persist=false", status)
		}
	}
}
