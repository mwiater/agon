# SERVICES.md

This guide explains how to install and set up services used by `llama.cpp`
and Agon Benchmark. It focuses on Windows and Linux builds only.

## Llama.cpp Service

The example service uses `llama-server` with a fixed host/port and a models directory.
Adjust paths, user/group, and server flags to match your environment.

1) Download latest release for your OS from:
   https://github.com/ggml-org/llama.cpp/releases

   - Linux:
     - Download the archive that matches your CPU (most systems are `x86_64`).
     - Prefer the prebuilt release unless you specifically need to compile from source.
   - Windows:
     - Download the Windows release archive (typically a `.zip`).
     - If you later decide to build from source, use the Windows build instructions
       in the repo, but the prebuilt release is easiest for service setup.
   - The same response settings (timings/usage) are available in the prebuilt
     binaries; you do not need a custom build to enable them.

   Optional: build from source (if you need a custom build)

   - Linux (CMake):
     - `git clone https://github.com/ggml-org/llama.cpp.git`
     - `cd llama.cpp`
     - `cmake -B build -S . -DCMAKE_BUILD_TYPE=Release`
     - `cmake --build build --config Release -j`
     - The server binary is typically in `build/bin/llama-server`.
   - Windows (CMake + Visual Studio Build Tools):
     - `git clone https://github.com/ggml-org/llama.cpp.git`
     - `cd llama.cpp`
     - `cmake -B build -S . -DCMAKE_BUILD_TYPE=Release`
     - `cmake --build build --config Release`
     - The server binary is typically in `build\bin\Release\llama-server.exe`.
     - If you do not see it, search under `build\` for `llama-server.exe`.

2) Unpack and install in your home directory

   - Linux:
     - Create a directory in your home, for example:
       `~/llama.cpp`
     - Unpack the archive there so you end up with the binaries inside that folder.
       Example:
       - `~/llama.cpp/llama-server`
       - `~/llama.cpp/llama-cli`
   - Windows:
     - Create a folder under your user profile, for example:
       `C:\Users\<you>\llama.cpp`
     - Unzip the release there. The `llama-server.exe` should be inside this folder.

3) Create a models directory and add model files

   You must create your own models directory and place model files there.
   This directory is referenced by `--models-dir` in the service file.

   - Linux example:
     - Create the directory:
       `mkdir -p ~/models/llama`
     - Copy or download model files into it.
   - Windows example:
     - Create the directory:
       `C:\Users\<you>\models\llama`
     - Copy or download model files into it.

4) Get the absolute path to llama-server binary {llama-server-path}

   - Linux:
     - Example path:
       `/home/<you>/llama.cpp/llama-server`
     - Verify it runs:
       `"/home/<you>/llama.cpp/llama-server" --help`
   - Windows:
     - Example path:
       `C:\Users\<you>\llama.cpp\llama-server.exe`
     - Verify it runs:
       `"C:\Users\<you>\llama.cpp\llama-server.exe" --help`
     - If the path contains spaces, always wrap it in quotes.

5) Create service file (Linux systemd)

   Create a file named `llamacpp.service`, for example:
   `/etc/systemd/system/llamacpp.service`

   Use this template (replace the placeholders):

```
[Unit]
Description=lammacpp Service
After=network-online.target

[Service]
ExecStart={llama-server-path} --host 0.0.0.0 --port 9991 --models-dir {models-dir} --models-max 1
--no-models-autoload --metrics --no-webui
User={user}
Group={group}
Restart=always

