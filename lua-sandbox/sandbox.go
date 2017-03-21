package sandbox

import (
	"errors"
	"os"

	luajson "github.com/yuin/gopher-json"
	lua "github.com/yuin/gopher-lua"
)

// errors
var (
	errLuaStateNil      = errors.New("Lua state is nil")
	errLuaStateCreation = errors.New("Lua state creation error")
	errLuaStateReset    = errors.New("Lua state reset error")
)

// Sandbox type definition
type Sandbox struct {
	luaState *lua.LState
}

// GetLuaState returns a pointer on the sandbox' Lua state
func (s *Sandbox) GetLuaState() *lua.LState {
	return s.luaState
}

// CreateSandbox creates a basic sandbox
func CreateSandbox() (*Sandbox, error) {
	var err error

	// create Lua state
	pLuaState := lua.NewState()
	if pLuaState == nil {
		return nil, errLuaStateCreation
	}

	// reset Lua state to our default state (minimal Lua sandbox)
	err = resetLuaState(pLuaState)
	if err != nil {
		return nil, err
	}

	// add Lua functions to the sandbox

	// io
	pLuaState.Env.RawSetString("print", pLuaState.NewFunction(luaPrint))

	// os
	osLuaTable := pLuaState.CreateTable(0, 0)
	osLuaTable.RawSetString("username", pLuaState.NewFunction(luaUsername))
	osLuaTable.RawSetString("home", pLuaState.NewFunction(luaHome))
	osLuaTable.RawSetString("setEnv", pLuaState.NewFunction(luaSetEnv))
	osLuaTable.RawSetString("getEnv", pLuaState.NewFunction(luaGetEnv))
	pLuaState.Env.RawSetString("os", osLuaTable)

	// expose json library in the Lua sandbox
	luajson.Expose(pLuaState)

	result := &Sandbox{
		luaState: pLuaState,
	}

	return result, nil
}

// Exec looks for a top level function in the sandbox (args[0])
// and executes it passing remaining arguments (args[1:])
// func (s *Sandbox) Exec(wd string, function string, args []string) (found bool, err error) {
func (s *Sandbox) Exec(args []string) (found bool, err error) {
	// found = false
	// err = nil

	// if len(args) == 0 {
	// 	err = errors.New("at least one argument required (function name)")
	// 	return
	// }

	// functionName := args[0]

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
	return false, errors.New("NOT IMPLEMENTED")
}

// TODO
// func (s *Sandbox) Find(name string) (...) { // find symbol
// }
// func (s *Sandbox) FindFunc(name string) ...
// value := s.luaState.GetGlobal(name)
// if value != lua.LNil {
// 	_, ok := value.(*lua.LFunction)
// 	if ok {
// 		return true
// 	}
// }
// return false

// DoFile loads Lua file into Sandbox's Lua state
func (s *Sandbox) DoFile(fpath string) (found bool, err error) {
	if s.luaState == nil {
		err = errLuaStateNil
		return
	}

	_, err = os.Stat(fpath)
	if os.IsNotExist(err) {
		return false, nil
	}

	return true, s.luaState.DoFile(fpath)
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

// AddTableToLuaState adds a lua table to the sandbox
func AddTableToLuaState(table *lua.LTable, state *lua.LState, name string) error {
	if state == nil {
		return errLuaStateNil
	}
	state.Env.RawSetString(name, table)
	return nil
}

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
