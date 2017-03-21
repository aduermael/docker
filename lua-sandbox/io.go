package sandbox

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// luaPrint takes one string argument and write its content into the process' stdout stream.
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

	// fmt.Println(args)
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
