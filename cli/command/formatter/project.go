package formatter

import project "github.com/docker/docker/proj"

const (
	defaultProjectQuietFormat = "{{.RootDir}}"
	defaultProjectTableFormat = "table {{.Name}}\t{{.RootDir}}"

	projectNameHeader    = "PROJECT NAME"
	projectRootDirHeader = "ROOT DIRECTORY"
)

// NewProjectFormat returns a format for use with a project Context
func NewProjectFormat(source string, quiet bool) Format {
	switch source {
	case TableFormatKey:
		if quiet {
			return defaultProjectQuietFormat
		}
		return defaultProjectTableFormat
	case RawFormatKey:
		if quiet {
			return `name: {{.Config.Name}}`
		}
		return `name: {{.Config.Name}}\ndir: {{.RootDirPath}}\n`
	}
	return Format(source)
}

// ProjectWrite writes formatted projects using the Context
func ProjectWrite(ctx Context, projects []*project.Project) error {
	render := func(format func(subContext subContext) error) error {
		for _, p := range projects {
			if err := format(&projectContext{v: *p}); err != nil {
				return err
			}
		}
		return nil
	}
	return ctx.Write(newProjectContext(), render)
}

type projectHeaderContext map[string]string

type projectContext struct {
	HeaderContext
	v project.Project
}

func newProjectContext() *projectContext {
	projectCtx := projectContext{}
	projectCtx.header = projectHeaderContext{
		"Name":    projectNameHeader,
		"RootDir": projectRootDirHeader,
	}
	return &projectCtx
}

func (c *projectContext) MarshalJSON() ([]byte, error) {
	return marshalJSON(c)
}

func (c *projectContext) Name() string {
	return c.v.Name
}

func (c *projectContext) RootDir() string {
	return c.v.RootDir
}
