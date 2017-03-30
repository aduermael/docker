--
-- images test
--

Tests = { currentTest = "" }

function Tests:start(testName)
	self.currentTest = testName
	printf('⚙️  %s', self.currentTest)
end

function Tests:fail(message)
	local str = self.currentTest
	if message ~= nil then
		str = self.currentTest .. ': ' .. message
	end
	printf('\r❌  %s\n', str)
	error(str)
end

function Tests:success()
	printf('\r👍  %s\n', self.currentTest)
end

function Tests:run()
  self:testDockerContainerInspect()
  self:testProjectScope()
end
  
function Tests:testDockerContainerInspect()
	self:start("docker.container.inspect()")

	-- cleanup to avoid collisions
	pcall(docker.silentCmd, 'rm -fv docker.container.inspect')
	
	local containerID = docker.silentCmd('run -ti -d --name docker.container.inspect alpine:3.5 ash')
	local container = docker.container.inspect(containerID)[1]
	if container.name ~= 'docker.container.inspect' then
		self:fail("container name is not the one expected")
	end

	-- cleanup
	docker.silentCmd('rm -fv docker.container.inspect')

	self:success()
end

function Tests:testProjectScope()
	self:start("project scope")

	local scope1 = "com.docker.test.scope.id.1"
	local scope2 = "com.docker.test.scope.id.2"
	local containers = nil

	-- clean both test scopes
	project.id = scope1
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		docker.silentCmd('rm -f ' .. container.id)
	end
	project.id = scope2
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		docker.silentCmd('rm -f ' .. container.id)
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

	-- run 3 containers in scope #1
	project.id = scope1
	docker.silentCmd('run -ti -d alpine:3.5 ash')
	docker.silentCmd('run -ti -d alpine:3.5 ash')
	docker.silentCmd('run -ti -d alpine:3.5 ash')
	-- run 1 container in scope #2
	project.id = scope2
	docker.silentCmd('run -ti -d alpine:3.5 ash')

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

	-- stop and rm all containers in scope #1
	project.id = scope1
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		docker.silentCmd('stop -t 0 ' .. container.id)
	end
	for i,container in ipairs(containers) do
		docker.silentCmd('rm ' .. container.id)
	end

	-- rm -f all containers in scope #2
	project.id = scope2
	containers = docker.container.list()
	for i,container in ipairs(containers) do
		docker.silentCmd('rm -f ' .. container.id)
	end

	self:success()
end

local tests = Tests

return tests
