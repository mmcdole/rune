package lua

import (
	"strings"
	"testing"
)

// TestInvalidRegexFailsAtRegistration verifies that a bad pattern is a
// loud error at trigger/alias creation, not a trigger that never fires.
func TestInvalidRegexFailsAtRegistration(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	err := engine.DoString("setup", `rune.trigger.regex("(unclosed", "look")`)
	if err == nil || !strings.Contains(err.Error(), "invalid trigger pattern") {
		t.Errorf("expected registration error for bad trigger pattern, got: %v", err)
	}

	err = engine.DoString("setup2", `rune.alias.regex("(unclosed", "look")`)
	if err == nil || !strings.Contains(err.Error(), "invalid alias pattern") {
		t.Errorf("expected registration error for bad alias pattern, got: %v", err)
	}
}

// TestCompiledRegexObject pins the documented compiled-object contract:
// re:match returns the full match at index 1 plus captures, or nil on
// no match. The identity assertion proves method lookup resolves to a
// shared function rather than manufacturing a fresh closure per call.
func TestCompiledRegexObject(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("regex_object", `
		local re = assert(rune.regex.compile("(\\w+) says (\\w+)"))

		local m = re:match("Bob says hi")
		assert(m[1] == "Bob says hi" and m[2] == "Bob" and m[3] == "hi",
			"match: " .. table.concat(m, ","))
		assert(re:match("nothing here") == nil, "non-match must be nil")
		assert(re:pattern() == "(\\w+) says (\\w+)", "pattern: " .. re:pattern())

		assert(re.match == re.match, "method lookup must return a shared function")

		local ok, err = rune.regex.compile("(unclosed")
		assert(ok == nil and err ~= nil, "bad pattern must return nil, err")
	`); err != nil {
		t.Fatal(err)
	}
}
