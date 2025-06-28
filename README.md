# Octoplex :octopus:

[![build status](https://github.com/rfwatson/octoplex/actions/workflows/build.yml/badge.svg)](https://github.com/rfwatson/octoplex/actions/workflows/build.yml)
[![scan status](https://github.com/rfwatson/octoplex/actions/workflows/codeql.yml/badge.svg)](https://github.com/rfwatson/octoplex/actions/workflows/codeql.yml)
![GitHub Release](https://img.shields.io/github/v/release/rfwatson/octoplex)
![Go version](https://img.shields.io/github/go-mod/go-version/rfwatson/octoplex)
[![Go Report Card](https://goreportcard.com/badge/git.netflux.io/rob/octoplex)](https://goreportcard.com/report/git.netflux.io/rob/octoplex)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

Octoplex is a Docker-native live video restreamer.

* Restream RTMP/RTMPS to unlimited destinations
* Add and remove destinations on-the-fly
* Broadcast using OBS or any RTMP encoder
* Automatic reconnections on drop
* Terminal UI with live metrics and health
* Scriptable CLI for automation

## Table of Contents

- [Quick start](#quick-start)
- [How it works](#how-it-works)
- [Installation](#installation)
  - [Docker Engine (only if you'll run Octoplex locally)](#docker-engine-only-if-youll-run-octoplex-locally)
  - [Octoplex](#octoplex)
    - [Homebrew](#homebrew)
    - [From GitHub](#from-github)
    - [With Docker](#with-docker)
- [Starting Octoplex](#starting-octoplex)
  - [All-in-one](#all-in-one)
  - [Client/server](#clientserver)
- [Interacting with Octoplex](#interacting-with-octoplex)
  - [Command-line interface (CLI)](#command-line-interface-cli)
    - [List destinations](#list-destinations)
    - [Add a destination](#add-a-destination)
    - [Update a destination](#update-a-destination)
    - [Start a destination](#start-a-destination)
    - [Stop a destination](#stop-a-destination)
    - [Remove a destination](#remove-a-destination)
  - [Subcommand reference](#subcommand-reference)
  - [Server flags](#server-flags)
  - [Client flags](#client-flags)
  - [All-in-one mode](#all-in-one-mode)
- [Security](#security)
  - [TLS](#tls)
  - [API tokens](#api-tokens)
  - [Incoming streams](#incoming-streams)
- [Restreaming with Octoplex](#restreaming-with-octoplex)
  - [Restreaming with OBS](#restreaming-with-obs)
    - [RTMP](#rtmp)
    - [RTMPS](#rtmps)
  - [Restreaming with FFmpeg](#restreaming-with-ffmpeg)
    - [RTMP](#rtmp-1)
    - [RTMPS](#rtmps-1)
- [Advanced](#advanced)
  - [Running with Docker](#running-with-docker)
    - [`docker run`](#docker-run)
    - [`docker-compose`](#docker-compose)
- [Contributing](#contributing)
  - [Bug reports](#bug-reports)
- [Acknowledgements](#acknowledgements)
- [Licence](#licence)

## Quick start

1. **Install Octoplex**

See the [Installation](#installation) section below.

2. **Launch all-in-one mode**

Starts the server _and_ the terminal UI in a single process &mdash; ideal for local testing.

```shell
octoplex run
```

3. **Point your encoder (OBS, FFmpeg, etc) at the RTMP server:**

Full examples are in [Restreaming with Octoplex](#restreaming-with-octoplex).

```shell
rtmp://localhost:1935/live         # stream key: live
```

Or, if your encoder supports **RTMPS**:

```shell
rtmps://localhost:1936/live        # self-signed TLS certificate by default
```

That's it &mdash; your local restreamer is live.
Add destinations and start relaying from the TUI or CLI; see [Interacting with Octoplex](#interacting-with-octoplex).

## How it works

Octoplex server runs on your Docker host (as a container or daemon process) and
spins up [MediaMTX](https://github.com/bluenviron/mediamtx) and
[FFmpeg](https://ffmpeg.org/) containers that ingest your feed and rebroadcast
it anywhere you point them.

It handles reconnection, TLS, and container wiring so you can focus on your
content.

```
         +------------------+             +-------------------+
         |      OBS          |   ----->   |     Octoplex      |
         |   (Encoder)       |   RTMP/S   |                   |
         +------------------+             +-------------------+
                                                 |
                                                 | Restream to multiple destinations
                                                 v
              +------------+     +------------+     +------------+     +--------------+
              |  Twitch.tv |     |   YouTube  |     | Facebook   |     |  Other       |
              +------------+     +------------+     +------------+     | Destinations |
                                                                       +--------------+
```

:warning: **Warning - alpha software:** Octoplex is in active development.
Features and configuration settings may change between releases. Double-check
your security configuration and exercise extra caution before deploying to
public-facing networks. See the [Security](#security) section for more.

## Installation

### Docker Engine (only if you'll run Octoplex locally)

Linux: See https://docs.docker.com/engine/install/.

macOS: https://docs.docker.com/desktop/setup/install/mac-install/

### Octoplex

#### Homebrew

Octoplex can be installed using Homebrew on macOS or Linux.

```shell
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

2. **Connect the interactive TUI client** (from your laptop or any host):

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

Besides the interactive TUI, you can also control Octoplex with one-off command-line calls.

Don't forget to replace `<PLACEHOLDER>` strings with your own values.

> **:information_source: Tip** Octoplex ships with a self-signed TLS certificate by default. When connecting remotely you'll usually need `--tls-skip-verify` (or `-k`).
> **Warning**: this disables certificate validation, use only on trusted networks.

#### List destinations

```shell
octoplex client destination list --tls-skip-verify
```

#### Add a destination

```shell
octoplex client destination add \
    --name "<NAME>" \
    --url "<RTMP_URL>" \
    --tls-skip-verify
```

Make a note of the destination ID that is printed to the terminal, e.g. `036e2a81-dc85-4758-ab04-303849f35cd3`.

#### Update a destination

```shell
octoplex client destination update \
    --id "<DESTINATION_ID>"  \
    --name "<NAME>" \
    --url "<RTMP_URL>" \
    --tls-skip-verify
```

#### Start a destination

```shell
octoplex client destination start \
    --id "<DESTINATION_ID>"  \
    --tls-skip-verify
```

#### Stop a destination

```shell
octoplex client destination stop \
    --id "<DESTINATION_ID>" \
    --tls-skip-verify
```

#### Remove a destination

```shell
octoplex client destination remove \
    --id "<DESTINATION_ID>" \
    --tls-skip-verify
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
`--listen`|`-l`|`server`|`OCTO_LISTEN`|`127.0.0.1:8080`|Listen address for non-TLS API and web traffic.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. Use `0:0.0.0:8080` to bind to all network interfaces. Pass `none` to disable entirely.
`--listen-tls`|`-a`|`server`|`OCTO_LISTEN_TLS`|`127.0.0.1:8443`|Listen address for TLS API and web traffic.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. Use `0:0.0.0:8443` to bind to all network interfaces. Pass `none` to disable entirely.
`--hostname`|`-H`|`server`|`OCTO_HOSTNAME`|`localhost`|DNS name of server
`--auth`||`server`|`OCTO_AUTH`|`auto`|Authentication mode for clients, one of `none`, `auto` and `token`. See [Security](#security).
`--insecure-allow-no-auth`||`server`|`OCTO_INSECURE_ALLOW_NO_AUTH`|`false`|Allow `--auth=none` when bound to non-local addresses. See [Security](#security).
`--tls-cert`||`server` `all-in-one`|`OCTO_TLS_CERT`||Path to custom TLS certifcate (PEM-encoded, must be valid for `hostname`). Used for gRPC and RTMPS connections.
`--tls-key`||`server` `all-in-one`|`OCTO_TLS_KEY`||Path to custom TLS key (PEM-encoded, must be valid for `hostname`). Used for gRPC and RTMPS connections.
`--in-docker`||`server`|`OCTO_DOCKER`|`false`|Configure Octoplex to run inside Docker
`--stream-key`||`server` `all-in-one`|`OCTO_STREAM_KEY`|`live`|Stream key, e.g. `rtmp://rtmp.example.com/live`
`--rtmp-enabled`||`server` `all-in-one`||`true`|Enable RTMP server
`--rtmp-listen`||`server` `all-in-one`||`127.0.0.1:1935`|Listen address for RTMP sources.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. See `--listen`.
`--rtmps-enabled`||`server` `all-in-one`||`false`|Enable RTMPS server
`--rtmps-listen`||`server` `all-in-one`||`127.0.0.1:1936`|Listen address for RTMPS sources.<br/>:warning: Must be set to a valid IP address to receive connections from other hosts. See `--listen`.
`--image-name-mediamtx`||`server` `all-in-one`|`OCTO_IMAGE_NAME_MEDIAMTX`|`ghcr.io/rfwatson/mediamtx-alpine:latest`|OCI-compatible image for launching MediaMTX
`--image-name-ffmpeg`||`server` `all-in-one`|`OCTO_IMAGE_NAME_FFMPEG`|`ghcr.io/jrottenberg/ffmpeg:7.1-scratch`|OCI-compatible image for launching FFmpeg

### Client flags

Flag|Alias|Default|Description
---|---|---|---
`--help`|`-h`||||Show help
`--host`|`-H`|`localhost:8443`|Remote Octoplex server to connect to
`--tls-skip-verify`|`-k`|`false`|Skip TLS certificate verification (insecure)
`--api-token`|`-t`||API token. See [Security](#security).
`--log-file`|||Path to log file

### All-in-one mode

:information_source: When running in all-in-one mode (`octoplex run`) some flags may be overridden or unavailable.

## Security

Read this section before putting Octoplex on any network you don't fully control.

### TLS

By default, the Octoplex server listens for HTTP and API traffic on ports 8080
(plain text) and 8443 (TLS with a self-signed certificate). Both listeners are
bound to 127.0.0.1 unless explicitly configured otherwise. See [Server
flags](#server-flags) for full configuration options.

When deploying on untrusted networks, ensure that plain-text ports are only
accessible behind a TLS-enabled reverse proxy. To disable non-TLS listeners
entirely, use `--listen=none` with `octoplex server start`, or set the
`OCTO_LISTEN=none` environment variable.

### API tokens

Octoplex automatically protects its internal API whenever it binds to anything other
than localhost.

When you run `octoplex server start`:

* `--auth=auto` (the default): if the API is bound only to **localhost/loopback addresses**, Octoplex requires no authentication; if it's bound to **any other network interface** the server securely generates a random API token, logs it once on startup, hashes it to disk, and requires every client call to include `--api-token "<API_TOKEN>"`.
* `--auth=token`: always require an API token, even on loopback.
* `--auth=none`: disable authentication completely, **but only for localhost binds**. If you set `--auth=none` with any non-loopback API listen addresses you must also pass `--insecure-allow-no-auth` to acknowledge the risk; otherwise the server refuses to start.

> **:information_source: Tip** To regenerate a new API token, delete `token.txt` from the Octoplex data directory, and restart the server. See the `--data-dir` option in [server flags](#server-flags).

### Incoming streams

Octoplex also listens for source streams (RTMP/RTMPS on ports 1935/1936 by
default). These are **not** covered by the API token. To stop anyone from pushing
an unsolicited feed, start the server with a unique stream key:

```shell
octoplex server start --stream-key "<YOUR_UNIQUE_STREAM_KEY>" ...
# or, set OCTO_STREAM_KEY=...
```

See [server flags](#server-flags) for more.

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
your self‑signed cert into your OS's trusted store. See the [server flags](#server-flags)
section above.

### Restreaming with FFmpeg

#### RTMP

```shell
ffmpeg -i input.mp4 -c copy -f flv rtmp://localhost:1935/live
```

#### RTMPS

```shell
ffmpeg -i input.mp4 -c copy -f flv rtmps://localhost:1936/live
```

## Advanced

### Running with Docker

`octoplex server` can be run from a Docker image on any Docker engine.

:warning: By design, Octoplex needs to launch and terminate Docker containers
on your host. If you run Octoplex inside Docker with a bind-mounted Docker
socket, it effectively has root-level access to your server. Evaluate the
security trade-offs carefully. If you're unsure, consider running Octoplex
directly on the host rather than in a container.

:information_source: Note: Running the TUI client from Docker is not
recommended. Install Octoplex natively via Homebrew or download a release from
GitHub instead. See [Installation](#Installation) for details.

#### `docker run`

Run the Octoplex server on all interfaces (ports 8080 and 8443/TLS):

```shell
docker run \
  --name octoplex                              \
  -v octoplex-data:/data                       \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -e OCTO_LISTEN=":8080"                       \
  -e OCTO_LISTEN_TLS=":8443"                   \
  -p 8080:8080                                 \
  -p 8443:8443                                 \
  --restart unless-stopped                     \
  ghcr.io/rfwatson/octoplex:latest
```

#### `docker-compose`

Run the Octoplex server on all interfaces (ports 8080 and 8443/TLS):

```yaml
---
services:
  octoplex:
    image: ghcr.io/rfwatson/octoplex:latest
    container_name: octoplex
    restart: unless-stopped
    volumes:
      - octoplex-data:/data
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      OCTO_LISTEN: "[::]:8080"     # bind to all network interfaces
      OCTO_LISTEN_TLS: "[::]:8443" # bind to all network interfaces
    ports:
      - "8080:8080"
      - "8443:8443"
volumes:
  octoplex-data:
    driver: local
```

See also [docker-compose.yaml](/docker-compose.yaml).

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
