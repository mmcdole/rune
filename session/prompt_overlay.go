package session

type promptOverlayDisplay interface {
	CommitPrompt(string)
	SetPrompt(string)
}

// promptOverlay owns the mutable text displayed below scrollback. Its active
// bit is independent from text because Lua may gag an update to an empty
// string without removing the boundary represented by that update.
type promptOverlay struct {
	display   promptOverlayDisplay
	text      string
	active    bool
	confirmed bool
}

func newPromptOverlay(display promptOverlayDisplay) promptOverlay {
	return promptOverlay{display: display}
}

// replace installs the latest snapshot. Partial snapshots replace each other;
// committing them here would expose socket read boundaries in scrollback.
func (p *promptOverlay) replace(text string, confirmed bool) {
	p.text = text
	p.active = true
	p.confirmed = confirmed
	p.display.SetPrompt(text)
}

// beforeLine preserves a confirmed prompt as its own record, but discards an
// unconfirmed preview because the completed line contains the same bytes.
func (p *promptOverlay) beforeLine() {
	if p.active && p.confirmed {
		p.commit()
		return
	}
	p.discard()
}

// beforeUpdate preserves a confirmed prompt before the next server record
// replaces it. Unconfirmed snapshots are replaced in place.
func (p *promptOverlay) beforeUpdate() {
	if p.active && p.confirmed {
		p.commit()
	}
}

// commit moves an active, visible overlay record to scrollback, then clears
// the overlay. It returns whether a record existed, including a gagged record.
func (p *promptOverlay) commit() bool {
	if !p.active {
		return false
	}
	p.display.CommitPrompt(p.text)
	p.clearState()
	return true
}

// discard clears the state and presentation without adding anything to
// scrollback. It also clears the presentation when no record is active, which
// makes it suitable for complete-line and connection-reset paths.
func (p *promptOverlay) discard() {
	p.clearState()
	p.display.SetPrompt("")
}

func (p *promptOverlay) clearState() {
	p.text = ""
	p.active = false
	p.confirmed = false
}
