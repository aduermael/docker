package project

import (
	"errors"
	"fmt"
	"io/ioutil"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/commands"
	cliconfig "github.com/docker/docker/cli/config"
	"github.com/docker/docker/cli/debug"
	cliflags "github.com/docker/docker/cli/flags"
	"github.com/docker/docker/dockerversion"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/templates"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// REQUIRED BY dockerContainerList

type psOptions struct {
	quiet   bool
	size    bool
	all     bool
	noTrunc bool
	nLatest bool
	last    int
	format  string
	filter  opts.FilterOpt
}

type listOptionsProcessor map[string]bool

// Size sets the size of the map when called by a template execution.
func (o listOptionsProcessor) Size() bool {
	o["size"] = true
	return true
}

// Label is needed here as it allows the correct pre-processing
// because Label() is a method with arguments
func (o listOptionsProcessor) Label(name string) string {
	return ""
}

func buildContainerListOptions(opts *psOptions) (*types.ContainerListOptions, error) {
	options := &types.ContainerListOptions{
		All:     opts.all,
		Limit:   opts.last,
		Size:    opts.size,
		Filters: opts.filter.Value(),
	}

	if opts.nLatest && opts.last == -1 {
		options.Limit = 1
	}

	tmpl, err := templates.Parse(opts.format)

	if err != nil {
		return nil, err
	}

	optionsProcessor := listOptionsProcessor{}
	// This shouldn't error out but swallowing the error makes it harder
	// to track down if preProcessor issues come up. Ref #24696
	if err := tmpl.Execute(ioutil.Discard, optionsProcessor); err != nil {
		return nil, err
	}
	// At the moment all we need is to capture .Size for preprocessor
	options.Size = opts.size || optionsProcessor["size"]

	return options, nil
}

// REQUIRED BY dockerImageList

type imagesOptions struct {
	matchName string

	quiet       bool
	all         bool
	noTrunc     bool
	showDigests bool
	format      string
	filter      opts.FilterOpt
}

// REQUIRED BY dockerCmd
// copied from /cmd/docker/docker.go
func newDockerCommand(dockerCli *command.DockerCli) *cobra.Command {
	opts := cliflags.NewClientOptions()
	var flags *pflag.FlagSet

	cmd := &cobra.Command{
		Use:              "docker [OPTIONS] COMMAND [ARG...]",
		Short:            "A self-sufficient runtime for containers",
		SilenceUsage:     true,
		SilenceErrors:    true,
		TraverseChildren: true,
		Args:             noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.Version {
				showVersion()
				return nil
			}
			return dockerCli.ShowHelp(cmd, args)
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// daemon command is special, we redirect directly to another binary
			if cmd.Name() == "daemon" {
				return nil
			}
			// flags must be the top-level command flags, not cmd.Flags()
			opts.Common.SetDefaultOptions(flags)
			dockerPreRun(opts)
			if err := dockerCli.Initialize(opts); err != nil {
				return err
			}
			return isSupported(cmd, dockerCli.Client().ClientVersion(), dockerCli.HasExperimental())
		},
	}
	cli.SetupRootCommand(cmd)

	cmd.SetHelpFunc(func(ccmd *cobra.Command, args []string) {
		if dockerCli.Client() == nil { // when using --help, PersistenPreRun is not called, so initialization is needed.
			// flags must be the top-level command flags, not cmd.Flags()
			opts.Common.SetDefaultOptions(flags)
			dockerPreRun(opts)
			dockerCli.Initialize(opts)
		}

		if err := isSupported(ccmd, dockerCli.Client().ClientVersion(), dockerCli.HasExperimental()); err != nil {
			ccmd.Println(err)
			return
		}

		hideUnsupportedFeatures(ccmd, dockerCli.Client().ClientVersion(), dockerCli.HasExperimental())

		if err := ccmd.Help(); err != nil {
			ccmd.Println(err)
		}
	})

	flags = cmd.Flags()
	flags.BoolVarP(&opts.Version, "version", "v", false, "Print version information and quit")
	flags.StringVar(&opts.ConfigDir, "config", cliconfig.Dir(), "Location of client config files")
	opts.Common.InstallFlags(flags)

	cmd.SetOutput(dockerCli.Out())
	// cmd.AddCommand(newDaemonCommand())
	commands.AddCommands(cmd, dockerCli)

	return cmd
}

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf(
		"docker: '%s' is not a docker command.\nSee 'docker --help'", args[0])
}

