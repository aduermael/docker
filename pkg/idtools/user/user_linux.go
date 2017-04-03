// +build linux

package user

import (
	"os"

	"github.com/docker/docker/pkg/idtools"
)

// GetUsername returns the username for the current user
func GetUsername() (string, error) {
	uid := os.Getuid()
	usr, err := idtools.LookupUID(uid)
	if err != nil {
		return "", err
	}
	return usr.Name, nil
}

// GetHomeDirPath returns the home directory path for the current user
func GetHomeDirPath() (string, error) {
	uid := os.Getuid()
	usr, err := idtools.LookupUID(uid)
	if err != nil {
		return "", err
	}
	return usr.Home, nil
}
