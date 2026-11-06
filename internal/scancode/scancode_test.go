package scancode

import (
	"reflect"
	"testing"
)

func TestResolveSingleKey(t *testing.T) {
	got, err := Resolve("enter")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"1c", "9c"}; !reflect.DeepEqual(got, want) {
		t.Errorf("enter = %v, want %v", got, want)
	}
}

func TestResolveCombo(t *testing.T) {
	// ctrl+alt+delete: make ctrl(1d) alt(38) delete(e0 53), then break in
	// reverse: delete(e0 d3) alt(b8) ctrl(9d).
	got, err := Resolve("ctrl+alt+delete")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1d", "38", "e0", "53", "e0", "d3", "b8", "9d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ctrl+alt+delete = %v, want %v", got, want)
	}
}

func TestResolveCaseAndSpaces(t *testing.T) {
	got, err := Resolve(" Shift + A ")
	if err != nil {
		t.Fatal(err)
	}
	// shift(2a) a(1e), break a(9e) shift(aa)
	want := []string{"2a", "1e", "9e", "aa"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("shift+a = %v, want %v", got, want)
	}
}

func TestResolveUnknown(t *testing.T) {
	if _, err := Resolve("notakey"); err == nil {
		t.Error("expected error for unknown key")
	}
}

func TestAvailableKeysSorted(t *testing.T) {
	keys := AvailableKeys()
	if len(keys) == 0 {
		t.Fatal("no keys")
	}
	for i := 1; i < len(keys); i++ {
		if keys[i-1] > keys[i] {
			t.Errorf("not sorted at %d: %q > %q", i, keys[i-1], keys[i])
		}
	}
}
