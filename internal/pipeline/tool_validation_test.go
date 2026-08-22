package pipeline

import (
	"os"
	"testing"
)

func TestValidateToolRejectsMissingConfiguration(t *testing.T) {
	if err := validateTool(nil); err == nil {
		t.Fatal("expected nil tool to be rejected")
	}
	if err := validateTool(&Tool{}); err == nil {
		t.Fatal("expected empty command to be rejected")
	}
}

func TestValidateToolAllowsEmptyArgs(t *testing.T) {
	// The current test binary is necessarily executable, making this test
	// independent of which third-party recon tools happen to be installed in CI.
	tool := &Tool{Command: os.Args[0], Args: nil}
	if err := validateTool(tool); err != nil {
		t.Fatalf("valid executable with empty args was rejected: %v", err)
	}
}
