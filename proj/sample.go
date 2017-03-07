package project

const dockerCommandsSample = `# this is a YAML sample`

const dockerscriptSample = `-- This file defines Docker project commands.
-- All top level functions are available using ` + "`docker FUNCTION_NAME`" + ` from within project directory.
-- Default Docker commands can be overridden using identical names.

-- Lists project containers
function ps(args)
local argsStr = utils.join(args, " ")
docker.cmd('ps ' .. argsStr .. ' --filter label=docker.project.id:' .. docker.project.id)
end

-- Stops running project containers
function stop(args)
-- retrieve command args
local argsStr = utils.join(args, " ")
-- stop project containers
local containers = docker.container.list('--filter label=docker.project.id:' .. docker.project.id)
for i, container in ipairs(containers) do
docker.cmd('stop ' .. argsStr .. ' ' .. container.name)
end
end

-- Removes project containers, images, volumes & networks
function clean()
-- stop project containers
stop()
-- remove project containers
local containers = docker.container.list('-a --filter label=docker.project.id:' .. docker.project.id)
for i, container in ipairs(containers) do
docker.cmd('rm ' .. container.name)
end
-- remove project images
local images = docker.image.list('--filter label=docker.project.id:' .. docker.project.id)
for i, image in ipairs(images) do
docker.cmd('rmi ' .. image.id)
end
-- remove project volumes
local volumes = docker.volume.list('--filter label=docker.project.id:' .. docker.project.id)
for i, volume in ipairs(volumes) do
docker.cmd('volume rm ' .. volume.name)
end
-- remove project networks
local networks = docker.network.list('--filter label=docker.project.id:' .. docker.project.id)
for i, network in ipairs(networks) do
docker.cmd('network rm ' .. network.id)
end
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
