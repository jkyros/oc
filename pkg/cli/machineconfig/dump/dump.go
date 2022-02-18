package dump

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	mcfgv1 "github.com/openshift/machine-config-operator/pkg/apis/machineconfiguration.openshift.io/v1"
	ctrlcommon "github.com/openshift/machine-config-operator/pkg/controller/common"
	mcfgclientset "github.com/openshift/machine-config-operator/pkg/generated/clientset/versioned"
	"github.com/vincent-petithory/dataurl"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	kcmdget "k8s.io/kubectl/pkg/cmd/get"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/util/templates"
)

var (
	MachineConfigsLong = templates.LongDesc(`
   
		MachineConfigs are things that do stuff. 

	`)
)

func NewCmdShowFiles(f kcmdutil.Factory, streams genericclioptions.IOStreams) *cobra.Command {
	o := MachineConfigOptions{}
	//TOOD: turn stream options into my options I guess/

	// Parent command to which all subcommands are added.
	cmd := &cobra.Command{
		Use:               "dump",
		Short:             "Dump machineconfig files",
		Long:              MachineConfigsLong,
		Aliases:           []string{"dump"},
		ValidArgsFunction: o.MachineConfigCompletionFunc(f),
		//	Run:     kcmdutil.DefaultSubCommandRun(streams.ErrOut),
		Run: func(c *cobra.Command, args []string) {
			kcmdutil.CheckErr(o.Complete(f, args))
			kcmdutil.CheckErr(o.Validate())
			kcmdutil.CheckErr(o.Run())
		},
	}

	cmd.Flags().StringVar(&o.DumpToDir, "to-dir", "", "write specified files out as files to directory")
	//cmds.AddCommand(NewCmdOneMachineConfig(f, streams))
	//cmds.AddCommand(NewCmdTwoMachineConfig(f, streams))

	return cmd
}

// MachineConfigOptions Structure holding state for processing MachineConfig linking and
// unlinking.
type MachineConfigOptions struct {
	MachineConfig string
	Files         []string

	Namespace string
	DumpToDir string

	KubeClient          corev1client.CoreV1Interface
	MachineConfigClient *mcfgclientset.Clientset
}

// Complete Parses the command line arguments and populates MachineConfigOptions
func (o *MachineConfigOptions) Complete(f kcmdutil.Factory, args []string) error {
	if len(args) < 1 {
		return errors.New("must specify machineconfig name")
	}

	o.MachineConfig = args[0]
	o.Files = args[1:]

	// Assuming mushing all their files together isn't what they want
	if len(o.Files) > 1 && o.DumpToDir == "" {
		return errors.New("if dumping multiple files specify --to-dir")
	}

	var err error
	clientConfig, err := f.ToRESTConfig()
	if err != nil {
		return err
	}

	o.KubeClient, err = corev1client.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	o.MachineConfigClient, err = mcfgclientset.NewForConfig(clientConfig)
	if err != nil {
		return err
	}

	o.Namespace, _, err = f.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	return nil
}

// Validate Ensures that all arguments have appropriate values
func (o MachineConfigOptions) Validate() error {
	if o.MachineConfig == "" {
		return errors.New("You must specify a machineconfig")
	}

	if o.KubeClient == nil {
		return errors.New("KubeClient must be present")
	}

	if o.MachineConfigClient == nil {
		return errors.New("MachineConfigClient must be present")
	}

	return nil
}

func (o *MachineConfigOptions) Run() error {
	mcfg, err := o.getMachineConfig(o.MachineConfig)
	if err != nil {
		return err
	}
	return o.GetFiles(mcfg)

}

// MachineConfigCompletionFunc Returns a completion function that completes as a first
// argument pod names that match the toComplete prefix, and as a second argument the containers
// within the specified pod.
func (o *MachineConfigOptions) MachineConfigCompletionFunc(f kcmdutil.Factory) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		var comps []string

		// If we're on the first arg, we're autocompleting machineconfig
		if len(args) == 0 {
			// TODO(jkyros): sort these in render order? otherwise they'll need to check their other screen.
			// or I suppose add sugar for 'for the current config in pool x show me this file'
			comps = kcmdget.CompGetResource(f, cmd, "MachineConfig", toComplete)

			// If we're on the second arg, it's going to be a file
			// TODO(jkyros): what about units ? users? otherstuff?
		} else if len(args) == 1 {
			comps = o.GetMachineConfigFileNames(f, args[0])
		}
		return comps, cobra.ShellCompDirectiveNoFileComp
	}
}

