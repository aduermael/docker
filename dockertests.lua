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
		return
	end
	printf('\r❌  %s\n', str)
	error(str)
end

function Tests:success()
	printf('\r👍  %s\n', self.currentTest)
end

function Tests:run()
  self:testDockerContainerInspect()
end
  
function Tests:testDockerContainerInspect()
	self:start("docker.container.inspect()")

	-- cleanup to avoid collisions
	pcall(docker.silentCmd, 'rm -fv docker.container.inspect')
	
	local containerID = docker.silentCmd('run -ti -d --name docker.container.inspect alpine:3.5 ash')
	local container = docker.container.inspect(containerID)[1]
	if container.name ~= testName then
		self:fail("container name is not the one expected")
	end

	-- cleanup
	docker.silentCmd('rm -fv docker.container.inspect')

	self:success()
end

local tests = Tests

return tests
