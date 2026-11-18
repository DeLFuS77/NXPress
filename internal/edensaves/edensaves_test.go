package edensaves

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dtrunk90/switch-library-manager-web/edensync"
)

// a made-up profile of Eden and a made-up game
var (
	player = edensync.ProfileID{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x01}
	title  = "0100000000010000"
)

const testKey = "the-key-of-this-computer-0123456789"

// fakeLibrary is Switch Library Manager Web as a computer sees it, in memory: finding, pairing
// and the paths of the saves of a key of the role PC.
type fakeLibrary struct {
	mutex     sync.Mutex
	saves     []edensync.LibrarySave
	zips      map[string][]byte
	pairs     edensync.Pairs
	conflicts []edensync.Conflict
	clock     int64
	// the pairing: answered once accepted or refused
	accept, refuse bool
	asked          string
	// an older library, without the path of the choice
	noKeep bool
}

func newFakeLibrary() *fakeLibrary {
	return &fakeLibrary{zips: map[string][]byte{}, clock: time.Now().Unix() - 3600, pairs: edensync.Pairs{Accounts: []string{}, Profiles: []edensync.PairedProfile{}}}
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := zip.NewWriter(&out)
	for name, content := range files {
		file, _ := writer.Create(name)
		file.Write([]byte(content))
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// addConsole keeps a save of a console, of an account.
func (l *fakeLibrary) addConsole(t *testing.T, account string, files map[string]string) edensync.LibrarySave {
	t.Helper()
	data := zipOf(t, files)
	l.mutex.Lock()
	defer l.mutex.Unlock()
	l.clock += 10
	save := edensync.LibrarySave{TitleId: title, Game: "Moss Knights", Id: fmt.Sprintf("%016x", l.clock), When: l.clock, Account: account, Console: "Switch", Size: int64(len(data))}
	l.saves = append(l.saves, save)
	l.zips[save.Id] = data
	if !contains(l.pairs.Accounts, account) {
		l.pairs.Accounts = append(l.pairs.Accounts, account)
	}
	return save
}

func contains(list []string, text string) bool {
	for _, known := range list {
		if known == text {
			return true
		}
	}
	return false
}

func (l *fakeLibrary) openConflicts() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	return len(l.conflicts)
}

func (l *fakeLibrary) emulatorSaves() int {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	count := 0
	for _, save := range l.saves {
		if save.Emulator {
			count++
		}
	}
	return count
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func (l *fakeLibrary) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /install/hello", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"app": appName, "id": "0123456789abcdef0123456789abcdef", "name": "Switch Library Manager", "version": "2.0.17", "port": 3000, "enabled": true})
	})
	mux.HandleFunc("POST /install/pair", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Console, Kind string }
		json.NewDecoder(r.Body).Decode(&body)
		l.mutex.Lock()
		l.asked = body.Kind + ":" + body.Console
		l.mutex.Unlock()
		writeJSON(w, 201, map[string]any{"id": "00112233445566778899aabbccddeeff", "code": "ABCD-EFGH", "expires": 600})
	})
	mux.HandleFunc("GET /install/pair/{id}", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		accept, refuse := l.accept, l.refuse
		l.mutex.Unlock()
		switch {
		case accept:
			writeJSON(w, 200, map[string]any{"state": "accepted", "key": testKey, "name": "NXPress on DESK-1", "user": "bob"})
		case refuse:
			writeJSON(w, 403, map[string]any{"state": "rejected"})
		default:
			writeJSON(w, 202, map[string]any{"state": "waiting"})
		}
	})
	saves := http.NewServeMux()
	saves.HandleFunc("GET /install/saves", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		list := []edensync.LibrarySave{}
		for _, save := range l.saves {
			data := l.zips[save.Id]
			save.Hash, _ = edensync.HashZip(bytes.NewReader(data), int64(len(data)))
			list = append(list, save)
		}
		writeJSON(w, 200, map[string]any{"saves": list})
	})
	saves.HandleFunc("GET /install/saves/{title}/{id}", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		data, ok := l.zips[r.PathValue("id")]
		l.mutex.Unlock()
		if !ok {
			writeJSON(w, 404, map[string]string{"error": "no such save"})
			return
		}
		w.Write(data)
	})
	saves.HandleFunc("POST /install/saves/{title}", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		query := r.URL.Query()
		l.mutex.Lock()
		defer l.mutex.Unlock()
		l.clock += 10
		save := edensync.LibrarySave{TitleId: r.PathValue("title"), Game: "Moss Knights", Id: fmt.Sprintf("%016x", l.clock), When: l.clock, Account: query.Get("account"),
			Console: "Eden", Size: int64(len(data)), Emulator: true, Profile: query.Get("profile"), Device: "NXPress on DESK-1", Auto: query.Get("auto") == "1"}
		l.saves = append(l.saves, save)
		l.zips[save.Id] = data
		writeJSON(w, 200, map[string]any{"id": save.Id, "when": save.When, "size": save.Size})
	})
	saves.HandleFunc("GET /install/saves/profiles", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		writeJSON(w, 200, l.pairs)
	})
	saves.HandleFunc("PUT /install/saves/profiles", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Emulator string
			Profiles []edensync.ProfileReport
		}
		json.NewDecoder(r.Body).Decode(&body)
		l.mutex.Lock()
		defer l.mutex.Unlock()
		kept := []edensync.PairedProfile{}
		for _, report := range body.Profiles {
			paired := edensync.PairedProfile{Profile: report.Profile, Name: report.Name, Emulator: body.Emulator, Mine: true}
			for _, old := range l.pairs.Profiles {
				if old.Profile == report.Profile {
					paired.Account, paired.Chosen = old.Account, old.Chosen
				}
			}
			if report.Account != nil {
				paired.Account, paired.Chosen = *report.Account, true
			}
			kept = append(kept, paired)
		}
		l.pairs.Profiles = kept
		writeJSON(w, 200, l.pairs)
	})
	saves.HandleFunc("GET /install/saves/conflicts", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		writeJSON(w, 200, map[string]any{"conflicts": append([]edensync.Conflict{}, l.conflicts...)})
	})
	saves.HandleFunc("POST /install/saves/conflicts", func(w http.ResponseWriter, r *http.Request) {
		var conflict edensync.Conflict
		json.NewDecoder(r.Body).Decode(&conflict)
		l.mutex.Lock()
		defer l.mutex.Unlock()
		conflict.Id = fmt.Sprintf("%016x", len(l.conflicts)+1)
		l.conflicts = append(l.conflicts, conflict)
		writeJSON(w, 200, map[string]string{"id": conflict.Id})
	})
	saves.HandleFunc("POST /install/saves/conflicts/{id}/done", func(w http.ResponseWriter, r *http.Request) {
		l.mutex.Lock()
		defer l.mutex.Unlock()
		for i, conflict := range l.conflicts {
			if conflict.Id == r.PathValue("id") {
				l.conflicts = append(l.conflicts[:i], l.conflicts[i+1:]...)
				writeJSON(w, 200, map[string]bool{"done": true})
				return
			}
		}
		writeJSON(w, 404, map[string]string{"error": "this choice is no longer asked"})
	})
	saves.HandleFunc("POST /install/saves/conflicts/{id}/keep", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Keep string }
		json.NewDecoder(r.Body).Decode(&body)
		l.mutex.Lock()
		defer l.mutex.Unlock()
		if l.noKeep {
			http.NotFound(w, r)
			return
		}
		for i, conflict := range l.conflicts {
			if conflict.Id == r.PathValue("id") {
				l.conflicts[i].Keep, l.conflicts[i].Decided = body.Keep, time.Now().Unix()
				writeJSON(w, 200, l.conflicts[i])
				return
			}
		}
		writeJSON(w, 404, map[string]string{"error": "this choice is no longer asked"})
	})
	// the saves only with the key of the computer, as the library asks
	mux.HandleFunc("/install/saves/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		saves.ServeHTTP(w, r)
	})
	mux.HandleFunc("/install/saves", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		saves.ServeHTTP(w, r)
	})
	return mux
}

