package edensaves

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/edensync"
)

// what a pass waits between its looks, as the design asked: a pass when NXPress starts, every
// minute and as soon as the emulator closes
const (
	passEvery  = time.Minute
	watchEvery = 5 * time.Second
	pairEvery  = 2 * time.Second
	// the longest name of a computer the library takes, "NXPress on " included
	maxComputerName = 40
)

// the kinds of the last error, which the page tells in its language
const (
	// the library cannot be reached: the next pass tries again
	ErrorOffline = "offline"
	// the library no longer knows the key of this computer
	ErrorKey = "key"
	// the library does not install over the network now: the saves go through it
	ErrorNetOff = "netOff"
	// no data folder of the emulator, or none with profiles
	ErrorFolder   = "folder"
	ErrorProfiles = "profiles"
	// some games could not be kept in step, the others were
	ErrorGames = "games"
	ErrorOther = "other"
)

// the reasons a pairing ended without a key
const (
	PairRejected   = "rejected"
	PairExpired    = "expired"
	PairNotLibrary = "notLibrary"
)

// ErrNotPaired tells that no library is paired.
var ErrNotPaired = errors.New("no library is paired")

// ErrKeepUnknown tells a library that does not let a computer choose the save of a conflict: an
// older version, where it is chosen on its page Saves.
var ErrKeepUnknown = errors.New("this version of the library does not take the choice from NXPress")

// ErrConflictGone tells a choice no longer asked: it was made on the page Saves meanwhile.
var ErrConflictGone = errors.New("this choice is no longer asked")

// Pairing is a pairing waiting for the user to accept it in the library.
type Pairing struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Library string `json:"library"`
	Expires int64  `json:"expires"`
}

// Status is what the page shows of the saves of Eden; never the key.
type Status struct {
	Paired  bool   `json:"paired"`
	Library string `json:"library"`
	Address string `json:"address"`
	User    string `json:"user"`
	// a pairing waiting, and why the last one ended without a key
	Pairing   *Pairing `json:"pairing,omitempty"`
	PairError string   `json:"pairError,omitempty"`
	// the data folder of the emulator: found by itself or chosen, and portable; "" when missing
	Folder   string `json:"folder"`
	Emulator string `json:"emulator"`
	Portable bool   `json:"portable"`
	Chosen   bool   `json:"chosen"`
	// the passes run, the emulator is open, a pass is going on
	On      bool `json:"on"`
	Running bool `json:"running"`
	Working bool `json:"working"`
	// the last pass that looked at the saves, and what the passes did since the pairing
	LastPass int64 `json:"lastPass"`
	Uploaded int   `json:"uploaded"`
	Brought  int   `json:"brought"`
	// the last error, in English, and its kind
	Error     string     `json:"error,omitempty"`
	ErrorKind string     `json:"errorKind,omitempty"`
	Profiles  []Profile  `json:"profiles"`
	Accounts  []string   `json:"accounts"`
	Games     []Game     `json:"games"`
	Conflicts []Conflict `json:"conflicts"`
}

// Notice is what a pass tells by a notice of the system: the games whose save went into the
// emulator, and the games whose saves both changed. At most one per pass.
type Notice struct {
	Brought   []string `json:"brought"`
	Conflicts []string `json:"conflicts"`
}

// Service keeps the saves of the emulator of this computer in step while NXPress is open.
type Service struct {
	// the folder of the settings of NXPress
	Dir string
	// the client of the library; one with timeouts to connect when nil
	Client *http.Client
	// replaced in the tests: the data folders of the emulators on this computer, whether one
	// is open, the clock, the name of this computer and how often the passes run
	FindFolders func() []edensync.Folder
	IsRunning   func(edensync.Emulator) (bool, error)
	Now         func() time.Time
	Computer    func() string
	Every       time.Duration
	Watch       time.Duration
	PairEvery   time.Duration
	// told the status as it changes, and the notice of a pass; called without a lock held
	Changed func(Status)
	Noticed func(Notice)
	// marks a pass that writes as work NXPress waits for before it closes or restarts into a new
	// version; an error leaves the pass out
	Busy func() (func(), error)

	mutex   sync.Mutex
	loaded  bool
	config  Config
	base    context.Context
	folder  *edensync.Folder
	view    view
	state   edensync.State
	running bool
	working bool
	errText string
	errKind string
	lastRun time.Time
	pairing *pairing
	pairErr string
	// what was asked of the loop: a pass, or a look only
	wake     chan struct{}
	wantPass bool
	wantLook bool
	// one pass or look at a time
	passes sync.Mutex
}

