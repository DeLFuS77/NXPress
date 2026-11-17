package main

import (
	"errors"

	"github.com/DeLFuS77/NXPress/internal/nup"
)

// EditNupTitle changes the title name of a NUP where it is (#144, brought from the edition of
// QuickWick, PR #151): the files stay as they are, its metadata is written again after its end.
func (a *App) EditNupTitle(path string, title string) error {
	if a.working() {
		return errors.New("wait for the work to end")
	}
	done, err := a.chore(true)
	if err != nil {
		return err
	}
	defer done()
	return nup.EditMeta(path, title, nil)
}
