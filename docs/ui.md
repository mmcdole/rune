# UI Enhancement Plan

## Input Bar Features

### Current State
- **Tab completion**: Word-level completion from word cache (server output + user input)
- **Picker**: Fuzzy-filter selection (Ctrl+R for history, Ctrl+T for aliases)
- **History navigation**: Up/down arrows with prefix matching
- **Basic editing**: Ctrl+W delete word, Ctrl+U clear line

### Planned Enhancements

#### 1. Word Navigation
**Status**: Implemented (Go-side)

- **Alt+Left / Alt+B**: Jump to previous word boundary
- **Alt+Right / Alt+F**: Jump to next word boundary
- **Ctrl+Left / Ctrl+Right**: Same as Alt+arrows (terminal compatibility)

**Files**: `ui/tui/widget/input.go`, `ui/tui/model.go`

---

#### 2. Ghost Suggestions (Fish-style)
**Status**: In progress - architectural decision needed

Show predictive completion as dim ghost text. Right arrow accepts.

```
> get sw[ord from chest]  ← ghost text from history
```

**Approach A: All in Go**
- History provider callback from session to UI
- Go does prefix matching internally
- Fast but inflexible

**Approach B: Go primitive + Lua logic (preferred)**
- Go exposes: `rune.input.set_suggestion(text)`
- Lua handles logic: what to suggest, when to show
- Users can customize behavior

**Go primitive needed**:
```go
// In api_input.go
rune.input.set_suggestion(text)  // Set ghost text (nil to clear)
```

**Lua implementation** (in core/55_suggestions.lua):
```lua
rune.hooks.on("input_changed", function(text)
    if text == "" then
        rune.input.set_suggestion(nil)
        return
    end

    local history = rune.history.get()
    for i = #history, 1, -1 do
        if history[i]:sub(1, #text) == text and history[i] ~= text then
            rune.input.set_suggestion(history[i])
            return
        end
    end
    rune.input.set_suggestion(nil)
end)
```

**User customization examples**:
- Show alias expansions as ghost instead of history
- Combine history + alias suggestions
- Disable entirely

---

#### 3. EDITOR Mode
**Status**: Planned

Press key to open input in $EDITOR, edit multi-line, save & quit returns content.

**Keybinding**: Ctrl+E (or Ctrl+X Ctrl+E for bash compatibility)

**Behavior**:
1. Suspend TUI
2. Write current input to temp file
3. Open $EDITOR (fallback: vim, nano)
4. On save+quit, read file contents
5. Resume TUI, set input to file contents
6. Multi-line: join with `;` or send each line

**Go implementation** (in tui.go):
```go
func (b *BubbleTeaUI) OpenEditor(currentInput string) (string, error) {
    f, _ := os.CreateTemp("", "rune-input-*.txt")
    f.WriteString(currentInput)
    f.Close()

    b.program.ReleaseTerminal()

    editor := os.Getenv("EDITOR")
    if editor == "" {
        editor = "vim"
    }
    cmd := exec.Command(editor, f.Name())
    cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
    cmd.Run()

    b.program.RestoreTerminal()

    content, _ := os.ReadFile(f.Name())
    os.Remove(f.Name())
    return strings.TrimSpace(string(content)), nil
}
```

---

#### 4. Undo/Redo
**Status**: Deferred

- **Ctrl+Z**: Undo last edit
- **Ctrl+Shift+Z / Ctrl+Y**: Redo

Would require undo stack in input widget tracking (value, cursor) pairs.

---

## Separation of Concerns

### Ghost (history) vs Tab (word cache)

| Feature | Source | Accept Key | Purpose |
|---------|--------|------------|---------|
| Ghost suggestion | Command history | Right arrow | Recall full commands |
| Tab completion | Word cache | Tab | Discover/complete words |

**Ghost** = "I've typed this before" (recall)
**Tab** = "What are my options here?" (discovery)

---

## New Lua API

### Input
```lua
rune.input.get()                    -- Get current input text (exists)
rune.input.set(text)                -- Set input text (exists)
rune.input.set_suggestion(text)     -- Set ghost suggestion (NEW)
rune.input.clear_suggestion()       -- Clear ghost suggestion (NEW)
```

### Hooks
```lua
-- Existing
rune.hooks.on("input", fn)          -- Fires on Enter (submission)

-- Needed for ghost suggestions
rune.hooks.on("input_changed", fn)  -- Fires on every keystroke (NEW?)
```

**Note**: `input_changed` hook may already exist via InputChangedMsg - need to verify.

---

## Implementation Order

1. **Word navigation** - Done (Go-side)
2. **Ghost suggestions primitive** - Add `rune.input.set_suggestion()`
3. **Ghost suggestions logic** - Lua script for history matching
4. **EDITOR mode** - TUI suspend/resume with $EDITOR

---

## Files to Modify

### For Ghost Suggestions
- `lua/api_input.go` - Add set_suggestion binding
- `ui/tui/widget/input.go` - Method to set suggestion text
- `ui/interface.go` - Add SetSuggestion to UI interface
- `ui/tui/tui.go` - Implement SetSuggestion
- `lua/core/55_suggestions.lua` - Default history-based suggestion logic

### For EDITOR Mode
- `ui/tui/tui.go` - OpenEditor method
- `ui/tui/model.go` - Handle Ctrl+E key
- Or: Lua binding that calls Go primitive
