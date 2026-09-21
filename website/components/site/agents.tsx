"use client";

import { Session } from "@/components/site/session";
import { Section } from "@/components/site/ui";
import { THEMES } from "@/lib/themes";

const COMMANDS: [string, string][] = [
  ["slat ls", "every pane, and what each one is doing"],
  ["slat pane new --cmd C", "open a pane running C, without taking your focus"],
  ["slat run PANE …", "type a command into a pane and press Enter"],
  ["slat wait PANE --for …", "block until idle, input, exit, or text=REGEX matches"],
  ["slat capture PANE", "read a pane's text, screen or scrollback"],
  ["slat send PANE --key ctrl-c", "answer a prompt, or interrupt"],
];

const STATUSES: [string, string, string][] = [
  ["idle", "text-cyan", "the pane's own shell has the terminal: the command finished"],
  ["working", "text-ink", "a program is running"],
  ["input", "text-ink", "something stopped to ask a human — the tab is marked"],
  ["exited", "text-ink-faint", "the pane's program is gone"],
];

const CODES: [string, string][] = [
  ["0", "it happened"],
  ["1", "the command was wrong"],
  ["2", "timed out"],
  ["3", "that pane is gone"],
];

export function Agents() {
  return (
    <Section
      id="agents"
      marker="slat wait 4 --for idle --timeout 10m"
      title="A terminal an agent can drive"
      lead="Agents are good at running commands and bad at watching them. 1.0 gives slat a command line of its own, so anything outside the terminal can open a pane, start the work, wait for it, and read the result — with exit codes instead of guesswork."
    >
      <div className="[&>*]:min-w-0 grid items-start gap-10 lg:grid-cols-[minmax(0,1fr)_minmax(0,0.85fr)] lg:gap-14">
        <div>
          <ul className="text-[15px]">
            {COMMANDS.map(([command, what]) => (
              <li
                key={command}
                className="hairline-top grid gap-1 py-3.5 sm:grid-cols-[minmax(0,22ch)_minmax(0,1fr)] sm:items-baseline sm:gap-8"
              >
                <code className="font-mono text-[13.5px] text-ink">{command}</code>
                <span className="text-ink-muted">{what}</span>
              </li>
            ))}
          </ul>

          <div className="mt-[var(--block-gap)] grid gap-8 sm:grid-cols-2">
            <div>
              <h3 className="font-mono text-[15px] text-ink">Four things a pane can be</h3>
              <dl className="mt-3 space-y-2.5 text-[14px] leading-[1.55]">
                {STATUSES.map(([name, tone, meaning]) => (
                  <div key={name}>
                    <dt className={`font-mono ${tone}`}>{name}</dt>
                    <dd className="text-ink-muted">{meaning}</dd>
                  </div>
                ))}
              </dl>
            </div>
            <div>
              <h3 className="font-mono text-[15px] text-ink">Exit codes are the contract</h3>
              <dl className="mt-3 space-y-2.5 text-[14px] leading-[1.55]">
                {CODES.map(([code, meaning]) => (
                  <div key={code} className="flex gap-3">
                    <dt className="font-mono text-ink">{code}</dt>
                    <dd className="text-ink-muted">{meaning}</dd>
                  </div>
                ))}
              </dl>
              <p className="mt-4 text-[14px] leading-[1.55] text-ink-faint">
                Add <code className="font-mono text-ink-muted">--json</code> to any command for
                one object with a <code className="font-mono text-ink-muted">schema</code> field.
              </p>
            </div>
          </div>
        </div>

        <div>
          <div className="glass min-w-0 rounded-2xl p-3">
            <Session theme={THEMES[0]} compact alertTab start={["ask"]} />
          </div>
          <div className="mt-6 space-y-4 text-[15px] leading-[1.6] text-ink-muted">
            <p>
              A build that stops on <span className="font-mono text-ink">Overwrite? [y/N]</span>{" "}
              is neither finished nor working. slat calls that{" "}
              <span className="font-mono text-ink">input</span>, marks the tab with a{" "}
              <span className="font-mono text-ink">?</span>, and can run a command to tell you.
              Waiting for <span className="font-mono text-ink">idle</span> wakes on it too, so
              an agent never sleeps through a question.
            </p>
            <p>
              Shell builtins count: <span className="font-mono text-ink">read -p</span> is caught
              even though the shell itself is the foreground process.
            </p>
            <p className="text-ink-faint">
              Nothing leaves the machine. slat has no network code — the commands talk to the
              session over its own socket, readable only by you.
            </p>
          </div>
        </div>
      </div>
    </Section>
  );
}