[Install]
WantedBy=multi-user.target
```

   Notes:
   - The `ExecStart` line must be a single line in the service file.
     The line breaks above are for readability only. Combine them into one line.
   - To ensure Agon receives timing + usage in `/v1/chat/completions` responses:
     - Do NOT enable response field filtering, or
     - If you do, include both `timings` and `usage` fields in the response.
       (In llama.cpp this is controlled by the response fields feature.)
   - `User` and `Group` should be a non-root account that has read access
     to your model files and execute access to the binary.
   - To find your current user and group:
     - Linux:
       - User: `whoami`
       - Primary group: `id -gn`
       - All groups (optional): `id -Gn`
     - Windows:
       - User: `whoami`
       - User + domain: `whoami /user`
       - Groups: `whoami /groups`
   - Set `{models-dir}` to the absolute path of your models directory.
   - Use an absolute path for `{llama-server-path}`.
   - `--metrics` enables the server metrics endpoint.

6a) Linux: install, enable, start, and check service via systemctl

   1. Reload systemd to pick up the new unit:
      `sudo systemctl daemon-reload`
   2. Enable the service at boot:
      `sudo systemctl enable llamacpp`
   3. Start the service:
      `sudo systemctl start llamacpp`
   4. Check status:
      `sudo systemctl status llamacpp`
   5. View logs:
      `journalctl -u llamacpp -f`

   Common troubleshooting:
   - If the service exits immediately, verify the `ExecStart` path and arguments.
   - If it cannot read models, confirm file permissions on `--models-dir`.
   - If the port is in use, change `--port` and restart the service.

6b) Windows: install, enable, start, and check service

   The easiest path is using NSSM (the Non-Sucking Service Manager).
   It is a small, reliable wrapper for running regular executables as Windows services.

   Steps with NSSM:
   1. Download NSSM from https://nssm.cc/download and unzip it.
   2. Open an elevated PowerShell (Run as Administrator).
   3. Install the service:
      `C:\path\to\nssm.exe install llamacpp`
   4. In the NSSM GUI:
      - Application Path:
        `C:\Users\<you>\llama.cpp\llama-server.exe`
      - Startup directory:
        `C:\Users\<you>\llama.cpp`
      - Arguments:
        `--host 0.0.0.0 --port 9991 --models-dir C:\Users\<you>\models\llama --models-max 1 --no-models-autoload --metrics --no-webui`
   5. Click "Install service".
   6. Start the service:
      `Start-Service llamacpp`
   7. Check status:
      `Get-Service llamacpp`

   Optional: Use the Services console
   - Run `services.msc`, find `llamacpp`, and manage it there.

   Notes for Windows:
   - Always use absolute paths. If a path contains spaces, wrap it in quotes.
   - Ensure the service account has read access to your models directory.
   - If you want the service to run under a specific user, set it in NSSM
     under the "Log on" tab.

## Agon Benchmark Service

This service runs the standalone benchmark server from `servers/benchmark`.
It expects a fixed folder layout relative to the working directory.

1) Run `goreleaser`.

2) Check the `goreleaser` output for where the benchmark server binary is located.
   Look for the `dist/agon-benchmark_linux_amd64_v1/agon-benchmark` or
   `dist/agon-benchmark_windows_amd64_v1/agon-benchmark.exe` artifact.

3) Copy the binary to the `llama.cpp` host as described in the first section:
   `~/agon-benchmark` (Linux) or `C:\Users\<you>\agon-benchmark` (Windows).

4) Create the expected folder layout next to the executable:

   - Linux example:
     - `~/agon-benchmark/agon-benchmark`
     - `~/agon-benchmark/llama.cpp-linux/llama-bench`
     - `~/agon-benchmark/servers/benchmark/agon-benchmark.yml`
   - Windows example:
     - `C:\Users\<you>\agon-benchmark\agon-benchmark.exe`
     - `C:\Users\<you>\agon-benchmark\llama.cpp-windows\llama-bench.exe`
     - `C:\Users\<you>\agon-benchmark\servers\benchmark\agon-benchmark.yml`

   Notes:
   - The benchmark server loads config from `servers/benchmark/agon-benchmark.yml`
     relative to its working directory.
   - The benchmark server runs `llama-bench` from `./llama.cpp-<os>/llama-bench`.
   - If these paths do not exist, the service will fail to start.

5) Copy and configure `agon-benchmark.yml` (details)

   The config file lives at `servers/benchmark/agon-benchmark.yml` and controls
   how the benchmark server listens and how it resolves model paths.

   - `host`: Bind address for the benchmark server.
     - Use `0.0.0.0` to listen on all interfaces.
     - Use `127.0.0.1` if only local access is needed.
   - `port`: TCP port for the benchmark server (default example uses `9999`).
     - Ensure the port is open in your firewall if you access it remotely.
   - `type`: Backend type. Must be `"llama.cpp"` (case-insensitive).
   - `api_base`: Base URL for the llama.cpp server.
     - This field is currently logged but not used by the benchmark server.
     - Keep it set to the llama.cpp server URL for future compatibility.
   - `models_path`: Absolute path to your GGUF models directory.
     - If the benchmark request sends a relative model name, it is joined
       to this directory.
     - If the benchmark request sends an absolute path, it is used as-is.
   - `timeout`: Hard timeout in seconds for a single benchmark run.
     - Increase this for very large models or slower hosts.

   Example (Linux):
   - `host: 0.0.0.0`
   - `port: 9999`
   - `type: "llama.cpp"`
   - `api_base: http://127.0.0.1:9991`
   - `models_path: /home/<you>/models/llama`
   - `timeout: 3600`

