package main

import "github.com/DeLFuS77/NXPress/internal/jobs"

// TrashCheck tells, before a work starts, the originals it would send to the recycle bin that
// cannot go there, by drive: their drive has none (a network folder, some USB drives), or they do
// not fit in it. The page asks whether to keep them or delete them for good.
func (a *App) TrashCheck(request Request) []jobs.NoTrash {
	return jobs.NoTrashOf(a.requestOptions(request))
}
