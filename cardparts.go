package main

import "github.com/DeLFuS77/NXPress/internal/jobs"

// CardParts are the titles a file holds with their IDs and versions: the card of a game tells them.
func (a *App) CardParts(path string) ([]jobs.CardPart, error) {
	return jobs.Parts(path)
}
