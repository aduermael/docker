package sandbox

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cli/command"
	cliflags "github.com/docker/docker/cli/flags"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/term"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerCmd executes the docker command passed as argument.
func (s *Sandbox) dockerCmd(L *lua.LState) int {
	var err error

	dockerCli := newDockerCli()
	cmd := newDockerCommand(dockerCli)

	// retrieve parameter
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("string parameter not found - func(\"string\"")
		return 0
	}

	args, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	cmd.SetArgs(args)
	err = cmd.Execute()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	return 0
}

// dockerSilentCmd executes the docker command passed as argument
// and returns output and error streams as Lua strings
// if there's no error, only output is returned (err will be nil)
// example: local out, err = dockerSilentCmd('run myimage')
func (s *Sandbox) dockerSilentCmd(L *lua.LState) int {
	var err error

	// retrieve parameter
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("string parameter not found - func(\"string\"")
		return 0
	}

	args, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	stdin, _, _ := term.StdStreams()
	outbuf := new(bytes.Buffer)
	errbuf := new(bytes.Buffer)
	dockerCli := command.NewDockerCli(stdin, outbuf, errbuf)
	dockerCli.Initialize(cliflags.NewClientOptions())
	cmd := newDockerCommand(dockerCli)

	cmd.SetArgs(args)
	err = cmd.Execute()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	outStr := strings.TrimSpace(outbuf.String())
	s.luaState.Push(lua.LString(outStr))
	errStr := strings.TrimSpace(errbuf.String())
	if errStr != "" {
		s.luaState.Push(lua.LString(errStr))
		return 2
	}
	return 1
}

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

// dockerImageList lists Docker images and returns a Lua table (array)
// containing the images' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker image ls` command.
// docker.image.list(arguments string)
func (s *Sandbox) dockerImageList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		// argsStr's default value is an empty string
		argsStr = ""
	}

	// convert string of arguments into an array of arguments
	argsArr, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// parse flags
	opts := imagesOptions{filter: opts.NewFilterOpt()}
	flags := pflag.NewFlagSet("dockerImageList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only show numeric IDs")
	flags.BoolVarP(&opts.all, "all", "a", false, "Show all images (default hides intermediate images)")
	flags.BoolVar(&opts.noTrunc, "no-trunc", false, "Don't truncate output")
	flags.BoolVar(&opts.showDigests, "digests", false, "Show digests")
	flags.StringVar(&opts.format, "format", "", "Pretty-print images using a Go template")
	flags.VarP(&opts.filter, "filter", "f", "Filter output based on conditions provided")
	flags.Parse(argsArr)

	// contact docker API
	ctx := context.Background()

	filters := opts.filter.Value()
	if opts.matchName != "" {
		filters.Add("reference", opts.matchName)
	}

	options := types.ImageListOptions{
		All:     opts.all,
		Filters: filters,
	}

	dockerCli := newDockerCli()
	images, err := dockerCli.Client().ImageList(ctx, options)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// Lua table containing all images
	imagesLuaTable := s.luaState.CreateTable(0, 0)

	// loop over the images
	for _, image := range images {
		// Lua table containing one image
		imageLuaTable := s.luaState.CreateTable(0, 0)

		// image id
		// removing prefixes like in image ids like:
		// sha256:5dae07823d481dab69d6a278b4014cb2978b96ef0874ac18fd2ad050a2a32699
		imageID := image.ID
		parts := strings.SplitN(imageID, ":", 2)
		if len(parts) > 1 {
			imageID = parts[1]
		}

		imageLuaTable.RawSetString("id", lua.LString(imageID))
		imageLuaTable.RawSetString("parentId", lua.LString(image.ParentID))
		imageLuaTable.RawSetString("created", lua.LNumber(float64(image.Created)))
		imageLuaTable.RawSetString("sharedSize", lua.LNumber(float64(image.SharedSize)))
		imageLuaTable.RawSetString("size", lua.LNumber(float64(image.Size)))
		imageLuaTable.RawSetString("virtualSize", lua.LNumber(float64(image.VirtualSize)))
		// add RepoTags
		repoTags := s.luaState.CreateTable(0, 0)
		for _, repoTag := range image.RepoTags {
			repoTags.Append(lua.LString(repoTag))
		}
		imageLuaTable.RawSetString("repoTags", repoTags)

		// add this image's Lua table to the Lua table containing all images
		imagesLuaTable.Append(imageLuaTable)
	}

	s.luaState.Push(imagesLuaTable)
	return 1
}

