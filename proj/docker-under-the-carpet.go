package project

import (
	"errors"
	"fmt"
	"io/ioutil"
	"runtime"
	"strings"

	"github.com/docker/docker/analytics"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/commands"
	cliconfig "github.com/docker/docker/cli/config"
	"github.com/docker/docker/cli/debug"
	cliflags "github.com/docker/docker/cli/flags"
	"github.com/docker/docker/client"
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

// REQUIRED BY dockerContainerInspect

type containerInspectOptions struct {
	format string
	size   bool
	refs   []string
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

			completeCmdName := cmd.Name()
			cobracmd := cmd
			for cobracmd.HasParent() {
				cobracmd = cobracmd.Parent()
				completeCmdName = cobracmd.Name() + " " + completeCmdName
			}
			analytics.Event("command", map[string]interface{}{"name": completeCmdName, "lua": false})

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
			return isSupported(cmd, dockerCli)
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {},
	}
	cli.SetupRootCommand(cmd)

	flags = cmd.Flags()
	flags.BoolVarP(&opts.Version, "version", "v", false, "Print version information and quit")
	flags.StringVar(&opts.ConfigDir, "config", cliconfig.Dir(), "Location of client config files")
	opts.Common.InstallFlags(flags)

	setFlagErrorFunc(dockerCli, cmd, flags, opts)

	setHelpFunc(dockerCli, cmd, flags, opts)

	cmd.SetOutput(dockerCli.Out())
	cmd.AddCommand(newDaemonCommand())
	commands.AddCommands(cmd, dockerCli)

	setValidateArgs(dockerCli, cmd, flags, opts)

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

func isSupported(cmd *cobra.Command, details versionDetails) error {
	clientVersion := details.Client().ClientVersion()
	osType := details.ServerInfo().OSType
	hasExperimental := details.ServerInfo().HasExperimental

	// Check recursively so that, e.g., `docker stack ls` returns the same output as `docker stack`
	for curr := cmd; curr != nil; curr = curr.Parent() {
		if cmdVersion, ok := curr.Tags["version"]; ok && versions.LessThan(clientVersion, cmdVersion) {
			return fmt.Errorf("%s requires API version %s, but the Docker daemon API version is %s", cmd.CommandPath(), cmdVersion, clientVersion)
		}
		if _, ok := curr.Tags["experimental"]; ok && !hasExperimental {
			return fmt.Errorf("%s is only supported on a Docker daemon with experimental features enabled", cmd.CommandPath())
		}
	}

	errs := []string{}

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Changed {
			if !isVersionSupported(f, clientVersion) {
				errs = append(errs, fmt.Sprintf("\"--%s\" requires API version %s, but the Docker daemon API version is %s", f.Name, getFlagAnnotation(f, "version"), clientVersion))
				return
			}
			if !isOSTypeSupported(f, osType) {
				errs = append(errs, fmt.Sprintf("\"--%s\" requires the Docker daemon to run on %s, but the Docker daemon is running on %s", f.Name, getFlagAnnotation(f, "ostype"), osType))
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

func setFlagErrorFunc(dockerCli *command.DockerCli, cmd *cobra.Command, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	// When invoking `docker stack --nonsense`, we need to make sure FlagErrorFunc return appropriate
	// output if the feature is not supported.
	// As above cli.SetupRootCommand(cmd) have already setup the FlagErrorFunc, we will add a pre-check before the FlagErrorFunc
	// is called.
	flagErrorFunc := cmd.FlagErrorFunc()
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		initializeDockerCli(dockerCli, flags, opts)
		if err := isSupported(cmd, dockerCli); err != nil {
			return err
		}
		return flagErrorFunc(cmd, err)
	})
}

func setHelpFunc(dockerCli *command.DockerCli, cmd *cobra.Command, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	cmd.SetHelpFunc(func(ccmd *cobra.Command, args []string) {
		initializeDockerCli(dockerCli, flags, opts)
		if err := isSupported(ccmd, dockerCli); err != nil {
			ccmd.Println(err)
			return
		}

		hideUnsupportedFeatures(ccmd, dockerCli)

		if err := ccmd.Help(); err != nil {
			ccmd.Println(err)
		}
	})
}

func newDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:                "daemon",
		Hidden:             true,
		Args:               cobra.ArbitraryArgs,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon()
		},
	}
}

func runDaemon() error {
	return fmt.Errorf(
		"`docker daemon` is not supported on %s. Please run `dockerd` directly",
		strings.Title(runtime.GOOS))
}

func setValidateArgs(dockerCli *command.DockerCli, cmd *cobra.Command, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	// The Args is handled by ValidateArgs in cobra, which does not allows a pre-hook.
	// As a result, here we replace the existing Args validation func to a wrapper,
	// where the wrapper will check to see if the feature is supported or not.
	// The Args validation error will only be returned if the feature is supported.
	visitAll(cmd, func(ccmd *cobra.Command) {
		// if there is no tags for a command or any of its parent,
		// there is no need to wrap the Args validation.
		if !hasTags(ccmd) {
			return
		}

		if ccmd.Args == nil {
			return
		}

		cmdArgs := ccmd.Args
		ccmd.Args = func(cmd *cobra.Command, args []string) error {
			initializeDockerCli(dockerCli, flags, opts)
			if err := isSupported(cmd, dockerCli); err != nil {
				return err
			}
			return cmdArgs(cmd, args)
		}
	})
}

type versionDetails interface {
	Client() client.APIClient
	ServerInfo() command.ServerInfo
}

func isVersionSupported(f *pflag.Flag, clientVersion string) bool {
	if v := getFlagAnnotation(f, "version"); v != "" {
		return versions.GreaterThanOrEqualTo(clientVersion, v)
	}
	return true
}

func getFlagAnnotation(f *pflag.Flag, annotation string) string {
	if value, ok := f.Annotations[annotation]; ok && len(value) == 1 {
		return value[0]
	}
	return ""
}

func isOSTypeSupported(f *pflag.Flag, osType string) bool {
	if v := getFlagAnnotation(f, "ostype"); v != "" && osType != "" {
		return osType == v
	}
	return true
}

func initializeDockerCli(dockerCli *command.DockerCli, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	if dockerCli.Client() == nil { // when using --help, PersistentPreRun is not called, so initialization is needed.
		// flags must be the top-level command flags, not cmd.Flags()
		opts.Common.SetDefaultOptions(flags)
		dockerPreRun(opts)
		dockerCli.Initialize(opts)
	}
}

func hideUnsupportedFeatures(cmd *cobra.Command, details versionDetails) {
	clientVersion := details.Client().ClientVersion()
	osType := details.ServerInfo().OSType
	hasExperimental := details.ServerInfo().HasExperimental

	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		// hide experimental flags
		if !hasExperimental {
			if _, ok := f.Annotations["experimental"]; ok {
				f.Hidden = true
			}
		}

		// hide flags not supported by the server
		if !isOSTypeSupported(f, osType) || !isVersionSupported(f, clientVersion) {
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

// visitAll will traverse all commands from the root.
// This is different from the VisitAll of cobra.Command where only parents
// are checked.
func visitAll(root *cobra.Command, fn func(*cobra.Command)) {
	for _, cmd := range root.Commands() {
		visitAll(cmd, fn)
	}
	fn(root)
}

// hasTags return true if any of the command's parents has tags
func hasTags(cmd *cobra.Command) bool {
	for curr := cmd; curr != nil; curr = curr.Parent() {
		if len(curr.Tags) > 0 {
			return true
		}
	}

	return false
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
