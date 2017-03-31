package volume

import (
	"fmt"
	"os"

	volumetypes "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/opts"
	project "github.com/docker/docker/proj"
	runconfigopts "github.com/docker/docker/runconfig/opts"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"golang.org/x/net/context"
)

type createOptions struct {
	name       string
	driver     string
	driverOpts opts.MapOpts
	labels     opts.ListOpts
}

func newCreateCommand(dockerCli command.Cli) *cobra.Command {
	opts := createOptions{
		driverOpts: *opts.NewMapOpts(nil, nil),
		labels:     opts.NewListOpts(opts.ValidateEnv),
	}

	cmd := &cobra.Command{
		Use:   "create [OPTIONS] [VOLUME]",
		Short: "Create a volume",
		Args:  cli.RequiresMaxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				if opts.name != "" {
					return errors.Errorf("Conflicting options: either specify --name or provide positional arg, not both\n")
				}
				opts.name = args[0]
			}
			return runCreate(dockerCli, opts)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&opts.driver, "driver", "d", "local", "Specify volume driver name")
	flags.StringVar(&opts.name, "name", "", "Specify volume name")
	flags.Lookup("name").Hidden = true
	flags.VarP(&opts.driverOpts, "opt", "o", "Set driver specific options")
	flags.Var(&opts.labels, "label", "Set metadata for a volume")

	return cmd
}

func runCreate(dockerCli command.Cli, opts createOptions) error {
	client := dockerCli.Client()

	volReq := volumetypes.VolumesCreateBody{
		Driver:     opts.driver,
		DriverOpts: opts.driverOpts.GetAll(),
		Name:       opts.name,
		Labels:     runconfigopts.ConvertKVStringsToMap(opts.labels.GetAll()),
	}

	// add label to identify project if needed
	// see if we're in the context of a Docker project or not
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	proj, err := project.Get(wd)
	if err != nil {
		return err
	}
	if proj != nil {
		volReq.Labels["docker.project.id:"+proj.Config.ID] = ""
		volReq.Labels["docker.project.name:"+proj.Config.Name] = ""
	}

	vol, err := client.VolumeCreate(context.Background(), volReq)
	if err != nil {
		return err
	}

	fmt.Fprintf(dockerCli.Out(), "%s\n", vol.Name)
	return nil
}
