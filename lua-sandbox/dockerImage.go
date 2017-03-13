package sandbox

import (
	"context"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/opts"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

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

	// get the non-flag command-line arguments
	args := flags.Args()

	if len(args) > 0 {
		opts.matchName = args[0]
	}

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
