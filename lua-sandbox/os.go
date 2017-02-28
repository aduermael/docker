package sandbox

import (
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
