package jobs

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/dtrunk90/switch-library-manager-web/switchfs"
)

// Npdm is what main.npdm tells of a program: who it is and what the system lets it do. It has a
// META header, an ACI0 (what the program asks for) and an ACID (the bounds Nintendo signs).
type Npdm struct {
	Name string `json:"name"`
	// "64-bit" or "32-bit", and the address space it asks for (32, 36 or 39 bits)
	Bits         string `json:"bits"`
	AddressSpace int    `json:"addressSpace"`
	// the main thread: its priority, the core it starts on and its stack
	Priority  int    `json:"priority"`
	Core      int    `json:"core"`
	Stack     uint32 `json:"stack"`
	ProgramID string `json:"programId"`
	// the program IDs the ACID lets it be, and whether it is for retail consoles
	ProgramMin string `json:"programMin"`
	ProgramMax string `json:"programMax"`
	Production bool   `json:"production"`
	// the permissions to the file systems, as a mask of bits
	FileSystem uint64 `json:"fileSystem"`
	// the services it may use, and those it offers itself
	Services []string `json:"services"`
	Hosts    []string `json:"hosts,omitempty"`
	Kernel   Kernel   `json:"kernel"`
}

// Kernel is what the kernel capabilities of the ACI0 let the program do.
type Kernel struct {
	// the priorities its threads may take and the cores they may run on
	LowestPriority  int `json:"lowestPriority"`
	HighestPriority int `json:"highestPriority"`
	MinCore         int `json:"minCore"`
	MaxCore         int `json:"maxCore"`
	// the system calls it may make, by number
	SystemCalls []int `json:"systemCalls"`
	// the memory and the interrupts of devices it maps
	MemoryMaps int   `json:"memoryMaps"`
	Interrupts []int `json:"interrupts,omitempty"`
	// "application", "applet" or "system"
	ProgramType   string `json:"programType,omitempty"`
	KernelVersion string `json:"kernelVersion,omitempty"`
	HandleTable   int    `json:"handleTable,omitempty"`
	AllowDebug    bool   `json:"allowDebug,omitempty"`
	ForceDebug    bool   `json:"forceDebug,omitempty"`
}

// ParseNpdm reads main.npdm. Its offsets are all checked against its size: a damaged one is
// refused, not read past.
func ParseNpdm(data []byte) (Npdm, error) {
	npdm := Npdm{}
	if len(data) < 0x80 || string(data[:4]) != "META" {
		return npdm, errors.New("main.npdm has no META header")
	}
	flags := data[0xC]
	npdm.Bits = "32-bit"
	if flags&1 != 0 {
		npdm.Bits = "64-bit"
	}
	npdm.AddressSpace = map[byte]int{0: 32, 1: 36, 2: 32, 3: 39}[flags>>1&7]
	npdm.Priority = int(data[0xE])
	npdm.Core = int(data[0xF])
	npdm.Stack = binary.LittleEndian.Uint32(data[0x1C:])
	npdm.Name = strings.TrimRight(string(data[0x20:0x30]), "\x00")
	aci, err := region(data, 0x70)
	if err != nil {
		return npdm, fmt.Errorf("ACI0: %w", err)
	}
	acid, err := region(data, 0x78)
	if err != nil {
		return npdm, fmt.Errorf("ACID: %w", err)
	}
	if len(aci) < 0x40 || string(aci[:4]) != "ACI0" {
		return npdm, errors.New("main.npdm has no ACI0")
	}
	if len(acid) < 0x240 || string(acid[0x200:0x204]) != "ACID" {
		return npdm, errors.New("main.npdm has no ACID")
	}
	npdm.ProgramID = fmt.Sprintf("%016X", binary.LittleEndian.Uint64(aci[0x10:]))
	npdm.Production = binary.LittleEndian.Uint32(acid[0x20C:])&1 != 0
	npdm.ProgramMin = fmt.Sprintf("%016X", binary.LittleEndian.Uint64(acid[0x210:]))
	npdm.ProgramMax = fmt.Sprintf("%016X", binary.LittleEndian.Uint64(acid[0x218:]))
	if fah, err := region(aci, 0x20); err == nil && len(fah) >= 0xC {
		npdm.FileSystem = binary.LittleEndian.Uint64(fah[4:])
	}
	sac, err := region(aci, 0x28)
	if err != nil {
		return npdm, fmt.Errorf("services: %w", err)
	}
	npdm.Services, npdm.Hosts = parseServices(sac)
	kac, err := region(aci, 0x30)
	if err != nil {
		return npdm, fmt.Errorf("kernel: %w", err)
	}
	npdm.Kernel = parseKernel(kac)
	return npdm, nil
}

