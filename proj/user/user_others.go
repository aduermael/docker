// +build !linux

package user

import (
	"os/user"
)

// GetUsername returns the username for the current user
func GetUsername() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return usr.Username, nil
}
