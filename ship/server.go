package ship

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"
)

type Server struct {
	Host     string
	Root     string
	Builds   string
	Builders map[string]*Builder
	Requests map[string]*Requests

	apps map[string]map[string]*App
	once sync.Once
	feed chan func()

	overview *template.Template
}

type Requests struct {
	Name        string
	Root        string
	Builds      []*Build
	Deployments []*Deploy

	output *os.File
	stream *bufio.Writer
}

type App struct {
	Name    string `json:"app"`
	URL     string `json:"url"`
	Version string `json:"md5"`
}

func (s *Server) initialize() {
	s.Builders = make(map[string]*Builder)
	s.Requests = make(map[string]*Requests)
	s.apps = make(map[string]map[string]*App)

	s.readBuilds()
	s.readRequests()
	s.readApps()

	var err error
	s.overview, err = template.New("overview").Parse(htmlOverview)
	if err != nil {
		log.Fatal(err)
	}

	// process events
	s.feed = make(chan func())
	go func() {
		for f := range s.feed {
			f()
		}
	}()
}

func (s *Server) readBuilds() {
	s.Builds = path.Join(s.Root, "builds")

	// make sure the root directory for builds exists
	os.MkdirAll(s.Builds, 0755)

	// inspect it
	entries, err := ioutil.ReadDir(s.Builds)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		name := entry.Name()

		// clean possibly corrupted builds
		if entry.IsDir() {
			if err := os.RemoveAll(path.Join(s.Builds, name)); err != nil {
				log.Fatal(err)
			}

			log.Println("removed", name)
			continue
		}

		// import builders
		i := strings.Index(name, ".")
		if i < 0 {
			log.Println("unknown", name)
			continue
		}

		switch name[i+1:] {
		case "json":
			s.readBuilder(name[:i])

		case "build", "gz":
		default:
			log.Println("unknown", name)
		}
	}
}

func (s *Server) readBuilder(name string) {
	b, ok := s.Builders[name]
	if !ok {
		b = &Builder{
			Name: name,
			Root: s.Root,
		}

		s.Builders[name] = b
	}

	file, err := os.Open(path.Join(s.Builds, name+".json"))
	if err != nil {
		log.Fatal(err)
	}

	err = json.NewDecoder(file).Decode(b)
	if err != nil {
		log.Fatal(err)
	}
}

func (s *Server) readRequests() {
	root := path.Join(s.Root, "logs")

	// make sure the root directory for requests exists
	os.MkdirAll(root, 0755)

	// inspect it
	entries, err := ioutil.ReadDir(root)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		// import requests
		i := strings.Index(name, ".")
		if i < 0 {
			log.Println("unknown", name)
			continue
		}

		file, err := os.Open(path.Join(root, name))
		if err != nil {
			log.Fatal(err)
		}

		lines := bufio.NewScanner(file)

		switch name[i+1:] {
		case "build":
			for lines.Scan() {
				b := new(Build)
				if err = json.Unmarshal(lines.Bytes(), b); err != nil {
					break
				}

				r := s.get(b.Filename)
				r.Builds = append(r.Builds, b)
			}

		case "deploy":
			for lines.Scan() {
				d := new(Deploy)
				if err = json.Unmarshal(lines.Bytes(), d); err != nil {
					break
				}

				r := s.get(d.Filename)
				r.Deployments = append(r.Deployments, d)
			}

		}

		if err != nil {
			log.Fatal("failed to parse", file.Name(), err)
		}

		if err = lines.Err(); err != nil {
			log.Fatal(err)
		}
	}
}

func (s *Server) readApps() {
	root := path.Join(s.Root, "apps")

	// make sure the root directory for requests exists
	os.MkdirAll(root, 0755)

	// inspect it
	entries, err := ioutil.ReadDir(root)
	if err != nil {
		log.Fatal(err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()

		file, err := os.Open(path.Join(root, name))
		if err != nil {
			log.Fatal(err)
		}

		items := make(map[string]*App)

		err = json.NewDecoder(file).Decode(&items)
		if err != nil {
			log.Fatal(err)
		}

		s.apps[name] = items
	}
}
func (s *Server) Write(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html")

	done := make(chan struct{})
	s.feed <- func() {
		err := s.overview.Execute(w, s)
		if err != nil {
			log.Println(err)
		}

		close(done)
	}

	<-done
}

func (s *Server) Start() (err error) {
	s.once.Do(s.initialize)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		s.Write(w)
	})

	http.HandleFunc("/builds/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !strings.HasSuffix(r.URL.Path, ".gz") {
			http.NotFound(w, r)
			return
		}

		http.ServeFile(w, r, path.Join(s.Root, r.URL.Path))
	})

	decode := func(w http.ResponseWriter, r *http.Request, q interface{}) {
		if r.Method != "POST" {
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := json.NewDecoder(r.Body).Decode(q)
		r.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/plain")

		err = s.Process(w, q)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	}

	http.HandleFunc("/request/build", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, new(Build))
	})

	http.HandleFunc("/request/deploy", func(w http.ResponseWriter, r *http.Request) {
		decode(w, r, new(Deploy))
	})

	http.HandleFunc("/app/instance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "405 method not allowed", http.StatusMethodNotAllowed)
			return
		}

		item := new(App)

		err := json.NewDecoder(r.Body).Decode(&item)
		r.Body.Close()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		s.feed <- func() {
			instances, ok := s.apps[item.Name]
			if !ok {
				instances = make(map[string]*App)
				s.apps[item.Name] = instances
			}

			instances[item.URL] = item
			body, err := json.MarshalIndent(&instances, "", "\t")
			if err != nil {
				log.Fatal(err)
			}

			err = ioutil.WriteFile(path.Join(s.Root, "apps", item.Name), body, 0666)
			if err != nil {
				log.Fatal(err)
			}
		}
	})
	return
}

