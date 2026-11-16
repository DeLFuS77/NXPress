//go:build windows

package main

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"syscall"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

// The browser of the window never opens a window of its own (#120). Files dropped on the window
// are taken by the page, which keeps the browser from them; but when the page does not get the
// drop (seen with several folders dragged from another screen), the browser opens each one in a
// window of its own. Here the browser is told, under the page, that every new window is handled:
// none opens. NXPress opens its links in the browser of the system, never in a window of its own.
//
// Wails does not hand its browser out, so it is read from the window Wails keeps in the context:
// the field chromium of its frontend, and the window that runs it, whose thread is the only one
// the browser takes calls from. If any of it is not where it was (another Wails), nothing is done
// and the page alone keeps the drops, as before.

var blockOnce sync.Once

func blockNewWindows(ctx context.Context) {
	blockOnce.Do(func() {
		if err := addNewWindowBlock(ctx); err != nil {
			fmt.Println("nxpress: new windows of the browser not blocked:", err)
		}
	})
}

// field returns the field name of the struct v points to, usable although it is not exported
func field(v reflect.Value, name string) (reflect.Value, error) {
	if v.Kind() == reflect.Interface {
		v = v.Elem()
	}
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("no struct for %s", name)
	}
	f := v.Elem().FieldByName(name)
	if !f.IsValid() {
		return reflect.Value{}, fmt.Errorf("no field %s", name)
	}
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem(), nil
}

func addNewWindowBlock(ctx context.Context) error {
	frontend := ctx.Value("frontend")
	if frontend == nil {
		return fmt.Errorf("no frontend")
	}
	fv := reflect.ValueOf(frontend)
	chromiumField, err := field(fv, "chromium")
	if err != nil {
		return err
	}
	chromium, ok := chromiumField.Interface().(*edge.Chromium)
	if !ok || chromium == nil {
		return fmt.Errorf("chromium is %s", chromiumField.Type())
	}
	window, err := field(fv, "mainWindow")
	if err != nil {
		return err
	}
	invoke := window.MethodByName("Invoke")
	if !invoke.IsValid() || invoke.Type().NumIn() != 1 || invoke.Type().In(0) != reflect.TypeOf(func() {}) {
		return fmt.Errorf("no Invoke on the window")
	}
	done := make(chan error, 1)
	invoke.Call([]reflect.Value{reflect.ValueOf(func() { done <- addOnThread(chromium) })})
	return <-done
}

// the place of add_NewWindowRequested in the table of ICoreWebView2, read from the binding of
// go-webview2 (44 in the interface of WebView2)
func newWindowSlot() (int, error) {
	table := reflect.TypeOf(edge.ICoreWebView2{}).Field(0).Type
	if table.Kind() != reflect.Pointer || table.Elem().Kind() != reflect.Struct {
		return 0, fmt.Errorf("no table of ICoreWebView2")
	}
	f, ok := table.Elem().FieldByName("AddNewWindowRequested")
	if !ok {
		return 0, fmt.Errorf("no AddNewWindowRequested")
	}
	return int(f.Offset / unsafe.Sizeof(uintptr(0))), nil
}

// comMethod is the method at slot of the COM object at object
func comMethod(object unsafe.Pointer, slot int) uintptr {
	table := *(*unsafe.Pointer)(object)
	return *(*uintptr)(unsafe.Add(table, uintptr(slot)*unsafe.Sizeof(uintptr(0))))
}

// addOnThread runs on the thread of the window
func addOnThread(chromium *edge.Chromium) error {
	slot, err := newWindowSlot()
	if err != nil {
		return err
	}
	controller := chromium.GetController()
	if controller == nil {
		return fmt.Errorf("no controller")
	}
	core, err := controller.GetCoreWebView2()
	if err != nil || core == nil {
		return fmt.Errorf("no webview: %v", err)
	}
	var token int64
	hr, _, _ := syscall.SyscallN(comMethod(unsafe.Pointer(core), slot),
		uintptr(unsafe.Pointer(core)), uintptr(unsafe.Pointer(newWindowHandler)), uintptr(unsafe.Pointer(&token)))
	if hr != 0 {
		return syscall.Errno(hr)
	}
	return nil
}

// newWindowHandler is an ICoreWebView2NewWindowRequestedEventHandler that answers every new
// window as handled, and gives none: nothing opens. It lives as long as NXPress, as the browser
// keeps only its address.
type comHandler struct{ table *[4]uintptr }

var newWindowHandler = &comHandler{table: &[4]uintptr{
	windows.NewCallback(func(this, iid unsafe.Pointer, object *unsafe.Pointer) uintptr {
		if object != nil {
			*object = this
		}
		return 0
	}),
	windows.NewCallback(func(this unsafe.Pointer) uintptr { return 1 }),
	windows.NewCallback(func(this unsafe.Pointer) uintptr { return 1 }),
	windows.NewCallback(func(this, sender, args unsafe.Pointer) uintptr {
		// put_Handled(TRUE), the 7th method of ICoreWebView2NewWindowRequestedEventArgs
		if args != nil {
			syscall.SyscallN(comMethod(args, 6), uintptr(args), 1)
		}
		return 0
	}),
}}
