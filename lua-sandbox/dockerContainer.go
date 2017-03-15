package sandbox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http/httputil"
	"runtime"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/container"
	"github.com/docker/docker/opts"
	opttypes "github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/promise"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/libnetwork/resolvconf/dns"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerContainerList lists Docker containers and returns a Lua table (array)
// containing the containers' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker container ls` command.
// docker.container.list(arguments string)
func (s *Sandbox) dockerContainerList(L *lua.LState) int {
	var err error

	// retrieve parameter
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	// it's ok if we can't find parameter, it's optional

	args := make([]string, 0)

	if found {
		args, err = shellwords.Parse(argsStr)
		if err != nil {
			L.RaiseError(err.Error())
			return 0
		}
	}

	// accept same flags as in `docker container ls`
	opts := psOptions{filter: opts.NewFilterOpt()}

	flags := pflag.NewFlagSet("dockerContainerList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only display numeric IDs")
	flags.BoolVarP(&opts.size, "size", "s", false, "Display total file sizes")
	flags.BoolVarP(&opts.all, "all", "a", false, "Show all containers (default shows just running)")
	flags.BoolVar(&opts.noTrunc, "no-trunc", false, "Don't truncate output")
	flags.BoolVarP(&opts.nLatest, "latest", "l", false, "Show the latest created container (includes all states)")
	flags.IntVarP(&opts.last, "last", "n", -1, "Show n last created containers (includes all states)")
	flags.StringVarP(&opts.format, "format", "", "", "Pretty-print containers using a Go template")
	flags.VarP(&opts.filter, "filter", "f", "Filter output based on conditions provided")

	flags.Parse(args)

	listOptions, err := buildContainerListOptions(&opts)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	ctx := context.Background()

	dockerCli := newDockerCli()
	containers, err := dockerCli.Client().ContainerList(ctx, *listOptions)
	if err != nil {
		fmt.Println("ERROR:", err.Error())
		L.RaiseError(err.Error())
		return 0
	}

	// create lua table listing containers

	containersTbl := s.luaState.CreateTable(0, 0)

	for _, container := range containers {

		containerTbl := s.luaState.CreateTable(0, 0)
		containerTbl.RawSetString("id", lua.LString(container.ID))

		containerNamesTbl := s.luaState.CreateTable(0, 0)
		if len(container.Names) > 0 {
			// TODO: why is there a "/" prefix?
			// removing it for now to make it easier when writing scripts
			containerTbl.RawSetString("name", lua.LString(strings.TrimPrefix(container.Names[0], "/")))
			for _, name := range container.Names {
				containerNamesTbl.Append(lua.LString(strings.TrimPrefix(name, "/")))
			}
		} else {
			containerTbl.RawSetString("name", lua.LString(""))
		}
		containerTbl.RawSetString("names", containerNamesTbl)

		containerTbl.RawSetString("image", lua.LString(container.Image))

		// image id
		// removing prefixes like in image ids like:
		// sha256:5dae07823d481dab69d6a278b4014cb2978b96ef0874ac18fd2ad050a2a32699
		imageID := container.ImageID
		parts := strings.SplitN(imageID, ":", 2)
		if len(parts) > 1 {
			imageID = parts[1]
		}

		containerTbl.RawSetString("imageId", lua.LString(imageID))
		containerTbl.RawSetString("created", lua.LNumber(container.Created))
		containerTbl.RawSetString("sizeRw", lua.LNumber(container.SizeRw))
		containerTbl.RawSetString("sizeRootFs", lua.LNumber(container.SizeRootFs))
		containerTbl.RawSetString("state", lua.LString(container.State))
		containerTbl.RawSetString("status", lua.LString(container.Status))

		// ports
		containerPortsTbl := s.luaState.CreateTable(0, 0)
		for _, port := range container.Ports {
			containerPortTbl := s.luaState.CreateTable(0, 0)
			containerPortTbl.RawSetString("ip", lua.LString(port.IP))
			containerPortTbl.RawSetString("public", lua.LNumber(port.PublicPort))
			containerPortTbl.RawSetString("private", lua.LNumber(port.PrivatePort))
			containerPortTbl.RawSetString("type", lua.LString(port.Type))
			containerPortTbl.RawSetString("string", lua.LString(api.DisplayablePorts([]types.Port{port})))

			containerPortsTbl.Append(containerPortTbl)
		}
		containerTbl.RawSetString("ports", containerPortsTbl)

		// labels
		containerLabelsTbl := s.luaState.CreateTable(0, 0)
		for key, value := range container.Labels {
			containerLabelsTbl.RawSetString(key, lua.LString(value))
		}
		containerTbl.RawSetString("labels", containerLabelsTbl)

		// TODO: Mounts, NetworkSettings & HostConfig

		containersTbl.Append(containerTbl)
	}

	s.luaState.Push(containersTbl)
	return 1
}

