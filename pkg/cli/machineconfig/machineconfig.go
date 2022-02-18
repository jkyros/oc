package mco

import (
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	ktemplates "k8s.io/kubectl/pkg/util/templates"

	mc "github.com/openshift/oc/pkg/cli/machineconfig/dump"
)

var (
	imageLong = ktemplates.LongDesc(`
		Manage machineconfig on OpenShift

		These commands help you deal with machineconfig on OpenShift.`)
)

// NewCmdMco exposes commands for modifying images.
func NewCmdMco(f kcmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	mco := &cobra.Command{
		Use:   "machineconfig COMMAND",
		Short: "Useful commands for managing machineconfig",
		Long:  imageLong,
		Run:   kcmdutil.DefaultSubCommandRun(streams.ErrOut),
	}

	groups := ktemplates.CommandGroups{
		{
			Message: "View or copy images:",
			Commands: []*cobra.Command{
				mc.NewCmdShowFiles(f, streams),
			},
		},
		{
			Message:  "Advanced commands:",
			Commands: []*cobra.Command{},
		},
	}
	groups.Add(mco)
	//cmdutil.ActsAsRootCommand(mco, []string{"options"}, groups...)
	return mco
}
