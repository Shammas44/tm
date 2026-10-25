import { claudeCode, createWorktree } from "@ai-hero/sandcastle";
import { docker } from "@ai-hero/sandcastle/sandboxes/docker";
import { noSandbox } from "@ai-hero/sandcastle/sandboxes/no-sandbox";
import { existsSync, mkdirSync } from "node:fs";
import { homedir } from "node:os";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";

// ==============================================================================
// AGENT & GIT WORKTREE CONFIGURATION (Configure on the fly)
// ==============================================================================
export const CONFIG = {
  // Docker image name built via ./build-image.sh
  imageName: process.env.IMAGE_NAME ?? "sandcastle:claude-dev",

  // Branch for the agent's dedicated git worktree
  branch: process.env.AGENT_BRANCH ?? "agent/claude-worktree",

  // Preserve the git worktree on the host after the session finishes
  preserveWorktree: true,

  // Scope Claude Code conversations exclusively to this project repo (.claude/projects/)
  scopeConversationsToProject: true,

  // Project-local directory to store Claude sessions
  projectClaudeDir: ".claude/projects",

  // Claude model: "claude-sonnet-4-6", "claude-opus-4-8", "claude-3-7-sonnet"
  model: process.env.CLAUDE_MODEL ?? "claude-sonnet-4-6",

  // Reasoning effort: "low" | "medium" | "high" | "xhigh" | "max"
  effort: "high" as const,

  // CPU cores allocated to the sandbox container
  cpus: 4,
};

const currentDir = fileURLToPath(new URL(".", import.meta.url));

async function main() {
  const isInsideContainer = existsSync("/.dockerenv") || process.env.SANDBOX_INSIDE_CONTAINER === "true";
  const projectSlug = process.env.TASKMANAGER_PROJECT ?? basename(process.cwd());

  const cliPrompt = process.argv.slice(2).join(" ").trim() || undefined;
  const sandcastlePrompt = cliPrompt ?? "Interactive developer session";

  let sandboxProvider;

  if (isInsideContainer) {
    sandboxProvider = noSandbox();
  } else {
    const projectRootDir = resolve(currentDir, "..");
    const hostProjectDir = join(homedir(), ".taskmanager", "projects", projectSlug);
    const hostClaudeConfig = join(homedir(), ".claude.json");

    // Locate static TaskManager Linux binary
    const candidatePaths = [
      process.env.TM_BINARY_PATH,
      resolve(currentDir, "../../../bin/tm-linux-amd64"),
      resolve(currentDir, "../../bin/tm-linux-amd64"),
      join(homedir(), ".local/bin/tm-linux-amd64"),
    ].filter(Boolean) as string[];

    const tmBinaryPath = candidatePaths.find((p) => existsSync(p)) || candidatePaths[1];

    mkdirSync(hostProjectDir, { recursive: true });

    const mounts = [
      // 1. TaskManager static Linux binary in agent's PATH
      {
        hostPath: tmBinaryPath,
        sandboxPath: "/home/agent/.local/bin/tm",
        readonly: true,
      },
      // 2. TaskManager SQLite project database directory
      {
        hostPath: hostProjectDir,
        sandboxPath: `/home/agent/.taskmanager/projects/${projectSlug}`,
      },
    ];

    if (CONFIG.scopeConversationsToProject) {
      // 3. Project-scoped Claude conversation history inside repository
      const localClaudeDir = join(projectRootDir, CONFIG.projectClaudeDir);
      mkdirSync(localClaudeDir, { recursive: true });

      mounts.push({
        hostPath: localClaudeDir,
        sandboxPath: "/home/agent/.claude/projects",
      });
    }

    // 4. Mount host Claude authentication credentials if present
    if (existsSync(hostClaudeConfig)) {
      mounts.push({
        hostPath: hostClaudeConfig,
        sandboxPath: "/home/agent/.claude.json",
        readonly: true,
      });
    }

    const env: Record<string, string> = {
      TASKMANAGER_PROJECT: projectSlug,
      EDITOR: "vim",
      VISUAL: "vim",
    };

    if (process.env.ANTHROPIC_API_KEY) {
      env.ANTHROPIC_API_KEY = process.env.ANTHROPIC_API_KEY;
    }

    sandboxProvider = docker({
      imageName: CONFIG.imageName,
      cpus: CONFIG.cpus,
      env,
      mounts,
    });
  }

  // 1. Create a dedicated git worktree for the agent
  const wt = await createWorktree({
    branchStrategy: { type: "branch", branch: CONFIG.branch },
  });

  console.log(`==================================================================`);
  console.log(` Sandcastle Interactive Agent Session (Claude Code)`);
  console.log(`==================================================================`);
  console.log(` Project:              ${projectSlug}`);
  console.log(` Git Worktree Branch:  ${wt.branch}`);
  console.log(` Git Worktree Path:    ${wt.worktreePath}`);
  console.log(` Container Workspace:  /home/agent/workspace`);
  console.log(` AI Model:             ${CONFIG.model}`);
  console.log(` Reasoning Effort:     ${CONFIG.effort}`);
  console.log(` Conversation Storage: ${CONFIG.scopeConversationsToProject ? `Project-isolated (${CONFIG.projectClaudeDir}/)` : "Host global (~/.claude/projects)"}`);
  if (cliPrompt) {
    console.log(` Initial Prompt:       "${cliPrompt}"`);
  } else {
    console.log(` Mode:                 Direct Claude TUI (type /resume to pick a past session)`);
  }
  console.log(`==================================================================\n`);

  // 2. Run the interactive Claude Code session inside the sandbox backed by the worktree
  const result = await wt.interactive({
    agent: claudeCode(CONFIG.model, {
      effort: CONFIG.effort,
    }),
    sandbox: sandboxProvider,
    prompt: sandcastlePrompt,
    name: `${projectSlug}-claude-interactive`,
    hooks: {
      sandbox: {
        onSandboxReady: [
          {
            command: "chmod +x /home/agent/.local/bin/tm 2>/dev/null || true",
          },
        ],
      },
    },
  });

  console.log(`\nInteractive session exited with code ${result.exitCode}`);
  console.log("Commits created during session:", result.commits);

  if (CONFIG.preserveWorktree) {
    console.log(`==================================================================`);
    console.log(` Worktree preserved on host:`);
    console.log(`   Path:   ${wt.worktreePath}`);
    console.log(`   Branch: ${wt.branch}`);
    console.log(`==================================================================`);
  } else {
    const closeResult = await wt.close();
    if (closeResult.preservedWorktreePath) {
      console.log(`Worktree preserved (uncommitted changes): ${closeResult.preservedWorktreePath}`);
    }
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
