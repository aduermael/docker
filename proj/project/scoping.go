package project

import (
	"fmt"
	"regexp"
	"strings"
)

// IsInProject tells if we are in the context of a Docker project
func IsInProject() bool {
	return CurrentProject != nil
}

// GlobalToScopedContainerName scopes container names ("projectid_db" -> "db")
func GlobalToScopedContainerName(globalName string) (scopedName string) {
	if CurrentProject == nil {
		return globalName
	}
	// we are in the context of a project
	projectID := CurrentProject.ID()
	namePrefix := projectID + "_"
	return strings.TrimPrefix(globalName, namePrefix)
}

// ScopedToGlobalContainerName ... ("db" -> "projectid_db")
func ScopedToGlobalContainerName(scopedName string) (global string) {
	if CurrentProject == nil {
		return scopedName
	}
	// we are in the context of a project
	projectID := CurrentProject.ID()
	namePrefix := projectID + "_"
	return namePrefix + scopedName
}

// CanBeContainerID ...
func CanBeContainerID(s string) bool {
	reg, err := regexp.Compile("^[0-9a-f]+$")
	if err != nil {
		fmt.Println("ERROR COMPILING REGEXP")
		return false
	}
	return reg.MatchString(s)
}
