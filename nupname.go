package main

import "github.com/DeLFuS77/NXPress/internal/jobs"

// NupNameFor is the name the NUP of a dump folder gets by the template of the name of each
// SuperNSP in Combine; the list of Combine tells it under the folder.
func (a *App) NupNameFor(dir string, template string) (string, error) {
	return jobs.NupNameBy(dir, template)
}
