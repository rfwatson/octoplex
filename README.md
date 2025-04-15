# Octoplex :octopus:

![build status](https://github.com/rfwatson/octoplex/actions/workflows/ci-build.yml/badge.svg)
![GitHub Release](https://img.shields.io/github/v/release/rfwatson/octoplex)
[![License: AGPL v3](https://img.shields.io/badge/License-AGPL_v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)

Octoplex is a live video restreamer for the terminal.

* Restream RTMP to unlimited destinations
* Broadcast using OBS and other standard tools
* Add and remove destinations while streaming
* Automatic reconnections
* Terminal user interface with real-time container metrics and health status
* Built on FFmpeg, Docker and other proven free software

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

First, make sure Docker Engine is installed. Octoplex uses Docker to manage
FFmpeg and other streaming tools.

Linux: See https://docs.docker.com/engine/install/.

MacOS: https://docs.docker.com/desktop/setup/install/mac-install/

### Octoplex

Download the latest build for your platform from the [releases page](https://github.com/rfwatson/octoplex/releases).

Unarchive the `octoplex` binary and copy it somewhere in your $PATH.

## Usage

Launch the `octoplex` binary.

```
$ octoplex
```

### Connecting with OBS

To connect with OBS, configure it to stream to `rtmp://localhost:1935/live`.

![OBS streaming settings](/assets/obs1.png)

### Subcommands

Subcommand|Description
---|---
None|Launch the terminal user interface
`print-config`|Echo the path to the configuration file to STDOUT
`edit-config`|Edit the configuration file in $EDITOR
`version`|Print the version
`help`|Print help screen

### Configuration file

Octoplex stores configuration state in a simple YAML file. (See [above](#subcommands) for its location.)

Sample configuration:

```yaml
logfile:
  enabled: true                        # defaults to false
  path: /path/to/logfile               # defaults to $XDG_STATE_HOME/octoplex/octoplex.log
sources:
  rtmp:
    enabled: true                      # must be true
    streamkey: live                    # defaults to "live"
destinations:
  - name: YouTube                      # Destination name, used only for display
    url: rtmp://rtmp.youtube.com/12345 # Destination  URL with stream key
  - name: Twitch.tv
    url: rtmp://rtmp.youtube.com/12345
  # other destinations here
```

:warning: It is also possible to add and remove destinations directly from the
terminal user interface.

## Contributing

### Bug reports

Open bug reports [on GitHub](https://github.com/rfwatson/octoplex/issues/new).

### Pull requests

Pull requests are welcome.

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