type pairing struct {
	Pairing
	cancel context.CancelFunc
}

func (s *Service) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return defaultClient
}

// defaultClient connects quickly on a local network; a save may take long to upload or download.
var defaultClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	ResponseHeaderTimeout: 2 * time.Minute,
}}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) every() time.Duration {
	if s.Every > 0 {
		return s.Every
	}
	return passEvery
}

func (s *Service) isRunning(emulator edensync.Emulator) (bool, error) {
	if s.IsRunning != nil {
		return s.IsRunning(emulator)
	}
	return edensync.Running(emulator)
}

func (s *Service) statePath() string {
	return filepath.Join(s.Dir, StateName)
}

// computerName is how the library names this computer: NXPress on DESKTOP-1.
func (s *Service) computerName() string {
	name := ""
	if s.Computer != nil {
		name = s.Computer()
	} else if host, err := os.Hostname(); err == nil {
		name = host
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		return "NXPress"
	}
	full := []rune("NXPress on " + name)
	if len(full) > maxComputerName {
		full = full[:maxComputerName]
	}
	return strings.TrimSpace(string(full))
}

// loadLocked reads the settings and the state once. With the mutex held.
func (s *Service) loadLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	config, err := LoadConfig(s.Dir)
	if err != nil {
		s.errText, s.errKind = err.Error(), ErrorOther
	}
	s.config = config
	if state, err := edensync.LoadState(s.statePath()); err == nil {
		s.state = *state
	}
}

// Status is the status as the page shows it.
func (s *Service) Status() Status {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	s.loadLocked()
	config := s.config
	status := Status{Paired: config.Paired(), Library: config.Library, Address: config.Address, User: config.User, PairError: s.pairErr,
		Chosen: config.Folder != "", On: config.On && config.Paired(), Running: s.running, Working: s.working,
		LastPass: s.state.LastPass, Uploaded: s.state.Uploaded, Brought: s.state.Brought, Error: s.errText, ErrorKind: s.errKind,
		Profiles: s.view.Profiles, Accounts: s.view.Accounts, Games: s.view.Games, Conflicts: s.view.Conflicts}
	if s.pairing != nil {
		shown := s.pairing.Pairing
		status.Pairing = &shown
	}
	if folder, err := s.findFolderLocked(); err == nil {
		status.Folder, status.Emulator, status.Portable = folder.Root, folder.Emulator.Name(), folder.Portable
	}
	// lists, never null, for the page
	if status.Profiles == nil {
		status.Profiles = []Profile{}
	}
	if status.Accounts == nil {
		status.Accounts = []string{}
	}
	if status.Games == nil {
		status.Games = []Game{}
	}
	if status.Conflicts == nil {
		status.Conflicts = []Conflict{}
	}
	return status
}

// changed tells the page the status, without the mutex held.
func (s *Service) changed() {
	if s.Changed != nil {
		s.Changed(s.Status())
	}
}

// findFolderLocked is the data folder of the emulator: the one chosen, else the first found.
// With the mutex held.
func (s *Service) findFolderLocked() (edensync.Folder, error) {
	if s.config.Folder != "" {
		emulator, ok := edensync.ParseEmulator(s.config.Emulator)
		if !ok {
			emulator = guessEmulator(s.config.Folder)
		}
		folder, err := edensync.Open(emulator, s.config.Folder)
		if err == nil {
			folder.Portable = isPortable(folder.Root)
		}
		return folder, err
	}
	found := edensync.FindAll()
	if s.FindFolders != nil {
		found = s.FindFolders()
	}
	if len(found) == 0 {
		return edensync.Folder{}, edensync.ErrNotFound
	}
	return found[0], nil
}

