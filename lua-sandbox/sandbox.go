package sandbox

import (
	"errors"
	"os"
	"strings"

	project "github.com/docker/docker/proj"
	luajson "github.com/yuin/gopher-json"
	lua "github.com/yuin/gopher-lua"
)

// errors
var (
	errLuaStateNil      = errors.New("Lua state is nil")
	errLuaStateCreation = errors.New("Lua state creation error")
	errDockerProjectNil = errors.New("docker project is nil")
	errLuaStateReset    = errors.New("Lua state reset error")
)

// Sandbox type definition
type Sandbox struct {
	luaState      *lua.LState
	dockerProject *project.Project
}

// NewSandbox creates a new Lua Sandbox.
func NewSandbox(proj *project.Project) (*Sandbox, error) {
	var err error

	if proj == nil {
		return nil, errDockerProjectNil
	}

	// create Lua state
	luaState := lua.NewState()
	if luaState == nil {
		return nil, errLuaStateCreation
	}

	// reset Lua state to our default state
	err = resetLuaState(luaState)
	if err != nil {
		return nil, err
	}

	result := &Sandbox{
		luaState:      luaState,
		dockerProject: proj,
	}

	// populate Lua state
	err = result.populateLuaState(result.luaState, proj)
	if err != nil {
		return nil, err
	}

	// load user's project scripts
	err = result.loadDockerscripts(proj)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Exec looks for a top level function in the sandbox (args[0])
// and executes it passing remaining arguments (args[1:])
func (s *Sandbox) Exec(args []string) (found bool, err error) {
	found = false
	err = nil

	if len(args) == 0 {
		err = errors.New("at least one argument required (function name)")
		return
	}

	functionName := args[0]

	value := s.luaState.GetGlobal(functionName)
	if value == lua.LNil {
		return
	}

	fn, ok := value.(*lua.LFunction)
	if !ok {
		err = errors.New(functionName + " is not a function")
		return
	}

	// from here we consider function has been found
	found = true

	// chdir to project root dir
	projectRootDir := s.dockerProject.RootDirPath
	currentWorkingDirectory, err := os.Getwd()
	if err != nil {
		return
	}
	os.Chdir(projectRootDir)
	defer os.Chdir(currentWorkingDirectory)

	argsTbl := s.luaState.CreateTable(0, 0)
	for _, arg := range args[1:] {
		if strings.Contains(arg, " ") {
			arg = strings.Replace(arg, "\"", "\\\"", -1)
			arg = "\"" + arg + "\""
		}
		argsTbl.Append(lua.LString(arg))
	}

	err = s.luaState.CallByParam(lua.P{
		Fn:      fn,
		NRet:    0,
		Protect: true,
	}, argsTbl)
	return
}

// ContainsGlobalFunction indicates whether a function exists in the sandbox
func (s *Sandbox) ContainsGlobalFunction(name string) bool {
	value := s.luaState.GetGlobal(name)
	if value != lua.LNil {
		_, ok := value.(*lua.LFunction)
		if ok {
			return true
		}
	}
	return false
}

// doFile loads Lua file into Sandbox's Lua state
func (s *Sandbox) doFile(fpath string) (found bool, err error) {
	if s.luaState == nil {
		err = errLuaStateNil
		return
	}

	found = false

	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		return
	}

	found = true
	err = s.luaState.DoFile(fpath)
	return
}

// doString loads Lua string into Sandbox's Lua state
func (s *Sandbox) doString(script string) error {
	if s.luaState == nil {
		return errLuaStateNil
	}
	return s.luaState.DoString(script)
}

// addFunction adds an external Go-implemented Lua function to the sandbox
func (s *Sandbox) addFunction(name string, fn lua.LGFunction) error {
	if s.luaState == nil {
		return errLuaStateNil
	}
	s.luaState.Env.RawSetString(name, s.luaState.NewFunction(fn))
	return nil
}

// addString adds a Lua string to the sandbox
func (s *Sandbox) addString(name string, str string) error {
	if s.luaState == nil {
		return errLuaStateNil
	}
	s.luaState.Env.RawSetString(name, lua.LString(str))
	return nil
}

// addTable adds a lua table to the sandbox
func (s *Sandbox) addTable(name string, table *lua.LTable) error {
	if s.luaState == nil {
		return errLuaStateNil
	}
	s.luaState.Env.RawSetString(name, table)
	return nil
}

