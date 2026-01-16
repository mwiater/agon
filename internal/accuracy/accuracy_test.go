package accuracy

import "testing"

func TestMatchesExpectedCommaFormatted(t *testing.T) {
	response := "299,792,484"
	if !matchesExpected(response, 299792458, 500) {
		t.Fatalf("expected comma-formatted response to match within margin; response=%q", response)
	}
}
