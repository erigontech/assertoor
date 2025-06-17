# VSCode Debugger Configuration

This document explains how to use the debug configuration for Assertoor in VSCode.

## Requirements

Before using the debugger, make sure you have installed:

- **Go v1.23.0** - Required programming language
- **yq** - Tool for manipulating YAML files, installable via Homebrew:

  ```bash
  brew install yq
  ```

## "Debug Go assertoor" Configuration

The `Debug Go assertoor` configuration in the `launch.json` file allows you to start Assertoor in debug mode directly from VSCode.

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

### Notes

- Make sure all requirements are installed before launching the debugger
- The `devnet-setup` task may take a few minutes the first time
- You can set breakpoints in the Go code before starting the debug session
