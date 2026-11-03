package system

import (
	"os/exec"
	"runtime"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	esContinuous     = 0x80000000
	esSystemRequired = 0x00000001
)

// preventSleep keeps the computer from going to sleep until the function it gives is called.
func preventSleep() func() {
	state := kernel32.NewProc("SetThreadExecutionState")
	done := make(chan struct{})
	released := make(chan struct{})
	// the state belongs to a thread: one goroutine, locked to its thread, holds it
	go func() {
		runtime.LockOSThread()
		state.Call(esContinuous | esSystemRequired)
		<-done
		state.Call(esContinuous)
		close(released)
	}()
	return func() {
		close(done)
		<-released
	}
}

// ShutDown turns the computer off; forced, the programs left open are closed without asking to
// save, which else can keep it on.
func ShutDown(force bool) error {
	arguments := []string{"/s", "/t", "0"}
	if force {
		arguments = append(arguments, "/f")
	}
	command := exec.Command("shutdown.exe", arguments...)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}

// CanHibernate tells whether the computer can hibernate: Windows lets it be turned off
// (powercfg /hibernate off), and then it is not offered.
func CanHibernate() bool {
	allowed := windows.NewLazySystemDLL("powrprof.dll").NewProc("IsPwrHibernateAllowed")
	if allowed.Find() != nil {
		return false
	}
	yes, _, _ := allowed.Call()
	return yes != 0
}

// Hibernate keeps what is open on the disk and turns the computer off; it comes back as it was
// left, with no program closed.
func Hibernate() error {
	command := exec.Command("shutdown.exe", "/h")
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	return command.Run()
}
