# Set up the USC CLI

`usc` gives you access to the user's USC accounts: Brightspace, Web Registration, Handshake, Leavey Library rooms, USC mobile ordering, and the public Schedule of Classes. Every command prints JSON. Follow these steps in order.

Use an environment with command execution and private persistent storage.

## 1. Install

On macOS or Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

It installs to `~/.local/bin/usc`. If that directory isn't on `PATH`, use its full path.

If Go 1.24+ is available, or on Windows, use `go install github.com/nsigel/usc-cli/cmd/usc@latest` instead.

Check it with `usc version`.

## 2. Install the skill

`usc skill` prints a SKILL.md describing every command. Save it to the directory your agent loads skills from, under `usc-cli/SKILL.md`. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

## 3. Sign in

Follow the "Sign in" section of the skill you just saved.

## 4. Finish

Tell the user setup is complete and suggest a few things to ask, for example:

- "when's my next exam?"
- "any Leavey rooms for 4 tomorrow afternoon?"
- "is CSCI 270 still open for spring?"
- "what career fairs are coming up?"

Rules for later: never submit WebReg registration (`usc webreg submit`) or book a room (`usc libcal book`) without the user's explicit go-ahead for that exact change.

Full docs: https://usc.sh/docs/
