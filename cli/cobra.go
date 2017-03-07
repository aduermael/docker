package cli

import (
	"bufio"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/term"
	"github.com/spf13/cobra"

	project "github.com/docker/docker/proj"
)

// SetupRootCommand sets default usage, help, and error handling for the
// root command.
func SetupRootCommand(rootCmd *cobra.Command) {
	cobra.AddTemplateFunc("hasSubCommands", hasSubCommands)
	cobra.AddTemplateFunc("hasManagementSubCommands", hasManagementSubCommands)
	cobra.AddTemplateFunc("operationSubCommands", operationSubCommands)
	cobra.AddTemplateFunc("managementSubCommands", managementSubCommands)
	cobra.AddTemplateFunc("wrappedFlagUsages", wrappedFlagUsages)

	cobra.AddTemplateFunc("hasProjectDefinedCommands", hasProjectDefinedCommands)
	cobra.AddTemplateFunc("projectDefinedCommands", projectDefinedCommands)

	cobra.AddTemplateFunc("swarmCommands", swarmCommands)

	rootCmd.SetUsageTemplate(usageTemplate)
	rootCmd.SetHelpTemplate(helpTemplate)
	rootCmd.SetFlagErrorFunc(FlagErrorFunc)
	rootCmd.SetHelpCommand(helpCommand)

	rootCmd.PersistentFlags().BoolP("help", "h", false, "Print usage")
	rootCmd.PersistentFlags().MarkShorthandDeprecated("help", "please use --help")
}

// FlagErrorFunc prints an error message which matches the format of the
// docker/docker/cli error messages
func FlagErrorFunc(cmd *cobra.Command, err error) error {
	if err == nil {
		return nil
	}

	usage := ""
	if cmd.HasSubCommands() {
		usage = "\n\n" + cmd.UsageString()
	}
	return StatusError{
		Status:     fmt.Sprintf("%s\nSee '%s --help'.%s", err, cmd.CommandPath(), usage),
		StatusCode: 125,
	}
}

var helpCommand = &cobra.Command{
	Use:               "help [command]",
	Short:             "Help about the command",
	PersistentPreRun:  func(cmd *cobra.Command, args []string) {},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {},
	RunE: func(c *cobra.Command, args []string) error {
		cmd, args, e := c.Root().Find(args)
		if cmd == nil || e != nil || len(args) > 0 {
			return fmt.Errorf("unknown help topic: %v", strings.Join(args, " "))
		}

		helpFunc := cmd.HelpFunc()
		helpFunc(cmd, args)
		return nil
	},
}

func hasSubCommands(cmd *cobra.Command) bool {
	return len(operationSubCommands(cmd)) > 0
}

func hasManagementSubCommands(cmd *cobra.Command) bool {
	return len(managementSubCommands(cmd)) > 0
}

// hasProjectDefinedCommands indicates whether user-defined commands are available.
// For now, they are only available in the context of a docker project.
func hasProjectDefinedCommands(cmd *cobra.Command) bool {
	return len(GetProjectDefinedFunctions()) > 0
}

func projectDefinedCommands(cmd *cobra.Command) []UDFunction {
	return GetProjectDefinedFunctions()
}

func operationSubCommands(cmd *cobra.Command) []*cobra.Command {
	cmds := []*cobra.Command{}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() && !sub.HasSubCommands() {
			cmds = append(cmds, sub)
		}
	}
	return cmds
}

func wrappedFlagUsages(cmd *cobra.Command) string {
	width := 80
	if ws, err := term.GetWinsize(0); err == nil {
		width = int(ws.Width)
	}
	return cmd.Flags().FlagUsagesWrapped(width - 1)
}

func managementSubCommands(cmd *cobra.Command) []*cobra.Command {
	cmds := []*cobra.Command{}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() && sub.HasSubCommands() {
			if isCommandSwarmRelated(sub) == false {
				cmds = append(cmds, sub)
			}
		}
	}
	return cmds
}

func swarmCommands(cmd *cobra.Command) []*cobra.Command {
	cmds := []*cobra.Command{}
	for _, sub := range cmd.Commands() {
		if sub.IsAvailableCommand() && sub.HasSubCommands() {
			if isCommandSwarmRelated(sub) {
				cmds = append(cmds, sub)
			}
		}
	}
	return cmds
}

