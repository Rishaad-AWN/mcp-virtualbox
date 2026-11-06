package tools

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// The guest password must reach VBoxManage through a file, never the command line.
func TestCredentialArgsUsesPasswordFile(t *testing.T) {
	args, cleanup, err := credentialArgs("bob", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "s3cret") || !strings.HasPrefix(args[2], "--passwordfile=") {
		t.Fatalf("password leaked or file flag missing: %v", args)
	}
	path := strings.TrimPrefix(args[2], "--passwordfile=")
	if b, err := os.ReadFile(path); err != nil || string(b) != "s3cret" {
		t.Fatalf("password file = %q, %v", b, err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("password file not removed: %v", err)
	}

	args, cleanup, _ = credentialArgs("bob", "")
	cleanup()
	if len(args) != 2 {
		t.Fatalf("blank password should add no password flag: %v", args)
	}
}

func TestWaitFor(t *testing.T) {
	states := []string{"running", "stopping", "poweroff"}
	i := 0
	get := func() (string, error) { s := states[min(i, len(states)-1)]; i++; return s, nil }
	if st, ok, err := waitFor(context.Background(), time.Second, time.Millisecond, get, "poweroff"); !ok || st != "poweroff" || err != nil {
		t.Fatalf("got %q %v %v", st, ok, err)
	}

	stuck := func() (string, error) { return "stopping", nil }
	start := time.Now()
	st, ok, _ := waitFor(context.Background(), 50*time.Millisecond, 10*time.Millisecond, stuck, "poweroff")
	if ok || st != "stopping" || time.Since(start) > time.Second {
		t.Fatalf("stuck VM: got %q ok=%v after %v", st, ok, time.Since(start))
	}
}

func TestCreatedHostOnlyName(t *testing.T) {
	out := "0%...10%...100%\nInterface 'VirtualBox Host-Only Ethernet Adapter #2' was successfully created\n"
	if got := createdHostOnlyName(out); got != "VirtualBox Host-Only Ethernet Adapter #2" {
		t.Fatalf("got %q", got)
	}
	if createdHostOnlyName("error") != "" {
		t.Fatal("no name expected")
	}
}
