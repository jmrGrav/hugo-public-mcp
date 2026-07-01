package pathguard

import "testing"

func TestValidateRelative(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "simple", input: "posts/hello", want: "posts/hello"},
		{name: "dot segments", input: "posts/./hello", want: "posts/hello"},
		{name: "traversal", input: "posts/../escape", wantErr: true},
		{name: "backslash", input: "posts\\hello", wantErr: true},
		{name: "absolute", input: "/posts/hello", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateRelative(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ValidateRelative(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateRelative(%q) error = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ValidateRelative(%q) = %q want %q", tc.input, got, tc.want)
			}
		})
	}
}
