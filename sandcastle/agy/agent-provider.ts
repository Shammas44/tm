import type {
  AgentCommandOptions,
  AgentProvider,
  ParsedStreamEvent,
  PrintCommand,
} from "@ai-hero/sandcastle";

export interface AntigravityCliOptions {
  /** The model to run with agy (e.g. "gemini-3.8-flash-medium", "gemini-4-argon-high", "claude-opus-5-5-high") */
  readonly model?: string;

  /** Reasoning effort: "low" | "medium" | "high" | "xhigh" | "max" */
  readonly effort?: "low" | "medium" | "high" | "xhigh" | "max";

  /** When true, runs agy with --dangerously-skip-permissions to auto-approve tool actions */
  readonly skipPermissions?: boolean;

  /** Initial prompt passed to agy with -i. If undefined, agy opens at the home screen without running a prompt */
  readonly initialPrompt?: string;

  /** Environment variables injected into the agent process */
  readonly env?: Record<string, string>;
}

/**
 * Sandcastle Agent Provider for Antigravity CLI (`agy`).
 * Runs the sandboxed agent inside the container with taskmanager and standard bash tools.
 */
export function antigravityCli(options?: AntigravityCliOptions): AgentProvider {
  const model = options?.model ?? process.env.AGY_MODEL ?? "gemini-3.8-flash-medium";
  const skipPerms = options?.skipPermissions ?? true;

  return {
    name: "antigravity",
    env: options?.env ?? {},
    captureSessions: false,
    buildPrintCommand({ prompt }: AgentCommandOptions): PrintCommand {
      const args: string[] = ["--print"];
      if (skipPerms) {
        args.push("--dangerously-skip-permissions");
      }
      if (model) {
        args.push("--model", model);
      }
      if (options?.effort) {
        args.push("--effort", options.effort);
      }
      return {
        command: `agy ${args.join(" ")}`,
        stdin: prompt,
      };
    },
    buildInteractiveArgs({ prompt }: AgentCommandOptions): string[] {
      const args: string[] = ["agy"];
      if (skipPerms) {
        args.push("--dangerously-skip-permissions");
      }
      if (model) {
        args.push("--model", model);
      }
      if (options?.effort) {
        args.push("--effort", options.effort);
      }
      // ONLY pass -i if an initial prompt was explicitly supplied by the caller
      if (options?.initialPrompt) {
        args.push("-i", options?.initialPrompt);
      }
      return args;
    },
    parseStreamLine(line: string): ParsedStreamEvent[] {
      return [{ type: "text", text: line }];
    },
  };
}
