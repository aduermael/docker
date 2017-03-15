package sandbox

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/docker/api/types/versions"
	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/commands"
	"github.com/docker/docker/cli/command/image"
	cliconfig "github.com/docker/docker/cli/config"
	"github.com/docker/docker/cli/debug"
	cliflags "github.com/docker/docker/cli/flags"
	apiclient "github.com/docker/docker/client"
	"github.com/docker/docker/dockerversion"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/signal"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/docker/pkg/templates"
	project "github.com/docker/docker/proj"
	"github.com/docker/docker/registry"
	runconfigopts "github.com/docker/docker/runconfig/opts"
	"github.com/docker/go-connections/nat"
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

// REQUIRED BY dockerContainerRun

type runOptions struct {
	detach     bool
	sigProxy   bool
	name       string
	detachKeys string
}

// containerOptions is a data object with all the options for creating a container
type containerOptions struct {
	attach             opts.ListOpts
	volumes            opts.ListOpts
	tmpfs              opts.ListOpts
	blkioWeightDevice  opts.WeightdeviceOpt
	deviceReadBps      opts.ThrottledeviceOpt
	deviceWriteBps     opts.ThrottledeviceOpt
	links              opts.ListOpts
	aliases            opts.ListOpts
	linkLocalIPs       opts.ListOpts
	deviceReadIOps     opts.ThrottledeviceOpt
	deviceWriteIOps    opts.ThrottledeviceOpt
	env                opts.ListOpts
	labels             opts.ListOpts
	deviceCgroupRules  opts.ListOpts
	devices            opts.ListOpts
	ulimits            *opts.UlimitOpt
	sysctls            *opts.MapOpts
	publish            opts.ListOpts
	expose             opts.ListOpts
	dns                opts.ListOpts
	dnsSearch          opts.ListOpts
	dnsOptions         opts.ListOpts
	extraHosts         opts.ListOpts
	volumesFrom        opts.ListOpts
	envFile            opts.ListOpts
	capAdd             opts.ListOpts
	capDrop            opts.ListOpts
	groupAdd           opts.ListOpts
	securityOpt        opts.ListOpts
	storageOpt         opts.ListOpts
	labelsFile         opts.ListOpts
	loggingOpts        opts.ListOpts
	privileged         bool
	pidMode            string
	utsMode            string
	usernsMode         string
	publishAll         bool
	stdin              bool
	tty                bool
	oomKillDisable     bool
	oomScoreAdj        int
	containerIDFile    string
	entrypoint         string
	hostname           string
	memory             opts.MemBytes
	memoryReservation  opts.MemBytes
	memorySwap         opts.MemSwapBytes
	kernelMemory       opts.MemBytes
	user               string
	workingDir         string
	cpuCount           int64
	cpuShares          int64
	cpuPercent         int64
	cpuPeriod          int64
	cpuRealtimePeriod  int64
	cpuRealtimeRuntime int64
	cpuQuota           int64
	cpus               opts.NanoCPUs
	cpusetCpus         string
	cpusetMems         string
	blkioWeight        uint16
	ioMaxBandwidth     opts.MemBytes
	ioMaxIOps          uint64
	swappiness         int64
	netMode            string
	macAddress         string
	ipv4Address        string
	ipv6Address        string
	ipcMode            string
	pidsLimit          int64
	restartPolicy      string
	readonlyRootfs     bool
	loggingDriver      string
	cgroupParent       string
	volumeDriver       string
	stopSignal         string
	stopTimeout        int
	isolation          string
	shmSize            opts.MemBytes
	noHealthcheck      bool
	healthCmd          string
	healthInterval     time.Duration
	healthTimeout      time.Duration
	healthRetries      int
	runtime            string
	autoRemove         bool
	init               bool
	initPath           string
	credentialSpec     string

	Image string
	Args  []string
}

