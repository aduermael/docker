package project

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/distribution/uuid"
)

// Init initiates a new project
func Init(dir, name string) error {
	if IsProjectRoot(dir) {
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

// isProjectRoot looks for a project configuration file at a given path.
func IsProjectRoot(dirPath string) (found bool) {
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
