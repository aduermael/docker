package project

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/distribution/uuid"
)

// Init initiates a new project
func Init(dir, name string) error {
	if isProjectRoot(dir) {
		return fmt.Errorf("target directory already is the root of a Docker project")
	}

	projectName := name
	projectID := uuid.Generate().String()

	// write config file
	configFile := filepath.Join(dir, ConfigFileName)
	sample := fmt.Sprintf(projectConfigSample, projectID, projectName)
	err := ioutil.WriteFile(configFile, []byte(sample), 0644)
	return err
}

// FindProjectRoot looks in current directory and parents until
// it finds a project config file. It then returns the parent
// of that directory, the root of the Docker project.
func FindProjectRoot(path string) (projectRootPath string, err error) {
	path = filepath.Clean(path)
	for {
		if isProjectRoot(path) {
			return path, nil
		}
		// break after / has been tested
		if path == filepath.Dir(path) {
			break
		}
		path = filepath.Dir(path)
	}
	return "", errors.New("can't find project root directory")
}

// UNEXPOSED

const projectConfigSample = `-- Docker project configuration

project = {
	"id" = "%s",
	"name" = "%s"
}

project.tasks = {
	"up" = up
}

-- functions

function up(args)
	print("up test")
end
`

// isProjectRoot looks for a project configuration file at a given path.
func isProjectRoot(dirPath string) (found bool) {
	found = false
	configFilePath := filepath.Join(dirPath, ConfigFileName)
	fileInfo, err := os.Stat(configFilePath)
	if os.IsNotExist(err) {
		return
	}
	if fileInfo.IsDir() {
		return
	}
	found = true
	return
}
