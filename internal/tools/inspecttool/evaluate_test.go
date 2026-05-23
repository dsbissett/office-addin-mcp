package inspecttool

import "testing"

func TestIsFetchFailure(t *testing.T) {
	hits := []string{
		"TypeError: Failed to fetch",
		"Uncaught (in promise) TypeError: Failed to fetch\n    at uploadPdfs",
		"NetworkError when attempting to fetch resource.",
		"net::ERR_CONNECTION_REFUSED",
		"Load failed",
	}
	for _, m := range hits {
		if !isFetchFailure(m) {
			t.Errorf("isFetchFailure(%q) = false, want true", m)
		}
	}
	misses := []string{
		"TypeError: undefined is not a function",
		"ReferenceError: foo is not defined",
		"SyntaxError: Unexpected token",
		"",
	}
	for _, m := range misses {
		if isFetchFailure(m) {
			t.Errorf("isFetchFailure(%q) = true, want false", m)
		}
	}
}
