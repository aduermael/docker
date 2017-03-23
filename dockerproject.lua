-- Docker project configuration

project = {
    id = "62b97e4d-f0e2-4459-9485-93554831db31",
    name = "docker",
}

project.tasks = {
    -- using anonymous function because up() is not defined yet at this point
    up = function() up() end,
}

-- function to be executed before each task
-- project.preTask = function() end

function up()
    print("work in progress")
    -- if compose file
        -- parse compose file to list required bind mounts
        -- run compose in a container
    -- else 
        -- print("can't find compose file")
    --
end
