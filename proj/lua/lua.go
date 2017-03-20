package lua

import (
	"errors"

	lua "github.com/yuin/gopher-lua"
)

// LoadProjectInfo ...
func LoadProjectInfo(projectConfigFilePath string) (id string, name string, err error) {
	pLuaState := lua.NewState()
	if pLuaState == nil {
		return "", "", errors.New("failed to create lua state")
	}
	emptyLuaState(pLuaState)

	err = pLuaState.DoFile(projectConfigFilePath)
	if err != nil {
		return "", "", err
	}

	projectTable, err := getTable(pLuaState, "project")
	if err != nil {
		return "", "", err
	}
	id, err = getStringFromTable(projectTable, "id")
	if err != nil {
		return "", "", err
	}
	name, err = getStringFromTable(projectTable, "name")
	if err != nil {
		return "", "", err
	}
	return id, name, nil
}

func emptyLuaState(ls *lua.LState) {
	// remove everything that is in the Lua state environment.
	ls.Env.ForEach(func(k, v lua.LValue) {
		ls.Env.RawSet(k, lua.LNil)
	})
}

func getTable(ls *lua.LState, name string) (*lua.LTable, error) {
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

func getStringFromTable(lt *lua.LTable, name string) (string, error) {
	if lt == nil {
		return "", errors.New("Lua table is nil")
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

//
//
//
//
//
//
//
//
//
//
//
//
