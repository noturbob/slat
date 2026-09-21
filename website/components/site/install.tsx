"use client";

import { useState } from "react";

import { Choice, CommandLine, Section } from "@/components/site/ui";

type Platform = {
  name: string;
  commands: { command: string; comment?: string }[];
  note?: string;
};

const PLATFORMS: Platform[] = [
  {
    name: "Debian / Ubuntu",
    commands: [
      {
        command:
          "curl -fsSL https://noturbob.github.io/slat/apt/key.gpg | sudo tee /usr/share/keyrings/slat.gpg > /dev/null",
        comment: "trust the signing key",
      },
      {
        command:
          'echo "deb [signed-by=/usr/share/keyrings/slat.gpg] https://noturbob.github.io/slat/apt stable main" | sudo tee /etc/apt/sources.list.d/slat.list',
      },
      { command: "sudo apt update && sudo apt install slat" },
    ],
    note: "slat is also on its way into Debian itself, sponsored through the usual review.",
  },
  {
    name: "Arch",
    commands: [
      {
        command:
          "curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.pkg.tar.zst",
      },
      { command: "sudo pacman -U slat_linux_amd64.pkg.tar.zst" },
    ],
  },
  {
    name: "Fedora / RHEL",
    commands: [
      {
        command:
          "curl -LO https://github.com/noturbob/slat/releases/latest/download/slat_linux_amd64.rpm",
      },
      { command: "sudo rpm -i slat_linux_amd64.rpm" },
    ],
  },
  {
    name: "macOS",
    commands: [
      { command: "go install github.com/noturbob/slat/cmd/slat@latest" },
    ],
    note: "Or take the darwin tarball from the releases page and put the binary on your PATH.",
  },
  {
    name: "Windows",
    commands: [{ command: "slat.exe", comment: "unzip slat_windows_amd64.zip, then run it" }],
    note: "Windows 10 1809 or later, which is when ConPTY arrived. Windows Terminal is recommended — the old console can't draw everything slat paints.",
  },
  {
    name: "From source",
    commands: [
      { command: "git clone https://github.com/noturbob/slat && cd slat" },
      { command: "go build -o ~/.local/bin/slat ./cmd/slat" },
    ],
    note: "Go 1.23 or later. No other dependencies, and nothing to run afterwards.",
  },
];

export function Install() {
  const [name, setName] = useState(PLATFORMS[0].name);
  const platform = PLATFORMS.find((p) => p.name === name) ?? PLATFORMS[0];

  return (
    <Section
      id="install"
      marker="sudo apt install slat"
      title="Install it"
      lead="One static binary, and a first run that needs no configuration. Type slat to start a session; type it again from anywhere to get that session back."
    >
      <div className="mt-8 max-w-[860px]">
        <Choice
          label="Platform"
          options={PLATFORMS.map((p) => p.name)}
          value={name}
          onChange={setName}
        />

        <div className="glass mt-5 rounded-2xl px-5 py-3">
          {platform.commands.map((entry) => (
            <CommandLine key={entry.command} command={entry.command} comment={entry.comment} />
          ))}
        </div>

        {platform.note ? (
          <p className="mt-4 max-w-[68ch] text-[14px] leading-[1.6] text-ink-faint">
            {platform.note}
          </p>
        ) : null}

        <p className="mt-8 max-w-[68ch] text-[14px] leading-[1.6] text-ink-faint">
          slat turns one this October, and 1.0 is tagged for the occasion. Until then the
          packages above install the current release, and building from{" "}
          <code className="font-mono text-ink-muted">main</code> gets you everything on this
          page.
        </p>
      </div>
    </Section>
  );
}
