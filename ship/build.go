package ship

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"strings"
	"time"
)

type Build struct {
	Name     string            `json:"package"`
	Filename string            `json:"file"`
	User     string            `json:"by"`
	When     time.Time         `json:"when"`
	Versions map[string]string `json:"versions"`
}

func NewBuild(command, wd string) (result *Build, err error) {
	p, err := NewProject(command, wd)
	if err != nil {
		return
	}

	d, err := p.Dependencies()
	if err != nil {
		return
	}

	u, err := user.Current()
	if err != nil {
		return
	}

	result = &Build{
		Name:     p.Name,
		Filename: p.Filename,
		User:     u.Username,
		Versions: d,
	}

	return
}

func RequestBuild(url, command, wd string) (version string, err error) {
	b, err := NewBuild(command, wd)
	if err != nil {
		return
	}

	output, err := b.Send(url)
	if err != nil {
		return
	}

	version = strings.TrimSpace(string(output))
	return
}

func (b *Build) Send(url string) (result []byte, err error) {
	data := &bytes.Buffer{}

	err = json.NewEncoder(data).Encode(b)
	if err != nil {
		return
	}

	req, err := http.Post(url+"/request/build", "application/json", data)
	if err != nil {
		return
	}

	body := &bytes.Buffer{}

	_, err = io.Copy(io.MultiWriter(os.Stdout, body), req.Body)
	if err != nil {
		return
	}

	fmt.Println()
	result = body.Bytes()
	return
}
