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
	Name      string `json:"name"`
	RootDir   string `json:"root"`
	ID        string `json:"id"`
	Timestamp int    `json:"t"`
}

type recentProjects []*recentProject

func (a recentProjects) Len() int           { return len(a) }
func (a recentProjects) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a recentProjects) Less(i, j int) bool { return a[i].Timestamp > a[j].Timestamp }

func (p *Project) SaveInRecentProjects() error {
	rProjects := getRecentProjects()
	inserted := false
	rp := &recentProject{Name: p.Config.Name, ID: p.Config.ID, RootDir: p.RootDirPath, Timestamp: int(time.Now().Unix())}

	for i, rProject := range rProjects {
		if rProject.ID == p.Config.ID {
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

func GetRecentProjects() []*Project {
	rp := getRecentProjects()
	resp := make([]*Project, len(rp))

	for i, p := range rp {
		resp[i] = &Project{Config: Config{ID: p.ID, Name: p.Name}, RootDirPath: p.RootDir}
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
