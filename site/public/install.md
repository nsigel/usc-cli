# Set up the USC CLI

`usc` gives you access to the user's USC accounts: Brightspace, Web Registration, Handshake, Leavey Library rooms, and the public Schedule of Classes. Every command prints JSON. Follow these steps in order.

## 1. Install

On macOS or Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

It installs to `~/.local/bin/usc`. If that directory isn't on `PATH`, call it by full path or tell the user how to add it.

If Go 1.24+ is available, or on Windows, use `go install github.com/nsigel/usc-cli/cmd/usc@latest` instead.

Check it with `usc version`.

## 2. Install the skill

`usc skill` prints a SKILL.md describing every command. Save it to the directory your agent loads skills from, under `usc-cli/SKILL.md`. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

## 3. Have the user sign in

Run `usc auth status`. If it prints `"authenticated":true`, you're done.

Otherwise the user must sign in themselves. Don't ask for their password or bypass code in the chat. Ask them to run this in their own terminal:

```sh
usc auth login
```

It prompts for their USC NetID, password, and a Duo bypass code, which they can get at https://account.usc.edu/2fa/duo-bypass-code. Credentials are saved locally and every later command signs itself in.

When they're done, run `usc auth status` again to confirm.

## 4. Finish

Tell the user setup is complete and suggest a few things to ask, for example:

- "when's my next exam?"
- "any Leavey rooms for 4 tomorrow afternoon?"
- "is CSCI 270 still open for spring?"
- "what career fairs are coming up?"

Rules for later: never submit WebReg registration (`usc webreg submit`) or book a room (`usc libcal book`) without the user's explicit go-ahead for that exact change.

Full docs: https://usc.sh/docs/
