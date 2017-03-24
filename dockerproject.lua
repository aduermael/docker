-- Docker project configuration

tests = require("dockertests")

project = {
    id = "62b97e4d-f0e2-4459-9485-93554831db31",
    name = "docker",
    root = project.root,
}

project.tasks = {
    -- using anonymous function because up() is not defined yet at this point
    up = {function() up() end, 'equivalent to docker-compose up'},
    status = {function() status() end, 'shows project status'},
    exportDE = {function(args) exportDE(args) end, 'export docker cli binaries for internal users'},
    exportEU = {function(args) exportEU(args) end, 'export docker cli binaries for external users'},
    dev = {function(args) dev(args) end, 'develop in container'},
    tests = {tests.tests, 'runs Lua tests'},   
}

-- function to be executed before each task
-- project.preTask = function() end

function up()
    print("work in progress...")
    -- if compose file
        -- parse compose file to list required bind mounts
        -- run compose in a container
    -- else 
        -- print("can't find compose file")
    --
end



-- Exports client binaries for all platforms (For Docker Employees)
function exportDE(args)
    hidden.export(args, '')
end

-- Exports client binaries for all platforms (For External Users)
function exportEU(args)
    hidden.export(args, 'DOCKER_BUILDTAGS=\\"$DOCKER_BUILDTAGS -tags IS_EXTERNAL_USER\\" ')
end

-- Creates a Docker image with full dev environment
function build()
    print("Assembling full dev environment... (this is slow the first time)")
    docker.cmd('build -t docker .')
end

-- Mounts your source in an interactive container
function dev(args)
    build()
    local argsStr = utils.join(args, " ")
    docker.cmd('run \
    ' .. argsStr .. ' \
    -v ' .. os.home() .. '/.docker/.testuserid:/root/.docker/.testuserid \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v ' .. project.root .. ':/go/src/github.com/docker/docker \
    --privileged \
    -i \
    -t \
    docker bash')
end

hidden = {}
hidden.export = function(args, tags)
    if #args ~= 1 then
        print("absolute path to destination directory expected")
        return
    end
    local exportDir = args[1]
    build()
    local command = 'run ' ..
        '-e DOCKER_CROSSPLATFORMS="linux/amd64 linux/arm darwin/amd64 windows/amd64" ' ..
        '-v ' .. exportDir .. ':/output ' ..
        '-v ' .. project.root .. ':/go/src/github.com/docker/docker ' ..
        '--privileged -i -t docker bash -c "' ..
        'VERSION=$(< ./VERSION) && ' ..
        tags .. 'hack/make.sh cross-client && ' ..
        'mkdir -p /output/linux && ' ..
        'mkdir -p /output/linux-arm && ' ..
        'mkdir -p /output/mac && ' ..
        'mkdir -p /output/windows && ' ..
        'pushd bundles/$VERSION/cross-client/linux/amd64 && mv docker-$VERSION docker && zip /output/linux/docker.zip docker && popd && ' ..
        'pushd bundles/$VERSION/cross-client/linux/arm && mv docker-$VERSION docker && zip /output/linux-arm/docker.zip docker && popd && ' ..
        'pushd bundles/$VERSION/cross-client/darwin/amd64 && mv docker-$VERSION docker && zip /output/mac/docker.zip docker && popd && ' ..
        'pushd bundles/$VERSION/cross-client/windows/amd64 && mv docker-$VERSION.exe docker.exe && zip /output/windows/docker.zip docker.exe"'
    docker.cmd(command)
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

