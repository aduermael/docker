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
    id = "%s",
    name = "%s",
    root = project.root,
}

project.tasks = {
    -- using anonymous function because up() is not defined yet at this point
    up = {function() up() end, 'equivalent to docker-compose up & docker stack deploy'},
    status = {function() status() end, 'shows project status'},
}

-- function to be executed before each task
-- project.preTask = function() end

-- 
function up()
    print("work in progress")
    -- if compose file
        -- parse compose file to list required bind mounts
        -- run compose in a container
    -- else 
        -- print("can't find compose file")
    --
end

-- Lists Docker entities involved in project
function status()
    local dockerhost = os.getEnv("DOCKER_HOST")
    if dockerhost == "" then
        dockerhost = "local"
    end
    print("Docker host: " .. dockerhost)

    local success, services = pcall(docker.service.list, '--filter label=docker.project.id:' .. project.id)
    local swarmMode = success

    if swarmMode then
        print("Services:")
        if #services == 0 then
            print("none")
        else
            for i, service in ipairs(services) do
                print(" - " .. service.name .. " image: " .. service.image)
            end
        end
    else
        local containers = docker.container.list('-a --filter label=docker.project.id:' .. project.id)
        print("Containers:")
        if #containers == 0 then
            print("none")
        else
            for i, container in ipairs(containers) do
                print(" - " .. container.name .. " (" .. container.status .. ") image: " .. container.image)
            end
        end
    end

    local volumes = docker.volume.list('--filter label=docker.project.id:' .. project.id)
    print("Volumes:")
    if #volumes == 0 then
        print("none")
    else
        for i, volume in ipairs(volumes) do
            print(" - " .. volume.name .. " (" .. volume.driver .. ")")
        end
    end

    local networks = docker.network.list('--filter label=docker.project.id:' .. project.id)
    print("Networks:")
    if #networks == 0 then
        print("none")
    else
        for i, network in ipairs(networks) do
            print(" - " .. network.name .. " (" .. network.driver .. ")")
        end
    end

    local images = docker.network.list('--filter label=docker.project.id:' .. project.id)
    print("Images (built within project):")
    if #networks == 0 then
        print("none")
    else
        for i, image in ipairs(images) do
            print(" - " .. image.name)
        end
    end
end

-- indicates whether the targeted daemon runs in swarm mode
function isSwarmMode() -- bool, err
    local out, err = docker.silentCmd("info --format '{{ .Swarm.LocalNodeState }}'")
    if err ~= nil then
        return false, err
    end
    return out == 'active', nil
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
