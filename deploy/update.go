// Copyright (c) 2015 Datacratic. All rights reserved.
package deploy

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
)

var Version string

func init() {
	if Version == "" {
		return
	}

	buf := bytes.Buffer{}
	if err := json.Indent(&buf, []byte(Version), "", "  "); err != nil {
		log.Println("version:", err)
	} else {
		log.Println("version:", buf.String())
	}
}

type Update struct {
	Address string
	Servers []string

	version string
	once    sync.Once
}

func (u *Update) Start() (err error) {
	u.once.Do(u.initialize)

	// install the handle for deployments
	http.Handle("/deploy/", u)

	// notify any build servers of our existence
	cmd := struct {
		Name    string `json:"app"`
		URL     string `json:"url"`
		Version string `json:"md5"`
	}{
		Name:    path.Base(os.Args[0]),
		URL:     u.Address,
		Version: u.version,
	}

	body := &bytes.Buffer{}
	err = json.NewEncoder(body).Encode(&cmd)
	if err != nil {
		return
	}

	publish := func(host string) {
		_, err := http.Post("http://"+host+"/app/instance", "application/json", body)
		if err != nil {
			log.Println(err)
		}
	}

	for _, item := range u.Servers {
		go publish(item)
	}

	return
}

func (u *Update) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.once.Do(u.initialize)

	if strings.HasSuffix(r.URL.Path, "/version") {
		io.Copy(ioutil.Discard, r.Body)
		r.Body.Close()
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, u.version)
		return
	}

	if strings.HasSuffix(r.URL.Path, "/new") {
		w.Header().Set("Content-Type", "text/plain")

		q := struct {
			URL string `json:"url"`
			MD5 string `json:"md5"`
		}{}

		err := json.NewDecoder(r.Body).Decode(&q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		err = u.update(q.URL, q.MD5)
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}

		fmt.Fprintf(w, "%s updated to version %s\n", os.Args[0], q.MD5)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		os.Exit(0)
	}

	http.NotFound(w, r)
}

func (u *Update) initialize() {
	binary, err := ioutil.ReadFile(os.Args[0])
	if err != nil {
		log.Println("can't read", os.Args[0])
		return
	}

	u.version = fmt.Sprintf("%x", md5.Sum(binary))
}

func (u *Update) update(url, version string) (err error) {
	if version == u.version {
		fmt.Errorf("already at version %s", version)
		return
	}

	// get a new version
	name, err := u.download(url, version)
	if err != nil {
		return
	}

	// atomic
	err = os.Rename(name, os.Args[0])
	return
}

func (u *Update) download(url, version string) (result string, err error) {
	log.Println("updating using", url)

	// get the archive from server over HTTP
	r, err := http.Get(url)
	if err != nil {
		return
	}

	defer r.Body.Close()

	z, err := gzip.NewReader(r.Body)
	if err != nil {
		return
	}

	defer z.Close()

	f, err := os.Create(os.Args[0] + ".update")
	if err != nil {
		return
	}

	_, err = io.Copy(f, z)
	if err != nil {
		return
	}

	f.Close()

	// validate version
	binary, err := ioutil.ReadFile(f.Name())
	if err != nil {
		return
	}

	value := fmt.Sprintf("%x", md5.Sum(binary))
	if value != version {
		err = fmt.Errorf("checksum failed: expected '%s' instead of '%s'", version, value)
		return
	}

	// enable execution
	if err = os.Chmod(f.Name(), 0755); err != nil {
		return
	}

	result = f.Name()
	return
}
