---
layout: ../../layouts/Docs.astro
title: Browser sync
description: Copy the usc session into Chrome so USC sites open already signed in.
---

`usc browser sync` copies your saved session cookies into a running Chrome over the DevTools protocol. Then any USC sign-in page, like Handshake's **Student Log-in**, goes through without asking for your password or Duo. Useful when an agent drives a browser.

Start Chrome with remote debugging:

```sh
chromium --remote-debugging-port=9222
```

Then sync:

```sh
usc browser sync
usc browser sync --cdp 9224
usc browser sync --cdp 127.0.0.1:9224
usc browser sync --cdp ws://127.0.0.1:9222/devtools/browser/...
```

`--cdp` takes a port, `host:port`, an HTTP(S) endpoint, or a `ws://` debugger URL. The default is `127.0.0.1:9222`. The output counts the cookies set and skipped and never includes cookie values.
