// +build linux

package namespaces

/*
#define _GNU_SOURCE
#include <sched.h>
// Use raw setns syscall for versions of glibc that don't include it (namely glibc-2.12)
#if __GLIBC__ == 2 && __GLIBC_MINOR__ < 14
#include "syscall.h"
#ifdef SYS_setns
int setns(int fd, int nstype) {
  return syscall(SYS_setns, fd, nstype);
}
#endif
#endif
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"runtime"
	"syscall"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/label"
	"github.com/dotcloud/docker/pkg/system"
)

// Enter the namespace for the given PID and execute the command in the given arguments.
func NsEnter(container *libcontainer.Config, nspid int, args []string) error {
	if err := enterNamespaces(nspid); err != nil {
		return err
	}
	// clear the current processes env and replace it with the environment
	// defined on the container
	if err := LoadContainerEnvironment(container); err != nil {
		return err
	}
	if err := FinalizeNamespace(container); err != nil {
		return err
	}

	if container.ProcessLabel != "" {
		if err := label.SetProcessLabel(container.ProcessLabel); err != nil {
			return err
		}
	}

	if err := system.Execv(args[0], args[0:], container.Env); err != nil {
		return err
	}
	panic("unreachable")
}

// On return, the current goroutine is locked to the current thread.
// This is necessary to preserve the effects of setns.
func enterNamespaces(nspid int) error {
	runtime.LockOSThread()

	nsDir, nss, err := findNamespaces(nspid)
	if err != nil {
		return err
	}
	for _, ns := range nss {
		if ns != "user" {
			if err = enterNamespace(nspid, nsDir, ns); err != nil {
				return err
			}
		}
	}

	// Need to fork to enter the PID namespace.
	// But fork may fail with an assertion ([1]), so use clone instead.
	//
	// [1] http://sourceware.org/ml/glibc-bugs/2013-04/msg00139.html
	childPid, err := system.Clone(0) // TODO: how to ensure SIGCHILD is sent to the parent?
	if childPid == -1 {
		log.Println("Clone failed: ", err)
		return err
	} else if childPid == 0 {
		log.Println("Child running")
		// Allow the child process to continue.
		return nil
	} else {
		log.Printf("Parent running, child pid: %d\n", childPid)
		// Exit with the child's exit code or kill the parent with the child's death signal.
		child, err := os.FindProcess(childPid)
		if err != nil {
			log.Println("FindProcess failed: ", err)
			return err
		}
	repeat:
		childState, err := child.Wait()
		if err != nil {
			//log.Println("Wait failed: ", err)
			goto repeat //TODO: remove nasty hack
			return err
		}
		childWaitStatus := childState.Sys().(syscall.WaitStatus)
		if childState.Exited() {
			os.Exit(childWaitStatus.ExitStatus())
		} else if childWaitStatus.Signaled() {
			syscall.Kill(os.Getpid(), childWaitStatus.Signal())
		} else {
			os.Exit(1)
		}
		panic("unreachable")
	}
}

// Determine the namespaces to enter based on the given PID.
// The user namespace is not currently entered since the support for that namespace is incomplete.
// The mount namespace must be entered after the PID namespace to ensure a valid proc file system.
func findNamespaces(nspid int) (nsDir string, nss []string, err error) {
	nsDir = fmt.Sprintf("/proc/%d/ns/", nspid)
	fileInfos, err := ioutil.ReadDir(nsDir)
	if err != nil {
		return
	}
	for _, fi := range fileInfos {
		log.Println("fileInfo name: ", fi.Name())
	}
	nss = make([]string, len(fileInfos))
	user, mnt := -1, -1
	for i, fi := range fileInfos {
		ns := fi.Name()
		if ns == "user" {
			user = i
		} else if ns == "mnt" {
			mnt = i
		}
		nss[i] = ns
	}
	if user >= 0 {
		nss = append(nss[0:user], nss[user+1:]...)
		if mnt >= 0 && mnt > user {
			mnt--
		}
	}
	if mnt >= 0 {
		nss = append(append(append(nss[0:mnt], nss[len(nss)-1]), nss[mnt+1:]...), nss[mnt])
	}
	log.Println("nss: ", nss)
	return
}

func enterNamespace(nspid int, nsDir string, ns string) error {
	fn := path.Join(nsDir, ns)
	f, err := os.Open(fn)
	if err != nil {
		log.Printf("Failed to open %q", fn)
		return err
	}
	defer f.Close()

	log.Printf("About to enter namespace %s using fd %v\n", fn, f.Fd())
	err = setns(int(f.Fd()))
	return err
}

// Associate the calling thread with the namespace specified by the given file descriptor.
// The file descriptor must refer to a namespace entry in /proc/[pid]/ns/, for some [pid].
// The namespace entry may be of any type.
//
// Note: Docker's system.Setns is equivalent, but for Linux on AMD64 only.
func setns(fd int) error {
	// Join any type of namespace referred to by fd.
	ret, err := C.setns(C.int(fd), 0)
	if ret == 0 {
		return nil
	} else {
		errNo := err.(syscall.Errno)
		log.Printf("C.setns(%v) failed: %s %d\n", C.int(fd), err, int(errNo))
		return err
	}
}
