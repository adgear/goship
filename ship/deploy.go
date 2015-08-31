package ship

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/user"
	"time"
)

type Deploy struct {
	Name     string    `json:"package"`
	Filename string    `json:"file"`
	User     string    `json:"by"`
	When     time.Time `json:"when"`
	Version  string    `json:"version"`
	Targets  []string  `json:"targets"`
	Logs     []string  `json:"logs"`
}

func NewDeploy(command, wd string, targets []string) (result *Deploy, err error) {
	p, err := NewProject(command, wd)
	if err != nil {
		return
	}

	u, err := user.Current()
	if err != nil {
		return
	}

	result = &Deploy{
		Name:     p.Name,
		Filename: p.Filename,
		User:     u.Username,
		Targets:  targets,
	}

	return
}

func RequestDeploy(url, command, wd, version string, targets []string) (err error) {
	d, err := NewDeploy(command, wd, targets)
	if err != nil {
		return
	}

	d.Version = version

	err = d.Send(url)
	return
}

func RequestRollback(url, command, wd string, targets []string) (err error) {
	d, err := NewDeploy(command, wd, targets)
	if err != nil {
		return
	}

	err = d.Send(url)
	return
}

func (d *Deploy) Send(url string) (err error) {
	data := &bytes.Buffer{}

	err = json.NewEncoder(data).Encode(d)
	if err != nil {
		return
	}

	req, err := http.Post(url+"/request/deploy", "application/json", data)
	if err != nil {
		return
	}

	_, err = io.Copy(os.Stdout, req.Body)
	return
}
