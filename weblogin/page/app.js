import { html, render, useEffect, useState } from "/preact.js";
import { Body, Button, Card, Check, Field, Header, Link, Note, Option, Options, Row, Text, Value } from "/ui.js";

const token = takeToken();

// The token arrives in the URL fragment, which is never sent to the server.
// Keeping it in this tab's storage lets a reload carry on.
function takeToken() {
  const fromLink = location.hash.slice(1);
  try {
    if (fromLink) {
      sessionStorage.setItem("token", fromLink);
      history.replaceState(null, "", location.pathname);
    }
    return sessionStorage.getItem("token");
  } catch {
    return fromLink;
  }
}

async function call(path, body) {
  const response = await fetch(path, {
    method: body ? "POST" : "GET",
    headers: { Authorization: `Bearer ${token}`, ...(body && { "Content-Type": "application/json" }) },
    body: body && JSON.stringify(body),
  });
  if (!response.ok) throw new Error(response.statusText);
  return response.json();
}

function App() {
  const [state, setState] = useState(null);
  const [expired, setExpired] = useState(!token);

  // A failed request falls back to reading the state, and only repeated
  // failures mean the server has gone.
  const send = async (path, body) => {
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        return setState(attempt ? await call("/api/state") : await call(path, body));
      } catch {
        await new Promise((resolve) => setTimeout(resolve, 800));
      }
    }
    setExpired(true);
  };

  useEffect(() => {
    if (token) send("/api/state");
  }, []);

  const waiting = !expired && (state?.step === "working" || state?.step === "push");
  useEffect(() => {
    if (!waiting) return;
    const timer = setInterval(() => send("/api/state"), 800);
    return () => clearInterval(timer);
  }, [waiting]);

  const finished = state?.step === "done" || state?.step === "failed";
  return html`<${Card} footer=${html`<${Footer} />`}>
    <${Screen} state=${finished || !expired ? state : { step: "expired" }} send=${send} />
  <//>`;
}

function Footer() {
  return html`<${Note} icon="lock">Local USC login<//>
    <${Link} icon="github" href="https://github.com/nsigel/usc-cli">GitHub<//>`;
}

function Screen({ state, send }) {
  switch (state?.step) {
    case "credentials":
      return html`<${Credentials} error=${state.error} send=${send} />`;
    case "method":
      return html`<${Methods} methods=${state.methods ?? []} error=${state.error} send=${send} />`;
    case "code":
      return html`<${Code} key=${state.round} prompt=${state.prompt} round=${state.round} send=${send} />`;
    case "push":
      return html`<${Push} code=${state.code} round=${state.round} />`;
    case "working":
      return html`<${Status} title="Signing in" text="Contacting USC…" busy />`;
    case "done":
      return html`<${Status} status="success" title="Signed in to USC CLI" text="You can close this tab." />`;
    case "failed":
      return html`<${Status} status="error" title="Sign-in didn't finish" text=${state.error} />`;
    case "expired":
      return html`<${Status} status="error" title="This link has expired" text="Ask for a new one to continue." />`;
    default:
      return null;
  }
}

let netID = "";

function Credentials({ error, send }) {
  const submit = (event) => {
    event.preventDefault();
    const form = new FormData(event.target);
    netID = form.get("username");
    send("/api/credentials", { username: netID, password: form.get("password") });
  };
  return html`<${Header} title="Sign in to USC CLI" error=${error} />
    <${Body} onSubmit=${submit}>
      <${Field} label="NetID" name="username" value=${netID} placeholder="netid@usc.edu" autocomplete="username" autocapitalize="none" spellcheck=${false} required autofocus=${!netID} />
      <${Field} label="Password" name="password" type="password" autocomplete="current-password" required autofocus=${!!netID} />
      <${Button} type="submit">Continue<//>
    <//>`;
}

const deviceMethods = {
  push: { icon: "phone", label: "Duo Push" },
  sms_otp: { icon: "message", label: "Text message" },
};

function Methods({ methods, error, send }) {
  const [bypass, setBypass] = useState(false);
  if (bypass) {
    const submit = (code) => send("/api/method", { type: "bypass_code", code });
    return html`<${CodeForm} title="Verify with Duo" label="Bypass code" submit=${submit}>
      <${Row}>
        <${Button} variant="ghost" type="button" icon="back" onClick=${() => setBypass(false)}>Back<//>
        <${Link} href="https://account.usc.edu/2fa/duo-bypass-code">Get a bypass code<//>
      <//>
    <//>`;
  }
  const choose = (type, device_id) => () => send("/api/method", { type, device_id });
  const ordered = [...methods].sort((a, b) => (a.type !== "push") - (b.type !== "push"));
  return html`<${Header} title="Verify with Duo" error=${error} />
    <${Options}>
      ${ordered.map(
        (method, index) => html`<${Option}
          key=${method.type + method.device_id}
          ...${deviceMethods[method.type]}
          detail=${method.name}
          tag=${index === 0 && method.type === "push" && "Recommended"}
          onClick=${choose(method.type, method.device_id)}
        />`,
      )}
      <${Option} icon="hash" label="Duo Mobile code" onClick=${choose("mobile_otp")} />
      <${Option} icon="key" label="Bypass code" onClick=${() => setBypass(true)} />
    <//>`;
}

// Duo asks twice: the first approval signs in, the second lets USC CLI add
// itself as a device so it can sign in later without asking.
function Code({ prompt, round, send }) {
  const source = prompt === "sms" ? "texted code" : "Duo Mobile code";
  const label = round > 1 ? `New ${source}` : source[0].toUpperCase() + source.slice(1);
  return html`<${CodeForm} round=${round} label=${label} submit=${(code) => send("/api/code", { code })} />`;
}

function CodeForm({ title, round, label, submit, children }) {
  const onSubmit = (event) => {
    event.preventDefault();
    submit(new FormData(event.target).get("code"));
  };
  return html`<${Header} title=${title ?? (round > 1 ? "Remember login" : "Approve sign-in")} />
    <${Body} onSubmit=${onSubmit}>
      ${round > 1 && html`<${Check}>Signed in<//>`}
      <${Field} label=${label} name="code" inputmode="numeric" autocomplete="one-time-code" required autofocus />
      <${Button} type="submit">${round > 1 ? "Remember" : "Verify"}<//>
      ${children}
    <//>`;
}

function Push({ code, round }) {
  const again = round > 1;
  return html`<${Header} title=${again ? "Remember login" : "Approve sign-in"} />
    <${Body}>
      ${again && html`<${Check}>Signed in<//>`}
      ${code && html`<${Value} label=${again ? "Enter this new code in Duo Mobile" : "Enter this code in Duo Mobile"}>${code}<//>`}
      <${Text} busy>${again ? "Approve once more in Duo Mobile…" : "Approve in Duo Mobile…"}<//>
    <//>`;
}

function Status({ status, title, text, busy }) {
  return html`<${Header} title=${title} status=${status} />
    <${Body}><${Text} busy=${busy}>${text}<//><//>`;
}

render(html`<${App} />`, document.getElementById("app"));