func addFlags(flags *pflag.FlagSet, args []string) *containerOptions {
	copts := &containerOptions{
		aliases:           opts.NewListOpts(nil),
		attach:            opts.NewListOpts(validateAttach),
		blkioWeightDevice: opts.NewWeightdeviceOpt(opts.ValidateWeightDevice),
		capAdd:            opts.NewListOpts(nil),
		capDrop:           opts.NewListOpts(nil),
		dns:               opts.NewListOpts(opts.ValidateIPAddress),
		dnsOptions:        opts.NewListOpts(nil),
		dnsSearch:         opts.NewListOpts(opts.ValidateDNSSearch),
		deviceCgroupRules: opts.NewListOpts(validateDeviceCgroupRule),
		deviceReadBps:     opts.NewThrottledeviceOpt(opts.ValidateThrottleBpsDevice),
		deviceReadIOps:    opts.NewThrottledeviceOpt(opts.ValidateThrottleIOpsDevice),
		deviceWriteBps:    opts.NewThrottledeviceOpt(opts.ValidateThrottleBpsDevice),
		deviceWriteIOps:   opts.NewThrottledeviceOpt(opts.ValidateThrottleIOpsDevice),
		devices:           opts.NewListOpts(validateDevice),
		env:               opts.NewListOpts(opts.ValidateEnv),
		envFile:           opts.NewListOpts(nil),
		expose:            opts.NewListOpts(nil),
		extraHosts:        opts.NewListOpts(opts.ValidateExtraHost),
		groupAdd:          opts.NewListOpts(nil),
		labels:            opts.NewListOpts(opts.ValidateEnv),
		labelsFile:        opts.NewListOpts(nil),
		linkLocalIPs:      opts.NewListOpts(nil),
		links:             opts.NewListOpts(opts.ValidateLink),
		loggingOpts:       opts.NewListOpts(nil),
		publish:           opts.NewListOpts(nil),
		securityOpt:       opts.NewListOpts(nil),
		storageOpt:        opts.NewListOpts(nil),
		sysctls:           opts.NewMapOpts(nil, opts.ValidateSysctl),
		tmpfs:             opts.NewListOpts(nil),
		ulimits:           opts.NewUlimitOpt(nil),
		volumes:           opts.NewListOpts(nil),
		volumesFrom:       opts.NewListOpts(nil),
	}

	// General purpose flags
	flags.VarP(&copts.attach, "attach", "a", "Attach to STDIN, STDOUT or STDERR")
	flags.Var(&copts.deviceCgroupRules, "device-cgroup-rule", "Add a rule to the cgroup allowed devices list")
	flags.Var(&copts.devices, "device", "Add a host device to the container")
	flags.VarP(&copts.env, "env", "e", "Set environment variables")
	flags.Var(&copts.envFile, "env-file", "Read in a file of environment variables")
	flags.StringVar(&copts.entrypoint, "entrypoint", "", "Overwrite the default ENTRYPOINT of the image")
	flags.Var(&copts.groupAdd, "group-add", "Add additional groups to join")
	flags.StringVarP(&copts.hostname, "hostname", "h", "", "Container host name")
	flags.BoolVarP(&copts.stdin, "interactive", "i", false, "Keep STDIN open even if not attached")
	flags.VarP(&copts.labels, "label", "l", "Set meta data on a container")
	flags.Var(&copts.labelsFile, "label-file", "Read in a line delimited file of labels")
	flags.BoolVar(&copts.readonlyRootfs, "read-only", false, "Mount the container's root filesystem as read only")
	flags.StringVar(&copts.restartPolicy, "restart", "no", "Restart policy to apply when a container exits")
	flags.StringVar(&copts.stopSignal, "stop-signal", signal.DefaultStopSignal, "Signal to stop a container")
	flags.IntVar(&copts.stopTimeout, "stop-timeout", 0, "Timeout (in seconds) to stop a container")
	flags.SetAnnotation("stop-timeout", "version", []string{"1.25"})
	flags.Var(copts.sysctls, "sysctl", "Sysctl options")
	flags.BoolVarP(&copts.tty, "tty", "t", false, "Allocate a pseudo-TTY")
	flags.Var(copts.ulimits, "ulimit", "Ulimit options")
	flags.StringVarP(&copts.user, "user", "u", "", "Username or UID (format: <name|uid>[:<group|gid>])")
	flags.StringVarP(&copts.workingDir, "workdir", "w", "", "Working directory inside the container")
	flags.BoolVar(&copts.autoRemove, "rm", false, "Automatically remove the container when it exits")

	// Security
	flags.Var(&copts.capAdd, "cap-add", "Add Linux capabilities")
	flags.Var(&copts.capDrop, "cap-drop", "Drop Linux capabilities")
	flags.BoolVar(&copts.privileged, "privileged", false, "Give extended privileges to this container")
	flags.Var(&copts.securityOpt, "security-opt", "Security Options")
	flags.StringVar(&copts.usernsMode, "userns", "", "User namespace to use")
	flags.StringVar(&copts.credentialSpec, "credentialspec", "", "Credential spec for managed service account (Windows only)")
	flags.SetAnnotation("credentialspec", "ostype", []string{"windows"})

	// Network and port publishing flag
	flags.Var(&copts.extraHosts, "add-host", "Add a custom host-to-IP mapping (host:ip)")
	flags.Var(&copts.dns, "dns", "Set custom DNS servers")
	// We allow for both "--dns-opt" and "--dns-option", although the latter is the recommended way.
	// This is to be consistent with service create/update
	flags.Var(&copts.dnsOptions, "dns-opt", "Set DNS options")
	flags.Var(&copts.dnsOptions, "dns-option", "Set DNS options")
	flags.MarkHidden("dns-opt")
	flags.Var(&copts.dnsSearch, "dns-search", "Set custom DNS search domains")
	flags.Var(&copts.expose, "expose", "Expose a port or a range of ports")
	flags.StringVar(&copts.ipv4Address, "ip", "", "IPv4 address (e.g., 172.30.100.104)")
	flags.StringVar(&copts.ipv6Address, "ip6", "", "IPv6 address (e.g., 2001:db8::33)")
	flags.Var(&copts.links, "link", "Add link to another container")
	flags.Var(&copts.linkLocalIPs, "link-local-ip", "Container IPv4/IPv6 link-local addresses")
	flags.StringVar(&copts.macAddress, "mac-address", "", "Container MAC address (e.g., 92:d0:c6:0a:29:33)")
	flags.VarP(&copts.publish, "publish", "p", "Publish a container's port(s) to the host")
	flags.BoolVarP(&copts.publishAll, "publish-all", "P", false, "Publish all exposed ports to random ports")
	// We allow for both "--net" and "--network", although the latter is the recommended way.
	flags.StringVar(&copts.netMode, "net", "default", "Connect a container to a network")
	flags.StringVar(&copts.netMode, "network", "default", "Connect a container to a network")
	flags.MarkHidden("net")
	// We allow for both "--net-alias" and "--network-alias", although the latter is the recommended way.
	flags.Var(&copts.aliases, "net-alias", "Add network-scoped alias for the container")
	flags.Var(&copts.aliases, "network-alias", "Add network-scoped alias for the container")
	flags.MarkHidden("net-alias")

	// Logging and storage
	flags.StringVar(&copts.loggingDriver, "log-driver", "", "Logging driver for the container")
	flags.StringVar(&copts.volumeDriver, "volume-driver", "", "Optional volume driver for the container")
	flags.Var(&copts.loggingOpts, "log-opt", "Log driver options")
	flags.Var(&copts.storageOpt, "storage-opt", "Storage driver options for the container")
	flags.Var(&copts.tmpfs, "tmpfs", "Mount a tmpfs directory")
	flags.Var(&copts.volumesFrom, "volumes-from", "Mount volumes from the specified container(s)")
	flags.VarP(&copts.volumes, "volume", "v", "Bind mount a volume")

	// Health-checking
	flags.StringVar(&copts.healthCmd, "health-cmd", "", "Command to run to check health")
	flags.DurationVar(&copts.healthInterval, "health-interval", 0, "Time between running the check (ns|us|ms|s|m|h) (default 0s)")
	flags.IntVar(&copts.healthRetries, "health-retries", 0, "Consecutive failures needed to report unhealthy")
	flags.DurationVar(&copts.healthTimeout, "health-timeout", 0, "Maximum time to allow one check to run (ns|us|ms|s|m|h) (default 0s)")
	flags.BoolVar(&copts.noHealthcheck, "no-healthcheck", false, "Disable any container-specified HEALTHCHECK")

	// Resource management
	flags.Uint16Var(&copts.blkioWeight, "blkio-weight", 0, "Block IO (relative weight), between 10 and 1000, or 0 to disable (default 0)")
	flags.Var(&copts.blkioWeightDevice, "blkio-weight-device", "Block IO weight (relative device weight)")
	flags.StringVar(&copts.containerIDFile, "cidfile", "", "Write the container ID to the file")
	flags.StringVar(&copts.cpusetCpus, "cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	flags.StringVar(&copts.cpusetMems, "cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	flags.Int64Var(&copts.cpuCount, "cpu-count", 0, "CPU count (Windows only)")
	flags.SetAnnotation("cpu-count", "ostype", []string{"windows"})
	flags.Int64Var(&copts.cpuPercent, "cpu-percent", 0, "CPU percent (Windows only)")
	flags.SetAnnotation("cpu-percent", "ostype", []string{"windows"})
	flags.Int64Var(&copts.cpuPeriod, "cpu-period", 0, "Limit CPU CFS (Completely Fair Scheduler) period")
	flags.Int64Var(&copts.cpuQuota, "cpu-quota", 0, "Limit CPU CFS (Completely Fair Scheduler) quota")
	flags.Int64Var(&copts.cpuRealtimePeriod, "cpu-rt-period", 0, "Limit CPU real-time period in microseconds")
	flags.SetAnnotation("cpu-rt-period", "version", []string{"1.25"})
	flags.Int64Var(&copts.cpuRealtimeRuntime, "cpu-rt-runtime", 0, "Limit CPU real-time runtime in microseconds")
	flags.SetAnnotation("cpu-rt-runtime", "version", []string{"1.25"})
	flags.Int64VarP(&copts.cpuShares, "cpu-shares", "c", 0, "CPU shares (relative weight)")
	flags.Var(&copts.cpus, "cpus", "Number of CPUs")
	flags.SetAnnotation("cpus", "version", []string{"1.25"})
	flags.Var(&copts.deviceReadBps, "device-read-bps", "Limit read rate (bytes per second) from a device")
	flags.Var(&copts.deviceReadIOps, "device-read-iops", "Limit read rate (IO per second) from a device")
	flags.Var(&copts.deviceWriteBps, "device-write-bps", "Limit write rate (bytes per second) to a device")
	flags.Var(&copts.deviceWriteIOps, "device-write-iops", "Limit write rate (IO per second) to a device")
	flags.Var(&copts.ioMaxBandwidth, "io-maxbandwidth", "Maximum IO bandwidth limit for the system drive (Windows only)")
	flags.SetAnnotation("io-maxbandwidth", "ostype", []string{"windows"})
	flags.Uint64Var(&copts.ioMaxIOps, "io-maxiops", 0, "Maximum IOps limit for the system drive (Windows only)")
	flags.SetAnnotation("io-maxiops", "ostype", []string{"windows"})
	flags.Var(&copts.kernelMemory, "kernel-memory", "Kernel memory limit")
	flags.VarP(&copts.memory, "memory", "m", "Memory limit")
	flags.Var(&copts.memoryReservation, "memory-reservation", "Memory soft limit")
	flags.Var(&copts.memorySwap, "memory-swap", "Swap limit equal to memory plus swap: '-1' to enable unlimited swap")
	flags.Int64Var(&copts.swappiness, "memory-swappiness", -1, "Tune container memory swappiness (0 to 100)")
	flags.BoolVar(&copts.oomKillDisable, "oom-kill-disable", false, "Disable OOM Killer")
	flags.IntVar(&copts.oomScoreAdj, "oom-score-adj", 0, "Tune host's OOM preferences (-1000 to 1000)")
	flags.Int64Var(&copts.pidsLimit, "pids-limit", 0, "Tune container pids limit (set -1 for unlimited)")

	// Low-level execution (cgroups, namespaces, ...)
	flags.StringVar(&copts.cgroupParent, "cgroup-parent", "", "Optional parent cgroup for the container")
	flags.StringVar(&copts.ipcMode, "ipc", "", "IPC namespace to use")
	flags.StringVar(&copts.isolation, "isolation", "", "Container isolation technology")
	flags.StringVar(&copts.pidMode, "pid", "", "PID namespace to use")
	flags.Var(&copts.shmSize, "shm-size", "Size of /dev/shm")
	flags.StringVar(&copts.utsMode, "uts", "", "UTS namespace to use")
	flags.StringVar(&copts.runtime, "runtime", "", "Runtime to use for this container")

	flags.BoolVar(&copts.init, "init", false, "Run an init inside the container that forwards signals and reaps processes")
	flags.SetAnnotation("init", "version", []string{"1.25"})
	flags.StringVar(&copts.initPath, "init-path", "", "Path to the docker-init binary")
	flags.SetAnnotation("init-path", "version", []string{"1.25"})

	//
	// HACKED
	//
	flags.Parse(args)
	//
	// HACKED
	//

	return copts
}

// validateAttach validates that the specified string is a valid attach option.
func validateAttach(val string) (string, error) {
	s := strings.ToLower(val)
	for _, str := range []string{"stdin", "stdout", "stderr"} {
		if s == str {
			return s, nil
		}
	}
	return val, fmt.Errorf("valid streams are STDIN, STDOUT and STDERR")
}

// validateDeviceCgroupRule validates a device cgroup rule string format
// It will make sure 'val' is in the form:
//    'type major:minor mode'
func validateDeviceCgroupRule(val string) (string, error) {
	if deviceCgroupRuleRegexp.MatchString(val) {
		return val, nil
	}

	return val, fmt.Errorf("invalid device cgroup format '%s'", val)
}

// validateDevice validates a path for devices
// It will make sure 'val' is in the form:
//    [host-dir:]container-path[:mode]
// It also validates the device mode.
func validateDevice(val string) (string, error) {
	return validatePath(val, validDeviceMode)
}

var (
	deviceCgroupRuleRegexp = regexp.MustCompile("^[acb] ([0-9]+|\\*):([0-9]+|\\*) [rwm]{1,3}$")
)

func validatePath(val string, validator func(string) bool) (string, error) {
	var containerPath string
	var mode string

	if strings.Count(val, ":") > 2 {
		return val, fmt.Errorf("bad format for path: %s", val)
	}

	split := strings.SplitN(val, ":", 3)
	if split[0] == "" {
		return val, fmt.Errorf("bad format for path: %s", val)
	}
	switch len(split) {
	case 1:
		containerPath = split[0]
		val = path.Clean(containerPath)
	case 2:
		if isValid := validator(split[1]); isValid {
			containerPath = split[0]
			mode = split[1]
			val = fmt.Sprintf("%s:%s", path.Clean(containerPath), mode)
		} else {
			containerPath = split[1]
			val = fmt.Sprintf("%s:%s", split[0], path.Clean(containerPath))
		}
	case 3:
		containerPath = split[1]
		mode = split[2]
		if isValid := validator(split[2]); !isValid {
			return val, fmt.Errorf("bad mode specified: %s", mode)
		}
		val = fmt.Sprintf("%s:%s:%s", split[0], containerPath, mode)
	}

	if !path.IsAbs(containerPath) {
		return val, fmt.Errorf("%s is not an absolute path", containerPath)
	}
	return val, nil
}

// validDeviceMode checks if the mode for device is valid or not.
// Valid mode is a composition of r (read), w (write), and m (mknod).
func validDeviceMode(mode string) bool {
	var legalDeviceMode = map[rune]bool{
		'r': true,
		'w': true,
		'm': true,
	}
	if mode == "" {
		return false
	}
	for _, c := range mode {
		if !legalDeviceMode[c] {
			return false
		}
		legalDeviceMode[c] = false
	}
	return true
}

// parse parses the args for the specified command and generates a Config,
// a HostConfig and returns them with the specified command.
// If the specified args are not valid, it will return an error.
func parse(flags *pflag.FlagSet, copts *containerOptions) (*container.Config, *container.HostConfig, *networktypes.NetworkingConfig, error) {
	var (
		attachStdin  = copts.attach.Get("stdin")
		attachStdout = copts.attach.Get("stdout")
		attachStderr = copts.attach.Get("stderr")
	)

	// Validate the input mac address
	if copts.macAddress != "" {
		if _, err := opts.ValidateMACAddress(copts.macAddress); err != nil {
			return nil, nil, nil, fmt.Errorf("%s is not a valid mac address", copts.macAddress)
		}
	}
	if copts.stdin {
		attachStdin = true
	}
	// If -a is not set, attach to stdout and stderr
	if copts.attach.Len() == 0 {
		attachStdout = true
		attachStderr = true
	}

	var err error

	swappiness := copts.swappiness
	if swappiness != -1 && (swappiness < 0 || swappiness > 100) {
		return nil, nil, nil, fmt.Errorf("invalid value: %d. Valid memory swappiness range is 0-100", swappiness)
	}

	var binds []string
	volumes := copts.volumes.GetMap()
	// add any bind targets to the list of container volumes
	for bind := range copts.volumes.GetMap() {
		if arr := volumeSplitN(bind, 2); len(arr) > 1 {
			// after creating the bind mount we want to delete it from the copts.volumes values because
			// we do not want bind mounts being committed to image configs
			binds = append(binds, bind)
			// We should delete from the map (`volumes`) here, as deleting from copts.volumes will not work if
			// there are duplicates entries.
			delete(volumes, bind)
		}
	}

	// Can't evaluate options passed into --tmpfs until we actually mount
	tmpfs := make(map[string]string)
	for _, t := range copts.tmpfs.GetAll() {
		if arr := strings.SplitN(t, ":", 2); len(arr) > 1 {
			tmpfs[arr[0]] = arr[1]
		} else {
			tmpfs[arr[0]] = ""
		}
	}

	var (
		runCmd     strslice.StrSlice
		entrypoint strslice.StrSlice
	)

	if len(copts.Args) > 0 {
		runCmd = strslice.StrSlice(copts.Args)
	}

	if copts.entrypoint != "" {
		entrypoint = strslice.StrSlice{copts.entrypoint}
	} else if flags.Changed("entrypoint") {
		// if `--entrypoint=` is parsed then Entrypoint is reset
		entrypoint = []string{""}
	}

	ports, portBindings, err := nat.ParsePortSpecs(copts.publish.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	// Merge in exposed ports to the map of published ports
	for _, e := range copts.expose.GetAll() {
		if strings.Contains(e, ":") {
			return nil, nil, nil, fmt.Errorf("invalid port format for --expose: %s", e)
		}
		//support two formats for expose, original format <portnum>/[<proto>] or <startport-endport>/[<proto>]
		proto, port := nat.SplitProtoPort(e)
		//parse the start and end port and create a sequence of ports to expose
		//if expose a port, the start and end port are the same
		start, end, err := nat.ParsePortRange(port)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("invalid range format for --expose: %s, error: %s", e, err)
		}
		for i := start; i <= end; i++ {
			p, err := nat.NewPort(proto, strconv.FormatUint(i, 10))
			if err != nil {
				return nil, nil, nil, err
			}
			if _, exists := ports[p]; !exists {
				ports[p] = struct{}{}
			}
		}
	}

	// parse device mappings
	deviceMappings := []container.DeviceMapping{}
	for _, device := range copts.devices.GetAll() {
		deviceMapping, err := parseDevice(device)
		if err != nil {
			return nil, nil, nil, err
		}
		deviceMappings = append(deviceMappings, deviceMapping)
	}

	// collect all the environment variables for the container
	envVariables, err := runconfigopts.ReadKVStrings(copts.envFile.GetAll(), copts.env.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	// collect all the labels for the container
	labels, err := runconfigopts.ReadKVStrings(copts.labelsFile.GetAll(), copts.labels.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	ipcMode := container.IpcMode(copts.ipcMode)
	if !ipcMode.Valid() {
		return nil, nil, nil, fmt.Errorf("--ipc: invalid IPC mode")
	}

	pidMode := container.PidMode(copts.pidMode)
	if !pidMode.Valid() {
		return nil, nil, nil, fmt.Errorf("--pid: invalid PID mode")
	}

	utsMode := container.UTSMode(copts.utsMode)
	if !utsMode.Valid() {
		return nil, nil, nil, fmt.Errorf("--uts: invalid UTS mode")
	}

	usernsMode := container.UsernsMode(copts.usernsMode)
	if !usernsMode.Valid() {
		return nil, nil, nil, fmt.Errorf("--userns: invalid USER mode")
	}

	restartPolicy, err := runconfigopts.ParseRestartPolicy(copts.restartPolicy)
	if err != nil {
		return nil, nil, nil, err
	}

	loggingOpts, err := parseLoggingOpts(copts.loggingDriver, copts.loggingOpts.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	securityOpts, err := parseSecurityOpts(copts.securityOpt.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	storageOpts, err := parseStorageOpts(copts.storageOpt.GetAll())
	if err != nil {
		return nil, nil, nil, err
	}

	// Healthcheck
	var healthConfig *container.HealthConfig
	haveHealthSettings := copts.healthCmd != "" ||
		copts.healthInterval != 0 ||
		copts.healthTimeout != 0 ||
		copts.healthRetries != 0
	if copts.noHealthcheck {
		if haveHealthSettings {
			return nil, nil, nil, fmt.Errorf("--no-healthcheck conflicts with --health-* options")
		}
		test := strslice.StrSlice{"NONE"}
		healthConfig = &container.HealthConfig{Test: test}
	} else if haveHealthSettings {
		var probe strslice.StrSlice
		if copts.healthCmd != "" {
			args := []string{"CMD-SHELL", copts.healthCmd}
			probe = strslice.StrSlice(args)
		}
		if copts.healthInterval < 0 {
			return nil, nil, nil, fmt.Errorf("--health-interval cannot be negative")
		}
		if copts.healthTimeout < 0 {
			return nil, nil, nil, fmt.Errorf("--health-timeout cannot be negative")
		}
		if copts.healthRetries < 0 {
			return nil, nil, nil, fmt.Errorf("--health-retries cannot be negative")
		}

		healthConfig = &container.HealthConfig{
			Test:     probe,
			Interval: copts.healthInterval,
			Timeout:  copts.healthTimeout,
			Retries:  copts.healthRetries,
		}
	}

	resources := container.Resources{
		CgroupParent:         copts.cgroupParent,
		Memory:               copts.memory.Value(),
		MemoryReservation:    copts.memoryReservation.Value(),
		MemorySwap:           copts.memorySwap.Value(),
		MemorySwappiness:     &copts.swappiness,
		KernelMemory:         copts.kernelMemory.Value(),
		OomKillDisable:       &copts.oomKillDisable,
		NanoCPUs:             copts.cpus.Value(),
		CPUCount:             copts.cpuCount,
		CPUPercent:           copts.cpuPercent,
		CPUShares:            copts.cpuShares,
		CPUPeriod:            copts.cpuPeriod,
		CpusetCpus:           copts.cpusetCpus,
		CpusetMems:           copts.cpusetMems,
		CPUQuota:             copts.cpuQuota,
		CPURealtimePeriod:    copts.cpuRealtimePeriod,
		CPURealtimeRuntime:   copts.cpuRealtimeRuntime,
		PidsLimit:            copts.pidsLimit,
		BlkioWeight:          copts.blkioWeight,
		BlkioWeightDevice:    copts.blkioWeightDevice.GetList(),
		BlkioDeviceReadBps:   copts.deviceReadBps.GetList(),
		BlkioDeviceWriteBps:  copts.deviceWriteBps.GetList(),
		BlkioDeviceReadIOps:  copts.deviceReadIOps.GetList(),
		BlkioDeviceWriteIOps: copts.deviceWriteIOps.GetList(),
		IOMaximumIOps:        copts.ioMaxIOps,
		IOMaximumBandwidth:   uint64(copts.ioMaxBandwidth),
		Ulimits:              copts.ulimits.GetList(),
		DeviceCgroupRules:    copts.deviceCgroupRules.GetAll(),
		Devices:              deviceMappings,
	}

	config := &container.Config{
		Hostname:     copts.hostname,
		ExposedPorts: ports,
		User:         copts.user,
		Tty:          copts.tty,
		// TODO: deprecated, it comes from -n, --networking
		// it's still needed internally to set the network to disabled
		// if e.g. bridge is none in daemon opts, and in inspect
		NetworkDisabled: false,
		OpenStdin:       copts.stdin,
		AttachStdin:     attachStdin,
		AttachStdout:    attachStdout,
		AttachStderr:    attachStderr,
		Env:             envVariables,
		Cmd:             runCmd,
		Image:           copts.Image,
		Volumes:         volumes,
		MacAddress:      copts.macAddress,
		Entrypoint:      entrypoint,
		WorkingDir:      copts.workingDir,
		Labels:          runconfigopts.ConvertKVStringsToMap(labels),
		Healthcheck:     healthConfig,
	}
	if flags.Changed("stop-signal") {
		config.StopSignal = copts.stopSignal
	}
	if flags.Changed("stop-timeout") {
		config.StopTimeout = &copts.stopTimeout
	}

	hostConfig := &container.HostConfig{
		Binds:           binds,
		ContainerIDFile: copts.containerIDFile,
		OomScoreAdj:     copts.oomScoreAdj,
		AutoRemove:      copts.autoRemove,
		Privileged:      copts.privileged,
		PortBindings:    portBindings,
		Links:           copts.links.GetAll(),
		PublishAllPorts: copts.publishAll,
		// Make sure the dns fields are never nil.
		// New containers don't ever have those fields nil,
		// but pre created containers can still have those nil values.
		// See https://github.com/docker/docker/pull/17779
		// for a more detailed explanation on why we don't want that.
		DNS:            copts.dns.GetAllOrEmpty(),
		DNSSearch:      copts.dnsSearch.GetAllOrEmpty(),
		DNSOptions:     copts.dnsOptions.GetAllOrEmpty(),
		ExtraHosts:     copts.extraHosts.GetAll(),
		VolumesFrom:    copts.volumesFrom.GetAll(),
		NetworkMode:    container.NetworkMode(copts.netMode),
		IpcMode:        ipcMode,
		PidMode:        pidMode,
		UTSMode:        utsMode,
		UsernsMode:     usernsMode,
		CapAdd:         strslice.StrSlice(copts.capAdd.GetAll()),
		CapDrop:        strslice.StrSlice(copts.capDrop.GetAll()),
		GroupAdd:       copts.groupAdd.GetAll(),
		RestartPolicy:  restartPolicy,
		SecurityOpt:    securityOpts,
		StorageOpt:     storageOpts,
		ReadonlyRootfs: copts.readonlyRootfs,
		LogConfig:      container.LogConfig{Type: copts.loggingDriver, Config: loggingOpts},
		VolumeDriver:   copts.volumeDriver,
		Isolation:      container.Isolation(copts.isolation),
		ShmSize:        copts.shmSize.Value(),
		Resources:      resources,
		Tmpfs:          tmpfs,
		Sysctls:        copts.sysctls.GetAll(),
		Runtime:        copts.runtime,
	}

	if copts.autoRemove && !hostConfig.RestartPolicy.IsNone() {
		return nil, nil, nil, fmt.Errorf("Conflicting options: --restart and --rm")
	}

	// only set this value if the user provided the flag, else it should default to nil
	if flags.Changed("init") {
		hostConfig.Init = &copts.init
	}

	// When allocating stdin in attached mode, close stdin at client disconnect
	if config.OpenStdin && config.AttachStdin {
		config.StdinOnce = true
	}

	networkingConfig := &networktypes.NetworkingConfig{
		EndpointsConfig: make(map[string]*networktypes.EndpointSettings),
	}

	if copts.ipv4Address != "" || copts.ipv6Address != "" || copts.linkLocalIPs.Len() > 0 {
		epConfig := &networktypes.EndpointSettings{}
		networkingConfig.EndpointsConfig[string(hostConfig.NetworkMode)] = epConfig

		epConfig.IPAMConfig = &networktypes.EndpointIPAMConfig{
			IPv4Address: copts.ipv4Address,
			IPv6Address: copts.ipv6Address,
		}

		if copts.linkLocalIPs.Len() > 0 {
			epConfig.IPAMConfig.LinkLocalIPs = make([]string, copts.linkLocalIPs.Len())
			copy(epConfig.IPAMConfig.LinkLocalIPs, copts.linkLocalIPs.GetAll())
		}
	}

	if hostConfig.NetworkMode.IsUserDefined() && len(hostConfig.Links) > 0 {
		epConfig := networkingConfig.EndpointsConfig[string(hostConfig.NetworkMode)]
		if epConfig == nil {
			epConfig = &networktypes.EndpointSettings{}
		}
		epConfig.Links = make([]string, len(hostConfig.Links))
		copy(epConfig.Links, hostConfig.Links)
		networkingConfig.EndpointsConfig[string(hostConfig.NetworkMode)] = epConfig
	}

	if copts.aliases.Len() > 0 {
		epConfig := networkingConfig.EndpointsConfig[string(hostConfig.NetworkMode)]
		if epConfig == nil {
			epConfig = &networktypes.EndpointSettings{}
		}
		epConfig.Aliases = make([]string, copts.aliases.Len())
		copy(epConfig.Aliases, copts.aliases.GetAll())
		networkingConfig.EndpointsConfig[string(hostConfig.NetworkMode)] = epConfig
	}

	return config, hostConfig, networkingConfig, nil
}

// takes a local seccomp daemon, reads the file contents for sending to the daemon
func parseSecurityOpts(securityOpts []string) ([]string, error) {
	for key, opt := range securityOpts {
		con := strings.SplitN(opt, "=", 2)
		if len(con) == 1 && con[0] != "no-new-privileges" {
			if strings.Contains(opt, ":") {
				con = strings.SplitN(opt, ":", 2)
			} else {
				return securityOpts, fmt.Errorf("Invalid --security-opt: %q", opt)
			}
		}
		if con[0] == "seccomp" && con[1] != "unconfined" {
			f, err := ioutil.ReadFile(con[1])
			if err != nil {
				return securityOpts, fmt.Errorf("opening seccomp profile (%s) failed: %v", con[1], err)
			}
			b := bytes.NewBuffer(nil)
			if err := json.Compact(b, f); err != nil {
				return securityOpts, fmt.Errorf("compacting json for seccomp profile (%s) failed: %v", con[1], err)
			}
			securityOpts[key] = fmt.Sprintf("seccomp=%s", b.Bytes())
		}
	}

	return securityOpts, nil
}

// parses storage options per container into a map
func parseStorageOpts(storageOpts []string) (map[string]string, error) {
	m := make(map[string]string)
	for _, option := range storageOpts {
		if strings.Contains(option, "=") {
			opt := strings.SplitN(option, "=", 2)
			m[opt[0]] = opt[1]
		} else {
			return nil, fmt.Errorf("invalid storage option")
		}
	}
	return m, nil
}

// reportError is a utility method that prints a user-friendly message
// containing the error that occurred during parsing and a suggestion to get help
func reportError(stderr io.Writer, name string, str string, withHelp bool) {
	str = strings.TrimSuffix(str, ".") + "."
	if withHelp {
		str += "\nSee '" + os.Args[0] + " " + name + " --help'."
	}
	fmt.Fprintf(stderr, "%s: %s\n", os.Args[0], str)
}

func parseLoggingOpts(loggingDriver string, loggingOpts []string) (map[string]string, error) {
	loggingOptsMap := runconfigopts.ConvertKVStringsToMap(loggingOpts)
	if loggingDriver == "none" && len(loggingOpts) > 0 {
		return map[string]string{}, fmt.Errorf("invalid logging opts for driver %s", loggingDriver)
	}
	return loggingOptsMap, nil
}

// volumeSplitN splits raw into a maximum of n parts, separated by a separator colon.
// A separator colon is the last `:` character in the regex `[:\\]?[a-zA-Z]:` (note `\\` is `\` escaped).
// In Windows driver letter appears in two situations:
// a. `^[a-zA-Z]:` (A colon followed  by `^[a-zA-Z]:` is OK as colon is the separator in volume option)
// b. A string in the format like `\\?\C:\Windows\...` (UNC).
// Therefore, a driver letter can only follow either a `:` or `\\`
// This allows to correctly split strings such as `C:\foo:D:\:rw` or `/tmp/q:/foo`.
func volumeSplitN(raw string, n int) []string {
	var array []string
	if len(raw) == 0 || raw[0] == ':' {
		// invalid
		return nil
	}
	// numberOfParts counts the number of parts separated by a separator colon
	numberOfParts := 0
	// left represents the left-most cursor in raw, updated at every `:` character considered as a separator.
	left := 0
	// right represents the right-most cursor in raw incremented with the loop. Note this
	// starts at index 1 as index 0 is already handle above as a special case.
	for right := 1; right < len(raw); right++ {
		// stop parsing if reached maximum number of parts
		if n >= 0 && numberOfParts >= n {
			break
		}
		if raw[right] != ':' {
			continue
		}
		potentialDriveLetter := raw[right-1]
		if (potentialDriveLetter >= 'A' && potentialDriveLetter <= 'Z') || (potentialDriveLetter >= 'a' && potentialDriveLetter <= 'z') {
			if right > 1 {
				beforePotentialDriveLetter := raw[right-2]
				// Only `:` or `\\` are checked (`/` could fall into the case of `/tmp/q:/foo`)
				if beforePotentialDriveLetter != ':' && beforePotentialDriveLetter != '\\' {
					// e.g. `C:` is not preceded by any delimiter, therefore it was not a drive letter but a path ending with `C:`.
					array = append(array, raw[left:right])
					left = right + 1
					numberOfParts++
				}
				// else, `C:` is considered as a drive letter and not as a delimiter, so we continue parsing.
			}
			// if right == 1, then `C:` is the beginning of the raw string, therefore `:` is again not considered a delimiter and we continue parsing.
		} else {
			// if `:` is not preceded by a potential drive letter, then consider it as a delimiter.
			array = append(array, raw[left:right])
			left = right + 1
			numberOfParts++
		}
	}
	// need to take care of the last part
	if left < len(raw) {
		if n >= 0 && numberOfParts >= n {
			// if the maximum number of parts is reached, just append the rest to the last part
			// left-1 is at the last `:` that needs to be included since not considered a separator.
			array[n-1] += raw[left-1:]
		} else {
			array = append(array, raw[left:])
		}
	}
	return array
}

// parseDevice parses a device mapping string to a container.DeviceMapping struct
func parseDevice(device string) (container.DeviceMapping, error) {
	src := ""
	dst := ""
	permissions := "rwm"
	arr := strings.Split(device, ":")
	switch len(arr) {
	case 3:
		permissions = arr[2]
		fallthrough
	case 2:
		if validDeviceMode(arr[1]) {
			permissions = arr[1]
		} else {
			dst = arr[1]
		}
		fallthrough
	case 1:
		src = arr[0]
	default:
		return container.DeviceMapping{}, fmt.Errorf("invalid device specification: %s", device)
	}

	if dst == "" {
		dst = src
	}

	deviceMapping := container.DeviceMapping{
		PathOnHost:        src,
		PathInContainer:   dst,
		CgroupPermissions: permissions,
	}
	return deviceMapping, nil
}

type cidFile struct {
	path    string
	file    *os.File
	written bool
}

func (cid *cidFile) Close() error {
	cid.file.Close()

	if cid.written {
		return nil
	}
	if err := os.Remove(cid.path); err != nil {
		return fmt.Errorf("failed to remove the CID file '%s': %s \n", cid.path, err)
	}

	return nil
}

func (cid *cidFile) Write(id string) error {
	if _, err := cid.file.Write([]byte(id)); err != nil {
		return fmt.Errorf("Failed to write the container ID to the file: %s", err)
	}
	cid.written = true
	return nil
}

func newCIDFile(path string) (*cidFile, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, fmt.Errorf("Container ID file found, make sure the other container isn't running or delete %s", path)
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("Failed to create the container ID file: %s", err)
	}

	return &cidFile{path: path, file: f}, nil
}

func createContainer(ctx context.Context, dockerCli *command.DockerCli, config *container.Config, hostConfig *container.HostConfig, networkingConfig *networktypes.NetworkingConfig, cidfile, name string) (*container.ContainerCreateCreatedBody, error) {
	stderr := dockerCli.Err()

	// Add label to identify project if needed.
	// Check whether we are in the context of a Docker project.
	proj, pErr := project.GetForWd()
	if pErr != nil {
		return nil, pErr
	}
	if proj != nil {
		config.Labels["docker.project.id:"+proj.Config.ID] = ""
		config.Labels["docker.project.name:"+proj.Config.Name] = ""
	}

	var (
		containerIDFile *cidFile
		trustedRef      reference.Canonical
		namedRef        reference.Named
	)

	if cidfile != "" {
		var err error
		if containerIDFile, err = newCIDFile(cidfile); err != nil {
			return nil, err
		}
		defer containerIDFile.Close()
	}

	ref, err := reference.ParseAnyReference(config.Image)
	if err != nil {
		return nil, err
	}
	if named, ok := ref.(reference.Named); ok {
		namedRef = reference.TagNameOnly(named)

		if taggedRef, ok := namedRef.(reference.NamedTagged); ok && command.IsTrusted() {
			var err error
			trustedRef, err = image.TrustedReference(ctx, dockerCli, taggedRef, nil)
			if err != nil {
				return nil, err
			}
			config.Image = reference.FamiliarString(trustedRef)
		}
	}

	//create the container
	response, err := dockerCli.Client().ContainerCreate(ctx, config, hostConfig, networkingConfig, name)

	//if image not found try to pull it
	if err != nil {
		if apiclient.IsErrImageNotFound(err) && namedRef != nil {
			fmt.Fprintf(stderr, "Unable to find image '%s' locally\n", reference.FamiliarString(namedRef))

			// we don't want to write to stdout anything apart from container.ID
			if err = pullImage(ctx, dockerCli, config.Image, stderr); err != nil {
				return nil, err
			}
			if taggedRef, ok := namedRef.(reference.NamedTagged); ok && trustedRef != nil {
				if err := image.TagTrusted(ctx, dockerCli, trustedRef, taggedRef); err != nil {
					return nil, err
				}
			}
			// Retry
			var retryErr error
			response, retryErr = dockerCli.Client().ContainerCreate(ctx, config, hostConfig, networkingConfig, name)
			if retryErr != nil {
				return nil, retryErr
			}
		} else {
			return nil, err
		}
	}

	for _, warning := range response.Warnings {
		fmt.Fprintf(stderr, "WARNING: %s\n", warning)
	}
	if containerIDFile != nil {
		if err = containerIDFile.Write(response.ID); err != nil {
			return nil, err
		}
	}
	return &response, nil
}

func setRawTerminal(streams command.Streams) error {
	if err := streams.In().SetRawTerminal(); err != nil {
		return err
	}
	return streams.Out().SetRawTerminal()
}

func waitExitOrRemoved(ctx context.Context, dockerCli *command.DockerCli, containerID string, waitRemove bool) chan int {
	if len(containerID) == 0 {
		// containerID can never be empty
		panic("Internal Error: waitExitOrRemoved needs a containerID as parameter")
	}

	var removeErr error
	statusChan := make(chan int)
	exitCode := 125

	// Get events via Events API
	f := filters.NewArgs()
	f.Add("type", "container")
	f.Add("container", containerID)
	options := types.EventsOptions{
		Filters: f,
	}
	eventCtx, cancel := context.WithCancel(ctx)
	eventq, errq := dockerCli.Client().Events(eventCtx, options)

	eventProcessor := func(e events.Message) bool {
		stopProcessing := false
		switch e.Status {
		case "die":
			if v, ok := e.Actor.Attributes["exitCode"]; ok {
				code, cerr := strconv.Atoi(v)
				if cerr != nil {
					logrus.Errorf("failed to convert exitcode '%q' to int: %v", v, cerr)
				} else {
					exitCode = code
				}
			}
			if !waitRemove {
				stopProcessing = true
			} else {
				// If we are talking to an older daemon, `AutoRemove` is not supported.
				// We need to fall back to the old behavior, which is client-side removal
				if versions.LessThan(dockerCli.Client().ClientVersion(), "1.25") {
					go func() {
						removeErr = dockerCli.Client().ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{RemoveVolumes: true})
						if removeErr != nil {
							logrus.Errorf("error removing container: %v", removeErr)
							cancel() // cancel the event Q
						}
					}()
				}
			}
		case "detach":
			exitCode = 0
			stopProcessing = true
		case "destroy":
			stopProcessing = true
		}
		return stopProcessing
	}

	go func() {
		defer func() {
			statusChan <- exitCode // must always send an exit code or the caller will block
			cancel()
		}()

		for {
			select {
			case <-eventCtx.Done():
				if removeErr != nil {
					return
				}
			case evt := <-eventq:
				if eventProcessor(evt) {
					return
				}
			case err := <-errq:
				logrus.Errorf("error getting events from daemon: %v", err)
				return
			}
		}
	}()

	return statusChan
}

func restoreTerminal(streams command.Streams, in io.Closer) error {
	streams.In().RestoreTerminal()
	streams.Out().RestoreTerminal()
	// WARNING: DO NOT REMOVE THE OS CHECK !!!
	// For some reason this Close call blocks on darwin..
	// As the client exists right after, simply discard the close
	// until we find a better solution.
	if in != nil && runtime.GOOS != "darwin" {
		return in.Close()
	}
	return nil
}

// holdHijackedConnection handles copying input to and output from streams to the
// connection
func holdHijackedConnection(ctx context.Context, streams command.Streams, tty bool, inputStream io.ReadCloser, outputStream, errorStream io.Writer, resp types.HijackedResponse) error {
	var (
		err         error
		restoreOnce sync.Once
	)
	if inputStream != nil && tty {
		if err := setRawTerminal(streams); err != nil {
			return err
		}
		defer func() {
			restoreOnce.Do(func() {
				restoreTerminal(streams, inputStream)
			})
		}()
	}

	receiveStdout := make(chan error, 1)
	if outputStream != nil || errorStream != nil {
		go func() {
			// When TTY is ON, use regular copy
			if tty && outputStream != nil {
				_, err = io.Copy(outputStream, resp.Reader)
				// we should restore the terminal as soon as possible once connection end
				// so any following print messages will be in normal type.
				if inputStream != nil {
					restoreOnce.Do(func() {
						restoreTerminal(streams, inputStream)
					})
				}
			} else {
				_, err = stdcopy.StdCopy(outputStream, errorStream, resp.Reader)
			}

			logrus.Debug("[hijack] End of stdout")
			receiveStdout <- err
		}()
	}

	stdinDone := make(chan struct{})
	go func() {
		if inputStream != nil {
			io.Copy(resp.Conn, inputStream)
			// we should restore the terminal as soon as possible once connection end
			// so any following print messages will be in normal type.
			if tty {
				restoreOnce.Do(func() {
					restoreTerminal(streams, inputStream)
				})
			}
			logrus.Debug("[hijack] End of stdin")
		}

		if err := resp.CloseWrite(); err != nil {
			logrus.Debugf("Couldn't send EOF: %s", err)
		}
		close(stdinDone)
	}()

	select {
	case err := <-receiveStdout:
		if err != nil {
			logrus.Debugf("Error receiveStdout: %s", err)
			return err
		}
	case <-stdinDone:
		if outputStream != nil || errorStream != nil {
			select {
			case err := <-receiveStdout:
				if err != nil {
					logrus.Debugf("Error receiveStdout: %s", err)
					return err
				}
			case <-ctx.Done():
			}
		}
	case <-ctx.Done():
	}

	return nil
}

// if container start fails with 'not found'/'no such' error, return 127
// if container start fails with 'permission denied' error, return 126
// return 125 for generic docker daemon failures
func runStartContainerErr(err error) error {
	trimmedErr := strings.TrimPrefix(err.Error(), "Error response from daemon: ")
	statusError := cli.StatusError{StatusCode: 125}
	if strings.Contains(trimmedErr, "executable file not found") ||
		strings.Contains(trimmedErr, "no such file or directory") ||
		strings.Contains(trimmedErr, "system cannot find the file specified") {
		statusError = cli.StatusError{StatusCode: 127}
	} else if strings.Contains(trimmedErr, syscall.EACCES.Error()) {
		statusError = cli.StatusError{StatusCode: 126}
	}

	return statusError
}

func pullImage(ctx context.Context, dockerCli *command.DockerCli, image string, out io.Writer) error {
	ref, err := reference.ParseNormalizedNamed(image)
	if err != nil {
		return err
	}

	// Resolve the Repository name from fqn to RepositoryInfo
	repoInfo, err := registry.ParseRepositoryInfo(ref)
	if err != nil {
		return err
	}

	authConfig := command.ResolveAuthConfig(ctx, dockerCli, repoInfo.Index)
	encodedAuth, err := command.EncodeAuthToBase64(authConfig)
	if err != nil {
		return err
	}

	options := types.ImageCreateOptions{
		RegistryAuth: encodedAuth,
	}

	responseBody, err := dockerCli.Client().ImageCreate(ctx, image, options)
	if err != nil {
		return err
	}
	defer responseBody.Close()

	return jsonmessage.DisplayJSONMessagesStream(
		responseBody,
		out,
		dockerCli.Out().FD(),
		dockerCli.Out().IsTerminal(),
		nil)
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

// REQUIRED BY dockerImageBuild

type buildOptions struct {
	context        string
	dockerfileName string
	tags           opts.ListOpts
	labels         opts.ListOpts
	buildArgs      opts.ListOpts
	extraHosts     opts.ListOpts
	ulimits        *opts.UlimitOpt
	memory         opts.MemBytes
	memorySwap     opts.MemSwapBytes
	shmSize        opts.MemBytes
	cpuShares      int64
	cpuPeriod      int64
	cpuQuota       int64
	cpuSetCpus     string
	cpuSetMems     string
	cgroupParent   string
	isolation      string
	quiet          bool
	noCache        bool
	rm             bool
	forceRm        bool
	pull           bool
	cacheFrom      []string
	compress       bool
	securityOpt    []string
	networkMode    string
	squash         bool
}

// validateTag checks if the given image name can be resolved.
func validateTag(rawRepo string) (string, error) {
	_, err := reference.ParseNormalizedNamed(rawRepo)
	if err != nil {
		return "", err
	}

	return rawRepo, nil
}

func isLocalDir(c string) bool {
	_, err := os.Stat(c)
	return err == nil
}

// resolvedTag records the repository, tag, and resolved digest reference
// from a Dockerfile rewrite.
type resolvedTag struct {
	digestRef reference.Canonical
	tagRef    reference.NamedTagged
}

type translatorFunc func(context.Context, reference.NamedTagged) (reference.Canonical, error)

// replaceDockerfileTarWrapper wraps the given input tar archive stream and
// replaces the entry with the given Dockerfile name with the contents of the
// new Dockerfile. Returns a new tar archive stream with the replaced
// Dockerfile.
func replaceDockerfileTarWrapper(ctx context.Context, inputTarStream io.ReadCloser, dockerfileName string, translator translatorFunc, resolvedTags *[]*resolvedTag) io.ReadCloser {
	pipeReader, pipeWriter := io.Pipe()
	go func() {
		tarReader := tar.NewReader(inputTarStream)
		tarWriter := tar.NewWriter(pipeWriter)

		defer inputTarStream.Close()

		for {
			hdr, err := tarReader.Next()
			if err == io.EOF {
				// Signals end of archive.
				tarWriter.Close()
				pipeWriter.Close()
				return
			}
			if err != nil {
				pipeWriter.CloseWithError(err)
				return
			}

			content := io.Reader(tarReader)
			if hdr.Name == dockerfileName {
				// This entry is the Dockerfile. Since the tar archive was
				// generated from a directory on the local filesystem, the
				// Dockerfile will only appear once in the archive.
				var newDockerfile []byte
				newDockerfile, *resolvedTags, err = rewriteDockerfileFrom(ctx, content, translator)
				if err != nil {
					pipeWriter.CloseWithError(err)
					return
				}
				hdr.Size = int64(len(newDockerfile))
				content = bytes.NewBuffer(newDockerfile)
			}

			if err := tarWriter.WriteHeader(hdr); err != nil {
				pipeWriter.CloseWithError(err)
				return
			}

			if _, err := io.Copy(tarWriter, content); err != nil {
				pipeWriter.CloseWithError(err)
				return
			}
		}
	}()

	return pipeReader
}

var dockerfileFromLinePattern = regexp.MustCompile(`(?i)^[\s]*FROM[ \f\r\t\v]+(?P<image>[^ \f\r\t\v\n#]+)`)

// rewriteDockerfileFrom rewrites the given Dockerfile by resolving images in
// "FROM <image>" instructions to a digest reference. `translator` is a
// function that takes a repository name and tag reference and returns a
// trusted digest reference.
func rewriteDockerfileFrom(ctx context.Context, dockerfile io.Reader, translator translatorFunc) (newDockerfile []byte, resolvedTags []*resolvedTag, err error) {
	scanner := bufio.NewScanner(dockerfile)
	buf := bytes.NewBuffer(nil)

	// Scan the lines of the Dockerfile, looking for a "FROM" line.
	for scanner.Scan() {
		line := scanner.Text()

		matches := dockerfileFromLinePattern.FindStringSubmatch(line)
		if matches != nil && matches[1] != api.NoBaseImageSpecifier {
			// Replace the line with a resolved "FROM repo@digest"
			var ref reference.Named
			ref, err = reference.ParseNormalizedNamed(matches[1])
			if err != nil {
				return nil, nil, err
			}
			ref = reference.TagNameOnly(ref)
			if ref, ok := ref.(reference.NamedTagged); ok && command.IsTrusted() {
				trustedRef, err := translator(ctx, ref)
				if err != nil {
					return nil, nil, err
				}

				line = dockerfileFromLinePattern.ReplaceAllLiteralString(line, fmt.Sprintf("FROM %s", reference.FamiliarString(trustedRef)))
				resolvedTags = append(resolvedTags, &resolvedTag{
					digestRef: trustedRef,
					tagRef:    ref,
				})
			}
		}

		_, err := fmt.Fprintln(buf, line)
		if err != nil {
			return nil, nil, err
		}
	}

	return buf.Bytes(), resolvedTags, scanner.Err()
}

// lastProgressOutput is the same as progress.Output except
// that it only output with the last update. It is used in
// non terminal scenarios to suppress verbose messages
type lastProgressOutput struct {
	output progress.Output
}

// WriteProgress formats progress information from a ProgressReader.
func (out *lastProgressOutput) WriteProgress(prog progress.Progress) error {
	if !prog.LastUpdate {
		return nil
	}

	return out.output.WriteProgress(prog)
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
