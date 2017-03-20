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
	Project
	Timestamp int `json:"t"`
}

type recentProjects []*recentProject

func (a recentProjects) Len() int           { return len(a) }
func (a recentProjects) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a recentProjects) Less(i, j int) bool { return a[i].Timestamp > a[j].Timestamp }

// SaveInRecentProjects inserts project or updates existing entry in the
// file storing recent projects
func (p *Project) SaveInRecentProjects() error {
	rProjects := getRecentProjects()
	inserted := false

	rp := &recentProject{Project: *p, Timestamp: int(time.Now().Unix())}

	for i, rProject := range rProjects {
		if rProject.ID == rp.ID {
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
func GetRecentProjects() []*Project {
	rp := getRecentProjects()
	resp := make([]*Project, len(rp))
	for i, p := range rp {
		resp[i] = &p.Project
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
