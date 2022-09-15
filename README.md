<img src="docs/cloudvelo-logo.svg" width="128">

## How to build

This version of Velociraptor depends on the open source codebase as
used on GitHub. The GitHub repo is included as a git submodule. We use
the same GUI so we need to have the React App built. Therefore we cant
use a simple go get to install the dependency.

1. First make sure the git submodule is cloned properly

```
git submodule update --init
```

2. Next build the Velociraptor GUI

```
make assets
```

3. Finally we can build the Cloud version by running make in the top level.

```
make linux_musl
```

You will find the binary in `./output/cvelociraptor`


## Try it out with Docker

Alternatively build the Docker image

```
make docker
```

Start the docker test system

```
cd Docker
make up
```

Clear the docker system

```
make clean
```

The Makefile contains startup commands for all components.


## Notes

In the codebase and below we use the term Elastic to refer to the
opensource backend database which originally was managed by Elastic
Inc. Recently, the original Elastic database was split into an
opensource project (https://opensearch.org/) and a non-open source
database offered by Elastic Inc. Further, the Elastic maintained Go
client libraries refuse to connect to the open source database.

As such, we need to decide which flavor of Elastic to support moving
forward. As an open source project we prefer to support open source
dependencies, so this project only supports the opensearch backend.

Any references to Elastic in the codebase or documentation actually
refer to opensearch and that is the only database that is supported at
this time.
