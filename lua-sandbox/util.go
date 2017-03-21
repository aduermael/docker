package sandbox

import (
	"errors"

	lua "github.com/yuin/gopher-lua"
)

// PopStringParam ...
func PopStringParam(L *lua.LState) (string, bool, error) {
	return popStringParam(L)
}

// PopBoolParam ...
func PopBoolParam(L *lua.LState) (bool, bool, error) {
	return popBoolParam(L)
}

// popStringParam gets the next argument and makes sure it is a string.
// If there is a next argument but it is not a string, an error is returned.
// If there isn't any next argument, no error is returned, but the second
// return value will be false.
// (This is useful in the case of optional parameters)
func popStringParam(L *lua.LState) (string, bool, error) {
	top := L.GetTop()
	if top > 0 {

		keeper := make([]lua.LValue, top-1)
		var lv lua.LValue

		j := 0
		for i := -top; i < 0; i++ {
			if i == -top {
				lv = L.Get(i)
			} else {
				keeper[j] = L.Get(i)
				j++
			}
		}

		L.Pop(top)

		for _, lvKept := range keeper {
			L.Push(lvKept)
		}

		if lv == lua.LNil {
			return "", true, errors.New("parameter is not a string")
		}

		if str, ok := lv.(lua.LString); ok {
			return string(str), true, nil
		}
		return "", true, errors.New("parameter is not a string")
	}

	return "", false, nil
}

// popBoolParam gets the next argument and makes sure it is a boolean.
// If there is a next argument but it is not a boolean, an error is returned.
// If there isn't any next argument, no error is returned, but the second
// return value will be false.
// (This is useful in the case of optional parameters)
func popBoolParam(L *lua.LState) (bool, bool, error) {
	top := L.GetTop()
	if top > 0 {

		keeper := make([]lua.LValue, top-1)
		var lv lua.LValue

		j := 0
		for i := -top; i < 0; i++ {
			if i == -top {
				lv = L.Get(i)
			} else {
				keeper[j] = L.Get(i)
				j++
			}
		}

		L.Pop(top)

		for _, lvKept := range keeper {
			L.Push(lvKept)
		}

		if lv == lua.LNil {
			return false, true, errors.New("parameter is not a boolean")
		}

		if _, ok := lv.(lua.LBool); ok {
			return lua.LVAsBool(lv), true, nil
		}
		return false, true, errors.New("parameter is not a boolean")
	}

	return false, false, nil
}
