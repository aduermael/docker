package sandbox

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

func luaPrint(L *lua.LState) int {

	argc := L.GetTop() // get number of arguments
	if argc <= 0 {
		return 0 // do nothing and return
	}

	args := make([]lua.LValue, argc)
	for i := -argc; i < 0; i++ {
		args[i+argc] = L.Get(i)
	}
	L.Pop(argc)

	for i, arg := range args {
		fmt.Printf("%s", arg.String())
		if i < len(args)-1 { // for all but last element
			fmt.Printf(" ")
		} else {
			fmt.Printf("\n")
		}
	}
	return 0
}

func luaPrintf(L *lua.LState) int {

	argc := L.GetTop() // get number of arguments
	if argc <= 0 {
		return 0 // do nothing and return
	}

	args := make([]lua.LValue, argc)
	for i := -argc; i < 0; i++ {
		args[i+argc] = L.Get(i)
	}
	L.Pop(argc)

	format := ""
	params := make([]interface{}, 0)

	for i, arg := range args {
		if i == 0 {
			format = arg.String()
			continue
		}

		if luaStr, ok := arg.(lua.LString); ok {
			params = append(params, luaStr.String())
		} else if luaBool, ok := arg.(lua.LBool); ok {
			params = append(params, luaBool == lua.LTrue)
		} else if luaNumber, ok := arg.(lua.LNumber); ok {
			params = append(params, float64(luaNumber))
			// TODO: convert to expected type depending on format
		} else {
			// not supporting LFunction, LUserData, LState, LTable & LChannel
			params = append(params, nil)
		}
	}

	fmt.Printf(format, params...)

	return 0
}
