# Octoplex :octopus:

[![build status](https://github.com/rfwatson/octoplex/actions/workflows/build.yml/badge.svg)](https://github.com/rfwatson/octoplex/actions/workflows/build.yml)
[![scan status](https://github.com/rfwatson/octoplex/actions/workflows/codeql.yml/badge.svg)](https://github.com/rfwatson/octoplex/actions/workflows/codeql.yml)
![GitHub Release](https://img.shields.io/github/v/release/rfwatson/octoplex)
[![Go Report Card](https://goreportcard.com/badge/git.netflux.io/rob/octoplex)](https://goreportcard.com/report/git.netflux.io/rob/octoplex)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

Octoplex is a Docker-native live video restreamer.

* Restream RTMP/RTMPS to unlimited destinations
* Broadcast using OBS or any standard tool
* Add and remove destinations on-the-fly
* Automatic reconnections on drop
* Terminal UI with live metrics and health status
* Powered by FFmpeg, Docker & other open source tools

:warning: **Security warning:** Octoplex's security hardening is a
work-in-progress. For now it is only suitable for running on localhost or
behind a trusted private network.

## How it works

```
         +------------------+             +-------------------+
         |      OBS          |  ---->     |     Octoplex      |
         | (Video Capture)   |   RTMP     |                   |
         +------------------+             +-------------------+
                                                 |
                                                 | Restream to multiple destinations
                                                 v
              +------------+     +------------+     +------------+     +--------------+
              |  Twitch.tv |     |   YouTube  |     | Facebook   |     |  Other       |
              +------------+     +------------+     +------------+     | Destinations |
                                                                       +--------------+
```

## Installation

### Docker Engine (only if you'll run Octoplex locally)

Linux: See https://docs.docker.com/engine/install/.

macOS: https://docs.docker.com/desktop/setup/install/mac-install/

### Octoplex

#### Homebrew

Octoplex can be installed using Homebrew on macOS or Linux.

```
brew tap rfwatson/octoplex
brew install octoplex
```

#### From GitHub

Alternatively, grab the latest build for your platform from the [releases page](https://github.com/rfwatson/octoplex/releases).

Unarchive the `octoplex` binary and copy it somewhere in your $PATH.

#### With Docker

See [Running with Docker](#running-with-docker).

## Starting Octoplex

Octoplex can run as a single process (all-in-one), or in a client/server pair.

Mode|Pick this when you...
---|---
[All-in-one](#all-in-one)|Are testing locally, debugging, or streaming from the same machine that runs Docker (e.g. your laptop).
[Client/server](#clientserver)|Want the server on a remote host (e.g., cloud VM) for long-running or unattended streams.

### All-in-one

```shell
octoplex run
```

Starts the Octoplex server **and** the terminal UI in one process.

_Docker must be running on the same machine._

### Client/server

1. **Start the server** (on the host that has Docker):

```shell
octoplex server start
```

2. **Connect the client** (from your laptop or any host):

```shell
octoplex client start # --host my.remotehost.com if on a different host
```

`client start` is a lightweight TUI and **does not need Docker to be installed**.

3. **Stop the server** (and clean up any Docker resources) on the remote host:

```shell
octoplex server stop
```

## Interacting with Octoplex

### Command-line interface (CLI)

Besides the interactive TUI, you can manage Octoplex with one-off command-line calls.

Don't forget to replace `<PLACEHOLDER>` strings with your own values.

#### List destinations

```shell
octoplex client destination list
```

#### Add a destination

```shell
octoplex client destination add --name "<NAME>" --url "<RTMP_URL>"
```

Make a note of the destination ID that is printed to the terminal, e.g. `036e2a81-dc85-4758-ab04-303849f35cd3`.

#### Update a destination

```shell
octoplex client destination update --id "<DESTINATION_ID>" --name "<NAME>" --url "<RTMP_URL>"
```

#### Start a destination

```shell
octoplex client destination start --id "<DESTINATION_ID>"
```

#### Stop a destination

```shell
octoplex client destination stop --id "<DESTINATION_ID>"
```

#### Remove a destination

```shell
octoplex client destination remove --id "<DESTINATION_ID>"
```

> **:information_source: Tip** Pass `--force` (or `-f`) to remove the destination even if it's live.

### Subcommand reference

Subcommand|Description
---|---
`octoplex run`|Launch both server and client in a single process
`octoplex server start`|Start the Octoplex server
`octoplex server stop`|Stop the Octoplex server
`octoplex client start`|Start the Octoplex TUI client
`octoplex client destination list`|List existing destinations
`octoplex client destination add`|Add a destination
`octoplex client destination update`|Update a destination
`octoplex client destination remove`|Remove a destination
`octoplex client destination start`|Start streaming to a destination
`octoplex client destination stop`|Stop streaming to a destination
`octoplex version`|Print the version
`octoplex help`|Print help screen

### Server flags

Flag|Alias|Modes|Env var|Default|Description
---|---|---|---|---|---
`--help`|`-h`|All|||Show help
`--data-dir`||`server` `all-in-one`|`OCTO_DATA_DIR`|`$HOME/.local/state/octoplex` (Linux) or`$HOME/Library/Caches/octoplex` (macOS)|Directory for storing persistent state and logs
`--listen-addr`|`-a`|`server`|`OCTO_LISTEN_ADDR`|`127.0.0.1:50051`|Listen address for gRPC API.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. Use `0:0.0.0:50051` to bind to all network interfaces.
`--hostname`|`-H`|`server`|`OCTO_HOSTNAME`|`localhost`|DNS name of server
`--tls-cert`||`server` `all-in-one`|`OCTO_TLS_CERT`||Path to custom TLS certifcate (PEM-encoded, must be valid for `hostname`). Used for gRPC and RTMPS connections.
`--tls-key`||`server` `all-in-one`|`OCTO_TLS_KEY`||Path to custom TLS key (PEM-encoded, must be valid for `hostname`). Used for gRPC and RTMPS connections.
`--in-docker`||`server`|`OCTO_DOCKER`|`false`|Configure Octoplex to run inside Docker
`--stream-key`||`server` `all-in-one`|`OCTO_STREAM_KEY`|`live`|Stream key, e.g. `rtmp://rtmp.example.com/live`
`--rtmp-enabled`||`server` `all-in-one`||`true`|Enable RTMP server
`--rtmp-listen-addr`||`server` `all-in-one`||`127.0.0.1:1935`|Listen address for RTMP sources.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. See `--listen-addr`.
`--rtmps-enabled`||`server` `all-in-one`||`false`|Enable RTMPS server
`--rtmps-listen-addr`||`server` `all-in-one`||`127.0.0.1:1936`|Listen address for RTMPS sources.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. See `--listen-addr`.

### Client flags

Flag|Alias|Default|Description
---|---|---|---
`--help`|`-h`|All|||Show help
`--host`|`-H`|`localhost:50051`|Remote Octoplex server to connect to
`--tls-skip-verify`||`false`|Skip TLS certificate verification (insecure)
`--log-file`|||Path to log file

### All-in-one mode

:information_source: When running in all-in-one mode (`octoplex run`) some flags may be overridden or unavailable.

## Restreaming with Octoplex

### Restreaming with OBS

#### RTMP

Use the following OBS stream configuration:

![OBS streaming settings for RTMP](/assets/obs1.png)

#### RTMPS

Or to connect with RTMPS:

![OBS streaming settings for RTMPS](/assets/obs2.png)

:warning: Warning: OBS may not accept self‑signed certificates.

If you see the error

> "The RTMP server sent an invalid SSL certificate."

then either install a CA‑signed TLS certificate for your RTMPS host, or import
your self‑signed cert into your OS’s trusted store. See the [server flags](#server-flags)
section above.

### Restreaming with FFmpeg

#### RTMP

```
ffmpeg -i input.mp4 -c copy -f flv rtmp://localhost:1935/live
```

#### RTMPS

```
ffmpeg -i input.mp4 -c copy -f flv rtmps://localhost:1936/live
```

## Advanced

### Running with Docker

`octoplex server` can be run from a Docker image on any Docker engine.

:warning: By design, Octoplex needs to launch and terminate Docker containers
on your host. If you run Octoplex inside Docker with a bind-mounted Docker
socket, it effectively has root-level access to your server. Evaluate the
security trade-offs carefully. If you’re unsure, consider running Octoplex
directly on the host rather than in a container.

#### `docker run`

Run the Octoplex gRPC server on all interfaces (port 50051):

```shell
docker run \
  --name octoplex                              \
  -v octoplex-data:/data                       \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e OCTO_LISTEN_ADDR=":50051"                 \
  -p 50051:50051                               \
  --restart unless-stopped                     \
  ghcr.io/rfwatson/octoplex:latest
```

:information_source: Note: Running the client &mdash; or the all-in-one server
mode &mdash; from Docker is not recommended. Install Octoplex natively via Homebrew or
download a release from GitHub instead. See [Installation](#Installation) for
details.

#### `docker-compose`

See [docker-compose.yaml](/docker-compose.yaml).

## Contributing

See [CONTRIBUTING.md](/CONTRIBUTING.md).

### Bug reports

Open bug reports [on GitHub](https://github.com/rfwatson/octoplex/issues/new).

## Acknowledgements

Octoplex is built on and/or makes use of other free and open source software,
most notably:

Name|License|URL
---|---|---
Docker|`Apache 2.0`|[GitHub](https://github.com/moby/moby/tree/master/client)
FFmpeg|`LGPL`|[Website](https://www.ffmpeg.org/legal.html)
MediaMTX|`MIT`|[GitHub](https://github.com/bluenviron/mediamtx)
tview|`MIT`|[GitHub](https://github.com/rivo/tview)

## Licence

Octoplex is released under the [AGPL v3](https://github.com/rfwatson/octoplex/blob/main/LICENSE) license.
