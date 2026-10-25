import { createWorktree } from "@ai-hero/sandcastle";
import { docker } from "@ai-hero/sandcastle/sandboxes/docker";
import { noSandbox } from "@ai-hero/sandcastle/sandboxes/no-sandbox";
import {
  chmodSync,
  copyFileSync,
  existsSync,
  mkdirSync,
  writeFileSync,
} from "node:fs";
import { homedir } from "node:os";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { antigravityCli } from "./agent-provider.js";

// ==============================================================================
// AGENT & GIT WORKTREE CONFIGURATION (Configure on the fly)
// ==============================================================================
export const CONFIG = {
  // Docker image name built via ./build-image.sh
  imageName: process.env.IMAGE_NAME ?? "sandcastle:agy-dev",

  // Branch for the agent's dedicated git worktree
  branch: process.env.AGENT_BRANCH ?? "agent/agy-worktree",

  // Preserve the git worktree on the host after the session finishes
  preserveWorktree: true,

  // Scope agy conversations exclusively to this project repo (.antigravity/)
  // When true: agent only sees conversations for this project (isolated from other host projects)
  // When false: mounts host ~/.gemini/antigravity-cli with global conversation history
  scopeConversationsToProject: true,

  // Project-local directory to store project conversations & agy state
  projectAgyDir: ".antigravity",

  // Model to run with agy (run `agy models` on host to see all options)
  // e.g.: "gemini-3.8-flash-medium", "gemini-4-argon-high", "claude-opus-5-5-high", "claude-sonnet-5-5-high"
  model: process.env.AGY_MODEL ?? "gemini-3.8-flash-medium",

  // Reasoning effort: "low" | "medium" | "high" | "xhigh" | "max"
  effort: "high" as const,

  // Automatically approve all tool actions (run commands, write files) inside the container
  skipPermissions: true,

  // CPU cores allocated to the sandbox container
  cpus: 4,
};

const currentDir = fileURLToPath(new URL(".", import.meta.url));

async function main() {
  const isInsideContainer = existsSync("/.dockerenv") || process.env.SANDBOX_INSIDE_CONTAINER === "true";
  const projectSlug = process.env.TASKMANAGER_PROJECT ?? basename(process.cwd());

  // Only pass an initial prompt if the user explicitly provided one via CLI args.
  // When no arg is passed, agy starts at the home screen so you can resume past conversations.
  const cliPrompt = process.argv.slice(2).join(" ").trim() || undefined;
  const sandcastlePrompt = cliPrompt ?? "Interactive developer session";

  let sandboxProvider;

  if (isInsideContainer) {
    sandboxProvider = noSandbox();
  } else {
    const projectRootDir = resolve(currentDir, "..");
    const hostProjectDir = join(homedir(), ".taskmanager", "projects", projectSlug);
    const hostGlobalGeminiDir = join(homedir(), ".gemini", "antigravity-cli");
    const hostGlobalOAuthToken = join(hostGlobalGeminiDir, "antigravity-oauth-token");

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
      // 1. TaskManager static Linux binary mounted into agent's PATH
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
      // 3. Project-scoped Antigravity storage inside this repository (.antigravity/)
      const localAgyDir = join(projectRootDir, CONFIG.projectAgyDir);
      mkdirSync(join(localAgyDir, "cache"), { recursive: true });
      mkdirSync(join(localAgyDir, "brain"), { recursive: true });

      // Synchronize OAuth token from host for authentication
      if (existsSync(hostGlobalOAuthToken)) {
        copyFileSync(hostGlobalOAuthToken, join(localAgyDir, "antigravity-oauth-token"));
        try {
          chmodSync(join(localAgyDir, "antigravity-oauth-token"), 0o600);
        } catch {}
      }

      // Pre-seed onboarding completion so questions are never prompted
      const onboardingPath = join(localAgyDir, "cache", "onboarding.json");
      if (!existsSync(onboardingPath)) {
        writeFileSync(
          onboardingPath,
          JSON.stringify(
            {
              consumerOnboardingComplete: true,
              enterpriseOnboardingComplete: false,
              onboardingComplete: true,
            },
            null,
            2,
          ) + "\n",
        );
      }

      // Pre-seed project settings with selected model & full permissions
      const settingsPath = join(localAgyDir, "settings.json");
      if (!existsSync(settingsPath)) {
        writeFileSync(
          settingsPath,
          JSON.stringify(
            {
              allowNonWorkspaceAccess: true,
              enableTerminalSandbox: false,
              model: CONFIG.model,
            },
            null,
            2,
          ) + "\n",
        );
      }

      mounts.push({
        hostPath: localAgyDir,
        sandboxPath: "/home/agent/.gemini/antigravity-cli",
      });
    } else {
      // Mount full global Antigravity state directory from host
      if (existsSync(hostGlobalGeminiDir)) {
        mounts.push({
          hostPath: hostGlobalGeminiDir,
          sandboxPath: "/home/agent/.gemini/antigravity-cli",
        });
      }
    }

    sandboxProvider = docker({
      imageName: CONFIG.imageName,
      cpus: CONFIG.cpus,
      env: {
        TASKMANAGER_PROJECT: projectSlug,
        EDITOR: "vim",
        VISUAL: "vim",
      },
      mounts,
    });
  }

  // 1. Create a dedicated git worktree for the agent
  const wt = await createWorktree({
    branchStrategy: { type: "branch", branch: CONFIG.branch },
  });

  console.log(`==================================================================`);
  console.log(` Sandcastle Interactive Agent Session (Antigravity CLI)`);
  console.log(`==================================================================`);
  console.log(` Project:              ${projectSlug}`);
  console.log(` Git Worktree Branch:  ${wt.branch}`);
  console.log(` Git Worktree Path:    ${wt.worktreePath}`);
  console.log(` Container Workspace:  /home/agent/workspace`);
  console.log(` AI Model:             ${CONFIG.model}`);
  console.log(` Reasoning Effort:     ${CONFIG.effort}`);
  console.log(` Skip Permissions:     ${CONFIG.skipPermissions}`);
  console.log(` Conversation Storage: ${CONFIG.scopeConversationsToProject ? `Project-isolated (${CONFIG.projectAgyDir}/)` : "Host global (~/.gemini/antigravity-cli)"}`);
  if (cliPrompt) {
    console.log(` Initial Prompt:       "${cliPrompt}"`);
  } else {
    console.log(` Mode:                 Direct home screen (select/resume past sessions)`);
  }
  console.log(`==================================================================\n`);

  // 2. Run the interactive session inside the sandbox backed by the worktree
  const result = await wt.interactive({
    agent: antigravityCli({
      model: CONFIG.model,
      effort: CONFIG.effort,
      skipPermissions: CONFIG.skipPermissions,
      initialPrompt: cliPrompt,
    }),
    sandbox: sandboxProvider,
    prompt: sandcastlePrompt,
    name: `${projectSlug}-agy-interactive`,
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
