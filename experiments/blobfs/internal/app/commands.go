package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// commands is the list of mounts: the subtrees the root serves, each built
// by the layer file that owns it. The domain commands come from domain.go
// and the admin commands from admin.go. This file composes and does nothing
// else. Every command family renders through out.
func commands(dom *Domain, adm *Admin, out *output.Output) []*cobra.Command {
	var all []*cobra.Command
	all = append(all, mountDomain(dom, out)...)
	all = append(all, mountAdmin(adm, out)...)
	return all
}
