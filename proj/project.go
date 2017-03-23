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
	ErrNilSandbox  = errors.New("sandbox is nil")
	ErrNilLuaState = errors.New("lua state is nil")
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
func (p *Project) Commands() []iface.Command {
	cmds, err := p.ListCommands()
	if err != nil {
		// error is not reported here TODO: gdevillele: error reporting !
		return make([]iface.Command, 0)
	}
	return cmds
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

// Exec ...
func (p *Project) Exec(args []string) (found bool, err error) {
	found = false
	err = nil

	if len(args) == 0 {
		return found, errors.New("at least one argument required (task name)")
	}

	functionName := args[0]

	cmds, err := p.ListCommands()
	if err != nil {
		return found, err
	}
	var cmd *iface.Command
	for _, c := range cmds {
		if c.Name == functionName {
			found = true
			cmd = &c
		}
	}

	if found == false {
		return found, nil
	}

	// chdir to project root dir
	currentWorkingDirectory, err := os.Getwd()
	projectRootDir := p.RootDir()
	if err != nil {
		return found, err
	}
	err = os.Chdir(projectRootDir)
	if err != nil {
		return found, err
	}
	defer os.Chdir(currentWorkingDirectory)

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

	// value := s.luaState.GetGlobal(functionName)
	// if value == lua.LNil {
	// 	return
	// }

	// fn, ok := value.(*lua.LFunction)
	// if !ok {
	// 	err = errors.New(functionName + " is not a function")
	// 	return
	// }

	// // from here we consider function has been found
	// found = true

	// // chdir to project root dir
	// projectRootDir := s.dockerProject.RootDir
	// currentWorkingDirectory, err := os.Getwd()
	// if err != nil {
	// 	return
	// }
	// os.Chdir(projectRootDir)
	// defer os.Chdir(currentWorkingDirectory)

	// argsTbl := s.luaState.CreateTable(0, 0)
	// for _, arg := range args[1:] {
	// 	if strings.Contains(arg, " ") {
	// 		arg = strings.Replace(arg, "\"", "\\\"", -1)
	// 		arg = "\"" + arg + "\""
	// 	}
	// 	argsTbl.Append(lua.LString(arg))
	// }

	// err = s.luaState.CallByParam(lua.P{
	// 	Fn:      fn,
	// 	NRet:    0,
	// 	Protect: true,
	// }, argsTbl)
	// return
}

// ListCommands returns commands defined for the project.
// This function parses the main "dockerfile.lua" but also the
func (p *Project) ListCommands() (cmds []iface.Command, err error) {
	cmds = make([]iface.Command, 0)

	if p.Sandbox == nil {
		return nil, ErrNilSandbox
	}
	ls := p.Sandbox.GetLuaState()
	if ls == nil {
		return nil, ErrNilLuaState
	}

	// get project table
	projectTable, err := getTableFromState(ls, "project")
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
		return nil, errors.New("tasks table has to be a pure map")
	}

	// loop over the keys (keys have to be strings)
	var keys []lua.LValue = make([]lua.LValue, 0)
	tasksTable.ForEach(func(k, v lua.LValue) {
		keys = append(keys, k)
	})

	for _, k := range keys {
		kStr, ok := luaValueToString(k)
		if !ok {
			return nil, errors.New("tasks names must be strings")
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
						return nil, errors.New("tasks defined as a one-cell array can only contain a function")
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
							return nil, errors.New("tasks defined as 2-cell arrays must contain a function and a string")
						}
					} else {
						return nil, errors.New("tasks defined as 2-cell arrays must contain a function and a string")
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
								return nil, errors.New("tasks defined as 3-cell arrays must contain a function and 2 strings")
							}
						} else {
							return nil, errors.New("tasks defined as 3-cell arrays must contain a function and 2 strings")
						}
					} else {
						return nil, errors.New("tasks defined as 3-cell arrays must contain a function and 2 strings")
					}
				} else {
					return nil, errors.New("tasks defined as arrays can only have 1, 2 or 3 elements")
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
					return nil, errors.New("the \"func\" field of a task must have a function value")
				}
			} else {
				return nil, errors.New("tasks can only bu pure \"map\" or pure \"array\" Lua tables")
			}
		} else {
			return nil, errors.New("tasks can only be Lua functions or Lua tables")
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

	// load config file
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

func (p *Project) getProjectInfo() (id string, name string, err error) {
	luaFuncName := "bla"

	if p.Sandbox == nil {
		return "", "", ErrNilSandbox
	}
	ls := p.Sandbox.GetLuaState()
	if ls == nil {
		return "", "", ErrNilLuaState
	}

	lv := ls.Env.RawGetString(luaFuncName)
	fn, ok := luaValueToFunction(lv)
	if ok == false {
		return "", "", errors.New("lua value is not a function")
	}

	ls.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	})

	ret := ls.Get(-1)
	ls.Pop(1)

	projectTable, ok := luaValueToTable(ret)
	if ok == false {
		return "", "", errors.New("lua value is not a table")
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
