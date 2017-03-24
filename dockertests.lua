--
-- images test
--


function tests()
	print("tests !!!")
	test1()
end


function assert(condition, errorMessage)
	if condition == false then
		error(errorMessage)
	end
end


-- -- check that listing 1 image and building 1 image give us the same result.
-- --     docker.cmd('build [...]')
-- --     docker.image.list
-- function test1()
-- 	local table_field_count = 5

-- 	local b_ret = docker.image.build('build -t docker -f Dockerfile .')
-- 	local l_arr = docker.image.list('docker')
-- 	local l_ret = l_arr[1]

-- 	-- check image tables contain the same keys

-- 	-- key count
-- 	local b_key_count = 0
-- 	for k,v in pairs(b_ret) do
-- 		b_key_count = b_key_count + 1
-- 	end

-- 	local l_key_count = 0
-- 	for k,v in pairs(l_ret) do
-- 		l_key_count = l_key_count + 1
-- 	end

-- 	if b_key_count ~= l_key_count then
-- 		error('tables have different sizes')
-- 	end

-- 	if b_key_count ~= table_field_count or l_key_count ~= table_field_count then
-- 		error('tables have incorrect size')
-- 	end

-- 	-- content
-- 	assert(b_ret['id'] ~= nil, 'a field is nil')
-- 	assert(b_ret['size'] ~= nil, 'a field is nil')
-- 	assert(b_ret['parentId'] ~= nil, 'a field is nil')
-- 	assert(b_ret['created'] ~= nil, 'a field is nil')
-- 	assert(b_ret['repoTags'] ~= nil, 'a field is nil')

-- 	assert(l_ret['id'] ~= nil, 'a field is nil')
-- 	assert(l_ret['size'] ~= nil, 'a field is nil')
-- 	assert(l_ret['parentId'] ~= nil, 'a field is nil')
-- 	assert(l_ret['created'] ~= nil, 'a field is nil')
-- 	assert(l_ret['repoTags'] ~= nil, 'a field is nil')

-- 	assert(b_ret['id'] == l_ret['id'], 'tables have different content')
-- 	assert(b_ret['size'] == l_ret['size'], 'tables have different content')
-- 	assert(b_ret['parentId'] == l_ret['parentId'], 'tables have different content')
-- 	assert(b_ret['created'] == l_ret['created'], 'tables have different content')
-- 	assert(#b_ret['repoTags'] == #l_ret['repoTags'], 'tables have different content')

-- 	for i,v in ipairs(b_ret['repoTags']) do
-- 		local found = false
-- 		for ii,vv in ipairs(l_ret['repoTags']) do
-- 			if v == vv then
-- 				found = true
-- 				break
-- 			end
-- 		end
-- 		assert(found, 'tables have different content')
-- 	end

-- 	-- remove built image
-- 	docker.cmd('rmi -f test1')
-- end

--
--
--
--
--

-- -- Lists Docker entities involved in project
-- function status()
-- 	local dockerhost = os.getEnv("DOCKER_HOST")
-- 	if dockerhost == "" then
-- 		dockerhost = "local"
-- 	end
-- 	print("Docker host: " .. dockerhost)

-- 	local success, services = pcall(docker.service.list, '--filter label=docker.project.id:' .. docker.project.id)
-- 	local swarmMode = success

-- 	if swarmMode then
-- 		print("Services:")
-- 		if #services == 0 then
-- 			print("none")
-- 		else
-- 			for i, service in ipairs(services) do
-- 				print(" - " .. service.name .. " image: " .. service.image)
-- 			end
-- 		end
-- 	else
-- 		local containers = docker.container.list('-a --filter label=docker.project.id:' .. docker.project.id)
-- 		print("Containers:")
-- 		if #containers == 0 then
-- 			print("none")
-- 		else
-- 			for i, container in ipairs(containers) do
-- 				print(" - " .. container.name .. " (" .. container.status .. ") image: " .. container.image)
-- 			end
-- 		end
-- 	end

-- 	local volumes = docker.volume.list('--filter label=docker.project.id:' .. docker.project.id)
-- 	print("Volumes:")
-- 	if #volumes == 0 then
-- 		print("none")
-- 	else
-- 		for i, volume in ipairs(volumes) do
-- 			print(" - " .. volume.name .. " (" .. volume.driver .. ")")
-- 		end
-- 	end

-- 	local networks = docker.network.list('--filter label=docker.project.id:' .. docker.project.id)
-- 	print("Networks:")
-- 	if #networks == 0 then
-- 		print("none")
-- 	else
-- 		for i, network in ipairs(networks) do
-- 			print(" - " .. network.name .. " (" .. network.driver .. ")")
-- 		end
-- 	end

-- 	local images = docker.network.list('--filter label=docker.project.id:' .. docker.project.id)
-- 	print("Images (built within project):")
-- 	if #networks == 0 then
-- 		print("none")
-- 	else
-- 		for i, image in ipairs(images) do
-- 			print(" - " .. image.name)
-- 		end
-- 	end
-- end
