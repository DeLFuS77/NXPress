package edensaves

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The library is found on the local network as NXDock finds it (source/discovery.cpp of NXDock,
// web/netpair.go of the library): a UDP message to the port 45730 of the whole network, which the
// app answers with where it listens, and, when nobody answers (an app in Docker whose UDP port is
// not published), its usual ports on every address of the network. Each one is checked with
// GET /install/hello, which tells it is the app.

const (
	discoveryPort = 45730
	discoveryAsk  = "NXDOCK-DISCOVER 1"
	appName       = "switch-library-manager-web"
	// how long the answers to the UDP message are waited for
	udpWait = 1500 * time.Millisecond
	// how long an address is given to answer hello, to connect and in all
	probeConnect = 700 * time.Millisecond
	probeWait    = 1500 * time.Millisecond
	// the addresses asked at once
	probeParallel = 64
)

// the port of the app, and the one most installs publish it on
var usualPorts = []int{3000, 3030}

// Found is a library that answered.
type Found struct {
	URL     string `json:"url"`
	Id      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// it installs (and keeps saves) over the network now
	Enabled bool `json:"enabled"`
}

// Host is how the address of a library is shown: 192.168.1.2:3000.
func Host(address string) string {
	if parsed, err := url.Parse(address); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return address
}

// ErrNotLibrary tells an address where no library answers.
var ErrNotLibrary = errors.New("no Switch Library Manager Web answers at this address")

// probeClient is the client of a probe: connecting is quick on a local network, or never.
var probeClient = &http.Client{
	Timeout:   probeWait,
	Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: probeConnect}).DialContext, Proxy: nil},
}

