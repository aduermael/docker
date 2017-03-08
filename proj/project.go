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

	yaml "gopkg.in/yaml.v2"
)

const (
	envVarDockerProjectNoSample = "DOCKER_PROJECT_NO_SAMPLE"
	// name of the file defining project commands
	customCommandsFileName = "docker.yml"
	// name of the main dockerscript file
	dockerscriptFileName = "dockerscript.lua"

	projectDirName = "docker.project"
)

// Project defines a Docker project
type Project struct {
	Config Config
	// path of docker.project's parent directory
	RootDirPath string
}

// LuaCommand describes a project custom command pointing to a Lua function
type LuaCommand struct {
	FunctionName string `yaml:"function"`
	Description  string `yaml:"description"`
}

// DockerprojDirPath returns the path of the *.dockerproj directory
func (p *Project) DockerprojDirPath() string {
	return filepath.Join(p.RootDirPath, projectDirName)
}

// DockerscriptFileName returns the name of the dockerscript file to be loaded by the Lua sandbox
func (p *Project) DockerscriptFileName() string {
	return dockerscriptFileName
}

// ListCustomCommands parses the docker.yaml file
func (p *Project) ListCustomCommands() (map[string]LuaCommand, error) {
	var err error
	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), customCommandsFileName)
	if _, err = os.Stat(dockerCmdsFilePath); err != nil {
		return nil, err
	}
	dockerCmdsYamlBytes, err := ioutil.ReadFile(dockerCmdsFilePath)
	if err != nil {
		return nil, err
	}
	result := make(map[string]LuaCommand)
	err = yaml.Unmarshal(dockerCmdsYamlBytes, result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// HasDockerCommandsFile indicates whether docker.yaml exists
func (p *Project) HasDockerCommandsFile() bool {
	var err error
	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), customCommandsFileName)
	_, err = os.Stat(dockerCmdsFilePath)
	return err == nil
}

// Config defines the configuration of a docker project
type Config struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Init initiates a new project
func Init(dir, name string) error {
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

	jsonBytes, err := json.Marshal(&config)
	if err != nil {
		return err
	}

	configFile := filepath.Join(projectDir, "config.json")
	err = ioutil.WriteFile(configFile, jsonBytes, 0644)
	if err != nil {
		return err
	}

	// create default dockerscript.lua
	projectNoSampleEnvVarValue := os.Getenv(envVarDockerProjectNoSample)
	// we install a sample except if env var value is "1".
	if projectNoSampleEnvVarValue != "1" {
		// install docker.yaml sample
		dockerCommands := filepath.Join(projectDir, customCommandsFileName)
		if err := ioutil.WriteFile(
			dockerCommands,
			[]byte(dockerCommandsSample),
			0644); err != nil {
			return err
		}
		// install dockerscript.lua sample
		scriptedCommands := filepath.Join(projectDir, dockerscriptFileName)
		if err := ioutil.WriteFile(
			scriptedCommands,
			[]byte(dockerscriptSample),
			0644); err != nil {
			return err
		}
	}

	return nil
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

// Load loads a project at the given path
// The path needs to point to a directory that
// contains a docker.project directory, and that
// one needs to contains a config.json file
func load(projectRootDirPath string) (*Project, error) {
	configFile := filepath.Join(projectRootDirPath, projectDirName, "config.json")
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
