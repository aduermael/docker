package project

import (
	"errors"

	lua "github.com/yuin/gopher-lua"
)

var (
	ErrNilLuaTable = errors.New("lua table is nil")
)

// get top-level table from Lua state
// TODO: gdevillele: check whether it works for local tables or just for globals
func getTableFromState(ls *lua.LState, name string) (*lua.LTable, error) {
	if ls == nil {
		return nil, errors.New("Lua state is nil")
	}
	luaValue := ls.Env.RawGetString(name)
	if luaValue == nil {
		return nil, errors.New("failed to get table from Lua state")
	}

	switch luaValue.Type() {
	case lua.LTNil:
		return nil, nil
	case lua.LTTable:
		table, ok := luaValue.(*lua.LTable)
		if ok == false {
			return nil, errors.New("failed to get table from Lua state")
		}
		return table, nil
	}
	return nil, errors.New("failed to get table from Lua state")
}

// getStringFromTable ...
func getStringFromTable(lt *lua.LTable, name string) (string, error) {
	if lt == nil {
		return "", ErrNilLuaTable
	}
	luaValue := lt.RawGetString(name)
	if luaValue == nil {
		return "", errors.New("failed to get string from Lua table")
	}
	switch luaValue.Type() {
	case lua.LTNil:
		return "", errors.New("failed to get string from Lua table")
	case lua.LTString:
		str, ok := luaValue.(lua.LString)
		if ok == false {
			return "", errors.New("failed to get string from Lua table")
		}
		return string(str), nil
	}
	return "", errors.New("failed to get string from Lua table")
}

// getTableFromTable ...
func getTableFromTable(lt *lua.LTable, name string) (*lua.LTable, error) {
	errMessage := errors.New("failed to get Lua table from Lua table")

	if lt == nil {
		return nil, ErrNilLuaTable
	}
	lv := lt.RawGetString(name)
	if lv == nil {
		return nil, errMessage
	}
	if lv.Type() != lua.LTTable {
		return nil, errors.New("lua value is not a table")
	}
	childTable, ok := lv.(*lua.LTable)
	if ok == false {
		return nil, errors.New("lua value is not a table")
	}
	return childTable, nil
}

//
//
// TYPE ASSERTION
//
//

func luaValueIsString(lv lua.LValue) bool {
	_, ok := lv.(lua.LString)
	return ok
}

func luaValueIsFunction(lv lua.LValue) bool {
	_, ok := lv.(*lua.LFunction)
	return ok
}

func luaValueIsTable(lv lua.LValue) bool {
	_, ok := lv.(*lua.LTable)
	return ok
}

func luaValueIsNumber(lv lua.LValue) bool {
	_, ok := lv.(lua.LNumber)
	return ok
}

func luaValueIsNil(lv lua.LValue) bool {
	return lv == lua.LNil
}

//
//
// TYPE CONVERSION
//
//

func luaValueToString(lv lua.LValue) (lua.LString, bool) {
	ret, ok := lv.(lua.LString)
	return ret, ok
}

func luaValueToTable(lv lua.LValue) (*lua.LTable, bool) {
	ret, ok := lv.(*lua.LTable)
	return ret, ok
}

func luaValueToFunction(lv lua.LValue) (*lua.LFunction, bool) {
	ret, ok := lv.(*lua.LFunction)
	return ret, ok
}

//
//
// TYPE VALIDATION
//
//

func luaTableIsArray(lt *lua.LTable) bool {
	onlyIntKeys := true
	keyCount := 0
	lt.ForEach(func(k, v lua.LValue) {
		keyCount++
		if luaValueIsNumber(k) == false {
			onlyIntKeys = false
		}
	})
	if onlyIntKeys == false {
		return false
	}
	if keyCount != lt.Len() {
		return false
	}
	return true
}

func luaTableIsMap(lt *lua.LTable) bool {
	return lt.Len() == 0
}
