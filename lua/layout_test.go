package lua

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mmcdole/rune/ui"
)

func intPtr(n int) *int          { return &n }
func stringPtr(s string) *string { return &s }

func outputPane() ui.LayoutNode {
	return ui.LayoutNode{
		Type: ui.LayoutTypePane, Name: ui.OutputPaneName,
		Border: ui.PaneBorderNone,
	}
}

func TestLegacyLayoutTableIsRejectedWithoutRaising(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	baseline := engine.GetLayout()
	host.DrainPresentationChanges()
	host.DrainPrintCalls()

	cases := []struct{ name, body string }{
		{name: "top and bottom", body: `{ top = { { name = "chat", height = 10 } }, bottom = { "input", "status" } }`},
		{name: "bottom only", body: `{ bottom = { "input" } }`},
		{name: "explicit version", body: `{ version = 1, top = { "chat" } }`},
		{name: "empty table", body: `{}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host.DrainPrintCalls()
			// A rejected table must not abort the rest of the script that
			// carried it, so the marker after the call still runs.
			if err := engine.DoString("legacy layout", `
				legacy_marker = nil
				legacy_result = rune.ui.layout(`+c.body+`)
				legacy_marker = true
			`); err != nil {
				t.Fatalf("legacy layout raised: %v", err)
			}
			assertLua(t, engine, `assert(legacy_marker == true, "script did not continue after the rejected layout")`)
			assertLua(t, engine, `assert(legacy_result == false, "rune.ui.layout must return false for a rejected table")`)
			if got := engine.GetLayout(); !reflect.DeepEqual(got, baseline) {
				t.Fatalf("rejected layout replaced the active tree: %#v", got)
			}
			if n := host.DrainPresentationChanges(); n != 0 {
				t.Fatalf("rejected layout published %d presentation changes, want none", n)
			}
			printed := strings.Join(host.DrainPrintCalls(), "\n")
			for _, want := range []string{
				"rune.ui.layout: top/bottom layout tables are no longer supported",
				"legacy layout:",
				"Pass a root node",
				"#migrating-from-topbottom-tables",
			} {
				if !strings.Contains(printed, want) {
					t.Fatalf("notice missing %q:\n%s", want, printed)
				}
			}
		})
	}

	assertLua(t, engine, `
		assert(rune.ui.layout({ type = "column", children = {
			{ type = "pane", name = "output" }, { type = "input" },
		} }) == true, "a valid tree must return true")
	`)
}

func TestLayoutParsesDirectStrictTreeWithFlatLeafFields(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("tree layout", `
		rune.ui.layout({
			type = "column",
			gap = 1,
			children = {
				{
					type = "row",
					size = "2fr",
					dividers = true,
					children = {
						{ type = "pane", name = "output", size = "2fr", border = "none" },
						{
							type = "column", id = "sidebar", hidden = true,
							size = "30%", min_size = 20, max_size = 40,
							children = {
								{ type = "pane", name = "chat", title = "Chat", border = "horizontal" },
								{ type = "bar", name = "vitals", size = 1 },
							},
						},
					},
				},
				{ type = "separator", char = "=", size = 1 },
				{ type = "input", size = "auto", max_size = 5 },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}

	want := ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Gap:  1,
		Children: []ui.LayoutNode{
			{
				Type:     ui.LayoutTypeRow,
				Size:     ui.Fraction(2),
				Dividers: true,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Fraction(2), Border: ui.PaneBorderNone},
					{
						Type: ui.LayoutTypeColumn, ID: "sidebar", Hidden: true,
						Size: ui.Percent(30), MinSize: intPtr(20), MaxSize: intPtr(40),
						Children: []ui.LayoutNode{
							{Type: ui.LayoutTypePane, Name: "chat", Title: stringPtr("Chat"), Border: ui.PaneBorderHorizontal},
							{Type: ui.LayoutTypeBar, Name: "vitals", Size: ui.Cells(1)},
						},
					},
				},
			},
			{Type: ui.LayoutTypeSeparator, Size: ui.Cells(1), SeparatorChar: "="},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize(), MaxSize: intPtr(5)},
		},
	}}
	want, err := ui.NormalizeLayoutTree(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.GetLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed native tree =\n%#v\nwant\n%#v", got, want)
	}
}

