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

## 3. Sign in

Run `usc auth status`. If it prints `"authenticated":true`, skip to step 4.

Otherwise ask the user which they prefer:

- **You sign them in.** Ask for their USC NetID, password, and a Duo bypass code (they can get one at https://account.usc.edu/2fa/duo-bypass-code). Then run:

  ```sh
  USC_USERNAME='NETID' USC_PASSWORD='PASSWORD' USC_DUO_BYPASS='CODE' usc auth login
  ```

  Don't repeat the credentials back or save them anywhere else; `usc` stores them in its own private file.

- **They sign in themselves.** Ask them to run `usc auth login` in their own terminal. It prompts for the same three things.

Either way, every later command signs itself in. Run `usc auth status` to confirm.

## 4. Finish

Tell the user setup is complete and suggest a few things to ask, for example:

- "when's my next exam?"
- "any Leavey rooms for 4 tomorrow afternoon?"
- "is CSCI 270 still open for spring?"
- "what career fairs are coming up?"

Rules for later: never submit WebReg registration (`usc webreg submit`) or book a room (`usc libcal book`) without the user's explicit go-ahead for that exact change.

Full docs: https://usc.sh/docs/
