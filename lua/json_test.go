package lua

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestJSONValues(t *testing.T) {
	for _, tc := range []struct{ name, code string }{
		{"scalars", `
   assert(rune.json.encode(nil) == "null")
   assert(rune.json.encode(rune.json.null) == "null")
   assert(rune.json.decode("null") == rune.json.null)
   local v, err = rune.json.decode("false")
   assert(v == false and err == nil)
   assert(rune.json.encode(true) == "true")
   assert(rune.json.decode('"hello"') == "hello")
   assert(rune.json.decode("12.5") == 12.5)
   assert(rune.json.decode("9.007199254740991e15") == 9007199254740991)
   assert(rune.json.encode(-9007199254740991) == "-9007199254740991")
  `},
		{"containers and null", `
   local v, err = rune.json.decode('{"a":[],"o":{},"n":null,"list":[null,false,null],"kind":"array"}')
   assert(err == nil and type(v) == "table")
   assert(v.n == rune.json.null and #v.list == 3)
   assert(v.list[1] == rune.json.null and v.list[2] == false and v.list[3] == rune.json.null)
   assert(rune.json.encode(v.a) == "[]" and rune.json.encode(v.o) == "{}")
   local again = assert(rune.json.decode(assert(rune.json.encode(v))))
   assert(again.n == rune.json.null and again.kind == "array")
   assert(rune.json.encode(again.list) == "[null,false,null]")
  `},
		{"mark and mutate", `
   assert(rune.json.encode({}) == "{}")
   assert(rune.json.encode(rune.json.array()) == "[]")
   assert(rune.json.encode(rune.json.object()) == "{}")
   local a = {1}
   assert(rawequal(rune.json.array(a), a))
   a[1] = nil
   assert(rune.json.encode(a) == "[]")
   assert(rawequal(rune.json.object(a), a))
   assert(rune.json.encode(a) == "{}")
   local decoded = assert(rune.json.decode('[1]'))
   decoded[1] = nil
   assert(rune.json.encode(decoded) == "[]")
   decoded[1] = false
   assert(rune.json.encode(decoded) == "[false]")
   local obj = assert(rune.json.decode('{"1":1}'))
   assert(obj["1"] == 1 and obj[1] == nil)
   obj["1"] = nil
   assert(rune.json.encode(obj) == "{}")
  `},
		{"escaping and Unicode", `
   local text = 'He said "hello"\n\\\t\r\b\f' .. string.char(0) .. 'é😀'
   assert(rune.json.decode(assert(rune.json.encode(text))) == text)
   assert(rune.json.decode('"\\uD83D\\uDE00"') == '😀')
   assert(rune.json.decode('"\\\\uD800"') == '\\uD800')
   assert(rune.json.decode('"\\u0000"') == string.char(0))
  `},
		{"metamethods are not called", `
   local function fail() error("metamethod called") end
   local mt = {__index=fail, __len=fail, __pairs=fail, __tostring=fail, __eq=fail, __newindex=fail}
   local a = setmetatable({1,2}, mt)
   assert(rune.json.array(a) == a and getmetatable(a) == mt)
   assert(rune.json.encode(a) == "[1,2]")
   local o = setmetatable({x=1}, mt)
   assert(rune.json.encode(o) == '{"x":1}')
  `},
		{"container marks do not retain tables", `
   local weak = setmetatable({}, {__mode="v"})
   local function make_array()
    local a = rune.json.array({1})
    weak[1] = a
    assert(rune.json.encode(a) == "[1]")
   end
   make_array()
   collectgarbage("collect")
   collectgarbage("collect")
   assert(weak[1] == nil)
  `},
		{"shared subtrees", `
   local child = {value=1}
   assert(rune.json.encode({child,child}) == '[{"value":1},{"value":1}]')
  `},
		{"userdata null is immutable", `
   assert(type(rune.json.null) == "userdata")
   assert(not pcall(function() rune.json.null.x = 1 end))
   assert(not pcall(function() rune.json.array(rune.json.null) end))
  `},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()
			if err := engine.DoString("json test", tc.code); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSONRejectsUnsupportedValues(t *testing.T) {
	for _, tc := range []struct{ name, expression string }{
		{"function", `function() end`},
		{"thread", `coroutine.create(function() end)`},
		{"userdata", `assert(rune.regex.compile("x"))`},
		{"mixed", `{[1]=1, name="x"}`},
		{"sparse", `{[1]=1, [3]=3}`},
		{"zero key", `{[0]=1}`},
		{"fractional key", `{[1.5]=1}`},
		{"table key", `{[{}]=1}`},
		{"boolean key", `{[false]=1}`},
		{"object with numeric keys", `rune.json.object({1})`},
		{"array with string keys", `rune.json.array({a=1})`},
		{"cycle", `(function() local v={} v.self=v return v end)()`},
		{"nan", `0/0`},
		{"infinity", `math.huge`},
		{"large integer", `9007199254740992`},
		{"invalid UTF8", `string.char(255)`},
		{"invalid UTF8 key", `{[string.char(255)]=1}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, _, cleanup := setupTest(t)
			defer cleanup()
			code := fmt.Sprintf(`local v, err = rune.json.encode(%s); assert(v == nil and type(err) == "string", "expected nil, err"); assert(rune.json.encode({1}) == "[1]")`, tc.expression)
			if err := engine.DoString("json failure", code); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestJSONRejectsMalformedInput(t *testing.T) {
	for _, text := range []string{
		``, `{`, `[`, `{"a":}`, `{"a":1,}`, `[1,]`, `{} []`, `true garbage`, `// comment`,
		`NaN`, `Infinity`, `01`, `1e999`, `9007199254740992`, `9007199254740993`, `-9007199254740993`,
		`{"a":null,"a":1}`, `{"a":1,"\u0061":2}`, `"\uD800"`, `"\uDC00"`, `"\uD800\u1234"`,
		`"\uD800\uD800"`, `"\uD800x"`, "\"raw\nline\"", "\"\xff\"",
	} {
		t.Run(fmt.Sprintf("%q", text), func(t *testing.T) {
			if _, err := decodeJSON(text, scriptJSON); err == nil {
				t.Fatalf("accepted %q", text)
			}
		})
	}
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if err := engine.DoString("json errors", `
  local value, err = rune.json.decode("{")
  assert(value == nil and type(err) == "string")
  for _, bad in ipairs({false, 12, {}, function() end}) do
   assert(not pcall(rune.json.decode, bad))
   assert(not pcall(rune.json.array, bad) or type(bad) == "table")
   assert(not pcall(rune.json.object, bad) or type(bad) == "table")
  end
  assert(not pcall(rune.json.decode))
  assert(rune.json.decode("true") == true)
 `); err != nil {
		t.Fatal(err)
	}
}

func TestJSONLimits(t *testing.T) {
	for _, count := range []int{jsonMaxDepth, jsonMaxDepth + 1} {
		text := strings.Repeat("[", count) + "0" + strings.Repeat("]", count)
		_, err := decodeJSON(text, scriptJSON)
		if (err == nil) != (count == jsonMaxDepth) {
			t.Fatalf("depth %d: %v", count, err)
		}
	}
	for _, size := range []int{jsonMaxBytes, jsonMaxBytes + 1} {
		text := `"` + strings.Repeat("a", size-2) + `"`
		_, err := decodeJSON(text, scriptJSON)
		if (err == nil) != (size == jsonMaxBytes) {
			t.Fatalf("decode size %d: %v", size, err)
		}
		_, err = encodeJSON(text[1:len(text)-1], scriptJSON)
		if (err == nil) != (size == jsonMaxBytes) {
			t.Fatalf("encode size %d: %v", size, err)
		}
	}
	for _, n := range []int{jsonMaxNodes - 1, jsonMaxNodes} {
		text := "[" + strings.Repeat("0,", n-1) + "0]"
		_, err := decodeJSON(text, scriptJSON)
		if (err == nil) != (n == jsonMaxNodes-1) {
			t.Fatalf("values %d: %v", n+1, err)
		}
	}
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if err := engine.DoString("json depth", `
  local text = string.rep("[",64) .. "0" .. string.rep("]",64)
  local v = assert(rune.json.decode(text))
  assert(rune.json.encode(v) == text)
  local bad, err = rune.json.encode({v})
  assert(bad == nil and err:find("container levels"))
  local escaped = string.rep(string.char(0), 900000)
  bad, err = rune.json.encode(escaped)
  assert(bad == nil and err:find("5 MiB"))
  bad, err = rune.json.encode(string.rep("a", 5*1024*1024))
  assert(bad == nil and err:find("5 MiB"))
  assert(rune.json.encode(false) == "false")
 `); err != nil {
		t.Fatal(err)
	}
}

// GMCP and storage share the codec but retain their previous conversion
// policies. In particular, JSON null remains Lua nil in those APIs.
func TestJSONCompatibilityPolicy(t *testing.T) {
	for _, text := range []string{`{"a":1,"a":2}`, `9007199254740993`, `"\uD800"`, "\"\xff\""} {
		got, err := decodeJSON(text, compatibleJSON)
		if err != nil {
			t.Fatal(err)
		}
		var want any
		if err := json.Unmarshal([]byte(text), &want); err != nil {
			t.Fatal(err)
		}
		actual, err := encodeJSON(got, compatibleJSON)
		if err != nil {
			t.Fatal(err)
		}
		expected, _ := json.Marshal(want)
		if actual != string(expected) {
			t.Fatalf("%q: got %q want %q", text, actual, expected)
		}
	}
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if err := engine.DoString("json compatibility", `
  assert(rune.store.set("empty", {}))
  assert(rune.store.set("large", 9007199254740992))
  assert(rune.store.get("large") == 9007199254740992)
  rune.gmcp.on("Test.Data", function(data)
   assert(data.n == nil and data.a == 2)
   assert(data.large == 9007199254740992)
   gmcp_json_seen = true
  end)
 `); err != nil {
		t.Fatal(err)
	}
	engine.OnGMCP("Test.Data", `{"n":null,"a":1,"a":2,"large":9007199254740992}`)
	if err := engine.DoString("check compatibility", `assert(gmcp_json_seen)`); err != nil {
		t.Fatal(err)
	}
}
