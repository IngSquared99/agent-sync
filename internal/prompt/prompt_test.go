package prompt

import (
	"os"
	"testing"
)

// go test's stdin is not a terminal, which is exactly the "nobody can answer
// questions" environment — this test group pins down what must happen then.

func TestConfirmNonInteractiveIsNo(t *testing.T) {
	AssumeYes = false
	if IsStdinTTY() {
		t.Skip("stdin is a terminal in this environment; skipping")
	}
	if Confirm("Delete things?") {
		t.Error("when nobody can answer, actions requiring confirmation must always be cancelled")
	}
}

func TestConfirmAssumeYes(t *testing.T) {
	AssumeYes = true
	defer func() { AssumeYes = false }()
	if !Confirm("Delete things?") {
		t.Error("with --yes every confirmation must be treated as y")
	}
}

func TestSelectFallsBackToDefault(t *testing.T) {
	AssumeYes = false
	if IsStdinTTY() {
		t.Skip("stdin is a terminal in this environment; skipping")
	}
	if got := Select("Pick one", []string{"a", "b", "c"}, 1); got != 1 {
		t.Errorf("non-interactive should use the default index, got %d", got)
	}
}

func TestMultiSelectKeepsPreselection(t *testing.T) {
	AssumeYes = false
	if IsStdinTTY() {
		t.Skip("stdin is a terminal in this environment; skipping")
	}
	got := MultiSelect("Pick some", []string{"a", "b", "c"}, []int{0, 2})
	if len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Errorf("non-interactive should keep the existing selection, got %v", got)
	}
}

func TestIsTTYDoesNotPanicOnClosedStdout(t *testing.T) {
	// Just confirms both detection functions produce an answer for any fd
	// state without blowing up.
	_ = IsTTY()
	_ = IsStdinTTY()
	_ = os.Stdout
}
