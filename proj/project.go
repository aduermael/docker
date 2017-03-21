package project

import (
	"errors"
	"os"
	"path/filepath"

	sandbox "github.com/docker/docker/lua-sandbox"
	iface "github.com/docker/docker/proj/project"
	lua "github.com/yuin/gopher-lua"
)

var (
	// CommandsAllowedToBeOverridden is the list of docker commands for which
	// override is allowed.
	CommandsAllowedToBeOverridden = []string{
		"build",
		"deploy",
		"export",
		"logs",
		"restart",
		"run",
		"start",
		"stats",
		"stop",
	}
)

// Project defines a Docker project
type Project struct {
	RootDirVal string           `json:"root"`
	Sandbox    *sandbox.Sandbox `json:"_"`
}

//
// iface.Project interface implementation
//
func (p *Project) RootDir() string {
	return p.RootDirVal
}
func (p *Project) ID() string {
	return "not implemented" // TODO: gdevillele: implement this
}
func (p *Project) Name() string {
	return "not implemented" // TODO: gdevillele: implement this
}
func (p *Project) Commands() []iface.Command {
	return make([]iface.Command, 0)
}

// GetConfigFilePath returns absolute path to configuration file
func (p *Project) GetConfigFilePath() (path string, err error) {
	absPath := filepath.Join(p.RootDirVal, iface.ConfigFileName)
	_, err = os.Stat(absPath)
	if err == nil {
		path = iface.ConfigFileName
	}
	return
}

// ListCommands returns commands defined for the project.
// This function parses the main "dockerfile.lua" but also the
func (p *Project) ListCommands() ([]ProjectCommand, error) {
	// // list project commands
	// dockerscript := filepath.Join(p.DockerProjectDirPath(), "dockerscript.lua")
	// cmds, err := listCommandsForDockerscript(dockerscript)
	// if err != nil {
	// 	return nil, err
	// }
	// // list user-specific project commands
	// userDockerscriptFileName, err := getUserDockerscriptFileName()
	// if err != nil {
	// 	return nil, err
	// }
	// userDockerscript := filepath.Join(p.DockerProjectDirPath(), userDockerscriptDirName, userDockerscriptFileName)
	// userCmds, err := listCommandsForDockerscript(userDockerscript)
	// if err != nil {
	// 	return nil, err
	// }
	// // build final list (user cmds override project cmds)
	// for _, usrCmd := range userCmds {
	// 	// check if it is an override
	// 	found := false
	// 	for j, prjCmd := range cmds {
	// 		if usrCmd.Name == prjCmd.Name {
	// 			cmds[j].Description = usrCmd.Description // override description
	// 			found = true
	// 		}
	// 	}
	// 	if found == false {
	// 		cmds = append(cmds, usrCmd)
	// 	}
	// }
	// return cmds, nil
	return nil, errors.New("not implemented")
}

// CommandExists indicates whether a command has been defined in the project
func (p *Project) CommandExists(cmd string) (bool, error) {
	commands, err := p.ListCommands()
	if err != nil {
		return false, err
	}
	for _, command := range commands {
		if command.Name == cmd {
			return true, nil
		}
	}
	return false, nil
}

// Get returns project for a given path.
// The configuration file can be in a parent directory, so we have to test all
// the way up to the root directory. If no configuration file is found then
// nil,nil is returned (no error)
func Load(path string) (*Project, error) {

	projectRootDirPath, err := iface.FindProjectRoot(path)
	if err != nil {
		// TODO: gdevillele: handle actual errors, for now we suppose no project is found
		return nil, nil
	}

	// config file path
	configFilePath := filepath.Join(projectRootDirPath, iface.ConfigFileName)

	// create Lua sandbox and load config
	sb, err := sandbox.CreateSandbox()
	if err != nil {
		return nil, err
	}

	// create project struct
	p := &Project{
		RootDirVal: projectRootDirPath,
		Sandbox:    sb,
	}

	err = populateLuaState(sb.GetLuaState(), p)
	if err != nil {
		return nil, err
	}

	found, err := sb.DoFile(configFilePath)
	if err != nil {
		return nil, err
	}
	if found == false {
		return nil, errors.New("config file not found")
	}

	return p, nil
}

