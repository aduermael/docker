package sandbox

import (
	"os"

	user "github.com/docker/docker/pkg/idtools/user"
	lua "github.com/yuin/gopher-lua"
)

// returns current user's username
func luaUsername(L *lua.LState) int {
	username, err := user.GetUsername()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	L.Push(lua.LString(username))
	return 1
}

// returns current user's home directory path
func luaHome(L *lua.LState) int {
	home, err := user.GetHomeDirPath()
	if err != nil {
		L.RaiseError(err.Error())
		return 0
	}
	L.Push(lua.LString(home))
	return 1
}

// sets environment variable
func luaSetEnv(L *lua.LState) int {
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
func luaGetEnv(L *lua.LState) int {
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
