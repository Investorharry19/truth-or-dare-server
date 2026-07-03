package room

import "testing"

func TestPasscodeIsSingleUse(t *testing.T) {
	r := &Room{Private: true, Password: "ABC123"}

	if !r.consumePasscodeLocked("ABC123") {
		t.Fatal("expected first use to succeed")
	}

	if r.consumePasscodeLocked("ABC123") {
		t.Fatal("expected second use to fail")
	}

	if r.Password != "" {
		t.Fatalf("expected password to be cleared, got %q", r.Password)
	}
}

func TestGeneratePasscodeCreatesNewValue(t *testing.T) {
	r := &Room{Private: true}
	first := r.generatePasscodeLocked()
	second := r.generatePasscodeLocked()

	if first == "" || second == "" {
		t.Fatal("expected generated passcodes to be non-empty")
	}

	if first == second {
		t.Fatal("expected a new passcode to be generated")
	}
}
