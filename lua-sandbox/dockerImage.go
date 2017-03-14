package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/docker/distribution/reference"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/builder/dockerignore"
	"github.com/docker/docker/cli/command"
	"github.com/docker/docker/cli/command/image"
	"github.com/docker/docker/cli/command/image/build"
	"github.com/docker/docker/opts"
	"github.com/docker/docker/pkg/archive"
	"github.com/docker/docker/pkg/fileutils"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/progress"
	"github.com/docker/docker/pkg/streamformatter"
	"github.com/docker/docker/pkg/urlutil"
	project "github.com/docker/docker/proj"
	runconfigopts "github.com/docker/docker/runconfig/opts"
	units "github.com/docker/go-units"
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
		imageLuaTable.RawSetString("id", lua.LString(removeImageIDHeader(image.ID)))
		imageLuaTable.RawSetString("parentId", lua.LString(removeImageIDHeader(image.ParentID)))
		imageLuaTable.RawSetString("created", lua.LNumber(float64(image.Created)))
		// (gdevillele:) removed this as, even if the field exists, the value
		//               it contains will always be -1 as this field is not used
		//               for listing images.
		// imageLuaTable.RawSetString("sharedSize", lua.LNumber(float64(image.SharedSize)))
		imageLuaTable.RawSetString("size", lua.LNumber(float64(image.Size)))
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

