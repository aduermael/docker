package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/docker/docker/cli"
	"github.com/docker/docker/cli/command"
	project "github.com/docker/docker/proj/project"
	"github.com/spf13/cobra"
)

type initOptions struct {
	projectName string
	projectDir  string
}

// NewInitCommand creates a new cobra.Command for `docker project init`
func NewInitCommand(dockerCli *command.DockerCli) *cobra.Command {
	var opts initOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initiate Docker project",
		Args:  cli.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(dockerCli, &opts)
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.projectDir, "dir", "d", "", "Target directory (default is current directory)")
	flags.StringVarP(&opts.projectName, "name", "n", "", "Project name, parent directory name will be used by default")

	return cmd
}

func runInit(dockerCli *command.DockerCli, opts *initOptions) error {

	// directory where project should be initiated
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	if opts.projectDir != "" {
		if filepath.IsAbs(opts.projectDir) {
			dir = opts.projectDir
		} else {
			dir = filepath.Clean(filepath.Join(dir, opts.projectDir))
		}
	}

	// if project name is empty, try to use parent folder name
	if opts.projectName == "" {
		opts.projectName = filepath.Base(dir)
	}

	// check project name
	b, err := regexp.MatchString("^[a-zA-Z0-9\\.-]+$", opts.projectName)
	if err != nil {
		return err
	}
	if b == false {
		return errors.New("project name can only contain alphanumeric characters (A-Z,a-z,0-9), hyphen (-), and period (.)")
	}

	err = project.Init(dir, opts.projectName)
	if err != nil {
		return err
	}
	fmt.Fprintf(dockerCli.Out(), "project %s created in %s\n", opts.projectName, dir)

	return nil
}