// fakeEden is a data folder of Eden with the profile Player and, when files are given, a save of
// the game.
func fakeEden(t *testing.T, files map[string]string) (edensync.Folder, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "eden")
	folder := edensync.Folder{Emulator: edensync.Eden, Root: root, Nand: filepath.Join(root, "nand")}
	profiles := filepath.Join(folder.Nand, "system", "save", "8000000000000010", "su", "avators", "profiles.dat")
	os.MkdirAll(filepath.Dir(profiles), 0o755)
	if err := os.WriteFile(profiles, edensync.ProfilesFile([]edensync.Profile{{ID: player, Name: "Player"}}), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := folder.SaveFolder(player, title)
	if files != nil {
		os.MkdirAll(dir, 0o755)
		for name, content := range files {
			os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644)
		}
	}
	return folder, dir
}

// waitFor waits for a condition, failing after a few seconds.
func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("waited too long for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// The settings, with the key, are written readable by the user only and read back; the status
// the page gets never holds the key.
func TestConfigKeepsTheKeyPrivate(t *testing.T) {
	dir := t.TempDir()
	config := Config{Address: "http://192.168.1.2:3000", Library: "192.168.1.2:3000", Key: testKey, User: "bob", On: true}
	if err := SaveConfig(dir, config); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, ConfigName))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("the settings are %v, %v", info.Mode().Perm(), err)
		}
	}
	read, err := LoadConfig(dir)
	if err != nil || read != config {
		t.Fatalf("read back: %+v, %v", read, err)
	}
	service := &Service{Dir: dir, FindFolders: func() []edensync.Folder { return nil }}
	data, _ := json.Marshal(service.Status())
	if strings.Contains(string(data), testKey) || !strings.Contains(string(data), `"paired":true`) {
		t.Fatalf("the status: %s", data)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, ".*.tmp")); len(left) > 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