// guessEmulator is the emulator of a folder chosen by hand, by its path: Citron when it says so.
func guessEmulator(path string) edensync.Emulator {
	if strings.Contains(strings.ToLower(filepath.ToSlash(path)), "citron") {
		return edensync.Citron
	}
	return edensync.Eden
}

// isPortable tells the folder "user" next to the program of the emulator.
func isPortable(root string) bool {
	if !strings.EqualFold(filepath.Base(root), "user") {
		return false
	}
	entries, err := os.ReadDir(filepath.Dir(root))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() && (strings.HasPrefix(name, "eden") || strings.HasPrefix(name, "citron")) {
			return true
		}
	}
	return false
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// library is the library paired.
func (s *Service) library(config Config) *edensync.HTTPLibrary {
	return &edensync.HTTPLibrary{BaseURL: config.Address, Key: config.Key, Client: s.client()}
}

// IsNetOff tells the answer of a library that does not install over the network now: its paths
// under /install/ are not found, with the plain page of a path that is not there; a save or a
// conflict that is gone is told in JSON instead.
func IsNetOff(status *edensync.StatusError) bool {
	return status.Status == http.StatusNotFound && strings.HasPrefix(status.Message, "404 page not found")
}

// kindOf is the kind of an error, for the page.
func kindOf(err error) string {
	var status *edensync.StatusError
	var netErr net.Error
	var urlErr *url.Error
	switch {
	case err == nil:
		return ""
	case errors.As(err, &status) && status.Status == http.StatusUnauthorized:
		return ErrorKey
	case errors.As(err, &status) && IsNetOff(status):
		return ErrorNetOff
	case errors.As(err, &status):
		return ErrorOther
	case errors.Is(err, edensync.ErrNotFound):
		return ErrorFolder
	case errors.Is(err, os.ErrNotExist), errors.Is(err, edensync.ErrProfiles):
		return ErrorProfiles
	case errors.As(err, &netErr), errors.As(err, &urlErr):
		return ErrorOffline
	}
	return ErrorOther
}

// Run runs the passes until ctx ends: one at once, then every minute and as soon as the emulator
// closes, and the ones asked for (see Wake). It returns when ctx ends.
func (s *Service) Run(ctx context.Context) {
	s.mutex.Lock()
	s.loadLocked()
	s.base = ctx
	wake := s.wakeLocked()
	s.mutex.Unlock()
	s.run(ctx, true)
	watch := s.Watch
	if watch <= 0 {
		watch = watchEvery
	}
	ticker := time.NewTicker(watch)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.mutex.Lock()
			if s.pairing != nil {
				s.pairing.cancel()
			}
			s.mutex.Unlock()
			return
		case <-wake:
			s.mutex.Lock()
			pass, look := s.wantPass, s.wantLook
			s.wantPass, s.wantLook = false, false
			s.mutex.Unlock()
			if pass || look {
				s.run(ctx, pass)
			}
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Service) wakeLocked() chan struct{} {
	if s.wake == nil {
		s.wake = make(chan struct{}, 1)
	}
	return s.wake
}

// Wake asks the loop for a pass (pass) or for a look at the saves only.
func (s *Service) Wake(pass bool) {
	s.mutex.Lock()
	if pass {
		s.wantPass = true
	} else {
		s.wantLook = true
	}
	wake := s.wakeLocked()
	s.mutex.Unlock()
	select {
	case wake <- struct{}{}:
	default:
	}
}