// LoadForWd returns project for current working directory
func LoadForWd() (*Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Load(wd)
}

// IsCommandOverrideAllowed indicates whether a command is allowed to be overridden
func IsCommandOverrideAllowed(cmd string) bool {
	for _, c := range CommandsAllowedToBeOverridden {
		if c == cmd {
			return true
		}
	}
	return false
}

////////////////////////////////////////
//
// LUA FUNCTIONS
//
////////////////////////////////////////

//
func (p *Project) luaRequire(L *lua.LState) int {

	// retrieve string argument
	filename, found, err := sandbox.PopStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("missing string argument")
		return 0
	}

	if filepath.Ext(filename) != ".lua" {
		filename += ".lua"
	}

	// create sandbox
	sb, err := sandbox.CreateSandbox()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	err = populateLuaState(sb.GetLuaState(), p)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	found, err = sb.DoFile(filename)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if found == false {
		L.RaiseError("file not found")
		return 0
	}

	L.Push(sb.GetLuaState().Env)
	return 1
}

// populateLuaState adds functions to the Lua sandbox
func populateLuaState(ls *lua.LState, p *Project) error {

	// require
	ls.Env.RawSetString("require", ls.NewFunction(p.luaRequire))

	// docker
	dockerLuaTable := ls.CreateTable(0, 0)
	dockerLuaTable.RawSetString("cmd", ls.NewFunction(dockerCmd))
	dockerLuaTable.RawSetString("silentCmd", ls.NewFunction(dockerSilentCmd))

	// docker.container
	dockerContainerLuaTable := ls.CreateTable(0, 0)
	dockerContainerLuaTable.RawSetString("list", ls.NewFunction(dockerContainerList))
	dockerLuaTable.RawSetString("container", dockerContainerLuaTable)

	// docker.image
	dockerImageLuaTable := ls.CreateTable(0, 0)
	// dockerImageLuaTable.RawSetString("build", ls.NewFunction(s.dockerImageBuild))
	dockerImageLuaTable.RawSetString("list", ls.NewFunction(dockerImageList))
	dockerLuaTable.RawSetString("image", dockerImageLuaTable)

	// docker network
	dockerNetworkLuaTable := ls.CreateTable(0, 0)
	dockerNetworkLuaTable.RawSetString("list", ls.NewFunction(dockerNetworkList))
	dockerLuaTable.RawSetString("network", dockerNetworkLuaTable)

	// docker secret
	dockerSecretLuaTable := ls.CreateTable(0, 0)
	dockerSecretLuaTable.RawSetString("list", ls.NewFunction(dockerSecretList))
	dockerLuaTable.RawSetString("secret", dockerSecretLuaTable)

	// docker service
	dockerServiceLuaTable := ls.CreateTable(0, 0)
	dockerServiceLuaTable.RawSetString("list", ls.NewFunction(dockerServiceList))
	dockerLuaTable.RawSetString("service", dockerServiceLuaTable)

	// docker volume
	dockerVolumeLuaTable := ls.CreateTable(0, 0)
	dockerVolumeLuaTable.RawSetString("list", ls.NewFunction(dockerVolumeList))
	dockerLuaTable.RawSetString("volume", dockerVolumeLuaTable)

	// // docker.project
	// if p != nil {
	// 	dockerProjectLuaTable := ls.CreateTable(0, 0)
	// 	dockerProjectLuaTable.RawSetString("id", lua.LString(p.ID()))
	// 	dockerProjectLuaTable.RawSetString("name", lua.LString(p.Name()))
	// 	dockerProjectLuaTable.RawSetString("root", lua.LString(p.RootDir()))
	// 	dockerLuaTable.RawSetString("project", dockerProjectLuaTable)
	// }

	err := sandbox.AddTableToLuaState(dockerLuaTable, ls, "docker")
	if err != nil {
		return err
	}

	return nil
}
