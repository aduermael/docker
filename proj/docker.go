package project

import (
	"bytes"
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cli/command"
	cliflags "github.com/docker/docker/cli/flags"
	sandbox "github.com/docker/docker/lua-sandbox"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/term"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerCmd executes the docker command passed as argument.
func dockerCmd(L *lua.LState) int {
	var err error

	dockerCli := newDockerCli()
	cmd := newDockerCommand(dockerCli)

	// retrieve parameter
	argsStr, found, err := sandbox.PopStringParam(L)
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
func dockerSilentCmd(L *lua.LState) int {
	var err error

	// retrieve parameter
	argsStr, found, err := sandbox.PopStringParam(L)
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
	L.Push(lua.LString(outStr))
	errStr := strings.TrimSpace(errbuf.String())
	if errStr != "" {
		L.Push(lua.LString(errStr))
		return 2
	}
	return 1
}

// dockerVolumeList lists Docker volumes and returns a Lua table (array)
// containing the volumes' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker volume ls` command.
// docker.volume.list(arguments string)
func dockerVolumeList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := sandbox.PopStringParam(L)
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
	volumesLuaTable := L.CreateTable(0, 0)

	// loop over the images
	for _, volume := range volumes.Volumes {
		if volume == nil {
			continue
		}
		// Lua table containing one volume
		volumeLuaTable := L.CreateTable(0, 0)
		// volume driver
		volumeLuaTable.RawSetString("driver", lua.LString(volume.Driver))
		// volume labels
		volumeLabels := L.CreateTable(0, 0)
		for key, value := range volume.Labels {
			volumeLabels.RawSetString(key, lua.LString(value))
		}
		volumeLuaTable.RawSetString("labels", volumeLabels)
		// volume mount point
		volumeLuaTable.RawSetString("mountPoint", lua.LString(volume.Mountpoint))
		// volume name
		volumeLuaTable.RawSetString("name", lua.LString(volume.Name))
		// volume options
		volumeOptions := L.CreateTable(0, 0)
		for _, volumeOption := range volume.Options {
			volumeOptions.Append(lua.LString(volumeOption))
		}
		volumeLuaTable.RawSetString("options", volumeOptions)
		// volume scope
		volumeLuaTable.RawSetString("scope", lua.LString(volume.Scope))

		// add this volume's Lua table to the Lua table containing all volumes
		volumesLuaTable.Append(volumeLuaTable)
	}

	L.Push(volumesLuaTable)
	return 1
}

// dockerNetworkList lists Docker networks and returns a Lua table (array)
// containing the networks' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker network ls` command.
// docker.network.list(arguments string)
func dockerNetworkList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := sandbox.PopStringParam(L)
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
	networksLuaTable := L.CreateTable(0, 0)

	// loop over the networks
	for _, network := range networks {
		// Lua table containing one network
		networkLuaTable := L.CreateTable(0, 0)
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
		networkOptions := L.CreateTable(0, 0)
		for _, networkOption := range network.Options {
			networkOptions.Append(lua.LString(networkOption))
		}
		networkLuaTable.RawSetString("options", networkOptions)
		// network labels
		networkLabels := L.CreateTable(0, 0)
		for key, value := range network.Labels {
			networkLabels.RawSetString(key, lua.LString(value))
		}
		networkLuaTable.RawSetString("labels", networkLabels)
		// add this image's Lua table to the Lua table containing all images
		networksLuaTable.Append(networkLuaTable)
	}

	L.Push(networksLuaTable)
	return 1
}

// dockerSecretList lists Docker secrets and returns a Lua table (array)
// containing the secrets' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker secret ls` command.
// docker.secret.list(arguments string)
func dockerSecretList(L *lua.LState) int {
	var err error

	// retrieve string argument
	argsStr, found, err := sandbox.PopStringParam(L)
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
	secretsLuaTable := L.CreateTable(0, 0)
	// loop over the networks
	for _, secret := range secrets {
		// Lua table containing one network
		secretLuaTable := L.CreateTable(0, 0)
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
		secretLabels := L.CreateTable(0, 0)
		for key, value := range secret.Spec.Annotations.Labels {
			secretLabels.RawSetString(key, lua.LString(value))
		}
		secretLuaTable.RawSetString("labels", secretLabels)

		// data
		secretLuaTable.RawSetString("data", lua.LString(string(secret.Spec.Data)))

		// add this secret's Lua table to the Lua table containing all secrets
		secretsLuaTable.Append(secretLuaTable)
	}

	L.Push(secretsLuaTable)
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
