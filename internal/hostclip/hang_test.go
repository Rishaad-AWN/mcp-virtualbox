package hostclip

import (
	"context"
	"testing"
	"time"
)

func TestSetReturnsPromptly(t *testing.T) {
	start := time.Now()
	err := Set(context.Background(), "hang-fix-test")
	d := time.Since(start)
	if err != nil {
		t.Logf("Set err (may be no-display in CI): %v", err)
	}
	if d > 3*time.Second {
		t.Fatalf("Set took %v — hang not fixed", d)
	}
	t.Logf("Set returned in %v (err=%v)", d, err)
}