// getMachineConfig Retrieve the service account object specified by the command
func (o MachineConfigOptions) getMachineConfig(mcName string) (*mcfgv1.MachineConfig, error) {

	return o.MachineConfigClient.MachineconfigurationV1().MachineConfigs().Get(context.TODO(), mcName, metav1.GetOptions{})
}

func (o *MachineConfigOptions) GetFiles(mc *mcfgv1.MachineConfig) error {

	// Convert the raw config to ignv3
	parsedConfig, err := ctrlcommon.ParseAndConvertConfig(mc.Spec.Config.Raw)
	if err != nil {
		return err
	}

	if len(o.Files) == 1 {

		for _, f := range parsedConfig.Storage.Files {
			for _, desiredFile := range o.Files {
				if desiredFile == f.Path {
					// Convert whatever we have to the actual bytes so we can inspect them
					if f.Contents.Source != nil {
						contents, err := dataurl.DecodeString(*f.Contents.Source)
						if err != nil {
							return err
						}
						fmt.Printf("%s\n", contents.Data)
					}
				}
			}
		}

	} else {

		// TODO(jkyros): no user will want this, write the files to a directory or something
		if o.DumpToDir != "" {

			for _, f := range parsedConfig.Storage.Files {

				// Convert whatever we have to the actual bytes so we can inspect them
				if f.Contents.Source != nil {
					contents, err := dataurl.DecodeString(*f.Contents.Source)
					if err != nil {
						return err
					}

					// Mangle the files's actual path into our directory
					outpath := filepath.Join(o.DumpToDir, f.Path)

					// Chop off the file name so we make just the directory
					outdir := filepath.Dir(outpath)

					// Make the directory if it doesn't exist since we're trying to mirror the machineconfig structure
					if _, err := os.Stat(outdir); os.IsNotExist(err) {
						err := os.MkdirAll(outdir, 0700) // Create your file
						if err != nil {
							return err
						}
					}

					// Write the file into our directory
					err = ioutil.WriteFile(outpath, contents.Data, 0775)
					if err != nil {
						return err
					}
					fmt.Printf("Wrote: %s\n", outpath)
				}

			}
		}
	}

	return nil
}

// This is ridiculous but it does autocomplete the file list :)
func (o *MachineConfigOptions) GetMachineConfigFileNames(f kcmdutil.Factory, mcName string) []string {
	var fileList []string

	// NOTE: so at the time this gets called, nothing is set up other than our factory because the
	// command isn't fully executed during the "autocomplete stage", so our setup functions haven't been called,
	// which means we have to build our client down here like we do up there
	// and while I'd freeload on kubectl cmd util, it doesn't know anything about what's inside
	// the machineconfig files so we have to grab those ourselves

	// We need the client config to talk to the cluster
	clientConfig, err := f.ToRESTConfig()
	if err != nil {
		return nil
	}

	// We need the machineconfig clientset to get the config
	mcc, err := mcfgclientset.NewForConfig(clientConfig)
	if err != nil {
		return nil
	}

	// Retrieve the machineconfig we've specified so we can get the files out of it
	mc, err := mcc.MachineconfigurationV1().MachineConfigs().Get(context.TODO(), mcName, metav1.GetOptions{})
	if err != nil {
		return nil
	}

	// I feel like there should be a better function I can call directly out of ignition but I don't know about it if there is
	parsedConfig, err := ctrlcommon.ParseAndConvertConfig(mc.Spec.Config.Raw)
	if err != nil {
		return nil
	}

	// Add all the files to the autocomplete list
	for _, file := range parsedConfig.Storage.Files {
		fileList = append(fileList, file.Path)
	}

	return fileList
}
