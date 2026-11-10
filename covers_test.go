package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/eshop"
	"github.com/DeLFuS77/NXPress/internal/jobs"
)

// standInForTheEShop serves a search with two games, one whose image does not come.
func standInForTheEShop(t *testing.T) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/en/select":
			fmt.Fprintf(w, `{"response":{"docs":[{"title":"Made-up Quest","system_names_txt":["Nintendo Switch 2"],"image_url_sq_s":"%[1]s/a.jpg"},{"title":"Gone","image_url_sq_s":"%[1]s/gone.jpg"}]}}`, server.URL)
		case "/a.jpg":
			w.Write([]byte("\xFF\xD8\xFF made-up image"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	address, hosts, client := eshop.SearchAddress, eshop.ImageHosts, coverClient
	eshop.SearchAddress = server.URL + "/en/select"
	eshop.ImageHosts = append(append([]string{}, hosts...), "127.0.0.1")
	coverClient = server.Client()
	t.Cleanup(func() { eshop.SearchAddress, eshop.ImageHosts, coverClient = address, hosts, client })
	return server
}

// standInForTheIcon records what a NUP would be given.
func standInForTheIcon(t *testing.T) *jobs.Options {
	t.Helper()
	given := &jobs.Options{}
	give := fixNupIcon
	fixNupIcon = func(_ context.Context, _ string, o jobs.Options) (string, error) { *given = o; return "given", nil }
	t.Cleanup(func() { fixNupIcon = give })
	return given
}

func TestSearchCoversAndUseOne(t *testing.T) {
	server := standInForTheEShop(t)
	given := standInForTheIcon(t)
	a := &App{}
	covers, err := a.SearchCovers("Made-up Quest [v0].nup", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(covers) != 1 || covers[0].Title != "Made-up Quest" || covers[0].Address != server.URL+"/a.jpg" ||
		covers[0].Image != "data:image/jpeg;base64,"+base64.StdEncoding.EncodeToString([]byte("\xFF\xD8\xFF made-up image")) {
		t.Fatalf("%+v", covers)
	}
	// the one chosen is not downloaded again
	server.Close()
	if note, err := a.UseCover("game.nup", covers[0].Address, true); err != nil || note != "given" {
		t.Fatalf("%q %v", note, err)
	}
	if !bytes.Equal(given.NupIconImage, []byte("\xFF\xD8\xFF made-up image")) || given.NupIconFrom != "the image of the eShop" || !given.ReplaceNupIcon {
		t.Errorf("%+v", given)
	}
	if _, err := a.UseCover("game.nup", "https://example.org/a.jpg", false); err == nil {
		t.Error("an image of elsewhere was used")
	}
}

func TestSearchCoversTellsImagesThatDoNotCome(t *testing.T) {
	server := standInForTheEShop(t)
	eshop.SearchAddress = server.URL + "/en/select"
	// only the game whose image does not come
	hosts := eshop.ImageHosts
	a := &App{}
	covers, err := a.SearchCovers("Gone", false)
	if err != nil || len(covers) != 1 {
		t.Fatalf("%+v %v", covers, err)
	}
	eshop.ImageHosts = hosts
	server.Close()
	if _, err := a.SearchCovers("Gone", false); err == nil || !strings.Contains(err.Error(), "eShop") {
		t.Errorf("%v", err)
	}
}

func TestPasteNupIcon(t *testing.T) {
	given := standInForTheIcon(t)
	a := &App{}
	image := []byte("\x89PNG\r\n\x1a\n made-up")
	if note, err := a.PasteNupIcon("game.nup", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(image), false); err != nil || note != "given" {
		t.Fatalf("%q %v", note, err)
	}
	if !bytes.Equal(given.NupIconImage, image) || given.NupIconFrom != "the image pasted" || given.ReplaceNupIcon {
		t.Errorf("%+v", given)
	}
	for _, data := range []string{"", "some text", "data:text/plain;base64,aGVsbG8=", "data:image/png;base64,***", "data:image/png;base64,"} {
		if _, err := a.PasteNupIcon("game.nup", data, false); err == nil {
			t.Errorf("%q was taken", data)
		}
	}
}
