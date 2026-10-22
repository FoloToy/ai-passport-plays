package instance

import "testing"

func TestSecondInstanceRefusedAndReleaseReopens(t *testing.T) {
	dir := t.TempDir()
	unlock, e := Acquire(dir)
	if e != nil {
		t.Fatal(e)
	}
	if second, e := Acquire(dir); e == nil {
		second()
		t.Fatal("second writer accepted")
	}
	unlock()
	again, e := Acquire(dir)
	if e != nil {
		t.Fatal(e)
	}
	again()
}
