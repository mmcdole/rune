package input

import "testing"

func TestRequiresStructuredEditor(t *testing.T) {
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
			if got := RequiresStructuredEditor(tt.value); got != tt.want {
				t.Fatalf("RequiresStructuredEditor(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestValidCommandTextRejectsInvalidUTF8(t *testing.T) {
	if !ValidCommandText("look") {
		t.Fatal("ordinary command text was rejected")
	}
	if ValidCommandText("one\ntwo") {
		t.Fatal("structured command text was accepted")
	}
	if ValidCommandText(string([]byte{'l', 0xff, 'k'})) {
		t.Fatal("invalid UTF-8 command text was accepted")
	}
}

func TestCommandAdmissionAllowsStructuredArgumentsOnlyForLocalCommands(t *testing.T) {
	for _, tc := range []struct {
		text  string
		valid bool
	}{
		{"/lua -- comment\n\trune.echo('hello')", true},
		{"/lua\r\nrune.echo('hello')", true},
		{"/echo first\n/quit", true},
		{"look\neast", false},
		{"say\thello", false},
		{"/\nquit", false},
		{" /lua\nprint('hello')", false},
		{"/lua print('\x1b')", false},
		{"/lua --\u202e\nprint('hello')", false},
	} {
		if got := ValidCommandText(tc.text); got != tc.valid {
			t.Errorf("ValidCommandText(%q) = %v", tc.text, got)
		}
	}
}