// dockerVolumeList lists Docker volumes and returns a Lua table (array)
// containing the volumes' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker volume ls` command.
// docker.volume.list(arguments string)
func (s *Sandbox) dockerVolumeList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		// argsStr's default value is an empty string
		argsStr = ""
	}

	// convert string of arguments into an array of arguments
	argsArr, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// parse flags
	opts := volumeListOptions{filter: opts.NewFilterOpt()}
	flags := pflag.NewFlagSet("dockerVolumeList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only display volume names")
	flags.StringVar(&opts.format, "format", "", "Pretty-print volumes using a Go template")
	flags.VarP(&opts.filter, "filter", "f", "Provide filter values (e.g. 'dangling=true')")
	flags.Parse(argsArr)

	dockerCli := newDockerCli()
	volumes, err := dockerCli.Client().VolumeList(context.Background(), opts.filter.Value())
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// Lua table containing all volumes
	volumesLuaTable := s.luaState.CreateTable(0, 0)

	// loop over the images
	for _, volume := range volumes.Volumes {
		if volume == nil {
			continue
		}
		// Lua table containing one volume
		volumeLuaTable := s.luaState.CreateTable(0, 0)
		// volume driver
		volumeLuaTable.RawSetString("driver", lua.LString(volume.Driver))
		// volume labels
		volumeLabels := s.luaState.CreateTable(0, 0)
		for key, value := range volume.Labels {
			volumeLabels.RawSetString(key, lua.LString(value))
		}
		volumeLuaTable.RawSetString("labels", volumeLabels)
		// volume mount point
		volumeLuaTable.RawSetString("mountPoint", lua.LString(volume.Mountpoint))
		// volume name
		volumeLuaTable.RawSetString("name", lua.LString(volume.Name))
		// volume options
		volumeOptions := s.luaState.CreateTable(0, 0)
		for _, volumeOption := range volume.Options {
			volumeOptions.Append(lua.LString(volumeOption))
		}
		volumeLuaTable.RawSetString("options", volumeOptions)
		// volume scope
		volumeLuaTable.RawSetString("scope", lua.LString(volume.Scope))

		// add this volume's Lua table to the Lua table containing all volumes
		volumesLuaTable.Append(volumeLuaTable)
	}

	s.luaState.Push(volumesLuaTable)
	return 1
}

// dockerNetworkList lists Docker networks and returns a Lua table (array)
// containing the networks' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker network ls` command.
// docker.network.list(arguments string)
func (s *Sandbox) dockerNetworkList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		// argsStr's default value is an empty string
		argsStr = ""
	}

	// convert string of arguments into an array of arguments
	argsArr, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// parse flags
	opts := networkListOptions{filter: opts.NewFilterOpt()}
	flags := pflag.NewFlagSet("dockerNetworkList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only display network IDs")
	flags.BoolVar(&opts.noTrunc, "no-trunc", false, "Do not truncate the output")
	flags.StringVar(&opts.format, "format", "", "Pretty-print networks using a Go template")
	flags.VarP(&opts.filter, "filter", "f", "Provide filter values (e.g. 'driver=bridge')")
	flags.Parse(argsArr)

	dockerCli := newDockerCli()
	options := types.NetworkListOptions{Filters: opts.filter.Value()}
	networks, err := dockerCli.Client().NetworkList(context.Background(), options)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// Lua table containing all networks
	networksLuaTable := s.luaState.CreateTable(0, 0)

	// loop over the networks
	for _, network := range networks {
		// Lua table containing one network
		networkLuaTable := s.luaState.CreateTable(0, 0)
		// network name
		networkLuaTable.RawSetString("name", lua.LString(network.Name))
		// network id
		networkLuaTable.RawSetString("id", lua.LString(network.ID))
		// network created
		networkLuaTable.RawSetString("created", lua.LNumber(network.Created.Unix()))
		// network scope
		networkLuaTable.RawSetString("scope", lua.LString(network.Scope))
		// network driver
		networkLuaTable.RawSetString("driver", lua.LString(network.Driver))
		// network EnableIPv6
		networkLuaTable.RawSetString("enableIPv6", lua.LBool(network.EnableIPv6))
		// network internal
		networkLuaTable.RawSetString("internal", lua.LBool(network.Internal))
		// network attachable
		networkLuaTable.RawSetString("attachable", lua.LBool(network.Attachable))
		// network options
		networkOptions := s.luaState.CreateTable(0, 0)
		for _, networkOption := range network.Options {
			networkOptions.Append(lua.LString(networkOption))
		}
		networkLuaTable.RawSetString("options", networkOptions)
		// network labels
		networkLabels := s.luaState.CreateTable(0, 0)
		for key, value := range network.Labels {
			networkLabels.RawSetString(key, lua.LString(value))
		}
		networkLuaTable.RawSetString("labels", networkLabels)
		// add this image's Lua table to the Lua table containing all images
		networksLuaTable.Append(networkLuaTable)
	}

	s.luaState.Push(networksLuaTable)
	return 1
}

