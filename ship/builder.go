package ship

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"strings"
)

type Builder struct {
	Workspace string
	Root      string
	Name      string
	Build     *Build

	output *os.File
	logger *log.Logger
}

func (b *Builder) Make() (result string, err error) {
	b.output, err = os.Create(path.Join(b.Workspace, "log"))
	if err != nil {
		return
	}

	// create log file
	b.logger = log.New(b.output, "", log.Ldate|log.Lmicroseconds)
	b.logger.Println("workspace", b.Workspace)

	err = b.checkout()
	if err != nil {
		return
	}

	err = b.compile()
	if err != nil {
		return
	}

	err = b.checksum()
	if err != nil {
		return
	}

	err = b.save()
	if err != nil {
		return
	}

	// save the state
	state, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return
	}

	err = ioutil.WriteFile(path.Join(b.Root, b.Name+".json"), state, 0644)
	if err != nil {
		return
	}

	b.logger.Printf("done")
	b.output.Close()

	// save the build logs
	err = os.Rename(b.output.Name(), path.Join(b.Root, b.Name+".build"))
	if err != nil {
		return
	}

	result = b.Name
	return
}

func (b *Builder) checkout() (err error) {
	results := make(chan error)

	clone := func(name, hash string) {
		output := &bytes.Buffer{}
		logger := log.New(output, "", log.Ldate|log.Lmicroseconds)
		defer io.Copy(b.output, output)

		git := func(path string, args ...string) (err error) {
			shell := fmt.Sprintf("git %s\n", strings.Join(args, " "))
			logger.Printf(shell)
			cmd := exec.Command("git", args...)
			cmd.Dir = path
			cmd.Stdout = output
			cmd.Stderr = output
			if err = cmd.Run(); err != nil {
				err = fmt.Errorf("%s\n%s", shell, err.Error())
			}

			return
		}

		dir := path.Join(b.Workspace, "src", name)

		// clone
		url := "git@" + strings.Replace(name, "/", ":", 1) + ".git"
		err := git(b.Workspace, "clone", "-q", "--no-checkout", url, path.Join("src", name))
		if err != nil {
			logger.Println(err)
			results <- err
			return
		}

		logger.Printf("cd %s\n", dir)

		// checkout
		err = git(dir, "checkout", "-q", hash)
		if err != nil {
			logger.Println(err)
		}

		results <- err
	}

	for name, hash := range b.Build.Versions {
		go clone(name, hash)
	}

	for i, n := 0, len(b.Build.Versions); i < n; i++ {
		if err = <-results; err != nil {
			return
		}
	}

	return
}

func (b *Builder) compile() (err error) {
	env := []string{"GOROOT=" + os.ExpandEnv("$GOROOT"), "GOPATH=" + b.Workspace}

	// add the build information
	ver, err := json.Marshal(b.Build)
	if err != nil {
		return
	}

	ld := fmt.Sprintf("-X github.com/datacratic/goship/deploy.Version '%q'", ver)
	log.Println("go install", ld)

	// invoke the compiler
	b.logger.Println(strings.Join(env, " "), "go install", b.Build.Name)
	cmd := exec.Command("go", "install", "-ldflags", ld, b.Build.Name)
	cmd.Dir = b.Workspace
	cmd.Env = env
	cmd.Stdout = b.output
	cmd.Stderr = b.output
	if err = cmd.Run(); err != nil {
		err = fmt.Errorf("go install\n", err.Error())
	}

	return
}

func (b *Builder) checksum() (err error) {
	w := &bytes.Buffer{}

	f, err := os.Open(path.Join(b.Workspace, "bin", b.Build.Filename))
	if err != nil {
		return
	}

	b.logger.Println("computing MD5 checksum...")

	_, err = io.Copy(w, f)
	if err != nil {
		return
	}

	f.Close()

	// got the name
	b.Name = fmt.Sprintf("%x", md5.Sum(w.Bytes()))
	b.logger.Println(b.Name)
	return
}

func (b *Builder) save() (err error) {
	f, err := os.Open(path.Join(b.Workspace, "bin", b.Build.Filename))
	if err != nil {
		return
	}

	z, err := os.Create(path.Join(b.Root, b.Name+".gz"))
	if err != nil {
		return
	}

	w := gzip.NewWriter(z)
	b.logger.Println("saving to", z.Name())

	_, err = io.Copy(w, f)
	if err != nil {
		return
	}

	f.Close()
	w.Close()
	z.Close()
	return
}