// region is the part of data an offset and a size at at tell (two words, offset first).
func region(data []byte, at int) ([]byte, error) {
	if len(data) < at+8 {
		return nil, errors.New("cut short")
	}
	offset := uint64(binary.LittleEndian.Uint32(data[at:]))
	size := uint64(binary.LittleEndian.Uint32(data[at+4:]))
	if offset > uint64(len(data)) || size > uint64(len(data))-offset {
		return nil, errors.New("beyond the file")
	}
	return data[offset : offset+size], nil
}

// parseServices reads the service access control: each entry a byte (its length less one in
// the low 3 bits, and the top bit for a service the program offers) and the name.
func parseServices(sac []byte) (services []string, hosts []string) {
	services, hosts = []string{}, []string{}
	for i := 0; i < len(sac); {
		control := sac[i]
		length := int(control&7) + 1
		if i+1+length > len(sac) {
			break
		}
		name := strings.TrimRight(string(sac[i+1:i+1+length]), "\x00")
		if control&0x80 != 0 {
			hosts = append(hosts, name)
		} else {
			services = append(services, name)
		}
		i += 1 + length
	}
	sort.Strings(services)
	sort.Strings(hosts)
	return services, hosts
}

// parseKernel reads the kernel capabilities: words told apart by how many of their low bits are
// set before the first clear one.
func parseKernel(kac []byte) Kernel {
	kernel := Kernel{}
	calls := map[int]bool{}
	for i := 0; i+4 <= len(kac); i += 4 {
		word := binary.LittleEndian.Uint32(kac[i:])
		if word == 0xFFFFFFFF {
			continue
		}
		ones := 0
		for word>>ones&1 == 1 {
			ones++
		}
		switch ones {
		case 3: // thread priorities and cores
			kernel.LowestPriority = int(word >> 4 & 0x3F)
			kernel.HighestPriority = int(word >> 10 & 0x3F)
			kernel.MinCore = int(word >> 16 & 0xFF)
			kernel.MaxCore = int(word >> 24 & 0xFF)
		case 4: // system calls: 24 of them by word, the index of the group in the top 3 bits
			mask, group := word>>5&0xFFFFFF, int(word>>29)
			for bit := 0; bit < 24; bit++ {
				if mask>>bit&1 == 1 {
					calls[group*24+bit] = true
				}
			}
		case 6: // memory map: two words, this one and the next
			kernel.MemoryMaps++
			i += 4
		case 7, 10: // a page of I/O memory, a region of memory
			kernel.MemoryMaps++
		case 11: // two interrupts, 0x3FF for none
			for _, interrupt := range []uint32{word >> 12 & 0x3FF, word >> 22 & 0x3FF} {
				if interrupt != 0x3FF {
					kernel.Interrupts = append(kernel.Interrupts, int(interrupt))
				}
			}
		case 13:
			kernel.ProgramType = map[uint32]string{0: "system", 1: "application", 2: "applet"}[word>>14&7]
		case 14:
			kernel.KernelVersion = fmt.Sprintf("%d.%d", word>>19&0x1FFF, word>>15&0xF)
		case 15:
			kernel.HandleTable = int(word >> 16 & 0x3FF)
		case 16:
			kernel.AllowDebug = word>>17&1 == 1
			kernel.ForceDebug = word>>18&1 == 1
		}
	}
	for call := range calls {
		kernel.SystemCalls = append(kernel.SystemCalls, call)
	}
	sort.Ints(kernel.SystemCalls)
	return kernel
}

// ReadPermissions reads main.npdm of the program a file holds: of the update when it holds one,
// as its program is the one that runs, else of the game. ok is false when there is none to read
// (a DLC, no keys, or a file that cannot be read).
func ReadPermissions(path string) (Npdm, bool) {
	titles, err := ReadTitles(path)
	if err != nil {
		return Npdm{}, false
	}
	sort.Slice(titles, func(i, j int) bool { return titles[i].TitleId > titles[j].TitleId })
	for _, title := range titles {
		if title.Type == "DLC" {
			continue
		}
		data, err := switchfs.ReadNpdm(path, title.TitleId)
		if err != nil {
			continue
		}
		if npdm, err := ParseNpdm(data); err == nil {
			return npdm, true
		}
	}
	return Npdm{}, false
}

