package edensaves

import (
	"context"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/edensync"
)

// the states of the save of a game, as the page tells them
const (
	// the emulator and the library hold the same save
	GameInStep = "inStep"
	// a newer save of a console, or of the emulator of another computer, goes into the emulator
	// when it closes
	GameConsoleNewer = "consoleNewer"
	GameOtherNewer   = "otherNewer"
	// the save of the emulator changed: it goes to the library when the emulator closes
	GameEdenNewer = "edenNewer"
	// the game was never opened with this profile in the emulator: a save goes in once it was
	GameOpenOnce = "openOnce"
	// both changed: the user chooses
	GameConflict = "conflict"
)

// Profile is a profile of the emulator and the account of the console it goes with.
type Profile struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	Account string `json:"account"`
}

// Game is the save of a game of a profile, as the page lists it.
type Game struct {
	TitleId string `json:"titleId"`
	Name    string `json:"name"`
	Profile string `json:"profile"`
	// when it changed in the emulator, and the newest save of the library of a console and of
	// another computer (seconds since 1970, 0 for none)
	Eden    int64  `json:"eden"`
	Console int64  `json:"console"`
	Other   int64  `json:"other"`
	State   string `json:"state"`
}

// ConflictSave is one of the two saves of a conflict.
type ConflictSave struct {
	Keep   string `json:"keep"`
	Source string `json:"source"`
	When   int64  `json:"when"`
	Size   int64  `json:"size"`
	// it is still in the library
	Found bool `json:"found"`
}

// Conflict is a game whose saves both changed, waiting for the choice of the user.
type Conflict struct {
	Id      string         `json:"id"`
	TitleId string         `json:"titleId"`
	Name    string         `json:"name"`
	Profile string         `json:"profile"`
	Saves   []ConflictSave `json:"saves"`
}

// view is what the library and the emulator tell of the saves, for the page.
type view struct {
	Profiles  []Profile
	Accounts  []string
	Games     []Game
	Conflicts []Conflict
}

// look reads the profiles of the emulator and what the library holds, and tells the state of the
// save of each game, as the next pass would see it. It only reads.
func look(ctx context.Context, folder edensync.Folder, library edensync.Library, state *edensync.State, keyName string) (view, error) {
	result := view{Profiles: []Profile{}, Accounts: []string{}, Games: []Game{}, Conflicts: []Conflict{}}
	profiles, err := folder.Profiles()
	if err != nil {
		return result, err
	}
	pairs, err := library.Pairs(ctx)
	if err != nil {
		return result, err
	}
	saves, err := library.Saves(ctx)
	if err != nil {
		return result, err
	}
	conflicts, err := library.Conflicts(ctx)
	if err != nil {
		return result, err
	}
	if state == nil {
		state = edensync.NewState()
	}
	result.Accounts = append(result.Accounts, pairs.Accounts...)
	names := map[string]string{}
	byId := map[string]edensync.LibrarySave{}
	for _, save := range saves {
		title, _ := edensync.NormalTitleID(save.TitleId)
		if save.Game != "" && names[title] == "" {
			names[title] = save.Game
		}
		byId[title+"/"+save.Id] = save
	}
	open := map[string]edensync.Conflict{}
	for _, conflict := range conflicts {
		title, ok := edensync.NormalTitleID(conflict.TitleId)
		if !ok {
			continue
		}
		open[conflict.Profile+"/"+title] = conflict
	}
	for _, profile := range profiles {
		id := profile.ID.String()
		account := accountOf(pairs, id, folder.Emulator)
		result.Profiles = append(result.Profiles, Profile{Id: id, Name: profile.Name, Account: account})
		for _, title := range titles(folder, profile, account, pairs, saves) {
			game, ok := gameOf(folder, profile, account, title, pairs, saves, state)
			if conflict, waiting := open[id+"/"+title]; waiting {
				switch conflict.Keep {
				case "":
					game.State = GameConflict
					result.Conflicts = append(result.Conflicts, conflictOf(conflict, title, names[title], profile.Name, byId, keyName))
				case edensync.KeepConsole:
					game.State = GameConsoleNewer
				case edensync.KeepEmulator:
					game.State = GameInStep
				}
				ok = true
			}
			if !ok {
				continue
			}
			game.Name = names[title]
			result.Games = append(result.Games, game)
		}
	}
	// the games to look at first, then the ones that changed last
	rank := map[string]int{GameConflict: 0, GameConsoleNewer: 1, GameOtherNewer: 1, GameEdenNewer: 1, GameOpenOnce: 2, GameInStep: 3}
	sort.SliceStable(result.Games, func(i, j int) bool {
		a, b := result.Games[i], result.Games[j]
		if rank[a.State] != rank[b.State] {
			return rank[a.State] < rank[b.State]
		}
		return max(a.Eden, a.Console, a.Other) > max(b.Eden, b.Console, b.Other)
	})
	return result, nil
}

// accountOf is the account of the console a profile of this computer goes with.
func accountOf(pairs edensync.Pairs, id string, emulator edensync.Emulator) string {
	account, found := "", false
	for _, paired := range pairs.Profiles {
		if paired.Profile != id {
			continue
		}
		if paired.Mine && strings.EqualFold(paired.Emulator, string(emulator)) {
			return paired.Account
		}
		if !found {
			account, found = paired.Account, true
		}
	}
	return account
}

