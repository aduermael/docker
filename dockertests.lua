--
-- images test
--

Tests = { currentTest = "", eraser = "" }

function Tests:cmd(str) 
	local success, out = pcall(docker.silentCmd, str)
	if success == false then
		-- out is an error in that case
		self:fail(out)
	end
	return out
end

function Tests:start(testName)
	self.currentTest = testName
	-- used to erase lines...
	-- +3 because of prefix emoji
	self.eraser = ""
	for i=1,string.len(self.currentTest) + 3 do
		self.eraser = self.eraser .. " "
	end
	printf('⚙️  %s', self.currentTest)
end

function Tests:fail(message)
	printf('\r%s', self.eraser)
	printf('\r❌  %s\n', self.currentTest)
	error(message)
end

function Tests:log(...)
	printf('\r%s\r', self.eraser)
	print(...)
	printf('⚙️  %s', self.currentTest)
end

function Tests:success()
	printf('\r%s', self.eraser)
	printf('\r👍  %s\n', self.currentTest)
end

function Tests:run()
  self:testDockerContainerInspect()
  self:testProjectScope()
  self:testVolumeCreateStdout()
end
  
function Tests:testDockerContainerInspect()
	self:start("docker.container.inspect()")

	-- cleanup to avoid collisions
	pcall(docker.silentCmd,'rm -fv docker.container.inspect')
	
	local containerID = self:cmd('run -ti -d --name docker.container.inspect alpine:3.5 ash')
	local container = docker.container.inspect(containerID)[1]
	if container.name ~= 'docker.container.inspect' then
		self:fail("container name is not the one expected")
	end

	-- cleanup
	self:cmd('rm -fv docker.container.inspect')

	self:success()
end

function Tests:testProjectScope()
	self:start("project scope")

	local scope1 = "com.docker.test.scope.id.1"
	local scope2 = "com.docker.test.scope.id.2"
	local containers = nil

	self:log("cleanup for both test scopes")

	-- clean both test scopes
	project.id = scope1
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		self:cmd('rm -f ' .. container.id)
	end
	project.id = scope2
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		self:cmd('rm -f ' .. container.id)
	end

	-- make sure both scopes are clean
	project.id = scope1
	containers = docker.container.list()
	if #containers ~= 0 then
		self:fail('scope #1 not clean')
	end
	project.id = scope2
	containers = docker.container.list()
	if #containers ~= 0 then
		self:fail('scope #2 not clean')
	end

	self:log("remove container with generated name")

	-- run container with no name
	-- try to remove it using its name
	self:cmd('run -ti -d --label generated_name alpine:3.5 ash')
	containers = docker.container.list('--filter label=generated_name')
	if #containers ~= 1 then
		self:fail('expecting 1 container with "generated_name" label')
	end
	self:cmd('rm -f '  .. containers[1].name)

	self:log("start containers in both scopes")

	-- run 3 containers in scope #1
	project.id = scope1
	self:cmd('run -ti -d alpine:3.5 ash')
	self:cmd('run -ti -d alpine:3.5 ash')
	self:cmd('run -ti -d alpine:3.5 ash')
	-- run 1 container in scope #2
	project.id = scope2
	self:cmd('run -ti -d alpine:3.5 ash')

	-- test number of containers in both scopes
	project.id = scope2
	containers = docker.container.list()
	if #containers ~= 1 then
		self:fail('expecting 1 container in scope #2')
	end
	project.id = scope1
	containers = docker.container.list()
	if #containers ~= 3 then
		self:fail('expecting 3 containers in scope #1')
	end

	-- TODO: insert more tests here

	self:log("stop + rm containers in scope #1")

	-- stop and rm all containers in scope #1
	project.id = scope1
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		self:cmd('stop -t 0 ' .. container.id)
	end
	for i,container in ipairs(containers) do
		self:cmd('rm ' .. container.id)
	end

	self:log("rm -f containers in scope #2")

	-- rm -f all containers in scope #2
	project.id = scope2
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		self:cmd('rm -f ' .. container.id)
	end

	self:success()
end

-- VOLUME --

-- check that volume create prints the correct output
function Tests:testVolumeCreateStdout()
	self:start('volume create output')
	project.id = 'com.docker.test.scope.id.1'
	local volumeName = 'foo'
	local out = self:cmd('volume create ' .. volumeName)
	if out ~= volumeName then
		self:fail('volume create did not print the volume\'s id')
	end
	self:cmd('volume rm ' .. volumeName)
	self:success()
end

local tests = Tests

return tests
