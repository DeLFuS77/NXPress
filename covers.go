package main

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"sync"

	"github.com/DeLFuS77/NXPress/internal/eshop"
	"github.com/DeLFuS77/NXPress/internal/jobs"
	"github.com/DeLFuS77/NXPress/internal/nup"
)

// The images of a NUP whose game titledb has no icon of (#102): searched in the eShop by the name
// of the game, which the card writes and one can change, as file names and title IDs are often
// wrong; searched on the web, in the browser, for what the eShop does not have; or pasted, an
// image copied in the browser. The eShop is asked only as a search is started.

// Cover is a game a search of the eShop gives, with its image as an address of data for the page
// (which loads no image from the internet) and the address it comes from, to choose it by.
type Cover struct {
	Title   string   `json:"title"`
	Systems []string `json:"systems"`
	Image   string   `json:"image"`
	Address string   `json:"address"`
}

// coverClient is the client the eShop is asked with, which the tests stand in for.
var coverClient = http.DefaultClient

// coverDownloads is how many images of a search are downloaded at once.
const coverDownloads = 6

// SearchCovers asks the eShop for the games of a name, the ones of the Switch 2 only when asked,
// with their images; a game whose image does not come is left out.
func (a *App) SearchCovers(name string, onlySwitch2 bool) ([]Cover, error) {
	games, err := eshop.Search(a.context(), coverClient, name, onlySwitch2)
	if err != nil {
		return nil, err
	}
	images := make([][]byte, len(games))
	var wait sync.WaitGroup
	slots := make(chan struct{}, coverDownloads)
	for i, game := range games {
		wait.Add(1)
		go func() {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			images[i], _ = eshop.DownloadImage(a.context(), coverClient, game.Image)
		}()
	}
	wait.Wait()
	covers := []Cover{}
	kept := map[string][]byte{}
	failed := 0
	for i, game := range games {
		if len(images[i]) == 0 {
			failed++
			continue
		}
		kept[game.Image] = images[i]
		covers = append(covers, Cover{Title: game.Title, Systems: game.Systems, Address: game.Image,
			Image: "data:" + http.DetectContentType(images[i]) + ";base64," + base64.StdEncoding.EncodeToString(images[i])})
	}
	a.coversMutex.Lock()
	a.covers = kept
	a.coversMutex.Unlock()
	if len(covers) == 0 && failed > 0 {
		return nil, errors.New("the images of the eShop cannot be downloaded: the network may not let them through")
	}
	return covers, nil
}

// UseCover gives a NUP the image of a game of the last search, the one of its address; with
// replace, a NUP given one already has it changed (#103).
func (a *App) UseCover(path string, address string, replace bool) (string, error) {
	if a.working() {
		return "", errors.New("wait for the work to end")
	}
	if !eshop.ImageAddress(address) {
		return "", errors.New("not an image of the eShop")
	}
	a.coversMutex.Lock()
	image := a.covers[address]
	a.coversMutex.Unlock()
	if image == nil {
		var err error
		if image, err = eshop.DownloadImage(a.context(), coverClient, address); err != nil {
			return "", err
		}
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return fixNupIcon(a.ctx, path, jobs.Options{NupIconImage: image, NupIconFrom: "the image of the eShop", ReplaceNupIcon: replace})
}

// PasteNupIcon gives a NUP an image pasted in the window, as the page reads it (an address of
// data); with replace, a NUP given one already has it changed (#103).
func (a *App) PasteNupIcon(path string, data string, replace bool) (string, error) {
	if a.working() {
		return "", errors.New("wait for the work to end")
	}
	image, err := pastedImage(data)
	if err != nil {
		return "", err
	}
	done, err := a.chore(true)
	if err != nil {
		return "", err
	}
	defer done()
	return fixNupIcon(a.ctx, path, jobs.Options{NupIconImage: image, NupIconFrom: "the image pasted", ReplaceNupIcon: replace})
}

// pastedImage reads an image pasted, as an address of data of an image.
func pastedImage(data string) ([]byte, error) {
	head, encoded, ok := strings.Cut(data, ",")
	if !ok || !strings.HasPrefix(head, "data:image/") || !strings.HasSuffix(head, ";base64") {
		return nil, errors.New("what was pasted is not an image")
	}
	if base64.StdEncoding.DecodedLen(len(encoded)) > nup.MaxIconFile {
		return nil, errors.New("the image is too big to be an icon")
	}
	image, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(image) == 0 {
		return nil, errors.New("what was pasted is not an image")
	}
	return image, nil
}

// SearchCoverOnWeb opens a search of images of a game in the browser, for what the eShop does
// not have: an image copied there is pasted in its card.
func (a *App) SearchCoverOnWeb(name string) {
	openURL(a.ctx, eshop.WebSearch(name))
}

// context is the context of the window, or one of its own before it has one (in the tests).
func (a *App) context() context.Context {
	if a.ctx != nil {
		return a.ctx
	}
	return context.Background()
}