// tick looks whether the emulator opened or closed, and runs the pass of every minute.
func (s *Service) tick(ctx context.Context) {
	s.mutex.Lock()
	on := s.config.On && s.config.Paired()
	due := s.now().Sub(s.lastRun) >= s.every()
	folder, wasRunning := s.folder, s.running
	s.mutex.Unlock()
	if !on {
		return
	}
	if due {
		s.run(ctx, true)
		return
	}
	if folder == nil {
		return
	}
	running, err := s.isRunning(folder.Emulator)
	if err != nil || running == wasRunning {
		return
	}
	if !running {
		// the emulator closed: what it saved goes now, and what waited for it goes in
		s.run(ctx, true)
		return
	}
	s.mutex.Lock()
	s.running = true
	s.mutex.Unlock()
	s.changed()
}

// run makes a pass (sync) when the passes are on, else, paired, looks at the saves only.
func (s *Service) run(ctx context.Context, sync bool) {
	s.passes.Lock()
	defer s.passes.Unlock()
	s.mutex.Lock()
	s.loadLocked()
	config := s.config
	s.lastRun = s.now()
	if !config.Paired() {
		s.mutex.Unlock()
		return
	}
	sync = sync && config.On
	folder, err := s.findFolderLocked()
	if err != nil {
		s.folder = nil
		s.errText, s.errKind = err.Error(), kindOf(err)
		s.mutex.Unlock()
		s.changed()
		return
	}
	s.folder = &folder
	s.working = sync
	s.mutex.Unlock()
	if sync {
		s.changed()
	}

	library := s.library(config)
	state, err := edensync.LoadState(s.statePath())
	if err != nil {
		state = edensync.NewState()
	}
	report := edensync.Report{}
	var passErr error
	if sync {
		passErr = s.pass(ctx, folder, library, state, &report)
	} else if running, err := s.isRunning(folder.Emulator); err == nil {
		report.Running = running
	}
	seen, lookErr := look(ctx, folder, library, state, config.KeyName)

	s.mutex.Lock()
	s.working = false
	s.running = report.Running
	s.state = *state
	if lookErr == nil {
		s.view = seen
	}
	err = passErr
	if err == nil {
		err = lookErr
	}
	switch {
	case err != nil:
		s.errText, s.errKind = err.Error(), kindOf(err)
	case len(report.Errors) > 0:
		s.errText, s.errKind = report.Errors[0].Error(), ErrorGames
	default:
		s.errText, s.errKind = "", ""
	}
	s.mutex.Unlock()
	s.changed()

	if notice := noticeOf(report, seen.names()); s.Noticed != nil && (len(notice.Brought) > 0 || len(notice.Conflicts) > 0) {
		s.Noticed(notice)
	}
}

// pass runs a pass of edensync, as work NXPress waits for before it closes.
func (s *Service) pass(ctx context.Context, folder edensync.Folder, library edensync.Library, state *edensync.State, report *edensync.Report) error {
	if s.Busy != nil {
		done, err := s.Busy()
		if err != nil {
			return nil
		}
		defer done()
	}
	syncer := &edensync.Syncer{Folder: folder, Library: library, State: state, StatePath: s.statePath(),
		IsRunning: func() (bool, error) { return s.isRunning(folder.Emulator) }, Now: s.Now}
	got, err := syncer.Pass(ctx)
	*report = got
	return err
}

// names are the names of the games, by title ID, as the library knows them.
func (v view) names() map[string]string {
	names := map[string]string{}
	for _, game := range v.Games {
		if game.Name != "" {
			names[game.TitleId] = game.Name
		}
	}
	for _, conflict := range v.Conflicts {
		if conflict.Name != "" {
			names[conflict.TitleId] = conflict.Name
		}
	}
	return names
}

// noticeOf is the notice of a pass: the games brought and the conflicts told, by name, each once.
func noticeOf(report edensync.Report, names map[string]string) Notice {
	notice := Notice{Brought: []string{}, Conflicts: []string{}}
	add := func(list []string, item edensync.Item) []string {
		name := names[strings.ToUpper(item.TitleId)]
		if name == "" {
			name = strings.ToUpper(item.TitleId)
		}
		for _, known := range list {
			if known == name {
				return list
			}
		}
		return append(list, name)
	}
	for _, item := range report.Brought {
		notice.Brought = add(notice.Brought, item)
	}
	for _, item := range report.Conflicts {
		notice.Conflicts = add(notice.Conflicts, item)
	}
	return notice
}