// imageBuild is a lua function mapping the "docker image build" command.
// It takes one string arguments, and returns a Lua table representing
// the built image or raises an error.
// local myImageTable = build('-t myImage .')
func (s *Sandbox) dockerImageBuild(L *lua.LState) int {
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

	ulimits := make(map[string]*units.Ulimit)
	options := buildOptions{
		tags:       opts.NewListOpts(validateTag),
		buildArgs:  opts.NewListOpts(opts.ValidateEnv),
		ulimits:    opts.NewUlimitOpt(&ulimits),
		labels:     opts.NewListOpts(opts.ValidateEnv),
		extraHosts: opts.NewListOpts(opts.ValidateExtraHost),
	}

	// parse flags
	flags := pflag.NewFlagSet("dockerImageBuild", pflag.ExitOnError)

	flags.VarP(&options.tags, "tag", "t", "Name and optionally a tag in the 'name:tag' format")
	flags.Var(&options.buildArgs, "build-arg", "Set build-time variables")
	flags.Var(options.ulimits, "ulimit", "Ulimit options")
	flags.StringVarP(&options.dockerfileName, "file", "f", "", "Name of the Dockerfile (Default is 'PATH/Dockerfile')")
	flags.VarP(&options.memory, "memory", "m", "Memory limit")
	flags.Var(&options.memorySwap, "memory-swap", "Swap limit equal to memory plus swap: '-1' to enable unlimited swap")
	flags.Var(&options.shmSize, "shm-size", "Size of /dev/shm")
	flags.Int64VarP(&options.cpuShares, "cpu-shares", "c", 0, "CPU shares (relative weight)")
	flags.Int64Var(&options.cpuPeriod, "cpu-period", 0, "Limit the CPU CFS (Completely Fair Scheduler) period")
	flags.Int64Var(&options.cpuQuota, "cpu-quota", 0, "Limit the CPU CFS (Completely Fair Scheduler) quota")
	flags.StringVar(&options.cpuSetCpus, "cpuset-cpus", "", "CPUs in which to allow execution (0-3, 0,1)")
	flags.StringVar(&options.cpuSetMems, "cpuset-mems", "", "MEMs in which to allow execution (0-3, 0,1)")
	flags.StringVar(&options.cgroupParent, "cgroup-parent", "", "Optional parent cgroup for the container")
	flags.StringVar(&options.isolation, "isolation", "", "Container isolation technology")
	flags.Var(&options.labels, "label", "Set metadata for an image")
	flags.BoolVar(&options.noCache, "no-cache", false, "Do not use cache when building the image")
	flags.BoolVar(&options.rm, "rm", true, "Remove intermediate containers after a successful build")
	flags.BoolVar(&options.forceRm, "force-rm", false, "Always remove intermediate containers")
	flags.BoolVarP(&options.quiet, "quiet", "q", false, "Suppress the build output and print image ID on success")
	flags.BoolVar(&options.pull, "pull", false, "Always attempt to pull a newer version of the image")
	flags.StringSliceVar(&options.cacheFrom, "cache-from", []string{}, "Images to consider as cache sources")
	flags.BoolVar(&options.compress, "compress", false, "Compress the build context using gzip")
	flags.StringSliceVar(&options.securityOpt, "security-opt", []string{}, "Security options")
	flags.StringVar(&options.networkMode, "network", "default", "Set the networking mode for the RUN instructions during build")
	flags.SetAnnotation("network", "version", []string{"1.25"})
	flags.Var(&options.extraHosts, "add-host", "Add a custom host-to-IP mapping (host:ip)")

	command.AddTrustVerificationFlags(flags)

	flags.BoolVar(&options.squash, "squash", false, "Squash newly built layers into a single new layer")
	flags.SetAnnotation("squash", "experimental", nil)
	flags.SetAnnotation("squash", "version", []string{"1.25"})

	flags.Parse(argsArr)

	// get the non-flag command-line arguments
	args := flags.Args()

	if len(args) < 1 {
		L.RaiseError("function requires exactly 1 (non-flag) argument. You have probably forgotten the context path.")
		return 0
	}

	if len(args) > 0 {
		options.context = args[0]
	}

	// force quiet flag
	options.quiet = true

	dockerCli := newDockerCli()

	var (
		buildCtx io.ReadCloser
		// err           error
		contextDir    string
		tempDir       string
		relDockerfile string
		progBuff      io.Writer
		// buildBuff     io.Writer
	)

	specifiedContext := options.context
	progBuff = dockerCli.Out()
	// buildBuff = dockerCli.Out()
	if options.quiet {
		progBuff = bytes.NewBuffer(nil)
		// buildBuff = bytes.NewBuffer(nil)
	}

	switch {
	case specifiedContext == "-":
		buildCtx, relDockerfile, err = build.GetContextFromReader(dockerCli.In(), options.dockerfileName)
	case isLocalDir(specifiedContext):
		contextDir, relDockerfile, err = build.GetContextFromLocalDir(specifiedContext, options.dockerfileName)
	case urlutil.IsGitURL(specifiedContext):
		tempDir, relDockerfile, err = build.GetContextFromGitURL(specifiedContext, options.dockerfileName)
	case urlutil.IsURL(specifiedContext):
		buildCtx, relDockerfile, err = build.GetContextFromURL(progBuff, specifiedContext, options.dockerfileName)
	default:
		L.RaiseError(fmt.Sprintf("unable to prepare context: path %q not found", specifiedContext))
		return 0
	}

	if err != nil {
		if options.quiet && urlutil.IsURL(specifiedContext) {
			fmt.Fprintln(dockerCli.Err(), progBuff)
		}
		L.RaiseError(fmt.Sprintf("unable to prepare context: %s", err))
		return 0
	}

	if tempDir != "" {
		defer os.RemoveAll(tempDir)
		contextDir = tempDir
	}

	if buildCtx == nil {
		// And canonicalize dockerfile name to a platform-independent one
		relDockerfile, err = archive.CanonicalTarNameForPath(relDockerfile)
		if err != nil {
			L.RaiseError(fmt.Sprintf("cannot canonicalize dockerfile path %s: %v", relDockerfile, err))
			return 0
		}

		f, err := os.Open(filepath.Join(contextDir, ".dockerignore"))
		if err != nil && !os.IsNotExist(err) {
			L.RaiseError(err.Error())
			return 0
		}
		defer f.Close()

		var excludes []string
		if err == nil {
			excludes, err = dockerignore.ReadAll(f)
			if err != nil {
				L.RaiseError(err.Error())
				return 0
			}
		}

		if err := build.ValidateContextDirectory(contextDir, excludes); err != nil {
			L.RaiseError(fmt.Sprintf("Error checking context: '%s'.", err))
			return 0
		}

		// If .dockerignore mentions .dockerignore or the Dockerfile
		// then make sure we send both files over to the daemon
		// because Dockerfile is, obviously, needed no matter what, and
		// .dockerignore is needed to know if either one needs to be
		// removed. The daemon will remove them for us, if needed, after it
		// parses the Dockerfile. Ignore errors here, as they will have been
		// caught by validateContextDirectory above.
		var includes = []string{"."}
		keepThem1, _ := fileutils.Matches(".dockerignore", excludes)
		keepThem2, _ := fileutils.Matches(relDockerfile, excludes)
		if keepThem1 || keepThem2 {
			includes = append(includes, ".dockerignore", relDockerfile)
		}

		compression := archive.Uncompressed
		if options.compress {
			compression = archive.Gzip
		}
		buildCtx, err = archive.TarWithOptions(contextDir, &archive.TarOptions{
			Compression:     compression,
			ExcludePatterns: excludes,
			IncludeFiles:    includes,
		})
		if err != nil {
			L.RaiseError(err.Error())
			return 0
		}
	}

	ctx := context.Background()

	var resolvedTags []*resolvedTag
	if command.IsTrusted() {
		translator := func(ctx context.Context, ref reference.NamedTagged) (reference.Canonical, error) {
			return image.TrustedReference(ctx, dockerCli, ref, nil)
		}
		// Wrap the tar archive to replace the Dockerfile entry with the rewritten
		// Dockerfile which uses trusted pulls.
		buildCtx = replaceDockerfileTarWrapper(ctx, buildCtx, relDockerfile, translator, &resolvedTags)
	}

	// Setup an upload progress bar
	progressOutput := streamformatter.NewStreamFormatter().NewProgressOutput(progBuff, true)
	if !dockerCli.Out().IsTerminal() {
		progressOutput = &lastProgressOutput{output: progressOutput}
	}

	var body io.Reader = progress.NewProgressReader(buildCtx, progressOutput, 0, "", "Sending build context to Docker daemon")

	authConfigs, _ := dockerCli.GetAllCredentials()
	buildOptions := types.ImageBuildOptions{
		Memory:         options.memory.Value(),
		MemorySwap:     options.memorySwap.Value(),
		Tags:           options.tags.GetAll(),
		SuppressOutput: options.quiet,
		NoCache:        options.noCache,
		Remove:         options.rm,
		ForceRemove:    options.forceRm,
		PullParent:     options.pull,
		Isolation:      container.Isolation(options.isolation),
		CPUSetCPUs:     options.cpuSetCpus,
		CPUSetMems:     options.cpuSetMems,
		CPUShares:      options.cpuShares,
		CPUQuota:       options.cpuQuota,
		CPUPeriod:      options.cpuPeriod,
		CgroupParent:   options.cgroupParent,
		Dockerfile:     relDockerfile,
		ShmSize:        options.shmSize.Value(),
		Ulimits:        options.ulimits.GetList(),
		BuildArgs:      runconfigopts.ConvertKVStringsToMapWithNil(options.buildArgs.GetAll()),
		AuthConfigs:    authConfigs,
		Labels:         runconfigopts.ConvertKVStringsToMap(options.labels.GetAll()),
		CacheFrom:      options.cacheFrom,
		SecurityOpt:    options.securityOpt,
		NetworkMode:    options.networkMode,
		Squash:         options.squash,
		ExtraHosts:     options.extraHosts.GetAll(),
	}

	// Add label to identify project if needed.
	// Check whether we are in the context of a Docker project.
	proj, pErr := project.GetForWd()
	if pErr != nil {
		L.RaiseError(pErr.Error())
		return 0
	}
	if proj != nil {
		buildOptions.Labels["docker.project.id:"+proj.Config.ID] = ""
		buildOptions.Labels["docker.project.name:"+proj.Config.Name] = ""
	}

	response, err := dockerCli.Client().ImageBuild(ctx, body, buildOptions)
	if err != nil {
		if options.quiet {
			fmt.Fprintf(dockerCli.Err(), "%s", progBuff)
		}
		L.RaiseError(err.Error())
		return 0
	}
	defer response.Body.Close()

	// decode response
	jsonDecoder := json.NewDecoder(response.Body)
	jsonMessages := make([]jsonmessage.JSONMessage, 0)
	for {
		var jm jsonmessage.JSONMessage
		err := jsonDecoder.Decode(&jm)
		if err != nil {
			if err != io.EOF {
				L.RaiseError(err.Error())
				return 0
			}
			break
		}
		jsonMessages = append(jsonMessages, jm)
	}

	// check for error
	lastMessage := jsonMessages[len(jsonMessages)-1]
	if lastMessage.Error != nil && len(lastMessage.Error.Message) > 0 {
		L.RaiseError(lastMessage.Error.Message)
		return 0
	}

	// find the image ID
	var imageID string
	if len(jsonMessages) != 1 {
		// this is not supposed to happen
		L.RaiseError("failed to parse engine response")
		return 0
	}
	imageID = strings.TrimSpace(lastMessage.Stream) // sha256:1234567890abcdef
	imageID = removeImageIDHeader(imageID)
	if len(imageID) == 0 {
		L.RaiseError("failed to parse engine response [2]")
		return 0
	}

	if command.IsTrusted() {
		// Since the build was successful, now we must tag any of the resolved
		// images from the above Dockerfile rewrite.
		for _, resolved := range resolvedTags {
			if err := image.TagTrusted(ctx, dockerCli, resolved.digestRef, resolved.tagRef); err != nil {
				L.RaiseError(err.Error())
				return 0
			}
		}
	}

	// retrieve image information
	client := dockerCli.Client()
	// imgInspect, imgBytes, err := client.ImageInspectWithRaw(ctx, ref)
	imgInspect, _, err := client.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	// construct result Lua table

	// Lua table containing one image
	imageLuaTable := s.luaState.CreateTable(0, 0)
	imageLuaTable.RawSetString("id", lua.LString(imageID))
	imageLuaTable.RawSetString("parentId", lua.LString(removeImageIDHeader(imgInspect.Parent)))
	const RFC3339NanoFixed = "2006-01-02T15:04:05.000000000Z07:00"
	createdTime, err := time.Parse(RFC3339NanoFixed, imgInspect.Created)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	imageLuaTable.RawSetString("created", lua.LNumber(float64(createdTime.Unix())))
	imageLuaTable.RawSetString("size", lua.LNumber(float64(imgInspect.Size)))
	// add RepoTags
	repoTags := s.luaState.CreateTable(0, 0)
	for _, repoTag := range imgInspect.RepoTags {
		repoTags.Append(lua.LString(repoTag))
	}
	imageLuaTable.RawSetString("repoTags", repoTags)

	s.luaState.Push(imageLuaTable)
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
