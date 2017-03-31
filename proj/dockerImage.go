package project

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/cli/command/inspect"
	sandbox "github.com/docker/docker/lua-sandbox"
	"github.com/docker/docker/opts"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/spf13/pflag"
	lua "github.com/yuin/gopher-lua"
)

// dockerContainerInspect inspects one or several images identified by names
// or ids, it returns a Lua table array containing one table (or nil) for
// each image
func dockerImageInspect(L *lua.LState) int {

	opts := imageInspectOptions{
		format: "",
		refs:   make([]string, 0),
	}

	for {
		// identifiers can be names or ids
		identifier, found, err := sandbox.PopStringParam(L)
		if err != nil {
			L.RaiseError(err.Error())
			return 0
		}
		if !found {
			break
		}
		opts.refs = append(opts.refs, identifier)
	}

	ctx := context.Background()
	dockerCli := newDockerCli()

	getRefFunc := func(ref string) (interface{}, []byte, error) {
		return dockerCli.Client().ImageInspectWithRaw(ctx, ref)
	}

	reader, writer := io.Pipe()
	dec := json.NewDecoder(reader)

	go func() {
		inspect.Inspect(writer, opts.refs, opts.format, getRefFunc)
	}()

	// read open bracket
	_, err := dec.Token()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	images := make([]types.ImageInspect, 0)

	// while the array contains values
	for dec.More() {
		var image types.ImageInspect
		// decode an array value (Message)
		err := dec.Decode(&image)
		if err != nil {
			L.RaiseError(err.Error())
			return 0
		}
		images = append(images, image)
	}

	// read closing bracket
	_, err = dec.Token()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	imagesTbl := L.CreateTable(0, 0)

	for _, image := range images {
		imagesTbl.Append(ImageInspectToLuaTable(&image, L))
	}

	L.Push(imagesTbl)
	return 1
}

// dockerImageList lists Docker images and returns a Lua table (array)
// containing the images' descriptions.
// It accepts one (optional) string argument, identical to CLI arguments
// received by `docker image ls` command.
// docker.image.list(arguments string)
func dockerImageList(L *lua.LState) int {
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
	imagesLuaTable := L.CreateTable(0, 0)

	// loop over the images
	for _, image := range images {
		// Lua table containing one image
		imageLuaTable := L.CreateTable(0, 0)

		// image id
		// removing prefixes like in image ids like:
		// sha256:5dae07823d481dab69d6a278b4014cb2978b96ef0874ac18fd2ad050a2a32699
		imageLuaTable.RawSetString("id", lua.LString(removeImageIDHeader(image.ID)))
		imageLuaTable.RawSetString("parentId", lua.LString(removeImageIDHeader(image.ParentID)))
		imageLuaTable.RawSetString("created", lua.LNumber(float64(image.Created)))
		// (gdevillele:) removed this as, even if the field exists, the value
		//               it contains will always be -1 as this field is not used
		//               for listing images.
		// imageLuaTable.RawSetString("sharedSize", lua.LNumber(float64(image.SharedSize)))
		imageLuaTable.RawSetString("size", lua.LNumber(float64(image.Size)))
		// add RepoTags
		repoTags := L.CreateTable(0, 0)
		for _, repoTag := range image.RepoTags {
			repoTags.Append(lua.LString(repoTag))
		}
		imageLuaTable.RawSetString("repoTags", repoTags)

		// add this image's Lua table to the Lua table containing all images
		imagesLuaTable.Append(imageLuaTable)
	}

	L.Push(imagesLuaTable)
	return 1
}

// removeImageIDHeader removes image ID header
// sha256:46777e73b612aaf22ed0ffc0f2cadb992d3e69580bb391174463a1ff45c5017b
func removeImageIDHeader(imageID string) string {
	if strings.Contains(imageID, ":") {
		parts := strings.SplitN(imageID, ":", 2)
		if len(parts) == 2 {
			imageID = parts[1]
		}
	}
	return imageID
}

func ImageInspectToLuaTable(i *types.ImageInspect, L *lua.LState) *lua.LTable {
	imageTbl := L.CreateTable(0, 0)

	imageTbl.RawSetString("id", lua.LString(i.ID))
	imageRepoTagsTbl := L.CreateTable(0, 0)
	for _, repoTag := range i.RepoTags {
		imageRepoTagsTbl.Append(lua.LString(repoTag))
	}
	imageTbl.RawSetString("repoTags", imageRepoTagsTbl)
	imageRepoDigestsTbl := L.CreateTable(0, 0)
	for _, repoDigest := range i.RepoDigests {
		imageRepoDigestsTbl.Append(lua.LString(repoDigest))
	}
	imageTbl.RawSetString("repoDigests", imageRepoDigestsTbl)
	imageTbl.RawSetString("parent", lua.LString(i.Parent))
	imageTbl.RawSetString("comment", lua.LString(i.Comment))
	imageTbl.RawSetString("created", lua.LString(i.Created))
	imageTbl.RawSetString("container", lua.LString(i.Container))
	// TODO: ContainerConfig
	imageTbl.RawSetString("dockerVersion", lua.LString(i.DockerVersion))
	imageTbl.RawSetString("author", lua.LString(i.Author))
	// TODO: Config
	imageTbl.RawSetString("architecture", lua.LString(i.Architecture))
	imageTbl.RawSetString("os", lua.LString(i.Os))
	imageTbl.RawSetString("osVersion", lua.LString(i.OsVersion))
	imageTbl.RawSetString("size", lua.LNumber(i.Size))
	imageTbl.RawSetString("virtualSize", lua.LNumber(i.VirtualSize))
	// graph driver
	imageGraphDriverTbl := L.CreateTable(0, 0)
	imageGraphDriverDataTbl := L.CreateTable(0, 0)
	for key, value := range i.GraphDriver.Data {
		imageGraphDriverDataTbl.RawSetString(key, lua.LString(value))
	}
	imageGraphDriverTbl.RawSetString("data", imageGraphDriverDataTbl)
	imageGraphDriverTbl.RawSetString("name", lua.LString(i.GraphDriver.Name))
	imageTbl.RawSetString("graphDriver", imageGraphDriverTbl)
	// rootFS
	imageRootFSTbl := L.CreateTable(0, 0)
	imageRootFSTbl.RawSetString("type", lua.LString(i.RootFS.Type))
	imageRootFSLayersTbl := L.CreateTable(0, 0)
	for _, layer := range i.RootFS.Layers {
		imageRootFSLayersTbl.Append(lua.LString(layer))
	}
	imageRootFSTbl.RawSetString("layers", imageRootFSLayersTbl)
	imageRootFSTbl.RawSetString("baseLayer", lua.LString(i.RootFS.BaseLayer))
	imageTbl.RawSetString("rootFS", imageRootFSTbl)

	return imageTbl

}
