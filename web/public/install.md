# Set up the USC CLI

`usc` gives you access to the user's USC accounts: Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library rooms, USC mobile ordering, and the public Schedule of Classes. Each command prints JSON. Do these steps in sequence.

You must be able to run commands. Private persistent storage keeps the user signed in between sessions. Without it, the user must sign in again in each session.

## 1. Install

On macOS or Linux, run:

```sh
curl -fsSL https://usc.sh/install | sh
```

The script installs `usc` in `~/.local/bin/usc`. If that directory is not on `PATH`, use the full path.

On Windows, or if you have Go 1.24 or later, run `go install github.com/nsigel/usc-cli/cmd/usc@latest`.

Run `usc version` to make sure that the install is correct.

## 2. Install the skill

`usc skill` prints a SKILL.md file. The file gives the commands and the rules for their use. Save it in your skills directory as `usc-cli/SKILL.md`. For Claude Code, run:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

If your agent does not have a skills directory, run `usc skill` at the start of each USC task. Then obey its instructions.

## 3. Sign in

Do the steps in the "Sign in" section of the skill.

## 4. Finish

Tell the user that the setup is complete. Give the user some example requests:

- "when's my next exam?"
- "any Leavey rooms for 4 tomorrow afternoon?"
- "is CSCI 270 still open for spring?"
- "what career fairs are coming up?"

For all later tasks, obey the "Rules" section of the skill.

## Hosts

`usc` connects to these hosts. `usc.sh` is for `usc auth login --remote`.

**`auth`, `duo`, `bypass`** (all commands that sign in use these hosts)

```text
login.microsoftonline.com
login.live.com
us.azureauth.duosecurity.com
api-22627695.duosecurity.com
uw1.devicemanagement.duosecurity.com
account.usc.edu
usc.sh
```

**`brightspace`**

```text
brightspace.usc.edu
```

**`webreg`**

```text
webreg.usc.edu
```

**`classes`**

```text
classes.usc.edu
```

**`handshake`**

```text
usc.joinhandshake.com
```

**`directory`**

```text
uscdirectory.usc.edu
```

**`libcal`**

```text
libcal.usc.edu
libauth.com
```

**`mobileorder`**

```text
mobileorderprodapi.transactcampus.com
```

Full docs: https://usc.sh/docs/
