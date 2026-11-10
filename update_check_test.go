package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// CheckUpdate tells a newer version, says GitHub answered when there is none (Check now, #106),
// and asks for an answer of now, not one a cache kept.
func TestCheckUpdateTellsWhetherGitHubAnswered(t *testing.T) {
	tag, status := "1.7.7", http.StatusOK
	cache := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cache = r.Header.Get("Cache-Control")
		w.WriteHeader(status)
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://github.com/DeLFuS77/NXPress/releases/tag/%[1]s","body":"notes"}`, tag)
	}))
	defer server.Close()
	address, kept := latestRelease, version
	latestRelease, version = server.URL, "1.7.6"
	defer func() { latestRelease, version = address, kept }()

	a := &App{}
	if update := a.CheckUpdate(); update.Version != "1.7.7" || !update.Checked {
		t.Errorf("newer: %+v", update)
	}
	if cache != "no-cache" {
		t.Errorf("Cache-Control %q", cache)
	}
	tag = "1.7.6"
	if update := a.CheckUpdate(); update.Version != "" || !update.Checked {
		t.Errorf("the same: %+v", update)
	}
	status = http.StatusForbidden
	if update := a.CheckUpdate(); update.Version != "" || update.Checked {
		t.Errorf("no answer: %+v", update)
	}
	// a build that is not a version asks nothing
	version = "dev"
	if update := a.CheckUpdate(); update.Checked {
		t.Errorf("dev: %+v", update)
	}
}
