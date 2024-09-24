# ProxyTV

[![CI](https://github.com/csfrancis/proxytv/actions/workflows/ci.yml/badge.svg)](https://github.com/csfrancis/proxytv/actions/workflows/ci.yml)

ProxyTV is a lightweight proxy server for IPTV and EPG streams. It allows you to remux and serve IPTV channels and EPG data with ease.

ProxyTV can use FFmpeg for remuxing streams. This is useful for bypassing some ISPs that may block certain IPTV streams. Simply run ProxyTV on a VPS and use the `serverAddress` option to point your client to the VPS address. Alternatively, you can run a VPN like Tailscale on both the client and VPS to simplify the process.

ProxyTV will filter channels based on the provided filters. The list of filtered channels will be included in the M3U and EPG XML files that are served. The ordering of the channels will be the same as the order of the filters. If no filters are provided, all channels will be included.

## Configuration

To configure ProxyTV, you need to create a YAML configuration file. Below is an example configuration file format:

```yaml
logLevel: "info" # Log level (optional, default: "info")
iptvUrl: "http://example.com/get.php?username=XXX&password=XXX&output=ts&type=m3u_plus" # URL to the IPTV M3U file (required)
epgUrl: "http://example.com/xmltv.php?username=XXX&password=XXX" # URL to the EPG XML file (required)
listenAddress: ":6078" # Address to listen on (optional, default: ":6078")
serverAddress: "localhost:6078" # Base server address (required)
refreshInterval: "12h" # Refresh interval (optional, default: "12h")
ffmpeg: true # Use FFMPEG for remuxing (optional, default: true)
maxStreams: 1 # Maximum number of concurrent streams (optional, default: 1)
filters: # List of filters (optional)
  - filter: "USA \| NFL" # Regular expression filter
    type: "group" # Filter type (name/group/id)
  - filter: "HBO.*UHD$"
    type: "name"
```

### Configuration Fields

- `logLevel`: The logging level. Default is "info". Valid values are `debug`, `info`, `warn`, `error`, and `fatal`.
- `iptvUrl`: The URL or file path to the IPTV M3U file. This field is required.
- `epgUrl`: The URL or file path to the EPG XML file.
- `listenAddress`: The address the server will listen on. Default is ":6078".
- `serverAddress`: The address used by the client to access the server. This field is required.
- `refreshInterval`: The interval at which the provider M3U and EPG files should be refreshed. Default is "12h".
- `ffmpeg`: Whether to use FFMPEG for remuxing streams. Default is `true`.
- `maxStreams`: The maximum number of concurrent streams. Default is `1`.
- `filters`: A list of filters to include channels based on regular expressions.

## Usage

Edit the `config.yaml` file to configure the server. Then run the server:

```sh
./proxytv -config /path/to/your/config.yaml
```

The server will run in the foreground and print logs to the console.

Configure your IPTV client to point to the server address in the config file. For example, if the `serverAddress` is `proxy:6078`, then your IPTV client should point to `http://proxy:6078/iptv.m3u`. The URL for the EPG file will be `http://proxy:6078/epg.xml`.


## HTTP Endpoints

ProxyTV provides several HTTP endpoints for interacting with the server:

- `GET /ping`: Returns "PONG" to check if the server is running.
- `GET /iptv.m3u`: Downloads the IPTV M3U file.
- `GET /epg.xml`: Downloads the EPG XML file.
- `GET /channel/:channelId`: Streams the specified channel by its ID.
- `PUT /refresh`: Refreshes the provider data.

## Building the Project

To build the ProxyTV project, you need to have Go (1.22 or later) installed on your machine. Follow the steps below to build the project:

1. Clone the repository:
    ```sh
    git clone https://github.com/csfrancis/proxytv.git
    cd proxytv
    ```

2. Install dependencies:
    ```sh
    make setup
    ```

3. Build the project:
    ```sh
    make build
    ```

4. Run the server:
    ```sh
    ./dist/proxytv -config /path/to/your/config.yaml
    ```

Make sure to replace `/path/to/your/config.yaml` with the actual path to your configuration file. By default, proxytv will look for a config file in the current working directory.

## Logging

ProxyTV uses `logrus` for logging. The log level can be configured in the YAML configuration file using the `logLevel` field.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.