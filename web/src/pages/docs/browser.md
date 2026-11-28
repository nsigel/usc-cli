---
layout: ../../layouts/Docs.astro
title: Browser sync
description: Copy the usc session into Chrome so USC sites open already signed in.
---

Copies your saved session cookies into Chrome. Then USC sign-in pages do not ask for a password or Duo. Use this command when an agent controls a browser.

```sh
usc browser sync [--cdp TARGET]
```

Chrome must run with remote debugging. The output does not show cookie values.

## Examples

```sh
chromium --remote-debugging-port=9222
usc browser sync
```
