# Set up or update the USC CLI

`usc` gives you access to the user's USC accounts: Brightspace, Web Registration, Handshake, the Student Directory, Leavey Library rooms, USC mobile ordering, and the public Schedule of Classes. Every command prints JSON. Follow these steps in order.

If `usc` is already installed, run the same steps to upgrade to the latest release and refresh the skill.

You need shell access to run commands. Private persistent storage keeps the user signed in across sessions; without it, the user will need to sign in again in each new session.

## 1. Install or update

On macOS or Linux:

```sh
curl -fsSL https://usc.sh/install | sh
```

This installs `usc` to `~/.local/bin/usc`. If `~/.local/bin` is not on `PATH`, call it by full path.

On Windows, or if Go 1.24+ is installed:

```sh
go install github.com/nsigel/usc-cli/cmd/usc@latest
```

Run `usc version` to confirm the installed binary works.

## 2. Install or update the skill

`usc skill` prints the bundled `SKILL.md` with commands and usage rules. Save it in your skills directory as `usc-cli/SKILL.md`. For Claude Code:

```sh
mkdir -p ~/.claude/skills/usc-cli
usc skill > ~/.claude/skills/usc-cli/SKILL.md
```

If your environment does not use a skills directory, run `usc skill` at the start of each USC task and follow its instructions.

## 3. Sign in

In a sandbox with outbound host approvals, sign-in stalls until the user approves each host `usc` contacts. If your environment lets you allowlist hosts ahead of time, allow the hosts in [Hosts](#hosts) now. Otherwise, open the sign-in link in your in-app browser so the user sees approval prompts as they appear. (Some sandboxes approve per exact host per session rather than permanently.)

Run `usc auth status`. If `authenticated` is `true`, skip to step 4.

Otherwise, follow the **Sign in** section of the skill (`usc skill`).

## 4. Finish

Tell the user setup (or update) is complete and share a few things they can ask:

- "when's my next exam?"
- "any Leavey rooms for 4 tomorrow afternoon?"
- "is CSCI 270 still open for spring?"
- "what career fairs are coming up?"

Follow the **Rules** section of the skill on all future USC tasks.

## Hosts

Outbound hosts `usc` connects to, grouped by command. `usc.sh` is used for the installer and `usc auth login --remote`.

**`auth`, `duo`, `bypass`** (required for sign-in and session refresh)

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
