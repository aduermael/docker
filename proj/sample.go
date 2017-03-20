package project

const projectConfigSample = `-- Docker project configuration

project = {
	"id" = "%s",
	"name" = "%s"
}

project.tasks = {
	"up" = up
}

-- functions

function up(args)
	print("up test")
end
`