// getTable returns a top-level lua table of the sandbox
func (s *Sandbox) getTable(name string) (*lua.LTable, error) {
	if s.luaState == nil {
		return nil, errLuaStateNil
	}
	luaValue := s.luaState.Env.RawGetString(name)
	if luaValue == nil {
		return nil, errors.New("failed to get table from sandbox")
	}
	switch luaValue.Type() {
	case lua.LTNil:
		return nil, nil
	case lua.LTTable:
		table, ok := luaValue.(*lua.LTable)
		if ok == false {
			return nil, errors.New("failed to get table from sandbox")
		}
		return table, nil
	}
	return nil, errors.New("failed to get table from sandbox")
}

// msiRepresentation returns a map[string]interface{} representation
// of the sandbox
func (s *Sandbox) msiRepresentation(tbl *lua.LTable) map[string]interface{} {

	if tbl == nil {
		tbl = s.luaState.Env
	}

	msi := make(map[string]interface{})

	tbl.ForEach(func(k lua.LValue, v lua.LValue) {

		key := ""

		if kk, ok := k.(lua.LString); ok {
			key = string(kk)
		} else {
			return
		}

		if _, ok := v.(*lua.LFunction); ok {
			msi[key] = "function"
		} else if tbl, ok := v.(*lua.LTable); ok {
			msi[key] = s.msiRepresentation(tbl)
		} else if _, ok := v.(*lua.LString); ok {
			msi[key] = "string"
		}
	})

	return msi
}

////////////////////////////////////////////////////////////
///
/// Sandbox unexposed functions
///
////////////////////////////////////////////////////////////

// resetLuaState sets a Lua state to what we call our "default state".
// It removes, among other things, access the "io" table, which contains
// functions to manipulate filesystems. Apart basic functions, the available
// functions are the ones for string and table manipulation, and math functions.
// http://lua-users.org/wiki/SandBoxes
func resetLuaState(s *lua.LState) error {

	// default state of Lua sandboxes
	const defaultLuaSandbox string = `
		sandbox_env = {
			tostring = tostring,
			tonumber = tonumber,
			pairs = pairs,
			ipairs = ipairs,
			unpack = unpack,
			error = error,
			assert = assert,
			pcall = pcall,
			os = {
				clock = os.clock,
				date = os.date,
				difftime = os.difftime,
				time = os.time},
			string = {
				byte = string.byte,
				char = string.char,
				find = string.find,
				format = string.format,
				gmatch = string.gmatch,
				gsub = string.gsub,
				len = string.len,
				lower = string.lower,
				match = string.match,
				rep = string.rep,
				reverse = string.reverse,
				sub = string.sub,
				upper = string.upper},
			table = {
				insert = table.insert,
				maxn = table.maxn,
				remove = table.remove,
				sort = table.sort,
				getn = table.getn,
				concat = table.concat},
			math = {
				abs = math.abs,
				acos = math.acos,
				asin = math.asin,
				atan = math.atan,
				atan2 = math.atan2,
				ceil = math.ceil,
				cos = math.cos,
				cosh = math.cosh,
				deg = math.deg,
				exp = math.exp,
				floor = math.floor,
				fmod = math.fmod,
				frexp = math.frexp,
				huge = math.huge,
				ldexp = math.ldexp,
				log = math.log,
				log10 = math.log10,
				max = math.max,
				min = math.min,
				modf = math.modf,
				pi = math.pi,
				pow = math.pow,
				rad = math.rad,
				random = math.random,
				sin = math.sin,
				sinh = math.sinh,
				sqrt = math.sqrt,
				tan = math.tan,
				tanh = math.tanh},
		}
	`

	var err error

	// store defaults in a global named "sandbox_env"
	err = s.DoString(defaultLuaSandbox)
	if err != nil {
		return err
	}
	sandboxEnv, ok := s.GetGlobal("sandbox_env").(*lua.LTable)
	if ok == false {
		return errLuaStateReset
	}

	// remove everything that is in the Lua state environment.
	s.Env.ForEach(func(k, v lua.LValue) {
		s.Env.RawSet(k, lua.LNil)
	})
	// replace that with our default environment
	sandboxEnv.ForEach(func(k, v lua.LValue) {
		s.Env.RawSet(k, v)
	})

	// cleanup by removing sandbox_env
	s.SetGlobal("sandbox_env", lua.LNil)

	return nil
}

