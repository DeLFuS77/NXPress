package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/edensaves"
	"github.com/dtrunk90/switch-library-manager-web/edensync"
)

// The answers of the saves of Eden the page tells in its language, and the others with their
// error.
func TestEdenAnswers(t *testing.T) {
	for _, answer := range []struct {
		err  error
		want string
	}{
		{nil, ""},
		{edensaves.ErrNotLibrary, "notLibrary"},
		{fmt.Errorf("opened: %w", edensync.ErrNotFound), "notEden"},
		{edensaves.ErrKeepUnknown, "keepOld"},
		{edensaves.ErrConflictGone, "gone"},
		{&edensync.StatusError{Status: 401, Message: "unauthorized"}, "key"},
		{&edensync.StatusError{Status: 404, Message: "404 page not found"}, "netOff"},
		{&edensync.StatusError{Status: 404, Message: "no such save"}, "other: the library answered 404: no such save"},
		{errors.New("disk full"), "other: disk full"},
	} {
		if got := edenAnswer(answer.err); got != answer.want {
			t.Errorf("%v: %q, not %q", answer.err, got, answer.want)
		}
	}
	// a window without its context gives lists, never null, and closes
	status := (&App{}).EdenSaves()
	if status.Profiles == nil || status.Accounts == nil || status.Games == nil || status.Conflicts == nil {
		t.Fatalf("the status: %+v", status)
	}
}