func showVersion() {
	fmt.Printf("Docker version %s, build %s\n", dockerversion.Version, dockerversion.GitCommit)
}

func dockerPreRun(opts *cliflags.ClientOptions) {
	cliflags.SetLogLevel(opts.Common.LogLevel)

	if opts.ConfigDir != "" {
		cliconfig.SetDir(opts.ConfigDir)
	}

	if opts.Common.Debug {
		debug.Enable()
	}
}

func hideUnsupportedFeatures(cmd *cobra.Command, clientVersion string, hasExperimental bool) {
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// hide experimental flags
		if !hasExperimental {
			if _, ok := f.Annotations["experimental"]; ok {
				f.Hidden = true
			}
		}

		// hide flags not supported by the server
		if !isFlagSupported(f, clientVersion) {
			f.Hidden = true
		}

	})

	for _, subcmd := range cmd.Commands() {
		// hide experimental subcommands
		if !hasExperimental {
			if _, ok := subcmd.Tags["experimental"]; ok {
				subcmd.Hidden = true
			}
		}

		// hide subcommands not supported by the server
		if subcmdVersion, ok := subcmd.Tags["version"]; ok && versions.LessThan(clientVersion, subcmdVersion) {
			subcmd.Hidden = true
		}
	}
}

func isSupported(cmd *cobra.Command, clientVersion string, hasExperimental bool) error {
	if !hasExperimental {
		if _, ok := cmd.Tags["experimental"]; ok {
			return errors.New("only supported on a Docker daemon with experimental features enabled")
		}
	}

	if cmdVersion, ok := cmd.Tags["version"]; ok && versions.LessThan(clientVersion, cmdVersion) {
		return fmt.Errorf("requires API version %s, but the Docker daemon API version is %s", cmdVersion, clientVersion)
	}

	errs := []string{}

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			if !isFlagSupported(f, clientVersion) {
				errs = append(errs, fmt.Sprintf("\"--%s\" requires API version %s, but the Docker daemon API version is %s", f.Name, getFlagVersion(f), clientVersion))
				return
			}
			if _, ok := f.Annotations["experimental"]; ok && !hasExperimental {
				errs = append(errs, fmt.Sprintf("\"--%s\" is only supported on a Docker daemon with experimental features enabled", f.Name))
			}
		}
	})
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}

	return nil
}

func getFlagVersion(f *pflag.Flag) string {
	if flagVersion, ok := f.Annotations["version"]; ok && len(flagVersion) == 1 {
		return flagVersion[0]
	}
	return ""
}

func isFlagSupported(f *pflag.Flag, clientVersion string) bool {
	if v := getFlagVersion(f); v != "" {
		return versions.GreaterThanOrEqualTo(clientVersion, v)
	}
	return true
}

// REQUIRED BY dockerVolumeList

type volumeListOptions struct {
	quiet  bool
	format string
	filter opts.FilterOpt
}

// REQUIRED BY dockerNetworkList

type networkListOptions struct {
	quiet   bool
	noTrunc bool
	format  string
	filter  opts.FilterOpt
}

// REQUIRED BY dockerServiceList

type listServiceOptions struct {
	quiet  bool
	format string
	filter opts.FilterOpt
}

// REQUIRED BY dockerSecretList

type listSecretOptions struct {
	quiet bool
}
