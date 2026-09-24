package bundle

import (
	"encoding/json"
	"testing"
)

func TestTemplatizeExpand(t *testing.T) {
	for _, tc := range []struct {
		name, root, home, in, templ string
	}{
		{
			name:  "plain paths",
			root:  "/Users/me/code/spools",
			home:  "/Users/me",
			in:    `{"a":"/Users/me/code/spools/main.go","b":"/Users/me/.zshrc"}`,
			templ: `{"a":"{{PROJECT}}/main.go","b":"{{HOME}}/.zshrc"}`,
		},
		{
			name:  "sibling with shared prefix is left alone",
			root:  "/Users/me/code/spools",
			home:  "/Users/me",
			in:    `{"a":"/Users/me/code/spools-web/x","b":"/Users/me/code/spools"}`,
			templ: `{"a":"{{HOME}}/code/spools-web/x","b":"{{PROJECT}}"}`,
		},
		{
			name:  "file uri with escaping",
			root:  "/Users/me/my code",
			home:  "/Users/me",
			in:    `{"uri":"file:///Users/me/my%20code/a.go","p":"/Users/me/my code/a.go"}`,
			templ: `{"uri":"file://{{PROJECT_URI}}/a.go","p":"{{PROJECT}}/a.go"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := string(Templatize([]byte(tc.in), tc.root, tc.home))
			if got != tc.templ {
				t.Fatalf("Templatize = %s, want %s", got, tc.templ)
			}
			if back := string(Expand([]byte(got), tc.root, tc.home)); back != tc.in {
				t.Fatalf("Expand = %s, want %s", back, tc.in)
			}
		})
	}
}

func TestExpandEscapesForJSON(t *testing.T) {
	root := `C:\Users\me\spools`
	got := Expand([]byte(`{"p":"{{PROJECT}}\\main.go"}`), root, "")
	var v struct{ P string }
	if err := json.Unmarshal(got, &v); err != nil {
		t.Fatalf("%s: %v", got, err)
	}
	if v.P != root+`\main.go` {
		t.Fatalf("got %q", v.P)
	}
}
