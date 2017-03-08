package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/analytics"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/commands"
	cliconfig "github.com/docker/docker/cli/config"
	"github.com/docker/docker/cli/debug"
	cliflags "github.com/docker/docker/cli/flags"
	"github.com/docker/docker/dockerversion"
	sandbox "github.com/docker/docker/lua-sandbox"
	"github.com/docker/docker/pkg/term"
	project "github.com/docker/docker/proj"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

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
			return isSupported(cmd, dockerCli.Client().ClientVersion(), dockerCli.OSType(), dockerCli.HasExperimental())
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			analytics.Close()
		},
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

func setFlagErrorFunc(dockerCli *command.DockerCli, cmd *cobra.Command, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	// When invoking `docker stack --nonsense`, we need to make sure FlagErrorFunc return appropriate
	// output if the feature is not supported.
	// As above cli.SetupRootCommand(cmd) have already setup the FlagErrorFunc, we will add a pre-check before the FlagErrorFunc
	// is called.
	flagErrorFunc := cmd.FlagErrorFunc()
	cmd.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		initializeDockerCli(dockerCli, flags, opts)
		if err := isSupported(cmd, dockerCli.Client().ClientVersion(), dockerCli.OSType(), dockerCli.HasExperimental()); err != nil {
			return err
		}
		return flagErrorFunc(cmd, err)
	})
}

func setHelpFunc(dockerCli *command.DockerCli, cmd *cobra.Command, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	cmd.SetHelpFunc(func(ccmd *cobra.Command, args []string) {
		initializeDockerCli(dockerCli, flags, opts)
		if err := isSupported(ccmd, dockerCli.Client().ClientVersion(), dockerCli.OSType(), dockerCli.HasExperimental()); err != nil {
			ccmd.Println(err)
			return
		}

		hideUnsupportedFeatures(ccmd, dockerCli.Client().ClientVersion(), dockerCli.OSType(), dockerCli.HasExperimental())

		if err := ccmd.Help(); err != nil {
			ccmd.Println(err)
		}
	})
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
			if err := isSupported(cmd, dockerCli.Client().ClientVersion(), dockerCli.OSType(), dockerCli.HasExperimental()); err != nil {
				return err
			}
			return cmdArgs(cmd, args)
		}
	})
}

func initializeDockerCli(dockerCli *command.DockerCli, flags *pflag.FlagSet, opts *cliflags.ClientOptions) {
	if dockerCli.Client() == nil { // when using --help, PersistentPreRun is not called, so initialization is needed.
		// flags must be the top-level command flags, not cmd.Flags()
		opts.Common.SetDefaultOptions(flags)
		dockerPreRun(opts)
		dockerCli.Initialize(opts)
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

func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf(
		"docker: '%s' is not a docker command.\nSee 'docker --help'", args[0])
}

func main() {
	os.Setenv("DOCKER_HIDE_LEGACY_COMMANDS", "1")

	// Set terminal emulation based on platform as required.
	stdin, stdout, stderr := term.StdStreams()
	logrus.SetOutput(stderr)

	dockerCli := command.NewDockerCli(stdin, stdout, stderr)

	// see if we're in the context of a Docker project or not
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return
	}
	proj, err := project.Get(wd)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return
	}

	cmd := newDockerCommand(dockerCli)

	// sandbox is used only if we are in the context of a docker project
	if proj != nil {

		projectCmds, err := proj.ListCustomCommands()
		if err != nil {
			fmt.Fprintln(stderr, err)
			return
		}

		// TODO: look for command in the list

		// TODO: update
		// check for prohibited command override
		overriddenCmds, err := overriddenCommands(projectCmds, cmd)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return
		}
		if overriddenCmds != nil && len(overriddenCmds) > 0 {
			errorMessage := "ERROR: one or several Docker commands are overridden in Dockerscript ["
			for _, c := range overriddenCmds {
				errorMessage = errorMessage + c + ","
			}
			errorMessage = errorMessage[:len(errorMessage)-1] + "]"
			fmt.Fprintln(stderr, errors.New(errorMessage))
			return
		}

		// create Lua sandbox
		sb, err := sandbox.NewSandbox(proj)
		if err != nil {
			fmt.Fprintln(stderr, err.Error())
			return
		}

		// this is required to initialize dockerCli
		// (done in PersistentPreRunE when not using Lua sandbox)
		opts := cliflags.NewClientOptions()
		// flags have already been initialized when calling newDockerCommand
		// just getting pointer to them
		flags := cmd.Flags()
		opts.Common.SetDefaultOptions(flags)
		dockerPreRun(opts)
		if err = dockerCli.Initialize(opts); err != nil {
			fmt.Fprintln(stderr, err.Error())
			return
		}

		// if found, execute command
		for cmdName, cmdContent := range projectCmds {
			if len(os.Args) > 1 && cmdName == os.Args[1] {
				luaArgs := []string{cmdContent.Name}
				luaArgs = append(luaArgs, os.Args[2:]...)
				found, err := sb.Exec(luaArgs)
				if found {
					if err != nil {
						fmt.Fprintln(stderr, err.Error())
						return
					}
					analytics.Event("command", map[string]interface{}{"name": "docker " + os.Args[1], "lua": true})
					analytics.Close()
					return
				}
				break
			}
		}
	}

	if err := cmd.Execute(); err != nil {
		if sterr, ok := err.(cli.StatusError); ok {
			if sterr.Status != "" {
				fmt.Fprintln(stderr, sterr.Status)
			}
			// StatusError should only be used for errors, and all errors should
			// have a non-zero exit status, so never exit with 0
			if sterr.StatusCode == 0 {
				os.Exit(1)
			}
			os.Exit(sterr.StatusCode)
		}
		fmt.Fprintln(stderr, err)
		os.Exit(1)
	}
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

func hideUnsupportedFeatures(cmd *cobra.Command, clientVersion, osType string, hasExperimental bool) {
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

func isSupported(cmd *cobra.Command, clientVersion, osType string, hasExperimental bool) error {
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

func getFlagAnnotation(f *pflag.Flag, annotation string) string {
	if value, ok := f.Annotations[annotation]; ok && len(value) == 1 {
		return value[0]
	}
	return ""
}

func isVersionSupported(f *pflag.Flag, clientVersion string) bool {
	if v := getFlagAnnotation(f, "version"); v != "" {
		return versions.GreaterThanOrEqualTo(clientVersion, v)
	}
	return true
}

func isOSTypeSupported(f *pflag.Flag, osType string) bool {
	if v := getFlagAnnotation(f, "ostype"); v != "" && osType != "" {
		return osType == v
	}
	return true
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

// overriddenCommands ...
func overriddenCommands(projCmds map[string]project.ProjectCommand, cmd *cobra.Command) ([]string, error) {
	if cmd == nil {
		return nil, errors.New("cmd is nil")
	}

	overriddenFuncs := make([]string, 0)

	// all Lua global functions
	for cmdName := range projCmds {
		mainCmds := cmd.Commands()
		for _, mainCmd := range mainCmds {
			if cmdName == mainCmd.Name() {
				overriddenFuncs = append(overriddenFuncs, cmdName)
				break
			}
		}
	}
	return overriddenFuncs, nil
}
