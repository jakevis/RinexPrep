package gnss

import "testing"

func TestValidate_AllPresent(t *testing.T) {
	m := Metadata{
		AntennaType:  "TRM57971.00",
		ReceiverType: "TRIMBLE NETR9",
		ApproxX:      -740000,
		ApproxY:      -5457000,
		ApproxZ:      3207000,
	}
	issues := m.Validate()
	if len(issues) != 0 {
		t.Errorf("Validate() returned %d issues, want 0: %v", len(issues), issues)
	}
}

func TestValidate_MissingAntennaType(t *testing.T) {
	m := Metadata{
		ReceiverType: "TRIMBLE NETR9",
		ApproxX:      1, ApproxY: 2, ApproxZ: 3,
	}
	issues := m.Validate()
	if !containsSubstring(issues, "antenna") {
		t.Errorf("Validate() should report missing antenna type, got %v", issues)
	}
}

func TestValidate_MissingPosition(t *testing.T) {
	m := Metadata{
		AntennaType:  "TRM57971.00",
		ReceiverType: "TRIMBLE NETR9",
		ApproxX:      0, ApproxY: 0, ApproxZ: 0,
	}
	issues := m.Validate()
	if !containsSubstring(issues, "position") {
		t.Errorf("Validate() should report missing position, got %v", issues)
	}
}

func TestValidate_MissingReceiverType(t *testing.T) {
	m := Metadata{
		AntennaType: "TRM57971.00",
		ApproxX:     1, ApproxY: 2, ApproxZ: 3,
	}
	issues := m.Validate()
	if !containsSubstring(issues, "receiver") {
		t.Errorf("Validate() should report missing receiver type, got %v", issues)
	}
}

func TestValidate_MultiMissing(t *testing.T) {
	m := Metadata{} // everything zero/empty
	issues := m.Validate()
	if len(issues) != 3 {
		t.Errorf("Validate() returned %d issues, want 3: %v", len(issues), issues)
	}
	if !containsSubstring(issues, "antenna") {
		t.Error("missing antenna type not reported")
	}
	if !containsSubstring(issues, "position") {
		t.Error("missing position not reported")
	}
	if !containsSubstring(issues, "receiver") {
		t.Error("missing receiver type not reported")
	}
}

func TestValidate_PartialPosition(t *testing.T) {
	// Only one coord non-zero → position is present
	m := Metadata{
		AntennaType:  "ANT",
		ReceiverType: "REC",
		ApproxX:      1,
	}
	issues := m.Validate()
	if containsSubstring(issues, "position") {
		t.Errorf("Validate() should NOT report missing position when X!=0, got %v", issues)
	}
}

// helper
func containsSubstring(ss []string, sub string) bool {
	for _, s := range ss {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