// dockerServiceList lists Docker services and returns a Lua table (array)
// containing the services' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker service ls` command.
// docker.service.list(arguments string)
func (s *Sandbox) dockerServiceList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		// argsStr's default value is an empty string
		argsStr = ""
	}

	// convert string of arguments into an array of arguments
	argsArr, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// parse flags
	opts := listServiceOptions{filter: opts.NewFilterOpt()}
	flags := pflag.NewFlagSet("dockerServiceList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only display IDs")
	flags.StringVar(&opts.format, "format", "", "Pretty-print services using a Go template")
	flags.VarP(&opts.filter, "filter", "f", "Filter output based on conditions provided")
	flags.Parse(argsArr)

	dockerCli := newDockerCli()
	options := types.ServiceListOptions{Filters: opts.filter.Value()}
	services, err := dockerCli.Client().ServiceList(context.Background(), options)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// Lua table containing all networks
	servicesLuaTable := s.luaState.CreateTable(0, 0)

	// loop over the networks
	for _, service := range services {
		// Lua table containing one network
		serviceLuaTable := s.luaState.CreateTable(0, 0)

		// service id
		serviceLuaTable.RawSetString("id", lua.LString(service.ID))
		// service version
		serviceLuaTable.RawSetString("version", lua.LNumber(service.Meta.Version.Index))
		// created
		serviceLuaTable.RawSetString("created", lua.LNumber(service.Meta.CreatedAt.Unix()))
		// updated
		serviceLuaTable.RawSetString("updated", lua.LNumber(service.Meta.UpdatedAt.Unix()))
		// name
		serviceLuaTable.RawSetString("name", lua.LString(service.Spec.Annotations.Name))
		// labels
		serviceLabels := s.luaState.CreateTable(0, 0)
		for key, value := range service.Spec.Annotations.Labels {
			serviceLabels.RawSetString(key, lua.LString(value))
		}
		serviceLuaTable.RawSetString("labels", serviceLabels)
		// image
		serviceLuaTable.RawSetString("image", lua.LString(service.Spec.TaskTemplate.ContainerSpec.Image))
		// add this image's Lua table to the Lua table containing all images
		servicesLuaTable.Append(serviceLuaTable)
	}

	s.luaState.Push(servicesLuaTable)
	return 1
}

// dockerSecretList lists Docker secrets and returns a Lua table (array)
// containing the secrets' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker secret ls` command.
// docker.secret.list(arguments string)
func (s *Sandbox) dockerSecretList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		// argsStr's default value is an empty string
		argsStr = ""
	}

	// convert string of arguments into an array of arguments
	argsArr, err := shellwords.Parse(argsStr)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// parse flags
	opts := listSecretOptions{}
	flags := pflag.NewFlagSet("dockerSecretList", pflag.ExitOnError)
	flags.BoolVarP(&opts.quiet, "quiet", "q", false, "Only display IDs")
	flags.Parse(argsArr)

	dockerCli := newDockerCli()
	options := types.SecretListOptions{}
	secrets, err := dockerCli.Client().SecretList(context.Background(), options)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// Lua table containing all networks
	secretsLuaTable := s.luaState.CreateTable(0, 0)
	// loop over the networks
	for _, secret := range secrets {
		// Lua table containing one network
		secretLuaTable := s.luaState.CreateTable(0, 0)
		// secret id
		secretLuaTable.RawSetString("id", lua.LString(secret.ID))
		// secret version
		secretLuaTable.RawSetString("version", lua.LNumber(secret.Meta.Version.Index))
		// created
		secretLuaTable.RawSetString("created", lua.LNumber(secret.Meta.CreatedAt.Unix()))
		// updated
		secretLuaTable.RawSetString("updated", lua.LNumber(secret.Meta.UpdatedAt.Unix()))

		// name
		secretLuaTable.RawSetString("name", lua.LString(secret.Spec.Annotations.Name))
		// labels
		secretLabels := s.luaState.CreateTable(0, 0)
		for key, value := range secret.Spec.Annotations.Labels {
			secretLabels.RawSetString(key, lua.LString(value))
		}
		secretLuaTable.RawSetString("labels", secretLabels)

		// data
		secretLuaTable.RawSetString("data", lua.LString(string(secret.Spec.Data)))

		// add this secret's Lua table to the Lua table containing all secrets
		secretsLuaTable.Append(secretLuaTable)
	}

	s.luaState.Push(secretsLuaTable)
	return 1
}

func newDockerCli() *command.DockerCli {
	// it's necessary to (re-)initiate the *command.DockerCli to consider
	// environment variable changes between to docker function calls
	stdin, stdout, stderr := term.StdStreams()
	dockerCli := command.NewDockerCli(stdin, stdout, stderr)
	dockerCli.Initialize(cliflags.NewClientOptions())
	return dockerCli
}
