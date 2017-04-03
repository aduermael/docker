# Changelog

### Version 0.0.7 - 04/03/2017

- Project scoping now implemented both ways (when creating and querying Docker entities). It's not required anymore to filter "manually" when listing containers, volumes, etc.
- New functions in the Lua sandbox:
	- `docker.container.inspect`
	- `docker.image.inspect`
	- fixed `require`

### Version 0.0.6 - 03/25/2017

- CLI was crashing when working outside a project

### Version 0.0.5 - 03/24/2017

- a `dockerproject.lua` file is sufficient to define a Docker project (replacing `docker.project` directory and its content)
- project variables now in `project` table, not `docker.project`
- added `docker project ls` to list recent projects

### Version 0.0.4

- first version published on that repository