//////////

// isCommandSwarmRelated ...
func isCommandSwarmRelated(cmd *cobra.Command) bool {
	if cmd.Name() == "node" ||
		cmd.Name() == "secret" ||
		cmd.Name() == "service" ||
		cmd.Name() == "stack" ||
		cmd.Name() == "swarm" {
		return true
	}
	return false
}

//////////

// UDFunction partially describes a user-define function written in Lua
type UDFunction struct {
	Name        string
	Description string
	Padding     int
}

// GetProjectDefinedFunctions lists functions defined in dockerscript file
func GetProjectDefinedFunctions() []UDFunction {
	// test if we are in the context of a project
	wd, err := os.Getwd()
	if err != nil {
		return make([]UDFunction, 0)
	}
	proj, err := project.Get(wd)
	if err != nil {
		return make([]UDFunction, 0)
	}
	if proj == nil {
		// we are not in the context of a project
		return make([]UDFunction, 0)
	}
	// we are in the context of a project,
	// we have to check if any there is any user-defined function.
	udFunctions := make([]UDFunction, 0)
	dockerscriptFilePath := filepath.Join(proj.DockerprojDirPath(), "dockerscript.lua")
	if fi, err := os.Stat(dockerscriptFilePath); err == nil {
		if fi.IsDir() == false {
			fileBytes, err := ioutil.ReadFile(dockerscriptFilePath)
			if err != nil {
				return make([]UDFunction, 0)
			}
			fileStringReader := bufio.NewReader(strings.NewReader(string(fileBytes)))
			// we store the previous line content to look for a comment in the
			// event of a function found on the current line.
			previousLine := ""
			for {
				line, err := fileStringReader.ReadString(byte('\n'))
				if err != nil {
					if err == io.EOF {
						break
					}
					return make([]UDFunction, 0)
				}
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "function ") {
					trimmedLine := strings.TrimPrefix(line, "function ")
					functionName := (strings.Split(trimmedLine, "("))[0]
					functionName = strings.TrimSpace(functionName)
					// check for description on the previous line
					functionDescription := ""
					if len(previousLine) > 0 && strings.HasPrefix(previousLine, "--") {
						trimmedLine = strings.TrimPrefix(previousLine, "--")
						functionDescription = strings.TrimSpace(trimmedLine)
					}
					udFunctions = append(udFunctions, UDFunction{Name: functionName, Description: functionDescription, Padding: 11})
				}
				previousLine = line
			}
			return udFunctions
		}
		return make([]UDFunction, 0)
	}
	return make([]UDFunction, 0)
}

var usageTemplate = `Usage:

{{- if not .HasSubCommands}}	{{.UseLine}}{{end}}
{{- if .HasSubCommands}}	{{ .CommandPath}} COMMAND{{end}}

{{ .Short | trim }}

{{- if gt .Aliases 0}}

Aliases:
  {{.NameAndAliases}}

{{- end}}
{{- if .HasExample}}

Examples:
{{ .Example }}

{{- end}}
{{- if .HasFlags}}

Options:
{{ wrappedFlagUsages . | trimRightSpace}}

{{- end}}


{{- if hasManagementSubCommands . }}

{{- if hasProjectDefinedCommands . }}

Project Commands:
{{- range projectDefinedCommands . }}
  {{rpad .Name .Padding }} {{.Description}}
{{- end}}
{{- end}}

Management Commands:

{{- range managementSubCommands . }}
  {{rpad .Name .NamePadding }} {{.Short}}
{{- end}}

Swarm Commands:

{{- range swarmCommands . }}
  {{rpad .Name .NamePadding }} {{.Short}}
{{- end}}

{{- end}}

{{- if hasSubCommands .}}

Commands:

{{- range operationSubCommands . }}
  {{rpad .Name .NamePadding }} {{.Short}}
{{- end}}
{{- end}}

{{- if .HasSubCommands }}

Run '{{.CommandPath}} COMMAND --help' for more information on a command.
{{- end}}
`

var helpTemplate = `
{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`
