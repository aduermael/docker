-- Docker project configuration

project = {
	id = '12345',
	name = 'youpi',
}


project.tasks = {
	up = up,
}

-- functions

function up(args)
	print("up test")
	-- local docker.cmdSilent('run --rm lookForComposeFile')
	-- local mounts = docker.cmdSilent('run --rm listRequiredMounts')
	-- docker.cmd('run dockerCompose' .. args)
end

-- MORE

project.preCmd = function()
	-- body
	for i,task in ipairs(listComposeTasks()) do
		tasks[task] = function(args)
			runComposeTask(task, args)
		end
	end
end

-- returns string array
function listComposeTasks()
end

-- takes task name and arguments
function runComposeTask(task, args)
end
