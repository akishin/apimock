# apimock

A simple mock server that simulates APIs just by placing JSON files in the `mock` directory.

## Features

*   **File-based Routing**: The directory structure maps directly to URLs.
*   **Detailed Response Control**: Define HTTP methods, status codes, headers, and response delays (sleep) within the JSON files.
*   **Flexible Configuration**: Configure the port and directory via command-line arguments or a configuration file (`.apimockrc`).

## Installation

If you have Go installed, you can install it with the following command:

```sh
go install github.com/akishin/apimock@latest
```

Alternatively, clone the repository and build it:

```sh
git clone https://github.com/akishin/apimock.git
cd apimock
go build
```

## Usage

### Starting the Server

By default, the server starts on port `8080` using the `mock` folder in the current directory as the root.

To run the built binary:

```sh
./apimock
```

To run directly from source:

```sh
go run main.go
```

If you don't have Go installed, you can use Docker:

```sh
docker build -t apimock:latest .
docker run -d \
  -v $(pwd)/mock:/mock \
  -p 8080:8080 \
  --name apimock \
  apimock:latest
```

Or using Docker Compose:

```sh
docker compose up -d
```

### Options

*   `--port`: Specifies the port number (default: `8080`).
*   `--dir`: Specifies the directory containing mock data (default: `mock`).

Example: Running with a `data` directory on port `3000`:

```sh
./apimock --dir data --port 3000
```

### Configuration File (.apimockrc)

You can override default settings by placing an `.apimockrc` file in your home directory or the current directory.

```json
{
  "dir": "mocks",
  "port": "9000"
}
```

## Creating Mock Data

### Directory Structure and URLs

JSON files corresponding to the requested URL path are loaded.

*   `GET /users` → `mock/users.json` or `mock/users/index.json`
*   `POST /users/created` → `mock/users/created.json` or `mock/users/created/index.json`

### JSON File Format

To control the response content, create a JSON file with the following fields:

| Field | Type | Description |
| :--- | :--- | :--- |
| `method` | `[]string` | Allowed HTTP methods (e.g., `["GET"]`, `["POST"]`). If unspecified, all methods are allowed, but specifying is recommended. |
| `status` | `int` | HTTP status code (default: `200`). |
| `delay` | `int` | Response delay in milliseconds. |
| `headers` | `map[string]string` | Response headers. |
| `body` | `any` | JSON data to be returned as the response body. |

#### Example 1: Get User List (GET /users)

`mock/users/index.json`:

```json
{
  "method": ["GET"],
  "status": 200,
  "body": [
    {"id": 1, "name": "Taro"},
    {"id": 2, "name": "Hanako"}
  ]
}
```

#### Example 2: Create User (POST /users/created)

`mock/users/created.json`:

```json
{
  "method": ["POST"],
  "status": 201,
  "delay": 1000,
  "headers": {
    "Location": "/users/999"
  },
  "body": {
    "id": 999,
    "message": "Created!"
  }
}
```

### Simple Mode

If you place a pure JSON file without the control fields above, its content will be returned directly as the response body (with a 200 status code).

```json
[
  {"id": 1, "name": "Simple Taro"}
]
```
