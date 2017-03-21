package project

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api"
	"github.com/docker/docker/api/types"
	sandbox "github.com/docker/docker/lua-sandbox"
	"github.com/docker/docker/opts"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerContainerList lists Docker containers and returns a Lua table (array)
// containing the containers' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker container ls` command.
// docker.container.list(arguments string)
func dockerContainerList(L *lua.LState) int {
	var err error

	// retrieve parameter
	argsStr, found, err := sandbox.PopStringParam(L)
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

	containersTbl := L.CreateTable(0, 0)

	for _, container := range containers {

		containerTbl := L.CreateTable(0, 0)
		containerTbl.RawSetString("id", lua.LString(container.ID))

		containerNamesTbl := L.CreateTable(0, 0)
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
		containerPortsTbl := L.CreateTable(0, 0)
		for _, port := range container.Ports {
			containerPortTbl := L.CreateTable(0, 0)
			containerPortTbl.RawSetString("ip", lua.LString(port.IP))
			containerPortTbl.RawSetString("public", lua.LNumber(port.PublicPort))
			containerPortTbl.RawSetString("private", lua.LNumber(port.PrivatePort))
			containerPortTbl.RawSetString("type", lua.LString(port.Type))
			containerPortTbl.RawSetString("string", lua.LString(api.DisplayablePorts([]types.Port{port})))

			containerPortsTbl.Append(containerPortTbl)
		}
		containerTbl.RawSetString("ports", containerPortsTbl)

		// labels
		containerLabelsTbl := L.CreateTable(0, 0)
		for key, value := range container.Labels {
			containerLabelsTbl.RawSetString(key, lua.LString(value))
		}
		containerTbl.RawSetString("labels", containerLabelsTbl)

		// TODO: Mounts, NetworkSettings & HostConfig

		containersTbl.Append(containerTbl)
	}

	L.Push(containersTbl)
	return 1
}
