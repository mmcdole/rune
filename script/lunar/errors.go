package lunar

import (
	"errors"

	lua "github.com/mmcdole/lunar"
)

// asResultCapacityError isolates the one place the backend inspects a Lunar
// error type, so the fixed-result-count path reads as one idea.
func asResultCapacityError(
	err error,
	target **lua.ResultCapacityError,
) bool {
	return errors.As(err, target)
}
