package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/mmcdole/rune/ui"
)

// ShowSearch opens the scrollback-search overlay. Unlike the picker
// there is no callback to settle, so refusing while a structured draft
// is active is a plain no-op. An open picker settles first: overlays
// are mutually exclusive and the newcomer wins.
func (c *inputController) ShowSearch(opts ui.ShowSearchMsg) {
	if c.input.IsComposing() {
		return
	}
	if c.mode() == modePickerModal || c.mode() == modePickerInline {
		c.closePicker(false, "")
	}
	if c.mode() != modeSearch {
		scope := c.search.OpenSearch()
		c.input.ShowSearch(opts.Query, scope)
	} else {
		c.input.Search().Reopen(opts.Query)
	}
	c.previewSearch()
}

// handleSearchKey traps all keys while the search overlay is open,
// like the modal picker: bound keys do not dispatch to Lua.
func (c *inputController) handleSearchKey(msg tea.KeyPressMsg) {
	if info, ok := numpadNavigation(msg); ok {
		msg = info.navigationFallback(msg)
	}
	switch {
	case matchesKey(msg, tea.KeyUp, 0):
		c.selectOlderSearch()

	case matchesKey(msg, tea.KeyDown, 0):
		c.selectNewerSearch()

	case matchesEnterKey(msg, 0):
		c.closeSearch(true)

	case matchesKey(msg, tea.KeyBackspace, 0):
		c.input.Search().Backspace()
		c.previewSearch()

	default:
		if msg.Text != "" {
			c.input.Search().TypeRunes([]rune(msg.Text))
			c.previewSearch()
		}
	}
}

// selectOlderSearch and selectNewerSearch are the semantic navigation seam
// shared by keyboard and mouse input. Device handlers do not need to forge a
// different device's event or re-enter the full key dispatcher.
func (c *inputController) selectOlderSearch() bool {
	if c.mode() != modeSearch {
		return false
	}
	c.input.Search().SelectOlder()
	c.previewSearch()
	return true
}

func (c *inputController) selectNewerSearch() bool {
	if c.mode() != modeSearch {
		return false
	}
	c.input.Search().SelectNewer()
	c.previewSearch()
	return true
}

// previewSearch centers the viewport on the current selection (live
// preview); with no match it restores the pre-search position so a
// query edit that empties the result set snaps back.
func (c *inputController) previewSearch() {
	m, ok := c.input.Search().Selected()
	c.search.PreviewSearch(m, ok)
}

// closeSearch is the single exit path from search mode: resets the
// mode, hides the overlay, and settles the viewport exactly once -
// committed (stay at the match) or cancelled (restore the snapshot).
// The final ScrollStateChangedMsg emitted by the effects is search's
// analog of the picker's settle message: it keeps the session's
// rune.state scroll view fresh.
func (c *inputController) closeSearch(accepted bool) {
	c.input.HideSearch()
	if accepted {
		c.search.CommitSearch()
	} else {
		c.search.CancelSearch()
	}
}
