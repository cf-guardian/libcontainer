// +build linux

package namespaces

import (
	"encoding/json"
	"os"
	"strconv"

	"github.com/docker/libcontainer"
	"github.com/dotcloud/docker/pkg/system"
)

// ExecIn uses an existing pid and joins the pid's namespaces with the new command.
func ExecIn(container *libcontainer.Config, state *libcontainer.State, args []string) error {
	// TODO(vmarmol): If this gets too long, send it over a pipe to the child.
	// Marshall the container into JSON since it won't be available in the namespace.
	containerJson, err := json.Marshal(container)
	if err != nil {
		return err
	}

	// Enter the namespace and then finish setup
	finalArgs := []string{os.Args[0], "nsenter", "--nspid", strconv.Itoa(state.InitPid), "--containerjson", string(containerJson), "--"}
	finalArgs = append(finalArgs, args...)
	if err := system.Execv(finalArgs[0], finalArgs[0:], os.Environ()); err != nil {
		return err
	}
	panic("unreachable")
}

