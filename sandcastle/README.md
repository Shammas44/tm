# Sandcastle Sandbox Templates for TaskManager

This directory provides two ready-to-use, **language-agnostic** templates demonstrating real-world Sandcastle Docker container sandboxing with **TaskManager (`tm`)** and isolated **Git worktrees**:

1. [`agy/`](agy/): Interactive sandbox using **Google Antigravity CLI (`agy`)**.
2. [`claude/`](claude/): Interactive sandbox using **Anthropic Claude Code (`claude`)**.

---

## Key Architecture & Features

```
┌────────────────────────────────────────────────────────────────────────┐
│ Host Machine                                                           │
│                                                                        │
│  • Global Sandcastle CLI & Runner (tsx .sandcastle/interactive.ts)     │
│  • TaskManager Static Linux Binary (bin/tm-linux-amd64)                │
│  • Project SQLite Backlog Database (~/.taskmanager/projects/<slug>/)   │
│  • Project-Scoped Conversations (.antigravity/ or .claude/projects/)   │
│  • Git Worktree (.sandcastle/worktrees/<branch>/)                      │
│                                │                                       │
│                    Bind Mounts │                                       │
│                                ▼                                       │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │ Docker Container (sandcastle:agy-dev or sandcastle:claude-dev)   │  │
│  │                                                                  │  │
│  │  • Working Directory: /home/agent/workspace (Isolated Worktree)  │  │
│  │  • Pre-configured Editor: Vim (EDITOR=vim, VISUAL=vim)           │  │
│  │  • CLI Tools: tm (TaskManager), agy or claude                    │  │
│  │  • Passwordless sudo for unprivileged 'agent' user               │  │
│  └──────────────────────────────────────────────────────────────────┘  │
└────────────────────────────────────────────────────────────────────────┘
```

- **Language Agnostic**: Clean base images with standard system packages (`curl`, `git`, `jq`, `sudo`, `build-essential`, `vim`). Add your language SDK (Rust, Go, Python, Node, C/C++) directly at the marked extension point in the `Dockerfile`.
- **First-Class Git Worktree**: Each run invokes Sandcastle's `createWorktree()`, ensuring the agent operates on an independent git branch without modifying your working branch.
- **Project-Scoped Conversations**: The agent only has access to conversation history belonging to the current project repo (`.antigravity/` or `.claude/projects/`). Conversations from other projects on your machine are invisible to the container.
- **Pre-configured Vim**: Vim is installed and set as `EDITOR` / `VISUAL` so interactive task edits (`tm edit`, TUI editor) function immediately.
- **Manual Debug Shell**: Dedicated `./shell.sh` script to jump inside any running container as `agent` with full terminal tools.

---

## Template Structure

Each template folder contains strictly `.sandcastle`-related files and tooling scripts:

```text
sandcastle/<agy|claude>/
├── .sandcastle/
│   ├── Dockerfile         # Language-agnostic container definition
│   ├── interactive.ts     # Worktree + project-scoped interactive session runner
│   ├── package.json       # Script definitions
│   └── tsconfig.json      # TypeScript module resolution (uses global Sandcastle)
├── build-image.sh         # Rebuilds the Docker image with host UID/GID
└── shell.sh               # Attaches a bash shell into the running container
```

---

## Quickstart

### 1. Build the Static TaskManager Binary (if not already built)

From the root of `taskmanager`:

```bash
make build-linux
```

### 2. Choose Your Agent Flavor

#### For Antigravity CLI (`agy`):

```bash
cd sandcastle/agy

# 1. Build the Docker image
./build-image.sh

# 2. Launch the interactive session (starts at home screen, project-scoped)
tsx --tsconfig=.sandcastle/tsconfig.json .sandcastle/interactive.ts

# 3. (In a separate terminal) Open a shell inside the running container
./shell.sh
```

#### For Claude Code (`claude`):

```bash
cd sandcastle/claude

# 1. Build the Docker image
./build-image.sh

# 2. Launch the interactive Claude Code TUI
tsx --tsconfig=.sandcastle/tsconfig.json .sandcastle/interactive.ts

# 3. (In a separate terminal) Open a shell inside the running container
./shell.sh
```

---

## Adapting to Your Project

1. Copy either `sandcastle/agy/` or `sandcastle/claude/` into your target repository.
2. Edit `.sandcastle/Dockerfile` at the marked `[PROJECT EXTENSION POINT]` to add your language runtimes (e.g. `apt-get install -y python3`, `rustup`, etc.).
3. Add the conversation directories to your `.gitignore`:
   ```gitignore
   .sandcastle/worktrees/
   .sandcastle/logs/
   .antigravity/
   .claude/projects/
   ```
4. Run `./build-image.sh` and start pairing!