// Hello asks an address whether it is the library: GET /install/hello.
func Hello(ctx context.Context, client *http.Client, address string) (Found, error) {
	if client == nil {
		client = probeClient
	}
	address = strings.TrimRight(address, "/")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address+"/install/hello", nil)
	if err != nil {
		return Found{}, err
	}
	request.Header.Set("Accept", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return Found{}, err
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	var hello struct {
		App     string `json:"app"`
		Id      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
		Enabled bool   `json:"enabled"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(data, &hello) != nil || hello.App != appName || hello.Id == "" {
		return Found{}, ErrNotLibrary
	}
	return Found{URL: address, Id: hello.Id, Name: hello.Name, Version: hello.Version, Enabled: hello.Enabled}, nil
}

// Addresses are the addresses a library typed by hand may be at: with http:// when it has no
// scheme, and with the usual ports of the app when it has no port.
func Addresses(typed string) []string {
	typed = strings.TrimRight(strings.TrimSpace(typed), "/")
	if typed == "" {
		return nil
	}
	if !strings.Contains(typed, "://") {
		typed = "http://" + typed
	}
	parsed, err := url.Parse(typed)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil
	}
	base := parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/")
	if parsed.Port() != "" {
		return []string{base}
	}
	list := []string{base}
	for _, port := range usualPorts {
		list = append(list, parsed.Scheme+"://"+net.JoinHostPort(parsed.Hostname(), strconv.Itoa(port))+strings.TrimRight(parsed.Path, "/"))
	}
	return list
}

// Reach finds the library at an address typed by hand.
func Reach(ctx context.Context, client *http.Client, typed string) (Found, error) {
	addresses := Addresses(typed)
	if len(addresses) == 0 {
		return Found{}, ErrNotLibrary
	}
	err := ErrNotLibrary
	for _, address := range addresses {
		found, tried := Hello(ctx, client, address)
		if tried == nil {
			return found, nil
		}
		if ctx.Err() != nil {
			return Found{}, ctx.Err()
		}
		err = tried
	}
	return Found{}, err
}

// network is an IPv4 network of this computer.
type network struct {
	ip   net.IP
	mask net.IPMask
}

func (n network) broadcast() net.IP {
	ip := n.ip.To4()
	out := make(net.IP, 4)
	for i := range out {
		out[i] = ip[i] | ^n.mask[i]
	}
	return out
}

// localNetworks are the private IPv4 networks of the interfaces of this computer that are up.
func localNetworks() []network {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	list := []network{}
	for _, face := range interfaces {
		if face.Flags&net.FlagUp == 0 || face.Flags&net.FlagLoopback != 0 {
			continue
		}
		addresses, err := face.Addrs()
		if err != nil {
			continue
		}
		for _, address := range addresses {
			prefix, ok := address.(*net.IPNet)
			if !ok {
				continue
			}
			ip := prefix.IP.To4()
			if ip == nil || !ip.IsPrivate() || len(prefix.Mask) != net.IPv4len {
				continue
			}
			list = append(list, network{ip: ip, mask: prefix.Mask})
		}
	}
	return list
}

// Discover finds the libraries on the local network: by the UDP message first, then, when
// nobody answered, on the usual ports of every address of the networks of this computer.
func Discover(ctx context.Context) []Found {
	networks := localNetworks()
	targets := []*net.UDPAddr{{IP: net.IPv4bcast, Port: discoveryPort}}
	for _, n := range networks {
		targets = append(targets, &net.UDPAddr{IP: n.broadcast(), Port: discoveryPort})
	}
	found := askByUDP(ctx, targets, udpWait)
	if len(found) == 0 && ctx.Err() == nil {
		found = askEveryAddress(ctx, scanTargets(networks))
	}
	sort.Slice(found, func(i, j int) bool { return found[i].URL < found[j].URL })
	return found
}

// askByUDP sends the question of NXDock to the targets and checks the libraries that answer
// within wait, on the port they tell and on the usual ones (in Docker the app may be published on
// another port than its own).
func askByUDP(ctx context.Context, targets []*net.UDPAddr, wait time.Duration) []Found {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil
	}
	defer conn.Close()
	for _, target := range targets {
		conn.WriteToUDP([]byte(discoveryAsk), target)
	}
	deadline := time.Now().Add(wait)
	conn.SetReadDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { conn.SetReadDeadline(time.Now()) })
	defer stop()
	found := []Found{}
	asked := map[string]bool{}
	buffer := make([]byte, 2048)
	for {
		n, from, err := conn.ReadFromUDP(buffer)
		if err != nil {
			break
		}
		var answer struct {
			App  string `json:"app"`
			Port int    `json:"port"`
		}
		if json.Unmarshal(buffer[:n], &answer) != nil || answer.App != appName {
			continue
		}
		ports := []int{}
		if answer.Port > 0 && answer.Port < 65536 {
			ports = append(ports, answer.Port)
		}
		for _, port := range usualPorts {
			if port != answer.Port {
				ports = append(ports, port)
			}
		}
		for _, port := range ports {
			address := "http://" + net.JoinHostPort(from.IP.String(), strconv.Itoa(port))
			if asked[address] {
				break
			}
			asked[address] = true
			if library, err := Hello(ctx, nil, address); err == nil {
				found = addFound(found, library)
				break
			}
		}
	}
	return found
}

// addFound adds a library once: the same app answers on more than one address at times.
func addFound(found []Found, library Found) []Found {
	for _, known := range found {
		if known.Id == library.Id {
			return found
		}
	}
	return append(found, library)
}

// scanTargets are the addresses asked on every address of the networks: of a big network, the
// /24 of this computer only, enough at home and quick.
func scanTargets(networks []network) []string {
	targets := []string{}
	seen := map[string]bool{}
	for _, n := range networks {
		ip := n.ip.To4()
		ones, bits := n.mask.Size()
		if bits != 32 || ones > 30 {
			// a network of one or two addresses has nobody else on it
			continue
		}
		ones = max(ones, 24)
		own := binary.BigEndian.Uint32(ip)
		first := own &^ (1<<(32-ones) - 1)
		last := first | (1<<(32-ones) - 1)
		for host := first + 1; host < last; host++ {
			if host == own {
				continue
			}
			text := net.IPv4(byte(host>>24), byte(host>>16), byte(host>>8), byte(host)).String()
			for _, port := range usualPorts {
				address := "http://" + net.JoinHostPort(text, strconv.Itoa(port))
				if !seen[address] {
					seen[address] = true
					targets = append(targets, address)
				}
			}
		}
	}
	return targets
}

// askEveryAddress asks hello of every address given, a few at a time.
func askEveryAddress(ctx context.Context, targets []string) []Found {
	var mutex sync.Mutex
	found := []Found{}
	work := make(chan string)
	var group sync.WaitGroup
	for range min(probeParallel, len(targets)) {
		group.Go(func() {
			for address := range work {
				if library, err := Hello(ctx, nil, address); err == nil {
					mutex.Lock()
					found = addFound(found, library)
					mutex.Unlock()
				}
			}
		})
	}
	for _, address := range targets {
		if ctx.Err() != nil {
			break
		}
		work <- address
	}
	close(work)
	group.Wait()
	return found
}
