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

## Asciicast :video_camera:

[![asciicast](https://asciinema.org/a/Es8hpa6rq82ov7cDM6bZTVyCT.svg)](https://asciinema.org/a/Es8hpa6rq82ov7cDM6bZTVyCT)

## Installation

### Docker Engine

First, ensure that Docker Engine is installed.

Linux: See https://docs.docker.com/engine/install/.

MacOS: https://docs.docker.com/desktop/setup/install/mac-install/

### Octoplex

#### Homebrew

Octoplex can be installed using Homebrew on MacOS or Linux.

```
$ brew tap rfwatson/octoplex
$ brew install octoplex
```

#### From Github

Alternatively, grab the latest build for your platform from the [releases page](https://github.com/rfwatson/octoplex/releases).

Unarchive the `octoplex` binary and copy it somewhere in your $PATH.

## Usage

Launch the `octoplex` binary.

```
$ octoplex
```

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

then either install a CA‑signed cert for your RTMPS host, or import your
self‑signed cert into your OS’s trusted store. See the
[configuration](#Configuration) section below.

### Restreaming with FFmpeg

#### RTMP

```
$ ffmpeg -i input.mp4 -c copy -f flv rtmp://localhost:1935/live
```

#### RTMPS

```
$ ffmpeg -i input.mp4 -c copy -f flv rtmps://localhost:1936/live
```

### Subcommands

Subcommand|Description
---|---
None|Launch the terminal user interface
`print-config`|Echo the path to the configuration file to STDOUT
`edit-config`|Edit the configuration file in $EDITOR
`version`|Print the version
`help`|Print help screen

### Configuration

Octoplex stores configuration state in a simple YAML file. (See [above](#subcommands) for its location.)

Sample configuration:

```yaml
logfile:
  enabled: true                        # defaults to false
  path: /path/to/logfile               # defaults to $XDG_STATE_HOME/octoplex/octoplex.log
sources:
  mediaServer:
    streamKey: live                    # defaults to "live"
    host: rtmp.example.com             # defaults to "localhost"
    tls:                               # optional TLS settings; RTMPS support is automatic.
      cert: /etc/mycert.pem            # If you omit cert/key, a self-signed keypair will be
      key: /etc/mykey.pem              # generated using the `host` value above.
    rtmp:
      enabled: true                    # defaults to false
      ip: 127.0.0.1                    # defaults to 127.0.0.1
      port: 1935                       # defaults to 1935
    rtmps:
      enabled: true                    # defaults to false
      ip: 0.0.0.0                      # defaults to 127.0.0.1
      port: 1936                       # defaults to 1936
destinations:
  - name: YouTube                      # Destination name, used only for display
    url: rtmp://rtmp.youtube.com/12345 # Destination  URL with stream key
  - name: Twitch.tv
    url: rtmp://rtmp.youtube.com/12345
  # other destinations here
```

:information_source: It is also possible to add and remove destinations directly from the
terminal user interface.

:warning: `sources.mediaServer.rtmp.ip` must be set to a valid IP address if
you want to accept connections from other hosts. Leave it blank to bind only to
localhost (`127.0.0.1`) or use `0.0.0.0` to bind to all network interfaces.

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
