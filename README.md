# GoSort

GoSort is a media file sorting and deduplication system. It consists of two components:
- **API Server**: Accepts file uploads, stores them organized by date, and maintains a database of checksums to prevent duplicates
- **Client**: Scans directories and uploads media files to the API server

## Building

Build all targets:
```bash
make all
```

Build specific targets:
```bash
make buildlinux        # Linux x86_64
make buildlinuxarm32   # Linux ARM 32-bit
make buildlinuxarm64   # Linux ARM 64-bit
make buildwin          # Windows x86_64
```

Binaries will be created in the `bin/` directory.

## Configuration

GoSort uses a YAML configuration file located at `~/.gosort.yml` by default. You can create a default configuration file using the `-init` flag on either application.

### Configuration File Format

```yaml
server:
  database_file: '%SAVEDIR%/gosort.db'  # Database file path (%SAVEDIR% is replaced with savedir value)
  savedir: '%HOME%/pictures'            # Directory to save uploaded files (%HOME% is replaced with user's home directory)
  ip: localhost                          # IP address to bind the API server
  port: 8080                             # Port to listen on

client:
  host: localhost:8080                   # API server host and port
```

### Special Variables

- `%HOME%`: Replaced with the user's home directory
- `%SAVEDIR%`: Replaced with the `savedir` value (useful for database_file)

## API Server

The API server accepts file uploads and organizes them by creation date.

### Initial Setup

Create a default configuration file:
```bash
./api -init
```

This creates `~/.gosort.yml` with default settings.

### Usage

**Basic usage (uses ~/.gosort.yml):**
```bash
./api
```

**Override specific settings:**
```bash
./api -ip 0.0.0.0 -port 9090 -savedir /path/to/files
```

**Use a custom config file:**
```bash
./api -config /path/to/custom.yml
```

### Command-Line Flags

| Flag | Description | Overrides Config |
|------|-------------|------------------|
| `-config` | Path to config file (default: ~/.gosort.yml) | - |
| `-database-file` | Database file path | `server.database_file` |
| `-savedir` | Directory to save uploaded files | `server.savedir` |
| `-ip` | IP address to bind to | `server.ip` |
| `-port` | Port to listen on | `server.port` |
| `-init` | Create default config file and exit | - |

### API Endpoints

- `POST /file` - Upload a file (requires multipart form with `file` and `media` fields)
- `GET /file` - Check if a file exists by checksum
- `POST /checksums` - Batch check multiple checksums
- `POST /checksum100k` - Batch check multiple 100k checksums
- `GET /version` - Get API version

### Examples

**Start server on all interfaces, port 9090:**
```bash
./api -ip 0.0.0.0 -port 9090
```

**Start server with custom save directory:**
```bash
./api -savedir /mnt/storage/photos -database-file /mnt/storage/gosort.db
```

**Create config and start server:**
```bash
./api -init
./api
```

## Client

The client scans directories and uploads media files to the API server.

### Initial Setup

Create a default configuration file:
```bash
./client -init
```

This creates `~/.gosort.yml` with default settings.

### Usage

**Basic usage (scans directory and uploads to server):**
```bash
./client /path/to/media/directory
```

**Override server host:**
```bash
./client -host 192.168.1.100:8080 /path/to/media/directory
```

**Use a custom config file:**
```bash
./client -config /path/to/custom.yml /path/to/media/directory
```

### Command-Line Flags

| Flag | Description | Overrides Config |
|------|-------------|------------------|
| `-config` | Path to config file (default: ~/.gosort.yml) | - |
| `-host` | Server host address (format: host:port) | `client.host` |
| `-init` | Create default config file and exit | - |

**Positional Arguments:**
- `<directory>` - Directory to scan and upload files from (required)

### How It Works

1. The client scans the specified directory recursively
2. For each media file found:
   - Calculates full file checksum (MD5)
   - Calculates first 100KB checksum (for quick duplicate detection)
   - Checks with the server if the file already exists
   - If not a duplicate, uploads the file to the server
3. The server organizes files by creation date and stores checksums in the database

### Examples

**Upload files from a directory:**
```bash
./client ~/Pictures
```

**Upload to a remote server:**
```bash
./client -host 192.168.1.100:8080 ~/Pictures
```

**Use custom config and upload:**
```bash
./client -config ~/.gosort-production.yml ~/Pictures
```

**Create config and upload:**
```bash
./client -init
./client ~/Pictures
```

## Configuration Priority

Command-line flags always override values from the configuration file. The priority order is:

1. Command-line flags (highest priority)
2. Configuration file values
3. Default values (if neither is specified)

## File Organization

Uploaded files are organized by creation date in the following structure:
```
savedir/
  YYYY-MM/
    YYYY-MM-DD HH.MM.SS.ext
    YYYY-MM-DD HH.MM.SS.1.ext  (if duplicate timestamp)
```

## Duplicate Detection

The system uses two-level duplicate detection:

1. **100KB checksum**: Quick check of the first 100KB of the file
2. **Full checksum**: Complete file checksum (MD5)

Both checksums must match for a file to be considered a duplicate. This prevents false positives from files that happen to have the same first 100KB but differ later.

## Troubleshooting

**Config file not found:**
```
Error loading config: config file does not exist: ~/.gosort.yml (use -init to create it)
```
Solution: Run `./api -init` or `./client -init` to create the default config file.

**Save directory does not exist:**
```
Save directory does not exist: /path/to/dir
```
Solution: Create the directory or update the `savedir` setting in the config file.

**Port already in use:**
```
Error: bind: address already in use
```
Solution: Change the port using `-port` flag or update the config file.

## License

[Add your license information here]
