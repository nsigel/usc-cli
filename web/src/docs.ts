import type { SpriteName } from "./sprites";

export const site = "https://usc.sh";

export const searchEndpoint = `${site}/.well-known/search?q={query}`;
export const searchInstructions =
  "Search usc.sh documentation for usc CLI commands, flags, sign-in, supported USC sites, and the Go library. Pass keywords in the q query parameter; returns up to 5 matching sections in Markdown.";

export const groups: { name: string; pages: { href: string; title: string; sprite?: SpriteName }[] }[] = [
  {
    name: "Start",
    pages: [
      { href: "/docs/", title: "Install" },
      { href: "/docs/auth/", title: "Sign in" },
      { href: "/docs/output/", title: "Output and errors" },
    ],
  },
  {
    name: "Sites",
    pages: [
      { href: "/docs/brightspace/", title: "Brightspace", sprite: "brightspace" },
      { href: "/docs/webreg/", title: "WebReg", sprite: "webreg" },
      { href: "/docs/handshake/", title: "Handshake", sprite: "handshake" },
      { href: "/docs/directory/", title: "Student Directory", sprite: "directory" },
      { href: "/docs/libcal/", title: "Leavey Library", sprite: "libcal" },
      { href: "/docs/mobileorder/", title: "Mobile ordering", sprite: "trojan" },
      { href: "/docs/classes/", title: "Schedule of Classes", sprite: "classes" },
    ],
  },
  {
    name: "More",
    pages: [{ href: "/docs/library/", title: "Go library" }],
  },
];

export interface Doc {
  href: string;
  title: string;
  description: string;
  body: string;
}

const sources = import.meta.glob<string>("./pages/docs/*.md", { query: "?raw", import: "default", eager: true });

// docs lists every docs page in sidebar order, with its Markdown body.
export const docs: Doc[] = groups.flatMap((group) =>
  group.pages.map(({ href }) => {
    const name = href === "/docs/" ? "index" : href.slice("/docs/".length, -1);
    const source = sources[`./pages/docs/${name}.md`];
    if (source === undefined) throw new Error(`no source for ${href}`);
    const [, front, body] = source.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/)!;
    const field = (key: string) => front.match(new RegExp(`^${key}: (.*)$`, "m"))![1];
    return { href, title: field("title"), description: field("description"), body: body.trim() };
  }),
);

// markdownPath is where the Markdown version of the page at href is served.
export function markdownPath(href: string): string {
  return `${href}index.md`;
}

// frontMatter is the YAML header of a Markdown page, advertising site search.
export function frontMatter(title: string, description: string, url: string): string {
  return [
    "---",
    `title: ${JSON.stringify(title)}`,
    `description: ${JSON.stringify(description)}`,
    `url: ${JSON.stringify(url)}`,
    "agent:",
    `  instructions: ${JSON.stringify(searchInstructions)}`,
    `  endpoint: ${JSON.stringify(searchEndpoint)}`,
    `  method: "GET"`,
    "---",
  ].join("\n");
}

export const homeTitle = "usc.sh: USC for your agent";
export const homeDescription =
  "An open-source CLI that connects your AI agent to USC's student portals — Brightspace, WebReg, Handshake, Leavey Library, Student Directory, and USC dining.";

// overview is the site summary served as /llms.txt and as the home page in Markdown.
export function overview(): string {
  const sections = groups.map((group) => {
    const links = group.pages.map((page) => {
      const doc = docs.find((doc) => doc.href === page.href)!;
      return `- [${doc.title}](${site}${markdownPath(doc.href)}): ${doc.description}`;
    });
    if (group.name === "Start") {
      links.unshift(
        `- [Agent setup](${site}/install.md): Step-by-step instructions to install usc, save the skill, and sign in.`,
      );
    }
    return `## ${group.name}\n\n${links.join("\n")}`;
  });
  return [
    "# usc.sh",
    "",
    `> ${homeDescription}`,
    "",
    `Every \`usc\` command prints JSON to stdout (except \`usc skill\`, which prints the bundled agent skill in Markdown) and writes errors as JSON to stderr with exit code 1. Search the docs with \`GET ${searchEndpoint}\` for matching Markdown sections, or request any page with \`Accept: text/markdown\`.`,
    "",
    sections.join("\n\n"),
    "",
  ].join("\n");
}
