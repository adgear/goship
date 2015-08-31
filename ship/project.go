package ship

import (
	"fmt"
	"go/build"
	"log"
	"os/exec"
	"path"
	"strings"
)

type Project struct {
	Name     string
	Filename string

	dependencies map[string]*build.Package
	repositories map[string]string
}

func NewProject(name, wd string) (p *Project, err error) {
	// command?
	pkg, err := build.Import(name, wd, 0)
	if err != nil {
		return
	}

	if !pkg.IsCommand() {
		err = fmt.Errorf("project must be a command")
		return
	}

	src := pkg.SrcRoot + "/"
	if !strings.HasPrefix(pkg.Dir, src) {
		err = fmt.Errorf("unable to figure out project name")
		return
	}

	dir := strings.TrimPrefix(pkg.Dir, src)

	// create the new project
	p = &Project{
		Name:         dir,
		Filename:     path.Base(dir),
		dependencies: make(map[string]*build.Package),
		repositories: make(map[string]string),
	}

	return
}

func (p *Project) Dependencies() (result map[string]string, err error) {
	// get package dependencies
	if err = p.include(p.Name); err != nil {
		return
	}

	// get workspace git SHA1 from repositories
	for _, pkg := range p.dependencies {
		if pkg.Goroot {
			continue
		}

		// figure out the path
		run := func(args ...string) (result []byte, err error) {
			cmd := exec.Command("git", args...)
			cmd.Dir = pkg.Dir
			if result, err = cmd.Output(); err != nil {
				err = fmt.Errorf("git %s\n%s", strings.Join(args, " "), err.Error())
			}

			return
		}

		var output []byte
		output, err = run("rev-parse", "--show-toplevel")
		if err != nil {
			return
		}

		src := pkg.SrcRoot + "/"
		dir := strings.TrimSpace(string(output))
		git := strings.TrimPrefix(dir, src)

		// already done?
		if _, ok := p.repositories[git]; ok {
			continue
		}

		// get the current SHA1
		p.repositories[git], err = p.commit(git, dir)
		if err != nil {
			return
		}
	}

	result = p.repositories
	return
}

func (p *Project) include(name string) (err error) {
	pkg, err := build.Import(name, "", 0)
	if err != nil {
		return
	}

	p.dependencies[pkg.ImportPath] = pkg

	// get all package imports
	list := pkg.Imports
	list = append(list, pkg.TestImports...)

	for _, item := range list {
		if _, ok := p.dependencies[item]; ok {
			continue
		}

		if pkg.Goroot {
			continue
		}

		// recurse
		if err = p.include(item); err != nil {
			return
		}
	}

	return
}

func (p *Project) commit(name, repo string) (result string, err error) {
	// git
	run := func(args ...string) (result []byte, err error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		if result, err = cmd.Output(); err != nil {
			err = fmt.Errorf("git %s\n%s", strings.Join(args, " "), err.Error())
		}

		return
	}

	// anything pending?
	status, err := run("status", "--porcelain", "--untracked-files=no")
	if err != nil {
		return
	}

	if len(status) != 0 {
		err = fmt.Errorf("repository '%s' is dirty", repo)
		return
	}

	// anything ahead?
	ahead, err := run("rev-list", "@{u}..HEAD")
	if err != nil {
		return
	}

	if len(ahead) != 0 {
		log.Printf("push '%s'\n", name)

		// let's try to push
		_, err = run("push", "--ff-only")
		if err != nil {
			return
		}
	}

	// get HEAD
	hash, err := run("rev-parse", "HEAD")
	if err != nil {
		return
	}

	result = strings.TrimSpace(string(hash))
	return
}
