--
-- images test
--

Tests = { id = "1" }

function Tests:run()
  print("RUNNING tests...")
  self:testDockerContainerInspect()
end

function Tests:testDockerContainerInspect()
	local testName = "docker.container.inspect"
	print('TEST: ' .. testName)

	-- cleanup to avoid collisions
	pcall(docker.silentCmd, 'rm -fv ' .. testName)
	
	local containerID = docker.silentCmd('run -ti -d --name ' .. testName .. ' alpine:3.5 ash')
	local container = docker.container.inspect(containerID)[1]
	if container.name ~= testName then
		error("container name is not the one expected")
	end

	-- cleanup
	docker.silentCmd('rm -fv ' .. testName)

	print("SUCCESS")
end


local tests = Tests
tests.id = "2"

return tests
