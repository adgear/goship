goship
======

This package contains a Go build server with a few tools to use it.

```
go get github.com/datacratic/goship
```

Then, create an empty folder that the build server can use. When invoking the Go compiler, it will define $GOPATH as the working folder.

```
mkdir ~/build
cd $_
$GOPATH/bin/shipd --address :8080
```

Now, the server is ready to accept requests to build a command package. The `ship` command tracks all the dependencies and queries the `git` commit hash currently checked-out. The request is then sent to the build server that returns the ID of that build.

The build server can be specified using `$GOBUILDSERVER`.

```
export GOBUILDSERVER="http://127.0.0.1:8080"
```

For the build to be reproductible, used repositories are required to have:

- no modified files
- no commits ahead of their remote branch

So as a convinience, local commits will be pushed when possible i.e. if the remote branch can be fast-forward using the local branch.

The tool begin deployed can be updated when specified. Simply add the following to your application:

```
u := deploy.Update{
    Address: "server.client.domain.com:6060",
    Servers: []string{"http://build-server.domain.com:8080"},
}

u.Start()
log.Fatal(http.ListenAndServe(":6060", nil))
```

This will connect to the build server and register that instance for deployments. When invoking `ship`, you can specify the host where it should be deployed.