6) Linux service setup (systemd)

   Create a unit file such as `/etc/systemd/system/agon-benchmark.service`:

```
[Unit]
Description=Agon Benchmark Server
After=network.target

[Service]
Type=simple
User=<you>
Group=<you>
WorkingDirectory=/home/<you>/projects/agon
ExecStart=/home/<you>/projects/agon/dist/agon-benchmark_linux_amd64_v1/agon-benchmark
Restart=on-failure
RestartSec=5
Environment=GODEBUG=madvdontneed=1

[Install]
WantedBy=multi-user.target
```

   Notes:
   - `WorkingDirectory` must be the folder that contains `servers/benchmark`
     and the `llama.cpp-<os>` directory.
   - Use the same `User` and `Group` guidance as in the Llama.cpp section.
   - If you change the location, update both `WorkingDirectory` and `ExecStart`.

   Start the service:
   - `sudo systemctl daemon-reload`
   - `sudo systemctl enable agon-benchmark`
   - `sudo systemctl start agon-benchmark`
   - `sudo systemctl status agon-benchmark`
   - `journalctl -u agon-benchmark -f`

7) Windows service setup (NSSM)

   Use NSSM to create a Windows service:
   1. Download NSSM from https://nssm.cc/download and unzip it.
   2. Open an elevated PowerShell (Run as Administrator).
   3. Install the service:
      `C:\path\to\nssm.exe install agon-benchmark`
   4. In the NSSM GUI:
      - Application Path:
        `C:\Users\<you>\agon-benchmark\agon-benchmark.exe`
      - Startup directory:
        `C:\Users\<you>\agon-benchmark`
      - Arguments:
        (leave empty)
   5. Click "Install service".
   6. Start the service:
      `Start-Service agon-benchmark`
   7. Check status:
      `Get-Service agon-benchmark`

   Notes for Windows:
   - The Startup directory must contain `servers\benchmark\agon-benchmark.yml`
     and the `llama.cpp-windows\llama-bench.exe` path.
   - Ensure the service account can read your `models_path` directory.

---

goreleaser release --snapshot --clean --skip=publish

dist/agon-benchmark_linux_amd64_v1/agon-benchmark

sudo nano /etc/systemd/system/agon-benchmark.service


sudo systemctl daemon-reload
sudo systemctl stop agon-benchmark
sudo systemctl start agon-benchmark
sudo systemctl status agon-benchmark

sudo journalctl -xeu agon-benchmark.service


sudo nano /etc/systemd/system/agon-benchmark.service

```
[Unit]
Description=Agon Benchmark Server
After=network.target

[Service]
Type=simple
User=matt
Group=matt
WorkingDirectory=/home/matt/projects/agon
ExecStart=/home/matt/projects/agon/dist/agon-benchmark_linux_amd64_v1/agon-benchmark
Restart=on-failure
RestartSec=5
Environment=GODEBUG=madvdontneed=1

[Install]
WantedBy=multi-user.target
```

nano /home/matt/projects/agon/servers/benchmark/agon-benchmark.yml



# benchmarks server config
host: 0.0.0.0
port: 9999
type: "llama.cpp"
api_base: https://o-udoo01.0nezer0.com
models_path: /home/matt/projects/gollama/models
timeout: 3600

# benchmarks server config
host: 0.0.0.0
port: 9999
type: "llama.cpp"
api_base: https://o-udoo02.0nezer0.com
models_path: /home/matt/projects/gollama/models
timeout: 3600

# benchmarks server config
host: 0.0.0.0
port: 9999
type: "llama.cpp"
api_base: https://o-udoo03.0nezer0.com
models_path: /home/matt/projects/gollama/models
timeout: 3600

# benchmarks server config
host: 0.0.0.0
port: 9999
type: "llama.cpp"
api_base: https://o-udoo04.0nezer0.com
models_path: /home/matt/projects/gollama/models
timeout: 3600