// Pairing: the address typed without its scheme, the code shown, the name of the computer sent
// with the kind pc, and the key kept once the library accepts it; the passes stay off until
// turned on. Refused, it says so and keeps nothing.
func TestPairing(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	dir := t.TempDir()
	statuses := atomic.Int32{}
	service := &Service{Dir: dir, PairEvery: 10 * time.Millisecond, Computer: func() string { return "DESK-1" },
		FindFolders: func() []edensync.Folder { return nil }, Changed: func(Status) { statuses.Add(1) }}

	shown, err := service.Pair(context.Background(), strings.TrimPrefix(server.URL, "http://"))
	if err != nil || shown.Code != "ABCD-EFGH" || shown.Name != "NXPress on DESK-1" || shown.Library != Host(server.URL) {
		t.Fatalf("asked: %+v, %v", shown, err)
	}
	library.mutex.Lock()
	asked := library.asked
	library.mutex.Unlock()
	if asked != "pc:NXPress on DESK-1" {
		t.Fatalf("the library was asked %q", asked)
	}
	if status := service.Status(); status.Pairing == nil || status.Paired {
		t.Fatalf("waiting: %+v", status)
	}
	library.mutex.Lock()
	library.accept = true
	library.mutex.Unlock()
	waitFor(t, "the key", func() bool { return service.Status().Paired })
	config, _ := LoadConfig(dir)
	if config.Key != testKey || config.Address != server.URL || config.User != "bob" || config.On {
		t.Fatalf("kept: %+v", config)
	}
	status := service.Status()
	if status.Pairing != nil || status.User != "bob" || status.On || statuses.Load() == 0 {
		t.Fatalf("paired: %+v", status)
	}

	// refused
	other := newFakeLibrary()
	other.refuse = true
	refusing := httptest.NewServer(other.handler())
	defer refusing.Close()
	dir = t.TempDir()
	service = &Service{Dir: dir, PairEvery: 10 * time.Millisecond, FindFolders: func() []edensync.Folder { return nil }}
	if _, err := service.Pair(context.Background(), refusing.URL); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the refusal", func() bool { return service.Status().PairError == PairRejected })
	if config, _ := LoadConfig(dir); config.Key != "" || service.Status().Paired {
		t.Fatal("a refusal keeps no key")
	}

	// an address where no library answers
	if _, err := service.Pair(context.Background(), "http://127.0.0.1:1"); err == nil || service.Status().PairError != PairNotLibrary {
		t.Fatalf("no library: %v, %q", err, service.Status().PairError)
	}
}

