package argv

import "testing"

func TestArgSetRejectsDuplicateNames(t *testing.T) {
	set := &argSpecs{}
	set.add("path", "first path")

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected duplicate argument panic")
		}
	}()

	set.add("path", "second path")
}
