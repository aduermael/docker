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
	project "github.com/docker/docker/proj"
	projectUser "github.com/docker/docker/proj/user"
	analytics "github.com/segmentio/analytics-go"
)

var (
	client         *analytics.Client
	cliTestVersion = "0.0.4"
	userid         = ""
	inproj         = false
	usernames      = ""
)

func init() {

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
		var proj *project.Project
		proj, err = project.Get(wd)
		if err == nil {
			if proj != nil {
				inproj = true
			}
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

	client = analytics.New("EMkyNVNnr7Ian1RrSOW8b4JdAt4GQ7lI")
	// client.Verbose = true
	client.Size = 1

	// identify users that are logged in
	if usernames != "" && userid != "" {
		client.Identify(&analytics.Identify{
			UserId: userid,
			Traits: map[string]interface{}{
				"login": usernames,
			},
		})
	}
}

// Event sends an event to the analytics platform
func Event(name string, properties map[string]interface{}) {
	t := &analytics.Track{
		Event:  name,
		UserId: userid,
		Properties: map[string]interface{}{
			"project":    inproj,
			"username":   usernames,
			"version":    cliTestVersion,
			"localuser":  getSystemUsername(),
			"isemployee": getIsDockerEmployee(),
			"os":         getOSName(),
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

// Close closes the analytics client after all the requests are finished
func Close() {
	client.Close()
}

func eventStartProcess(track *analytics.Track) {
	// json marshal track struct
	jsonBytes, _ := json.Marshal(track) // ignore error
	// start new docker process to upload event
	cmd := exec.Command(os.Args[0], string(jsonBytes))
	cmd.Env = append(cmd.Env, "DOCKERSCRIPT_ANALYTICS=1")
	cmd.Start()
}

func eventDirect(track *analytics.Track) error {
	return client.Track(track)
}

func getOSName() string {
	return runtime.GOOS
}

func getIsDockerEmployee() bool {
	return isDockerEmployee
}

func getSystemUsername() string {
	usrName, err := projectUser.GetUsername()
	if err != nil {
		return ""
	}
	return usrName
}
