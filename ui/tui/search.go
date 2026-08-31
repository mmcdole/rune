package tui

import (
	"github.com/mmcdole/rune/ui"
	"github.com/mmcdole/rune/ui/tui/widget"
)

// searchViewState is Model's viewport-side state for one search lifecycle.
// Search itself owns query/results; the controller owns interaction mode.
type searchViewState struct {
	snapshot   widget.ScrollPos
	focus      *widget.SearchMatch
	priorFocus *widget.SearchMatch
}

// Model implements searchEffects: the viewport half of scrollback
// search. The controller drives the mode; these methods move the
// viewport. Every position change reports the resulting scroll state
// so the session's rune.state view cannot go stale - the settle
// message of a search is its final ScrollStateChangedMsg.

// OpenSearch snapshots the viewport and committed highlight for cancel, then
// returns the temporal origin the Search widget should scan around.
func (m *Model) OpenSearch() widget.SearchScope {
	m.searchView.snapshot = m.output.viewport.SaveScroll()
	m.searchView.priorFocus = m.searchView.focus
	m.notifySession(ui.SearchStateChangedMsg(true))

	scope := widget.SearchScope{}
	if m.output.buffer.Count() > 0 {
		scope.OriginSeq = m.searchView.snapshot.BottomSeq
		scope.OriginSet = true
	}
	if m.searchView.focus != nil {
		scope.ResumeSeq = m.searchView.focus.Seq
		scope.ResumeSet = true
	}
	return scope
}

// recenterSearchFocus reapplies the active preview after layout geometry
// changes. Search owns the anchor; layout owns only viewport dimensions.
// The return value reports whether session-visible scroll state changed.
func (m *Model) recenterSearchFocus() bool {
	if !m.input.SearchActive() || m.searchView.focus == nil {
		return false
	}
	beforeMode := m.output.viewport.Mode()
	beforeLines := m.output.viewport.NewLineCount()
	m.output.viewport.CenterOn(m.searchView.focus.Seq)
	return beforeMode != m.output.viewport.Mode() || beforeLines != m.output.viewport.NewLineCount()
}

// PreviewSearch centers and highlights the selected match; with no
// match it returns the viewport to the pre-search position.
func (m *Model) PreviewSearch(match widget.SearchMatch, ok bool) {
	if ok {
		m.searchView.focus = &match
		m.syncViewportSize()
		m.recenterSearchFocus()
		m.output.viewport.SetHighlight(match.Seq, match.Ranges)
	} else {
		m.searchView.focus = nil
		m.syncViewportSize()
		m.output.viewport.ClearHighlight()
		m.output.viewport.RestoreScroll(m.searchView.snapshot)
	}
	m.updateScrollState()
}

// CommitSearch keeps the accepted match centered and highlighted after the
// navigator closes. Manual viewport navigation clears the marker.
func (m *Model) CommitSearch() {
	m.syncViewportSize()
	if m.searchView.focus != nil {
		m.output.viewport.CenterOn(m.searchView.focus.Seq)
		m.output.viewport.SetHighlight(m.searchView.focus.Seq, m.searchView.focus.Ranges)
	} else {
		m.output.viewport.ClearHighlight()
	}
	m.searchView.priorFocus = nil
	m.notifySession(ui.SearchStateChangedMsg(false))
	m.updateScrollState()
}

// CancelSearch restores both the position and any committed highlight that
// existed when the navigator opened.
func (m *Model) CancelSearch() {
	m.syncViewportSize()
	m.output.viewport.RestoreScroll(m.searchView.snapshot)
	if m.searchView.priorFocus != nil {
		m.searchView.focus = m.searchView.priorFocus
		m.output.viewport.SetHighlight(m.searchView.focus.Seq, m.searchView.focus.Ranges)
	} else {
		m.searchView.focus = nil
		m.output.viewport.ClearHighlight()
	}
	m.searchView.priorFocus = nil
	m.notifySession(ui.SearchStateChangedMsg(false))
	m.updateScrollState()
}

// clearCommittedSearchFocus retires the accepted marker before deliberate
// viewport navigation. An active search still owns its preview marker.
func (m *Model) clearCommittedSearchFocus() {
	if m.input.SearchActive() || m.searchView.focus == nil {
		return
	}
	m.searchView.focus = nil
	m.output.viewport.ClearHighlight()
}
