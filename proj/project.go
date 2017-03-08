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
)

// Project defines a Docker project
type Project struct {
	Config Config
	// path of *.dockerproj's parent directory
	RootDirPath string
}

// LuaCommand describes a project custom command pointing to a Lua function
type LuaCommand struct {
	FunctionName string `yaml:"function"`
	Description  string `yaml:"description"`
}

// DockerprojDirPath returns the path of the *.dockerproj directory
func (p *Project) DockerprojDirPath() string {
	return filepath.Join(p.RootDirPath, p.Config.Name+".dockerproj")
}

// ListCustomCommands parses the docker-commands.yaml file
func (p *Project) ListCustomCommands() (map[string]LuaCommand, error) {
	var err error
	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), "docker-commands.yaml")
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

// HasDockerCommandsFile indicates whether docker-commands.yaml exists
func (p *Project) HasDockerCommandsFile() bool {
	var err error
	dockerCmdsFilePath := filepath.Join(p.DockerprojDirPath(), "docker-commands.yaml")
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
	projectDir := filepath.Join(dir, name+".dockerproj")
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
		// install docker-commands.yaml sample
		dockerCommands := filepath.Join(projectDir, "docker-commands.yaml")
		if err := ioutil.WriteFile(
			dockerCommands,
			[]byte(dockerCommandsSample),
			0644); err != nil {
			return err
		}
		// install dockerscript.lua sample
		scriptedCommands := filepath.Join(projectDir, "dockerscript.lua")
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
// the .dockerproj folder can be in a parent
// folder, so we have to test all the way up
// to the root folder
// If we can't find any .dockerproj folder,
// then nil,nil is returned (no error)
func Get(path string) (*Project, error) {
	rootDirPath, projectDirName, err := findProjectRoot(path)
	if err != nil {
		// TODO: handle actual errors, for now we suppose no project is found
		return nil, nil
	}
	project, err := load(rootDirPath, projectDirName)
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
// contains a .dockerproj directory, and that
// one needs to contains a config.json file
func load(projectRootDirPath, projectDirName string) (*Project, error) {
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
// it finds a *.dockerproj directory. It then returns the parent
// of that directory, the root of the Docker project.
func findProjectRoot(path string) (projectRootPath string, projectDirName string, err error) {
	path = filepath.Clean(path)

	for {
		var found bool
		found, projectDirName, err = isProjectRoot(path)
		if err != nil {
			return
		}
		if found {
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

// isProjectRoot looks for a *.dockerproj directory at a given path.
// dirPath must exist and must be the path of a directory.
// For now, if multiple *.dockerproj directories are found, an error is returned.
func isProjectRoot(dirPath string) (found bool, projectDirName string, err error) {
	found = false
	projectDirName = ""

	// test if dirPath exists and check that it is a path to a directory
	var fileInfo os.FileInfo
	fileInfo, err = os.Stat(dirPath)
	if os.IsNotExist(err) {
		err = errors.New("file not found: " + dirPath)
		return
	}
	// TODO: gdevillele: maybe we should follow links here...
	if fileInfo.IsDir() == false {
		err = errors.New("file is not a directory: " + dirPath)
		return
	}

	// look for a *.dockerproj directory in the dirPath directory
	var childrenFiles []os.FileInfo
	childrenFiles, err = ioutil.ReadDir(dirPath)
	if err != nil {
		return
	}

	for _, childFile := range childrenFiles {
		if childFile.IsDir() && filepath.Ext(childFile.Name()) == ".dockerproj" {
			if len(projectDirName) == 0 {
				projectDirName = childFile.Name()
			} else {
				err = errors.New("mutliple docker projects found")
				return
			}
		}
	}

	if len(projectDirName) > 0 {
		// a *.dockerproj directory has been found
		found = true
		err = nil
		return
	}
	// no *.dockerproj directory has been found
	return
}