// pairedService is a service paired with the fake library, on the passes, for the folder given.
func pairedService(t *testing.T, server *httptest.Server, folder edensync.Folder, running *atomic.Bool) *Service {
	t.Helper()
	dir := t.TempDir()
	if err := SaveConfig(dir, Config{Address: server.URL, Library: Host(server.URL), Key: testKey, KeyName: "NXPress on DESK-1", User: "bob", On: true}); err != nil {
		t.Fatal(err)
	}
	return &Service{Dir: dir, Watch: 10 * time.Millisecond, Every: time.Hour,
		FindFolders: func() []edensync.Folder { return []edensync.Folder{folder} },
		IsRunning:   func(edensync.Emulator) (bool, error) { return running.Load(), nil }}
}

// The loop: a pass at once uploads the save of Eden; with Eden open nothing moves, and the newer
// save of the console waits for it to close; once it closes it goes in at once, with a notice.
// Both changed: a conflict, told once by a notice, shown with both saves, and chosen from
// NXPress; the next pass applies it.
func TestPassesWhileNXPressIsOpen(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	library.addConsole(t, "Bob", map[string]string{"data.bin": "old of the console"})
	folder, dir := fakeEden(t, map[string]string{"data.bin": "played in Eden"})
	running := &atomic.Bool{}
	service := pairedService(t, server, folder, running)
	var mutex sync.Mutex
	notices := []Notice{}
	service.Noticed = func(notice Notice) { mutex.Lock(); notices = append(notices, notice); mutex.Unlock() }
	busy := atomic.Int32{}
	service.Busy = func() (func(), error) { busy.Add(1); return func() {}, nil }
	noticed := func() []Notice { mutex.Lock(); defer mutex.Unlock(); return append([]Notice{}, notices...) }

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() { service.Run(ctx); close(done) }()

	// never in step with the library and different from the console: both kept, the user asks
	waitFor(t, "the first pass", func() bool { return service.Status().LastPass > 0 })
	status := service.Status()
	if library.emulatorSaves() != 1 || len(status.Conflicts) != 1 || busy.Load() == 0 {
		t.Fatalf("first pass: %d saves of Eden, %+v", library.emulatorSaves(), status)
	}
	if status.Profiles[0].Account != "Bob" || status.Profiles[0].Name != "Player" || status.Accounts[0] != "Bob" {
		t.Fatalf("the only profile goes with the only account: %+v", status.Profiles)
	}
	conflict := status.Conflicts[0]
	if conflict.Name != "Moss Knights" || len(conflict.Saves) != 2 || conflict.Saves[0].Keep != edensync.KeepEmulator || conflict.Saves[0].Source != "Eden" ||
		conflict.Saves[1].Source != "Switch" || !conflict.Saves[1].Found || conflict.Saves[1].Size == 0 {
		t.Fatalf("the conflict: %+v", conflict)
	}
	if len(status.Games) != 1 || status.Games[0].State != GameConflict {
		t.Fatalf("the games: %+v", status.Games)
	}
	if got := noticed(); len(got) != 1 || len(got[0].Conflicts) != 1 || got[0].Conflicts[0] != "Moss Knights" {
		t.Fatalf("the notice: %+v", got)
	}

	// chosen in NXPress: the save of Eden stays, and the next pass closes the conflict
	if err := service.Keep(context.Background(), conflict.Id, edensync.KeepEmulator); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the choice applied", func() bool { return len(service.Status().Conflicts) == 0 && library.openConflicts() == 0 })
	if content, _ := os.ReadFile(filepath.Join(dir, "data.bin")); string(content) != "played in Eden" {
		t.Fatalf("Eden keeps its save: %q", content)
	}
	waitFor(t, "in step", func() bool { games := service.Status().Games; return len(games) == 1 && games[0].State == GameInStep })

	// Eden open: the newer save of the console waits
	running.Store(true)
	waitFor(t, "Eden open", func() bool { return service.Status().Running })
	library.addConsole(t, "Bob", map[string]string{"data.bin": "new of the console"})
	service.Wake(true)
	waitFor(t, "the newer save seen", func() bool {
		games := service.Status().Games
		return len(games) == 1 && games[0].State == GameConsoleNewer
	})
	if content, _ := os.ReadFile(filepath.Join(dir, "data.bin")); string(content) != "played in Eden" {
		t.Fatal("nothing goes in while Eden is open")
	}

	// Eden closes: in at once, the save replaced being in the library already
	before := library.emulatorSaves()
	running.Store(false)
	waitFor(t, "brought", func() bool {
		content, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
		return string(content) == "new of the console"
	})
	waitFor(t, "the status after", func() bool { status := service.Status(); return !status.Running && status.Brought == 1 })
	if library.emulatorSaves() != before {
		t.Fatalf("the save replaced was in the library: %d saves of Eden, %d before", library.emulatorSaves(), before)
	}
	waitFor(t, "the notice of what was brought", func() bool { got := noticed(); return len(got) == 2 && len(got[1].Brought) == 1 })

	// both changed: a conflict chosen for the console, which goes in
	os.WriteFile(filepath.Join(dir, "data.bin"), []byte("played again"), 0o644)
	library.addConsole(t, "Bob", map[string]string{"data.bin": "played on the console"})
	service.Wake(true)
	waitFor(t, "the second conflict", func() bool { return len(service.Status().Conflicts) == 1 })
	if err := service.Keep(context.Background(), service.Status().Conflicts[0].Id, edensync.KeepConsole); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the console kept", func() bool {
		content, _ := os.ReadFile(filepath.Join(dir, "data.bin"))
		return string(content) == "played on the console"
	})
	stop()
	<-done
}

