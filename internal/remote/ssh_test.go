package remote

import "testing"

func TestCommand(t *testing.T) {
	got := Command("spools", "zed", "import", "-", "--project", "/my dir/it's")
	want := `spools zed import - --project '/my dir/it'\''s'`
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
