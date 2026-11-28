import { html } from "./preact.js";

const strokes = {
  lock: html`<rect x="3" y="11" width="18" height="11" rx="2" /><path d="M7 11V7a5 5 0 0 1 10 0v4" />`,
  check: html`<circle cx="12" cy="12" r="10" /><path d="m7.5 12.5 3 3 6-7" />`,
  alert: html`<circle cx="12" cy="12" r="10" /><path d="M12 7v6M12 16.5h.01" />`,
  phone: html`<rect x="5" y="2" width="14" height="20" rx="2" /><path d="M12 18h.01" />`,
  message: html`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />`,
  hash: html`<path d="M4 9h16M4 15h16M10 3 8 21M16 3l-2 18" />`,
  key: html`<path d="m21 2-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.778 7.778 5.5 5.5 0 0 1 7.777-7.777zm0 0L15.5 7.5m0 0 3 3L22 7l-3-3m-3.5 3.5L19 4" />`,
  chevron: html`<path d="m9 18 6-6-6-6" />`,
  back: html`<path d="m12 19-7-7 7-7M19 12H5" />`,
};

const github =
  "M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z";

function Icon({ name, size = 16 }) {
  if (name === "github") {
    return html`<svg class="icon" width=${size} height=${size} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
      <path d=${github} />
    </svg>`;
  }
  return html`<svg
    class="icon"
    width=${size}
    height=${size}
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width=${size > 24 ? 1.5 : 2}
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    ${strokes[name]}
  </svg>`;
}

export function Card({ footer, children }) {
  return html`<main class="card" aria-live="polite">${children}</main>
    <footer class="footer">${footer}</footer>`;
}

export function Header({ title, status, error }) {
  return html`<header class="card-header">
    ${status && html`<span class=${status}><${Icon} name=${status === "success" ? "check" : "alert"} size=${32} /></span>`}
    <h1 class="title">${title}</h1>
    ${error && html`<p class="alert" role="alert"><${Icon} name="alert" />${error}</p>`}
  </header>`;
}

export function Body({ children, ...rest }) {
  const tag = rest.onSubmit ? "form" : "div";
  return html`<${tag} class="card-body" ...${rest}>${children}<//>`;
}

// Browsers honour the autofocus attribute once per page, so later screens focus by hand.
export function Field({ label, autofocus, ...input }) {
  return html`<label class="field">
    <span class="label">${label}</span>
    <input class="input" ref=${autofocus && ((element) => element?.focus())} ...${input} />
  </label>`;
}

export function Value({ label, children }) {
  return html`<div class="field">
    <span class="label">${label}</span>
    <span class="value">${children}</span>
  </div>`;
}

export function Text({ busy, children }) {
  return html`<p class="text" role=${busy && "status"}>${busy && html`<span class="spinner"></span>`}${children}</p>`;
}

export function Check({ children }) {
  return html`<p class="text success"><${Icon} name="check" size=${20} />${children}</p>`;
}

export function Button({ variant = "primary", icon, children, ...rest }) {
  return html`<button class=${`button button-${variant}`} ...${rest}>
    ${icon && html`<${Icon} name=${icon} />`}${children}
  </button>`;
}

export function Row({ children }) {
  return html`<div class="row">${children}</div>`;
}

export function Options({ children }) {
  return html`<div class="options">${children}</div>`;
}

export function Option({ icon, label, detail, tag, onClick }) {
  return html`<button type="button" class="option" onClick=${onClick}>
    <${Icon} name=${icon} size=${20} />
    <span class="option-label">${label}${detail && html`<span class="option-detail">${detail}</span>`}</span>
    ${tag && html`<span class="option-tag">${tag}</span>`}
    <span class="option-chevron"><${Icon} name="chevron" /></span>
  </button>`;
}

export function Note({ icon, children }) {
  return html`<span class="note"><${Icon} name=${icon} />${children}</span>`;
}

export function Link({ href, icon, children }) {
  return html`<a class=${icon ? "link link-plain" : "link"} href=${href} target="_blank" rel="noopener noreferrer">
    ${icon && html`<${Icon} name=${icon} />`}${children}
  </a>`;
}
