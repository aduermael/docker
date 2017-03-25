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
    -- using anonymous function because compose() is not defined yet
    compose = {function(args) compose(args) end, 'just like docker-compose'},
    status = {function() status() end, 'shows project status'},
}

-- Behaves like docker-compose binary (https://docs.docker.com/compose/)
function compose(args)
    local jsonstr, err = docker.silentCmd('run --rm -e HOST_BIND_MOUNTS=1 ' ..
        '-w ' .. project.root .. ' ' ..
        'aduermael/compose ' .. utils.join(args, ' '))
    if err ~= nil then
        error(err)
    end

    local bindMounts = json.decode(jsonstr)
    for i,bindmount in ipairs(bindMounts) do
        bindMounts[i] = '-v ' .. bindmount .. ':'  .. bindmount .. ':ro'
    end

    pcall(docker.cmd, 'run --rm ' ..
        '-w ' .. project.root .. ' ' ..
        '-v /var/run/docker.sock:/var/run/docker.sock ' ..
        utils.join(bindMounts, ' ') .. ' ' ..
        'aduermael/compose -p ' .. project.name .. ' ' ..
        utils.join(args, ' '))
end

-- Lists Docker entities involved in project
function status()
    local dockerhost = os.getEnv("DOCKER_HOST")
    if dockerhost == "" then
        dockerhost = "local"
    end
    print("Docker host: " .. dockerhost)

    local swarmMode, err = isSwarmMode()
    if err ~= nil then
        error(err)
    end

    if swarmMode then
        local services = docker.service.list('--filter label=docker.project.id:' .. project.id)
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

-- returns true if daemon runs in swarm mode
function isSwarmMode() -- bool, err
    local out, err = docker.silentCmd("info --format '{{ .Swarm.LocalNodeState }}'")
    if err ~= nil then
        return false, err
    end
    return out == 'active', nil
end

----------------
-- UTILS
----------------

utils = {}

-- returns a string combining strings from  string array in parameter
-- an optional string separator can be provided.
utils.join = function(arr, sep)
    str = ""
    if sep == nil then
        sep = ""
    end
    if arr ~= nil then
        for i,v in ipairs(arr) do
            if str == "" then
                str = v
            else
                str = str .. sep ..  v
            end
        end
    end
    return str
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
