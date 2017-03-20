package project

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/docker/distribution/uuid"

	projlua "github.com/docker/docker/proj/lua"
)

const (
	configFileName = "dockerproject.lua"
)

var (
	// CommandsAllowedToBeOverridden is the list of docker commands for which
	// override is allowed.
	CommandsAllowedToBeOverridden = []string{
		"build",
		"deploy",
		"export",
		"logs",
		"restart",
		"run",
		"start",
		"stats",
		"stop",
	}
)

// Project defines a Docker project
type Project struct {
	RootDir string `json:"root"`
	Name    string `json:"name"`
	ID      string `json:"id"`
}

// Init initiates a new project
func Init(dir, name string) error {
	if isProjectRoot(dir) {
		return fmt.Errorf("target directory already is the root of a Docker project")
	}

	project := &Project{Name: name, RootDir: dir, ID: uuid.Generate().String()}

	// write config file
	configFile := filepath.Join(dir, configFileName)
	sample := fmt.Sprintf(projectConfigSample, project.ID, project.Name)
	err := ioutil.WriteFile(configFile, []byte(sample), 0644)
	if err != nil {
		return err
	}

	return nil
}

// GetConfigFilePath returns absolute path to configuration file
func (p *Project) GetConfigFilePath() (path string, err error) {
	absPath := filepath.Join(p.RootDir, configFileName)
	_, err = os.Stat(absPath)
	if err == nil {
		path = configFileName
	}
	return
}

// ListCommands returns commands defined for the project.
// This function parses the main "dockerfile.lua" but also the
// <CURRENT_USER_USERNAME>-dockerfile.lua if it exists.
func (p *Project) ListCommands() ([]ProjectCommand, error) {
	// // list project commands
	// dockerscript := filepath.Join(p.DockerProjectDirPath(), "dockerscript.lua")
	// cmds, err := listCommandsForDockerscript(dockerscript)
	// if err != nil {
	// 	return nil, err
	// }
	// // list user-specific project commands
	// userDockerscriptFileName, err := getUserDockerscriptFileName()
	// if err != nil {
	// 	return nil, err
	// }
	// userDockerscript := filepath.Join(p.DockerProjectDirPath(), userDockerscriptDirName, userDockerscriptFileName)
	// userCmds, err := listCommandsForDockerscript(userDockerscript)
	// if err != nil {
	// 	return nil, err
	// }
	// // build final list (user cmds override project cmds)
	// for _, usrCmd := range userCmds {
	// 	// check if it is an override
	// 	found := false
	// 	for j, prjCmd := range cmds {
	// 		if usrCmd.Name == prjCmd.Name {
	// 			cmds[j].Description = usrCmd.Description // override description
	// 			found = true
	// 		}
	// 	}
	// 	if found == false {
	// 		cmds = append(cmds, usrCmd)
	// 	}
	// }
	// return cmds, nil
	return nil, errors.New("not implemented")
}

// CommandExists indicates whether a command has been defined in the project
func (p *Project) CommandExists(cmd string) (bool, error) {
	commands, err := p.ListCommands()
	if err != nil {
		return false, err
	}
	for _, command := range commands {
		if command.Name == cmd {
			return true, nil
		}
	}
	return false, nil
}

// Get returns project for a given path.
// The configuration file can be in a parent directory, so we have to test all
// the way up to the root directory. If no configuration file is found then
// nil,nil is returned (no error)
// TODO: gdevillele: disable top-level functions auto-exec during loading
func Load(path string) (*Project, error) {

	projectRootDirPath, err := findProjectRoot(path)
	if err != nil {
		// TODO: gdevillele: handle actual errors, for now we suppose no project is found
		return nil, nil
	}

	// config file path
	configFilePath := filepath.Join(projectRootDirPath, configFileName)

	// retrieve project id and name
	id, name, err := projlua.LoadProjectInfo(configFilePath)
	if err != nil {
		return nil, err
	}

	p := &Project{
		RootDir: projectRootDirPath,
		Name:    name,
		ID:      id,
	}

	return p, nil
}

// LoadForWd returns project for current working directory
func LoadForWd() (*Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Load(wd)
}

// IsCommandOverrideAllowed indicates whether a command is allowed to be overridden
func IsCommandOverrideAllowed(cmd string) bool {
	for _, c := range CommandsAllowedToBeOverridden {
		if c == cmd {
			return true
		}
	}
	return false
}

// findProjectRoot looks in current directory and parents until
// it finds a project config file. It then returns the parent
// of that directory, the root of the Docker project.
func findProjectRoot(path string) (projectRootPath string, err error) {
	path = filepath.Clean(path)
	for {
		b := isProjectRoot(path)
		if b {
			projectRootPath = path
			return
		}
		// break after / has been tested
		if path == filepath.Dir(path) {
			break
		}
		path = filepath.Dir(path)
	}
	err = errors.New("can't find project root directory")
	return
}

// isProjectRoot looks for a project configuration file at a given path.
func isProjectRoot(dirPath string) (found bool) {
	found = false
	configFilePath := filepath.Join(dirPath, configFileName)
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
