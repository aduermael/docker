package project

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"sort"
	"time"

	cliconfig "github.com/docker/docker/cli/config"
)

const (
	recentProjectsFileName = ".recentProjects.json"
)

type recentProject struct {
	IDVal      string `json:"id"`
	NameVal    string `json:"name"`
	RootDirVal string `json:"root"`

	Timestamp int `json:"t"`
}

// Project interface implementation
func (rp *recentProject) RootDir() string {
	return rp.RootDirVal
}
func (rp *recentProject) ID() string {
	return rp.IDVal
}
func (rp *recentProject) Name() string {
	return rp.NameVal
}
func (rp *recentProject) Commands() []Command {
	return make([]Command, 0)
}

type recentProjects []*recentProject

func (a recentProjects) Len() int           { return len(a) }
func (a recentProjects) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a recentProjects) Less(i, j int) bool { return a[i].Timestamp > a[j].Timestamp }

// SaveInRecentProjects inserts project or updates existing entry in the
// file storing recent projects
func SaveInRecentProjects(p Project) error {
	rProjects := getRecentProjects()
	inserted := false

	rp := &recentProject{
		IDVal:      p.ID(),
		NameVal:    p.Name(),
		RootDirVal: p.RootDir(),
		Timestamp:  int(time.Now().Unix()),
	}

	for i, rProject := range rProjects {
		if rProject.IDVal == rp.IDVal {
			rProjects[i] = rp
			inserted = true
			break
		}
	}

	if !inserted {
		rProjects = append(rProjects, rp)
	}

	sort.Sort(rProjects)

	err := saveRecentProjects(rProjects)
	if err != nil {
		return err
	}

	return nil
}

// GetRecentProjects returns the ordered list of recent projects.
func GetRecentProjects() []Project {
	rp := getRecentProjects()
	resp := make([]Project, len(rp))
	for i, p := range rp {
		resp[i] = p
	}
	return resp
}

func getRecentProjects() recentProjects {
	rProjects := make(recentProjects, 0)
	jsonBytes, err := ioutil.ReadFile(recentProjectsFile())
	if err == nil {
		err := json.Unmarshal(jsonBytes, &rProjects)
		if err != nil {
			return make(recentProjects, 0)
		}
	}
	return rProjects
}

func recentProjectsFile() string {
	return filepath.Join(cliconfig.Dir(), recentProjectsFileName)
}

func saveRecentProjects(rProjects recentProjects) error {
	jsonBytes, err := json.Marshal(rProjects)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(recentProjectsFile(), jsonBytes, 0644)
	if err != nil {
		return err
	}
	return nil
}
