package jobs

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// The kinds of package a game file is, by what it holds.
const (
	// as the eShop gives it: downloaded NCA files signed by Nintendo, with the ticket of each
	// rights ID
	OriginPublished = "published"
	// a dump of a game card: an XCI whose NCA files were made for a game card
	OriginCard = "card"
	// an NSP made from a game card: its NCA files were made for a game card
	OriginFromCard = "from-card"
	// an XCI made from a download
	OriginToCard = "to-card"
	// homebrew: no NCA signed by Nintendo and no rights ID
	OriginHomebrew = "homebrew"
	// changed by a tool: NCA files rewritten (not signed by Nintendo), or tickets taken out
	OriginModified = "modified"
	// a content its CNMT lists, or the ticket of a rights ID, is not in it
	OriginIncomplete = "incomplete"
)

// Origin is the kind of package a game file is, with what it is missing when it is incomplete.
type Origin struct {
	Kind string `json:"kind"`
	// the contents a CNMT lists that the file does not hold, and the rights IDs whose ticket it
	// does not hold
	Missing        []MissingContent `json:"missing,omitempty"`
	MissingTickets []string         `json:"missingTickets,omitempty"`
}

// MissingContent is a content a CNMT lists that the file does not hold: its kind (Program,
// Control…) and the title it is of.
type MissingContent struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
}

// contentTypeNames are the kinds of content of a CNMT, by number.
var contentTypeNames = map[byte]string{0: "Meta", 1: "Program", 2: "Data", 3: "Control", 4: "HtmlDocument", 5: "LegalInformation", 6: "DeltaFragment"}

// ClassifyOrigin tells the kind of package of a file from its inventory. Incomplete comes first,
// as it tells what to fetch; then homebrew, then where its NCA files were made for (a game card
// or a download) against what holds them, then whether a tool changed them.
func ClassifyOrigin(inventory switchfs.PackageInventory) Origin {
	origin := Origin{}
	held := map[string]bool{}
	for _, nca := range inventory.Ncas {
		if nca.ID != "" {
			held[nca.ID] = true
		}
	}
	for _, title := range inventory.Titles {
		for _, content := range title.Contents {
			// a delta fragment only patches an older update over the network: trimming takes it away
			if content.Type == 6 || held[strings.ToLower(content.ID)] {
				continue
			}
			kind := contentTypeNames[content.Type]
			if kind == "" {
				kind = fmt.Sprintf("content %d", content.Type)
			}
			origin.Missing = append(origin.Missing, MissingContent{Kind: kind, Title: strings.ToUpper(title.TitleID)})
		}
	}
	tickets := map[string]bool{}
	for _, id := range inventory.Tickets {
		tickets[strings.ToLower(id)] = true
	}
	rights := map[string]bool{}
	signed, unsigned, card := 0, 0, 0
	for _, nca := range inventory.Ncas {
		if nca.RightsID != "" {
			rights[strings.ToLower(nca.RightsID)] = true
		}
		if nca.GameCard {
			card++
		}
		switch {
		case nca.Signed:
			signed++
		case nca.SignatureKnown:
			unsigned++
		}
	}
	for id := range rights {
		if !tickets[id] {
			origin.MissingTickets = append(origin.MissingTickets, strings.ToUpper(id))
		}
	}
	sort.Strings(origin.MissingTickets)
	switch {
	case len(origin.Missing) > 0 || len(origin.MissingTickets) > 0:
		origin.Kind = OriginIncomplete
	case signed == 0 && unsigned > 0 && len(rights) == 0:
		origin.Kind = OriginHomebrew
	case inventory.Card && card == len(inventory.Ncas):
		origin.Kind = OriginCard
	case inventory.Card:
		origin.Kind = OriginToCard
	case card > 0:
		origin.Kind = OriginFromCard
	case unsigned > 0 || (len(rights) == 0 && len(inventory.Tickets) == 0 && hasNintendoTitles(inventory)):
		origin.Kind = OriginModified
	default:
		origin.Kind = OriginPublished
	}
	return origin
}

// hasNintendoTitles tells whether a package holds a game or an update with a program, which the
// eShop always gives with a rights ID: one without it had its tickets taken out by a tool. A DLC
// alone or a package without a program says nothing of it.
func hasNintendoTitles(inventory switchfs.PackageInventory) bool {
	for _, title := range inventory.Titles {
		if title.Type == "BASE" || title.Type == "UPD" {
			for _, content := range title.Contents {
				if content.Type == 1 {
					return true
				}
			}
		}
	}
	return false
}

// ReadOrigin tells the kind of package of a game file; ok is false when it cannot be told (no
// keys, or a file that cannot be read).
func ReadOrigin(path string) (Origin, bool) {
	inventory, err := switchfs.ReadInventory(path)
	if err != nil || len(inventory.Ncas) == 0 {
		return Origin{}, false
	}
	return ClassifyOrigin(inventory), true
}

// OriginLine is the kind of package as Details writes it in English.
func OriginLine(origin Origin) string {
	text := map[string]string{
		OriginPublished:  "as published: downloaded files signed by Nintendo, with their tickets",
		OriginCard:       "game card dump: an XCI with the files of the game card",
		OriginFromCard:   "converted: an NSP made from a game card",
		OriginToCard:     "converted: an XCI made from a download",
		OriginHomebrew:   "homebrew: not signed by Nintendo and without rights ID",
		OriginModified:   "changed by a tool: files rewritten or tickets taken out",
		OriginIncomplete: "incomplete",
	}[origin.Kind]
	missing := []string{}
	for _, content := range origin.Missing {
		missing = append(missing, content.Kind+" NCA of "+content.Title)
	}
	for _, id := range origin.MissingTickets {
		missing = append(missing, "ticket of "+id)
	}
	if len(missing) > 0 {
		text += ": missing " + strings.Join(missing, ", ")
	}
	return text
}
