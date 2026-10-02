package platform

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procShellExecuteExW = windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW")

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize        uint32
	fMask         uint32
	hwnd          windows.Handle
	verb          *uint16
	file          *uint16
	parameters    *uint16
	directory     *uint16
	show          int32
	instApp       windows.Handle
	idList        uintptr
	class         *uint16
	keyClass      windows.Handle
	hotKey        uint32
	iconOrMonitor windows.Handle
	process       windows.Handle
}

const seeMaskNoCloseProcess = 0x40

// ErrCancelled means the person said no to Windows' administrator prompt.
var ErrCancelled = errors.New("the administrator prompt was declined")

// RunElevated runs exe with args (one command line, quoted as the
// program expects) as administrator, after Windows asks the person, and
// waits for it to exit. It returns the exit code.
func RunElevated(ctx context.Context, exe, args, dir string) (uint32, error) {
	verb, _ := windows.UTF16PtrFromString("runas")
	file, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess, verb: verb, file: file, show: windows.SW_SHOWNORMAL}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if args != "" {
		info.parameters, _ = windows.UTF16PtrFromString(args)
	}
	if dir != "" {
		info.directory, _ = windows.UTF16PtrFromString(dir)
	}
	if ok, _, err := procShellExecuteExW.Call(uintptr(unsafe.Pointer(&info))); ok == 0 {
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return 0, ErrCancelled
		}
		return 0, err
	}
	if info.process == 0 {
		return 0, errors.New("Windows didn't say which process started")
	}
	defer windows.CloseHandle(info.process)
	for {
		ev, err := windows.WaitForSingleObject(info.process, 500)
		if err != nil {
			return 0, err
		}
		if ev == windows.WAIT_OBJECT_0 {
			break
		}
		if ctx.Err() != nil {
			return 0, ctx.Err() // it goes on; an elevated process can't be stopped from here
		}
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.process, &code); err != nil {
		return 0, err
	}
	return code, nil
}
