package project

import (
	"context"

	"github.com/docker/docker/api/types"
	sandbox "github.com/docker/docker/lua-sandbox"
	"github.com/docker/docker/opts"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerServiceList lists Docker services and returns a Lua table (array)
// containing the services' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker service ls` command.
// docker.service.list(arguments string)
func dockerServiceList(L *lua.LState) int {
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
	servicesLuaTable := L.CreateTable(0, 0)

	// loop over the networks
	for _, service := range services {
		// Lua table containing one network
		serviceLuaTable := L.CreateTable(0, 0)

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
		serviceLabels := L.CreateTable(0, 0)
		for key, value := range service.Spec.Annotations.Labels {
			serviceLabels.RawSetString(key, lua.LString(value))
		}
		serviceLuaTable.RawSetString("labels", serviceLabels)
		// image
		serviceLuaTable.RawSetString("image", lua.LString(service.Spec.TaskTemplate.ContainerSpec.Image))
		// add this image's Lua table to the Lua table containing all images
		servicesLuaTable.Append(serviceLuaTable)
	}

	L.Push(servicesLuaTable)
	return 1
}
