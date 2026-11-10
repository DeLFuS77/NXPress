package eshop

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestQuery(t *testing.T) {
	for name, want := range map[string]string{
		"Made-up Quest [0400486024DEC000][v65536].nup": "Made-up Quest",
		"  Made_up   Quest (EU) [v0] ":                 "Made up Quest",
		"Made-up Quest™ Deluxe.nsz":                    "Made-up Quest Deluxe",
		"[0400486024DEC000]":                           "",
	} {
		if got := Query(name); got != want {
			t.Errorf("%q: %q, not %q", name, got, want)
		}
	}
}

func TestImageAddress(t *testing.T) {
	for address, want := range map[string]bool{
		"https://www.nintendo.com/eu/media/images/game.jpg":     true,
		"https://fs-prod-cdn.nintendo-europe.com/x/game.png":    true,
		"https://img-eshop.cdn.nintendo.net/i/game.jpg":         true,
		"http://www.nintendo.com/eu/media/images/game.jpg":      false,
		"https://nintendo.com.example.org/game.jpg":             false,
		"https://evilnintendo.com/game.jpg":                     false,
		"https://user@www.nintendo.com/game.jpg":                false,
		"file:///C:/Windows/win.ini":                            false,
		"https://www.nintendo.com/" + strings.Repeat("a", 3000): false,
	} {
		if got := ImageAddress(address); got != want {
			t.Errorf("%q: %v", address, got)
		}
	}
}

// serve stands in for the eShop for the length of a test: its search and its images.
func serve(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	address, kept, hosts := SearchAddress, ImageHosts, ImageHosts
	SearchAddress = server.URL + "/en/select"
	ImageHosts = append(append([]string{}, hosts...), "127.0.0.1")
	t.Cleanup(func() { SearchAddress, ImageHosts = address, kept })
	return server
}

func TestSearch(t *testing.T) {
	var asked url.Values
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		asked = r.URL.Query()
		w.Write([]byte(`{"response":{"docs":[
			{"title":"Made-up Quest","system_names_txt":["Nintendo Switch 2"],"image_url_sq_s":"https://www.nintendo.com/eu/a.jpg"},
			{"title":"Made-up Quest again","image_url_sq_s":"https://www.nintendo.com/eu/a.jpg"},
			{"title":"No image"},
			{"title":"Elsewhere","image_url_sq_s":"https://example.org/b.jpg"},
			{"title":" Made-up Quest DX ","system_names_txt":["Nintendo Switch"],"image_url_sq_s":"https://www.nintendo.com/eu/c.jpg"}]}}`))
	})
	games, err := Search(t.Context(), server.Client(), "Made-up Quest [0400486024DEC000][v0].nup", true)
	if err != nil {
		t.Fatal(err)
	}
	if asked.Get("q") != "Made-up Quest" || asked.Get("fq") != "type:GAME AND system_type:nintendoswitch2" {
		t.Errorf("asked %v", asked)
	}
	if len(games) != 2 || games[0].Title != "Made-up Quest" || games[0].Systems[0] != "Nintendo Switch 2" || games[1].Title != "Made-up Quest DX" {
		t.Errorf("%+v", games)
	}
	if _, err := Search(t.Context(), server.Client(), "Made-up Quest", false); err != nil || asked.Get("fq") != "type:GAME" {
		t.Errorf("%v %v", err, asked)
	}
	if _, err := Search(t.Context(), server.Client(), " [v0] ", true); err == nil {
		t.Error("an empty name was searched")
	}

	// an answer that is no search
	server = serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<html>")) })
	if _, err := Search(t.Context(), server.Client(), "Made-up Quest", true); err == nil || !strings.Contains(err.Error(), "cannot be read") {
		t.Errorf("%v", err)
	}
	server = serve(t, func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusServiceUnavailable) })
	if _, err := Search(t.Context(), server.Client(), "Made-up Quest", true); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("%v", err)
	}
}

func TestDownloadImage(t *testing.T) {
	image := bytes.Repeat([]byte{0xFF}, 1000)
	server := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/big.jpg" {
			w.Write(make([]byte, MaxImage+1))
			return
		}
		w.Write(image)
	})
	if data, err := DownloadImage(t.Context(), server.Client(), server.URL+"/a.jpg"); err != nil || !bytes.Equal(data, image) {
		t.Errorf("%v", err)
	}
	if _, err := DownloadImage(t.Context(), server.Client(), server.URL+"/big.jpg"); err == nil {
		t.Error("too large downloaded")
	}
	if _, err := DownloadImage(t.Context(), server.Client(), "https://example.org/a.jpg"); err == nil {
		t.Error("an address of elsewhere downloaded")
	}
}

func TestWebSearch(t *testing.T) {
	address := WebSearch("Made-up Quest [v0].nup")
	parsed, err := url.Parse(address)
	if err != nil || parsed.Host != "duckduckgo.com" || parsed.Query().Get("q") != "Made-up Quest Switch 2 cover" || parsed.Query().Get("iax") != "images" {
		t.Errorf("%s %v", address, err)
	}
}
