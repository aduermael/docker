package project

import (
	lua "github.com/yuin/gopher-lua"
)

var (
	CurrentProject Project = nil
)

const (
	ConfigFileName = "dockerproject.lua"
)

// Project is the interface used by the cli package
type Project interface {
	RootDir() string     // returns the project root directory's path
	ID() string          // returns project id
	Name() string        // return project name
	Commands() []Command // returns list of custom commands
}

type Command struct {
	Name             string
	ShortDescription string
	Description      string
	Function         *lua.LFunction
}
