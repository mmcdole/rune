---
title: rune.json
description: Encode Lua values as JSON and decode JSON for HTTP APIs and session storage.
---

Built-in JSON conversion, available on both Lunar and LuaJIT. No extra
library or `require` is needed.

```lua
rune.json.encode(value)   -- JSON string, or nil, err
rune.json.decode(text)    -- Lua value, or nil, err
rune.json.null            -- JSON null
rune.json.array(table?)   -- mark as an array; creates a table if omitted
rune.json.object(table?)  -- mark as an object; creates a table if omitted
```

## Encoding and decoding

```lua
local text, err = rune.json.encode({name = "Drake", active = true})
if err then
    rune.echo(err)
    return
end

local value, decodeErr = rune.json.decode(text)
if decodeErr then
    rune.echo(decodeErr)
    return
end
rune.echo(value.name)
```

`rune.json.encode` accepts strings, booleans, finite numbers, supported
tables, `nil`, and `rune.json.null`. Both `nil` and `rune.json.null` encode
as `null`. Output is compact JSON; object member order and the original
spelling of decoded numbers are not part of the API contract.

`rune.json.decode` accepts a string containing exactly one JSON value,
with optional surrounding whitespace. Its result may be a table, string,
number, boolean, or `rune.json.null`. Check the error result rather than
using `if not value`: JSON `false` is a successful decode.

## Null and absent values

Assigning `nil` removes a Lua table entry. JSON null is therefore represented
by `rune.json.null`, a single immutable userdata value within the current VM:

```lua
local data = assert(rune.json.decode('{"name":null,"items":[1,null,3]}'))
assert(data.name == rune.json.null)  -- explicitly null
assert(data.missing == nil)         -- no such field
assert(data.items[2] == rune.json.null)
assert(#data.items == 3)
```

The sentinel is truthy. Compare it explicitly when an API permits null:

```lua
if data.name ~= nil and data.name ~= rune.json.null then
    rune.echo(data.name)
end
```

Null entries survive encoding and decoding without disappearing or creating
array holes. Store the encoded string to carry them across `/reload`; the
new VM has its own sentinel.

## Arrays and objects

Lua uses tables for both JSON container types. Without an explicit mark,
contiguous integer keys `1..n` produce an array, string keys produce an
object, and an empty table produces `{}`.

```lua
rune.json.encode({"north", "east"})  -- ["north","east"]
rune.json.encode({name = "Drake"})   -- {"name":"Drake"}
rune.json.encode({})                 -- {}
rune.json.encode(rune.json.array())  -- []
rune.json.encode(rune.json.object()) -- {}
```

`rune.json.array(t)` and `rune.json.object(t)` mark and return the same table.
They leave its entries and metatable alone. Calling either with no argument
(or `nil`) creates a new table. Encoding checks that the current entries fit
the marked kind.

Decoded containers are marked automatically. Their kind survives deleting
the last entry:

```lua
local route = assert(rune.json.decode('["north"]'))
route[1] = nil
assert(rune.json.encode(route) == "[]")
```

Marks belong to the table's identity. A copy made by another library does not
automatically inherit them. Both decoded objects and arrays remain ordinary
Lua tables that work with `pairs`, indexing, and normal mutation. Encoding
uses raw entries and does not invoke user metamethods.

## Errors and limits

Conversion failures return `nil, err`. Passing a non-string to `decode`, or
a non-table other than `nil` to `array` or `object`, raises an argument error.

The converter rejects:

- Cycles, mixed string/numeric keys, sparse arrays, and keys incompatible
  with an explicit array/object mark. Use `rune.json.null` for a deliberate
  null array entry.
- Functions, threads, and userdata other than `rune.json.null`.
- Invalid UTF-8, unpaired Unicode surrogate escapes, duplicate object keys,
  comments, trailing commas, and extra values after the first JSON value.
- NaN, infinity, and integer values outside `-9007199254740991` through
  `9007199254740991` (`±(2^53-1)`). Use strings for large IDs. Other numbers
  use Lua's double-precision floating-point representation; decimal values
  may be rounded. Numeric input that overflows that representation fails.

Each call allows at most 64 nested containers, 100,000 values (including
containers and nulls, excluding object keys), and 5 MiB of JSON text. The
text limit applies to both decode input and encode output, including escape
sequences. No partial result is returned on failure.

## HTTP and storage

Use `encode` for a request body and `decode` for a JSON response body. Set
`Content-Type: application/json` yourself. The [HTTP reference](/reference/api/http/#json)
shows the complete flow, including transport and status checks.

`rune.session` accepts strings, so encode before storing structured state
and decode after retrieving it. See [session storage](/scripting/storage/#session-store).

`rune.store` and GMCP already convert ordinary tables and retain their
existing behavior: nulls become Lua nil, and empty arrays and objects become
plain tables. They do not preserve JSON container marks or accept the new
null sentinel as an ordinary value. Use an encoded string when exact JSON
structure must be retained in storage, or `rune.gmcp.send_raw` when sending
pre-encoded GMCP data.