// relevant are the saves of a game for a profile, as a pass counts them (edensync): of the
// consoles of its account, and of the emulators of the profiles that go with the same account.
func relevant(saves []edensync.LibrarySave, profile, account, title string, pairs edensync.Pairs) []edensync.LibrarySave {
	list := []edensync.LibrarySave{}
	for _, save := range saves {
		if !strings.EqualFold(save.TitleId, title) {
			continue
		}
		switch {
		case !save.Emulator:
			if account != "" && strings.EqualFold(strings.TrimSpace(save.Account), account) {
				list = append(list, save)
			}
		case save.Profile == profile:
			list = append(list, save)
		case save.Profile != "" && account != "" && strings.EqualFold(pairedAccount(pairs, save.Profile), account):
			list = append(list, save)
		}
	}
	return list
}

// pairedAccount is the account a profile of any computer goes with, the one of this computer
// first.
func pairedAccount(pairs edensync.Pairs, id string) string {
	account, found := "", false
	for _, paired := range pairs.Profiles {
		if paired.Profile == id && (!found || paired.Mine) {
			account, found = paired.Account, true
		}
	}
	return account
}

// newestOf is the newest save of the consoles (emulator false) or of the emulators.
func newestOf(saves []edensync.LibrarySave, emulator bool) *edensync.LibrarySave {
	var newest *edensync.LibrarySave
	for i := range saves {
		if saves[i].Emulator == emulator && (newest == nil || saves[i].When > newest.When) {
			newest = &saves[i]
		}
	}
	return newest
}

// titles are the games of a profile: with a folder in the emulator, or a save of the library.
func titles(folder edensync.Folder, profile edensync.Profile, account string, pairs edensync.Pairs, saves []edensync.LibrarySave) []string {
	found := map[string]bool{}
	games, _ := folder.Games(profile.ID)
	for _, game := range games {
		found[game] = true
	}
	for _, save := range saves {
		title, ok := edensync.NormalTitleID(save.TitleId)
		if ok && !found[title] && len(relevant([]edensync.LibrarySave{save}, profile.ID.String(), account, title, pairs)) > 0 {
			found[title] = true
		}
	}
	list := make([]string, 0, len(found))
	for title := range found {
		list = append(list, title)
	}
	sort.Strings(list)
	return list
}

// gameOf tells the state of the save of a game of a profile with what edensync.Decide would do;
// false for a game with no save anywhere.
func gameOf(folder edensync.Folder, profile edensync.Profile, account, title string, pairs edensync.Pairs, saves []edensync.LibrarySave, state *edensync.State) (Game, bool) {
	game := Game{TitleId: title, Profile: profile.Name}
	dir := folder.SaveFolder(profile.ID, title)
	local, _ := edensync.HashFolder(dir)
	known := relevant(saves, profile.ID.String(), account, title, pairs)
	console, emulator := newestOf(known, false), newestOf(known, true)
	if console != nil {
		game.Console = console.When
	}
	for _, save := range known {
		if save.Emulator && save.Profile != profile.ID.String() {
			game.Other = max(game.Other, save.When)
		}
	}
	if local == "" && len(known) == 0 {
		return game, false
	}
	game.Eden = changedAt(dir)
	last := state.Games[edensync.StateKey(folder.Emulator, profile.ID, title)]
	if last == nil && local != "" {
		for _, save := range known {
			if save.Hash == local && (last == nil || save.When > last.Seen) {
				last = &edensync.Synced{SaveId: save.Id, Hash: local, Seen: save.When, SeenIds: []string{save.Id}}
			}
		}
	}
	decision := edensync.Decide(last, local, console, emulator)
	switch decision.Action {
	case edensync.DoNothing:
		game.State = GameInStep
	case edensync.DoUpload:
		game.State = GameEdenNewer
	case edensync.DoAsk:
		game.State = GameConflict
	case edensync.DoBring:
		switch {
		case !isDir(dir):
			game.State = GameOpenOnce
		case decision.Save != nil && decision.Save.Emulator:
			game.State = GameOtherNewer
		default:
			game.State = GameConsoleNewer
		}
	}
	return game, true
}

// conflictOf is a conflict as the page shows it: the save of the emulator first.
func conflictOf(conflict edensync.Conflict, title, name, profile string, byId map[string]edensync.LibrarySave, keyName string) Conflict {
	shown := Conflict{Id: conflict.Id, TitleId: title, Name: name, Profile: profile, Saves: []ConflictSave{}}
	for _, side := range []struct{ keep, id string }{{edensync.KeepEmulator, conflict.EmulatorSave}, {edensync.KeepConsole, conflict.ConsoleSave}} {
		save, found := byId[title+"/"+side.id]
		source := save.Console
		if save.Emulator && save.Device != "" && save.Device != keyName {
			source += " · " + save.Device
		}
		shown.Saves = append(shown.Saves, ConflictSave{Keep: side.keep, Source: source, When: save.When, Size: save.Size, Found: found})
	}
	return shown
}

// changedAt is when a file of the game in a save folder last changed, 0 for none.
func changedAt(dir string) int64 {
	newest := int64(0)
	filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || edensync.Ignored(entry.Name()) {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			newest = max(newest, info.ModTime().Unix())
		}
		return nil
	})
	return newest
}