func (s *Server) Process(w io.Writer, r interface{}) (err error) {
	switch r := r.(type) {
	case *Build:
		err = s.makeBuilder(w, r)
	case *Deploy:
		err = s.makeDeploy(w, r)
	default:
		err = fmt.Errorf("unknown type of request: %T", r)
	}

	return
}

func (s *Server) get(name string) *Requests {
	r, ok := s.Requests[name]
	if !ok {
		r = &Requests{
			Root: path.Join(s.Root, "logs"),
			Name: name,
		}

		s.Requests[name] = r
	}

	return r
}

func (s *Server) makeBuilder(w io.Writer, b *Build) (err error) {
	s.once.Do(s.initialize)

	// record the time when the request was received
	b.When = time.Now().UTC()

	// new workspace
	dir, err := ioutil.TempDir(s.Builds, b.Filename+"-")
	if err != nil {
		return
	}

	// keep track of the build request
	s.feed <- func() {
		r := s.get(b.Filename)
		r.Builds = append(r.Builds, b)
		r.save(b)
	}

	builder := &Builder{
		Workspace: dir,
		Root:      s.Builds,
		Build:     b,
	}

	// build
	name, err := builder.Make()
	if err != nil {
		return
	}

	// keep track of the build
	s.feed <- func() {
		s.Builders[name] = builder
	}

	io.WriteString(w, name)
	return
}

func (s *Server) makeDeploy(w io.Writer, d *Deploy) (err error) {
	s.once.Do(s.initialize)

	hosts, ok := s.apps[d.Filename]
	if ok {
		return
	}

	// record the time when the request was received
	d.When = time.Now().UTC()

	r := struct {
		URL string `json:"url"`
		MD5 string `json:"md5"`
	}{
		URL: s.Host + "/versions/" + d.Version + ".gz",
		MD5: d.Version,
	}

	body, err := json.Marshal(&r)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan string, len(hosts))

	update := func(host string) {
		r, err := http.Post("http://"+host+"/deploy/new", "application/json", bytes.NewReader(body))
		if err != nil {
			log.Println(host, err)
			done <- host + " " + err.Error()
			return
		}

		io.Copy(os.Stdout, r.Body)
		done <- host + " OK"
	}

	// send update requests
	n := 0
	for _, item := range d.Targets {
		for host, _ := range hosts {
			if !strings.Contains(host, item) {
				continue
			}

			n++
			go update(host)
		}
	}

	// wait for the requests to complete
	lines := make([]string, n)
	for i := 0; i < n; i++ {
		lines[i] = <-done
		fmt.Fprintf(w, "%s\n", lines[i])
	}

	// keep track of the deployment request
	s.feed <- func() {
		r := s.get(d.Filename)
		d.Logs = lines
		r.Deployments = append(r.Deployments, d)
		r.save(d)
	}

	return
}

func (r *Requests) save(item interface{}) {
	// get type
	kind := strings.ToLower(strings.TrimPrefix(fmt.Sprintf("%T", item), "*ship."))

	// and type filename
	name := fmt.Sprintf("%s-%s.%s", r.Name, time.Now().UTC().Format("20060102"), kind)

	// to get the log file
	filename := path.Join(r.Root, name)
	if r.output == nil || r.output.Name() != filename {
		if r.output != nil {
			r.stream.Flush()
			r.output.Close()
		}

		output, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}

		r.output = output
		r.stream = bufio.NewWriter(r.output)
	}

	err := json.NewEncoder(r.stream).Encode(item)
	if err != nil {
		log.Fatal(err)
	}

	r.stream.Flush()
}
