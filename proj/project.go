package project

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	projectuser "github.com/docker/docker/proj/user"
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
	Config Config
	// path of docker.project's parent directory
	RootDirPath string
}

// Config defines the configuration of a docker project
type Config struct {
	Name string `json:"name"`
	ID   string `json:"id"`
}

// Init initiates a new project
func Init(dir, name string) error {
	if isProjectRoot(dir) {
		return fmt.Errorf("target directory already is the root of a Docker project")
	}

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
		if err = ioutil.WriteFile(
			scriptedCommands,
			[]byte(dockerscriptSample),
			0644); err != nil {
			return err
		}

		// create users directory with USERNAME-dockerscript.lua sample
		err = createUserDockerscript(projectDir)
		if err != nil {
			return err
		}
	}

	return nil
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

// CreateDockerscriptForUser creates a docker.project/dev/USERNAME-dockerscript.lua
func (p *Project) CreateDockerscriptForUser() error {
	return createUserDockerscript(p.DockerProjectDirPath())
}

// ListCommands returns commands defined for the project.
// This function parses the main "dockerfile.lua" but also the
// <CURRENT_USER_USERNAME>-dockerfile.lua if it exists.
func (p *Project) ListCommands() ([]ProjectCommand, error) {
	// list project commands
	dockerscript := filepath.Join(p.DockerProjectDirPath(), "dockerscript.lua")
	cmds, err := listCommandsForDockerscript(dockerscript)
	if err != nil {
		return nil, err
	}
	// list user-specific project commands
	userDockerscriptFileName, err := getUserDockerscriptFileName()
	if err != nil {
		return nil, err
	}
	userDockerscript := filepath.Join(p.DockerProjectDirPath(), userDockerscriptDirName, userDockerscriptFileName)
	userCmds, err := listCommandsForDockerscript(userDockerscript)
	if err != nil {
		return nil, err
	}
	// build final list (user cmds override project cmds)
	for _, usrCmd := range userCmds {
		// check if it is an override
		found := false
		for j, prjCmd := range cmds {
			if usrCmd.Name == prjCmd.Name {
				cmds[j].Description = usrCmd.Description // override description
				found = true
			}
		}
		if found == false {
			cmds = append(cmds, usrCmd)
		}
	}
	return cmds, nil
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

// GetDockerscriptPath returns the relative path of project's dockerscript.lua
// (relative to project's root directory)
func (p *Project) GetDockerscriptPath() (path string, exists bool, err error) {
	path = filepath.Join(p.DockerProjectDirPath(), dockerscriptFileName)
	var f os.FileInfo
	f, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
			return
		}
		return
	}
	if f.IsDir() {
		err = fmt.Errorf(dockerscriptFileName + " is a directory")
		return
	}
	exists = true
	path = filepath.Join(projectDirName, dockerscriptFileName)
	return
}

// GetUserDockerscriptPath returns relative path where current user script should be
// stored. It also returns a boolean to indicate whether the file exists or not.
// (relative to project's root directory)
func (p *Project) GetUserDockerscriptPath() (path string, exists bool, err error) {
	// get current user's username
	var username string
	username, err = projectuser.GetUsername()
	if err != nil {
		return "", false, err
	}

	fileName := fmt.Sprintf(userDockerscriptFileName, username)
	path = filepath.Join(p.DockerProjectDirPath(), userDockerscriptDirName, fileName)

	var f os.FileInfo
	f, err = os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			err = nil
			return
		}
		return
	}
	if f.IsDir() {
		err = fmt.Errorf(fileName + " is a directory")
		return
	}
	exists = true
	path = filepath.Join(projectDirName, userDockerscriptDirName, fileName)
	return
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

// getUserDockerscriptFileName returns the name of the current user's
// dockerscript, not matter if the file actually exists or not.
// The file name is <USERNAME>-dockerscript.lua
func getUserDockerscriptFileName() (string, error) {
	username, err := projectuser.GetUsername()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(userDockerscriptFileName, username), nil
}

// createUserDockerscript creates a user-specific dockerscript for the current user.
func createUserDockerscript(dockerProjectDirPath string) error {
	// if the "no sample" env var is set, we do nothing
	projectNoSampleEnvVarValue := os.Getenv(envVarDockerProjectNoSample)
	if projectNoSampleEnvVarValue == "1" {
		return nil
	}

	usersDir := filepath.Join(dockerProjectDirPath, userDockerscriptDirName)
	// check if users directory exists
	usersDirFi, err := os.Stat(usersDir)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(usersDir, 0777); err != nil {
				return err
			}
		} else {
			return err
		}
	}
	if usersDirFi != nil && usersDirFi.IsDir() == false {
		return errors.New("docker.project/users exists but is not a directory")
	}

	fileName, err := getUserDockerscriptFileName()
	if err != nil {
		return err
	}
	userScriptedCommands := filepath.Join(usersDir, fileName)

	// check if users/USERNAME-dockerscript.lua exists
	_, err = os.Stat(userScriptedCommands)
	if err != nil {
		if os.IsNotExist(err) {
			err = ioutil.WriteFile(
				userScriptedCommands,
				[]byte(userDockerscriptSample),
				0644)
			return err
		}
		return err
	}
	return nil
}

// listCommandsForDockerscript manually parses a dockerscript (lua file)
// and returns a list of top-level functions and their description.
func listCommandsForDockerscript(path string) ([]ProjectCommand, error) {
	result := make([]ProjectCommand, 0)

	fileInfo, err := os.Stat(path)
	if err != nil {
		// if dockerscript doesn't exists we return an empty array
		if os.IsNotExist(err) {
			return result, nil
		}
		return nil, err
	}
	if fileInfo.IsDir() {
		return nil, errors.New("path points to a directory")
	}
	// read dockerscript
	fileBytes, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	fileStringReader := bufio.NewReader(strings.NewReader(string(fileBytes)))
	// we store the previous line content to look for a comment in the
	// event of a function found on the current line.
	previousLine := ""
	for {
		line, err := fileStringReader.ReadString(byte('\n'))
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "function ") {
			trimmedLine := strings.TrimPrefix(line, "function ")
			functionName := (strings.Split(trimmedLine, "("))[0]
			functionName = strings.TrimSpace(functionName)
			// check for description on the previous line
			functionDescription := ""
			if len(previousLine) > 0 && strings.HasPrefix(previousLine, "--") {
				trimmedLine = strings.TrimPrefix(previousLine, "--")
				functionDescription = strings.TrimSpace(trimmedLine)
			}
			result = append(result, ProjectCommand{Name: functionName, Description: functionDescription})
		}
		previousLine = line
	}
	return result, nil
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
