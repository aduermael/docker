package project

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

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

	// errors
	ErrNilSandbox           = errors.New("sandbox is nil")
	ErrNilLuaState          = errors.New("lua state is nil")
	ErrProjectValueNotFound = errors.New("project lua value not found")
	ErrLuaValueNotATable    = errors.New("lua value is not a table")
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
	id, _, err := p.getProjectInfo()
	if err != nil {
		log.Fatalln(err.Error())
	}
	return id
}
func (p *Project) Name() string {
	_, name, err := p.getProjectInfo()
	if err != nil {
		log.Fatalln(err.Error())
	}
	return name
}
func (p *Project) Commands() ([]iface.Command, error) {
	cmds, err := p.listCommands()
	if err != nil {
		return nil, err
	}
	return cmds, nil
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

func (p *Project) chRootDir() (previousWorkDir string, err error) {
	previousWorkDir, err = os.Getwd()
	if err != nil {
		return
	}
	err = os.Chdir(p.RootDir())
	if err != nil {
		return
	}
	return
}

// Exec ...
func (p *Project) Exec(args []string) (found bool, err error) {
	found = false
	err = nil

	if len(args) == 0 {
		return found, errors.New("at least one argument required (task name)")
	}

	functionName := args[0]

	// go to project root dir
	previousWorkDir, err := p.chRootDir()
	if err != nil {
		return
	}
	defer os.Chdir(previousWorkDir)

	cmds, err := p.listCommands()
	if err != nil {
		return found, err
	}
	var cmd *iface.Command
	for _, c := range cmds {
		if c.Name == functionName {
			found = true
			cmd = &c
			break
		}
	}

	if found == false {
		return found, nil
	}

	argsTbl := p.Sandbox.GetLuaState().CreateTable(0, 0)
	for _, arg := range args[1:] {
		if strings.Contains(arg, " ") {
			arg = strings.Replace(arg, "\"", "\\\"", -1)
			arg = "\"" + arg + "\""
		}
		argsTbl.Append(lua.LString(arg))
	}
	err = p.Sandbox.GetLuaState().CallByParam(lua.P{
		Fn:      cmd.Function,
		NRet:    0,
		Protect: true,
	}, argsTbl)

	return found, err
}

// listCommands returns commands defined for the project.
// This function parses the main "dockerfile.lua" but also the
func (p *Project) listCommands() (cmds []iface.Command, err error) {
	cmds = make([]iface.Command, 0)
	errorPrefix := "error in Lua tasks definition: "

	// get project table
	projectTable, err := p.getProjectTable()
	if err != nil {
		return nil, err
	}

	// retrieve "project.tasks" table
	tasksTable, err := getTableFromTable(projectTable, "tasks")
	if err != nil {
		return nil, err
	}

	// tasks table cannot be an array, it has to be a map
	if tasksTable.Len() != 0 {
		return nil, errors.New(errorPrefix + "can't accept arrays, only pure maps")
	}

	// loop over the keys (keys have to be strings)
	var keys []lua.LValue = make([]lua.LValue, 0)
	tasksTable.ForEach(func(k, v lua.LValue) {
		keys = append(keys, k)
	})

	for _, k := range keys {
		kStr, ok := luaValueToString(k)
		if !ok {
			return nil, errors.New(errorPrefix + "task names must be strings")
		}
		v := tasksTable.RawGetString(string(kStr))
		// value can be a function
		if luaFunction, ok := luaValueToFunction(v); ok {
			cmds = append(cmds, iface.Command{
				Name:             string(kStr),
				ShortDescription: "",
				Description:      "",
				Function:         luaFunction,
			})
		} else if lt, ok := luaValueToTable(v); ok {
			if luaTableIsArray(lt) { // value can be a table (array)
				if lt.Len() == 1 { // one-cell array (must be function)
					if luaFunction, ok := luaValueToFunction(lt.RawGetInt(1)); ok {
						cmds = append(cmds, iface.Command{
							Name:             string(kStr),
							ShortDescription: "",
							Description:      "",
							Function:         luaFunction,
						})
					} else {
						return nil, errors.New(errorPrefix + "one-cell array must contain a function (" + string(kStr) + ")")
					}
				} else if lt.Len() == 2 {
					if luaFunction, ok := luaValueToFunction(lt.RawGetInt(1)); ok {
						if str, ok := luaValueToString(lt.RawGetInt(2)); ok {
							cmds = append(cmds, iface.Command{
								Name:             string(kStr),
								ShortDescription: string(str),
								Description:      string(str),
								Function:         luaFunction,
							})
						} else {
							return nil, errors.New(errorPrefix + "2-cell array must contain a function and a string (" + string(kStr) + ")")
						}
					} else {
						return nil, errors.New(errorPrefix + "2-cell array must contain a function and a string (" + string(kStr) + ")")
					}
				} else if lt.Len() == 3 {
					if luaFunction, ok := luaValueToFunction(lt.RawGetInt(1)); ok {
						if str1, ok1 := luaValueToString(lt.RawGetInt(2)); ok1 {
							if str2, ok2 := luaValueToString(lt.RawGetInt(3)); ok2 {
								cmds = append(cmds, iface.Command{
									Name:             string(kStr),
									ShortDescription: string(str1),
									Description:      string(str2),
									Function:         luaFunction,
								})
							} else {
								return nil, errors.New(errorPrefix + "3-cell array must contain a function and 2 strings (" + string(kStr) + ")")
							}
						} else {
							return nil, errors.New(errorPrefix + "3-cell array must contain a function and 2 strings (" + string(kStr) + ")")
						}
					} else {
						return nil, errors.New(errorPrefix + "3-cell array must contain a function and 2 strings (" + string(kStr) + ")")
					}
				} else {
					return nil, errors.New(errorPrefix + "tasks defined as arrays can only have 1, 2 or 3 elements (" + string(kStr) + ")")
				}
			} else if luaTableIsMap(lt) { // value can be a table (map)
				funcVal := lt.RawGetString("func")
				shortVal := lt.RawGetString("short")
				descVal := lt.RawGetString("desc")

				if luaFunction, ok := luaValueToFunction(funcVal); ok {
					shortStr := ""
					descStr := ""
					if luaStr, ok := luaValueToString(shortVal); ok {
						shortStr = string(luaStr)
					}
					if luaStr, ok := luaValueToString(descVal); ok {
						descStr = string(luaStr)
					}
					if shortStr == "" && descStr != "" {
						shortStr = descStr
					} else if shortStr != "" && descStr == "" {
						descStr = shortStr
					}
					cmds = append(cmds, iface.Command{
						Name:             string(kStr),
						ShortDescription: string(shortStr),
						Description:      string(descStr),
						Function:         luaFunction,
					})
				} else {
					return nil, errors.New(errorPrefix + "\"func\" field of a task must be a function (" + string(kStr) + ")")
				}
			} else {
				return nil, errors.New(errorPrefix + "definition accepts a pure \"map\" OR a pure \"array\" (" + string(kStr) + ")")
			}
		} else {
			return nil, errors.New(errorPrefix + "definition can only be a Lua function or a Lua table (" + string(kStr) + ")")
		}
	}

	sort.Sort(ByName(cmds))

	return cmds, nil
}

type ByName []iface.Command

func (a ByName) Len() int           { return len(a) }
func (a ByName) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool { return a[i].Name < a[j].Name }

// CommandExists indicates whether a command has been defined in the project
func (p *Project) CommandExists(cmd string) (bool, error) {
	commands, err := p.listCommands()
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

	// go to project root dir
	previousWorkDir, err := p.chRootDir()
	if err != nil {
		return nil, err
	}
	defer os.Chdir(previousWorkDir)

	// add project root dir path to the sandbox
	ls := sb.GetLuaState()
	if ls == nil {
		return nil, ErrNilLuaState
	}

	err = populateLuaState(ls, p)
	if err != nil {
		return nil, err
	}

	projTable := ls.CreateTable(0, 0)
	projTable.RawSetString("root", lua.LString(projectRootDirPath))
	ls.Env.RawSetString("project", projTable)

	// load config file
	found, err := sb.DoFile(iface.ConfigFileName)
	if err != nil {
		return nil, err
	}
	if found == false {
		return nil, errors.New("can't find " + iface.ConfigFileName)
	}

	// make sure commands are correctly implemented
	_, err = p.listCommands()
	if err != nil {
		return nil, err
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

	_, err = os.Stat(filename)
	if os.IsNotExist(err) {
		L.RaiseError("file not found")
		return 0
	}

	fn, err := p.Sandbox.GetLuaState().LoadFile(filename)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	err = p.Sandbox.GetLuaState().CallByParam(lua.P{
		Fn:      fn,
		NRet:    lua.MultRet,
		Protect: true,
	})

	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	return p.Sandbox.GetLuaState().GetTop()
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
	dockerContainerLuaTable.RawSetString("inspect", ls.NewFunction(dockerContainerInspect))
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

// getProjectInfo returns project's id and name
func (p *Project) getProjectInfo() (id string, name string, err error) {
	projectTable, err := p.getProjectTable()
	if err != nil {
		return "", "", err
	}
	idVal := projectTable.RawGetString("id")
	nameVal := projectTable.RawGetString("name")
	idLuaStr, ok := luaValueToString(idVal)
	if ok == false {
		return "", "", errors.New("project id is not a string")
	}
	nameLuaStr, ok := luaValueToString(nameVal)
	if ok == false {
		return "", "", errors.New("project name is not a string")
	}
	return string(idLuaStr), string(nameLuaStr), nil
}

// getProjectTable retrieves the "project" table from project config
func (p *Project) getProjectTable() (*lua.LTable, error) {
	// get project's sandbox
	if p.Sandbox == nil {
		return nil, ErrNilSandbox
	}
	// get sandbox' lua state
	ls := p.Sandbox.GetLuaState()
	if ls == nil {
		return nil, ErrNilLuaState
	}
	// get global value named "project"
	lv := ls.Env.RawGetString("project")
	if lv == nil {
		return nil, ErrProjectValueNotFound
	}
	// cast Lua value into a Lua table
	lt, ok := luaValueToTable(lv)
	if ok == false {
		return nil, ErrLuaValueNotATable
	}
	return lt, nil
}
