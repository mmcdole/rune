package input

import "testing"

func TestRequiresVerbatim(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "empty", value: "", want: false},
		{name: "plain command", value: "say hello;look", want: false},
		{name: "ordinary unicode", value: "café e\u0301 👩‍💻", want: false},
		{name: "carriage return", value: "one\rtwo", want: true},
		{name: "line feed", value: "one\ntwo", want: true},
		{name: "tab", value: "one\ttwo", want: true},
		{name: "C0 control", value: "one\x1btwo", want: true},
		{name: "C1 control", value: "one\u0085two", want: true},
		{name: "line separator", value: "one\u2028two", want: true},
		{name: "paragraph separator", value: "one\u2029two", want: true},
		{name: "bidi mark", value: "one\u200ftwo", want: true},
		{name: "bidi override", value: "one\u202etwo", want: true},
		{name: "bidi isolate", value: "one\u2067two", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequiresVerbatim(tt.value); got != tt.want {
				t.Fatalf("RequiresVerbatim(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