// populateLuaState adds functions to the Lua sandbox
func (s *Sandbox) populateLuaState(luaState *lua.LState, proj *project.Project) error {

	// require
	luaState.Env.RawSetString("require", luaState.NewFunction(s.require))

	// print
	luaState.Env.RawSetString("print", luaState.NewFunction(s.print))

	// add username() & home() to os table
	osLv := luaState.Env.RawGetString("os")
	if osTbl, ok := osLv.(*lua.LTable); ok {
		osTbl.RawSetString("username", luaState.NewFunction(s.username))
		osTbl.RawSetString("home", luaState.NewFunction(s.home))
		osTbl.RawSetString("setEnv", luaState.NewFunction(s.setEnv))
		osTbl.RawSetString("getEnv", luaState.NewFunction(s.getEnv))
	}

	// docker
	dockerLuaTable := luaState.CreateTable(0, 0)
	dockerLuaTable.RawSetString("cmd", luaState.NewFunction(s.dockerCmd))
	dockerLuaTable.RawSetString("silentCmd", luaState.NewFunction(s.dockerSilentCmd))

	// docker.container
	dockerContainerLuaTable := luaState.CreateTable(0, 0)
	dockerContainerLuaTable.RawSetString("list", luaState.NewFunction(s.dockerContainerList))
	dockerLuaTable.RawSetString("container", dockerContainerLuaTable)

	// docker.image
	dockerImageLuaTable := luaState.CreateTable(0, 0)
	dockerImageLuaTable.RawSetString("list", luaState.NewFunction(s.dockerImageList))
	dockerLuaTable.RawSetString("image", dockerImageLuaTable)

	// docker volume
	dockerVolumeLuaTable := luaState.CreateTable(0, 0)
	dockerVolumeLuaTable.RawSetString("list", luaState.NewFunction(s.dockerVolumeList))
	dockerLuaTable.RawSetString("volume", dockerVolumeLuaTable)

	// docker network
	dockerNetworkLuaTable := luaState.CreateTable(0, 0)
	dockerNetworkLuaTable.RawSetString("list", luaState.NewFunction(s.dockerNetworkList))
	dockerLuaTable.RawSetString("network", dockerNetworkLuaTable)

	// docker service
	dockerServiceLuaTable := luaState.CreateTable(0, 0)
	dockerServiceLuaTable.RawSetString("list", luaState.NewFunction(s.dockerServiceList))
	dockerLuaTable.RawSetString("service", dockerServiceLuaTable)

	// docker secret
	dockerSecretLuaTable := luaState.CreateTable(0, 0)
	dockerSecretLuaTable.RawSetString("list", luaState.NewFunction(s.dockerSecretList))
	dockerLuaTable.RawSetString("secret", dockerSecretLuaTable)

	// docker.project
	if proj != nil {
		dockerProjectLuaTable := luaState.CreateTable(0, 0)
		dockerProjectLuaTable.RawSetString("id", lua.LString(proj.Config.ID))
		dockerProjectLuaTable.RawSetString("name", lua.LString(proj.Config.Name))
		dockerProjectLuaTable.RawSetString("root", lua.LString(proj.RootDirPath))
		dockerLuaTable.RawSetString("project", dockerProjectLuaTable)
	}

	err := s.addTable("docker", dockerLuaTable)
	if err != nil {
		return err
	}

	// expose json library in the Lua sandbox
	luajson.Expose(luaState)

	return nil
}

// loadDockerscripts loads project's Dockerscripts
// dockerscript.lua from docker.project folder as well
// as user defined dockerscript (from devs directory)
func (s *Sandbox) loadDockerscripts(proj *project.Project) error {
	var err error

	// return an error if no docker project was provided
	if proj == nil {
		return errDockerProjectNil
	}

	// load docker.project/dockerscript.lua
	path, exists, err := proj.GetDockerscriptPath()
	if err != nil {
		return err
	}
	if exists {
		err = s.luaState.DoFile(path)
		if err != nil {
			return err
		}
	}

	// load user specific script
	path, exists, err = proj.GetUserDockerscriptPath()
	if err != nil {
		return err
	}
	if exists {
		err = s.luaState.DoFile(path)
		if err != nil {
			return err
		}
	}

	return nil
}

//
func (s *Sandbox) require(L *lua.LState) int {

	// retrieve string argument
	filename, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("missing string argument")
		return 0
	}

	st := lua.NewState()
	if st == nil {
		L.RaiseError("failed to import lua file [" + filename + "]")
		return 0
	}
	err = resetLuaState(st)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	err = s.populateLuaState(st, s.dockerProject)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	err = st.DoFile(filename)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	L.Push(st.Env)
	return 1
}
