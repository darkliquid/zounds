package clap

import (
	"errors"
	"testing"
)

func TestIsInvalidAttentionMaskInputError(t *testing.T) {
	if !isInvalidAttentionMaskInputError(errors.New("Error running network: Invalid input name: attention_mask")) {
		t.Fatal("expected attention_mask error to be detected")
	}
	if isInvalidAttentionMaskInputError(errors.New("some other error")) {
		t.Fatal("did not expect unrelated error to be detected")
	}
}