// dockerContainerRun ...
// - has no return value except when --detach is used (in which case the container id is returned as a string)
func (s *Sandbox) dockerContainerRun(L *lua.LState) int {
	var err error
	var retContainerID string

	// retrieve parameter
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if found == false {
		L.RaiseError("function requires 1 argument")
		return 0
	}
	var argsArr []string
	argsArr, err = shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	var opts runOptions
	var copts *containerOptions

	flags := pflag.NewFlagSet("dockerContainerList", pflag.ExitOnError)
	flags.SetInterspersed(false)

	// These are flags not stored in Config/HostConfig
	flags.BoolVarP(&opts.detach, "detach", "d", false, "Run container in background and print container ID")
	flags.BoolVar(&opts.sigProxy, "sig-proxy", true, "Proxy received signals to the process")
	flags.StringVar(&opts.name, "name", "", "Assign a name to the container")
	flags.StringVar(&opts.detachKeys, "detach-keys", "", "Override the key sequence for detaching a container")

	// Add an explicit help that doesn't have a `-h` to prevent the conflict
	// with hostname
	flags.Bool("help", false, "Print usage")
	command.AddTrustVerificationFlags(flags)

	copts = addFlags(flags, argsArr) // this parses flags as well

	// get the non-flag command-line arguments
	args := flags.Args()

	copts.Image = args[0]
	if len(args) > 1 {
		copts.Args = args[1:]
	}

	dockerCli := newDockerCli()

	stdout, stderr, stdin := dockerCli.Out(), dockerCli.Err(), dockerCli.In()
	client := dockerCli.Client()
	// TODO: pass this as an argument
	cmdPath := "run"

	var (
		flAttach                *opttypes.ListOpts
		ErrConflictAttachDetach = errors.New("Conflicting options: -a and -d")
	)

	config, hostConfig, networkingConfig, err := parse(flags, copts)

	// just in case the parse does not exit
	if err != nil {
		reportError(stderr, cmdPath, err.Error(), true)
		L.RaiseError(cli.StatusError{StatusCode: 125}.Error())
		return 0
	}

	if hostConfig.OomKillDisable != nil && *hostConfig.OomKillDisable && hostConfig.Memory == 0 {
		fmt.Fprintln(stderr, "WARNING: Disabling the OOM killer on containers without setting a '-m/--memory' limit may be dangerous.")
	}

	if len(hostConfig.DNS) > 0 {
		// check the DNS settings passed via --dns against
		// localhost regexp to warn if they are trying to
		// set a DNS to a localhost address
		for _, dnsIP := range hostConfig.DNS {
			if dns.IsLocalhost(dnsIP) {
				fmt.Fprintf(stderr, "WARNING: Localhost DNS setting (--dns=%s) may fail in containers.\n", dnsIP)
				break
			}
		}
	}

	config.ArgsEscaped = false

	if !opts.detach {
		if err := dockerCli.In().CheckTty(config.AttachStdin, config.Tty); err != nil {
			L.RaiseError(err.Error())
			return 0
		}
	} else {
		if fl := flags.Lookup("attach"); fl != nil {
			flAttach = fl.Value.(*opttypes.ListOpts)
			if flAttach.Len() != 0 {
				L.RaiseError(ErrConflictAttachDetach.Error())
				return 0
			}
		}

		config.AttachStdin = false
		config.AttachStdout = false
		config.AttachStderr = false
		config.StdinOnce = false
	}

	// Disable sigProxy when in TTY mode
	if config.Tty {
		opts.sigProxy = false
	}

	// Telling the Windows daemon the initial size of the tty during start makes
	// a far better user experience rather than relying on subsequent resizes
	// to cause things to catch up.
	if runtime.GOOS == "windows" {
		hostConfig.ConsoleSize[0], hostConfig.ConsoleSize[1] = dockerCli.Out().GetTtySize()
	}

	ctx, cancelFun := context.WithCancel(context.Background())

	createResponse, err := createContainer(ctx, dockerCli, config, hostConfig, networkingConfig, hostConfig.ContainerIDFile, opts.name)
	if err != nil {
		reportError(stderr, cmdPath, err.Error(), true)
		L.RaiseError(runStartContainerErr(err).Error())
		return 0
	}

	retContainerID = createResponse.ID

	if opts.sigProxy {
		sigc := container.ForwardAllSignals(ctx, dockerCli, createResponse.ID)
		defer signal.StopCatch(sigc)
	}
	var (
		waitDisplayID chan struct{}
		errCh         chan error
	)
	if !config.AttachStdout && !config.AttachStderr {
		// Make this asynchronous to allow the client to write to stdin before having to read the ID
		waitDisplayID = make(chan struct{})
		go func() {
			defer close(waitDisplayID)
			fmt.Fprintln(stdout, createResponse.ID)
		}()
	}
	attach := config.AttachStdin || config.AttachStdout || config.AttachStderr
	if attach {
		var (
			out, cerr io.Writer
			in        io.ReadCloser
		)
		if config.AttachStdin {
			in = stdin
		}
		if config.AttachStdout {
			out = stdout
		}
		if config.AttachStderr {
			if config.Tty {
				cerr = stdout
			} else {
				cerr = stderr
			}
		}

		if opts.detachKeys != "" {
			dockerCli.ConfigFile().DetachKeys = opts.detachKeys
		}

		options := types.ContainerAttachOptions{
			Stream:     true,
			Stdin:      config.AttachStdin,
			Stdout:     config.AttachStdout,
			Stderr:     config.AttachStderr,
			DetachKeys: dockerCli.ConfigFile().DetachKeys,
		}

		resp, errAttach := client.ContainerAttach(ctx, createResponse.ID, options)
		if errAttach != nil && errAttach != httputil.ErrPersistEOF {
			// ContainerAttach returns an ErrPersistEOF (connection closed)
			// means server met an error and put it in Hijacked connection
			// keep the error and read detailed error message from hijacked connection later
			L.RaiseError(errAttach.Error())
			return 0
		}
		defer resp.Close()

		errCh = promise.Go(func() error {
			if errHijack := holdHijackedConnection(ctx, dockerCli, config.Tty, in, out, cerr, resp); errHijack != nil {
				return errHijack
			}
			return errAttach
		})
	}

	statusChan := waitExitOrRemoved(ctx, dockerCli, createResponse.ID, copts.autoRemove)

	//start the container
	if err := client.ContainerStart(ctx, createResponse.ID, types.ContainerStartOptions{}); err != nil {
		// If we have holdHijackedConnection, we should notify
		// holdHijackedConnection we are going to exit and wait
		// to avoid the terminal are not restored.
		if attach {
			cancelFun()
			<-errCh
		}

		reportError(stderr, cmdPath, err.Error(), false)
		if copts.autoRemove {
			// wait container to be removed
			<-statusChan
		}
		L.RaiseError(runStartContainerErr(err).Error())
		return 0
	}

	if (config.AttachStdin || config.AttachStdout || config.AttachStderr) && config.Tty && dockerCli.Out().IsTerminal() {
		if err := container.MonitorTtySize(ctx, dockerCli, createResponse.ID, false); err != nil {
			fmt.Fprintln(stderr, "Error monitoring TTY size:", err)
		}
	}

	if errCh != nil {
		if err := <-errCh; err != nil {
			logrus.Debugf("Error hijack: %s", err)
			L.RaiseError(err.Error())
			return 0
		}
	}

	// Detached mode: wait for the id to be displayed and return.
	if !config.AttachStdout && !config.AttachStderr {
		// Detached mode
		<-waitDisplayID
		luaRet := lua.LString(retContainerID)
		L.Push(luaRet)
		return 1
	}

	status := <-statusChan
	if status != 0 {
		L.RaiseError(cli.StatusError{StatusCode: status}.Error())
		return 0
	}
	return 0
}
