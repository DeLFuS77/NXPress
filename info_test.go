package main

import (
	"strings"
	"testing"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Info tells the firmware a game needs, the version of its game a DLC needs, and its NCA files.
func TestTitleDetails(t *testing.T) {
	if got := firmware(0x0C100000); got != "3.1.0" {
		t.Errorf("firmware %s", got)
	}
	if got := firmware(805306368); got != "12.0.0" {
		t.Errorf("firmware %s", got)
	}
	game := &switchfs.ContentMetaAttributes{Type: "BASE", RequiredTitleVersion: 805306368, Contents: map[string]switchfs.Content{"Program": {ID: "aa"}, "Meta": {ID: "bb"}}}
	if got := strings.Join(titleDetails(game), "|"); got != "  needs firmware 12.0.0|  Meta NCA bb|  Program NCA aa" {
		t.Errorf("game: %q", got)
	}
	dlc := &switchfs.ContentMetaAttributes{Type: "DLC", RequiredTitleVersion: 65536}
	if got := strings.Join(titleDetails(dlc), "|"); got != "  needs version 65536 of its game" {
		t.Errorf("DLC: %q", got)
	}
}
