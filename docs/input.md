# Input Bar Behavior Specification

This document describes the input bar features for a MUD client.

## Features

### 1. History Navigation

**Purpose**: Recall previously entered commands.

**Behavior**:
- **Up arrow**: Show previous command from history
- **Down arrow**: Show next command (or return to current input)
- **Prefix filtering**: If you've typed "get", Up only shows history entries starting with "get"

**Visual**: Replaces input text entirely when navigating.

---

### 2. Inline Suggestion (Ghost Text)

**Purpose**: Predictive completion shown as you type.

**Visual**: Dim/gray text appears after your cursor showing the suggested completion.

```
> get sw         ← what you typed
> get sword from chest
        ^^^^^^^^^^^^^^^^ dim gray text (suggestion)
```

**Accept**: Press Tab or Right arrow to accept the suggestion.

**Source options**:
- Command history (commands you've typed before)
- Word cache (words seen in server output)
- Combined/prioritized

---

### 3. Word Cache

**Purpose**: Remember words from server output for completion.

When the server sends "A goblin attacks you!", the words "goblin" and "attacks" are cached. Later, typing "gob" could suggest "goblin".

**Question**: Should this be separate from history suggestions, or combined into one suggestion?

---

## Key Behaviors

| Key | Action |
|-----|--------|
| **Enter** | Submit command |
| **Up** | Previous history entry (prefix-filtered if text entered) |
| **Down** | Next history entry |
| **Tab** | Accept suggestion (if visible) |
| **Right arrow** | Accept suggestion (if visible and cursor at end), otherwise move cursor |
| **Ctrl+R** | Open history picker (fuzzy search) |

---

## Design Questions

### Question 1: One suggestion or two?

**Option A: Unified suggestion**
- One suggestion shown (from best source: history or word cache)
- Tab and Right both accept it
- Simpler

**Option B: Separate mechanisms**
- Ghost text from history (Right arrow accepts)
- Tab completion from word cache (Tab accepts)
- More complex, but distinct purposes

### Question 2: Word-level vs command-level completion

**Word-level**: Complete the current word only
- Type "get gob" → Tab → "get goblin" (just the word)

**Command-level**: Complete the entire command
- Type "get sw" → shows "get sword from chest" (full command from history)

**Combined**: Show command-level ghost, but Tab could complete just the current word?

### Question 3: Where does logic live?

**Option A: Go handles everything**
- Fast, but not customizable

**Option B: Lua handles logic**
- Lua decides what to suggest based on input
- Users can customize behavior
- Go just renders and accepts

---

## Example Scenarios

### Scenario 1: Repeating a command
```
History contains: "kill goblin", "get sword", "north"

User types: k
Suggestion shown: kill goblin (dim)
User presses: Right arrow
Result: "kill goblin" is filled in
```

### Scenario 2: Completing a word from server output
```
Server previously sent: "A menacing orc appears!"
Word cache contains: "menacing", "orc", "appears"

User types: kill m
Suggestion shown: menacing (dim, completing the word)
User presses: Tab
Result: "kill menacing" is filled in
```

### Scenario 3: History with prefix filtering
```
History: "north", "kill goblin", "north", "get sword"

User types: n
User presses: Up
Result: "north" (most recent starting with "n")
User presses: Up again
Result: "north" (the earlier one)
```

---

## Open Questions

1. Should Tab and Right arrow do the same thing, or different things?
2. Should word completion and history completion be visually distinct?
3. Should suggestions be automatic, or only appear on a keypress?
4. What's the priority when multiple sources match? (history vs word cache)
