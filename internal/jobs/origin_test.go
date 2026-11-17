package jobs

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeLFuS77/NXPress/internal/testkit"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

const (
	programID = "0123456789abcdef0123456789abcdef"
	controlID = "fedcba9876543210fedcba9876543210"
	metaID    = "00000000000000000000000000000001"
	rightsID  = "0100000000010000000000000000000a"
)

// game is the inventory of a game of one title, its program and control NCA as given; the Meta
// NCA is signed, and made for a game card, as the last one given.
func game(card bool, ncas ...switchfs.NcaFacts) switchfs.PackageInventory {
	meta := ncas[len(ncas)-1]
	meta.Name, meta.ID, meta.ContentType, meta.RightsID = metaID+".cnmt.nca", metaID, 1, ""
	inventory := switchfs.PackageInventory{Card: card, Ncas: append(ncas, meta)}
	inventory.Titles = []switchfs.TitleContents{{TitleID: "0100000000010000", Type: "BASE", Contents: []switchfs.ListedContent{{ID: programID, Type: 1}, {ID: controlID, Type: 3}, {ID: "deadbeefdeadbeefdeadbeefdeadbeef", Type: 6}}}}
	return inventory
}

func nca(id string, facts switchfs.NcaFacts) switchfs.NcaFacts {
	facts.Name, facts.ID = id+".nca", id
	return facts
}

// Each kind of package is told by what it holds: the delta fragment the CNMT lists is never
// missed, as trimming takes it away.
func TestClassifyOrigin(t *testing.T) {
	signed := switchfs.NcaFacts{Signed: true, SignatureKnown: true}
	unsigned := switchfs.NcaFacts{SignatureKnown: true}
	licensed := signed
	licensed.RightsID = rightsID
	onCard := signed
	onCard.GameCard = true
	withTicket := func(inventory switchfs.PackageInventory) switchfs.PackageInventory {
		inventory.Tickets, inventory.Certificates = []string{rightsID}, 1
		return inventory
	}
	rewritten := unsigned
	cases := map[string]switchfs.PackageInventory{
		OriginPublished: withTicket(game(false, nca(programID, licensed), nca(controlID, licensed))),
		OriginCard:      game(true, nca(programID, onCard), nca(controlID, onCard)),
		OriginFromCard:  game(false, nca(programID, onCard), nca(controlID, onCard)),
		OriginToCard:    game(true, nca(programID, signed), nca(controlID, signed)),
		OriginHomebrew:  game(false, nca(programID, unsigned), nca(controlID, unsigned)),
		// the tickets taken out: the NCA rewritten to carry their key, the Meta NCA as it was
		OriginModified:   game(false, nca(programID, rewritten), nca(controlID, signed)),
		OriginIncomplete: withTicket(game(false, nca(programID, licensed))),
	}
	for want, inventory := range cases {
		if got := ClassifyOrigin(inventory); got.Kind != want {
			t.Errorf("%s: told %+v", want, got)
		}
	}
	incomplete := ClassifyOrigin(cases[OriginIncomplete])
	if len(incomplete.Missing) != 1 || incomplete.Missing[0] != (MissingContent{Kind: "Control", Title: "0100000000010000"}) || len(incomplete.MissingTickets) != 0 {
		t.Errorf("%+v", incomplete)
	}
	if line := OriginLine(incomplete); line != "incomplete: missing Control NCA of 0100000000010000" {
		t.Errorf("%q", line)
	}
	// a rights ID without its ticket cannot be installed: it is missing as well
	noTicket := ClassifyOrigin(game(false, nca(programID, licensed), nca(controlID, licensed)))
	if noTicket.Kind != OriginIncomplete || len(noTicket.MissingTickets) != 1 || noTicket.MissingTickets[0] != strings.ToUpper(rightsID) {
		t.Errorf("%+v", noTicket)
	}
	// a game whose NCA are all signed but without rights ID or ticket: the eShop gives none such,
	// so a tool took its tickets out
	stripped := ClassifyOrigin(game(false, nca(programID, signed), nca(controlID, signed)))
	if stripped.Kind != OriginModified {
		t.Errorf("%+v", stripped)
	}
	// a DLC of the eShop alone tells nothing of tickets taken out
	dlc := game(false, nca(programID, signed))
	dlc.Titles = []switchfs.TitleContents{{TitleID: "0100000000011001", Type: "DLC", Contents: []switchfs.ListedContent{{ID: programID, Type: 4}}}}
	if got := ClassifyOrigin(dlc); got.Kind != OriginPublished {
		t.Errorf("%+v", got)
	}
}

// A package of the test keys is read from the file: its NCA are not signed by Nintendo and have
// no rights ID, so it is homebrew; a CNMT that lists a program it does not hold makes it
// incomplete. Without an inventory nothing is told.
func TestReadOriginOfFiles(t *testing.T) {
	testkit.Isolate(t)
	folder := t.TempDir()
	path := testkit.TitleNsp(t, filepath.Join(folder, "game.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, 1, nil)
	if origin, ok := ReadOrigin(path); !ok || origin.Kind != OriginHomebrew {
		t.Fatalf("%+v %v", origin, ok)
	}
	program := testkit.ProgramNcaWith(testkit.GameData(0x10000, 4), nil)
	broken := testkit.NspListing(t, filepath.Join(folder, "broken.nsp"), testkit.BaseID, 0, switchfs.ContentMetaType_Application, program, testkit.ContentName(program, ".nca"), programID, nil)
	if origin, ok := ReadOrigin(broken); !ok || origin.Kind != OriginIncomplete || len(origin.Missing) != 1 || origin.Missing[0].Kind != "Program" {
		t.Fatalf("%+v %v", origin, ok)
	}
	if _, ok := ReadOrigin(filepath.Join(folder, "gone.nsp")); ok {
		t.Fatal("a file that is not there")
	}
}