// Pair asks the library at an address (found, or typed by hand) to pair this computer: the page
// shows the code, which the user checks in the library as they accept it. The key comes by
// itself once accepted (see waitPairing).
func (s *Service) Pair(ctx context.Context, typed string) (Pairing, error) {
	s.CancelPairing()
	s.mutex.Lock()
	s.pairErr = ""
	s.mutex.Unlock()
	found, err := Reach(ctx, nil, typed)
	if err != nil {
		s.mutex.Lock()
		s.pairErr = PairNotLibrary
		s.mutex.Unlock()
		s.changed()
		return Pairing{}, err
	}
	name := s.computerName()
	asked, err := edensync.AskPairing(ctx, s.client(), found.URL, name)
	if err != nil {
		return Pairing{}, err
	}
	s.mutex.Lock()
	base := s.base
	if base == nil {
		base = context.Background()
	}
	wait, cancel := context.WithTimeout(base, time.Duration(max(asked.Expires, 60))*time.Second)
	shown := Pairing{Code: asked.Code, Name: name, Library: Host(found.URL), Expires: s.now().Unix() + int64(asked.Expires)}
	s.pairing = &pairing{Pairing: shown, cancel: cancel}
	mine := s.pairing
	s.mutex.Unlock()
	s.changed()
	go s.waitPairing(wait, mine, found, asked.Id)
	return shown, nil
}

// waitPairing asks the library every few seconds whether the pairing was accepted, and keeps the
// key once it was. The library forgets the key once given, so it is written at once.
func (s *Service) waitPairing(ctx context.Context, mine *pairing, found Found, id string) {
	defer mine.cancel()
	every := s.PairEvery
	if every <= 0 {
		every = pairEvery
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	end := func(reason string) {
		s.mutex.Lock()
		if s.pairing == mine {
			s.pairing, s.pairErr = nil, reason
		}
		s.mutex.Unlock()
		s.changed()
	}
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				end(PairExpired)
			}
			return
		case <-ticker.C:
		}
		answer, err := edensync.CheckPairing(ctx, s.client(), found.URL, id)
		if err != nil {
			continue
		}
		switch answer.State {
		case edensync.PairWaiting:
			continue
		case edensync.PairRejected, edensync.PairExpired:
			end(answer.State)
			return
		case edensync.PairAccepted:
			if answer.Key == "" {
				end(PairExpired)
				return
			}
			s.mutex.Lock()
			if s.pairing != mine {
				s.mutex.Unlock()
				return
			}
			config := s.config
			config.Address, config.Id, config.Library = found.URL, found.Id, Host(found.URL)
			config.Key, config.KeyName, config.User = answer.Key, answer.Name, answer.User
			// off until the user turns it on
			config.On = false
			err := SaveConfig(s.Dir, config)
			if err == nil {
				s.config = config
				s.pairing, s.pairErr = nil, ""
				// a new library: what the passes remember of another one does not count
				os.Remove(s.statePath())
				s.state = edensync.State{}
				s.view = view{}
			} else {
				s.pairing, s.pairErr = nil, err.Error()
			}
			s.mutex.Unlock()
			s.changed()
			s.Wake(false)
			return
		}
	}
}

// CancelPairing stops waiting for the pairing asked.
func (s *Service) CancelPairing() {
	s.mutex.Lock()
	waiting := s.pairing
	s.pairing = nil
	s.mutex.Unlock()
	if waiting != nil {
		waiting.cancel()
		s.changed()
	}
}

// Forget forgets the library paired, its key and what the passes remember. The key stays in the
// library until it is deleted there, in API keys.
func (s *Service) Forget() error {
	s.mutex.Lock()
	s.loadLocked()
	config := s.config
	config.Address, config.Id, config.Library, config.Key, config.KeyName, config.User, config.On = "", "", "", "", "", "", false
	err := SaveConfig(s.Dir, config)
	if err == nil {
		s.config = config
		os.Remove(s.statePath())
		s.state, s.view = edensync.State{}, view{}
		s.errText, s.errKind, s.pairErr = "", "", ""
		s.running = false
	}
	s.mutex.Unlock()
	s.changed()
	return err
}

