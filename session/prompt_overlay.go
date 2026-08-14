package session

type promptOverlayDisplay interface {
	CommitPrompt(string)
	SetPrompt(string)
}

// promptOverlay tracks server text displayed below scrollback. active remains
// true when Lua gags the text to an empty string.
type promptOverlay struct {
	display   promptOverlayDisplay
	text      string
	active    bool
	confirmed bool
}

func newPromptOverlay(display promptOverlayDisplay) promptOverlay {
	return promptOverlay{display: display}
}

func (p *promptOverlay) replace(text string, confirmed bool) {
	p.text = text
	p.active = true
	p.confirmed = confirmed
	p.display.SetPrompt(text)
}

// commitOrDiscard commits a confirmed prompt and discards a superseded partial line.
func (p *promptOverlay) commitOrDiscard() {
	if !p.active {
		return
	}
	if p.confirmed {
		p.commit()
		return
	}
	p.discard()
}

// commitIfConfirmed commits a confirmed prompt before replacing the overlay.
func (p *promptOverlay) commitIfConfirmed() {
	if p.active && p.confirmed {
		p.commit()
	}
}

// commit moves the active prompt overlay to scrollback. It returns true even
// when Lua gagged the overlay text.
func (p *promptOverlay) commit() bool {
	if !p.active {
		return false
	}
	p.display.CommitPrompt(p.text)
	p.reset()
	return true
}

func (p *promptOverlay) discard() {
	if !p.active {
		return
	}
	p.reset()
	p.display.SetPrompt("")
}

func (p *promptOverlay) reset() {
	p.text = ""
	p.active = false
	p.confirmed = false
}
