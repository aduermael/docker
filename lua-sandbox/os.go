package sandbox

import (
	"os"
	"os/user"

	lua "github.com/yuin/gopher-lua"
)

// returns current user's username
func (s *Sandbox) username(L *lua.LState) int {
	usr, err := user.Current()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	L.Push(lua.LString(usr.Username))
	return 1
}

// returns current user's home directory path
func (s *Sandbox) home(L *lua.LState) int {
	usr, err := user.Current()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	L.Push(lua.LString(usr.HomeDir))
	return 1
}

// sets environment variable
func (s *Sandbox) setEnv(L *lua.LState) int {
	// retrieve string argument
	key, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("can't get env value for empty key")
		return 0
	}

	value, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("value is empty")
		return 0
	}

	err = os.Setenv(key, value)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}

	return 0
}

// returns environment for given key
func (s *Sandbox) getEnv(L *lua.LState) int {
	// retrieve string argument
	key, found, err := popStringParam(L)
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	if !found {
		L.RaiseError("can't get env value for empty key")
		return 0
	}
	value := os.Getenv(key)
	L.Push(lua.LString(value))
	return 1
}
