package project

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
)

const (
	// name of the project config directory
	projectDirName = "docker.project"
	// project config file name
	projectConfigFileName = "config.json"
	// name of the main dockerscript file
	dockerscriptFileName = "dockerscript.lua"
	// name of user specific dockerscripts (%s is replaced by the username)
	userDockerScriptFileName = "%s-dockerscript.lua"
	// directory wher to put user specific scripts
	userDockerScriptDirName = "devs"
	// env var that can prevent `docker init` from dumping samples
	envVarDockerProjectNoSample = "DOCKER_PROJECT_NO_SAMPLE"

	// YAML related
	// // name of the file defining project tasks and env variables
	// projectFileName = "docker.yml"
	// // similar to docker.yml, can be used to override or define project
	// // tasks or env variables specific to a user
	// projectUserFileName = "user.yml"
)

var (
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
	Config Config
	// path of docker.project's parent directory
	RootDirPath string
}

// DockerProjectDirPath returns the path of the docker.project directory
func (p *Project) DockerProjectDirPath() string {

	return filepath.Join(p.RootDirPath, projectDirName)
}

// DockerscriptFileName returns the name of the default dockerscript file to be
// loaded by the Lua sandbox
func (p *Project) DockerscriptFileName() string {
	return dockerscriptFileName
}

// Config defines the configuration of a docker project
type Config struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Init initiates a new project
func Init(dir, name string) error {
	// create docker.project directory
	projectDir := filepath.Join(dir, projectDirName)
	if err := os.MkdirAll(projectDir, 0777); err != nil {
		return err
	}
	config := Config{Name: name, ID: ""}

	// create project id (random hash)
	data := make([]byte, 64)
	_, err := rand.Read(data)
	if err != nil {
		return err
	}
	config.ID = fmt.Sprintf("%x", sha256.Sum256(data))

	// write config.json
	jsonBytes, err := json.Marshal(&config)
	if err != nil {
		return err
	}
	configFile := filepath.Join(projectDir, projectConfigFileName)
	err = ioutil.WriteFile(configFile, jsonBytes, 0644)
	if err != nil {
		return err
	}

	// install sample files
	projectNoSampleEnvVarValue := os.Getenv(envVarDockerProjectNoSample)
	// we install a sample except if env var value is "1".
	if projectNoSampleEnvVarValue != "1" {
		// YAML related
		// // install docker.yml sample
		// dockerCommands := filepath.Join(projectDir, projectFileName)
		// if err := ioutil.WriteFile(
		// 	dockerCommands,
		// 	[]byte(dockerCommandsSample),
		// 	0644); err != nil {
		// 	return err
		// }

		// install dockerscript.lua sample
		scriptedCommands := filepath.Join(projectDir, dockerscriptFileName)
		if err := ioutil.WriteFile(
			scriptedCommands,
			[]byte(dockerscriptSample),
			0644); err != nil {
			return err
		}

		// TODO: install user specific dockerscript in devs directory
	}

	return nil
}

// ListCustomCommands returns commands defined for the project
func (p *Project) ListCustomCommands() (map[string]ProjectCommand, error) {
	// TODO: implement
	result := make(map[string]ProjectCommand)
	return result, nil
}

func (p *Project) CommandExists(cmd string) (bool, error) {
	commands, err := p.ListCustomCommands()
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

// Get returns project for given path
// the docker.project folder can be in a parent
// folder, so we have to test all the way up
// to the root folder
// If we can't find any docker.project folder,
// then nil,nil is returned (no error)
func Get(path string) (*Project, error) {
	rootDirPath, err := findProjectRoot(path)
	if err != nil {
		// TODO: handle actual errors, for now we suppose no project is found
		return nil, nil
	}
	project, err := load(rootDirPath)
	if err != nil {
		return nil, err
	}
	return project, nil
}

// GetForWd returns project for current working directory
func GetForWd() (*Project, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return Get(wd)
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

// Load loads a project at the given path
// The path needs to point to a directory that
// contains a docker.project directory, and that
// one needs to contains a configuration file
func load(projectRootDirPath string) (*Project, error) {
	configFile := filepath.Join(projectRootDirPath, projectDirName, projectConfigFileName)
	jsonBytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, err
	}
	var config Config
	err = json.Unmarshal(jsonBytes, &config)
	if err != nil {
		return nil, err
	}
	return &Project{Config: config, RootDirPath: projectRootDirPath}, nil
}

// findProjectRoot looks in current directory and parents until
// it finds a docker.project directory. It then returns the parent
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

// isProjectRoot looks for a docker.project directory at a given path.
// dirPath must exist and must be the path of a directory.
func isProjectRoot(dirPath string) (found bool) {
	found = false
	projectDirPath := filepath.Join(dirPath, projectDirName)
	// test if dirPath exists and check that it is a path to a directory
	fileInfo, err := os.Stat(projectDirPath)
	if os.IsNotExist(err) {
		return
	}
	if fileInfo.IsDir() == false {
		return
	}
	found = true
	return
}

// YAML related
// // LuaCommand describes a project custom command pointing to a Lua function
// type LuaCommand struct {
// 	FunctionName string `yaml:"function"`
// 	Description  string `yaml:"description"`
// }

// // ListCustomCommands parses the docker.yml file
// // TODO: consider project user file
// func (p *Project) ListCustomCommands() (map[string]LuaCommand, error) {
// 	var err error
// 	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), projectFileName)
// 	if _, err = os.Stat(dockerCmdsFilePath); err != nil {
// 		return nil, err
// 	}
// 	dockerCmdsYamlBytes, err := ioutil.ReadFile(dockerCmdsFilePath)
// 	if err != nil {
// 		return nil, err
// 	}
// 	result := make(map[string]LuaCommand)
// 	err = yaml.Unmarshal(dockerCmdsYamlBytes, result)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return result, nil
// }

// // HasProjectFile indicates whether docker.yml exists
// // TODO: check for both projectFileName & projectUserFileName
// func (p *Project) HasProjectFile() bool {
// 	var err error
// 	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), projectFileName)
// 	_, err = os.Stat(dockerCmdsFilePath)
// 	return err == nil
// }
