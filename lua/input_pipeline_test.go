package lua

import "github.com/mmcdole/rune/input"

// dispatchTestSubmission drives the two phases that Session separates with
// echo and history commits. Tests focused on Lua routing do not need to model
// those Session-owned commits.
func dispatchTestSubmission(engine *Engine, submission input.Submission) bool {
	effective, proceed := engine.ApplyInputHooks(submission)
	if proceed {
		engine.DispatchSubmission(effective)
	}
	return proceed
}

func dispatchTestCommand(engine *Engine, text string) bool {
	return dispatchTestSubmission(engine, input.Command(text))
}
