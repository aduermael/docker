-- Docker project configuration

project = {
	id = '12345',
	name = 'youpi',
}

myFunc = function()
	-- body
end

project.tasks = {
	a = myFunc,

	b = {myFunc},

	c = {myFunc, 'short'},

	d = {myFunc, 'short', 'long'},

	e = {
		func = myFunc,
		short = 'short',
		desc = 'long',
	},

	f = {
		func = myFunc,
		desc = 'long',
	},
}



-- called before any custom command
project.onPreCmd = function()
	-- body
end

-- functions



-- 
function up(args)
	print("up test")
	-- local docker.cmdSilent('run --rm lookForComposeFile')
	-- local mounts = docker.cmdSilent('run --rm listRequiredMounts')
	-- docker.cmd('run dockerCompose' .. args)
end

-- returns string array
function listComposeTasks()
end

-- takes task name and arguments
function runComposeTask(task, args)
end

-- called once on project load
function loadYAMLCommands()
	for i,task in ipairs(listComposeTasks()) do
		tasks[task] = function(args)
			runComposeTask(task, args)
		end
	end	
end

-- loadYAMLCommands()