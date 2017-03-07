-- This file defines Docker project commands.
-- All top level functions are available using `docker FUNCTION_NAME` from within project directory.
-- Default Docker commands can be overridden using identical names.

-- Creates a Docker image with full dev environment
function build()
	print("Assembling full dev environment... (this is slow the first time)")
	docker.cmd('build -t docker .')
end

-- Mounts your source in an interactive container
function dev(args)
	build()
	local argsStr = utils.join(args, " ")
	docker.cmd('run ' .. argsStr .. ' -v ' .. docker.project.root .. ':/go/src/github.com/docker/docker --privileged -i -t docker bash')
end

-- Exports client binaries for all platforms
function export(args)
	if #args ~= 1 then
		print("absolute path to destination directory expected")
		return
	end
	local exportDir = args[1]
	build()
	docker.cmd('run ' ..
		'-e DOCKER_CROSSPLATFORMS="linux/amd64 linux/arm darwin/amd64 windows/amd64" ' ..
		'-v ' .. exportDir .. ':/output ' ..
		'-v ' .. docker.project.root .. ':/go/src/github.com/docker/docker ' ..
		'--privileged -i -t docker bash -c "' ..
		'VERSION=$(< ./VERSION) && ' ..
		'hack/make.sh cross-client && ' ..
		'mkdir -p /output/linux && ' ..
		'mkdir -p /output/linux-arm && ' ..
		'mkdir -p /output/mac && ' ..
		'mkdir -p /output/windows && ' ..
		'pushd bundles/$VERSION/cross-client/linux/amd64 && mv docker-$VERSION docker && zip /output/linux/docker.zip docker && popd && ' ..
		'pushd bundles/$VERSION/cross-client/linux/arm && mv docker-$VERSION docker && zip /output/linux-arm/docker.zip docker && popd && ' ..
		'pushd bundles/$VERSION/cross-client/darwin/amd64 && mv docker-$VERSION docker && zip /output/mac/docker.zip docker && popd && ' ..
		'pushd bundles/$VERSION/cross-client/windows/amd64 && mv docker-$VERSION.exe docker.exe && zip /output/windows/docker.zip docker.exe"'
	)
end

-- Runs the test suite
function test()
	build()
	docker.cmd('run -v ' .. docker.project.root .. ':/go/src/github.com/docker/docker --privileged docker hack/make.sh test-unit test-integration-cli test-docker-py')
end




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

