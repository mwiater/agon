package accuracy

import "testing"

func TestMatchesExpectedCommaFormatted(t *testing.T) {
	response := "299,792,484"
	if !matchesExpected(response, 299792458, 500) {
		t.Fatalf("expected comma-formatted response to match within margin; response=%q", response)
	}
}

func TestMatchesExpectedIgnoresThinkBlockAfterAnswer(t *testing.T) {
	response := "180\n<think>A triangle has three angles. 3-2=1, so 180.</think>"
	if !matchesExpected(response, 180, 0) {
		t.Fatalf("expected response with trailing <think> block to match; response=%q", response)
	}
}

func TestNormalizeResponseAvoidsConcatenatingRepeatedAnswers(t *testing.T) {
	response := "15\n<think>x=5, y=x+10 so y=15.</think>\n15"
	normalized := normalizeResponse(response)
	if normalized != "15" {
		t.Fatalf("expected normalized response to be 15, got %q", normalized)
	}
}