func TestLayoutAllowsNoOutputAndNamespacesResourceNames(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("input-only layout", `rune.ui.layout({ type = "input" })`); err != nil {
		t.Fatalf("input-only tree layout: %v", err)
	}
	if got, want := engine.GetLayout(), (ui.LayoutTree{Root: ui.LayoutNode{Type: ui.LayoutTypeInput}}); !reflect.DeepEqual(got, want) {
		t.Fatalf("input-only layout = %#v, want %#v", got, want)
	}

	if err := engine.DoString("cross-namespace names", `
		rune.ui.layout({
			type = "column",
			children = {
				{ type = "pane", name = "shared" },
				{ type = "bar", name = "shared" },
				{ type = "input" },
			},
		})
	`); err != nil {
		t.Fatalf("pane and bar sharing a name: %v", err)
	}
}

func TestLayoutNormalizesIntrinsicLeafDefaults(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("intrinsic defaults", `
		rune.ui.layout({
			type = "column",
			children = {
				{ type = "pane", name = "output" },
				{ type = "separator" },
				{ type = "bar", name = "status" },
				{ type = "input" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}

	want := ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{Type: ui.LayoutTypePane, Name: ui.OutputPaneName},
			{Type: ui.LayoutTypeSeparator, Size: ui.AutoSize()},
			{Type: ui.LayoutTypeBar, Name: "status", Size: ui.AutoSize()},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize()},
		},
	}}
	want, err := ui.NormalizeLayoutTree(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.GetLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("intrinsic defaults =\n%#v\nwant\n%#v", got, want)
	}
}

func TestLayoutAcceptsEmptySingletonAndUnsizedRowLeaves(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()
	if err := engine.DoString("composable containers", `
		rune.ui.layout({ type = "column", children = {
			{ type = "column", id = "empty", children = {} },
			{ type = "column", id = "single", children = {
				{ type = "pane", name = "output" },
			} },
			{ type = "row", children = {
				{ type = "input" },
				{ type = "bar", name = "status" },
				{ type = "separator" },
			} },
		} })
		assert(rune.ui.regions.is_hidden("empty") == false)
		assert(rune.ui.regions.hide("empty"))
		assert(rune.ui.regions.is_hidden("empty") == true)
	`); err != nil {
		t.Fatal(err)
	}
	for _, child := range engine.GetLayout().Root.Children[2].Children {
		if child.Size != ui.Fraction(1) {
			t.Errorf("unsized row child %s = %v, want 1fr", child.Type, child.Size)
		}
	}
}

func TestLayoutParsesConciseMudClientExample(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("concise layout", `
		rune.ui.layout({
			type = "column",
			children = {
				{
					type = "row",
					children = {
						{ type = "pane", name = "output", size = "3fr", border = "none" },
						{
							type = "pane", name = "chat", size = "1fr",
							min_size = 24, title = "Chat", border = "full",
						},
					},
				},
				{ type = "separator" },
				{ type = "input", max_size = 5 },
				{ type = "bar", name = "status" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}

	want := ui.LayoutTree{Root: ui.LayoutNode{
		Type: ui.LayoutTypeColumn,
		Children: []ui.LayoutNode{
			{
				Type: ui.LayoutTypeRow,
				Children: []ui.LayoutNode{
					{Type: ui.LayoutTypePane, Name: ui.OutputPaneName, Size: ui.Fraction(3), Border: ui.PaneBorderNone},
					{Type: ui.LayoutTypePane, Name: "chat", Size: ui.Fraction(1), MinSize: intPtr(24), Title: stringPtr("Chat"), Border: ui.PaneBorderFull},
				},
			},
			{Type: ui.LayoutTypeSeparator, Size: ui.AutoSize()},
			{Type: ui.LayoutTypeInput, Size: ui.AutoSize(), MaxSize: intPtr(5)},
			{Type: ui.LayoutTypeBar, Name: "status", Size: ui.AutoSize()},
		},
	}}
	want, err := ui.NormalizeLayoutTree(want)
	if err != nil {
		t.Fatal(err)
	}
	if got := engine.GetLayout(); !reflect.DeepEqual(got, want) {
		t.Fatalf("concise layout =\n%#v\nwant\n%#v", got, want)
	}
}

func TestLayoutValidationIsStrictAndAtomic(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("baseline", `
		rune.ui.layout({
			type = "column",
			children = {
				{ type = "pane", name = "output", border = "none" },
				{ type = "input", size = "auto" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}
	baseline := engine.GetLayout()
	host.DrainPresentationChanges()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "native version wrapper", body: `{ type="column", version=2, children={{type="pane",name="output"},{type="input"}} }`, want: `unknown field "version"`},
		{name: "native root wrapper", body: `{ type="column", root={type="input"}, children={{type="pane",name="output"},{type="input"}} }`, want: `unknown field "root"`},
		{name: "missing type", body: `{ typo = 1 }`, want: "root.type must be a non-empty string"},
		{name: "empty type", body: `{ type = "" }`, want: "non-empty string"},
		{name: "unknown main type", body: `{ type = "main" }`, want: `unknown type "main"`},
		{name: "output shorthand", body: `{ type = "output" }`, want: `unknown type "output"`},
		{name: "unknown component type", body: `{ type = "component", name = "status" }`, want: `unknown type "component"`},
		{name: "unknown node field", body: `{ type="input", typo=1 }`, want: `unknown field "typo"`},
		{name: "name on input", body: `{ type="input", name="composer" }`, want: `unknown field "name"`},
		{name: "pane missing name", body: `{ type="column", children={{type="pane"},{type="input"}} }`, want: "pane requires a non-empty name"},
		{name: "pane empty name", body: `{ type="column", children={{type="pane",name=""},{type="input"}} }`, want: "name must be a non-empty string"},
		{name: "pane non-string name", body: `{ type="column", children={{type="pane",name=1},{type="input"}} }`, want: "name must be a non-empty string"},
		{name: "bar missing name", body: `{ type="column", children={{type="bar"},{type="input"}} }`, want: "bar requires a non-empty name"},
		{name: "root id", body: `{ type="column",id="screen",children={{type="pane",name="output"},{type="input"}} }`, want: "root.id is not allowed"},
		{name: "root size", body: `{ type="input",size="1fr" }`, want: "root.size is not allowed"},
		{name: "root hidden", body: `{ type="column",hidden=false,children={{type="pane",name="output"},{type="input"}} }`, want: "root.hidden is not allowed"},
		{name: "leaf children", body: `{ type="pane",name="output",children={} }`, want: `unknown field "children"`},
		{name: "container options table", body: `{ type="column",options={},children={{type="pane",name="output"},{type="input"}} }`, want: `unknown field "options"`},
		{name: "leaf options table", body: `{ type="column",children={{type="pane",name="output",options={}},{type="input"}} }`, want: `unknown field "options"`},
		{name: "leaf gap", body: `{ type="column",children={{type="pane",name="output",gap=1},{type="input"}} }`, want: `unknown field "gap"`},
		{name: "leaf dividers", body: `{ type="column",children={{type="pane",name="output",dividers=true},{type="input"}} }`, want: `unknown field "dividers"`},
		{name: "non-boolean dividers", body: `{ type="column",dividers=1,children={{type="pane",name="output"},{type="input"}} }`, want: "dividers must be a boolean"},
		{name: "zero fixed size", body: `{ type="column",children={{type="pane",name="output"},{type="input",size=0}} }`, want: "between 1"},
		{name: "fractional fixed size", body: `{ type="column",children={{type="pane",name="output"},{type="input",size=1.5}} }`, want: "finite integer"},
		{name: "numeric size string", body: `{ type="column",children={{type="pane",name="output"},{type="input",size="5"}} }`, want: "expected auto, Nfr, or P%"},
		{name: "bad fraction", body: `{ type="column",children={{type="pane",name="output"},{type="input",size="0fr"}} }`, want: "between 1"},
		{name: "bad percent", body: `{ type="column",children={{type="pane",name="output"},{type="input",size="101%"}} }`, want: "between 1 and 100"},
		{name: "auto width leaf", body: `{ type="row",children={{type="pane",name="output",size="auto"},{type="input"}} }`, want: "intrinsic width is not supported"},
		{name: "auto width container", body: `{ type="row",children={{type="column",size="auto",children={{type="pane",name="output"},{type="bar",name="status"}}},{type="input"}} }`, want: "intrinsic width is not supported"},
		{name: "zero maximum", body: `{ type="column",children={{type="pane",name="output"},{type="input",max_size=0}} }`, want: "max_size must be between 1"},
		{name: "min above max", body: `{ type="column",children={{type="pane",name="output"},{type="input",min_size=4,max_size=3}} }`, want: "min_size"},
		{name: "hidden wrong type", body: `{ type="column",children={{type="row",id="panel",hidden="yes",children={{type="pane",name="output"},{type="bar",name="status"}}},{type="input"}} }`, want: "hidden must be a boolean"},
		{name: "hidden anonymous region", body: `{ type="column",children={{type="row",hidden=true,children={{type="pane",name="output"},{type="bar",name="status"}}},{type="input"}} }`, want: "hidden requires an id"},
		{name: "hidden pane wrong type", body: `{ type="column",children={{type="pane",name="output",hidden="yes"},{type="input"}} }`, want: "hidden must be a boolean"},
		{name: "hidden bar leaf", body: `{ type="column",children={{type="pane",name="output"},{type="bar",name="status",hidden=true},{type="input"}} }`, want: `unknown field "hidden"`},
		{name: "hidden input leaf", body: `{ type="column",children={{type="pane",name="output"},{type="input",hidden=true}} }`, want: `unknown field "hidden"`},
		{name: "id on leaf", body: `{ type="column",children={{type="pane",name="output",id="leaf"},{type="input"}} }`, want: `unknown field "id"`},
		{name: "duplicate id", body: `{ type="column",children={{type="row",id="same",children={{type="pane",name="one"},{type="bar",name="one",size=1}}},{type="row",id="same",children={{type="pane",name="two"},{type="bar",name="two",size=1}}},{type="input"}} }`, want: "duplicate id"},
		{name: "hidden region contains input", body: `{ type="column",children={{type="row",id="composer",hidden=true,children={{type="pane",name="output"},{type="input",size="1fr"}}},{type="bar",name="status"}} }`, want: `region "composer" contains input and cannot be hidden`},
		{name: "duplicate pane name", body: `{ type="column",children={{type="pane",name="chat"},{type="pane",name="chat"},{type="input"}} }`, want: `duplicate pane name "chat"`},
		{name: "duplicate bar name", body: `{ type="column",children={{type="bar",name="status"},{type="bar",name="status"},{type="input"}} }`, want: `duplicate bar name "status"`},
		{name: "pane title type", body: `{ type="column",children={{type="pane",name="output",title=1},{type="input"}} }`, want: "title must be a string"},
		{name: "pane border boolean", body: `{ type="column",children={{type="pane",name="output",border=false},{type="input"}} }`, want: "border must be"},
		{name: "pane border value", body: `{ type="column",children={{type="pane",name="output",border="rounded"},{type="input"}} }`, want: "border must be"},
		{name: "bar title", body: `{ type="column",children={{type="bar",name="status",title="Status"},{type="input"}} }`, want: `unknown field "title"`},
		{name: "separator char type", body: `{ type="column",children={{type="separator",char=1},{type="input"}} }`, want: "char must be a string"},
		{name: "separator char width", body: `{ type="column",children={{type="separator",char="wide"},{type="input"}} }`, want: "exactly one terminal cell"},
		{name: "missing input", body: `{ type="row",children={{type="pane",name="output"},{type="bar",name="status",size="1fr"}} }`, want: "exactly one input"},
		{name: "duplicate input", body: `{ type="column",children={{type="pane",name="output"},{type="input"},{type="input"}} }`, want: "exactly one input"},
		{name: "children not dense", body: `{ type="column",children={[1]={type="pane",name="output"},[3]={type="input"}} }`, want: "dense array"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := engine.DoString("invalid layout", "rune.ui.layout("+test.body+")")
			if err == nil {
				t.Fatal("invalid layout was accepted")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error %q does not contain %q", err, test.want)
			}
			if got := engine.GetLayout(); !reflect.DeepEqual(got, baseline) {
				t.Fatalf("invalid replacement mutated layout:\n got  %#v\n want %#v", got, baseline)
			}
			if got := host.DrainPresentationChanges(); got != 0 {
				t.Fatalf("invalid replacement published %d presentation changes", got)
			}
		})
	}
}

func TestLayoutRejectsCyclicNodeTableAtomically(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()
	baseline := engine.GetLayout()
	host.DrainPresentationChanges()

	err := engine.DoString("cyclic layout", `
		local root = { type = "column" }
		root.children = { { type = "pane", name = "output" }, root, { type = "input" } }
		rune.ui.layout(root)
	`)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cyclic layout error = %v, want cycle error", err)
	}
	if got := engine.GetLayout(); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("cyclic layout mutated state: got %#v, want %#v", got, baseline)
	}
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("cyclic layout published %d presentation changes", got)
	}
}

func TestRegionVisibilityPreservesPriorSnapshotsAndPublishesChanges(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("region layout", `
		rune.ui.layout({
			type = "column",
			children = {
				{
					type = "row", id = "workspace", hidden = true,
					children = {
						{ type = "pane", name = "output", border = "none" },
						{ type = "pane", name = "chat" },
					},
				},
				{ type = "input" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}
	host.DrainPresentationChanges()
	before := engine.GetLayout()

	assertLua(t, engine, `
		assert(rune.ui.regions.is_hidden("workspace") == true)
		assert(rune.ui.regions.is_hidden("missing") == nil)
	`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("visibility queries published %d presentation changes", got)
	}

	assertLua(t, engine, `assert(rune.ui.regions.show("workspace") == true)`)
	if got := host.DrainPresentationChanges(); got != 1 {
		t.Fatalf("show published %d presentation changes, want 1", got)
	}
	if !before.Root.Children[0].Hidden {
		t.Fatal("region show mutated the previously returned layout snapshot")
	}
	if got := engine.GetLayout().Root.Children[0].Hidden; got {
		t.Fatal("region remained hidden after show")
	}

	assertLua(t, engine, `assert(rune.ui.regions.show("workspace") == true)`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("idempotent show published %d presentation changes", got)
	}
	assertLua(t, engine, `
		assert(rune.ui.regions.hide("missing") == false)
		assert(rune.ui.regions.toggle("missing") == false)
	`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("unknown region operations published %d presentation changes", got)
	}

	assertLua(t, engine, `assert(rune.ui.regions.toggle("workspace") == true)`)
	if got := host.DrainPresentationChanges(); got != 1 {
		t.Fatalf("toggle published %d presentation changes, want 1", got)
	}
	if got := engine.GetLayout().Root.Children[0].Hidden; !got {
		t.Fatal("toggle did not hide the region")
	}

	assertLua(t, engine, `assert(rune.ui.regions.hide("workspace") == true)`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("idempotent hide published %d presentation changes", got)
	}
}

func TestRegionContainingInputCanBeIdentifiedButNotHidden(t *testing.T) {
	engine, host, cleanup := setupTest(t)
	defer cleanup()

	if err := engine.DoString("region layout", `
		rune.ui.layout({
			type = "column",
			children = {
				{
					type = "row", id = "workspace",
					children = {
						{ type = "pane", name = "chat", size = 20 },
						{
							type = "column", id = "io",
							children = {
								{ type = "pane", name = "output", border = "none" },
								{ type = "input" },
							},
						},
					},
				},
				{ type = "bar", name = "status" },
			},
		})
	`); err != nil {
		t.Fatal(err)
	}
	host.DrainPresentationChanges()
	before := engine.GetLayout()

	assertLua(t, engine, `
		assert(rune.ui.regions.is_hidden("io") == false)
		assert(rune.ui.regions.is_hidden("workspace") == false)
		assert(rune.ui.regions.show("io") == true)
		assert(rune.ui.regions.show("workspace") == true)
		for _, id in ipairs({ "io", "workspace" }) do
			for _, op in ipairs({ "hide", "toggle" }) do
				local ok, err = rune.ui.regions[op](id)
				assert(ok == nil, op .. " " .. id .. " returned " .. tostring(ok))
				assert(err == "rune.ui.regions." .. op .. ": region \"" .. id .. "\" contains input and cannot be hidden", err)
			end
		end
		assert(rune.ui.regions.is_hidden("io") == false)
		assert(rune.ui.regions.is_hidden("workspace") == false)
	`)
	if got := host.DrainPresentationChanges(); got != 0 {
		t.Fatalf("refused region operations published %d presentation changes", got)
	}
	if got := engine.GetLayout(); !reflect.DeepEqual(got, before) {
		t.Fatalf("refused region operations mutated layout:\n got  %#v\n want %#v", got, before)
	}
}

func TestRegionVisibilityAPIRejectsInvalidIDs(t *testing.T) {
	engine, _, cleanup := setupTest(t)
	defer cleanup()

	for _, call := range []string{
		`rune.ui.regions.show()`,
		`rune.ui.regions.hide(1)`,
		`rune.ui.regions.toggle("")`,
		`rune.ui.regions.is_hidden(false)`,
	} {
		err := engine.DoString("invalid region id", call)
		if err == nil || !strings.Contains(err.Error(), "id must be a non-empty string") {
			t.Fatalf("%s error = %v, want non-empty string error", call, err)
		}
	}
}
