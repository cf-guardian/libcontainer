// +build linux

package console

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/docker/libcontainer/label"
)

// for error injection testing
var (
	osChmod    = os.Chmod
	osChown    = os.Chown
	osCreate   = os.Create
	osOpenFile = os.OpenFile

	syscallOpen    = syscall.Open
	syscallDup2    = syscall.Dup2
	syscallSyscall = syscall.Syscall
	syscallMount   = syscall.Mount
)

// Setup initializes the proper /dev/console inside the rootfs path
func Setup(rootfs, consolePath, mountLabel string) error {
	oldMask := syscall.Umask(0000)
	defer syscall.Umask(oldMask)

	// TODO: test
	if err := osChmod(consolePath, 0600); err != nil {
		return err
	}

	// TODO: test
	if err := osChown(consolePath, 0, 0); err != nil {
		return err
	}

	if err := label.SetFileLabel(consolePath, mountLabel); err != nil {
		return fmt.Errorf("set file label %s %s", consolePath, err)
	}

	dest := filepath.Join(rootfs, "dev/console")

	// TODO: extract private function and test
	f, err := osCreate(dest)
	if err != nil && !os.IsExist(err) {
		return fmt.Errorf("create %s %s", dest, err)
	}

	if f != nil {
		f.Close()
	}
	// end TODO

	// TODO: test
	if err := syscallMount(consolePath, dest, "bind", syscall.MS_BIND, ""); err != nil {
		return fmt.Errorf("bind %s to %s %s", consolePath, dest, err)
	}

	return nil
}

func OpenAndDup(consolePath string) error {
	slave, err := OpenTerminal(consolePath, syscall.O_RDWR)
	if err != nil {
		return fmt.Errorf("open terminal %s", err)
	}

	// TODO: test
	if err := syscallDup2(int(slave.Fd()), 0); err != nil {
		return err
	}

	// TODO: test
	if err := syscallDup2(int(slave.Fd()), 1); err != nil {
		return err
	}

	// TODO: test
	return syscallDup2(int(slave.Fd()), 2)
}

// Unlockpt unlocks the slave pseudoterminal device corresponding to the master pseudoterminal referred to by f.
// Unlockpt should be called before opening the slave side of a pseudoterminal.
func Unlockpt(f *os.File) error {
	var u int

	return Ioctl(f.Fd(), syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&u)))
}

// Ptsname retrieves the name of the first available pts for the given master.
func Ptsname(f *os.File) (string, error) {
	var n int

	if err := Ioctl(f.Fd(), syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n))); err != nil {
		return "", err
	}

	return fmt.Sprintf("/dev/pts/%d", n), nil
}

// CreateMasterAndConsole will open /dev/ptmx on the host and retreive the
// pts name for use as the pty slave inside the container
func CreateMasterAndConsole() (*os.File, string, error) {
	// TODO: test
	master, err := osOpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, "", err
	}

	console, err := Ptsname(master)
	if err != nil {
		return nil, "", err
	}

	if err := Unlockpt(master); err != nil {
		return nil, "", err
	}

	return master, console, nil
}

// OpenPtmx opens /dev/ptmx, i.e. the PTY master.
func OpenPtmx() (*os.File, error) {
	// TODO: test
	// O_NOCTTY and O_CLOEXEC are not present in os package so we use the syscall's one for all.
	return osOpenFile("/dev/ptmx", syscall.O_RDONLY|syscall.O_NOCTTY|syscall.O_CLOEXEC, 0)
}

// OpenTerminal is a clone of os.OpenFile without the O_CLOEXEC
// used to open the pty slave inside the container namespace
func OpenTerminal(name string, flag int) (*os.File, error) {
	// TODO: test
	r, e := syscallOpen(name, flag, 0)
	if e != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: e}
	}
	return os.NewFile(uintptr(r), name), nil
}

func Ioctl(fd uintptr, flag, data uintptr) error {
	// TODO: test
	if _, _, err := syscallSyscall(syscall.SYS_IOCTL, fd, flag, data); err != 0 {
		return err
	}

	return nil
}
