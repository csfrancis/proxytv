# ProxyTV

ProxyTV is a lightweight proxy server for IPTV and EPG streams. It allows you to remux and serve IPTV channels and EPG data with ease.

ProxyTV will filter channels based on the provided filters. The list of filtered channels will be included in the M3U and EPG XML files that are served. The ordering of the channels will be the same as the order of the filters.If no filters are provided, all channels will be included.

ProxyTV can use FFMPEG for remuxing streams. This is useful for bypassing some ISPs that may block certain IPTV streams. Simply run ProxyTV on a VPS and use the `baseAddress` option to point your client to the VPS address. Alternatively, you can run a VPN like Tailscale on both the client and VPS to simplify the process.

## Configuration

To configure ProxyTV, you need to create a YAML configuration file. Below is an example configuration file format:

```yaml
logLevel: "info" # Log level (optional, default: "info")
iptvUrl: "http://example.com/iptv.m3u" # URL to the IPTV M3U file (required)
epgUrl: "http://example.com/epg.xml" # URL to the EPG XML file (required)
listenAddress: "localhost:6078" # Address to listen on (optional, default: "localhost:6078")
baseAddress: "http://localhost:6078" # Base address for the server (required)
ffmpeg: false # Use FFMPEG for remuxing (optional, default: false)
maxStreams: 1 # Maximum number of concurrent streams (optional, default: 1)
filters: # List of filters (optional)
  - filter: "USA \| NFL" # Regular expression filter
    type: "group" # Filter type (name/group/id)
```

### Configuration Fields

- `logLevel`: The logging level. Default is "info".
- `iptvUrl`: The URL to the IPTV M3U file. This field is required.
- `epgUrl`: The URL to the EPG XML file. This field is required.
- `listenAddress`: The address the server will listen on. Default is "localhost:6078".
- `baseAddress`: The base address for the server. This field is required.
- `ffmpeg`: Whether to use FFMPEG for remuxing streams. Default is `false`.
- `maxStreams`: The maximum number of concurrent streams. Default is `1`.
- `filters`: A list of filters to include or exclude channels based on regular expressions.

## HTTP Endpoints

ProxyTV provides several HTTP endpoints for interacting with the server:

- `GET /ping`: Returns "PONG" to check if the server is running.
- `GET /get.php`: Downloads the IPTV M3U file.
- `GET /xmltv.php`: Downloads the EPG XML file.
- `GET /channel/:channelId`: Streams the specified channel by its ID.
- `PUT /refresh`: Refreshes the provider data.

## Building the Project

To build the ProxyTV project, you need to have Go installed on your machine. Follow the steps below to build the project:

1. Clone the repository:
    ```sh
    git clone https://github.com/yourusername/proxytv.git
    cd proxytv
    ```

2. Build the project:
    ```sh
    go build -o proxytv
    ```

3. Run the server:
    ```sh
    ./proxytv -config /path/to/your/config.yaml
    ```

Make sure to replace `/path/to/your/config.yaml` with the actual path to your configuration file.

## Logging

ProxyTV uses `logrus` for logging. The log level can be configured in the YAML configuration file using the `logLevel` field.

## HTTP Routing

ProxyTV uses `gin` for HTTP routing. The routes are defined in the `server.go` file.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.