// SetOn turns the passes on or off; on, a pass runs at once.
func (s *Service) SetOn(on bool) error {
	s.mutex.Lock()
	s.loadLocked()
	if on && !s.config.Paired() {
		s.mutex.Unlock()
		return ErrNotPaired
	}
	config := s.config
	config.On = on
	err := SaveConfig(s.Dir, config)
	if err == nil {
		s.config = config
	}
	s.mutex.Unlock()
	s.changed()
	if err == nil && on {
		s.Wake(true)
	}
	return err
}

// UseFolder takes the data folder of the emulator chosen by hand ("" to find it by itself again).
func (s *Service) UseFolder(path string) error {
	s.mutex.Lock()
	s.loadLocked()
	config := s.config
	config.Folder, config.Emulator = "", ""
	if path != "" {
		emulator := guessEmulator(path)
		folder, err := edensync.Open(emulator, path)
		if err != nil {
			s.mutex.Unlock()
			return err
		}
		config.Folder, config.Emulator = folder.Root, string(emulator)
	}
	err := SaveConfig(s.Dir, config)
	if err == nil {
		s.config = config
		s.view = view{}
	}
	s.mutex.Unlock()
	s.changed()
	s.Wake(true)
	return err
}

// SetProfile pairs a profile of the emulator with an account of the console ("" for none),
// as the page Saves of the library does, and looks at the saves again.
func (s *Service) SetProfile(ctx context.Context, id, account string) error {
	s.mutex.Lock()
	s.loadLocked()
	config := s.config
	folder, err := s.findFolderLocked()
	s.mutex.Unlock()
	if !config.Paired() {
		return ErrNotPaired
	}
	if err != nil {
		return err
	}
	profiles, err := folder.Profiles()
	if err != nil {
		return err
	}
	reports := []edensync.ProfileReport{}
	found := false
	for _, profile := range profiles {
		report := edensync.ProfileReport{Profile: profile.ID.String(), Name: profile.Name}
		if profile.ID.String() == id {
			report.Account, found = &account, true
		}
		reports = append(reports, report)
	}
	if !found {
		return errors.New("this profile is no longer in the emulator")
	}
	if _, err := s.library(config).ReportProfiles(ctx, folder.Emulator, reports); err != nil {
		return err
	}
	s.Wake(true)
	return nil
}

// Keep tells the library which save the emulator keeps of a conflict (KeepConsole or
// KeepEmulator), as the page Saves does; the next pass applies it, at once when the emulator is
// closed.
func (s *Service) Keep(ctx context.Context, id, keep string) error {
	if keep != edensync.KeepConsole && keep != edensync.KeepEmulator {
		return errors.New("the choice is console or emulator")
	}
	s.mutex.Lock()
	s.loadLocked()
	config := s.config
	s.mutex.Unlock()
	if !config.Paired() {
		return ErrNotPaired
	}
	data, _ := json.Marshal(map[string]string{"keep": keep})
	address := strings.TrimRight(config.Address, "/") + "/install/saves/conflicts/" + url.PathEscape(id) + "/keep"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, address, bytes.NewReader(data))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+config.Key)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := s.client().Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	switch {
	case response.StatusCode >= 200 && response.StatusCode <= 299:
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusMethodNotAllowed:
		// a conflict gone is told in JSON; an older library has no such path
		var answer struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &answer) == nil && answer.Error != "" && response.StatusCode == http.StatusNotFound {
			s.Wake(false)
			return ErrConflictGone
		}
		return ErrKeepUnknown
	default:
		return &edensync.StatusError{Status: response.StatusCode, Message: strings.TrimSpace(strings.SplitN(string(body), "\n", 2)[0])}
	}
	s.Wake(true)
	return nil
}
