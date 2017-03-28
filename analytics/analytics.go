package analytics

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/docker/docker/cli/config"
	user "github.com/docker/docker/pkg/idtools/user"
	project "github.com/docker/docker/proj/project"
	analytics "github.com/segmentio/analytics-go"
)

var (
	cliTestVersion = "0.0.6"
	patch          = 1
	userid         = ""
	inproj         = false
	usernames      = ""
)

func init() {
	// disable init in detached process
	if os.Getenv("DOCKERSCRIPT_ANALYTICS") == "1" {
		return
	}

	configDir := config.Dir()
	// just making sure it exists...
	os.MkdirAll(configDir, 0777)

	idPath := filepath.Join(configDir, ".testuserid")

	// create if not found
	if _, err := os.Stat(idPath); err != nil {
		// generate user id
		data := make([]byte, 64)
		_, err := rand.Read(data)
		if err == nil {
			userid = fmt.Sprintf("%x", sha256.Sum256(data))
			ioutil.WriteFile(idPath, []byte(userid), 0644)
		}
	} else {
		idbytes, err := ioutil.ReadFile(idPath)
		if err == nil {
			userid = string(idbytes)
		}
	}

	// see if command is executed in Docker project context
	wd, err := os.Getwd()
	if err == nil {
		_, err := project.FindProjectRoot(wd)
		if err == nil {
			inproj = true
		}
	}

	var usernamesArr = make([]string, 0)

	conf, err := config.Load("")
	if err == nil {
		if conf.ContainsAuth() {
			for _, authConfig := range conf.AuthConfigs {
				usernamesArr = append(usernamesArr, authConfig.Username)
			}
		}
	}
	// remove duplicated usernames
	var usernamesMap = make(map[string]bool)
	for _, username := range usernamesArr {
		usernamesMap[username] = true
	}
	usernamesArr = make([]string, 0)
	for k := range usernamesMap {
		usernamesArr = append(usernamesArr, k)
	}

	// order usernames in alphabetical order
	sort.Strings(usernamesArr)

	// generate usernames string
	usernames = strings.Join(usernamesArr, ",")
}

// Event sends an event to the analytics platform
func Event(name string, properties map[string]interface{}) {
	t := &analytics.Track{
		Event:  name,
		UserId: userid,
		Properties: map[string]interface{}{
			"project":   inproj,
			"username":  usernames,
			"version":   cliTestVersion,
			"patch":     patch,
			"localuser": getSystemUsername(),
			"os":        getOSName(),
		},
	}
	for k, v := range properties {
		if _, exists := t.Properties[k]; exists {
			continue
		}
		t.Properties[k] = v
	}
	eventStartProcess(t)
}

func eventStartProcess(track *analytics.Track) {
	// json marshal track struct
	jsonBytes, _ := json.Marshal(track) // ignore error
	// start new docker process to upload event
	cmd := exec.Command(os.Args[0], string(jsonBytes))
	cmd.Env = append(cmd.Env, "DOCKERSCRIPT_ANALYTICS=1")
	cmd.Start()
}

func getOSName() string {
	return runtime.GOOS
}

func getSystemUsername() string {
	usrName, err := user.GetUsername()
	if err != nil {
		return ""
	}
	return usrName
}
