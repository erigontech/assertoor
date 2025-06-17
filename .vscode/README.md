# VSCode Debugger Configuration

This document explains how to use the debug configuration for Assertoor in VSCode.

## Requirements

Before using the debugger, make sure you have installed:

- **Go v1.23.0** - Required programming language
- **Docker** - Container platform required by Kurtosis. Install Docker Desktop for your platform from [docker.com](https://www.docker.com/products/docker-desktop/)
- **Kurtosis CLI** - Development environment orchestration tool. Install following the official guide:
  
  **macOS (Homebrew):**

  ```bash
  brew install kurtosis-tech/tap/kurtosis-cli
  ```
  
  **Ubuntu (APT):**

  ```bash
  echo "deb [trusted=yes] https://apt.fury.io/kurtosis-tech/ /" | sudo tee /etc/apt/sources.list.d/kurtosis.list
  sudo apt update
  sudo apt install kurtosis-cli
  ```
  
  **RHEL (YUM):**

  ```bash
  echo '[kurtosis]
  name=Kurtosis
  baseurl=https://yum.fury.io/kurtosis-tech/
  enabled=1
  gpgcheck=0' | sudo tee /etc/yum.repos.d/kurtosis.repo
  sudo yum install kurtosis-cli
  ```
  
  **Other platforms:** Download from [releases page](https://github.com/kurtosis-tech/kurtosis-cli-release-artifacts/releases)

- **yq** - Tool for manipulating YAML files:
  
  **macOS:**

  ```bash
  brew install yq
  ```
  
  **Ubuntu/Debian:**

  ```bash
  sudo apt install yq
  ```
  
  **Other platforms:** Download from [yq releases](https://github.com/mikefarah/yq/releases)

- **PostgreSQL Client (psql)** - Required if you want to use PostgreSQL as database
  
  **macOS:**

  ```bash
  brew install postgresql
  ```
  
  **Ubuntu/Debian:**

  ```bash
  sudo apt install postgresql-client
  ```
  
  **RHEL/CentOS:**
  
  ```bash
  sudo yum install postgresql
  ```

## "Clean debug Go assertoor" Configuration

The `Clean debug Go assertoor` configuration in the `launch.json` file allows you to start Assertoor in debug mode directly from VSCode.

### What happens when you launch the configuration

1. **Pre-Launch Task**: The `devnet-setup` task is automatically executed to prepare the development environment
2. **Program Launch**: The Go binary of Assertoor is launched from the workspace root
3. **Configuration**: The program is started with the configuration file `.hack/devnet/generated-assertoor-config.yaml`
4. **Logging**: Logs are displayed in the VSCode debug console (`showLog: true`)

### Configuration Parameters

- **Program**: `${workspaceFolder}` - Executes the main package from the project root
- **Args**: `["--config", ".hack/devnet/generated-assertoor-config.yaml"]` - Specifies the configuration file to use
- **PreLaunchTask**: `devnet-setup` - Task that runs before starting the debug session
- **Mode**: `auto` - VSCode automatically determines whether to debug the binary or source code

### How to use

1. Open the Assertoor project in VSCode
2. Go to the "Run and Debug" panel (Ctrl+Shift+D / Cmd+Shift+D)
3. Select "Debug Go assertoor" from the dropdown
4. Click the play button or press F5
5. The debugger will start automatically after completing the environment setup

## Custom Enclave test configuration

You can create a custom devnet configuration to customize the testing environment according to your needs.

### How to add a custom configuration

1. Create the file `.hack/devnet/custom-kurtosis.devnet.config.yaml` in your workspace
2. Add your desired configuration to this file
3. The devnet setup will automatically use your custom configuration if it exists

### Example Configuration

Here's an example of a custom configuration:

```yaml
participants:
- el_type: erigon
  el_image: test/erigon:current
  cl_type: prysm
  count: 2
additional_services:
- assertoor
- dora
assertoor_params:
  run_stability_check: false
  run_block_proposal_check: true
```

This configuration:

- Sets up 2 nodes with Erigon (execution layer) and Prysm (consensus layer)
- Enables additional services (Assertoor and Dora)
- Configures Assertoor with specific parameters and test playbooks
- Uses custom Docker images for testing

### Notes

- Make sure all requirements are installed before launching the debugger
- The `devnet-setup` task may take a few minutes the first time
- You can set breakpoints in the Go code before starting the debug session
- Custom configurations allow you to test different network setups and scenarios

## Custom Assertoor Configuration

You can create a personalized Assertoor configuration that overrides or extends the automatically generated settings.

### Creating your custom Assertoor configuration

To create a custom configuration:

1. **Create the file `custom-assertoor.devnet.config.yaml`** in the `.hack/devnet/` directory
2. **Add your custom settings** that will override or extend the generated config
3. **Use environment variables** like `${DEVNET_DIR}` which are automatically substituted

### Example custom configuration

```yaml
# Custom Assertoor configuration
# This file overrides/adds settings to the automatically generated config

web:
  server:
    host: "0.0.0.0"
    port: 8080
  frontend:
    enabled: true
    debug: true

# Custom external tests
externalTests:
  - file: https://raw.githubusercontent.com/noku-team/assertoor/master/playbooks/dev/tx-pool-check-short.yaml
  
# Custom logging configuration
logging:
  level: "debug"
  colorOutput: true
  
# Custom database settings (optional, but needed for macos, by default it uses /app folder, that is not writable)
database:
  engine: "sqlite"
  sqlite:
    file: "${DEVNET_DIR}/sqlite.db"

# database:
#   engine: "pgsql"
#   pgsql:
#     host: "localhost"
#     port: "5432"
#     username: "postgres"
#     password: ""
#     database: "assertoor"
#     max_open_conns: 50
#     max_idle_conns: 10
```

### Common customizations

- **Change web port**: Modify `web.server.port`
- **Add custom tests**: Use `externalTests` to include your test playbooks
- **Custom logging**: Configure log level and format
- **Different database**: Switch from PostgreSQL to SQLite or customize connection settings

### Important notes

- The custom config file is **always preserved** between runs
- The generated config is recreated each time you run the devnet setup
- Environment variables in your config (like `${DEVNET_DIR}`) are automatically substituted
- Your custom settings will override any conflicting settings from the generated config
