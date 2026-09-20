package app

import (
	"github.com/spf13/cobra"

	"github.com/standards-lab/org/experiments/blobfs/output"
)

// commands is the list of mounts: the subtrees the root serves, each built
// by the layer file that owns it. The admin commands come from admin.go;
// the domain commands join the list when domain.go arrives. This file
// composes and does nothing else. Every command family renders through out.
func commands(adm *Admin, out *output.Output) []*cobra.Command {
	var all []*cobra.Command
	all = append(all, mountAdmin(adm, out)...)
	return all
}
