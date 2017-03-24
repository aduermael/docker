# Dockerscript prototype

This prototype is a modified Docker CLI that supports scripting and project scoping.

## How to install it?

### Docker for Mac

Download [this binary](https://github.com/docker/cli-init-cmd/raw/master/binaries/mac/docker.zip) and put it on your Desktop.

Then type these 2 commands in a terminal:

```bash
# don't worry, this is just a link to the actual binary
# restarting Docker for Mac restores it
rm /usr/local/bin/docker
# create new link pointing to the binary you downloaded
ln -s ~/Desktop/docker /usr/local/bin/docker
```

Tadam! 😀🎉

You can now use `docker init` (or `docker project init`, it's actually the same thing).

To uninstall, just restart Docker for Mac, the symlink will be restored.

### Docker for Windows

No precise instructions yet. Just use [this binary](https://github.com/docker/cli-init-cmd/raw/master/binaries/windows/docker.zip).

### Linux

🤓 - Do you really need instructions if you're on Linux?

You do need the binaries though: [linux](https://github.com/docker/cli-init-cmd/raw/master/binaries/linux/docker.zip) or [linux-arm](https://github.com/docker/cli-init-cmd/raw/master/binaries/linux-arm/docker.zip) (only bundles the CLI, not the daemon)


## How to use it?

From within your Docker project, type `docker --help` or just `docker`. "From within your project" means from the root directory or from any of its children.

You'll see `status` listed under **Project Commands**. It means that `docker status` has been defined in the scope of your project.

You can see how it works if you open `dockerscript.lua` under the `docker.project` directory that's been created.

😁 - Now you can create your own Docker commands!

These scripts are executed in a Lua sandbox. A few functions are available in there for you to build your own:

- Variables about your project:

	```lua
	docker.project.id
	docker.project.name
	-- absolute path of the project root directory (parent directory of docker.project)
	docker.project.root
	```

- Docker functions:

	```lua
	-- use this to call any docker command (build, run, attach...)
	docker.cmd('run -ti -d myimage')
	-- silent version to catch output and error streams
	-- err will be nil in that example if there's no error:
	local stdout, stderr = docker.silentCmd('build .')

	-- to catch errors you can use protected calls:
	local status, err = pcall(docker.cmd, 'run -ti -d myimage')
	-- of course it also work with silent version:
	local status, err, stdout, stderr = pcall(docker.silentCmd, 'run -ti -d myimage')

	-- specific functions to list Docker items:
	local containers = docker.container.list()
	local images = docker.image.list()
	local volumes = docker.volume.list()
	local networks = docker.network.list()
	local services = docker.service.list()
	local secrets = docker.secret.list()

	-- list functions accept same arguments as corresponding Docker commands:
	containers = docker.container.list('--filter label=docker.project.id=' .. docker.project.id)
	```

- Things that you usually find in Lua environments:

	```lua
	print, tostring, tonumber, pairs, ipairs, unpack, error, assert, pcall, string, table
	```	
	
- An `os` table:

	```lua
	-- returns current user's home directory path
	os.home()
	-- returns current user's username
	os.username()
	-- get/set environment variables
	-- (can be used to set DOCKER_HOST for example)
	os.setEnv("KEY","VALUE")
	local value = os.getEnv("KEY")
	```

- There's also a JSON parser:

	```lua
	-- prints all Docker images in json form:
	local images = docker.image.list()
	-- when debugging it provides an easy way to see what's in your tables
	print(json.encode(images))

	-- that doesn't do anything:
	images = json.decode(json.encode(images))
	```
	
- `require` function:

	```lua
	-- you can import Dockerscripts defined in different files using require()
	local tunnel = require("tunnel.lua")
	-- the lua file extension can be omitted:
	local tunnel = require("tunnel")
	
	-- import paths are relative to the docker.project directory
	local tunnel = require("utils/tunnel")
	
	-- you can also use absolute paths
	local tunnel = require(os.home() .. "/dockerscripts/tunnel")
	
	-- assuming start() is a top level function defined in tunnel.lua
	tunnel.start('123.888.888.888')
	```
	
### Collaboration / user specific scripts

`docker init` by default creates a directory called `devs` within `docker.project`. In that directory you should see a file called `USERNAME-dockerscript.lua`. You can use it to define scripts that are user specific. You can override things from the main `dockerscript.lua`. 

Other developers on the same project can use their username prefixed Dockerscripts. 

A common thing to do is to reference the `devs` directory in your `.gitignore` to avoid committing user specific scripts, especially if you plan to include some secret things in them...

![project directory](https://github.com/docker/cli-init-cmd/raw/master/docker-init/img/project-dir.png)