// callRanges writes system calls as hexadecimal ranges: 0x01-0x29, 0x2C.
func callRanges(calls []int) string {
	parts := []string{}
	for i := 0; i < len(calls); {
		j := i
		for j+1 < len(calls) && calls[j+1] == calls[j]+1 {
			j++
		}
		if i == j {
			parts = append(parts, fmt.Sprintf("0x%02X", calls[i]))
		} else {
			parts = append(parts, fmt.Sprintf("0x%02X-0x%02X", calls[i], calls[j]))
		}
		i = j + 1
	}
	return strings.Join(parts, ", ")
}

// PermissionsLine is the short summary of the permissions Details writes in English.
func PermissionsLine(npdm Npdm) string {
	line := fmt.Sprintf("permissions: %d services, %d system calls, cores %d-%d", len(npdm.Services), len(npdm.Kernel.SystemCalls), npdm.Kernel.MinCore, npdm.Kernel.MaxCore)
	if len(npdm.Hosts) > 0 {
		line += fmt.Sprintf(", offers %d of its own", len(npdm.Hosts))
	}
	if npdm.Kernel.AllowDebug || npdm.Kernel.ForceDebug {
		line += ", can be debugged"
	}
	return line
}

// acidNote is what Details says of the signature of the ACID: it is not checked, as the key of
// Nintendo it needs is not one NXPress carries.
const acidNote = "signature of the ACID not checked (it needs a key of Nintendo that NXPress does not carry)"

// permissionsReport is the full list of the permissions, as the report writes it.
func permissionsReport(npdm Npdm) []string {
	label := func(name string, value string) string { return fmt.Sprintf("%-*s%s", reportLabelWidth-3, name, value) }
	lines := []string{}
	if npdm.Name != "" {
		lines = append(lines, label("Name:", npdm.Name))
	}
	lines = append(lines, label("Program:", fmt.Sprintf("%s, address space of %d bits", npdm.Bits, npdm.AddressSpace)))
	lines = append(lines, label("Main thread:", fmt.Sprintf("priority %d, core %d, stack %s", npdm.Priority, npdm.Core, size(int64(npdm.Stack)))))
	lines = append(lines, label("Program ID:", npdm.ProgramID))
	ids := npdm.ProgramMin
	if npdm.ProgramMax != npdm.ProgramMin {
		ids += " to " + npdm.ProgramMax
	}
	kind := "development"
	if npdm.Production {
		kind = "production"
	}
	lines = append(lines, label("ACID:", fmt.Sprintf("%s, program IDs %s", kind, ids)))
	lines = append(lines, label("", acidNote))
	lines = append(lines, label("File systems:", fmt.Sprintf("0x%016X", npdm.FileSystem)))
	lines = append(lines, label(fmt.Sprintf("Services (%d):", len(npdm.Services)), strings.Join(npdm.Services, ", ")))
	if len(npdm.Hosts) > 0 {
		lines = append(lines, label(fmt.Sprintf("Offers (%d):", len(npdm.Hosts)), strings.Join(npdm.Hosts, ", ")))
	}
	kernel := npdm.Kernel
	lines = append(lines, label("Threads:", fmt.Sprintf("priorities %d to %d, cores %d to %d", kernel.HighestPriority, kernel.LowestPriority, kernel.MinCore, kernel.MaxCore)))
	lines = append(lines, label(fmt.Sprintf("System calls (%d):", len(kernel.SystemCalls)), callRanges(kernel.SystemCalls)))
	extra := []string{}
	if kernel.ProgramType != "" {
		extra = append(extra, kernel.ProgramType)
	}
	if kernel.KernelVersion != "" {
		extra = append(extra, "kernel "+kernel.KernelVersion)
	}
	if kernel.HandleTable > 0 {
		extra = append(extra, fmt.Sprintf("%d handles", kernel.HandleTable))
	}
	if kernel.MemoryMaps > 0 {
		extra = append(extra, fmt.Sprintf("%d memory maps", kernel.MemoryMaps))
	}
	if len(kernel.Interrupts) > 0 {
		extra = append(extra, fmt.Sprintf("%d interrupts", len(kernel.Interrupts)))
	}
	switch {
	case kernel.ForceDebug:
		extra = append(extra, "always debugged")
	case kernel.AllowDebug:
		extra = append(extra, "can be debugged")
	}
	if len(extra) > 0 {
		lines = append(lines, label("Kernel:", strings.Join(extra, ", ")))
	}
	return lines
}
