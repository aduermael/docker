-- Docker project configuration

tests = require("dockertests")

project = {
    id = "62b97e4d-f0e2-4459-9485-93554831db31",
    name = "docker",
}

project.tasks = {
    -- using anonymous function because up() is not defined yet at this point
    up = {function() up() end, 'equivalent to docker-compose up'},
    exportDE = {function(args) exportDE(args) end, 'export docker cli binaries for internal users'},
    exportEU = {function(args) exportEU(args) end, 'export docker cli binaries for external users'},

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
    print("ARGS:", args)
    -- print('ARGS:', table.getn(args))
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
        '-v ' .. docker.project.root .. ':/go/src/github.com/docker/docker ' ..
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