// writeLock writes the lock of the folder of Eden as a program on this computer would.
func writeLock(t *testing.T, folder edensync.Folder, owner, computer string) {
	t.Helper()
	data, _ := json.Marshal(edensync.Lock{Owner: owner, Computer: computer, Pid: os.Getpid(), Updated: time.Now().Unix()})
	if err := os.WriteFile(filepath.Join(folder.Root, edensync.LockName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Switch Library Manager Web keeping the saves of Eden in step on this computer: the status tells
// it and the passes wait; once it lets go, NXPress takes them, and gives them up as it is turned
// off and as it stops.
func TestWaitsForTheLibraryOnThisComputer(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	folder, _ := fakeEden(t, map[string]string{"data.bin": "played in Eden"})
	computer, _ := os.Hostname()
	writeLock(t, folder, edensync.OwnerLibrary, computer)
	lockPath := filepath.Join(folder.Root, edensync.LockName)
	service := pairedService(t, server, folder, &atomic.Bool{})
	passes := atomic.Int32{}
	service.Busy = func() (func(), error) { passes.Add(1); return func() {}, nil }
	if status := service.Status(); status.HeldBy != edensync.OwnerLibrary {
		t.Fatalf("told before any pass: %+v", status)
	}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	done := make(chan struct{})
	go func() { service.Run(ctx); close(done) }()
	waitFor(t, "the first pass", func() bool { return passes.Load() > 0 && !service.Status().Working })
	status := service.Status()
	if status.HeldBy != edensync.OwnerLibrary || status.LastPass != 0 || library.emulatorSaves() != 0 || status.ErrorKind != "" {
		t.Fatalf("the passes wait: %+v, %d saves of Eden", status, library.emulatorSaves())
	}

	// turned off in the library
	os.Remove(lockPath)
	service.Wake(true)
	waitFor(t, "NXPress takes them", func() bool { return service.Status().LastPass > 0 })
	if lock, ok := edensync.ReadLock(folder); !ok || lock.Owner != edensync.OwnerNXPress || library.emulatorSaves() != 1 || service.Status().HeldBy != "" {
		t.Fatalf("the lock: %+v, %d saves of Eden", lock, library.emulatorSaves())
	}

	if err := service.SetOn(false); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the lock given up as it is turned off", func() bool { _, err := os.Stat(lockPath); return os.IsNotExist(err) })
	if err := service.SetOn(true); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the lock taken again", func() bool { lock, ok := edensync.ReadLock(folder); return ok && lock.Owner == edensync.OwnerNXPress })
	stop()
	<-done
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("the lock given up as NXPress stops: %v", err)
	}

	// NXPress of another computer with the same folder is not told as the library
	writeLock(t, folder, edensync.OwnerNXPress, "ANOTHER-COMPUTER")
	if status := service.Status(); status.HeldBy != "" {
		t.Fatalf("held by: %q", status.HeldBy)
	}
}

// Without a folder of Eden, or with the key refused, the status tells why; turned off, no pass
// writes, and the profiles are still shown and paired from NXPress.
func TestStatusTellsWhatIsMissing(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	library.addConsole(t, "Bob", map[string]string{"data.bin": "of the console"})
	library.addConsole(t, "Ann", map[string]string{"data.bin": "of another user"})
	running := &atomic.Bool{}

	service := pairedService(t, server, edensync.Folder{}, running)
	service.FindFolders = func() []edensync.Folder { return nil }
	service.run(context.Background(), true)
	if status := service.Status(); status.ErrorKind != ErrorFolder || status.Folder != "" {
		t.Fatalf("no folder: %+v", status)
	}

	folder, dir := fakeEden(t, nil)
	service = pairedService(t, server, folder, running)
	if err := service.SetOn(false); err != nil {
		t.Fatal(err)
	}
	service.run(context.Background(), true)
	status := service.Status()
	if status.On || status.ErrorKind != "" || len(status.Profiles) != 1 || len(status.Accounts) != 2 || status.Folder != folder.Root || status.Emulator != "Eden" {
		t.Fatalf("off: %+v", status)
	}
	if isDir(dir) || library.emulatorSaves() != 0 {
		t.Fatal("a pass ran while off")
	}
	// two accounts: nothing is paired by itself, the user chooses
	if status.Profiles[0].Account != "" {
		t.Fatalf("paired by itself: %+v", status.Profiles)
	}
	if err := service.SetProfile(context.Background(), player.String(), "Ann"); err != nil {
		t.Fatal(err)
	}
	service.run(context.Background(), false)
	if status := service.Status(); status.Profiles[0].Account != "Ann" || len(status.Games) != 1 || status.Games[0].State != GameOpenOnce {
		t.Fatalf("paired with Ann: %+v", status)
	}

	// the key no longer taken
	config, _ := LoadConfig(service.Dir)
	config.Key = "another"
	SaveConfig(service.Dir, config)
	service = &Service{Dir: service.Dir, FindFolders: service.FindFolders, IsRunning: service.IsRunning}
	service.run(context.Background(), false)
	if status := service.Status(); status.ErrorKind != ErrorKey {
		t.Fatalf("the key refused: %+v", status)
	}
	if kindOf(&net.OpError{Op: "dial", Err: os.ErrDeadlineExceeded}) != ErrorOffline {
		t.Fatal("a library that cannot be reached")
	}
	// the network install off, told apart from a save that is gone
	if kindOf(&edensync.StatusError{Status: 404, Message: "404 page not found"}) != ErrorNetOff || kindOf(&edensync.StatusError{Status: 404, Message: "no such save"}) != ErrorOther {
		t.Fatal("a path not found")
	}

	// forgotten: no key, no state, off
	if err := service.Forget(); err != nil {
		t.Fatal(err)
	}
	if config, _ := LoadConfig(service.Dir); config.Key != "" || config.On || service.Status().Paired {
		t.Fatalf("forgotten: %+v", config)
	}
}

// An older library has no path for the choice: NXPress says to choose on its page; a choice no
// longer asked is told apart.
func TestKeepOnAnOlderLibrary(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	folder, _ := fakeEden(t, nil)
	service := pairedService(t, server, folder, &atomic.Bool{})
	if err := service.Keep(context.Background(), "00000000000000aa", edensync.KeepConsole); err != ErrConflictGone {
		t.Fatalf("gone: %v", err)
	}
	library.mutex.Lock()
	library.noKeep = true
	library.mutex.Unlock()
	if err := service.Keep(context.Background(), "00000000000000aa", edensync.KeepConsole); err != ErrKeepUnknown {
		t.Fatalf("older: %v", err)
	}
	if err := service.Keep(context.Background(), "00000000000000aa", "both"); err == nil {
		t.Fatal("an unknown choice")
	}
}

// Finding the library: the UDP question of NXDock answered with the port of the app, checked
// with hello; and the addresses asked one by one when nobody answers.
func TestDiscovery(t *testing.T) {
	library := newFakeLibrary()
	server := httptest.NewServer(library.handler())
	defer server.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))

	answering, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Skip(err)
	}
	defer answering.Close()
	go func() {
		buffer := make([]byte, 256)
		for {
			n, from, err := answering.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			if string(buffer[:n]) == discoveryAsk {
				answering.WriteToUDP([]byte(`{"app":"switch-library-manager-web","id":"x","port":`+port+`}`), from)
			}
		}
	}()
	found := askByUDP(context.Background(), []*net.UDPAddr{answering.LocalAddr().(*net.UDPAddr)}, 500*time.Millisecond)
	if len(found) != 1 || found[0].URL != server.URL || found[0].Version != "2.0.17" || !found[0].Enabled {
		t.Fatalf("found by UDP: %+v", found)
	}

	if found := askEveryAddress(context.Background(), []string{server.URL, "http://127.0.0.1:1"}); len(found) != 1 || found[0].URL != server.URL {
		t.Fatalf("found by its port: %+v", found)
	}
	targets := scanTargets([]network{{ip: net.IPv4(192, 168, 1, 20).To4(), mask: net.CIDRMask(24, 32)}})
	if len(targets) != 253*len(usualPorts) || contains(targets, "http://192.168.1.20:3000") || !contains(targets, "http://192.168.1.254:3030") || contains(targets, "http://192.168.1.255:3000") {
		t.Fatalf("the addresses of a /24: %d, %v", len(targets), targets[:4])
	}
	if targets := scanTargets([]network{{ip: net.IPv4(10, 1, 2, 3).To4(), mask: net.CIDRMask(8, 32)}}); len(targets) != 253*len(usualPorts) || !contains(targets, "http://10.1.2.1:3000") {
		t.Fatalf("a big network is looked at in its /24: %d", len(targets))
	}
	if targets := scanTargets([]network{{ip: net.IPv4(10, 1, 2, 3).To4(), mask: net.CIDRMask(32, 32)}}); len(targets) != 0 {
		t.Fatalf("a network of one address: %v", targets)
	}

	// typed by hand
	for typed, want := range map[string][]string{
		"nas":                     {"http://nas", "http://nas:3000", "http://nas:3030"},
		"192.168.1.2:3030/":       {"http://192.168.1.2:3030"},
		"https://library.example": {"https://library.example", "https://library.example:3000", "https://library.example:3030"},
		"ftp://nas":               nil,
		" ":                       nil,
	} {
		if got := Addresses(typed); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%q: %v", typed, got)
		}
	}
	if found, err := Reach(context.Background(), nil, "127.0.0.1:"+port); err != nil || found.URL != server.URL {
		t.Fatalf("reached: %+v, %v", found, err)
	}
	if _, err := Reach(context.Background(), nil, "127.0.0.1:1"); err == nil {
		t.Fatal("nothing at that address")
	}
	if Host("http://192.168.1.2:3000") != "192.168.1.2:3000" {
		t.Fatal("the host")
	}
	// the networks of this computer are only read
	for _, n := range localNetworks() {
		if !n.ip.IsPrivate() {
			t.Fatalf("a network that is not private: %v", n.ip)
		}
	}
}
